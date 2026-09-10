<p align="center">
  <img src="logo.png" alt="Clerk logo" width="170" />
</p>

<h1 align="center">Clerk</h1>
<p align="center"><strong>Discover and manage AWS Parameter Store and Secrets Manager values</strong></p>

<p align="center">
  <a href="https://github.com/yachiko/clerk/actions/workflows/ci.yml"><img src="https://github.com/yachiko/clerk/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
  <a href="https://github.com/yachiko/clerk/releases"><img src="https://img.shields.io/github/v/release/yachiko/clerk" alt="Release"></a>
  <a href="LICENSE"><img src="https://img.shields.io/github/license/yachiko/clerk" alt="License"></a>
  <a href="go.mod"><img src="https://img.shields.io/github/go-mod/go-version/yachiko/clerk" alt="Go version"></a>
</p>

--- 

## Features

- **Put**: Create or update Parameter Store parameters and Secrets Manager text or binary secrets
- **Get**: Retrieve SSM parameters or Secrets Manager text/binary values with version support
- **Delete**: Remove secrets with confirmation
- **List**: List metadata from one selected backend with glob filtering
- **Copy/Move**: Duplicate or relocate Parameter Store parameters
- **Lifecycle**: Schedule, permanently delete, or restore Secrets Manager secrets
- **Tags**: Add, replace, or remove tags in either backend
- **Browse**: Provider-specific terminal UIs for exploring and managing resources
- **Cache**: Local caching for fast browsing and searching
- **Config**: Configuration management for profiles and preferences

## Installation

### Homebrew (macOS / Linux)

```sh
brew install yachiko/tap/clerk
```

### Using Go

```sh
go install github.com/yachiko/clerk/cmd/clerk@latest
```

### From Binary

Download the latest release from the [releases page](https://github.com/yachiko/clerk/releases).

```sh
# Linux/macOS
tar -xzf clerk-linux-amd64.tar.gz
sudo mv clerk-linux-amd64 /usr/local/bin/clerk
```

### From Source

```sh
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
clerk put "/dev/db_password" --stdin --tags "env=dev,team=backend" --backend ssm < ./db-password.txt

# Get a secret
clerk get "/dev/db_password" --backend ssm

# List Parameter Store parameters (the default backend)
clerk list "/dev/*"

# Browse interactively
clerk browse
```

## Commands

### Data Commands

| Command | Description | Usage |
| ------- | ----------- | ----- |
| `put` | Create or update a resource | `clerk put <name> [value] [flags]` |
| `get` | Retrieve a value | `clerk get <name[@version]> [flags]` |
| `delete` | Delete a resource | `clerk delete <name> [flags]` |
| `restore` | Restore a scheduled SM secret | `clerk restore <name-or-arn> --backend secretsmanager` |
| `tag` | Add or replace tags | `clerk tag <name-or-arn> <key=value...>` |
| `untag` | Remove tags | `clerk untag <name-or-arn> <key...>` |
| `list` | List backend metadata | `clerk list [pattern] [flags]` |
| `cp` | Copy an SSM parameter | `clerk cp <src> <dst> [flags]` |
| `mv` | Move an SSM parameter | `clerk mv <src> <dst> [flags]` |
| `browse` | Interactive provider UI | `clerk browse [flags]` |
| `refresh` | Refresh the selected cache | `clerk refresh [flags]` |

`--backend` accepts `ssm` (the default) or `secretsmanager`; `all` is not
supported. Every command operates on its selected backend. `cp` and `mv` are
SSM-only. `restore` is Secrets Manager-only.

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
| `region`            | `""` (SDK resolution) | Explicit AWS region override                                           |
| `profile`           | `""` (SDK default)    | AWS profile; empty uses standard SDK credential chain |
| `cache_ttl`         | `3h`                  | Cache time-to-live                                   |
| `clipboard_timeout` | `60s`                 | Clear clipboard after duration                       |
| `default_type`      | `SecureString`        | Default parameter type                               |
| `default_sort`      | `name`                | Default sort order                                   |
| `parallel_fetches`  | `10`                  | Concurrent API calls for refresh (1–50)               |
| `describe_page_size` | `50`                | Metadata page size (1–50)                             |
| `describe_max_items` | `0`                 | Refresh inventory cap; 0 means unlimited              |
| `describe_version_batch_size` | `10`       | History value batch size                             |
| `decrypt_by_default` | `false`             | Retrieve plaintext automatically in detail view       |
| `browse_auto_refresh` | `true`             | Refresh stale metadata when browse starts             |
| `browse_refresh_cooldown` | `5m`           | Minimum age for startup refresh                       |
| `search_slash_prefix` | `true`             | Start interactive search with `/`                     |

Cache files live under
`~/.clerk/cache/v2/<partition>/<account-id>/<region>/<backend>.json`. The v2
path isolates SSM and Secrets Manager caches as well as AWS partition, account,
and region. Values are never cached. The location isn't user-configurable.

Region and profile precedence is command flag, explicitly configured Clerk value,
then the AWS SDK environment/shared configuration. An unset region produces an
actionable error if the SDK cannot resolve one. Explicit `--profile default`
selects that profile even when `AWS_PROFILE` names a different profile. Leaving
the profile unset keeps the standard credential chain, including environment
credentials and roles. Confirm the resolved account and region before mutations.

Duration fields accept strings such as `3h` and legacy numeric nanoseconds;
configuration saves use strings. Negative durations and invalid concurrency or
pagination values are rejected. `cache_path` is deprecated and does not change
the account/region cache location.

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

Detail browsing starts masked by default. Reveal or copy explicitly retrieves a
value; older versions are loaded on demand. Closing detail drops retained value
references without promising physical memory erasure. Editors return through the
terminal UI lifecycle and preserve bytes. SSM edits skip unchanged saves and
reject detected concurrent updates; Secrets Manager edits create a new version.

Startup requires online AWS identity resolution. Cached fallback after a refresh
failure is visibly stale; offline startup without verified account identity is
unsupported. Automatic refresh is a startup freshness check, not a periodic monitor.

Run `clerk browse` for Parameter Store or `clerk browse --backend
secretsmanager` for the separate Secrets Manager UI. Both show metadata first;
values are fetched only when revealed, copied, or edited.

### Shared Shortcuts

| Key | Action |
| --- | ------ |
| `↑/↓`, `j/k`, `PgUp/PgDn`, `Home/End` | Navigate |
| `d` or `Enter` | Open details |
| `c` / `C` | Copy value / copy name |
| `e` | Edit value or create a new version |
| `Delete` | Delete (with confirmation) |
| `/` | Search/filter |
| `r` | Refresh |
| `Esc` / `q` | Back, cancel, or quit |

### Parameter Store Shortcuts

`t` toggles tree and flat views; `Space` expands a tree node; `s`/`S` change
sorting; `f` filters parameter type; `p` copies and `m` moves a parameter. In
details, `Tab`/`Shift+Tab` select versions, `g` selects the latest, `x` reveals,
`w` toggles wrapping, `a`/`r`/`m` manage version labels, and `T`/`D` add/remove
tags.

### Secrets Manager Shortcuts

`n` creates a secret; `T`/`D` add/remove tags; `u` restores a secret scheduled
for deletion. In details, `Tab`/`Shift+Tab` (or `j`/`k`) select versions, `g`
selects `AWSCURRENT`, and `x` reveals the selected value. Delete prompts for a
7-30 day recovery window or permanent deletion. Rotation information can be
viewed when AWS returns it; Clerk does not manage rotation.

## Examples

### Create Secrets

```bash
# Create a simple string secret
clerk put "/app/api_key" "sk_live_abc123" --backend ssm

# Create from file content
clerk put "/app/certificate" --file ./certs/cert.pem --backend ssm

# Create with tags
clerk put "/prod/db/password" "pass123" --tags "env=prod,team=backend,criticality=high" --backend ssm

# Create as StringList
clerk put "/app/allowed_hosts" "host1.com,host2.com,host3.com" --type StringList --backend ssm

# Create with specific KMS key
clerk put "/secure/secret" "value" --kms-key-id alias/my-key --backend ssm

# Create a Secrets Manager text secret from a file
clerk put "app/certificate" file://./certs/cert.pem --backend secretsmanager

# Create a Secrets Manager binary secret from a file
clerk put "app/certificate" fileb://./certs/cert.p12 --backend secretsmanager
```

Positional values are always literal, including text equal to an existing filename.
Use `--file` to read a file or `--stdin` to read standard input; these modes preserve
leading/trailing whitespace and newlines exactly. Missing files fail. This replaces
the old implicit filename detection and trimming behavior. For sensitive values,
prefer file/stdin input to keep them out of shell history and process arguments.

For Secrets Manager, a positional `file://PATH` creates or updates an exact text
value and `fileb://PATH` creates or updates raw binary bytes. `--file PATH` and
`--stdin` also provide exact text values. Creation accepts `--description`,
`--kms-key-id`, and `--tags`; updating an existing Secrets Manager secret writes
a new `AWSCURRENT` version and does not accept those creation-only options.

### Retrieve Secrets

```bash
# Get latest version
clerk get "/app/api_key" --backend ssm

# Get specific version
clerk get "/app/api_key@2" --backend ssm

# Get masked value
clerk get "/app/api_key" --mask --backend ssm

# Get only the value (for scripts)
clerk get "/app/api_key" --value --backend ssm

# Get as JSON
clerk get "/app/api_key" --output json --backend ssm

# Get the current Secrets Manager text or base64-encoded binary value
clerk get "app/api_key" --backend secretsmanager

# Select a Secrets Manager stage or opaque version ID
clerk get "app/api_key" --backend secretsmanager --stage AWSPREVIOUS
clerk get "app/api_key" --backend secretsmanager --version-id VERSION_ID
```

SSM `get --value` emits its exact string without adding a newline. Secrets Manager
binary values are base64 by default; `--raw --value` emits exact binary bytes.
Raw output can contain terminal controls, so redirect it to a file or pipe. Human
detail output escapes unsafe controls, JSON uses JSON escaping, and masked values
use a fixed mask without revealing length or fragments.

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

Listing and cache matching are case-sensitive. `*` crosses `/`, so
`/*/database/*` matches nested paths; a name without `*` is exact. Use `/dev/*`
for a hierarchy and `/` for all names, including flat parameter names. Character
classes and backslash escapes are unsupported and produce an error. Interactive
search also supports substring matching.

Inventory uses metadata APIs without retrieving values or history. `--tags`
requests tag enrichment and shows unavailable tags separately from an empty set.
A capped refresh is incomplete and preserves previously known unseen entries;
it cannot establish that those parameters have been deleted.

### Copy and Move

```bash
# Copy secret to new location
clerk cp "/dev/api_key" "/staging/api_key" --backend ssm

# Move (rename) secret
clerk mv "/old/path/secret" "/new/path/secret" --backend ssm
```

Transfers create destinations by default. Use `--overwrite` to deliberately replace
an existing target. Moves require confirmation; scripts must pass `--force`,
independently of output format. Copy/move decrypt and verify the destination and
preserve supported type, KMS key, tier, description, allowed pattern, policies, and
tags. Tag updates merge supplied keys; omitted keys remain unchanged.

A transfer copies the current value and supported metadata; it does not copy
labels or version history. A move deletes source history when the source is deleted.
If a step fails after a destination write, Clerk retains the destination and reports
the partial result. Inspect both resources before retrying. A source-delete timeout
may have completed remotely. The source version check reduces concurrent-update
risk but SSM read/check/delete and editor read/check/write are not atomic.

### Tags and Secrets Manager Lifecycle

```bash
# Add or replace tags in either backend
clerk tag "/app/api_key" env=prod owner=platform
clerk tag "app/api_key" --tags "env=prod,owner=platform" --backend secretsmanager

# Remove tags
clerk untag "/app/api_key" owner
clerk untag "app/api_key" --keys owner,temporary --backend secretsmanager

# Schedule Secrets Manager deletion with an explicit recovery period
clerk delete "app/api_key" --backend secretsmanager --recovery-window 14 --force

# Permanently delete a Secrets Manager secret
clerk delete "app/api_key" --backend secretsmanager --force-delete-without-recovery --force

# Cancel a scheduled deletion
clerk restore "app/api_key" --backend secretsmanager
```

`tag` accepts positional `key=value` pairs or `--tags`; `untag` accepts
positional keys or `--keys`. For Secrets Manager, noninteractive deletion
requires `--recovery-window` (7-30) or `--force-delete-without-recovery`.
`--force` skips confirmation only. Scheduled deletion can be restored; permanent
deletion and SSM deletion cannot.

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
- **Clipboard cleanup** - Expiry and normal UI shutdown clear the last Clerk-owned value only when clipboard contents still match; ownership checks are best effort and do not clear external clipboard history.
- **Editor files** - Editing uses private temporary storage with restrictive permissions and best-effort cleanup. Abrupt termination, editor-managed backups elsewhere, and physical erasure on SSD/COW filesystems are outside that guarantee.
- **Standard AWS auth** - Uses AWS SDK v2 with standard credential chain
- **Encryption support** - Full support for SecureString parameters with KMS

## Requirements

- AWS credentials configured (environment, config file, or IAM role)
- Appropriate IAM permissions for each selected backend

### IAM policy examples

Replace the example region `eu-west-1`, account `123456789012`, parameter prefix
`app/`, and KMS key ID with your own scope. These policies are additive: choose
inventory, add the reader policy for values, and add the writer policy for mutations.
Clerk resolves the account with `sts:GetCallerIdentity`; that operation does not
require an identity-policy grant. The examples have not been exercised in a live
account; validate them with your key policy and an intentionally denied resource
before deployment.

Inventory (`DescribeParameters` requires account-wide resource scope):

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": "ssm:DescribeParameters",
      "Resource": "*"
    },
    {
      "Effect": "Allow",
      "Action": "ssm:ListTagsForResource",
      "Resource": "arn:aws:ssm:eu-west-1:123456789012:parameter/app/*"
    }
  ]
}
```

Tag lookup is needed for enriched inventory and transfers. Plain metadata listing
does not require it. `DescribeParameters` exposes metadata across the selected
account and region; a parameter prefix in Clerk is a filter, not an IAM boundary.

Secrets Manager inventory and value reader:

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": "secretsmanager:ListSecrets",
      "Resource": "*"
    },
    {
      "Effect": "Allow",
      "Action": ["secretsmanager:ListSecretVersionIds", "secretsmanager:GetSecretValue"],
      "Resource": "arn:aws:secretsmanager:eu-west-1:123456789012:secret:app/*"
    }
  ]
}
```

Secrets encrypted with a customer-managed key also require `kms:Decrypt` as
permitted by the key policy. `list` and initial TUI browsing use metadata APIs;
values are requested only by `get`, reveal, copy, or text editing.

Secrets Manager writer (add the inventory/reader permissions above when the
workflow needs existence checks, cache reconciliation, or value editing):

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "secretsmanager:CreateSecret",
        "secretsmanager:PutSecretValue",
        "secretsmanager:TagResource",
        "secretsmanager:UntagResource",
        "secretsmanager:DeleteSecret",
        "secretsmanager:RestoreSecret",
        "secretsmanager:DescribeSecret"
      ],
      "Resource": "arn:aws:secretsmanager:eu-west-1:123456789012:secret:app/*"
    }
  ]
}
```

For customer-managed Secrets Manager keys, grant the KMS permissions AWS
requires for the chosen key through both IAM and the key policy. Scope the key
and secret ARNs to the intended resources. Clerk displays rotation metadata but
does not create, configure, or rotate secrets.

Value reader (including explicit history access):

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": ["ssm:GetParameter", "ssm:GetParameters", "ssm:GetParameterHistory"],
      "Resource": "arn:aws:ssm:eu-west-1:123456789012:parameter/app/*"
    },
    {
      "Effect": "Allow",
      "Action": "kms:Decrypt",
      "Resource": "arn:aws:kms:eu-west-1:123456789012:key/11111111-2222-3333-4444-555555555555",
      "Condition": {
        "StringEquals": {"kms:ViaService": "ssm.eu-west-1.amazonaws.com"}
      }
    }
  ]
}
```

Writer (also requires reader permissions for transfer verification and edits):

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "ssm:PutParameter", "ssm:DeleteParameter",
        "ssm:AddTagsToResource", "ssm:RemoveTagsFromResource",
        "ssm:LabelParameterVersion", "ssm:UnlabelParameterVersion"
      ],
      "Resource": "arn:aws:ssm:eu-west-1:123456789012:parameter/app/*"
    },
    {
      "Effect": "Allow",
      "Action": ["kms:Encrypt", "kms:GenerateDataKey"],
      "Resource": "arn:aws:kms:eu-west-1:123456789012:key/11111111-2222-3333-4444-555555555555",
      "Condition": {
        "StringEquals": {"kms:ViaService": "ssm.eu-west-1.amazonaws.com"}
      }
    }
  ]
}
```

Standard secure writes use `kms:Encrypt`; advanced secure writes use
`kms:GenerateDataKey`. Access also depends on the KMS key policy. See
[AWS Parameter Store KMS permissions](https://docs.aws.amazon.com/systems-manager/latest/userguide/secure-string-parameter-kms-encryption.html).
The exact `kms:ViaService` endpoint deliberately uses `StringEquals`; wildcard
patterns require `StringLike`, as documented in
[IAM condition operators](https://docs.aws.amazon.com/IAM/latest/UserGuide/reference_policies_elements_condition_operators.html).

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
