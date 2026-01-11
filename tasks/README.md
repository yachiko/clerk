# Clerk CLI - Task Implementation Guide

## Overview

This directory contains detailed task files for implementing the Clerk CLI tool. Tasks are ordered by dependency and should be completed sequentially.

## Task Summary

| Task | Name                                             | Description                                             | Estimated Effort |
| ---- | ------------------------------------------------ | ------------------------------------------------------- | ---------------- |
| 01   | [Project Setup](01-project-setup.md)             | Initialize Go module, dependencies, directory structure | 1-2 hours        |
| 02   | [Config Module](02-config-module.md)             | Configuration management system                         | 2-3 hours        |
| 03   | [AWS SSM Client](03-aws-ssm-client.md)           | AWS Parameter Store client wrapper                      | 3-4 hours        |
| 04   | [Cache Module](04-cache-module.md)               | Local caching for parameter metadata                    | 3-4 hours        |
| 05   | [Utility Modules](05-utility-modules.md)         | Clipboard, editor, output formatting, signals           | 2-3 hours        |
| 06   | [PUT Command](06-put-command.md)                 | Create/update secrets                                   | 2-3 hours        |
| 07   | [GET Command](07-get-command.md)                 | Retrieve secrets                                        | 2 hours          |
| 08   | [DELETE Command](08-delete-command.md)           | Delete secrets with confirmation                        | 2 hours          |
| 09   | [LIST Command](09-list-command.md)               | List secrets with filtering                             | 2-3 hours        |
| 10   | [CP/MV Commands](10-copy-move-commands.md)       | Copy and move secrets                                   | 2-3 hours        |
| 11   | [REFRESH Command](11-refresh-command.md)         | Manual cache refresh                                    | 2 hours          |
| 12   | [Browse UI Core](12-browse-ui-core.md)           | Interactive TUI framework                               | 4-5 hours        |
| 13   | [Browse UI Views](13-browse-ui-views.md)         | View rendering and styling                              | 3-4 hours        |
| 14   | [Browse UI Actions](14-browse-ui-actions.md)     | Edit and delete in browse mode                          | 3-4 hours        |
| 15   | [Shell Completion](15-shell-completion.md)       | Bash, zsh, fish completions                             | 2 hours          |
| 16   | [Build & Distribution](16-build-distribution.md) | Makefile, README, release process                       | 2 hours          |
| 17   | [Unit Tests](17-unit-tests.md)                   | Comprehensive unit tests across all modules             | 4-5 hours        |
| 18   | [Integration Tests](18-integration-tests.md)     | Integration tests with moto server, fixtures            | 4-5 hours        |

**Total Estimated Effort: 43-55 hours**

## Dependency Graph

```
01-project-setup
├── 02-config-module
│   ├── 03-aws-ssm-client
│   │   ├── 04-cache-module
│   │   │   ├── 06-put-command
│   │   │   ├── 07-get-command
│   │   │   ├── 08-delete-command
│   │   │   ├── 09-list-command
│   │   │   ├── 10-copy-move-commands
│   │   │   └── 11-refresh-command
│   │   └── 05-utility-modules
│   │       ├── 12-browse-ui-core
│   │       │   ├── 13-browse-ui-views
│   │       │   └── 14-browse-ui-actions
│   │       └── 15-shell-completion
│   └── 16-build-distribution
│       ├── 17-unit-tests
│       └── 18-integration-tests
```

## Implementation Notes for Claude Haiku 4.5

Each task file includes:

1. **Objective** - Clear goal statement
2. **Prerequisites** - Required prior tasks
3. **Deliverables** - Complete code with file paths
4. **Acceptance Criteria** - Checklist for completion
5. **Example Output** - Expected behavior
6. **Notes** - Implementation guidance

### Best Practices

- Read the entire task file before starting
- Follow the code exactly as provided
- Check acceptance criteria after implementation
- Test each command before moving to next task
- Keep files in the specified locations

### Testing Strategy

1. After each task, run `go build ./...` to verify compilation
2. For command tasks (06-11), test with actual AWS credentials
3. For UI tasks (12-14), run `clerk browse` and verify interactions
4. Run `make test` periodically to catch regressions
5. Run `make test-unit` for unit tests (Task 17)
6. Run `make test-integration` for integration tests with moto (Task 18)

## Quick Start

```bash
# After completing Task 01
cd /Users/ahoma/Projects/mTab/clerk
go mod tidy
go build ./cmd/clerk
./bin/clerk --help

# After completing all tasks
make all
./bin/clerk browse
```

## External Dependencies

```
github.com/spf13/cobra           # CLI framework
github.com/charmbracelet/bubbletea # Terminal UI
github.com/charmbracelet/bubbles  # UI components
github.com/charmbracelet/lipgloss # Styling
github.com/fatih/color           # Colored output
github.com/aws/aws-sdk-go-v2     # AWS SDK
github.com/schollz/progressbar/v3 # Progress bars
github.com/atotto/clipboard      # Clipboard access
github.com/stretchr/testify      # Testing assertions
```

## File Structure (Final)

```
clerk/
├── cmd/
│   └── clerk/
│       └── main.go
├── internal/
│   ├── aws/
│   │   ├── ssm.go
│   │   ├── types.go
│   │   ├── errors.go
│   │   ├── ssm_test.go
│   │   ├── types_test.go
│   │   └── errors_test.go
│   ├── cache/
│   │   ├── cache.go
│   │   ├── types.go
│   │   ├── cache_test.go
│   │   └── types_test.go
│   ├── cli/
│   │   ├── root.go
│   │   ├── exitcodes.go
│   │   ├── exitcodes_test.go
│   │   ├── commands_test.go
│   │   ├── put.go
│   │   ├── get.go
│   │   ├── delete.go
│   │   ├── list.go
│   │   ├── browse.go
│   │   ├── refresh.go
│   │   ├── config.go
│   │   ├── mv.go
│   │   ├── cp.go
│   │   ├── completion.go
│   │   └── completion_helpers.go
│   ├── config/
│   │   ├── config.go
│   │   ├── types.go
│   │   ├── config_test.go
│   │   └── types_test.go
│   ├── integration/
│   │   ├── ssm_test.go
│   │   ├── scale_test.go
│   │   └── benchmark_test.go
│   ├── testutil/
│   │   ├── helpers.go
│   │   ├── fixtures.go
│   │   └── integration.go
│   ├── ui/
│   │   ├── model.go
│   │   ├── types.go
│   │   ├── views.go
│   │   ├── tree.go
│   │   └── confirm.go
│   └── util/
│       ├── clipboard.go
│       ├── clipboard_test.go
│       ├── editor.go
│       ├── editor_test.go
│       ├── output.go
│       ├── output_test.go
│       ├── signal.go
│       └── signal_test.go
├── scripts/
│   ├── install-moto.sh
│   ├── run-integration-tests.sh
│   └── generate-fixtures.sh
├── tasks/
│   └── (task files)
├── go.mod
├── go.sum
├── Makefile
├── README.md
├── .gitignore
├── .goreleaser.yaml
└── docker-compose.test.yml
```
