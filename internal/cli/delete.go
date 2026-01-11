package cli

import (
	"bufio"
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

var deleteForce bool

// InitDeleteCommand initializes the DELETE command
func InitDeleteCommand() *cobra.Command {
	deleteCmd := &cobra.Command{
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

	deleteCmd.Flags().BoolVar(&deleteForce, "force", false, "Skip confirmation prompt")

	return deleteCmd
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
	region := globalOpts.Region
	if region == "" && cfg.Region != "" {
		region = cfg.Region
	}
	if region == "" {
		region = "us-east-1"
	}

	profile := globalOpts.Profile
	if profile == "" && cfg.Profile != "" {
		profile = cfg.Profile
	}

	awsOpts := aws.ClientOptions{
		Region:           region,
		Profile:          profile,
		DescribePageSize: cfg.DescribePageSize,
		DescribeMaxItems: cfg.DescribeMaxItems,
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

	// Remove from cache with region and account ID
	cacheMgr, err := cache.NewManager(cfg, client.GetRegion(), client.GetAccountID())
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

	confirmWord := "delete me"
	fmt.Printf("Type '%s' to confirm deletion: ", confirmWord)

	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		return false, err
	}

	input = strings.TrimSpace(input)
	return input == confirmWord, nil
}

// getConfirmationWord returns the word user must type to confirm
func getConfirmationWord(name string) string {
	parts := strings.Split(name, "/")
	word := parts[len(parts)-1]
	if len(word) > 10 {
		word = word[:10]
	}
	return word
}

// outputDeleteResult outputs the deletion result
func outputDeleteResult(name string, param *aws.Parameter) error {
	if globalOpts.Output == "json" {
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
