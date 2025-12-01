package remote

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"

	"gobox/pkg/gobox"
	"gobox/pkg/protocol"
)

// StreamParser parses streaming responses from the remote API.
type StreamParser struct {
	reader   *bufio.Reader
	format   StreamFormat
	fallback bool
}

// StreamFormat defines the response format.
type StreamFormat int

const (
	// FormatAuto automatically detects the format.
	FormatAuto StreamFormat = iota
	// FormatJSONL uses JSON Lines format.
	FormatJSONL
	// FormatSSE uses Server-Sent Events format.
	FormatSSE
	// FormatLegacy uses the legacy tag-based format.
	FormatLegacy
)

// NewStreamParser creates a new stream parser.
func NewStreamParser(r io.Reader, format StreamFormat) *StreamParser {
	return &StreamParser{
		reader:   bufio.NewReader(r),
		format:   format,
		fallback: format == FormatAuto,
	}
}

// Parse reads all chunks from the stream.
func (p *StreamParser) Parse(ctx context.Context) ([]gobox.ExecChunk, error) {
	chunks := make([]gobox.ExecChunk, 0)

	for {
		chunk, err := p.Next(ctx)
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

// ParseToChan reads chunks and sends them to a channel.
func (p *StreamParser) ParseToChan(ctx context.Context, chunkChan chan<- gobox.ExecChunk) error {
	for {
		chunk, err := p.Next(ctx)
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}

		select {
		case chunkChan <- chunk:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// Next reads the next chunk from the stream.
func (p *StreamParser) Next(ctx context.Context) (gobox.ExecChunk, error) {
	select {
	case <-ctx.Done():
		return gobox.ExecChunk{}, ctx.Err()
	default:
	}

	switch p.format {
	case FormatJSONL:
		return p.parseJSONL()
	case FormatSSE:
		return p.parseSSE()
	case FormatLegacy:
		return p.parseLegacy()
	default:
		return p.parseAuto()
	}
}

func (p *StreamParser) parseJSONL() (gobox.ExecChunk, error) {
	line, err := p.reader.ReadBytes('\n')
	if err != nil {
		return gobox.ExecChunk{}, err
	}

	return protocol.DecodeJSONL(line)
}

func (p *StreamParser) parseSSE() (gobox.ExecChunk, error) {
	decoder := protocol.NewSSEDecoder(p.reader)
	return decoder.DecodeChunk()
}

func (p *StreamParser) parseLegacy() (gobox.ExecChunk, error) {
	// Read until we find a complete tag
	var buf bytes.Buffer

	for {
		b, err := p.reader.ReadByte()
		if err != nil {
			if buf.Len() > 0 {
				// Try to parse whatever we have
				chunks := protocol.ParseLegacy(buf.String())
				if len(chunks) > 0 {
					return chunks[0], nil
				}
			}
			return gobox.ExecChunk{}, err
		}

		buf.WriteByte(b)

		// Check if we have a complete chunk
		chunks := protocol.ParseLegacy(buf.String())
		if len(chunks) > 0 {
			return chunks[0], nil
		}
	}
}

func (p *StreamParser) parseAuto() (gobox.ExecChunk, error) {
	line, err := p.reader.ReadBytes('\n')
	if err != nil {
		return gobox.ExecChunk{}, err
	}

	line = bytes.TrimSpace(line)
	if len(line) == 0 {
		return p.parseAuto() // Skip empty lines
	}

	// Try JSONL first
	chunk, err := protocol.DecodeJSONL(line)
	if err == nil {
		p.format = FormatJSONL // Lock to this format
		return chunk, nil
	}

	// Try legacy format
	chunks := protocol.ParseLegacy(string(line))
	if len(chunks) > 0 {
		p.format = FormatLegacy
		return chunks[0], nil
	}

	// Check for SSE format
	if strings.HasPrefix(string(line), "event:") || strings.HasPrefix(string(line), "data:") {
		p.format = FormatSSE
		// Re-parse with SSE decoder
		decoder := protocol.NewSSEDecoder(io.MultiReader(bytes.NewReader(line), p.reader))
		return decoder.DecodeChunk()
	}

	// Plain text fallback
	return gobox.ExecChunk{
		Type:    gobox.ChunkTypeText,
		Content: string(line),
	}, nil
}

// StreamResult represents a parsed stream result.
type StreamResult struct {
	Chunks []gobox.ExecChunk
	Error  error
}

// CollectStream reads all chunks from a stream into a result.
func CollectStream(ctx context.Context, r io.Reader) *StreamResult {
	parser := NewStreamParser(r, FormatAuto)
	chunks, err := parser.Parse(ctx)
	return &StreamResult{
		Chunks: chunks,
		Error:  err,
	}
}

// StreamToExecResult converts a stream result to an ExecResult.
func StreamToExecResult(result *StreamResult) (*gobox.ExecResult, error) {
	if result.Error != nil && result.Error != io.EOF {
		return nil, result.Error
	}
	return &gobox.ExecResult{
		Chunks: result.Chunks,
	}, nil
}

// MergeTextChunks merges consecutive text chunks.
func MergeTextChunks(chunks []gobox.ExecChunk) []gobox.ExecChunk {
	if len(chunks) == 0 {
		return chunks
	}

	merged := make([]gobox.ExecChunk, 0, len(chunks))
	var textBuf bytes.Buffer
	lastWasText := false

	for _, chunk := range chunks {
		if chunk.Type == gobox.ChunkTypeText {
			textBuf.WriteString(chunk.Content)
			lastWasText = true
		} else {
			if lastWasText && textBuf.Len() > 0 {
				merged = append(merged, gobox.ExecChunk{
					Type:    gobox.ChunkTypeText,
					Content: textBuf.String(),
				})
				textBuf.Reset()
			}
			merged = append(merged, chunk)
			lastWasText = false
		}
	}

	if lastWasText && textBuf.Len() > 0 {
		merged = append(merged, gobox.ExecChunk{
			Type:    gobox.ChunkTypeText,
			Content: textBuf.String(),
		})
	}

	return merged
}

// ResponseMessage represents a JSON response message.
type ResponseMessage struct {
	Type    string          `json:"type"`
	Content string          `json:"content,omitempty"`
	Data    json.RawMessage `json:"data,omitempty"`
	Error   string          `json:"error,omitempty"`
}

// ToChunk converts a ResponseMessage to an ExecChunk.
func (m *ResponseMessage) ToChunk() gobox.ExecChunk {
	chunkType := gobox.ChunkType(m.Type)
	if chunkType == "" {
		chunkType = gobox.ChunkTypeText
	}

	content := m.Content
	if content == "" && m.Data != nil {
		content = string(m.Data)
	}
	if m.Error != "" {
		chunkType = gobox.ChunkTypeError
		content = m.Error
	}

	return gobox.ExecChunk{
		Type:    chunkType,
		Content: content,
	}
}
