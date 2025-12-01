package protocol

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"

	"gobox/pkg/gobox"
)

// EncodeJSONL encodes a single chunk to JSONL format (single line of JSON).
// Returns the JSON bytes with a trailing newline.
func EncodeJSONL(chunk gobox.ExecChunk) ([]byte, error) {
	data, err := json.Marshal(chunk)
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

// DecodeJSONL decodes a single line of JSONL to a chunk.
func DecodeJSONL(data []byte) (gobox.ExecChunk, error) {
	var chunk gobox.ExecChunk
	// Trim trailing newline if present
	data = bytes.TrimSpace(data)
	err := json.Unmarshal(data, &chunk)
	return chunk, err
}

// JSONLEncoder wraps a writer for streaming JSONL output.
type JSONLEncoder struct {
	w io.Writer
}

// NewJSONLEncoder creates a new JSONL encoder.
func NewJSONLEncoder(w io.Writer) *JSONLEncoder {
	return &JSONLEncoder{w: w}
}

// Encode encodes and writes a chunk to the underlying writer.
func (e *JSONLEncoder) Encode(chunk gobox.ExecChunk) error {
	data, err := EncodeJSONL(chunk)
	if err != nil {
		return err
	}
	_, err = e.w.Write(data)
	return err
}

// JSONLDecoder wraps a reader for streaming JSONL input.
type JSONLDecoder struct {
	scanner *bufio.Scanner
}

// NewJSONLDecoder creates a new JSONL decoder.
func NewJSONLDecoder(r io.Reader) *JSONLDecoder {
	scanner := bufio.NewScanner(r)
	// Set a larger buffer for long lines
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	return &JSONLDecoder{scanner: scanner}
}

// Decode reads and decodes the next chunk from the stream.
// Returns io.EOF when no more chunks are available.
func (d *JSONLDecoder) Decode() (gobox.ExecChunk, error) {
	if !d.scanner.Scan() {
		if err := d.scanner.Err(); err != nil {
			return gobox.ExecChunk{}, err
		}
		return gobox.ExecChunk{}, io.EOF
	}

	return DecodeJSONL(d.scanner.Bytes())
}

// DecodeAll reads all chunks from the decoder.
func (d *JSONLDecoder) DecodeAll() ([]gobox.ExecChunk, error) {
	var chunks []gobox.ExecChunk
	for {
		chunk, err := d.Decode()
		if err == io.EOF {
			break
		}
		if err != nil {
			return chunks, err
		}
		chunks = append(chunks, chunk)
	}
	return chunks, nil
}

// ParseJSONLString parses a JSONL-formatted string into chunks.
func ParseJSONLString(data string) ([]gobox.ExecChunk, error) {
	decoder := NewJSONLDecoder(bytes.NewReader([]byte(data)))
	return decoder.DecodeAll()
}

// EncodeJSONLString encodes chunks to a JSONL-formatted string.
func EncodeJSONLString(chunks []gobox.ExecChunk) (string, error) {
	var buf bytes.Buffer
	encoder := NewJSONLEncoder(&buf)

	for _, chunk := range chunks {
		if err := encoder.Encode(chunk); err != nil {
			return "", err
		}
	}

	return buf.String(), nil
}
