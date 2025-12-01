// Package server provides the HTTP API server for GoBox.
// It exposes sandbox functionality over a RESTful API.
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"

	"gobox/pkg/gobox"
)

// Server is the GoBox HTTP API server.
type Server struct {
	sandbox    gobox.Sandbox
	config     *Config
	mux        *http.ServeMux
	httpServer *http.Server
	logger     *zap.Logger
}

// Config holds configuration for the HTTP server.
type Config struct {
	// Port is the port to listen on.
	Port int
	// Host is the host to bind to.
	Host string
	// ReadTimeout is the maximum duration for reading the request.
	ReadTimeout time.Duration
	// WriteTimeout is the maximum duration for writing the response.
	WriteTimeout time.Duration
	// IdleTimeout is the maximum duration for idle connections.
	IdleTimeout time.Duration
	// MaxBodySize is the maximum size of request bodies.
	MaxBodySize int64
	// EnableAuth enables authentication middleware.
	EnableAuth bool
	// AuthToken is the required authentication token.
	AuthToken string
	// EnableCORS enables CORS headers.
	EnableCORS bool
	// AllowedOrigins is the list of allowed CORS origins.
	AllowedOrigins []string
	// EnableRateLimit enables rate limiting.
	EnableRateLimit bool
	// RateLimitRPS is the requests per second limit.
	RateLimitRPS int
	// RateLimitBurst is the maximum burst size.
	RateLimitBurst int
}

// DefaultConfig returns a Config with default values.
func DefaultConfig() *Config {
	return &Config{
		Port:           8069,
		Host:           "0.0.0.0",
		ReadTimeout:    30 * time.Second,
		WriteTimeout:   60 * time.Second,
		IdleTimeout:    120 * time.Second,
		MaxBodySize:    10 * 1024 * 1024, // 10MB
		EnableAuth:     false,
		EnableCORS:     true,
		AllowedOrigins: []string{"*"},
		EnableRateLimit: false,
		RateLimitRPS:   100,
		RateLimitBurst: 200,
	}
}

// New creates a new HTTP server.
func New(sandbox gobox.Sandbox, config *Config, logger *zap.Logger) *Server {
	if config == nil {
		config = DefaultConfig()
	}

	if logger == nil {
		logger, _ = zap.NewProduction()
	}

	s := &Server{
		sandbox: sandbox,
		config:  config,
		mux:     http.NewServeMux(),
		logger:  logger,
	}

	s.setupRoutes()

	return s
}

// setupRoutes configures the HTTP routes.
func (s *Server) setupRoutes() {
	// Health check
	s.mux.HandleFunc("GET /", s.handleHealth)
	s.mux.HandleFunc("GET /health", s.handleHealth)

	// Code execution
	s.mux.HandleFunc("POST /exec", s.handleExec)

	// File operations
	s.mux.HandleFunc("GET /files", s.handleListFiles)
	s.mux.HandleFunc("POST /files/upload", s.handleUpload)
	s.mux.HandleFunc("GET /files/download/{filename...}", s.handleDownload)

	// Package installation
	s.mux.HandleFunc("POST /install", s.handleInstall)

	// Session management
	s.mux.HandleFunc("POST /restart", s.handleRestart)
}

// Handler returns the HTTP handler with middleware applied.
func (s *Server) Handler() http.Handler {
	handler := http.Handler(s.mux)

	// Apply middleware in reverse order (last applied is first executed)
	handler = s.recoveryMiddleware(handler)
	handler = s.loggingMiddleware(handler)

	if s.config.EnableAuth {
		handler = s.authMiddleware(handler)
	}

	if s.config.EnableCORS {
		handler = s.corsMiddleware(handler)
	}

	return handler
}

// ListenAndServe starts the HTTP server.
func (s *Server) ListenAndServe() error {
	addr := fmt.Sprintf("%s:%d", s.config.Host, s.config.Port)

	s.httpServer = &http.Server{
		Addr:         addr,
		Handler:      s.Handler(),
		ReadTimeout:  s.config.ReadTimeout,
		WriteTimeout: s.config.WriteTimeout,
		IdleTimeout:  s.config.IdleTimeout,
	}

	s.logger.Info("Starting server", zap.String("addr", addr))

	return s.httpServer.ListenAndServe()
}

// Shutdown gracefully shuts down the server.
func (s *Server) Shutdown(ctx context.Context) error {
	if s.httpServer == nil {
		return nil
	}

	s.logger.Info("Shutting down server")
	return s.httpServer.Shutdown(ctx)
}

// Run starts the server and handles graceful shutdown on signals.
func (s *Server) Run() error {
	// Create shutdown channel
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)

	// Start server in goroutine
	errChan := make(chan error, 1)
	go func() {
		errChan <- s.ListenAndServe()
	}()

	// Wait for shutdown signal or error
	select {
	case err := <-errChan:
		if err != http.ErrServerClosed {
			return err
		}
	case <-shutdown:
		s.logger.Info("Received shutdown signal")

		// Graceful shutdown with timeout
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if err := s.Shutdown(ctx); err != nil {
			return err
		}
	}

	return nil
}

// Request/Response types

// ExecRequest represents a code execution request.
type ExecRequest struct {
	Code    string `json:"code"`
	Kernel  string `json:"kernel,omitempty"`
	Timeout int    `json:"timeout,omitempty"`
	Cwd     string `json:"cwd,omitempty"`
}

// InstallRequest represents a package installation request.
type InstallRequest struct {
	Packages []string `json:"packages"`
}

// ErrorResponse represents an error response.
type ErrorResponse struct {
	Error ErrorDetail `json:"error"`
}

// ErrorDetail contains error details.
type ErrorDetail struct {
	Code    string                 `json:"code"`
	Message string                 `json:"message"`
	Details map[string]interface{} `json:"details,omitempty"`
}

// Helper functions

func (s *Server) sendJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func (s *Server) sendError(w http.ResponseWriter, status int, code, message string) {
	s.sendJSON(w, status, ErrorResponse{
		Error: ErrorDetail{
			Code:    code,
			Message: message,
		},
	})
}

func (s *Server) sendStreamingResponse(w http.ResponseWriter, chunkChan <-chan gobox.ExecChunk, errChan <-chan error) {
	// Set streaming headers
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		s.sendError(w, http.StatusInternalServerError, "STREAMING_NOT_SUPPORTED", "Streaming not supported")
		return
	}

	// Send initial flush to establish connection
	flusher.Flush()

	for {
		select {
		case chunk, ok := <-chunkChan:
			if !ok {
				// Check for errors
				select {
				case err := <-errChan:
					if err != nil {
						data, _ := json.Marshal(gobox.ExecChunk{
							Type:    gobox.ChunkTypeError,
							Content: err.Error(),
						})
						w.Write(data)
						w.Write([]byte("\n"))
						flusher.Flush()
					}
				default:
				}
				return
			}

			data, err := json.Marshal(chunk)
			if err != nil {
				s.logger.Error("Failed to marshal chunk", zap.Error(err))
				continue
			}

			w.Write(data)
			w.Write([]byte("\n"))
			flusher.Flush()

		case err := <-errChan:
			if err != nil {
				data, _ := json.Marshal(gobox.ExecChunk{
					Type:    gobox.ChunkTypeError,
					Content: err.Error(),
				})
				w.Write(data)
				w.Write([]byte("\n"))
				flusher.Flush()
			}
			return
		}
	}
}

// Handler implementations

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	status := "ok"
	err := s.sandbox.HealthCheck(ctx)
	if err != nil {
		status = "error"
	}

	s.sendJSON(w, http.StatusOK, map[string]interface{}{
		"status":    status,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

func (s *Server) handleExec(w http.ResponseWriter, r *http.Request) {
	// Parse request
	var req ExecRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, s.config.MaxBodySize)).Decode(&req); err != nil {
		s.sendError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	if req.Code == "" {
		s.sendError(w, http.StatusBadRequest, "INVALID_CODE", "Code cannot be empty")
		return
	}

	// Build options
	opts := []gobox.ExecOption{}
	if req.Kernel != "" {
		opts = append(opts, gobox.WithKernel(req.Kernel))
	}
	if req.Timeout > 0 {
		opts = append(opts, gobox.WithTimeout(time.Duration(req.Timeout)*time.Second))
	}
	if req.Cwd != "" {
		opts = append(opts, gobox.WithCwd(req.Cwd))
	}

	// Execute code with streaming
	chunkChan, errChan := s.sandbox.StreamExec(r.Context(), req.Code, opts...)

	// Send streaming response
	s.sendStreamingResponse(w, chunkChan, errChan)
}

func (s *Server) handleListFiles(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	files, err := s.sandbox.ListFiles(ctx)
	if err != nil {
		s.sendError(w, http.StatusInternalServerError, "LIST_FILES_FAILED", err.Error())
		return
	}

	s.sendJSON(w, http.StatusOK, files)
}

func (s *Server) handleUpload(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	// Parse multipart form
	if err := r.ParseMultipartForm(s.config.MaxBodySize); err != nil {
		s.sendError(w, http.StatusBadRequest, "INVALID_FORM", err.Error())
		return
	}

	files := make([]gobox.File, 0)
	for _, fhs := range r.MultipartForm.File {
		for _, fh := range fhs {
			file, err := fh.Open()
			if err != nil {
				s.sendError(w, http.StatusBadRequest, "FILE_OPEN_FAILED", err.Error())
				return
			}
			defer file.Close()

			files = append(files, gobox.File{
				Name:    fh.Filename,
				Content: file,
			})
		}
	}

	if len(files) == 0 {
		s.sendError(w, http.StatusBadRequest, "NO_FILES", "No files provided")
		return
	}

	if err := s.sandbox.Upload(ctx, files...); err != nil {
		s.sendError(w, http.StatusInternalServerError, "UPLOAD_FAILED", err.Error())
		return
	}

	s.sendJSON(w, http.StatusOK, map[string]interface{}{
		"uploaded": len(files),
	})
}

func (s *Server) handleDownload(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	filename := r.PathValue("filename")
	if filename == "" {
		s.sendError(w, http.StatusBadRequest, "INVALID_FILENAME", "Filename is required")
		return
	}

	reader, err := s.sandbox.Download(ctx, filename)
	if err != nil {
		if err == gobox.ErrFileNotFound {
			s.sendError(w, http.StatusNotFound, "FILE_NOT_FOUND", "File not found")
			return
		}
		s.sendError(w, http.StatusInternalServerError, "DOWNLOAD_FAILED", err.Error())
		return
	}
	defer reader.Close()

	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	w.Header().Set("Content-Type", "application/octet-stream")

	io.Copy(w, reader)
}

func (s *Server) handleInstall(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()

	var req InstallRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, s.config.MaxBodySize)).Decode(&req); err != nil {
		s.sendError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	if len(req.Packages) == 0 {
		s.sendError(w, http.StatusBadRequest, "NO_PACKAGES", "No packages specified")
		return
	}

	if err := s.sandbox.Install(ctx, req.Packages...); err != nil {
		s.sendError(w, http.StatusInternalServerError, "INSTALL_FAILED", err.Error())
		return
	}

	s.sendJSON(w, http.StatusOK, map[string]interface{}{
		"installed": req.Packages,
	})
}

func (s *Server) handleRestart(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	if err := s.sandbox.Restart(ctx); err != nil {
		s.sendError(w, http.StatusInternalServerError, "RESTART_FAILED", err.Error())
		return
	}

	s.sendJSON(w, http.StatusOK, map[string]interface{}{
		"status": "restarted",
	})
}
