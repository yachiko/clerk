# Task 16: Build and Distribution

## Objective
Set up the build process for creating distributable binaries and document installation instructions.

## Prerequisites
- All previous tasks completed
- Go 1.21+ installed

## Deliverables

### 1. Create Makefile

Create file `Makefile`:

```makefile
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

.PHONY: all build clean test lint fmt vet deps install uninstall release help

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

## Run tests
test:
	@echo "Running tests..."
	$(GOTEST) -v -race -cover ./...

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

## Install binary to GOPATH/bin
install: build
	@echo "Installing $(BINARY_NAME)..."
	cp $(BINARY_PATH) $(GOPATH)/bin/$(BINARY_NAME)

## Uninstall binary from GOPATH/bin
uninstall:
	@echo "Uninstalling $(BINARY_NAME)..."
	rm -f $(GOPATH)/bin/$(BINARY_NAME)

## Build for all platforms
release:
	@echo "Building releases for version $(VERSION)..."
	@mkdir -p dist
	@for platform in $(PLATFORMS); do \
		GOOS=$${platform%/*} GOARCH=$${platform#*/} \
		CGO_ENABLED=0 $(GOBUILD) $(LDFLAGS) -o dist/$(BINARY_NAME)-$${platform%/*}-$${platform#*/}$$([ "$${platform%/*}" = "windows" ] && echo ".exe") ./cmd/clerk; \
		echo "  Built: $(BINARY_NAME)-$${platform%/*}-$${platform#*/}"; \
	done
	@echo "Creating archives..."
	@cd dist && for f in $(BINARY_NAME)-*; do \
		if [ "$${f##*.}" = "exe" ]; then \
			zip "$${f%.exe}.zip" "$$f"; \
		else \
			tar -czvf "$$f.tar.gz" "$$f"; \
		fi; \
	done

## Clean build artifacts
clean:
	@echo "Cleaning..."
	rm -rf bin/ dist/ coverage.out coverage.html

## Show help
help:
	@echo "Clerk CLI - Makefile targets"
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@echo "  all        - Run deps, lint, test, and build (default)"
	@echo "  build      - Build the binary"
	@echo "  dev        - Build for development (no optimizations)"
	@echo "  test       - Run tests"
	@echo "  coverage   - Run tests with coverage report"
	@echo "  lint       - Run linter"
	@echo "  vet        - Run go vet"
	@echo "  fmt        - Format code"
	@echo "  fmt-check  - Check code formatting"
	@echo "  deps       - Download and tidy dependencies"
	@echo "  install    - Install binary to GOPATH/bin"
	@echo "  uninstall  - Remove binary from GOPATH/bin"
	@echo "  release    - Build for all platforms"
	@echo "  clean      - Remove build artifacts"
	@echo "  help       - Show this help message"
```

### 2. Update Main with Build Info

Update `cmd/clerk/main.go`:

```go
package main

import (
	"os"

	"github.com/yachiko/clerk/internal/cli"
)

// Build information - set via ldflags
var (
	Version   = "dev"
	Commit    = "unknown"
	BuildTime = "unknown"
)

func main() {
	if err := cli.Execute(Version, Commit, BuildTime); err != nil {
		os.Exit(1)
	}
}
```

### 3. Update Root Command for Version Info

Update `internal/cli/root.go`:

```go
package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	profile   string
	region    string
	version   string
	commit    string
	buildTime string
)

func Execute(v, c, bt string) error {
	version = v
	commit = c
	buildTime = bt
	return rootCmd.Execute()
}

var rootCmd = &cobra.Command{
	Use:   "clerk",
	Short: "A CLI tool for managing secrets in AWS Parameter Store",
	Long: `Clerk is a command-line tool that helps you manage secrets stored in 
AWS Parameter Store. It provides commands for adding, retrieving, updating, 
deleting, and browsing secrets with an interactive terminal UI.`,
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	rootCmd.PersistentFlags().StringVar(&profile, "profile", "", "AWS profile to use")
	rootCmd.PersistentFlags().StringVar(&region, "region", "", "AWS region to use")
	
	rootCmd.AddCommand(versionCmd)
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version number of clerk",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("clerk %s\n", version)
		fmt.Printf("  commit: %s\n", commit)
		fmt.Printf("  built:  %s\n", buildTime)
	},
}
```

### 4. Create README.md

Create file `README.md`:

```markdown
# Clerk

A CLI tool for managing secrets in AWS Parameter Store.

## Features

- **Put**: Create or update secrets with tags and encryption
- **Get**: Retrieve secrets with version support
- **Delete**: Remove secrets with confirmation
- **List**: List secrets with glob pattern filtering
- **Copy/Move**: Duplicate or relocate secrets
- **Browse**: Interactive terminal UI for exploring secrets
- **Cache**: Local caching for fast browsing and searching

## Installation

### Using Go

```bash
go install github.com/yachiko/clerk/cmd/clerk@latest
```

### From Binary

Download the latest release from the [releases page](https://github.com/yachiko/clerk/releases).

```bash
# Linux/macOS
tar -xzf clerk-linux-amd64.tar.gz
sudo mv clerk-linux-amd64 /usr/local/bin/clerk

# macOS with Homebrew (coming soon)
brew install yachiko/tap/clerk
```

### From Source

```bash
git clone https://github.com/yachiko/clerk.git
cd clerk
make install
```

## Quick Start

```bash
# Configure AWS credentials (if not already done)
export AWS_PROFILE=myprofile
export AWS_REGION=us-east-1

# Or use clerk config
clerk config set profile myprofile
clerk config set region us-east-1

# Create a secret
clerk put "/dev/db_password" "mysecretpassword" --tags "env=dev,team=backend"

# Get a secret
clerk get "/dev/db_password"

# List secrets
clerk list "/dev/*"

# Browse interactively
clerk browse
```

## Commands

| Command | Description |
|---------|-------------|
| `put` | Create or update a secret |
| `get` | Retrieve a secret value |
| `delete` | Delete a secret |
| `list` | List secrets with filtering |
| `cp` | Copy a secret to a new path |
| `mv` | Move/rename a secret |
| `browse` | Interactive terminal UI |
| `refresh` | Refresh the local cache |
| `config` | Manage configuration |
| `completion` | Generate shell completions |
| `version` | Show version information |

## Configuration

Configuration is stored in `~/.clerk/config.json`.

| Option | Default | Description |
|--------|---------|-------------|
| `region` | `us-east-1` | AWS region |
| `profile` | `default` | AWS profile |
| `cache_path` | `~/.clerk/cache.json` | Cache file location |
| `cache_ttl` | `3h` | Cache time-to-live |
| `clipboard_timeout` | `60s` | Clear clipboard after duration |
| `default_type` | `SecureString` | Default parameter type |
| `default_sort` | `name` | Default sort order |
| `parallel_fetches` | `10` | Concurrent API calls for refresh |

## Browse Mode

The browse mode provides a k9s-style interface for managing secrets.

### Keyboard Shortcuts

| Key | Action |
|-----|--------|
| `↑/↓` or `j/k` | Navigate up/down |
| `PgUp/PgDn` | Move page up/down |
| `Home/End` | Jump to first/last |
| `d` or `Enter` | Describe secret |
| `c` | Copy value to clipboard |
| `e` | Edit in $EDITOR |
| `Delete` | Delete (with confirmation) |
| `/` | Search/filter |
| `t` | Toggle tree/flat view |
| `Space` | Expand/collapse (tree view) |
| `x` | Toggle value masking (describe view) |
| `Esc` | Back/cancel |
| `q` | Quit |

## Shell Completion

```bash
# Bash
clerk completion bash > /etc/bash_completion.d/clerk

# Zsh
clerk completion zsh > "${fpath[1]}/_clerk"

# Fish
clerk completion fish > ~/.config/fish/completions/clerk.fish
```

## Security

- Secret values are never cached locally
- Clipboard is automatically cleared after 60 seconds (configurable)
- Temp files for editing are securely deleted
- Uses AWS SDK v2 with standard credential chain

## Requirements

- AWS credentials configured (environment, config file, or IAM role)
- Appropriate IAM permissions for SSM Parameter Store

### Required IAM Permissions

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "ssm:GetParameter",
        "ssm:GetParameters",
        "ssm:GetParametersByPath",
        "ssm:GetParameterHistory",
        "ssm:PutParameter",
        "ssm:DeleteParameter",
        "ssm:DescribeParameters",
        "ssm:ListTagsForResource"
      ],
      "Resource": "*"
    }
  ]
}
```

## License

MIT License - see [LICENSE](LICENSE) for details.
```

### 5. Create .gitignore

Create file `.gitignore`:

```gitignore
# Binaries
bin/
dist/
*.exe
clerk

# Test coverage
coverage.out
coverage.html

# IDE
.idea/
.vscode/
*.swp
*.swo

# OS
.DS_Store
Thumbs.db

# Go
vendor/

# Local config (for testing)
.clerk/
```

### 6. Create goreleaser Config (Optional)

Create file `.goreleaser.yaml`:

```yaml
project_name: clerk

before:
  hooks:
    - go mod tidy

builds:
  - main: ./cmd/clerk
    binary: clerk
    env:
      - CGO_ENABLED=0
    goos:
      - linux
      - darwin
      - windows
    goarch:
      - amd64
      - arm64
    ldflags:
      - -s -w
      - -X main.Version={{.Version}}
      - -X main.Commit={{.ShortCommit}}
      - -X main.BuildTime={{.Date}}

archives:
  - format: tar.gz
    name_template: "{{ .ProjectName }}_{{ .Version }}_{{ .Os }}_{{ .Arch }}"
    format_overrides:
      - goos: windows
        format: zip

checksum:
  name_template: 'checksums.txt'

snapshot:
  name_template: "{{ incpatch .Version }}-next"

changelog:
  sort: asc
  filters:
    exclude:
      - '^docs:'
      - '^test:'
      - '^chore:'
```

## Acceptance Criteria

- [ ] `make build` produces working binary
- [ ] `make test` runs all tests
- [ ] `make release` builds for all platforms
- [ ] `make install` installs to GOPATH/bin
- [ ] `./clerk version` shows version, commit, and build time
- [ ] Binary is statically linked (CGO_ENABLED=0)
- [ ] Binary size is optimized (-s -w flags)
- [ ] README documents all commands and options
- [ ] .gitignore excludes appropriate files
- [ ] Cross-compilation works for linux, darwin, windows (amd64, arm64)

## Build Commands Summary

```bash
# Development build
make dev

# Production build
make build

# Run tests
make test

# Build all platforms
make release

# Install locally
make install

# Clean up
make clean
```

## Notes

- Version is determined from git tags or defaults to "dev"
- Cross-compilation enabled for major platforms
- CGO disabled for maximum portability
- Binary stripped of debug info for smaller size
- GoReleaser config provided for GitHub releases automation
