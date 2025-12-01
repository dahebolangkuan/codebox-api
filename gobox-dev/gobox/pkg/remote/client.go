package remote

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"gobox/pkg/gobox"
	"gobox/pkg/protocol"
)

// Client provides low-level HTTP operations for the remote API.
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
	headers    map[string]string
}

// NewClient creates a new HTTP client for the remote API.
func NewClient(baseURL, apiKey string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	return &Client{
		baseURL:    strings.TrimSuffix(baseURL, "/"),
		apiKey:     apiKey,
		httpClient: httpClient,
		headers:    make(map[string]string),
	}
}

// SetHeader sets a custom header for all requests.
func (c *Client) SetHeader(key, value string) {
	c.headers[key] = value
}

// Do executes an HTTP request with authentication and custom headers.
func (c *Client) Do(req *http.Request) (*http.Response, error) {
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("User-Agent", "GoBox/1.0")

	for k, v := range c.headers {
		req.Header.Set(k, v)
	}

	return c.httpClient.Do(req)
}

// Get performs a GET request.
func (c *Client) Get(ctx context.Context, path string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+path, nil)
	if err != nil {
		return nil, err
	}
	return c.Do(req)
}

// Post performs a POST request with JSON body.
func (c *Client) Post(ctx context.Context, path string, body interface{}) (*http.Response, error) {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+path, reader)
	if err != nil {
		return nil, err
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	return c.Do(req)
}

// PostRaw performs a POST request with raw body.
func (c *Client) PostRaw(ctx context.Context, path string, contentType string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+path, body)
	if err != nil {
		return nil, err
	}

	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	return c.Do(req)
}

// StreamResponse reads a streaming response and sends chunks to a channel.
func StreamResponse(ctx context.Context, body io.Reader, chunkChan chan<- gobox.ExecChunk) error {
	reader := bufio.NewReader(body)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		line, err := reader.ReadBytes('\n')
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}

		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}

		// Try JSONL format
		chunk, err := protocol.DecodeJSONL(line)
		if err == nil {
			select {
			case chunkChan <- chunk:
			case <-ctx.Done():
				return ctx.Err()
			}
			continue
		}

		// Try legacy format
		chunks := protocol.ParseLegacy(string(line))
		for _, c := range chunks {
			select {
			case chunkChan <- c:
			case <-ctx.Done():
				return ctx.Err()
			}
		}

		// Plain text fallback
		if len(chunks) == 0 {
			select {
			case chunkChan <- gobox.ExecChunk{Type: gobox.ChunkTypeText, Content: string(line)}:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
}

// ParseResponse parses a non-streaming JSON response.
func ParseResponse(body io.Reader, v interface{}) error {
	return json.NewDecoder(body).Decode(v)
}

// ParseError parses an error response.
func ParseError(resp *http.Response) error {
	if resp.StatusCode == http.StatusUnauthorized {
		return gobox.ErrUnauthorized
	}
	if resp.StatusCode == http.StatusNotFound {
		return gobox.ErrFileNotFound
	}
	if resp.StatusCode >= 400 && resp.StatusCode < 500 {
		body, _ := io.ReadAll(resp.Body)
		return gobox.NewExecError("client", string(body))
	}
	if resp.StatusCode >= 500 {
		body, _ := io.ReadAll(resp.Body)
		return gobox.NewSystemError(string(body))
	}
	return nil
}
