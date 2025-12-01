# GoBox - Sandboxed Code Execution for LLM Applications

GoBox is a high-performance sandbox code execution orchestrator written in Go. It provides secure, isolated environments for running Python and Bash code, designed specifically for LLM applications.

## Features

- **High Concurrency**: Leverages Go's goroutines to support thousands of concurrent sessions
- **Container Pooling**: Pre-warmed container pool for millisecond-level response times
- **Cloud Native**: Native support for Docker and Kubernetes orchestration
- **Single Binary**: No dependencies, cross-platform deployment
- **Streaming Execution**: Real-time output streaming via JSONL/SSE
- **Multiple Drivers**: Docker (local), Remote (API), Kubernetes (planned)

## Quick Start

### Installation

```bash
# Build from source
go build -o gobox ./cmd/gobox
go build -o gobox-server ./cmd/gobox-server

# Or use go install
go install gobox/cmd/gobox@latest
go install gobox/cmd/gobox-server@latest
```

### Basic Usage

```go
package main

import (
    "context"
    "fmt"
    "log"

    "gobox/pkg/gobox"
    _ "gobox/pkg/docker" // Register Docker driver
)

func main() {
    // Create a sandbox using Docker driver
    sandbox, err := gobox.New(
        gobox.WithDriver("docker"),
        gobox.WithImage("shroominic/codebox:latest"),
    )
    if err != nil {
        log.Fatal(err)
    }
    defer sandbox.Close()

    // Execute Python code
    result, err := sandbox.Exec(context.Background(), "print('Hello, World!')")
    if err != nil {
        log.Fatal(err)
    }

    fmt.Println(result.Text()) // Output: Hello, World!
}
```

### CLI Usage

```bash
# Execute code directly
gobox exec "print('Hello, World!')"

# Execute from file
gobox exec -f script.py

# Execute with bash kernel
gobox exec -k bash "echo Hello"

# Start interactive REPL
gobox repl

# Upload files
gobox upload data.csv

# Download files
gobox download output.csv

# Install packages
gobox install numpy pandas matplotlib

# Check health
gobox health
```

### Server Usage

```bash
# Start the API server
gobox-server --port 8069

# Start with custom configuration
gobox-server \
    --port 8080 \
    --driver docker \
    --image shroominic/codebox:latest \
    --pool-min 2 \
    --pool-max 10
```

## API Reference

### Sandbox Interface

```go
type Sandbox interface {
    // Execute code and return complete result
    Exec(ctx context.Context, code string, opts ...ExecOption) (*ExecResult, error)
    
    // Execute code with streaming output
    StreamExec(ctx context.Context, code string, opts ...ExecOption) (<-chan ExecChunk, <-chan error)
    
    // File operations
    Upload(ctx context.Context, files ...File) error
    Download(ctx context.Context, filename string) (io.ReadCloser, error)
    ListFiles(ctx context.Context) ([]FileInfo, error)
    
    // Package management
    Install(ctx context.Context, packages ...string) error
    
    // Lifecycle management
    HealthCheck(ctx context.Context) error
    Restart(ctx context.Context) error
    Close() error
}
```

### HTTP API

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/` | GET | Health check |
| `/health` | GET | Health check |
| `/exec` | POST | Execute code (streaming) |
| `/files` | GET | List files |
| `/files/upload` | POST | Upload files |
| `/files/download/{filename}` | GET | Download file |
| `/install` | POST | Install packages |
| `/restart` | POST | Restart sandbox |

### Request/Response Examples

**Execute Code:**
```bash
curl -X POST http://localhost:8069/exec \
  -H "Content-Type: application/json" \
  -d '{"code": "print(\"Hello\")", "kernel": "python"}'
```

**Response (JSONL streaming):**
```jsonl
{"type":"txt","content":"Hello\n"}
```

## Configuration

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `GOBOX_API_KEY` | API key for remote driver | - |
| `GOBOX_BASE_URL` | Base URL for remote driver | `https://codeboxapi.com/api/v2` |
| `GOBOX_DOCKER_IMAGE` | Docker image to use | `shroominic/codebox:latest` |
| `GOBOX_POOL_MIN_SIZE` | Minimum pool size | 2 |
| `GOBOX_POOL_MAX_SIZE` | Maximum pool size | 10 |
| `GOBOX_DEFAULT_TIMEOUT` | Default execution timeout | 60s |
| `GOBOX_SERVER_PORT` | Server port | 8069 |

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                          User Code                               │
│   sandbox := gobox.New(gobox.WithDriver("docker"))              │
│   result, _ := sandbox.Exec(ctx, "print('Hello')")              │
└─────────────────────────────────────────────────────────────────┘
                                 │
                                 ▼
┌─────────────────────────────────────────────────────────────────┐
│                     GoBox Core (pkg/gobox)                       │
│   Sandbox Interface, Types, Errors, Options                      │
└─────────────────────────────────────────────────────────────────┘
                                 │
          ┌──────────────────────┼──────────────────────┐
          ▼                      ▼                      ▼
┌─────────────────┐   ┌─────────────────┐   ┌─────────────────┐
│  DockerDriver   │   │  RemoteDriver   │   │   K8sDriver     │
│  (pkg/docker)   │   │  (pkg/remote)   │   │   (pkg/k8s)     │
│                 │   │                 │   │                 │
│  Container Pool │   │  HTTP Client    │   │  Pod Manager    │
│  Docker SDK     │   │  JSONL/SSE      │   │  client-go      │
└─────────────────┘   └─────────────────┘   └─────────────────┘
```

## Development

### Building

```bash
make build      # Build all binaries
make test       # Run tests
make lint       # Run linters
make docker     # Build Docker image
```

### Running Tests

```bash
# Unit tests
go test ./...

# Integration tests (requires Docker)
go test -tags=integration ./...

# With coverage
go test -cover ./...
```

## License

MIT License - see LICENSE file for details.

## Contributing

Contributions are welcome! Please read our contributing guidelines before submitting PRs.
