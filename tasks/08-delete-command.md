# Task 08: DELETE Command

## Objective
Implement the `clerk delete` command for removing secrets from AWS Parameter Store.

## Prerequisites
- Task 01 completed (project setup)
- Task 02 completed (configuration module)
- Task 03 completed (AWS SSM client)
- Task 04 completed (cache module)

## Command Specification

```
clerk delete <name> [flags]

Arguments:
  name     Parameter name to delete (must start with /)

Flags:
  --force         Skip confirmation prompt
  --output string Output format: plain, json (default "plain")
  -h, --help      Help for delete

Global Flags:
  --profile string  AWS profile to use
  --region string   AWS region to use
```

## Deliverables

### 1. Create DELETE Command

Create file `internal/cli/delete.go`:

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
	deleteForce  bool
	deleteOutput string
)

func init() {
	rootCmd.AddCommand(deleteCmd)

	deleteCmd.Flags().BoolVar(&deleteForce, "force", false, "Skip confirmation prompt")
	deleteCmd.Flags().StringVarP(&deleteOutput, "output", "o", "plain", "Output format: plain, json")
}

var deleteCmd = &cobra.Command{
	Use:   "delete <name>",
	Short: "Delete a secret from AWS Parameter Store",
	Long: `Delete a secret from AWS Parameter Store.

By default, you will be prompted for confirmation before deletion.
Use --force to skip the confirmation prompt.

WARNING: This action cannot be undone. The parameter and all its 
version history will be permanently deleted.

Examples:
  # Delete with confirmation prompt
  clerk delete "/dev/old_secret"

  # Delete without confirmation
  clerk delete "/dev/old_secret" --force

  # Delete and output JSON
  clerk delete "/dev/old_secret" --force --output json`,
	Args: cobra.ExactArgs(1),
	RunE: runDelete,
}

func runDelete(cmd *cobra.Command, args []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	name := args[0]

	// Validate name starts with /
	if !strings.HasPrefix(name, "/") {
		return fmt.Errorf("parameter name must start with /")
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

	// Check if parameter exists
	param, err := client.GetParameter(ctx, name, false)
	if err != nil {
		if aws.IsParameterNotFoundError(err) {
			return fmt.Errorf("parameter not found: %s", name)
		}
		return fmt.Errorf("failed to get parameter: %w", err)
	}

	// Confirm deletion unless --force is used
	if !deleteForce {
		confirmed, err := confirmDeletion(name, param)
		if err != nil {
			return err
		}
		if !confirmed {
			color.Yellow("Deletion cancelled")
			return nil
		}
	}

	// Delete parameter
	if err := client.DeleteParameter(ctx, name); err != nil {
		return fmt.Errorf("failed to delete parameter: %w", err)
	}

	// Remove from cache
	cacheMgr, err := cache.NewManager(cfg)
	if err == nil {
		_ = cacheMgr.Delete(name)
	}

	// Output result
	return outputDeleteResult(name, param)
}

// confirmDeletion prompts user to confirm deletion
func confirmDeletion(name string, param *aws.Parameter) (bool, error) {
	color.Yellow("\nWARNING: You are about to delete the following parameter:\n")
	fmt.Printf("  Name:    %s\n", name)
	fmt.Printf("  Type:    %s\n", param.Type)
	fmt.Printf("  Version: %d\n", param.Version)
	fmt.Printf("  Modified: %s\n\n", param.LastModifiedDate.Format(time.RFC3339))
	color.Red("This action cannot be undone!\n\n")

	prompt := promptui.Prompt{
		Label:     fmt.Sprintf("Type '%s' to confirm deletion", getConfirmationWord(name)),
		AllowEdit: false,
	}

	result, err := prompt.Run()
	if err != nil {
		if err == promptui.ErrInterrupt {
			return false, nil
		}
		return false, err
	}

	return result == getConfirmationWord(name), nil
}

// getConfirmationWord returns the word user must type to confirm
func getConfirmationWord(name string) string {
	// Use last segment of the path
	parts := strings.Split(name, "/")
	word := parts[len(parts)-1]
	if len(word) > 10 {
		word = word[:10]
	}
	return word
}

// outputDeleteResult outputs the deletion result
func outputDeleteResult(name string, param *aws.Parameter) error {
	if deleteOutput == "json" {
		output := struct {
			Name    string `json:"name"`
			Deleted bool   `json:"deleted"`
			Message string `json:"message"`
		}{
			Name:    name,
			Deleted: true,
			Message: fmt.Sprintf("Parameter '%s' has been deleted", name),
		}

		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(output)
	}

	// Plain output
	color.Green("✓ Successfully deleted parameter: %s", name)
	return nil
}
```

## Acceptance Criteria

- [ ] `clerk delete "/test/secret"` prompts for confirmation
- [ ] Confirmation requires typing the parameter name (last segment)
- [ ] Typing incorrect confirmation word cancels deletion
- [ ] Pressing Ctrl+C during confirmation cancels deletion
- [ ] `--force` flag skips confirmation prompt
- [ ] Parameter not found error when parameter doesn't exist
- [ ] Cache is updated after successful deletion
- [ ] `--output json` outputs JSON format
- [ ] Exit code 0 on success
- [ ] Exit code 0 on cancelled deletion
- [ ] Exit code 2 on AWS error
- [ ] Exit code 3 on invalid input

## Example Output

### Confirmation Prompt
```
WARNING: You are about to delete the following parameter:

  Name:    /dev/old_secret
  Type:    SecureString
  Version: 5
  Modified: 2026-01-02T10:30:00Z

This action cannot be undone!

Type 'old_secret' to confirm deletion: 
```

### Successful Deletion (Plain)
```
✓ Successfully deleted parameter: /dev/old_secret
```

### Cancelled Deletion
```
Deletion cancelled
```

### JSON Output
```json
{
  "name": "/dev/old_secret",
  "deleted": true,
  "message": "Parameter '/dev/old_secret' has been deleted"
}
```

## Notes

- Confirmation word is the last path segment (max 10 chars) to prevent copy-paste
- Parameter existence is verified before prompting for confirmation
- `promptui.ErrInterrupt` handles Ctrl+C gracefully
- Cache deletion is best-effort (failure doesn't fail the command)
- AWS Parameter Store deletion is permanent - no recycle bin
- Consider warning about orphaned references in other parameters
