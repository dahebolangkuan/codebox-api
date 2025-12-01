package docker

import (
	"archive/tar"
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"

	"gobox/pkg/gobox"
	"gobox/pkg/protocol"
)

// Driver implements the gobox.Sandbox interface using Docker containers.
type Driver struct {
	cli       *client.Client
	config    *Config
	pool      *Pool
	container *Container
	mu        sync.Mutex
	closed    bool
}

// Factory creates Docker driver instances.
type Factory struct{}

// Name returns the driver name.
func (f *Factory) Name() string {
	return "docker"
}

// Create creates a new Docker driver instance.
func (f *Factory) Create(sandboxConfig *gobox.SandboxConfig) (gobox.Sandbox, error) {
	config := &Config{
		Image:       sandboxConfig.Image,
		Timeout:     sandboxConfig.Timeout,
		Memory:      sandboxConfig.Memory,
		CPUQuota:    sandboxConfig.CPUQuota,
		NetworkMode: sandboxConfig.NetworkMode,
		WorkDir:     "/app",
		AutoRemove:  true,
	}

	if sandboxConfig.PoolConfig != nil {
		config.PoolConfig = &PoolConfig{
			MinSize:     sandboxConfig.PoolConfig.MinSize,
			MaxSize:     sandboxConfig.PoolConfig.MaxSize,
			IdleTimeout: sandboxConfig.PoolConfig.IdleTimeout,
			WarmupCount: sandboxConfig.PoolConfig.WarmupCount,
		}
	}

	return New(
		WithImage(config.Image),
		WithTimeout(config.Timeout),
		WithMemory(config.Memory),
		WithCPUQuota(config.CPUQuota),
		WithNetworkMode(config.NetworkMode),
		WithPoolConfig(config.PoolConfig),
	)
}

func init() {
	// Register the Docker driver factory
	gobox.RegisterDriver(&Factory{})
}

// New creates a new Docker driver with the given options.
func New(opts ...Option) (*Driver, error) {
	config := ApplyOptions(opts...)

	// Create Docker client
	cli, err := client.NewClientWithOpts(
		client.FromEnv,
		client.WithAPIVersionNegotiation(),
	)
	if err != nil {
		return nil, gobox.NewDriverError("docker", "new", err)
	}

	// Verify Docker is accessible
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err = cli.Ping(ctx)
	if err != nil {
		cli.Close()
		return nil, gobox.NewDriverError("docker", "ping", err)
	}

	driver := &Driver{
		cli:    cli,
		config: config,
	}

	// Create container pool if configured
	if config.PoolConfig != nil && config.PoolConfig.MaxSize > 0 {
		pool, err := NewPool(driver, config.PoolConfig)
		if err != nil {
			cli.Close()
			return nil, gobox.NewDriverError("docker", "create_pool", err)
		}
		driver.pool = pool
	}

	return driver, nil
}

// Exec executes code and returns the complete result.
func (d *Driver) Exec(ctx context.Context, code string, opts ...gobox.ExecOption) (*gobox.ExecResult, error) {
	if code == "" {
		return nil, gobox.ErrInvalidCode
	}

	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		return nil, gobox.ErrSandboxClosed
	}
	d.mu.Unlock()

	config := gobox.ApplyExecOptions(opts...)

	// Validate kernel
	if config.Kernel != "python" && config.Kernel != "bash" {
		return nil, gobox.ErrInvalidKernel
	}

	// Apply timeout
	if config.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, config.Timeout)
		defer cancel()
	}

	// Collect streaming output
	chunks := make([]gobox.ExecChunk, 0)
	start := time.Now()

	chunkChan, errChan := d.StreamExec(ctx, code, opts...)

	for {
		select {
		case chunk, ok := <-chunkChan:
			if !ok {
				// Channel closed, check for errors
				select {
				case err := <-errChan:
					if err != nil {
						return nil, err
					}
				default:
				}
				return &gobox.ExecResult{
					Chunks:   chunks,
					Duration: time.Since(start),
				}, nil
			}
			chunks = append(chunks, chunk)

		case err := <-errChan:
			if err != nil {
				return nil, err
			}

		case <-ctx.Done():
			return nil, gobox.NewTimeoutError(config.Timeout.String())
		}
	}
}

// StreamExec executes code and returns streaming output.
func (d *Driver) StreamExec(ctx context.Context, code string, opts ...gobox.ExecOption) (<-chan gobox.ExecChunk, <-chan error) {
	chunkChan := make(chan gobox.ExecChunk, 100)
	errChan := make(chan error, 1)

	go func() {
		defer close(chunkChan)
		defer close(errChan)

		d.mu.Lock()
		if d.closed {
			d.mu.Unlock()
			errChan <- gobox.ErrSandboxClosed
			return
		}
		d.mu.Unlock()

		config := gobox.ApplyExecOptions(opts...)

		// Get or create container
		container, err := d.getContainer(ctx)
		if err != nil {
			errChan <- err
			return
		}

		// Execute in container and stream output
		resultChunks, err := d.execInContainer(ctx, container.ID, code, config.Kernel)
		if err != nil {
			errChan <- err
			return
		}

		// Send chunks
		for _, chunk := range resultChunks.Chunks {
			select {
			case chunkChan <- chunk:
			case <-ctx.Done():
				errChan <- ctx.Err()
				return
			}
		}
	}()

	return chunkChan, errChan
}

// Upload uploads files to the sandbox environment.
func (d *Driver) Upload(ctx context.Context, files ...gobox.File) error {
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		return gobox.ErrSandboxClosed
	}
	d.mu.Unlock()

	container, err := d.getContainer(ctx)
	if err != nil {
		return err
	}

	for _, file := range files {
		if err := d.uploadToContainer(ctx, container.ID, file); err != nil {
			return err
		}
	}

	return nil
}

// Download retrieves a file from the sandbox environment.
func (d *Driver) Download(ctx context.Context, filename string) (io.ReadCloser, error) {
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		return nil, gobox.ErrSandboxClosed
	}
	d.mu.Unlock()

	container, err := d.getContainer(ctx)
	if err != nil {
		return nil, err
	}

	return d.downloadFromContainer(ctx, container.ID, filename)
}

// ListFiles lists files in the sandbox environment.
func (d *Driver) ListFiles(ctx context.Context) ([]gobox.FileInfo, error) {
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		return nil, gobox.ErrSandboxClosed
	}
	d.mu.Unlock()

	container, err := d.getContainer(ctx)
	if err != nil {
		return nil, err
	}

	return d.listFilesInContainer(ctx, container.ID)
}

// Install installs Python packages.
func (d *Driver) Install(ctx context.Context, packages ...string) error {
	if len(packages) == 0 {
		return nil
	}

	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		return gobox.ErrSandboxClosed
	}
	d.mu.Unlock()

	container, err := d.getContainer(ctx)
	if err != nil {
		return err
	}

	// Install using pip
	cmd := "pip install " + strings.Join(packages, " ")
	_, err = d.execInContainer(ctx, container.ID, cmd, "bash")
	return err
}

// HealthCheck verifies the sandbox is healthy.
func (d *Driver) HealthCheck(ctx context.Context) error {
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		return gobox.ErrSandboxClosed
	}
	d.mu.Unlock()

	// Try to execute a simple command
	result, err := d.Exec(ctx, "print('ok')")
	if err != nil {
		return err
	}

	if !strings.Contains(result.Text(), "ok") {
		return gobox.ErrContainerNotReady
	}

	return nil
}

// Restart restarts the sandbox environment.
func (d *Driver) Restart(ctx context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.closed {
		return gobox.ErrSandboxClosed
	}

	// Destroy current container
	if d.container != nil {
		d.container.Destroy(ctx)
		d.container = nil
	}

	// Get a fresh container
	_, err := d.getContainer(ctx)
	return err
}

// Close releases all resources.
func (d *Driver) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.closed {
		return nil
	}
	d.closed = true

	// Close pool if exists
	if d.pool != nil {
		d.pool.Close()
	}

	// Destroy container if exists
	if d.container != nil {
		d.container.Destroy(context.Background())
	}

	// Close Docker client
	return d.cli.Close()
}

// getContainer returns a container for execution, either from the pool or creating a new one.
func (d *Driver) getContainer(ctx context.Context) (*Container, error) {
	// If we have a container, use it
	if d.container != nil && d.container.IsHealthy() {
		return d.container, nil
	}

	// If we have a pool, get from pool
	if d.pool != nil {
		container, err := d.pool.Get(ctx)
		if err != nil {
			return nil, err
		}
		d.container = container
		return container, nil
	}

	// Create a new container
	container, err := d.createContainer(ctx)
	if err != nil {
		return nil, err
	}
	d.container = container
	return container, nil
}

// createContainer creates a new Docker container.
func (d *Driver) createContainer(ctx context.Context) (*Container, error) {
	// Ensure image is available
	if err := d.ensureImage(ctx); err != nil {
		return nil, err
	}

	// Create container configuration
	containerConfig := &container.Config{
		Image:      d.config.Image,
		Tty:        true,
		OpenStdin:  true,
		WorkingDir: d.config.WorkDir,
		Env:        mapToEnvSlice(d.config.Env),
		Labels:     d.config.Labels,
		ExposedPorts: nat.PortSet{
			"8888/tcp": struct{}{},
		},
	}

	// Create host configuration with resource limits
	hostConfig := &container.HostConfig{
		AutoRemove:  d.config.AutoRemove,
		NetworkMode: container.NetworkMode(d.config.NetworkMode),
		Resources: container.Resources{
			Memory:   d.config.Memory,
			CPUQuota: d.config.CPUQuota,
		},
		PortBindings: nat.PortMap{
			"8888/tcp": []nat.PortBinding{
				{HostIP: "0.0.0.0", HostPort: "0"}, // Let Docker assign a port
			},
		},
		Mounts: []mount.Mount{},
	}

	if d.config.Privileged {
		hostConfig.Privileged = true
	}

	// Create the container
	resp, err := d.cli.ContainerCreate(ctx, containerConfig, hostConfig, nil, nil, "")
	if err != nil {
		return nil, gobox.NewDriverError("docker", "create_container", err)
	}

	// Start the container
	if err := d.cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		// Clean up on failure
		d.cli.ContainerRemove(ctx, resp.ID, container.RemoveOptions{Force: true})
		return nil, gobox.NewDriverError("docker", "start_container", err)
	}

	// Get the assigned port
	inspect, err := d.cli.ContainerInspect(ctx, resp.ID)
	if err != nil {
		d.cli.ContainerRemove(ctx, resp.ID, container.RemoveOptions{Force: true})
		return nil, gobox.NewDriverError("docker", "inspect_container", err)
	}

	port := 8888
	if portBindings, ok := inspect.NetworkSettings.Ports["8888/tcp"]; ok && len(portBindings) > 0 {
		fmt.Sscanf(portBindings[0].HostPort, "%d", &port)
	}

	// Wait for container to be ready
	if err := d.waitForContainerReady(ctx, resp.ID); err != nil {
		d.cli.ContainerRemove(ctx, resp.ID, container.RemoveOptions{Force: true})
		return nil, err
	}

	return &Container{
		ID:        resp.ID,
		Status:    "running",
		CreatedAt: time.Now(),
		LastUsed:  time.Now(),
		Port:      port,
		driver:    d,
	}, nil
}

// ensureImage ensures the Docker image is available locally.
func (d *Driver) ensureImage(ctx context.Context) error {
	// Check if image exists locally
	images, err := d.cli.ImageList(ctx, image.ListOptions{
		Filters: filters.NewArgs(filters.Arg("reference", d.config.Image)),
	})
	if err != nil {
		return gobox.NewDriverError("docker", "list_images", err)
	}

	if len(images) > 0 {
		return nil // Image exists
	}

	// Pull the image
	out, err := d.cli.ImagePull(ctx, d.config.Image, image.PullOptions{})
	if err != nil {
		return gobox.NewDriverError("docker", "pull_image", err)
	}
	defer out.Close()

	// Wait for pull to complete
	io.Copy(io.Discard, out)

	return nil
}

// waitForContainerReady waits for the container to be ready for execution.
func (d *Driver) waitForContainerReady(ctx context.Context, containerID string) error {
	// Wait for container to be in running state
	for i := 0; i < 30; i++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}

		inspect, err := d.cli.ContainerInspect(ctx, containerID)
		if err != nil {
			return gobox.NewDriverError("docker", "inspect", err)
		}

		if inspect.State.Running {
			// Try a simple exec to verify Python is ready
			result, err := d.execInContainer(ctx, containerID, "print('ready')", "python")
			if err == nil && strings.Contains(result.Text(), "ready") {
				return nil
			}
		}
	}

	return gobox.ErrContainerNotReady
}

// execInContainer executes code inside a container.
func (d *Driver) execInContainer(ctx context.Context, containerID, code, kernel string) (*gobox.ExecResult, error) {
	var cmd []string
	switch kernel {
	case "python":
		cmd = []string{"python", "-c", code}
	case "bash":
		cmd = []string{"bash", "-c", code}
	default:
		return nil, gobox.ErrInvalidKernel
	}

	execConfig := container.ExecOptions{
		Cmd:          cmd,
		AttachStdout: true,
		AttachStderr: true,
		Tty:          false,
	}

	execID, err := d.cli.ContainerExecCreate(ctx, containerID, execConfig)
	if err != nil {
		return nil, gobox.NewDriverError("docker", "exec_create", err)
	}

	resp, err := d.cli.ContainerExecAttach(ctx, execID.ID, container.ExecStartOptions{})
	if err != nil {
		return nil, gobox.NewDriverError("docker", "exec_attach", err)
	}
	defer resp.Close()

	// Read all output
	var stdout, stderr bytes.Buffer
	_, err = stdCopy(&stdout, &stderr, resp.Reader)
	if err != nil && err != io.EOF {
		// Try reading as plain text (when TTY is enabled)
		_, err = io.Copy(&stdout, resp.Reader)
		if err != nil && err != io.EOF {
			return nil, gobox.NewDriverError("docker", "exec_read", err)
		}
	}

	// Build chunks from output
	chunks := make([]gobox.ExecChunk, 0)

	if stdout.Len() > 0 {
		chunks = append(chunks, gobox.ExecChunk{
			Type:    gobox.ChunkTypeText,
			Content: stdout.String(),
		})
	}

	if stderr.Len() > 0 {
		chunks = append(chunks, gobox.ExecChunk{
			Type:    gobox.ChunkTypeError,
			Content: stderr.String(),
		})
	}

	// Check exit code
	execInspect, err := d.cli.ContainerExecInspect(ctx, execID.ID)
	exitCode := 0
	if err == nil {
		exitCode = execInspect.ExitCode
	}

	return &gobox.ExecResult{
		Chunks:   chunks,
		ExitCode: exitCode,
	}, nil
}

// streamExecInContainer executes code and streams output.
func (d *Driver) streamExecInContainer(ctx context.Context, containerID, code, kernel string) (<-chan gobox.ExecChunk, <-chan error) {
	chunkChan := make(chan gobox.ExecChunk, 100)
	errChan := make(chan error, 1)

	go func() {
		defer close(chunkChan)
		defer close(errChan)

		result, err := d.execInContainer(ctx, containerID, code, kernel)
		if err != nil {
			errChan <- err
			return
		}

		for _, chunk := range result.Chunks {
			select {
			case chunkChan <- chunk:
			case <-ctx.Done():
				errChan <- ctx.Err()
				return
			}
		}
	}()

	return chunkChan, errChan
}

// uploadToContainer uploads a file to the container.
func (d *Driver) uploadToContainer(ctx context.Context, containerID string, file gobox.File) error {
	// Create a tar archive containing the file
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)

	// Read file content
	content, err := io.ReadAll(file.Content)
	if err != nil {
		return gobox.NewDriverError("docker", "read_file", err)
	}

	// Add file to tar
	hdr := &tar.Header{
		Name: file.Name,
		Mode: 0644,
		Size: int64(len(content)),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return gobox.NewDriverError("docker", "tar_header", err)
	}
	if _, err := tw.Write(content); err != nil {
		return gobox.NewDriverError("docker", "tar_write", err)
	}
	if err := tw.Close(); err != nil {
		return gobox.NewDriverError("docker", "tar_close", err)
	}

	// Copy to container
	err = d.cli.CopyToContainer(ctx, containerID, d.config.WorkDir, &buf, container.CopyToContainerOptions{})
	if err != nil {
		return gobox.NewDriverError("docker", "copy_to_container", err)
	}

	return nil
}

// downloadFromContainer downloads a file from the container.
func (d *Driver) downloadFromContainer(ctx context.Context, containerID, filename string) (io.ReadCloser, error) {
	path := filepath.Join(d.config.WorkDir, filename)

	reader, _, err := d.cli.CopyFromContainer(ctx, containerID, path)
	if err != nil {
		return nil, gobox.NewDriverError("docker", "copy_from_container", err)
	}

	// Extract file from tar archive
	tr := tar.NewReader(reader)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			reader.Close()
			return nil, gobox.ErrFileNotFound
		}
		if err != nil {
			reader.Close()
			return nil, gobox.NewDriverError("docker", "tar_read", err)
		}

		if hdr.Typeflag == tar.TypeReg {
			// Read file content
			content, err := io.ReadAll(tr)
			if err != nil {
				reader.Close()
				return nil, gobox.NewDriverError("docker", "read_tar_file", err)
			}
			reader.Close()

			return io.NopCloser(bytes.NewReader(content)), nil
		}
	}
}

// listFilesInContainer lists files in the container's work directory.
func (d *Driver) listFilesInContainer(ctx context.Context, containerID string) ([]gobox.FileInfo, error) {
	// Use Python to list files as JSON
	code := `
import os
import json
files = []
for entry in os.listdir('.'):
    stat = os.stat(entry)
    files.append({
        'path': entry,
        'size': stat.st_size,
        'is_dir': os.path.isdir(entry)
    })
print(json.dumps(files))
`

	result, err := d.execInContainer(ctx, containerID, code, "python")
	if err != nil {
		return nil, err
	}

	// Parse JSON output
	var files []gobox.FileInfo
	if err := json.Unmarshal([]byte(strings.TrimSpace(result.Text())), &files); err != nil {
		return nil, gobox.NewDriverError("docker", "parse_files", err)
	}

	return files, nil
}

// stopContainer stops a container.
func (d *Driver) stopContainer(ctx context.Context, containerID string) error {
	timeout := 10
	return d.cli.ContainerStop(ctx, containerID, container.StopOptions{Timeout: &timeout})
}

// destroyContainer stops and removes a container.
func (d *Driver) destroyContainer(ctx context.Context, containerID string) error {
	return d.cli.ContainerRemove(ctx, containerID, container.RemoveOptions{Force: true})
}

// getContainerStats retrieves resource usage statistics.
func (d *Driver) getContainerStats(ctx context.Context, containerID string) (*ContainerStats, error) {
	resp, err := d.cli.ContainerStats(ctx, containerID, false)
	if err != nil {
		return nil, gobox.NewDriverError("docker", "stats", err)
	}
	defer resp.Body.Close()

	var stats struct {
		CPUStats struct {
			CPUUsage struct {
				TotalUsage uint64 `json:"total_usage"`
			} `json:"cpu_usage"`
			SystemCPUUsage uint64 `json:"system_cpu_usage"`
		} `json:"cpu_stats"`
		PreCPUStats struct {
			CPUUsage struct {
				TotalUsage uint64 `json:"total_usage"`
			} `json:"cpu_usage"`
			SystemCPUUsage uint64 `json:"system_cpu_usage"`
		} `json:"precpu_stats"`
		MemoryStats struct {
			Usage uint64 `json:"usage"`
			Limit uint64 `json:"limit"`
		} `json:"memory_stats"`
		Networks map[string]struct {
			RxBytes uint64 `json:"rx_bytes"`
			TxBytes uint64 `json:"tx_bytes"`
		} `json:"networks"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&stats); err != nil {
		return nil, gobox.NewDriverError("docker", "decode_stats", err)
	}

	// Calculate CPU percentage
	cpuDelta := float64(stats.CPUStats.CPUUsage.TotalUsage - stats.PreCPUStats.CPUUsage.TotalUsage)
	systemDelta := float64(stats.CPUStats.SystemCPUUsage - stats.PreCPUStats.SystemCPUUsage)
	cpuPercent := 0.0
	if systemDelta > 0 {
		cpuPercent = (cpuDelta / systemDelta) * 100.0
	}

	// Calculate memory percentage
	memPercent := 0.0
	if stats.MemoryStats.Limit > 0 {
		memPercent = float64(stats.MemoryStats.Usage) / float64(stats.MemoryStats.Limit) * 100.0
	}

	// Sum network stats
	var networkRx, networkTx int64
	for _, net := range stats.Networks {
		networkRx += int64(net.RxBytes)
		networkTx += int64(net.TxBytes)
	}

	return &ContainerStats{
		CPUPercent:    cpuPercent,
		MemoryUsage:   int64(stats.MemoryStats.Usage),
		MemoryLimit:   int64(stats.MemoryStats.Limit),
		MemoryPercent: memPercent,
		NetworkRx:     networkRx,
		NetworkTx:     networkTx,
	}, nil
}

// Helper functions

func mapToEnvSlice(env map[string]string) []string {
	result := make([]string, 0, len(env))
	for k, v := range env {
		result = append(result, k+"="+v)
	}
	return result
}

// stdCopy demultiplexes the Docker stream format used when TTY is disabled.
//
// Docker uses a multiplexed stream format where stdout and stderr are combined
// into a single stream. Each message is prefixed with an 8-byte header:
//
// Header format:
//
//	[0]      - Stream type: 0=stdin (unused for output), 1=stdout, 2=stderr
//	[1-3]    - Reserved (always 0)
//	[4-7]    - Frame size as 32-bit big-endian unsigned integer
//
// After the header, the frame data follows with exactly the number of bytes
// specified in the size field.
//
// This function reads frames from src and writes them to the appropriate
// destination (stdout or stderr) based on the stream type byte.
//
// Reference: https://docs.docker.com/engine/api/v1.43/#operation/ContainerAttach
func stdCopy(stdout, stderr io.Writer, src io.Reader) (written int64, err error) {
	reader := bufio.NewReader(src)
	header := make([]byte, 8)

	for {
		_, err := io.ReadFull(reader, header)
		if err == io.EOF {
			return written, nil
		}
		if err != nil {
			return written, err
		}

		// Get stream type and size from header
		streamType := header[0]
		size := int64(header[4])<<24 | int64(header[5])<<16 | int64(header[6])<<8 | int64(header[7])

		// Select destination writer based on stream type
		var dest io.Writer
		switch streamType {
		case 1: // stdout
			dest = stdout
		case 2: // stderr
			dest = stderr
		default: // stdin (0) or unknown - discard
			dest = io.Discard
		}

		// Copy frame data to destination
		n, err := io.CopyN(dest, reader, size)
		written += n
		if err != nil {
			return written, err
		}
	}
}

// execCodeViaHTTP executes code via the container's HTTP API (if available).
// This is an alternative to exec for containers that expose an HTTP API.
func (d *Driver) execCodeViaHTTP(ctx context.Context, containerPort int, code, kernel string) (*gobox.ExecResult, error) {
	url := fmt.Sprintf("http://localhost:%d/exec", containerPort)

	reqBody := map[string]string{
		"code":   code,
		"kernel": kernel,
	}
	reqJSON, err := json.Marshal(reqBody)
	if err != nil {
		return nil, gobox.NewDriverError("docker", "marshal_request", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(reqJSON))
	if err != nil {
		return nil, gobox.NewDriverError("docker", "create_request", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: d.config.Timeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, gobox.NewDriverError("docker", "http_request", err)
	}
	defer resp.Body.Close()

	// Parse streaming response
	chunks := make([]gobox.ExecChunk, 0)
	decoder := protocol.NewJSONLDecoder(resp.Body)

	for {
		chunk, err := decoder.Decode()
		if err == io.EOF {
			break
		}
		if err != nil {
			// Try parsing as legacy format
			body, _ := io.ReadAll(resp.Body)
			chunks = protocol.ParseLegacy(string(body))
			break
		}
		chunks = append(chunks, chunk)
	}

	return &gobox.ExecResult{Chunks: chunks}, nil
}
