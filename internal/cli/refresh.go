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
		Short: "Refresh the local cache of parameter metadata",
		Long: `Refresh the local cache by fetching all parameter metadata from AWS Parameter Store.

This command fetches parameter names, types, versions, modification dates, and tags.
Secret values are NOT cached for security reasons.

The refresh process uses parallel fetching to speed up the operation.

Examples:
  # Refresh cache
  clerk refresh

  # Refresh with specific profile
  clerk refresh --profile production

  # Refresh with JSON output
  clerk refresh --output json`,
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

	// Initialize cache manager
	cacheMgr, err := cache.NewManager(cfg)
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

	// Perform refresh
	effectiveRegion := awsOpts.Region
	if effectiveRegion == "" {
		effectiveRegion = cfg.Region
	}

	err = cacheMgr.Refresh(ctx, client, effectiveRegion, cfg.ParallelFetches, progressCb)
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
