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
var moveOverwrite bool

// InitMoveCommand initializes the MOVE command
func InitMoveCommand() *cobra.Command {
	moveCmd := &cobra.Command{
		Use:   "mv <source> <destination>",
		Short: "Move a secret in AWS Parameter Store",
		Long: `Move (rename) a secret in AWS Parameter Store.

Parameter Store is used by default. Secrets Manager moves are not supported.

This is a verified copy followed by a source deletion. It preserves supported
metadata and leaves both parameters in place if source deletion fails.

Requires confirmation unless --force is provided.

Examples:
  # Move a secret (with confirmation)
  clerk mv "/dev/database-password" "/dev/database-password-old" --backend ssm

  # Move without confirmation
  clerk mv "/dev/api-key" "/dev/api-key-v2" --force --backend ssm

  # Move as JSON output
  clerk mv "/dev/secret" "/dev/secret-renamed" --output json --force --backend ssm`,
		Args:    cobra.ExactArgs(2),
		PreRunE: func(cmd *cobra.Command, _ []string) error { return requireSSMBackend(cmd) },
		RunE:    runMove,
	}

	moveCmd.Flags().BoolVar(&moveForce, "force", false, "Skip confirmation prompt")
	moveCmd.Flags().BoolVar(&moveOverwrite, "overwrite", false, "Replace an existing destination")

	return moveCmd
}

func runMove(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	source := args[0]
	destination := args[1]
	if err := validateParameterIdentifier(source, false); err != nil {
		return err
	}
	if err := validateParameterIdentifier(destination, false); err != nil {
		return err
	}

	// Load config
	cfgMgr, err := config.NewManager()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	cfg := cfgMgr.Get()

	// Create AWS client
	awsOpts, err := resolveAWSOptions(cmd, cfg)
	if err != nil {
		return err
	}

	client, err := aws.NewClient(ctx, awsOpts)
	if err != nil {
		return fmt.Errorf("failed to create AWS client: %w", err)
	}

	// Confirm if not force
	if !moveForce {
		if globalOpts.Output == "json" {
			_ = json.NewEncoder(os.Stdout).Encode(map[string]string{"error": "--force is required for a move with JSON output"})
			return fmt.Errorf("--force is required for a move with JSON output")
		}
		fmt.Fprintf(os.Stderr, "You are about to move parameter: %s\n", source)
		fmt.Fprintf(os.Stderr, "To destination: %s\n", destination)
		fmt.Fprintf(os.Stderr, "Account: %s  Region: %s\n", client.GetAccountID(), client.GetRegion())
		fmt.Fprint(os.Stderr, "Type 'move' to confirm: ")

		var confirmation string
		if _, err := fmt.Fscanln(os.Stdin, &confirmation); err != nil {
			return fmt.Errorf("move not confirmed: %w", err)
		}

		if confirmation != "move" {
			fmt.Fprintln(os.Stderr, "Cancelled.")
			return nil
		}
	}
	// The destructive-operation deadline starts after the human decision, so a
	// slow prompt cannot consume the time budget for the remote mutation.
	transferCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	transfer, err := client.Transfer(transferCtx, aws.TransferInput{Source: source, Destination: destination, Move: true, Overwrite: moveOverwrite})
	if err != nil {
		return fmt.Errorf("move incomplete: %w", err)
	}

	// Update cache with region and account ID
	cacheMgr, err := cache.NewManagerForBackend(cfg, client.GetPartition(), client.GetRegion(), client.GetAccountID(), aws.BackendSSM)

	// Cache mutations are best-effort — the next refresh reconciles.
	if err == nil {
		_ = cacheMgr.Delete(source)
		destParamRetrieved, getErr := client.GetParameter(ctx, destination, false)
		if getErr == nil {
			_ = cacheMgr.Update(cache.CacheEntry{
				Name:             destParamRetrieved.Name,
				Type:             destParamRetrieved.Type,
				Version:          destParamRetrieved.Version,
				LastModifiedDate: destParamRetrieved.LastModifiedDate,
				Tags:             destParamRetrieved.Tags,
			})
		}
	}

	// Output result
	if globalOpts.Output == "json" {
		result := map[string]interface{}{
			"source":      source,
			"destination": destination,
			"type":        transfer.Source.Type,
			"account_id":  client.GetAccountID(),
			"region":      client.GetRegion(),
			"message":     "Parameter moved successfully",
		}
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(result)
	}

	fmt.Printf("✓ Parameter moved\n")
	fmt.Printf("  From: %s\n", source)
	fmt.Printf("  To:   %s\n", destination)
	fmt.Printf("  Account: %s\n", client.GetAccountID())
	fmt.Printf("  Region:  %s\n", client.GetRegion())

	return nil
}
