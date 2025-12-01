package gobox

import (
	"errors"
	"fmt"
)

// Standard errors that can be returned by sandbox operations.
// These errors can be checked using errors.Is().
var (
	// ErrTimeout indicates that code execution exceeded the configured timeout.
	ErrTimeout = errors.New("execution timeout")

	// ErrContainerNotReady indicates that the container is not in a ready state.
	// This can happen if the container is still starting up or has crashed.
	ErrContainerNotReady = errors.New("container not ready")

	// ErrPoolExhausted indicates that all containers in the pool are in use.
	// The caller should retry after a delay or increase the pool size.
	ErrPoolExhausted = errors.New("container pool exhausted")

	// ErrInvalidKernel indicates that the specified execution kernel is not supported.
	// Supported kernels are typically "python" and "bash".
	ErrInvalidKernel = errors.New("invalid kernel")

	// ErrFileNotFound indicates that the requested file does not exist in the sandbox.
	ErrFileNotFound = errors.New("file not found")

	// ErrUnauthorized indicates that the provided credentials are invalid.
	ErrUnauthorized = errors.New("unauthorized")

	// ErrSandboxClosed indicates that operations were attempted on a closed sandbox.
	ErrSandboxClosed = errors.New("sandbox is closed")

	// ErrInvalidCode indicates that the provided code is empty or invalid.
	ErrInvalidCode = errors.New("invalid code")

	// ErrConnectionFailed indicates that the connection to the remote server failed.
	ErrConnectionFailed = errors.New("connection failed")

	// ErrFileTooLarge indicates that the file exceeds the maximum allowed size.
	ErrFileTooLarge = errors.New("file too large")

	// ErrOperationNotSupported indicates that the operation is not supported by this driver.
	ErrOperationNotSupported = errors.New("operation not supported")
)

// ExecError represents a detailed execution error with additional context.
// This is used when code execution fails due to syntax errors, runtime exceptions, etc.
type ExecError struct {
	// Type categorizes the error: "syntax", "runtime", "timeout", "system"
	Type string `json:"type"`
	// Message is the human-readable error message.
	Message string `json:"message"`
	// Traceback contains the full stack trace if available.
	Traceback string `json:"traceback,omitempty"`
	// LineNumber indicates the line where the error occurred (if applicable).
	LineNumber int `json:"line_number,omitempty"`
	// ColumnNumber indicates the column where the error occurred (if applicable).
	ColumnNumber int `json:"column_number,omitempty"`
}

// Error implements the error interface.
func (e *ExecError) Error() string {
	if e.LineNumber > 0 {
		return fmt.Sprintf("%s error at line %d: %s", e.Type, e.LineNumber, e.Message)
	}
	return fmt.Sprintf("%s error: %s", e.Type, e.Message)
}

// Unwrap returns nil as ExecError is a leaf error.
func (e *ExecError) Unwrap() error {
	return nil
}

// NewExecError creates a new ExecError with the given type and message.
func NewExecError(errType, message string) *ExecError {
	return &ExecError{
		Type:    errType,
		Message: message,
	}
}

// NewSyntaxError creates an ExecError for syntax errors.
func NewSyntaxError(message string, line int) *ExecError {
	return &ExecError{
		Type:       "syntax",
		Message:    message,
		LineNumber: line,
	}
}

// NewRuntimeError creates an ExecError for runtime errors.
func NewRuntimeError(message, traceback string) *ExecError {
	return &ExecError{
		Type:      "runtime",
		Message:   message,
		Traceback: traceback,
	}
}

// NewTimeoutError creates an ExecError for timeout errors.
func NewTimeoutError(timeout string) *ExecError {
	return &ExecError{
		Type:    "timeout",
		Message: fmt.Sprintf("execution exceeded timeout of %s", timeout),
	}
}

// NewSystemError creates an ExecError for system-level errors.
func NewSystemError(message string) *ExecError {
	return &ExecError{
		Type:    "system",
		Message: message,
	}
}

// DriverError wraps errors from specific drivers with context.
type DriverError struct {
	// Driver is the name of the driver that produced the error.
	Driver string
	// Op is the operation that failed.
	Op string
	// Err is the underlying error.
	Err error
}

// Error implements the error interface.
func (e *DriverError) Error() string {
	return fmt.Sprintf("%s driver: %s: %v", e.Driver, e.Op, e.Err)
}

// Unwrap returns the underlying error.
func (e *DriverError) Unwrap() error {
	return e.Err
}

// NewDriverError creates a new DriverError.
func NewDriverError(driver, op string, err error) *DriverError {
	return &DriverError{
		Driver: driver,
		Op:     op,
		Err:    err,
	}
}

// IsTimeout checks if the error is a timeout error.
func IsTimeout(err error) bool {
	return errors.Is(err, ErrTimeout)
}

// IsPoolExhausted checks if the error indicates pool exhaustion.
func IsPoolExhausted(err error) bool {
	return errors.Is(err, ErrPoolExhausted)
}

// IsUnauthorized checks if the error is an authorization error.
func IsUnauthorized(err error) bool {
	return errors.Is(err, ErrUnauthorized)
}

// IsContainerNotReady checks if the error indicates container not ready.
func IsContainerNotReady(err error) bool {
	return errors.Is(err, ErrContainerNotReady)
}
