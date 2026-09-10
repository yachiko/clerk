package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"
	"github.com/yachiko/clerk/internal/aws"
	"github.com/yachiko/clerk/internal/cache"
	"github.com/yachiko/clerk/internal/config"
)

var deleteForce bool
var deleteRecoveryWindow int64
var deleteForceWithoutRecovery bool

// InitDeleteCommand initializes the DELETE command
func InitDeleteCommand() *cobra.Command {
	deleteCmd := &cobra.Command{
		Use:   "delete <name>",
		Short: "Delete a secret from the selected backend",
		Long: `Delete a secret from Parameter Store or Secrets Manager.

Parameter Store is used by default. Secrets Manager deletion must use either a
7-30 day --recovery-window or --force-delete-without-recovery.

By default, you will be prompted for confirmation before deletion.
Use --force to skip only the confirmation prompt; it does not select permanent
deletion.

SSM deletion and --force-delete-without-recovery are irreversible. Scheduled
Secrets Manager deletion can be cancelled with clerk restore.

Examples:
  # Delete with confirmation prompt
  clerk delete "/dev/old_secret" --backend ssm

  # Delete without confirmation
  clerk delete "/dev/old_secret" --force --backend ssm

  # Delete and output JSON
  clerk delete "/dev/old_secret" --force --output json --backend ssm

  # Schedule Secrets Manager deletion with a 14-day recovery window
  clerk delete "prod/old-secret" --backend secretsmanager --recovery-window 14 --force`,
		Args:    cobra.ExactArgs(1),
		PreRunE: validateDeleteFlags,
		RunE:    runDelete,
	}

	deleteCmd.Flags().BoolVar(&deleteForce, "force", false, "Skip confirmation prompt")
	deleteCmd.Flags().Int64Var(&deleteRecoveryWindow, "recovery-window", 0, "Secrets Manager recovery window in days (7-30)")
	deleteCmd.Flags().BoolVar(&deleteForceWithoutRecovery, "force-delete-without-recovery", false, "Permanently delete a Secrets Manager secret")

	return deleteCmd
}

func runDelete(cmd *cobra.Command, args []string) error {
	backend, err := selectedBackend(cmd)
	if err != nil {
		return err
	}
	if backend == aws.BackendSecretsManager {
		return runSecretsManagerDelete(cmd, args[0])
	}
	return runSSMDelete(cmd, args[0])
}

func runSSMDelete(cmd *cobra.Command, name string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := validateParameterIdentifier(name, false); err != nil {
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
	cacheMgr, err := cache.NewManagerForBackend(cfg, client.GetPartition(), client.GetRegion(), client.GetAccountID(), aws.BackendSSM)
	if err == nil {
		_ = cacheMgr.Delete(name)
	}

	// Output result
	return outputDeleteResult(name, param)
}

func validateDeleteFlags(cmd *cobra.Command, _ []string) error {
	backend, err := selectedBackend(cmd)
	if err != nil {
		return err
	}
	if backend == aws.BackendSSM {
		if deleteRecoveryWindow != 0 || deleteForceWithoutRecovery {
			return fmt.Errorf("--recovery-window and --force-delete-without-recovery are supported only with --backend secretsmanager")
		}
		return nil
	}
	if deleteRecoveryWindow != 0 && deleteForceWithoutRecovery {
		return fmt.Errorf("--recovery-window and --force-delete-without-recovery are mutually exclusive")
	}
	if deleteRecoveryWindow != 0 && (deleteRecoveryWindow < 7 || deleteRecoveryWindow > 30) {
		return fmt.Errorf("--recovery-window must be between 7 and 30 days")
	}
	if deleteRecoveryWindow == 0 && !deleteForceWithoutRecovery && !stdinIsTerminal() {
		return fmt.Errorf("Secrets Manager deletion requires --recovery-window 7..30 or --force-delete-without-recovery in noninteractive use")
	}
	return nil
}

func stdinIsTerminal() bool {
	fd := os.Stdin.Fd()
	return isatty.IsTerminal(fd) || isatty.IsCygwinTerminal(fd)
}

func runSecretsManagerDelete(cmd *cobra.Command, name string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("secret ID cannot be empty")
	}
	reader := bufio.NewReader(os.Stdin)
	recoveryWindow, permanent := deleteRecoveryWindow, deleteForceWithoutRecovery
	if recoveryWindow == 0 && !permanent {
		fmt.Fprint(os.Stderr, "Recovery window in days [30]: ")
		line, err := reader.ReadString('\n')
		if err != nil && len(line) == 0 {
			return fmt.Errorf("read recovery window: %w", err)
		}
		line = strings.TrimSpace(line)
		if line == "" {
			recoveryWindow = 30
		} else {
			parsed, parseErr := strconv.ParseInt(line, 10, 64)
			if parseErr != nil || parsed < 7 || parsed > 30 {
				return fmt.Errorf("recovery window must be between 7 and 30 days")
			}
			recoveryWindow = parsed
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cfgMgr, err := config.NewManager()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	cfg := cfgMgr.Get()
	awsOpts, err := resolveAWSOptions(cmd, cfg)
	if err != nil {
		return err
	}
	resolved, err := aws.ResolveContext(ctx, awsOpts)
	if err != nil {
		return fmt.Errorf("failed to resolve AWS context: %w", err)
	}
	client, err := aws.NewSecretsManagerClient(resolved)
	if err != nil {
		return fmt.Errorf("failed to create Secrets Manager client: %w", err)
	}
	metadata, err := client.DescribeSecret(ctx, name)
	if err != nil {
		return err
	}
	if !deleteForce {
		mode := fmt.Sprintf("with a %d-day recovery window", recoveryWindow)
		if permanent {
			mode = "permanently without recovery"
		}
		fmt.Fprintf(os.Stderr, "Delete Secrets Manager secret %q %s? Type 'delete me' to confirm: ", metadata.Name, mode)
		line, readErr := reader.ReadString('\n')
		if readErr != nil && len(line) == 0 {
			return readErr
		}
		if strings.TrimSpace(line) != "delete me" {
			fmt.Fprintln(os.Stderr, "Deletion cancelled")
			return nil
		}
	}
	result, err := client.DeleteSecret(ctx, aws.DeleteSecretRequest{SecretID: name, RecoveryWindowDays: recoveryWindow, Permanent: permanent})
	if err != nil {
		return err
	}
	if permanent {
		manager, cacheErr := cache.NewManagerForBackend(cfg, resolved.Partition, resolved.Region, resolved.AccountID, aws.BackendSecretsManager)
		if cacheErr == nil {
			_ = manager.DeleteByIdentity(result.Identity)
		}
	} else {
		reconcileSecretCache(ctx, cfg, resolved, client, result.Name)
	}
	return outputSecretDeleteResult(os.Stdout, result, permanent)
}

func outputSecretDeleteResult(out io.Writer, result *aws.DeleteSecretResult, permanent bool) error {
	if globalOpts.Output == "json" {
		return json.NewEncoder(out).Encode(struct {
			Name         string     `json:"name"`
			Deleted      bool       `json:"deleted"`
			Permanent    bool       `json:"permanent"`
			DeletionDate *time.Time `json:"deletion_date,omitempty"`
		}{result.Name, true, permanent, result.DeletionDate})
	}
	if permanent {
		_, err := fmt.Fprintf(out, "Permanently deleted secret: %s\n", result.Name)
		return err
	}
	_, err := fmt.Fprintf(out, "Scheduled deletion for secret: %s (%s)\n", result.Name, result.DeletionDate.Format(time.RFC3339))
	return err
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
