// Package protocol provides encoding and decoding for GoBox communication protocols.
// It supports multiple formats including JSONL (JSON Lines), SSE (Server-Sent Events),
// and the legacy tag-based protocol (<txt>, <img>, <err>).
package protocol

import (
	"strings"

	"gobox/pkg/gobox"
)

// ParseLegacy parses the legacy tag-based protocol format.
// Format: <txt>text content</txt><img>base64 image</img><err>error message</err>
//
// Example input: "<txt>Hello</txt><img>iVBORw0KGgo...</img><txt>World</txt>"
func ParseLegacy(data string) []gobox.ExecChunk {
	chunks := make([]gobox.ExecChunk, 0)
	
	// Process each tag type
	for _, tagType := range []string{"txt", "img", "err"} {
		openTag := "<" + tagType + ">"
		closeTag := "</" + tagType + ">"
		
		remaining := data
		for {
			// Find opening tag
			start := strings.Index(remaining, openTag)
			if start == -1 {
				break
			}
			
			// Find closing tag after opening tag
			contentStart := start + len(openTag)
			end := strings.Index(remaining[contentStart:], closeTag)
			if end == -1 {
				break
			}
			
			// Extract content
			content := remaining[contentStart : contentStart+end]
			
			chunks = append(chunks, gobox.ExecChunk{
				Type:    gobox.ChunkType(tagType),
				Content: content,
			})
			
			// Move past this match
			remaining = remaining[contentStart+end+len(closeTag):]
		}
	}
	
	// Sort chunks by their original position in the data
	// This ensures order is preserved
	return sortChunksByPosition(data, chunks)
}

// EncodeLegacy encodes chunks to the legacy tag-based format.
func EncodeLegacy(chunks []gobox.ExecChunk) string {
	var builder strings.Builder

	for _, chunk := range chunks {
		builder.WriteString("<")
		builder.WriteString(string(chunk.Type))
		builder.WriteString(">")
		builder.WriteString(chunk.Content)
		builder.WriteString("</")
		builder.WriteString(string(chunk.Type))
		builder.WriteString(">")
	}

	return builder.String()
}

// EncodeLegacyChunk encodes a single chunk to legacy format.
func EncodeLegacyChunk(chunk gobox.ExecChunk) string {
	return "<" + string(chunk.Type) + ">" + chunk.Content + "</" + string(chunk.Type) + ">"
}

// sortChunksByPosition sorts chunks by their position in the original data.
func sortChunksByPosition(data string, chunks []gobox.ExecChunk) []gobox.ExecChunk {
	type posChunk struct {
		pos   int
		chunk gobox.ExecChunk
	}

	// Find position of each chunk in original data
	posChunks := make([]posChunk, 0, len(chunks))
	for _, chunk := range chunks {
		encoded := EncodeLegacyChunk(chunk)
		pos := strings.Index(data, encoded)
		posChunks = append(posChunks, posChunk{pos: pos, chunk: chunk})
	}

	// Simple bubble sort by position (chunks list is usually small)
	for i := 0; i < len(posChunks)-1; i++ {
		for j := 0; j < len(posChunks)-i-1; j++ {
			if posChunks[j].pos > posChunks[j+1].pos {
				posChunks[j], posChunks[j+1] = posChunks[j+1], posChunks[j]
			}
		}
	}

	// Extract sorted chunks
	result := make([]gobox.ExecChunk, len(posChunks))
	for i, pc := range posChunks {
		result[i] = pc.chunk
	}

	return result
}
