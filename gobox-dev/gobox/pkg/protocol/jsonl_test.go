package protocol

import (
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gobox/pkg/gobox"
)

func TestEncodeJSONL(t *testing.T) {
	chunk := gobox.ExecChunk{
		Type:    gobox.ChunkTypeText,
		Content: "Hello World",
	}

	data, err := EncodeJSONL(chunk)
	require.NoError(t, err)
	assert.Contains(t, string(data), `"type":"txt"`)
	assert.Contains(t, string(data), `"content":"Hello World"`)
	assert.True(t, bytes.HasSuffix(data, []byte("\n")))
}

func TestDecodeJSONL(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		expected gobox.ExecChunk
		wantErr  bool
	}{
		{
			name:     "valid text chunk",
			input:    []byte(`{"type":"txt","content":"Hello"}`),
			expected: gobox.ExecChunk{Type: gobox.ChunkTypeText, Content: "Hello"},
			wantErr:  false,
		},
		{
			name:     "valid image chunk",
			input:    []byte(`{"type":"img","content":"base64"}`),
			expected: gobox.ExecChunk{Type: gobox.ChunkTypeImage, Content: "base64"},
			wantErr:  false,
		},
		{
			name:     "with newline",
			input:    []byte(`{"type":"txt","content":"Hello"}` + "\n"),
			expected: gobox.ExecChunk{Type: gobox.ChunkTypeText, Content: "Hello"},
			wantErr:  false,
		},
		{
			name:    "invalid json",
			input:   []byte(`{invalid}`),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chunk, err := DecodeJSONL(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, chunk)
			}
		})
	}
}

func TestJSONLEncoder(t *testing.T) {
	var buf bytes.Buffer
	encoder := NewJSONLEncoder(&buf)

	chunks := []gobox.ExecChunk{
		{Type: gobox.ChunkTypeText, Content: "Hello"},
		{Type: gobox.ChunkTypeText, Content: "World"},
	}

	for _, chunk := range chunks {
		err := encoder.Encode(chunk)
		require.NoError(t, err)
	}

	// Decode and verify
	decoder := NewJSONLDecoder(&buf)
	decoded, err := decoder.DecodeAll()
	require.NoError(t, err)
	assert.Equal(t, chunks, decoded)
}

func TestJSONLDecoder(t *testing.T) {
	input := `{"type":"txt","content":"Hello"}
{"type":"img","content":"image1"}
{"type":"err","content":"error"}
`

	decoder := NewJSONLDecoder(bytes.NewReader([]byte(input)))

	// Read first chunk
	chunk, err := decoder.Decode()
	require.NoError(t, err)
	assert.Equal(t, gobox.ChunkTypeText, chunk.Type)
	assert.Equal(t, "Hello", chunk.Content)

	// Read second chunk
	chunk, err = decoder.Decode()
	require.NoError(t, err)
	assert.Equal(t, gobox.ChunkTypeImage, chunk.Type)
	assert.Equal(t, "image1", chunk.Content)

	// Read third chunk
	chunk, err = decoder.Decode()
	require.NoError(t, err)
	assert.Equal(t, gobox.ChunkTypeError, chunk.Type)
	assert.Equal(t, "error", chunk.Content)

	// Read past end
	_, err = decoder.Decode()
	assert.Equal(t, io.EOF, err)
}

func TestJSONLDecoder_DecodeAll(t *testing.T) {
	input := `{"type":"txt","content":"Hello"}
{"type":"txt","content":"World"}
`

	decoder := NewJSONLDecoder(bytes.NewReader([]byte(input)))
	chunks, err := decoder.DecodeAll()
	require.NoError(t, err)
	assert.Len(t, chunks, 2)
	assert.Equal(t, "Hello", chunks[0].Content)
	assert.Equal(t, "World", chunks[1].Content)
}

func TestParseJSONLString(t *testing.T) {
	input := `{"type":"txt","content":"Line1"}
{"type":"txt","content":"Line2"}
`

	chunks, err := ParseJSONLString(input)
	require.NoError(t, err)
	assert.Len(t, chunks, 2)
}

func TestEncodeJSONLString(t *testing.T) {
	chunks := []gobox.ExecChunk{
		{Type: gobox.ChunkTypeText, Content: "Hello"},
		{Type: gobox.ChunkTypeText, Content: "World"},
	}

	result, err := EncodeJSONLString(chunks)
	require.NoError(t, err)
	assert.Contains(t, result, "Hello")
	assert.Contains(t, result, "World")

	// Verify round trip
	decoded, err := ParseJSONLString(result)
	require.NoError(t, err)
	assert.Equal(t, chunks, decoded)
}

func TestJSONLRoundTrip(t *testing.T) {
	original := []gobox.ExecChunk{
		{Type: gobox.ChunkTypeText, Content: "Line 1\nLine 2"},
		{Type: gobox.ChunkTypeImage, Content: "base64data=="},
		{Type: gobox.ChunkTypeError, Content: "Error: something went wrong"},
	}

	encoded, err := EncodeJSONLString(original)
	require.NoError(t, err)

	decoded, err := ParseJSONLString(encoded)
	require.NoError(t, err)

	assert.Equal(t, original, decoded)
}
