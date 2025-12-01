package protocol

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"gobox/pkg/gobox"
)

// SSEEvent represents a Server-Sent Event.
type SSEEvent struct {
	// Event is the event type (optional).
	Event string
	// Data is the event data payload.
	Data string
	// ID is the event ID (optional).
	ID string
	// Retry is the reconnection time in milliseconds (optional).
	Retry int
}

// EncodeSSE encodes a chunk as a Server-Sent Event.
func EncodeSSE(chunk gobox.ExecChunk) ([]byte, error) {
	var buf bytes.Buffer

	// Write event type based on chunk type
	buf.WriteString("event: ")
	buf.WriteString(string(chunk.Type))
	buf.WriteString("\n")

	// Encode the chunk as JSON for the data field
	data, err := json.Marshal(chunk)
	if err != nil {
		return nil, err
	}

	buf.WriteString("data: ")
	buf.Write(data)
	buf.WriteString("\n\n")

	return buf.Bytes(), nil
}

// EncodeSSEData encodes raw data as an SSE event.
func EncodeSSEData(event, data string) []byte {
	var buf bytes.Buffer

	if event != "" {
		buf.WriteString("event: ")
		buf.WriteString(event)
		buf.WriteString("\n")
	}

	// Handle multi-line data
	lines := strings.Split(data, "\n")
	for _, line := range lines {
		buf.WriteString("data: ")
		buf.WriteString(line)
		buf.WriteString("\n")
	}

	buf.WriteString("\n")
	return buf.Bytes()
}

// SSEDecoder reads and decodes Server-Sent Events from a stream.
type SSEDecoder struct {
	scanner *bufio.Scanner
}

// NewSSEDecoder creates a new SSE decoder.
func NewSSEDecoder(r io.Reader) *SSEDecoder {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	return &SSEDecoder{scanner: scanner}
}

// Decode reads the next SSE event from the stream.
// Returns io.EOF when no more events are available.
func (d *SSEDecoder) Decode() (*SSEEvent, error) {
	event := &SSEEvent{}
	var dataLines []string
	hasContent := false

	for d.scanner.Scan() {
		line := d.scanner.Text()

		// Empty line signals end of event
		if line == "" {
			if hasContent {
				event.Data = strings.Join(dataLines, "\n")
				return event, nil
			}
			continue
		}

		hasContent = true

		// Parse field
		if strings.HasPrefix(line, "event:") {
			event.Event = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		} else if strings.HasPrefix(line, "data:") {
			dataLines = append(dataLines, strings.TrimPrefix(line, "data:"))
		} else if strings.HasPrefix(line, "id:") {
			event.ID = strings.TrimSpace(strings.TrimPrefix(line, "id:"))
		} else if strings.HasPrefix(line, "retry:") {
			retry := strings.TrimSpace(strings.TrimPrefix(line, "retry:"))
			fmt.Sscanf(retry, "%d", &event.Retry)
		}
		// Ignore comments (lines starting with :) and unknown fields
	}

	if err := d.scanner.Err(); err != nil {
		return nil, err
	}

	// Return any remaining event data
	if hasContent {
		event.Data = strings.Join(dataLines, "\n")
		return event, nil
	}

	return nil, io.EOF
}

// DecodeChunk decodes an SSE event into an ExecChunk.
func (d *SSEDecoder) DecodeChunk() (gobox.ExecChunk, error) {
	event, err := d.Decode()
	if err != nil {
		return gobox.ExecChunk{}, err
	}

	// Try to parse the data as JSON chunk first
	var chunk gobox.ExecChunk
	if err := json.Unmarshal([]byte(strings.TrimSpace(event.Data)), &chunk); err == nil {
		return chunk, nil
	}

	// Fallback: use event type and raw data
	chunkType := gobox.ChunkType(event.Event)
	if chunkType == "" {
		chunkType = gobox.ChunkTypeText
	}

	return gobox.ExecChunk{
		Type:    chunkType,
		Content: strings.TrimSpace(event.Data),
	}, nil
}

// SSEEncoder writes Server-Sent Events to a stream.
type SSEEncoder struct {
	w io.Writer
}

// NewSSEEncoder creates a new SSE encoder.
func NewSSEEncoder(w io.Writer) *SSEEncoder {
	return &SSEEncoder{w: w}
}

// Encode encodes and writes a chunk as an SSE event.
func (e *SSEEncoder) Encode(chunk gobox.ExecChunk) error {
	data, err := EncodeSSE(chunk)
	if err != nil {
		return err
	}
	_, err = e.w.Write(data)
	return err
}

// EncodeEvent writes a raw SSE event.
func (e *SSEEncoder) EncodeEvent(event, data string) error {
	_, err := e.w.Write(EncodeSSEData(event, data))
	return err
}
