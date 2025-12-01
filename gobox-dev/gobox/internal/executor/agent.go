// Package executor provides the internal agent that runs inside containers.
// This agent handles code execution requests and returns streaming results.
package executor

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Agent handles code execution inside a container.
type Agent struct {
	workDir string
	server  *http.Server
	mu      sync.Mutex
}

// Config holds configuration for the agent.
type Config struct {
	// WorkDir is the working directory for code execution.
	WorkDir string
	// Port is the HTTP port to listen on.
	Port int
	// ReadTimeout is the HTTP read timeout.
	ReadTimeout time.Duration
	// WriteTimeout is the HTTP write timeout.
	WriteTimeout time.Duration
}

// DefaultConfig returns default agent configuration.
func DefaultConfig() *Config {
	return &Config{
		WorkDir:      "/app",
		Port:         8888,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 300 * time.Second,
	}
}

// NewAgent creates a new executor agent.
func NewAgent(config *Config) *Agent {
	if config == nil {
		config = DefaultConfig()
	}

	return &Agent{
		workDir: config.WorkDir,
	}
}

// ExecRequest represents a code execution request.
type ExecRequest struct {
	Code    string `json:"code"`
	Kernel  string `json:"kernel"`
	Timeout int    `json:"timeout,omitempty"`
	Cwd     string `json:"cwd,omitempty"`
}

// ExecChunk represents an output chunk.
type ExecChunk struct {
	Type    string `json:"type"`
	Content string `json:"content"`
}

// Run starts the HTTP server and listens for requests.
func (a *Agent) Run(config *Config) error {
	if config == nil {
		config = DefaultConfig()
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /", a.handleHealth)
	mux.HandleFunc("GET /health", a.handleHealth)
	mux.HandleFunc("POST /exec", a.handleExec)
	mux.HandleFunc("POST /files/upload", a.handleUpload)
	mux.HandleFunc("GET /files/download/{filename...}", a.handleDownload)
	mux.HandleFunc("GET /files", a.handleListFiles)

	a.server = &http.Server{
		Addr:         fmt.Sprintf(":%d", config.Port),
		Handler:      mux,
		ReadTimeout:  config.ReadTimeout,
		WriteTimeout: config.WriteTimeout,
	}

	return a.server.ListenAndServe()
}

// Shutdown gracefully shuts down the agent.
func (a *Agent) Shutdown(ctx context.Context) error {
	if a.server != nil {
		return a.server.Shutdown(ctx)
	}
	return nil
}

// handleHealth responds to health check requests.
func (a *Agent) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "ok",
	})
}

// handleExec handles code execution requests with streaming output.
func (a *Agent) handleExec(w http.ResponseWriter, r *http.Request) {
	var req ExecRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Set streaming headers
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	// Execute code
	a.mu.Lock()
	defer a.mu.Unlock()

	ctx := r.Context()
	if req.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(req.Timeout)*time.Second)
		defer cancel()
	}

	// Choose execution method based on kernel
	var cmd *exec.Cmd
	switch req.Kernel {
	case "python", "":
		cmd = exec.CommandContext(ctx, "python", "-c", req.Code)
	case "bash":
		cmd = exec.CommandContext(ctx, "bash", "-c", req.Code)
	default:
		a.sendChunk(w, flusher, ExecChunk{Type: "err", Content: "unsupported kernel: " + req.Kernel})
		return
	}

	// Set working directory
	workDir := a.workDir
	if req.Cwd != "" {
		workDir = req.Cwd
	}
	cmd.Dir = workDir

	// Create pipes for stdout and stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		a.sendChunk(w, flusher, ExecChunk{Type: "err", Content: err.Error()})
		return
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		a.sendChunk(w, flusher, ExecChunk{Type: "err", Content: err.Error()})
		return
	}

	// Start command
	if err := cmd.Start(); err != nil {
		a.sendChunk(w, flusher, ExecChunk{Type: "err", Content: err.Error()})
		return
	}

	// Read output in goroutines
	var wg sync.WaitGroup
	wg.Add(2)

	// Read stdout
	go func() {
		defer wg.Done()
		a.streamOutput(w, flusher, stdout, "txt")
	}()

	// Read stderr
	go func() {
		defer wg.Done()
		a.streamOutput(w, flusher, stderr, "err")
	}()

	// Wait for output to be read
	wg.Wait()

	// Wait for command to complete
	if err := cmd.Wait(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			a.sendChunk(w, flusher, ExecChunk{Type: "err", Content: "execution timeout"})
		}
	}
}

// streamOutput reads from reader and sends chunks.
func (a *Agent) streamOutput(w http.ResponseWriter, flusher http.Flusher, reader io.Reader, chunkType string) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)

	for scanner.Scan() {
		content := scanner.Text() + "\n"
		a.sendChunk(w, flusher, ExecChunk{Type: chunkType, Content: content})
	}
}

// sendChunk sends a single chunk to the response.
func (a *Agent) sendChunk(w http.ResponseWriter, flusher http.Flusher, chunk ExecChunk) {
	data, _ := json.Marshal(chunk)
	w.Write(data)
	w.Write([]byte("\n"))
	flusher.Flush()
}

// handleUpload handles file upload requests.
func (a *Agent) handleUpload(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(100 * 1024 * 1024); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	uploaded := 0
	for _, fhs := range r.MultipartForm.File {
		for _, fh := range fhs {
			src, err := fh.Open()
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}

			dstPath := filepath.Join(a.workDir, fh.Filename)
			dst, err := os.Create(dstPath)
			if err != nil {
				src.Close()
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			_, err = io.Copy(dst, src)
			src.Close()
			dst.Close()

			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			uploaded++
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]int{"uploaded": uploaded})
}

// handleDownload handles file download requests.
func (a *Agent) handleDownload(w http.ResponseWriter, r *http.Request) {
	filename := r.PathValue("filename")
	if filename == "" {
		http.Error(w, "filename required", http.StatusBadRequest)
		return
	}

	filePath := filepath.Join(a.workDir, filename)

	// Security check: ensure path is within workDir
	absPath, err := filepath.Abs(filePath)
	if err != nil || !strings.HasPrefix(absPath, a.workDir) {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}

	file, err := os.Open(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			http.Error(w, "file not found", http.StatusNotFound)
		} else {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}
	defer file.Close()

	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	w.Header().Set("Content-Type", "application/octet-stream")
	io.Copy(w, file)
}

// handleListFiles handles file listing requests.
func (a *Agent) handleListFiles(w http.ResponseWriter, r *http.Request) {
	entries, err := os.ReadDir(a.workDir)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	type FileInfo struct {
		Path  string `json:"path"`
		Size  int64  `json:"size"`
		IsDir bool   `json:"is_dir"`
	}

	files := make([]FileInfo, 0, len(entries))
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}

		files = append(files, FileInfo{
			Path:  entry.Name(),
			Size:  info.Size(),
			IsDir: entry.IsDir(),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(files)
}

// ExecutePython executes Python code directly (without HTTP).
func ExecutePython(ctx context.Context, code string) (string, error) {
	cmd := exec.CommandContext(ctx, "python", "-c", code)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		if stderr.Len() > 0 {
			return "", fmt.Errorf("%s: %s", err, stderr.String())
		}
		return "", err
	}

	return stdout.String(), nil
}

// ExecuteBash executes Bash code directly (without HTTP).
func ExecuteBash(ctx context.Context, code string) (string, error) {
	cmd := exec.CommandContext(ctx, "bash", "-c", code)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		if stderr.Len() > 0 {
			return "", fmt.Errorf("%s: %s", err, stderr.String())
		}
		return "", err
	}

	return stdout.String(), nil
}

// CaptureMatplotlib captures matplotlib figures and returns them as base64 images.
func CaptureMatplotlib(ctx context.Context, code string) ([]string, error) {
	// Wrapper code to save figures to files
	wrappedCode := `
import matplotlib
matplotlib.use('Agg')
import matplotlib.pyplot as plt
import base64
import io
import sys

# Original code
` + code + `

# Save all figures
images = []
for i, fig in enumerate(plt.get_fignums()):
    buf = io.BytesIO()
    plt.figure(fig)
    plt.savefig(buf, format='png')
    buf.seek(0)
    images.append(base64.b64encode(buf.read()).decode('utf-8'))
    plt.close(fig)

# Output as JSON
import json
print('__IMAGES__' + json.dumps(images))
`

	output, err := ExecutePython(ctx, wrappedCode)
	if err != nil {
		return nil, err
	}

	// Parse images from output
	idx := strings.Index(output, "__IMAGES__")
	if idx == -1 {
		return nil, nil
	}

	var images []string
	jsonData := output[idx+len("__IMAGES__"):]
	jsonData = strings.TrimSpace(jsonData)
	if err := json.Unmarshal([]byte(jsonData), &images); err != nil {
		return nil, err
	}

	return images, nil
}

// EncodeImageBase64 encodes an image file to base64.
func EncodeImageBase64(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(data), nil
}
