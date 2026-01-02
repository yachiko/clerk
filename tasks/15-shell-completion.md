# Task 15: Shell Completion

## Objective
Implement shell completion support for bash, zsh, and fish shells.

## Prerequisites
- Task 01 completed (project setup)
- All commands implemented (Tasks 06-11)

## Command Specification

```
clerk completion <shell>

Arguments:
  shell    Shell type: bash, zsh, fish, powershell

Examples:
  # Generate bash completion
  clerk completion bash > /etc/bash_completion.d/clerk

  # Generate zsh completion  
  clerk completion zsh > "${fpath[1]}/_clerk"

  # Generate fish completion
  clerk completion fish > ~/.config/fish/completions/clerk.fish

  # Temporarily load in current shell
  source <(clerk completion bash)
```

## Deliverables

### 1. Create Completion Command

Create file `internal/cli/completion.go`:

```go
package cli

import (
	"os"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(completionCmd)
}

var completionCmd = &cobra.Command{
	Use:   "completion [bash|zsh|fish|powershell]",
	Short: "Generate shell completion scripts",
	Long: `Generate shell completion scripts for clerk.

To load completions:

Bash:
  # Linux
  $ clerk completion bash > /etc/bash_completion.d/clerk
  
  # macOS (requires bash-completion)
  $ clerk completion bash > $(brew --prefix)/etc/bash_completion.d/clerk

  # Load in current session
  $ source <(clerk completion bash)

Zsh:
  # If shell completion is not already enabled in your environment,
  # you will need to enable it. Execute the following once:
  $ echo "autoload -U compinit; compinit" >> ~/.zshrc

  # To load completions for each session, execute once:
  $ clerk completion zsh > "${fpath[1]}/_clerk"

  # You may need to start a new shell for this setup to take effect.

Fish:
  $ clerk completion fish > ~/.config/fish/completions/clerk.fish

PowerShell:
  PS> clerk completion powershell | Out-String | Invoke-Expression

  # To load completions for every new session, run:
  PS> clerk completion powershell > clerk.ps1
  # and source this file from your PowerShell profile.
`,
	DisableFlagsInUseLine: true,
	ValidArgs:             []string{"bash", "zsh", "fish", "powershell"},
	Args:                  cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
	RunE: func(cmd *cobra.Command, args []string) error {
		switch args[0] {
		case "bash":
			return rootCmd.GenBashCompletion(os.Stdout)
		case "zsh":
			return rootCmd.GenZshCompletion(os.Stdout)
		case "fish":
			return rootCmd.GenFishCompletion(os.Stdout, true)
		case "powershell":
			return rootCmd.GenPowerShellCompletionWithDesc(os.Stdout)
		}
		return nil
	},
}
```

### 2. Add Dynamic Completions for Parameter Names

Update relevant command files to add custom completions:

Add to `internal/cli/get.go`:

```go
func init() {
	rootCmd.AddCommand(getCmd)

	// ... existing flags ...

	// Add completion for parameter names
	getCmd.ValidArgsFunction = completeParameterNames
}

// completeParameterNames provides completion for parameter names
func completeParameterNames(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) >= 1 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	// Load config and cache
	cfgMgr, err := config.NewManager()
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	cfg := cfgMgr.Get()

	cacheMgr, err := cache.NewManager(cfg)
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}

	// Get all entries from cache
	entries := cacheMgr.GetAll()
	if len(entries) == 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	// Filter entries that match the prefix
	var completions []string
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name, toComplete) {
			completions = append(completions, entry.Name)
		}
	}

	return completions, cobra.ShellCompDirectiveNoFileComp
}
```

Add to `internal/cli/delete.go`:

```go
func init() {
	rootCmd.AddCommand(deleteCmd)

	// ... existing flags ...

	deleteCmd.ValidArgsFunction = completeParameterNames
}
```

Add to `internal/cli/cp.go` and `internal/cli/mv.go`:

```go
func init() {
	rootCmd.AddCommand(cpCmd) // or mvCmd

	// ... existing flags ...

	cpCmd.ValidArgsFunction = completeParameterNamesMultiple
}

// completeParameterNamesMultiple provides completion for multiple parameter arguments
func completeParameterNamesMultiple(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) >= 2 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	return completeParameterNames(cmd, nil, toComplete)
}
```

### 3. Add Completion for Config Keys

Add to `internal/cli/config.go`:

```go
func init() {
	// ... existing code ...

	configGetCmd.ValidArgsFunction = completeConfigKeys
	configSetCmd.ValidArgsFunction = completeConfigKeysAndValues
}

func completeConfigKeys(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) >= 1 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	keys := []string{
		"region\tAWS region",
		"profile\tAWS profile",
		"cache_path\tCache file location",
		"cache_ttl\tCache time-to-live",
		"clipboard_timeout\tClipboard clear timeout",
		"default_type\tDefault parameter type",
		"default_sort\tDefault sort order",
		"parallel_fetches\tParallel fetch count",
	}

	return keys, cobra.ShellCompDirectiveNoFileComp
}

func completeConfigKeysAndValues(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) == 0 {
		return completeConfigKeys(cmd, args, toComplete)
	}

	if len(args) >= 2 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	// Provide value suggestions based on key
	switch args[0] {
	case "default_type":
		return []string{"String", "StringList", "SecureString"}, cobra.ShellCompDirectiveNoFileComp
	case "default_sort":
		return []string{"name", "created", "modified"}, cobra.ShellCompDirectiveNoFileComp
	case "region":
		return []string{
			"us-east-1", "us-east-2", "us-west-1", "us-west-2",
			"eu-west-1", "eu-west-2", "eu-central-1",
			"ap-northeast-1", "ap-southeast-1", "ap-southeast-2",
		}, cobra.ShellCompDirectiveNoFileComp
	}

	return nil, cobra.ShellCompDirectiveNoFileComp
}
```

### 4. Add Completion for Sort Options

Add to `internal/cli/list.go`:

```go
func init() {
	rootCmd.AddCommand(listCmd)

	listCmd.Flags().StringVarP(&listSort, "sort", "s", "", "Sort by: name (n), created (c), modified (m)")
	listCmd.RegisterFlagCompletionFunc("sort", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{
			"name\tSort by name",
			"n\tSort by name (short)",
			"created\tSort by creation date",
			"c\tSort by creation date (short)",
			"modified\tSort by modification date",
			"m\tSort by modification date (short)",
		}, cobra.ShellCompDirectiveNoFileComp
	})

	// ... rest of init
}
```

### 5. Add Completion for Output Format

Create helper in `internal/cli/completion_helpers.go`:

```go
package cli

import "github.com/spf13/cobra"

// completeOutputFormat provides completion for --output flag
func completeOutputFormat(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return []string{
		"plain\tPlain text output",
		"json\tJSON output",
	}, cobra.ShellCompDirectiveNoFileComp
}

// Register output completion for commands that have it
func registerOutputCompletion(cmd *cobra.Command) {
	cmd.RegisterFlagCompletionFunc("output", completeOutputFormat)
}
```

Add to each command that has `--output`:

```go
func init() {
	// ... existing init code ...
	registerOutputCompletion(getCmd)
}
```

## Acceptance Criteria

- [ ] `clerk completion bash` outputs valid bash completion script
- [ ] `clerk completion zsh` outputs valid zsh completion script
- [ ] `clerk completion fish` outputs valid fish completion script
- [ ] `clerk completion powershell` outputs valid PowerShell completion script
- [ ] Tab completion works for command names
- [ ] Tab completion works for parameter names (from cache)
- [ ] Tab completion works for config keys
- [ ] Tab completion works for `--sort` values
- [ ] Tab completion works for `--output` values
- [ ] Tab completion works for `--type` values
- [ ] Descriptions shown for completion items (where supported)

## Testing Completions

### Bash
```bash
source <(clerk completion bash)
clerk get /dev/<TAB>  # Should show parameters starting with /dev/
clerk config set <TAB>  # Should show config keys
```

### Zsh
```zsh
source <(clerk completion zsh)
clerk get /dev/<TAB>
```

### Fish
```fish
clerk completion fish | source
clerk get /dev/<TAB>
```

## Notes

- Cobra provides built-in completion generation
- Custom `ValidArgsFunction` enables dynamic completions
- Completions read from cache (fast, no AWS calls during tab completion)
- Descriptions after `\t` are shown in supported shells
- `cobra.ShellCompDirectiveNoFileComp` prevents file path suggestions
- User must manually install the completion script to their shell config
- Consider caching completion results for very large parameter stores
