.PHONY: all build build-cli build-server install test test-short test-cover lint fmt vet tidy clean docker docker-push run-server run-cli deps update-deps generate check release help

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod
GOFMT=gofmt
GOLINT=golangci-lint

# Binary names
CLI_BINARY=gobox
SERVER_BINARY=gobox-server

# Build directories
BUILD_DIR=bin
DOCKER_IMAGE=gobox
EXECUTOR_IMAGE=gobox-executor

# Version info
VERSION?=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME=$(shell date -u '+%Y-%m-%d_%H:%M:%S')
LDFLAGS=-ldflags "-X main.version=$(VERSION) -X main.buildTime=$(BUILD_TIME)"

all: build

## help: Show this help message
help:
	@echo "GoBox Makefile"
	@echo ""
	@echo "Usage:"
	@echo "  make [target]"
	@echo ""
	@echo "Targets:"
	@sed -n 's/^##//p' $(MAKEFILE_LIST) | column -t -s ':' |  sed -e 's/^/ /'

## build: Build all binaries and executor image
build: build-executor build-cli build-server

## build-cli: Build the CLI binary
build-cli:
	@echo "Building $(CLI_BINARY)..."
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(CLI_BINARY) ./cmd/gobox

## build-server: Build the server binary
build-server:
	@echo "Building $(SERVER_BINARY)..."
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(SERVER_BINARY) ./cmd/gobox-server

## install: Install binaries to GOPATH/bin
install:
	@echo "Installing binaries..."
	$(GOCMD) install $(LDFLAGS) ./cmd/gobox
	$(GOCMD) install $(LDFLAGS) ./cmd/gobox-server

## test: Run all tests
test:
	@echo "Running tests..."
	$(GOTEST) -v -race -cover ./...

## test-short: Run tests without integration tests
test-short:
	@echo "Running short tests..."
	$(GOTEST) -v -short ./...

## test-cover: Run tests with coverage report
test-cover:
	@echo "Running tests with coverage..."
	$(GOTEST) -v -race -coverprofile=coverage.out ./...
	$(GOCMD) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

## lint: Run linters
lint:
	@echo "Running linters..."
	$(GOLINT) run ./...

## fmt: Format code
fmt:
	@echo "Formatting code..."
	$(GOFMT) -s -w .

## vet: Run go vet
vet:
	@echo "Running go vet..."
	$(GOCMD) vet ./...

## tidy: Tidy go modules
tidy:
	@echo "Tidying modules..."
	$(GOMOD) tidy

## clean: Clean build artifacts
clean:
	@echo "Cleaning..."
	$(GOCLEAN)
	rm -rf $(BUILD_DIR)
	rm -f coverage.out coverage.html

## build-executor: Build the executor Docker image
build-executor:
	@echo "Building executor image $(EXECUTOR_IMAGE)..."
	@if ! docker images $(EXECUTOR_IMAGE):latest -q | grep -q .; then \
		docker build -t $(EXECUTOR_IMAGE):latest -f docker/Dockerfile.executor .; \
	else \
		echo "Executor image $(EXECUTOR_IMAGE):latest already exists, skipping build."; \
	fi

## build-executor-force: Force rebuild the executor Docker image
build-executor-force:
	@echo "Force building executor image $(EXECUTOR_IMAGE)..."
	docker build -t $(EXECUTOR_IMAGE):latest -f docker/Dockerfile.executor .

## docker: Build Docker image
docker:
	@echo "Building Docker image..."
	docker build -t $(DOCKER_IMAGE):$(VERSION) -t $(DOCKER_IMAGE):latest .

## docker-push: Push Docker image
docker-push:
	@echo "Pushing Docker image..."
	docker push $(DOCKER_IMAGE):$(VERSION)
	docker push $(DOCKER_IMAGE):latest

## run-server: Run the server locally
run-server: build-server
	@echo "Starting server..."
	./$(BUILD_DIR)/$(SERVER_BINARY)

## run-cli: Run the CLI with example
run-cli: build-cli
	@echo "Running CLI example..."
	./$(BUILD_DIR)/$(CLI_BINARY) exec "print('Hello from GoBox!')"

## deps: Download dependencies
deps:
	@echo "Downloading dependencies..."
	$(GOMOD) download

## update-deps: Update dependencies
update-deps:
	@echo "Updating dependencies..."
	$(GOMOD) tidy
	$(GOCMD) get -u ./...

## generate: Run go generate
generate:
	@echo "Running go generate..."
	$(GOCMD) generate ./...

## check: Run all checks (fmt, vet, lint, test)
check: fmt vet lint test
	@echo "All checks passed!"

## release: Build release binaries for multiple platforms
release: clean
	@echo "Building release binaries..."
	@mkdir -p $(BUILD_DIR)/release
	GOOS=linux GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/release/$(CLI_BINARY)-linux-amd64 ./cmd/gobox
	GOOS=linux GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/release/$(SERVER_BINARY)-linux-amd64 ./cmd/gobox-server
	GOOS=darwin GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/release/$(CLI_BINARY)-darwin-amd64 ./cmd/gobox
	GOOS=darwin GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/release/$(SERVER_BINARY)-darwin-amd64 ./cmd/gobox-server
	GOOS=darwin GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/release/$(CLI_BINARY)-darwin-arm64 ./cmd/gobox
	GOOS=darwin GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/release/$(SERVER_BINARY)-darwin-arm64 ./cmd/gobox-server
	GOOS=windows GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/release/$(CLI_BINARY)-windows-amd64.exe ./cmd/gobox
	GOOS=windows GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/release/$(SERVER_BINARY)-windows-amd64.exe ./cmd/gobox-server
	@echo "Release binaries built in $(BUILD_DIR)/release/"
