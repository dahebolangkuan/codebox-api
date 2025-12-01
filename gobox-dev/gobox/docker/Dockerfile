# Build stage
FROM golang:1.22-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git ca-certificates

WORKDIR /app

# Copy go mod files first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the server binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-w -s" \
    -o /gobox-server \
    ./cmd/gobox-server

# Runtime stage
FROM alpine:3.19

# Install runtime dependencies
RUN apk add --no-cache \
    ca-certificates \
    docker-cli \
    tzdata

# Create non-root user
RUN adduser -D -g '' gobox
USER gobox

# Copy binary from builder
COPY --from=builder /gobox-server /usr/local/bin/gobox-server

# Expose port
EXPOSE 8069

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8069/health || exit 1

# Set entrypoint
ENTRYPOINT ["gobox-server"]
