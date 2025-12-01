package gobox

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestExecResult_Text(t *testing.T) {
	tests := []struct {
		name     string
		chunks   []ExecChunk
		expected string
	}{
		{
			name:     "empty chunks",
			chunks:   []ExecChunk{},
			expected: "",
		},
		{
			name: "single text chunk",
			chunks: []ExecChunk{
				{Type: ChunkTypeText, Content: "Hello"},
			},
			expected: "Hello",
		},
		{
			name: "multiple text chunks",
			chunks: []ExecChunk{
				{Type: ChunkTypeText, Content: "Hello "},
				{Type: ChunkTypeText, Content: "World"},
			},
			expected: "Hello World",
		},
		{
			name: "mixed chunks",
			chunks: []ExecChunk{
				{Type: ChunkTypeText, Content: "Hello "},
				{Type: ChunkTypeImage, Content: "base64..."},
				{Type: ChunkTypeText, Content: "World"},
				{Type: ChunkTypeError, Content: "error"},
			},
			expected: "Hello World",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := &ExecResult{Chunks: tt.chunks}
			assert.Equal(t, tt.expected, result.Text())
		})
	}
}

func TestExecResult_Images(t *testing.T) {
	result := &ExecResult{
		Chunks: []ExecChunk{
			{Type: ChunkTypeText, Content: "text"},
			{Type: ChunkTypeImage, Content: "image1"},
			{Type: ChunkTypeImage, Content: "image2"},
			{Type: ChunkTypeError, Content: "error"},
		},
	}

	images := result.Images()
	assert.Equal(t, 2, len(images))
	assert.Equal(t, "image1", images[0])
	assert.Equal(t, "image2", images[1])
}

func TestExecResult_Errors(t *testing.T) {
	result := &ExecResult{
		Chunks: []ExecChunk{
			{Type: ChunkTypeText, Content: "text"},
			{Type: ChunkTypeError, Content: "error1"},
			{Type: ChunkTypeError, Content: "error2"},
		},
	}

	errors := result.Errors()
	assert.Equal(t, 2, len(errors))
	assert.Equal(t, "error1", errors[0])
	assert.Equal(t, "error2", errors[1])
}

func TestExecResult_HasError(t *testing.T) {
	tests := []struct {
		name     string
		chunks   []ExecChunk
		expected bool
	}{
		{
			name:     "no errors",
			chunks:   []ExecChunk{{Type: ChunkTypeText, Content: "text"}},
			expected: false,
		},
		{
			name:     "has error",
			chunks:   []ExecChunk{{Type: ChunkTypeError, Content: "error"}},
			expected: true,
		},
		{
			name:     "empty",
			chunks:   []ExecChunk{},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := &ExecResult{Chunks: tt.chunks}
			assert.Equal(t, tt.expected, result.HasError())
		})
	}
}

func TestChunkType_String(t *testing.T) {
	assert.Equal(t, ChunkType("txt"), ChunkTypeText)
	assert.Equal(t, ChunkType("img"), ChunkTypeImage)
	assert.Equal(t, ChunkType("err"), ChunkTypeError)
}

func TestFileInfo(t *testing.T) {
	info := FileInfo{
		Path:  "/app/test.py",
		Size:  1024,
		IsDir: false,
	}

	assert.Equal(t, "/app/test.py", info.Path)
	assert.Equal(t, int64(1024), info.Size)
	assert.False(t, info.IsDir)
}

func TestHealthStatus(t *testing.T) {
	status := HealthStatus{
		Healthy:   true,
		Message:   "OK",
		LastCheck: time.Now(),
	}

	assert.True(t, status.Healthy)
	assert.Equal(t, "OK", status.Message)
}

func TestSessionInfo(t *testing.T) {
	now := time.Now()
	info := SessionInfo{
		ID:        "session-123",
		FactoryID: "factory-456",
		CreatedAt: now,
		Status:    "active",
	}

	assert.Equal(t, "session-123", info.ID)
	assert.Equal(t, "factory-456", info.FactoryID)
	assert.Equal(t, now, info.CreatedAt)
	assert.Equal(t, "active", info.Status)
}
