package gobox

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExecError_Error(t *testing.T) {
	tests := []struct {
		name     string
		err      *ExecError
		expected string
	}{
		{
			name:     "without line number",
			err:      &ExecError{Type: "runtime", Message: "division by zero"},
			expected: "runtime error: division by zero",
		},
		{
			name:     "with line number",
			err:      &ExecError{Type: "syntax", Message: "invalid syntax", LineNumber: 5},
			expected: "syntax error at line 5: invalid syntax",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.err.Error())
		})
	}
}

func TestExecError_Unwrap(t *testing.T) {
	err := &ExecError{Type: "test", Message: "test"}
	assert.Nil(t, err.Unwrap())
}

func TestNewExecError(t *testing.T) {
	err := NewExecError("runtime", "test error")
	assert.Equal(t, "runtime", err.Type)
	assert.Equal(t, "test error", err.Message)
}

func TestNewSyntaxError(t *testing.T) {
	err := NewSyntaxError("invalid syntax", 10)
	assert.Equal(t, "syntax", err.Type)
	assert.Equal(t, "invalid syntax", err.Message)
	assert.Equal(t, 10, err.LineNumber)
}

func TestNewRuntimeError(t *testing.T) {
	err := NewRuntimeError("division by zero", "Traceback...")
	assert.Equal(t, "runtime", err.Type)
	assert.Equal(t, "division by zero", err.Message)
	assert.Equal(t, "Traceback...", err.Traceback)
}

func TestNewTimeoutError(t *testing.T) {
	err := NewTimeoutError("30s")
	assert.Equal(t, "timeout", err.Type)
	assert.Contains(t, err.Message, "30s")
}

func TestNewSystemError(t *testing.T) {
	err := NewSystemError("container crashed")
	assert.Equal(t, "system", err.Type)
	assert.Equal(t, "container crashed", err.Message)
}

func TestDriverError(t *testing.T) {
	cause := errors.New("connection refused")
	err := NewDriverError("docker", "connect", cause)

	assert.Equal(t, "docker", err.Driver)
	assert.Equal(t, "connect", err.Op)
	assert.Equal(t, cause, err.Err)

	assert.Contains(t, err.Error(), "docker")
	assert.Contains(t, err.Error(), "connect")
	assert.Contains(t, err.Error(), "connection refused")

	assert.Equal(t, cause, err.Unwrap())
}

func TestErrorCheckers(t *testing.T) {
	assert.True(t, IsTimeout(ErrTimeout))
	assert.True(t, IsPoolExhausted(ErrPoolExhausted))
	assert.True(t, IsUnauthorized(ErrUnauthorized))
	assert.True(t, IsContainerNotReady(ErrContainerNotReady))

	assert.False(t, IsTimeout(ErrPoolExhausted))
	assert.False(t, IsPoolExhausted(ErrTimeout))
	assert.False(t, IsUnauthorized(ErrTimeout))
	assert.False(t, IsContainerNotReady(ErrTimeout))
}

func TestStandardErrors(t *testing.T) {
	// Verify all standard errors are defined
	assert.NotNil(t, ErrTimeout)
	assert.NotNil(t, ErrContainerNotReady)
	assert.NotNil(t, ErrPoolExhausted)
	assert.NotNil(t, ErrInvalidKernel)
	assert.NotNil(t, ErrFileNotFound)
	assert.NotNil(t, ErrUnauthorized)
	assert.NotNil(t, ErrSandboxClosed)
	assert.NotNil(t, ErrInvalidCode)
	assert.NotNil(t, ErrConnectionFailed)
	assert.NotNil(t, ErrFileTooLarge)
	assert.NotNil(t, ErrOperationNotSupported)
}

func TestWrappedErrors(t *testing.T) {
	cause := ErrTimeout
	err := NewDriverError("docker", "exec", cause)

	// Should be able to unwrap to the original error
	assert.True(t, errors.Is(err, cause))
}
