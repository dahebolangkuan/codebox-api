package server

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"time"

	"go.uber.org/zap"

	"gobox/pkg/gobox"
)

// HandlersConfig contains configuration for handlers.
type HandlersConfig struct {
	MaxCodeSize int64
	MaxTimeout  time.Duration
}

// Handlers provides additional HTTP handlers for the server.
type Handlers struct {
	sandbox gobox.Sandbox
	config  *HandlersConfig
	logger  *zap.Logger
}

// NewHandlers creates a new Handlers instance.
func NewHandlers(sandbox gobox.Sandbox, config *HandlersConfig, logger *zap.Logger) *Handlers {
	if config == nil {
		config = &HandlersConfig{
			MaxCodeSize: 1024 * 1024, // 1MB
			MaxTimeout:  10 * time.Minute,
		}
	}

	return &Handlers{
		sandbox: sandbox,
		config:  config,
		logger:  logger,
	}
}

// ExecBatchRequest represents a batch execution request.
type ExecBatchRequest struct {
	Items []ExecBatchItem `json:"items"`
}

// ExecBatchItem represents a single item in a batch execution.
type ExecBatchItem struct {
	ID      string `json:"id"`
	Code    string `json:"code"`
	Kernel  string `json:"kernel,omitempty"`
	Timeout int    `json:"timeout,omitempty"`
}

// ExecBatchResponse represents a batch execution response.
type ExecBatchResponse struct {
	Results []ExecBatchResult `json:"results"`
}

// ExecBatchResult represents a single result in a batch response.
type ExecBatchResult struct {
	ID     string            `json:"id"`
	Result *gobox.ExecResult `json:"result,omitempty"`
	Error  string            `json:"error,omitempty"`
}

// HandleExecBatch handles batch code execution.
func (h *Handlers) HandleExecBatch(w http.ResponseWriter, r *http.Request) {
	var req ExecBatchRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, h.config.MaxCodeSize*10)).Decode(&req); err != nil {
		h.sendError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	if len(req.Items) == 0 {
		h.sendError(w, http.StatusBadRequest, "EMPTY_BATCH", "No items in batch")
		return
	}

	if len(req.Items) > 100 {
		h.sendError(w, http.StatusBadRequest, "BATCH_TOO_LARGE", "Maximum 100 items per batch")
		return
	}

	results := make([]ExecBatchResult, len(req.Items))

	for i, item := range req.Items {
		results[i].ID = item.ID

		opts := []gobox.ExecOption{}
		if item.Kernel != "" {
			opts = append(opts, gobox.WithKernel(item.Kernel))
		}
		if item.Timeout > 0 {
			opts = append(opts, gobox.WithTimeout(time.Duration(item.Timeout)*time.Second))
		}

		result, err := h.sandbox.Exec(r.Context(), item.Code, opts...)
		if err != nil {
			results[i].Error = err.Error()
		} else {
			results[i].Result = result
		}
	}

	h.sendJSON(w, http.StatusOK, ExecBatchResponse{Results: results})
}

// SessionInfo represents session information.
type SessionInfo struct {
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	Status    string    `json:"status"`
}

// HandleSessionInfo returns information about the current session.
func (h *Handlers) HandleSessionInfo(w http.ResponseWriter, r *http.Request) {
	// This is a placeholder - actual implementation depends on sandbox type
	info := SessionInfo{
		ID:        "default",
		CreatedAt: time.Now(),
		Status:    "active",
	}

	h.sendJSON(w, http.StatusOK, info)
}

// MetricsResponse contains server metrics.
type MetricsResponse struct {
	Uptime     string            `json:"uptime"`
	Requests   int64             `json:"requests"`
	Executions int64             `json:"executions"`
	Errors     int64             `json:"errors"`
	Stats      map[string]string `json:"stats,omitempty"`
}

// HandleMetrics returns server metrics (placeholder).
func (h *Handlers) HandleMetrics(w http.ResponseWriter, r *http.Request) {
	metrics := MetricsResponse{
		Uptime:     "unknown",
		Requests:   0,
		Executions: 0,
		Errors:     0,
		Stats:      make(map[string]string),
	}

	h.sendJSON(w, http.StatusOK, metrics)
}

// HandleVersion returns version information.
func (h *Handlers) HandleVersion(w http.ResponseWriter, r *http.Request) {
	h.sendJSON(w, http.StatusOK, map[string]string{
		"version": "1.0.0",
		"name":    "gobox",
		"go":      "1.22",
	})
}

// HandleExecSync handles synchronous code execution (non-streaming).
func (h *Handlers) HandleExecSync(w http.ResponseWriter, r *http.Request) {
	var req ExecRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, h.config.MaxCodeSize)).Decode(&req); err != nil {
		h.sendError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	if req.Code == "" {
		h.sendError(w, http.StatusBadRequest, "INVALID_CODE", "Code cannot be empty")
		return
	}

	opts := []gobox.ExecOption{}
	if req.Kernel != "" {
		opts = append(opts, gobox.WithKernel(req.Kernel))
	}
	if req.Timeout > 0 {
		timeout := time.Duration(req.Timeout) * time.Second
		if timeout > h.config.MaxTimeout {
			timeout = h.config.MaxTimeout
		}
		opts = append(opts, gobox.WithTimeout(timeout))
	}

	result, err := h.sandbox.Exec(r.Context(), req.Code, opts...)
	if err != nil {
		h.sendError(w, http.StatusInternalServerError, "EXEC_FAILED", err.Error())
		return
	}

	h.sendJSON(w, http.StatusOK, result)
}

// WebSocketHandler handles WebSocket connections for real-time execution.
// This is a placeholder - actual WebSocket implementation would require additional libraries.
func (h *Handlers) WebSocketHandler(w http.ResponseWriter, r *http.Request) {
	h.sendError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "WebSocket support not yet implemented")
}

// Helper functions

func (h *Handlers) sendJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func (h *Handlers) sendError(w http.ResponseWriter, status int, code, message string) {
	h.sendJSON(w, status, ErrorResponse{
		Error: ErrorDetail{
			Code:    code,
			Message: message,
		},
	})
}

// ParseTimeout parses a timeout value from request.
func ParseTimeout(r *http.Request) (time.Duration, error) {
	timeoutStr := r.URL.Query().Get("timeout")
	if timeoutStr == "" {
		return 0, nil
	}

	seconds, err := strconv.Atoi(timeoutStr)
	if err != nil {
		return 0, err
	}

	return time.Duration(seconds) * time.Second, nil
}

// ParseKernel parses a kernel value from request.
func ParseKernel(r *http.Request) string {
	kernel := r.URL.Query().Get("kernel")
	if kernel == "" {
		kernel = "python"
	}
	return kernel
}
