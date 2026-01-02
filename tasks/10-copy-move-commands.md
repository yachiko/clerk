# Task 10: COPY and MOVE Commands

## Objective
Implement the `clerk cp` and `clerk mv` commands for copying and moving secrets within AWS Parameter Store.

## Prerequisites
- Task 01 completed (project setup)
- Task 02 completed (configuration module)
- Task 03 completed (AWS SSM client)
- Task 04 completed (cache module)

## Command Specification

### Copy Command
```
clerk cp <source> <destination> [flags]

Arguments:
  source       Source parameter name
  destination  Destination parameter name

Flags:
  --output string  Output format: plain, json (default "plain")
  -h, --help       Help for cp

Global Flags:
  --profile string  AWS profile to use
  --region string   AWS region to use
```

### Move Command
```
clerk mv <source> <destination> [flags]

Arguments:
  source       Source parameter name
  destination  Destination parameter name

Flags:
  --force          Skip confirmation prompt
  --output string  Output format: plain, json (default "plain")
  -h, --help       Help for mv

Global Flags:
  --profile string  AWS profile to use
  --region string   AWS region to use
```

## Deliverables

### 1. Create COPY Command

Create file `internal/cli/cp.go`:

```go
package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"github.com/yachiko/clerk/internal/aws"
	"github.com/yachiko/clerk/internal/cache"
	"github.com/yachiko/clerk/internal/config"
)

var cpOutput string

func init() {
	rootCmd.AddCommand(cpCmd)

	cpCmd.Flags().StringVarP(&cpOutput, "output", "o", "plain", "Output format: plain, json")
}

var cpCmd = &cobra.Command{
	Use:   "cp <source> <destination>",
	Short: "Copy a secret to a new location",
	Long: `Copy a secret from one path to another in AWS Parameter Store.

The copied parameter will have the same value and type as the source.
Tags are NOT copied (AWS limitation for PutParameter).

Examples:
  # Copy a secret to a new path
  clerk cp "/dev/db_password" "/staging/db_password"

  # Copy with JSON output
  clerk cp "/dev/api_key" "/prod/api_key" --output json`,
	Args: cobra.ExactArgs(2),
	RunE: runCp,
}

func runCp(cmd *cobra.Command, args []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	source := args[0]
	destination := args[1]

	// Validate paths
	if !strings.HasPrefix(source, "/") {
		return fmt.Errorf("source parameter name must start with /")
	}
	if !strings.HasPrefix(destination, "/") {
		return fmt.Errorf("destination parameter name must start with /")
	}
	if source == destination {
		return fmt.Errorf("source and destination cannot be the same")
	}

	// Load config
	cfgMgr, err := config.NewManager()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	cfg := cfgMgr.Get()

	// Create AWS client
	awsOpts := aws.ClientOptions{
		Region:  region,
		Profile: profile,
	}
	if awsOpts.Region == "" {
		awsOpts.Region = cfg.Region
	}
	if awsOpts.Profile == "" {
		awsOpts.Profile = cfg.Profile
	}

	client, err := aws.NewClient(ctx, awsOpts)
	if err != nil {
		return fmt.Errorf("failed to create AWS client: %w", err)
	}

	// Get source parameter (with decryption)
	sourceParam, err := client.GetParameter(ctx, source, true)
	if err != nil {
		if aws.IsParameterNotFoundError(err) {
			return fmt.Errorf("source parameter not found: %s", source)
		}
		return fmt.Errorf("failed to get source parameter: %w", err)
	}

	// Check if destination exists
	_, err = client.GetParameter(ctx, destination, false)
	destExists := err == nil

	// Create destination parameter
	input := &aws.PutParameterInput{
		Name:      destination,
		Value:     sourceParam.Value,
		Type:      sourceParam.Type,
		Overwrite: destExists,
	}

	output, err := client.PutParameter(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to create destination parameter: %w", err)
	}

	// Update cache
	cacheMgr, err := cache.NewManager(cfg)
	if err == nil {
		cacheEntry := cache.CacheEntry{
			Name:             destination,
			Type:             sourceParam.Type,
			Version:          output.Version,
			LastModifiedDate: time.Now(),
		}
		_ = cacheMgr.Update(cacheEntry)
	}

	// Output result
	return outputCpResult(source, destination, output.Version, destExists)
}

func outputCpResult(source, destination string, version int64, wasUpdate bool) error {
	if cpOutput == "json" {
		result := struct {
			Source      string `json:"source"`
			Destination string `json:"destination"`
			Version     int64  `json:"version"`
			Action      string `json:"action"`
		}{
			Source:      source,
			Destination: destination,
			Version:     version,
			Action:      "created",
		}
		if wasUpdate {
			result.Action = "updated"
		}

		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(result)
	}

	// Plain output
	action := "Created"
	if wasUpdate {
		action = "Updated"
	}
	color.Green("✓ Copied %s → %s", source, destination)
	color.Cyan("  %s: %s (version %d)", action, destination, version)

	return nil
}
```

### 2. Create MOVE Command

Create file `internal/cli/mv.go`:

```go
package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
	"github.com/yachiko/clerk/internal/aws"
	"github.com/yachiko/clerk/internal/cache"
	"github.com/yachiko/clerk/internal/config"
)

var (
	mvForce  bool
	mvOutput string
)

func init() {
	rootCmd.AddCommand(mvCmd)

	mvCmd.Flags().BoolVar(&mvForce, "force", false, "Skip confirmation prompt")
	mvCmd.Flags().StringVarP(&mvOutput, "output", "o", "plain", "Output format: plain, json")
}

var mvCmd = &cobra.Command{
	Use:   "mv <source> <destination>",
	Short: "Move/rename a secret to a new location",
	Long: `Move a secret from one path to another in AWS Parameter Store.

This operation copies the parameter to the new location and then deletes
the source parameter. Use with caution as the source will be permanently
deleted.

Tags are NOT copied (AWS limitation for PutParameter).

Examples:
  # Move/rename a secret
  clerk mv "/dev/old_name" "/dev/new_name"

  # Move without confirmation
  clerk mv "/dev/secret" "/prod/secret" --force`,
	Args: cobra.ExactArgs(2),
	RunE: runMv,
}

func runMv(cmd *cobra.Command, args []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	source := args[0]
	destination := args[1]

	// Validate paths
	if !strings.HasPrefix(source, "/") {
		return fmt.Errorf("source parameter name must start with /")
	}
	if !strings.HasPrefix(destination, "/") {
		return fmt.Errorf("destination parameter name must start with /")
	}
	if source == destination {
		return fmt.Errorf("source and destination cannot be the same")
	}

	// Load config
	cfgMgr, err := config.NewManager()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	cfg := cfgMgr.Get()

	// Create AWS client
	awsOpts := aws.ClientOptions{
		Region:  region,
		Profile: profile,
	}
	if awsOpts.Region == "" {
		awsOpts.Region = cfg.Region
	}
	if awsOpts.Profile == "" {
		awsOpts.Profile = cfg.Profile
	}

	client, err := aws.NewClient(ctx, awsOpts)
	if err != nil {
		return fmt.Errorf("failed to create AWS client: %w", err)
	}

	// Get source parameter (with decryption)
	sourceParam, err := client.GetParameter(ctx, source, true)
	if err != nil {
		if aws.IsParameterNotFoundError(err) {
			return fmt.Errorf("source parameter not found: %s", source)
		}
		return fmt.Errorf("failed to get source parameter: %w", err)
	}

	// Confirm move unless --force is used
	if !mvForce {
		confirmed, err := confirmMove(source, destination, sourceParam)
		if err != nil {
			return err
		}
		if !confirmed {
			color.Yellow("Move cancelled")
			return nil
		}
	}

	// Check if destination exists
	_, err = client.GetParameter(ctx, destination, false)
	destExists := err == nil

	// Create destination parameter
	input := &aws.PutParameterInput{
		Name:      destination,
		Value:     sourceParam.Value,
		Type:      sourceParam.Type,
		Overwrite: destExists,
	}

	output, err := client.PutParameter(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to create destination parameter: %w", err)
	}

	// Delete source parameter
	if err := client.DeleteParameter(ctx, source); err != nil {
		// Rollback: delete destination if source deletion failed
		color.Red("Warning: Failed to delete source, attempting rollback...")
		_ = client.DeleteParameter(ctx, destination)
		return fmt.Errorf("failed to delete source parameter: %w", err)
	}

	// Update cache
	cacheMgr, err := cache.NewManager(cfg)
	if err == nil {
		// Add new entry
		cacheEntry := cache.CacheEntry{
			Name:             destination,
			Type:             sourceParam.Type,
			Version:          output.Version,
			LastModifiedDate: time.Now(),
		}
		_ = cacheMgr.Update(cacheEntry)
		// Remove old entry
		_ = cacheMgr.Delete(source)
	}

	// Output result
	return outputMvResult(source, destination, output.Version, destExists)
}

func confirmMove(source, destination string, param *aws.Parameter) (bool, error) {
	color.Yellow("\nYou are about to move/rename a parameter:\n")
	fmt.Printf("  From: %s\n", source)
	fmt.Printf("  To:   %s\n", destination)
	fmt.Printf("  Type: %s\n", param.Type)
	fmt.Printf("  Version: %d\n\n", param.Version)
	color.Red("WARNING: The source parameter will be permanently deleted!\n\n")

	prompt := promptui.Prompt{
		Label:     "Continue with move",
		IsConfirm: true,
	}

	_, err := prompt.Run()
	if err != nil {
		if err == promptui.ErrAbort {
			return false, nil
		}
		return false, err
	}

	return true, nil
}

func outputMvResult(source, destination string, version int64, wasUpdate bool) error {
	if mvOutput == "json" {
		result := struct {
			Source      string `json:"source"`
			Destination string `json:"destination"`
			Version     int64  `json:"version"`
			Action      string `json:"action"`
		}{
			Source:      source,
			Destination: destination,
			Version:     version,
			Action:      "moved",
		}

		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(result)
	}

	// Plain output
	color.Green("✓ Moved %s → %s", source, destination)
	color.Cyan("  New location: %s (version %d)", destination, version)
	color.Yellow("  Source deleted: %s", source)

	return nil
}
```

## Acceptance Criteria

### Copy Command
- [ ] `clerk cp "/src" "/dest"` copies parameter to new location
- [ ] Source parameter remains intact after copy
- [ ] Destination version is 1 for new parameters
- [ ] Destination version increments for existing parameters
- [ ] SecureString values are properly copied (decrypted then re-encrypted)
- [ ] `--output json` outputs JSON format
- [ ] Error when source doesn't exist
- [ ] Error when source and destination are the same

### Move Command
- [ ] `clerk mv "/src" "/dest"` moves parameter to new location
- [ ] Source parameter is deleted after successful move
- [ ] Confirmation prompt by default
- [ ] `--force` skips confirmation
- [ ] Rollback on failure (delete destination if source deletion fails)
- [ ] `--output json` outputs JSON format
- [ ] Cache is updated (new entry added, old entry removed)
- [ ] Error when source doesn't exist

## Example Output

### Copy - Plain Output
```
✓ Copied /dev/db_password → /staging/db_password
  Created: /staging/db_password (version 1)
```

### Copy - JSON Output
```json
{
  "source": "/dev/db_password",
  "destination": "/staging/db_password",
  "version": 1,
  "action": "created"
}
```

### Move - Confirmation Prompt
```
You are about to move/rename a parameter:

  From: /dev/old_name
  To:   /dev/new_name
  Type: SecureString
  Version: 3

WARNING: The source parameter will be permanently deleted!

Continue with move (y/N): 
```

### Move - Plain Output
```
✓ Moved /dev/old_name → /dev/new_name
  New location: /dev/new_name (version 1)
  Source deleted: /dev/old_name
```

## Notes

- AWS Parameter Store doesn't have native move/rename - we implement it as copy+delete
- Tags are NOT copied due to AWS API limitations (tags can only be set on create)
- Move includes rollback logic: if source deletion fails, destination is deleted
- SecureString values must be decrypted to copy (requires KMS decrypt permission)
- Move is inherently risky - confirmation is required by default
- Consider adding `--copy-tags` flag in future to manually copy tags via separate API calls
