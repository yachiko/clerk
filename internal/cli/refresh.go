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
		color.Cyan("Loading parameters...")
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
	return outputRefreshResult(prevStats, newStats, duration)
}

func refreshBackend(cmd *cobra.Command) (aws.Backend, error) {
	if !backendWasExplicit(cmd) {
		return aws.BackendSSM, nil
	}
	return requireConcreteBackend(cmd, "refresh", false)
}

func outputRefreshResult(prev, current cache.CacheStats, duration time.Duration) error {
	if globalOpts.Output == "json" {
		result := struct {
			TotalParameters int     `json:"total_parameters"`
			PreviousCount   int     `json:"previous_count"`
			Region          string  `json:"region"`
			LastRefresh     string  `json:"last_refresh"`
			DurationSeconds float64 `json:"duration_seconds"`
			Added           int     `json:"added,omitempty"`
			Removed         int     `json:"removed,omitempty"`
		}{
			TotalParameters: current.TotalEntries,
			PreviousCount:   prev.TotalEntries,
			Region:          current.Region,
			LastRefresh:     current.LastRefresh.Format(time.RFC3339),
			DurationSeconds: duration.Seconds(),
		}

		if current.TotalEntries > prev.TotalEntries {
			result.Added = current.TotalEntries - prev.TotalEntries
		} else if current.TotalEntries < prev.TotalEntries {
			result.Removed = prev.TotalEntries - current.TotalEntries
		}

		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(result)
	}

	// Plain output
	fmt.Println()
	color.Green("✓ Cache refreshed successfully")
	fmt.Println()

	color.Cyan("Statistics:")
	fmt.Printf("  Total parameters: %d\n", current.TotalEntries)

	// Show change
	diff := current.TotalEntries - prev.TotalEntries
	if diff > 0 {
		color.Green("  Change: +%d new parameters\n", diff)
	} else if diff < 0 {
		color.Yellow("  Change: %d parameters removed\n", -diff)
	} else if prev.TotalEntries > 0 {
		fmt.Println("  Change: no change")
	}

	fmt.Printf("  Region: %s\n", current.Region)
	fmt.Printf("  Duration: %.2fs\n", duration.Seconds())
	fmt.Printf("  Cache valid until: %s\n", current.LastRefresh.Add(3*time.Hour).Format(time.RFC3339))

	return nil
}
