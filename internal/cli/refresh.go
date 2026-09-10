package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"github.com/yachiko/clerk/internal/aws"
	"github.com/yachiko/clerk/internal/cache"
	"github.com/yachiko/clerk/internal/config"
)

// InitRefreshCommand initializes the REFRESH command
func InitRefreshCommand() *cobra.Command {
	refreshCmd := &cobra.Command{
		Use:   "refresh",
		Short: "Refresh one backend's local metadata cache",
		Long: `Refresh the local metadata cache for one backend.

The compatibility default is Parameter Store. Use --backend secretsmanager to
refresh Secrets Manager metadata. Secret values are never cached.

The refresh process uses parallel fetching to speed up the operation.

Examples:
  # Refresh cache
  clerk refresh

  # Refresh with specific profile
  clerk refresh --profile production

  # Refresh with JSON output
  clerk refresh --output json`,
		PreRunE: func(cmd *cobra.Command, _ []string) error {
			_, err := refreshBackend(cmd)
			return err
		},
		RunE: runRefresh,
	}

	return refreshCmd
}

func runRefresh(cmd *cobra.Command, args []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Load config
	cfgMgr, err := config.NewManager()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	cfg := cfgMgr.Get()
	backend, err := refreshBackend(cmd)
	if err != nil {
		return err
	}

	// Create AWS client
	awsOpts, err := resolveAWSOptions(cmd, cfg)
	if err != nil {
		return err
	}

	resolved, err := aws.ResolveContext(ctx, awsOpts)
	if err != nil {
		return fmt.Errorf("failed to resolve AWS context: %w", err)
	}

	cacheMgr, err := cache.NewManagerForBackend(cfg, resolved.Partition, resolved.Region, resolved.AccountID, backend)
	if err != nil {
		return fmt.Errorf("failed to initialize cache: %w", err)
	}

	// Get previous stats for comparison
	prevStats := cacheMgr.GetStats()

	startTime := time.Now()

	// Create progress callback (for plain output)
	var progressCb cache.RefreshProgressCallback

	if globalOpts.Output != "json" {
		resourceName := "parameters"
		if backend == aws.BackendSecretsManager {
			resourceName = "secrets"
		}
		color.Cyan("Loading %s...", resourceName)
		progressCb = func(current, total int) {
			fmt.Printf("\rLoaded: %d", current)
		}
	}

	if backend == aws.BackendSecretsManager {
		client, clientErr := aws.NewSecretsManagerClient(resolved)
		if clientErr != nil {
			return fmt.Errorf("failed to create Secrets Manager client: %w", clientErr)
		}
		secrets, listErr := client.ListSecrets(ctx)
		if listErr != nil {
			return fmt.Errorf("failed to refresh cache: %w", listErr)
		}
		entries := make([]cache.CacheEntry, 0, len(secrets))
		for _, secret := range secrets {
			entry := cache.CacheEntry{Identity: secret.Identity, Name: secret.Name, Tags: secret.Tags, TagsComplete: true}
			if secret.LastChangedDate != nil {
				entry.LastModifiedDate = *secret.LastChangedDate
			} else if secret.CreatedDate != nil {
				entry.LastModifiedDate = *secret.CreatedDate
			}
			entries = append(entries, entry)
		}
		err = cacheMgr.ReplaceSnapshot(entries)
	} else {
		client, clientErr := aws.NewClientFromContext(resolved, awsOpts)
		if clientErr != nil {
			return fmt.Errorf("failed to create SSM client: %w", clientErr)
		}
		err = cacheMgr.Refresh(ctx, client, resolved.Region, cfg.ParallelFetches, progressCb)
	}
	if err != nil {
		return fmt.Errorf("failed to refresh cache: %w", err)
	}

	if globalOpts.Output != "json" {
		fmt.Println() // New line after progress
	}

	duration := time.Since(startTime)

	// Get new stats
	newStats := cacheMgr.GetStats()

	// Output result
	return outputRefreshResult(prevStats, newStats, duration, backend)
}

func refreshBackend(cmd *cobra.Command) (aws.Backend, error) {
	return selectedBackend(cmd)
}

func outputRefreshResult(prev, current cache.CacheStats, duration time.Duration, backend aws.Backend) error {
	if globalOpts.Output == "json" {
		result := map[string]any{
			"previous_count":   prev.TotalEntries,
			"region":           current.Region,
			"last_refresh":     current.LastRefresh.Format(time.RFC3339),
			"duration_seconds": duration.Seconds(),
		}
		if backend == aws.BackendSecretsManager {
			result["backend"] = backend
			result["total_secrets"] = current.TotalEntries
		} else {
			result["total_parameters"] = current.TotalEntries
		}

		if current.TotalEntries > prev.TotalEntries {
			result["added"] = current.TotalEntries - prev.TotalEntries
		} else if current.TotalEntries < prev.TotalEntries {
			result["removed"] = prev.TotalEntries - current.TotalEntries
		}

		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(result)
	}

	// Plain output
	fmt.Println()
	color.Green("✓ Cache refreshed successfully")
	fmt.Println()

	resourceName := "parameters"
	if backend == aws.BackendSecretsManager {
		resourceName = "secrets"
	}
	color.Cyan("Statistics:")
	fmt.Printf("  Total %s: %d\n", resourceName, current.TotalEntries)

	// Show change
	diff := current.TotalEntries - prev.TotalEntries
	if diff > 0 {
		color.Green("  Change: +%d new %s\n", diff, resourceName)
	} else if diff < 0 {
		color.Yellow("  Change: %d %s removed\n", -diff, resourceName)
	} else if prev.TotalEntries > 0 {
		fmt.Println("  Change: no change")
	}

	fmt.Printf("  Region: %s\n", current.Region)
	fmt.Printf("  Duration: %.2fs\n", duration.Seconds())
	fmt.Printf("  Cache valid until: %s\n", current.LastRefresh.Add(3*time.Hour).Format(time.RFC3339))

	return nil
}
