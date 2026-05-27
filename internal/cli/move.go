package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/yachiko/clerk/internal/aws"
	"github.com/yachiko/clerk/internal/cache"
	"github.com/yachiko/clerk/internal/config"
)

var moveForce bool

// InitMoveCommand initializes the MOVE command
func InitMoveCommand() *cobra.Command {
	moveCmd := &cobra.Command{
		Use:   "mv <source> <destination>",
		Short: "Move a secret in AWS Parameter Store",
		Long: `Move (rename) a secret in AWS Parameter Store.

This is effectively a copy followed by a delete.
Tags are NOT copied (AWS parameter tag limitations).
If the delete fails, the source parameter is NOT deleted (rollback).

Requires confirmation unless --force is provided.

Examples:
  # Move a secret (with confirmation)
  clerk mv "/dev/database-password" "/dev/database-password-old"

  # Move without confirmation
  clerk mv "/dev/api-key" "/dev/api-key-v2" --force

  # Move as JSON output
  clerk mv "/dev/secret" "/dev/secret-renamed" --output json --force`,
		Args: cobra.ExactArgs(2),
		RunE: runMove,
	}

	moveCmd.Flags().BoolVar(&moveForce, "force", false, "Skip confirmation prompt")

	return moveCmd
}

func runMove(cmd *cobra.Command, args []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	source := args[0]
	destination := args[1]

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

	// Get source parameter
	sourceParam, err := client.GetParameter(ctx, source, false)
	if err != nil {
		if aws.IsParameterNotFoundError(err) {
			return fmt.Errorf("source parameter not found: %s", source)
		}
		return fmt.Errorf("failed to get source parameter: %w", err)
	}

	// Confirm if not force
	if !moveForce && globalOpts.Output != "json" {
		fmt.Printf("You are about to move parameter: %s\n", source)
		fmt.Printf("To destination: %s\n", destination)
		fmt.Print("Type 'move' to confirm: ")

		var confirmation string
		_, _ = fmt.Scanln(&confirmation)

		if confirmation != "move" {
			fmt.Println("Cancelled.")
			return nil
		}
	}

	// Copy parameter
	input := &aws.PutParameterInput{
		Name:      destination,
		Value:     sourceParam.Value,
		Type:      sourceParam.Type,
		Overwrite: true,
	}

	_, err = client.PutParameter(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to copy parameter: %w", err)
	}

	// Delete source parameter (if copy succeeded)
	err = client.DeleteParameter(ctx, source)
	if err != nil {
		// Rollback: try to delete the destination we just created
		deleteErr := client.DeleteParameter(ctx, destination)
		if deleteErr == nil {
			return fmt.Errorf("failed to delete source parameter (rolled back copy): %w", err)
		}
		// If rollback also failed, report both errors
		return fmt.Errorf("failed to delete source parameter: %w (rollback also failed: %v)", err, deleteErr)
	}

	// Update cache with region and account ID
	cacheMgr, err := cache.NewManager(cfg, client.GetRegion(), client.GetAccountID())
	if err != nil {
		return fmt.Errorf("failed to initialize cache: %w", err)
	}

	// Cache mutations are best-effort — the next refresh reconciles.
	_ = cacheMgr.Delete(source)
	destParamRetrieved, err := client.GetParameter(ctx, destination, false)
	if err == nil {
		_ = cacheMgr.Update(cache.CacheEntry{
			Name:             destParamRetrieved.Name,
			Type:             destParamRetrieved.Type,
			Version:          destParamRetrieved.Version,
			LastModifiedDate: destParamRetrieved.LastModifiedDate,
			Tags:             destParamRetrieved.Tags,
		})
	}

	// Output result
	if globalOpts.Output == "json" {
		result := map[string]interface{}{
			"source":      source,
			"destination": destination,
			"type":        sourceParam.Type,
			"message":     "Parameter moved successfully",
		}
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(result)
	}

	fmt.Printf("✓ Parameter moved\n")
	fmt.Printf("  From: %s\n", source)
	fmt.Printf("  To:   %s\n", destination)

	return nil
}
