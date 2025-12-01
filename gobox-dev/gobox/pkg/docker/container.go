package docker

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"gobox/pkg/gobox"
)

// Container represents a Docker container instance.
type Container struct {
	// ID is the container ID.
	ID string
	// Status is the current container status.
	Status string
	// CreatedAt is when the container was created.
	CreatedAt time.Time
	// LastUsed is when the container was last used.
	LastUsed time.Time
	// Port is the port the container's internal API is exposed on.
	Port int

	mu     sync.RWMutex
	driver *Driver
}

// IsHealthy checks if the container is healthy and ready for use.
func (c *Container) IsHealthy() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.Status == "running"
}

// Reset resets the container state for reuse.
// This clears any temporary files and resets the execution environment.
func (c *Container) Reset(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Execute cleanup commands inside the container
	cleanupCode := `
import shutil
import os
import glob

# Clear /app directory except for essential files
for f in glob.glob('/app/*'):
    if os.path.isfile(f):
        os.remove(f)
    elif os.path.isdir(f):
        shutil.rmtree(f)
`

	if c.driver != nil {
		_, err := c.driver.execInContainer(ctx, c.ID, cleanupCode, "python")
		return err
	}

	return nil
}

// Exec executes code in this container.
func (c *Container) Exec(ctx context.Context, code string, kernel string) (*gobox.ExecResult, error) {
	c.mu.Lock()
	c.LastUsed = time.Now()
	c.mu.Unlock()

	if c.driver == nil {
		return nil, gobox.NewDriverError("docker", "exec", fmt.Errorf("container driver not set"))
	}

	return c.driver.execInContainer(ctx, c.ID, code, kernel)
}

// StreamExec executes code in this container with streaming output.
func (c *Container) StreamExec(ctx context.Context, code string, kernel string) (<-chan gobox.ExecChunk, <-chan error) {
	c.mu.Lock()
	c.LastUsed = time.Now()
	c.mu.Unlock()

	if c.driver == nil {
		errCh := make(chan error, 1)
		errCh <- gobox.NewDriverError("docker", "stream_exec", fmt.Errorf("container driver not set"))
		close(errCh)
		return nil, errCh
	}

	return c.driver.streamExecInContainer(ctx, c.ID, code, kernel)
}

// Upload uploads a file to the container.
func (c *Container) Upload(ctx context.Context, file gobox.File) error {
	c.mu.Lock()
	c.LastUsed = time.Now()
	c.mu.Unlock()

	if c.driver == nil {
		return gobox.NewDriverError("docker", "upload", fmt.Errorf("container driver not set"))
	}

	return c.driver.uploadToContainer(ctx, c.ID, file)
}

// Download downloads a file from the container.
func (c *Container) Download(ctx context.Context, filename string) (io.ReadCloser, error) {
	c.mu.Lock()
	c.LastUsed = time.Now()
	c.mu.Unlock()

	if c.driver == nil {
		return nil, gobox.NewDriverError("docker", "download", fmt.Errorf("container driver not set"))
	}

	return c.driver.downloadFromContainer(ctx, c.ID, filename)
}

// ListFiles lists files in the container.
func (c *Container) ListFiles(ctx context.Context) ([]gobox.FileInfo, error) {
	c.mu.Lock()
	c.LastUsed = time.Now()
	c.mu.Unlock()

	if c.driver == nil {
		return nil, gobox.NewDriverError("docker", "list_files", fmt.Errorf("container driver not set"))
	}

	return c.driver.listFilesInContainer(ctx, c.ID)
}

// Stop stops the container.
func (c *Container) Stop(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.driver == nil {
		return nil
	}

	c.Status = "stopped"
	return c.driver.stopContainer(ctx, c.ID)
}

// Destroy stops and removes the container.
func (c *Container) Destroy(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.driver == nil {
		return nil
	}

	c.Status = "destroyed"
	return c.driver.destroyContainer(ctx, c.ID)
}

// Age returns how long the container has been running.
func (c *Container) Age() time.Duration {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return time.Since(c.CreatedAt)
}

// IdleTime returns how long since the container was last used.
func (c *Container) IdleTime() time.Duration {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return time.Since(c.LastUsed)
}

// Stats returns resource usage statistics for the container.
type ContainerStats struct {
	CPUPercent    float64
	MemoryUsage   int64
	MemoryLimit   int64
	MemoryPercent float64
	NetworkRx     int64
	NetworkTx     int64
}

// Stats retrieves resource usage statistics for the container.
func (c *Container) Stats(ctx context.Context) (*ContainerStats, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.driver == nil {
		return nil, gobox.NewDriverError("docker", "stats", fmt.Errorf("container driver not set"))
	}

	return c.driver.getContainerStats(ctx, c.ID)
}
