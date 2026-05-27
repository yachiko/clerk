<p align="center">
  <img src="logo.png" alt="Clerk logo" width="170" />
</p>

<h1 align="center">Clerk</h1>
<p align="center"><strong>Tool for managing secrets in AWS Parameter Store with an interactive terminal UI</strong></p>

<p align="center">
  <a href="https://github.com/yachiko/clerk/actions/workflows/ci.yml"><img src="https://github.com/yachiko/clerk/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
  <a href="https://github.com/yachiko/clerk/releases"><img src="https://img.shields.io/github/v/release/yachiko/clerk" alt="Release"></a>
  <a href="LICENSE"><img src="https://img.shields.io/github/license/yachiko/clerk" alt="License"></a>
  <a href="go.mod"><img src="https://img.shields.io/github/go-mod/go-version/yachiko/clerk" alt="Go version"></a>
</p>

--- 

## Features

- **Put**: Create or update secrets with tags and encryption
- **Get**: Retrieve secrets with version support and masking
- **Delete**: Remove secrets with confirmation
- **List**: List secrets with glob pattern filtering and sorting
- **Copy/Move**: Duplicate or relocate secrets
- **Browse**: Interactive k9s-style terminal UI for exploring and managing secrets
- **Cache**: Local caching for fast browsing and searching
- **Config**: Configuration management for profiles and preferences

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

### Data Commands

| Command  | Description                 | Usage                                      |
| -------- | --------------------------- | ------------------------------------------ |
| `put`    | Create or update a secret   | `clerk put <name> <value\|file> [flags]` |
| `get`    | Retrieve a secret value     | `clerk get <name[@version]> [flags]`     |
| `delete` | Delete a secret             | `clerk delete <name> [flags]`             |
| `list`   | List secrets with filtering | `clerk list [path] [flags]`               |
| `cp`     | Copy a secret to a new path | `clerk cp <src> <dst> [flags]`            |
| `mv`     | Move/rename a secret        | `clerk mv <src> <dst> [flags]`            |
| `browse` | Interactive terminal UI     | `clerk browse [flags]`                    |
| `refresh`| Refresh the local cache     | `clerk refresh [flags]`                   |

### Management Commands

| Command     | Description              | Usage                           |
| ----------- | ------------------------ | ------------------------------- |
| `config`    | Manage configuration     | `clerk config <get\|set\|show>` |
| `completion`| Generate shell completions | `clerk completion <shell>`      |
| `version`   | Show version information | `clerk version`                 |

## Configuration

Configuration is stored in `~/.clerk/config.json`.

### Available Settings

| Option              | Default               | Description                                          |
| ------------------- | --------------------- | ---------------------------------------------------- |
| `region`            | `us-east-1`           | AWS region                                           |
| `profile`           | `""` (SDK default)    | AWS profile; empty uses standard SDK credential chain |
| `cache_ttl`         | `3h`                  | Cache time-to-live                                   |
| `clipboard_timeout` | `60s`                 | Clear clipboard after duration                       |
| `default_type`      | `SecureString`        | Default parameter type                               |
| `default_sort`      | `name`                | Default sort order                                   |
| `parallel_fetches`  | `10`                  | Concurrent API calls for refresh                     |

Cache files live under `~/.clerk/cache/<account-id>/<region>.json` — separate
files per AWS account and region, so switching profiles doesn't invalidate
unrelated caches. The location isn't user-configurable.

### Example Config

```json
{
  "region": "us-east-1",
  "profile": "production",
  "cache_ttl": "3h0m0s",
  "clipboard_timeout": "60s",
  "default_type": "SecureString",
  "default_sort": "name",
  "parallel_fetches": 10
}
```

## Browse Mode

The browse mode provides an interactive terminal UI similar to k9s for Kubernetes.

### Keyboard Shortcuts

| Key            | Action                               |
| -------------- | ------------------------------------ |
| `↑/↓` or `j/k` | Navigate up/down                     |
| `PgUp/PgDn`    | Move page up/down                    |
| `Home/End`     | Jump to first/last                   |
| `d` or `Enter` | Describe secret (show details)       |
| `c`            | Copy value to clipboard              |
| `e`            | Edit in $EDITOR                      |
| `Delete`       | Delete (with confirmation)           |
| `/`            | Search/filter                        |
| `t`            | Toggle tree/flat view                |
| `Space`        | Expand/collapse (tree view)          |
| `x`            | Toggle value masking (describe view) |
| `Esc`          | Back/cancel                          |
| `q`            | Quit                                 |

## Examples

### Create Secrets

```bash
# Create a simple string secret
clerk put "/app/api_key" "sk_live_abc123"

# Create from file content
clerk put "/app/certificate" ./certs/cert.pem

# Create with tags
clerk put "/prod/db/password" "pass123" --tags "env=prod,team=backend,criticality=high"

# Create as StringList
clerk put "/app/allowed_hosts" "host1.com,host2.com,host3.com" --type StringList

# Create with specific KMS key
clerk put "/secure/secret" "value" --kms-key-id alias/my-key
```

### Retrieve Secrets

```bash
# Get latest version
clerk get "/app/api_key"

# Get specific version
clerk get "/app/api_key@2"

# Get masked value
clerk get "/app/api_key" --mask

# Get only the value (for scripts)
clerk get "/app/api_key" --value

# Get as JSON
clerk get "/app/api_key" --output json
```

### List and Filter

```bash
# List all secrets
clerk list

# List under specific path
clerk list "/prod/*"

# List nested path
clerk list "/*/database/*"

# List sorted by modification date
clerk list "/dev/*" --sort modified

# List with tags
clerk list --tags
```

### Copy and Move

```bash
# Copy secret to new location
clerk cp "/dev/api_key" "/staging/api_key"

# Move (rename) secret
clerk mv "/old/path/secret" "/new/path/secret"
```

## Shell Completion

Enable auto-completion in your shell:

```bash
# Bash
clerk completion bash > /etc/bash_completion.d/clerk

# Zsh
clerk completion zsh > "${fpath[1]}/_clerk"

# Fish
clerk completion fish > ~/.config/fish/completions/clerk.fish

# Load in current session
source <(clerk completion bash)
```

## Security

- **Secret values are never cached** - Only metadata is cached locally
- **Clipboard auto-clear** - Clipboard is automatically cleared after 60 seconds (configurable)
- **Secure temp files** - Temp files for editing are securely deleted
- **Standard AWS auth** - Uses AWS SDK v2 with standard credential chain
- **Encryption support** - Full support for SecureString parameters with KMS

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
    },
    {
      "Effect": "Allow",
      "Action": [
        "kms:Decrypt",
        "kms:DescribeKey"
      ],
      "Resource": "*",
      "Condition": {
        "StringEquals": {
          "kms:ViaService": [
            "ssm.*.amazonaws.com"
          ]
        }
      }
    }
  ]
}
```

## Building

### Development Build

```bash
make dev
```

### Production Build

```bash
make build
```

### Cross-Compile for Multiple Platforms

```bash
make release
```

This creates binaries for:
- Linux (amd64, arm64)
- macOS (amd64, arm64)
- Windows (amd64, arm64)

## Testing

### Run Unit Tests

```bash
make test-unit
```

### Run with Coverage

```bash
make coverage
```

### Run Integration Tests (requires Docker)

```bash
make test-integration
```

### Run All Tests

```bash
make test-all
```

## Development

### Project Structure

```
clerk/
├── cmd/clerk/          # CLI entry point
├── internal/
│   ├── aws/           # AWS SSM client
│   ├── cache/         # Local caching
│   ├── cli/           # Command implementations
│   ├── config/        # Configuration management
│   ├── testutil/      # Testing utilities
│   ├── ui/            # Terminal UI
│   └── util/          # Helper utilities
├── tasks/             # Task definitions
├── Makefile           # Build automation
└── README.md          # This file
```

### Code Style

Code follows [idiomatic Go](https://go.dev/doc/effective_go) practices and is formatted with `gofmt`.

```bash
# Format code
make fmt

# Check formatting
make fmt-check

# Run linter
make lint
```

## Contributing

Contributions are welcome! Please ensure:

1. Code is properly formatted (`make fmt`)
2. All tests pass (`make test`)
3. Code follows idiomatic Go practices
4. Commit messages are clear and descriptive

## License

MIT License - see [LICENSE](LICENSE) for details.

## Support

For issues, questions, or feature requests, please open a GitHub issue.
