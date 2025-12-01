package protocol

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"gobox/pkg/gobox"
)

func TestParseLegacy(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []gobox.ExecChunk
	}{
		{
			name:     "empty input",
			input:    "",
			expected: []gobox.ExecChunk{},
		},
		{
			name:  "single text",
			input: "<txt>Hello World</txt>",
			expected: []gobox.ExecChunk{
				{Type: gobox.ChunkTypeText, Content: "Hello World"},
			},
		},
		{
			name:  "single image",
			input: "<img>base64data</img>",
			expected: []gobox.ExecChunk{
				{Type: gobox.ChunkTypeImage, Content: "base64data"},
			},
		},
		{
			name:  "single error",
			input: "<err>Error message</err>",
			expected: []gobox.ExecChunk{
				{Type: gobox.ChunkTypeError, Content: "Error message"},
			},
		},
		{
			name:  "multiple chunks",
			input: "<txt>Hello</txt><img>image1</img><txt>World</txt><err>error</err>",
			expected: []gobox.ExecChunk{
				{Type: gobox.ChunkTypeText, Content: "Hello"},
				{Type: gobox.ChunkTypeImage, Content: "image1"},
				{Type: gobox.ChunkTypeText, Content: "World"},
				{Type: gobox.ChunkTypeError, Content: "error"},
			},
		},
		{
			name:  "multiline content",
			input: "<txt>Line1\nLine2\nLine3</txt>",
			expected: []gobox.ExecChunk{
				{Type: gobox.ChunkTypeText, Content: "Line1\nLine2\nLine3"},
			},
		},
		{
			name:     "invalid tag",
			input:    "<invalid>content</invalid>",
			expected: []gobox.ExecChunk{},
		},
		{
			name:     "unclosed tag",
			input:    "<txt>content",
			expected: []gobox.ExecChunk{},
		},
		{
			name:     "mismatched tags",
			input:    "<txt>content</img>",
			expected: []gobox.ExecChunk{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseLegacy(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestEncodeLegacy(t *testing.T) {
	tests := []struct {
		name     string
		chunks   []gobox.ExecChunk
		expected string
	}{
		{
			name:     "empty chunks",
			chunks:   []gobox.ExecChunk{},
			expected: "",
		},
		{
			name: "single chunk",
			chunks: []gobox.ExecChunk{
				{Type: gobox.ChunkTypeText, Content: "Hello"},
			},
			expected: "<txt>Hello</txt>",
		},
		{
			name: "multiple chunks",
			chunks: []gobox.ExecChunk{
				{Type: gobox.ChunkTypeText, Content: "Hello"},
				{Type: gobox.ChunkTypeImage, Content: "img"},
				{Type: gobox.ChunkTypeError, Content: "err"},
			},
			expected: "<txt>Hello</txt><img>img</img><err>err</err>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := EncodeLegacy(tt.chunks)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestEncodeLegacyChunk(t *testing.T) {
	chunk := gobox.ExecChunk{Type: gobox.ChunkTypeText, Content: "Hello"}
	result := EncodeLegacyChunk(chunk)
	assert.Equal(t, "<txt>Hello</txt>", result)
}

func TestRoundTrip(t *testing.T) {
	original := []gobox.ExecChunk{
		{Type: gobox.ChunkTypeText, Content: "Hello"},
		{Type: gobox.ChunkTypeImage, Content: "base64data"},
		{Type: gobox.ChunkTypeError, Content: "error message"},
	}

	encoded := EncodeLegacy(original)
	decoded := ParseLegacy(encoded)

	assert.Equal(t, original, decoded)
}
