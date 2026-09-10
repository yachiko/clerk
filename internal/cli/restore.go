package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/yachiko/clerk/internal/aws"
	"github.com/yachiko/clerk/internal/config"
)

func InitRestoreCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "restore <name-or-arn>",
		Short: "Restore a Secrets Manager secret scheduled for deletion",
		Long: `Cancel a scheduled Secrets Manager deletion.

This command is available only with --backend secretsmanager. It restores the
secret itself; it does not restore an older value version.`,
		Args: cobra.ExactArgs(1),
		PreRunE: func(cmd *cobra.Command, _ []string) error {
			backend, err := selectedBackend(cmd)
			if err != nil {
				return err
			}
			if backend != aws.BackendSecretsManager {
				return fmt.Errorf("restore is supported only with --backend secretsmanager")
			}
			return nil
		},
		RunE: runRestore,
	}
}

func runRestore(cmd *cobra.Command, args []string) error {
	if strings.TrimSpace(args[0]) == "" {
		return fmt.Errorf("secret ID cannot be empty")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cfgMgr, err := config.NewManager()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	cfg := cfgMgr.Get()
	options, err := resolveAWSOptions(cmd, cfg)
	if err != nil {
		return err
	}
	resolved, err := aws.ResolveContext(ctx, options)
	if err != nil {
		return fmt.Errorf("failed to resolve AWS context: %w", err)
	}
	client, err := aws.NewSecretsManagerClient(resolved)
	if err != nil {
		return fmt.Errorf("failed to create Secrets Manager client: %w", err)
	}
	result, err := client.RestoreSecret(ctx, aws.RestoreSecretRequest{SecretID: args[0]})
	if err != nil {
		return err
	}
	reconcileSecretCache(ctx, cfg, resolved, client, result.Name)
	return outputRestoreResult(os.Stdout, result)
}

func outputRestoreResult(out io.Writer, result *aws.RestoreSecretResult) error {
	if globalOpts.Output == "json" {
		return json.NewEncoder(out).Encode(struct {
			Name     string `json:"name"`
			ARN      string `json:"arn"`
			Restored bool   `json:"restored"`
		}{result.Name, result.ARN, true})
	}
	_, err := fmt.Fprintf(out, "Restored secret: %s\n", result.Name)
	return err
}
