// Package gobox provides the core types and interfaces for the GoBox sandbox execution system.
// GoBox is a high-performance sandbox code execution orchestrator for LLM applications.
package gobox

import (
	"io"
	"time"
)

// ChunkType defines the type of output chunk in execution results.
// The chunk type indicates whether the content is text output, an image, or an error message.
type ChunkType string

const (
	// ChunkTypeText represents plain text output from code execution.
	ChunkTypeText ChunkType = "txt"
	// ChunkTypeImage represents base64-encoded image output (e.g., matplotlib plots).
	ChunkTypeImage ChunkType = "img"
	// ChunkTypeError represents error output or exception messages.
	ChunkTypeError ChunkType = "err"
)

// ExecChunk represents a single chunk of streaming output from code execution.
// During streaming execution, output is delivered as a sequence of chunks.
type ExecChunk struct {
	// Type indicates the kind of content in this chunk.
	Type ChunkType `json:"type"`
	// Content holds the actual output content.
	// For text/error types, this is the raw string.
	// For image types, this is base64-encoded image data.
	Content string `json:"content"`
}

// ExecResult represents the complete result of a code execution.
// It aggregates all output chunks and provides convenience methods for accessing specific output types.
type ExecResult struct {
	// Chunks contains all output chunks from the execution in order.
	Chunks []ExecChunk `json:"chunks"`
	// Duration is the total time taken for execution.
	Duration time.Duration `json:"duration"`
	// ExitCode is the exit code from the execution (0 typically indicates success).
	ExitCode int `json:"exit_code,omitempty"`
}

// Text returns all text output concatenated together.
// This is useful when you only care about the standard output.
func (r *ExecResult) Text() string {
	var text string
	for _, chunk := range r.Chunks {
		if chunk.Type == ChunkTypeText {
			text += chunk.Content
		}
	}
	return text
}

// Images returns all image outputs as base64-encoded strings.
// This is useful for extracting generated plots or visualizations.
func (r *ExecResult) Images() []string {
	var images []string
	for _, chunk := range r.Chunks {
		if chunk.Type == ChunkTypeImage {
			images = append(images, chunk.Content)
		}
	}
	return images
}

// Errors returns all error messages from the execution.
// This includes exceptions, syntax errors, and runtime errors.
func (r *ExecResult) Errors() []string {
	var errors []string
	for _, chunk := range r.Chunks {
		if chunk.Type == ChunkTypeError {
			errors = append(errors, chunk.Content)
		}
	}
	return errors
}

// HasError returns true if the execution produced any error output.
func (r *ExecResult) HasError() bool {
	for _, chunk := range r.Chunks {
		if chunk.Type == ChunkTypeError {
			return true
		}
	}
	return false
}

// File represents a file to be uploaded to the sandbox environment.
// The file content is provided as an io.Reader for efficient streaming.
type File struct {
	// Name is the filename as it should appear in the sandbox.
	Name string
	// Content provides the file content for reading.
	Content io.Reader
}

// FileInfo provides metadata about a file in the sandbox environment.
type FileInfo struct {
	// Path is the full path to the file within the sandbox.
	Path string `json:"path"`
	// Size is the file size in bytes.
	Size int64 `json:"size"`
	// ModTime is the last modification time.
	ModTime time.Time `json:"mod_time,omitempty"`
	// IsDir indicates if this is a directory.
	IsDir bool `json:"is_dir,omitempty"`
}

// HealthStatus represents the health state of a sandbox.
type HealthStatus struct {
	// Healthy indicates if the sandbox is ready for use.
	Healthy bool `json:"healthy"`
	// Message provides additional details about the health state.
	Message string `json:"message,omitempty"`
	// LastCheck is the timestamp of the last health check.
	LastCheck time.Time `json:"last_check"`
}

// SessionInfo contains information about the current sandbox session.
type SessionInfo struct {
	// ID is the unique identifier for this session.
	ID string `json:"id"`
	// FactoryID identifies the factory that created this session.
	FactoryID string `json:"factory_id,omitempty"`
	// CreatedAt is when the session was created.
	CreatedAt time.Time `json:"created_at"`
	// Status is the current session status.
	Status string `json:"status"`
}
