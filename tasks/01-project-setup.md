# Task 01: Project Setup and Structure

## Objective
Initialize the Go project with proper module structure, dependencies, and foundational code organization.

## Prerequisites
- Go 1.21+ installed
- Git initialized in the repository

## Deliverables

### 1. Initialize Go Module

Create `go.mod` with the following content:

```go
module github.com/yachiko/clerk

go 1.21
```

### 2. Install Dependencies

Run the following commands to add required dependencies:

```bash
go get github.com/spf13/cobra@latest
go get github.com/manifoldco/promptui@latest
go get github.com/fatih/color@latest
go get github.com/aws/aws-sdk-go-v2@latest
go get github.com/aws/aws-sdk-go-v2/config@latest
go get github.com/aws/aws-sdk-go-v2/service/ssm@latest
go get github.com/schollz/progressbar/v3@latest
go get github.com/atotto/clipboard@latest
```

### 3. Create Directory Structure

```
clerk/
├── cmd/
│   └── clerk/
│       └── main.go
├── internal/
│   ├── aws/
│   │   └── ssm.go
│   ├── cache/
│   │   └── cache.go
│   ├── cli/
│   │   ├── root.go
│   │   ├── put.go
│   │   ├── get.go
│   │   ├── delete.go
│   │   ├── list.go
│   │   ├── browse.go
│   │   ├── refresh.go
│   │   ├── config.go
│   │   ├── mv.go
│   │   └── cp.go
│   ├── config/
│   │   └── config.go
│   ├── ui/
│   │   ├── browse.go
│   │   ├── describe.go
│   │   └── tree.go
│   └── util/
│       ├── clipboard.go
│       └── editor.go
├── tasks/
├── go.mod
├── go.sum
└── idea.md
```

### 4. Create Main Entry Point

Create file `cmd/clerk/main.go`:

```go
package main

import (
	"os"

	"github.com/yachiko/clerk/internal/cli"
)

// Version is set at build time via ldflags
var Version = "dev"

func main() {
	if err := cli.Execute(Version); err != nil {
		os.Exit(1)
	}
}
```

### 5. Create Root Command

Create file `internal/cli/root.go`:

```go
package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	profile string
	region  string
	version string
)

func Execute(v string) error {
	version = v
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
		fmt.Printf("clerk version %s\n", version)
	},
}
```

### 6. Create Exit Codes Constants

Create file `internal/cli/exitcodes.go`:

```go
package cli

const (
	ExitSuccess      = 0
	ExitGeneralError = 1
	ExitAWSError     = 2
	ExitInvalidInput = 3
)
```

## Acceptance Criteria

- [ ] `go mod tidy` runs without errors
- [ ] `go build ./cmd/clerk` produces a binary
- [ ] `./clerk --help` displays help text
- [ ] `./clerk version` displays version information
- [ ] All directories exist as specified
- [ ] No compilation errors

## Notes

- Use `internal/` directory to prevent external imports
- Keep `main.go` minimal - all logic in `internal/` packages
- Version will be injected at build time using `-ldflags`
