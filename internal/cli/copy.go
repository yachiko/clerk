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

// InitCopyCommand initializes the COPY command
func InitCopyCommand() *cobra.Command {
	copyCmd := &cobra.Command{
		Use:   "cp <source> <destination>",
		Short: "Copy a secret in AWS Parameter Store",
		Long: `Copy a secret from source to destination in AWS Parameter Store.

The destination parameter inherits the type from source.
Tags are NOT copied (AWS parameter tag limitations).

If destination already exists, it will be overwritten.

Examples:
  # Copy a secret
  clerk cp "/dev/database-password" "/dev/database-password-backup"

  # Copy with confirmation
  clerk cp "/prod/api-key" "/prod/api-key-backup"

  # Copy as JSON output
  clerk cp "/dev/secret" "/dev/secret-copy" --output json`,
		Args: cobra.ExactArgs(2),
		RunE: runCopy,
	}

	return copyCmd
}

func runCopy(cmd *cobra.Command, args []string) error {
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
		Region:  region,
		Profile: profile,
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

	// Put destination parameter with same type and value
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

	// Update cache
	cacheMgr, err := cache.NewManager(cfg)
	if err != nil {
		return fmt.Errorf("failed to initialize cache: %w", err)
	}

	// Refresh cache entry for destination
	destParamRetrieved, err := client.GetParameter(ctx, destination, false)
	if err == nil {
		cacheMgr.Update(cache.CacheEntry{
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
			"message":     "Parameter copied successfully",
		}
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(result)
	}

	fmt.Printf("✓ Parameter copied\n")
	fmt.Printf("  Source:      %s\n", source)
	fmt.Printf("  Destination: %s\n", destination)
	fmt.Printf("  Type:        %s\n", sourceParam.Type)

	return nil
}
