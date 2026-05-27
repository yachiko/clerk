# Clerk Makefile

# Build variables
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME ?= $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')
LDFLAGS := -ldflags "-s -w -X main.Version=$(VERSION) -X main.Commit=$(COMMIT) -X main.BuildTime=$(BUILD_TIME)"

# Go variables
GOCMD := go
GOBUILD := $(GOCMD) build
GOTEST := $(GOCMD) test
GOMOD := $(GOCMD) mod
GOVET := $(GOCMD) vet
GOFMT := gofmt

# Binary name
BINARY_NAME := clerk
BINARY_PATH := bin/$(BINARY_NAME)

# Platforms for cross-compilation
PLATFORMS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64

.PHONY: all build dev clean test coverage lint fmt fmt-check vet deps install uninstall tag test-unit test-integration test-all moto-start moto-stop help

## Default target
all: deps lint test build

## Build the binary
build:
	@echo "Building $(BINARY_NAME) $(VERSION)..."
	@mkdir -p bin
	CGO_ENABLED=0 $(GOBUILD) $(LDFLAGS) -o $(BINARY_PATH) ./cmd/clerk

## Build for current OS/arch (development)
dev:
	@echo "Building $(BINARY_NAME) for development..."
	$(GOBUILD) -o $(BINARY_PATH) ./cmd/clerk

## Run all tests (unit + integration)
test: test-unit

## Run unit tests only
test-unit:
	@echo "Running unit tests..."
	$(GOTEST) -v -race -short ./...

## Run unit tests with verbose output
test-verbose:
	@echo "Running tests with verbose output..."
	$(GOTEST) -v -race ./... 2>&1 | tee test-output.log

## Run tests with coverage report
coverage:
	@echo "Running tests with coverage..."
	$(GOTEST) -v -race -coverprofile=coverage.out ./...
	$(GOCMD) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

## Run linter
lint:
	@echo "Running linter..."
	@if command -v golangci-lint > /dev/null; then \
		golangci-lint run ./...; \
	else \
		echo "golangci-lint not installed, skipping..."; \
	fi

## Run go vet
vet:
	@echo "Running go vet..."
	$(GOVET) ./...

## Format code
fmt:
	@echo "Formatting code..."
	$(GOFMT) -s -w .

## Check formatting
fmt-check:
	@echo "Checking code formatting..."
	@if [ -n "$$($(GOFMT) -s -l .)" ]; then \
		echo "Code is not formatted. Run 'make fmt'"; \
		exit 1; \
	fi

## Download dependencies
deps:
	@echo "Downloading dependencies..."
	$(GOMOD) download
	$(GOMOD) tidy

## Install binary to /usr/local/bin
install: build
	@echo "Installing $(BINARY_NAME)..."
	cp $(BINARY_PATH) /usr/local/bin/$(BINARY_NAME)

## Uninstall binary from /usr/local/bin
uninstall:
	@echo "Uninstalling $(BINARY_NAME)..."
	rm -f /usr/local/bin/$(BINARY_NAME)

## Create and push next patch version tag (vX.Y.(Z+1)); release pipeline builds artifacts via GoReleaser
tag:
	@set -e; \
	last=$$(git tag --list 'v*' --sort=-v:refname | head -1); \
	if [ -z "$$last" ]; then \
	  new="v0.0.1"; \
	else \
	  ver=$${last#v}; \
	  major=$${ver%%.*}; rest=$${ver#*.}; minor=$${rest%%.*}; patch=$${rest#*.}; \
	  patch=$$((patch+1)); \
	  new="v$$major.$$minor.$$patch"; \
	fi; \
	echo "Last tag: $$last"; \
	echo "New tag: $$new"; \
	git tag $$new; \
	git push origin $$new; \
	echo "✅ Created and pushed $$new"

## Start moto server for integration tests
moto-start:
	@echo "Starting moto server..."
	docker-compose -f docker-compose.test.yml up -d
	@echo "Waiting for moto to be ready..."
	@sleep 3

## Stop moto server
moto-stop:
	@echo "Stopping moto server..."
	docker-compose -f docker-compose.test.yml down

## Run integration tests
test-integration: build moto-start
	@echo "Running integration tests..."
	MOTO_ENDPOINT=http://localhost:5000 $(GOTEST) -v -tags=integration ./internal/integration/...
	$(MAKE) moto-stop

## Run integration tests with fixtures (hundreds of parameters)
test-integration-large: build moto-start
	@echo "Running large scale integration tests..."
	MOTO_ENDPOINT=http://localhost:5000 $(GOTEST) -v -tags=integration -timeout 10m ./internal/integration/... -run "LargeScale"
	$(MAKE) moto-stop

## Run integration benchmarks
bench-integration: build moto-start
	@echo "Running integration benchmarks..."
	MOTO_ENDPOINT=http://localhost:5000 $(GOTEST) -v -tags=integration -bench=. -benchmem ./internal/integration/...
	$(MAKE) moto-stop

## Run all tests (unit + integration)
test-all: test-unit test-integration

## Clean build artifacts
clean:
	@echo "Cleaning..."
	rm -rf bin/ dist/ coverage.out coverage.html test-output.log

## Show help
help:
	@echo "Clerk CLI - Makefile targets"
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@echo "  all                 - Run deps, lint, test, and build (default)"
	@echo "  build               - Build the binary"
	@echo "  dev                 - Build for development (no optimizations)"
	@echo "  test                - Run unit tests"
	@echo "  test-unit           - Run unit tests only"
	@echo "  test-verbose        - Run tests with verbose output"
	@echo "  test-integration    - Run integration tests with moto"
	@echo "  test-integration-lg - Run large scale integration tests"
	@echo "  bench-integration   - Run integration benchmarks"
	@echo "  test-all            - Run unit + integration tests"
	@echo "  coverage            - Run tests with coverage report"
	@echo "  lint                - Run linter"
	@echo "  vet                 - Run go vet"
	@echo "  fmt                 - Format code"
	@echo "  fmt-check           - Check code formatting"
	@echo "  deps                - Download and tidy dependencies"
	@echo "  install             - Install binary to GOPATH/bin"
	@echo "  uninstall           - Remove binary from GOPATH/bin"
	@echo "  tag                 - Create and push next patch version tag"
	@echo "  moto-start          - Start moto server for integration tests"
	@echo "  moto-stop           - Stop moto server"
	@echo "  clean               - Remove build artifacts"
	@echo "  help                - Show this help message"
