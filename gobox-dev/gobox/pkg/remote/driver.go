// Package remote provides the remote API driver for GoBox.
// This driver communicates with a remote GoBox server via HTTP.
package remote

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"sync"
	"time"

	"gobox/pkg/gobox"
	"gobox/pkg/protocol"
)

// Driver implements the gobox.Sandbox interface for remote servers.
type Driver struct {
	baseURL    string
	apiKey     string
	factoryID  string
	sessionID  string
	httpClient *http.Client
	mu         sync.Mutex
	closed     bool
}

// Config holds configuration for the remote driver.
type Config struct {
	// BaseURL is the base URL of the remote GoBox server.
	BaseURL string
	// APIKey is the API key for authentication.
	APIKey string
	// FactoryID identifies the factory to use.
	FactoryID string
	// SessionID is an optional session identifier for resuming sessions.
	SessionID string
	// Timeout is the default timeout for HTTP requests.
	Timeout time.Duration
}

// DefaultConfig returns a Config with default values.
func DefaultConfig() *Config {
	return &Config{
		BaseURL:   "https://codeboxapi.com/api/v2",
		FactoryID: "default",
		Timeout:   60 * time.Second,
	}
}

// Factory creates remote driver instances.
type Factory struct{}

// Name returns the driver name.
func (f *Factory) Name() string {
	return "remote"
}

// Create creates a new remote driver instance.
func (f *Factory) Create(sandboxConfig *gobox.SandboxConfig) (gobox.Sandbox, error) {
	return New(&Config{
		BaseURL:   sandboxConfig.BaseURL,
		APIKey:    sandboxConfig.APIKey,
		FactoryID: sandboxConfig.FactoryID,
		SessionID: sandboxConfig.SessionID,
		Timeout:   sandboxConfig.Timeout,
	})
}

func init() {
	// Register the remote driver factory
	gobox.RegisterDriver(&Factory{})
}

// New creates a new remote driver with the given configuration.
func New(config *Config) (*Driver, error) {
	if config == nil {
		config = DefaultConfig()
	}

	if config.APIKey == "" {
		return nil, gobox.ErrUnauthorized
	}

	if config.BaseURL == "" {
		config.BaseURL = DefaultConfig().BaseURL
	}

	// Ensure base URL doesn't have trailing slash
	config.BaseURL = strings.TrimSuffix(config.BaseURL, "/")

	return &Driver{
		baseURL:   config.BaseURL,
		apiKey:    config.APIKey,
		factoryID: config.FactoryID,
		sessionID: config.SessionID,
		httpClient: &http.Client{
			Timeout: config.Timeout,
		},
	}, nil
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
	start := time.Now()

	// Collect streaming output
	chunks := make([]gobox.ExecChunk, 0)
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

		// Build request URL
		url := d.buildURL("/exec")

		// Build request body
		reqBody := map[string]interface{}{
			"code":   code,
			"kernel": config.Kernel,
		}
		if config.Timeout > 0 {
			reqBody["timeout"] = int(config.Timeout.Seconds())
		}
		if config.Cwd != "" {
			reqBody["cwd"] = config.Cwd
		}

		jsonBody, err := json.Marshal(reqBody)
		if err != nil {
			errChan <- gobox.NewDriverError("remote", "marshal_request", err)
			return
		}

		// Create request
		req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(jsonBody))
		if err != nil {
			errChan <- gobox.NewDriverError("remote", "create_request", err)
			return
		}

		d.setHeaders(req)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/x-ndjson")

		// Execute request
		resp, err := d.httpClient.Do(req)
		if err != nil {
			errChan <- gobox.NewDriverError("remote", "http_request", err)
			return
		}
		defer resp.Body.Close()

		// Handle error responses
		if resp.StatusCode != http.StatusOK {
			d.handleErrorResponse(resp, errChan)
			return
		}

		// Parse streaming response
		d.parseStreamingResponse(ctx, resp.Body, chunkChan, errChan)
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

	for _, file := range files {
		if err := d.uploadFile(ctx, file); err != nil {
			return err
		}
	}

	return nil
}

func (d *Driver) uploadFile(ctx context.Context, file gobox.File) error {
	url := d.buildURL("/files/upload")

	// Create multipart form
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	// Add file
	part, err := writer.CreateFormFile("file", file.Name)
	if err != nil {
		return gobox.NewDriverError("remote", "create_form_file", err)
	}

	if _, err := io.Copy(part, file.Content); err != nil {
		return gobox.NewDriverError("remote", "copy_file", err)
	}

	if err := writer.Close(); err != nil {
		return gobox.NewDriverError("remote", "close_writer", err)
	}

	// Create request
	req, err := http.NewRequestWithContext(ctx, "POST", url, &buf)
	if err != nil {
		return gobox.NewDriverError("remote", "create_request", err)
	}

	d.setHeaders(req)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	// Execute request
	resp, err := d.httpClient.Do(req)
	if err != nil {
		return gobox.NewDriverError("remote", "http_request", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return d.parseErrorResponse(resp)
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

	url := d.buildURL("/files/download/" + filename)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, gobox.NewDriverError("remote", "create_request", err)
	}

	d.setHeaders(req)

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return nil, gobox.NewDriverError("remote", "http_request", err)
	}

	if resp.StatusCode == http.StatusNotFound {
		resp.Body.Close()
		return nil, gobox.ErrFileNotFound
	}

	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		return nil, d.parseErrorResponse(resp)
	}

	return resp.Body, nil
}

// ListFiles lists files in the sandbox environment.
func (d *Driver) ListFiles(ctx context.Context) ([]gobox.FileInfo, error) {
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		return nil, gobox.ErrSandboxClosed
	}
	d.mu.Unlock()

	url := d.buildURL("/files")

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, gobox.NewDriverError("remote", "create_request", err)
	}

	d.setHeaders(req)

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return nil, gobox.NewDriverError("remote", "http_request", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, d.parseErrorResponse(resp)
	}

	var files []gobox.FileInfo
	if err := json.NewDecoder(resp.Body).Decode(&files); err != nil {
		return nil, gobox.NewDriverError("remote", "decode_response", err)
	}

	return files, nil
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

	url := d.buildURL("/install")

	reqBody := map[string]interface{}{
		"packages": packages,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return gobox.NewDriverError("remote", "marshal_request", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(jsonBody))
	if err != nil {
		return gobox.NewDriverError("remote", "create_request", err)
	}

	d.setHeaders(req)
	req.Header.Set("Content-Type", "application/json")

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return gobox.NewDriverError("remote", "http_request", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return d.parseErrorResponse(resp)
	}

	return nil
}

// HealthCheck verifies the sandbox is healthy.
func (d *Driver) HealthCheck(ctx context.Context) error {
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		return gobox.ErrSandboxClosed
	}
	d.mu.Unlock()

	url := d.buildURL("/health")

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return gobox.NewDriverError("remote", "create_request", err)
	}

	d.setHeaders(req)

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return gobox.NewDriverError("remote", "http_request", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return gobox.ErrContainerNotReady
	}

	return nil
}

// Restart restarts the sandbox environment.
func (d *Driver) Restart(ctx context.Context) error {
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		return gobox.ErrSandboxClosed
	}
	d.mu.Unlock()

	url := d.buildURL("/restart")

	req, err := http.NewRequestWithContext(ctx, "POST", url, nil)
	if err != nil {
		return gobox.NewDriverError("remote", "create_request", err)
	}

	d.setHeaders(req)

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return gobox.NewDriverError("remote", "http_request", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return d.parseErrorResponse(resp)
	}

	return nil
}

// Close releases all resources.
func (d *Driver) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.closed {
		return nil
	}
	d.closed = true

	// Notify server to close session if we have one
	if d.sessionID != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		url := d.buildURL("/session/close")
		req, err := http.NewRequestWithContext(ctx, "POST", url, nil)
		if err == nil {
			d.setHeaders(req)
			resp, err := d.httpClient.Do(req)
			if err == nil {
				resp.Body.Close()
			}
		}
	}

	return nil
}

// Helper methods

func (d *Driver) buildURL(path string) string {
	url := d.baseURL + path
	if d.sessionID != "" {
		if strings.Contains(url, "?") {
			url += "&session_id=" + d.sessionID
		} else {
			url += "?session_id=" + d.sessionID
		}
	}
	return url
}

func (d *Driver) setHeaders(req *http.Request) {
	req.Header.Set("Authorization", "Bearer "+d.apiKey)
	if d.factoryID != "" {
		req.Header.Set("Factory-Id", d.factoryID)
	}
	if d.sessionID != "" {
		req.Header.Set("Session-Id", d.sessionID)
	}
	req.Header.Set("User-Agent", "GoBox/1.0")
}

func (d *Driver) handleErrorResponse(resp *http.Response, errChan chan error) {
	errChan <- d.parseErrorResponse(resp)
}

func (d *Driver) parseErrorResponse(resp *http.Response) error {
	switch resp.StatusCode {
	case http.StatusUnauthorized:
		return gobox.ErrUnauthorized
	case http.StatusNotFound:
		return gobox.ErrFileNotFound
	case http.StatusRequestTimeout, http.StatusGatewayTimeout:
		return gobox.ErrTimeout
	default:
		body, _ := io.ReadAll(resp.Body)
		return gobox.NewExecError("remote", fmt.Sprintf("HTTP %d: %s", resp.StatusCode, string(body)))
	}
}

func (d *Driver) parseStreamingResponse(ctx context.Context, body io.Reader, chunkChan chan gobox.ExecChunk, errChan chan error) {
	reader := bufio.NewReader(body)

	for {
		select {
		case <-ctx.Done():
			errChan <- ctx.Err()
			return
		default:
		}

		line, err := reader.ReadBytes('\n')
		if err == io.EOF {
			return
		}
		if err != nil {
			errChan <- gobox.NewDriverError("remote", "read_response", err)
			return
		}

		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}

		// Try parsing as JSONL first
		chunk, err := protocol.DecodeJSONL(line)
		if err == nil {
			select {
			case chunkChan <- chunk:
			case <-ctx.Done():
				errChan <- ctx.Err()
				return
			}
			continue
		}

		// Try parsing as legacy format
		chunks := protocol.ParseLegacy(string(line))
		for _, c := range chunks {
			select {
			case chunkChan <- c:
			case <-ctx.Done():
				errChan <- ctx.Err()
				return
			}
		}

		// If neither format matches, treat as plain text
		if len(chunks) == 0 {
			// Check if it's base64 encoded image
			if isBase64Image(line) {
				chunkChan <- gobox.ExecChunk{
					Type:    gobox.ChunkTypeImage,
					Content: string(line),
				}
			} else {
				chunkChan <- gobox.ExecChunk{
					Type:    gobox.ChunkTypeText,
					Content: string(line),
				}
			}
		}
	}
}

// isBase64Image checks if the data looks like a base64-encoded image.
// It performs efficient checks without decoding large amounts of data.
func isBase64Image(data []byte) bool {
	if len(data) == 0 {
		return false
	}

	str := string(data)

	// Check for data URI prefix first (most efficient)
	if strings.HasPrefix(str, "data:image/") {
		return true
	}

	// For raw base64, only check if it looks like valid base64
	// and is long enough to potentially be an image
	if len(data) < 100 {
		return false
	}

	// Check first few characters are valid base64
	const base64Chars = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/="
	for i := 0; i < 20 && i < len(str); i++ {
		if !strings.ContainsRune(base64Chars, rune(str[i])) {
			return false
		}
	}

	// Try to decode just enough to check magic bytes
	// Decode 16 bytes (which requires ~22 base64 chars)
	sample := str
	if len(sample) > 24 {
		sample = sample[:24]
	}

	decoded, err := base64.StdEncoding.DecodeString(sample)
	if err != nil {
		return false
	}

	// Check for PNG/JPEG magic bytes
	if len(decoded) >= 4 {
		// PNG magic bytes: 89 50 4E 47
		if decoded[0] == 0x89 && decoded[1] == 'P' && decoded[2] == 'N' && decoded[3] == 'G' {
			return true
		}
		// JPEG magic bytes: FF D8 FF
		if decoded[0] == 0xFF && decoded[1] == 0xD8 {
			return true
		}
	}

	return false
}
