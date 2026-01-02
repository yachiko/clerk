# Task 11: REFRESH Command

## Objective
Implement the `clerk refresh` command for manually refreshing the local cache of parameter metadata.

## Prerequisites
- Task 01 completed (project setup)
- Task 02 completed (configuration module)
- Task 03 completed (AWS SSM client)
- Task 04 completed (cache module)

## Command Specification

```
clerk refresh [flags]

Flags:
  --output string  Output format: plain, json (default "plain")
  -h, --help       Help for refresh

Global Flags:
  --profile string  AWS profile to use
  --region string   AWS region to use
```

## Deliverables

### 1. Create REFRESH Command

Create file `internal/cli/refresh.go`:

```go
package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/fatih/color"
	"github.com/schollz/progressbar/v3"
	"github.com/spf13/cobra"
	"github.com/yachiko/clerk/internal/aws"
	"github.com/yachiko/clerk/internal/cache"
	"github.com/yachiko/clerk/internal/config"
)

var refreshOutput string

func init() {
	rootCmd.AddCommand(refreshCmd)

	refreshCmd.Flags().StringVarP(&refreshOutput, "output", "o", "plain", "Output format: plain, json")
}

var refreshCmd = &cobra.Command{
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

	// Initialize cache manager
	cacheMgr, err := cache.NewManager(cfg)
	if err != nil {
		return fmt.Errorf("failed to initialize cache: %w", err)
	}

	// Get previous stats for comparison
	prevStats := cacheMgr.GetStats()

	startTime := time.Now()

	// Create progress bar (for plain output)
	var bar *progressbar.ProgressBar
	var progressCb cache.RefreshProgressCallback

	if refreshOutput == "plain" {
		// First, count total parameters
		color.Cyan("Counting parameters...")
		params, err := client.DescribeAllParameters(ctx)
		if err != nil {
			return fmt.Errorf("failed to count parameters: %w", err)
		}
		total := len(params)

		if total == 0 {
			color.Yellow("No parameters found in AWS Parameter Store")
			return nil
		}

		bar = progressbar.NewOptions(total,
			progressbar.OptionSetDescription("Fetching metadata"),
			progressbar.OptionSetWidth(40),
			progressbar.OptionShowCount(),
			progressbar.OptionShowIts(),
			progressbar.OptionSetItsString("params"),
			progressbar.OptionSetTheme(progressbar.Theme{
				Saucer:        "=",
				SaucerHead:    ">",
				SaucerPadding: " ",
				BarStart:      "[",
				BarEnd:        "]",
			}),
			progressbar.OptionClearOnFinish(),
		)

		progressCb = func(current, total int) {
			bar.Set(current)
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

	if bar != nil {
		bar.Finish()
	}

	duration := time.Since(startTime)

	// Get new stats
	newStats := cacheMgr.GetStats()

	// Output result
	return outputRefreshResult(prevStats, newStats, duration)
}

func outputRefreshResult(prev, current cache.CacheStats, duration time.Duration) error {
	if refreshOutput == "json" {
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
		color.Green("  Change: +%d new parameters", diff)
	} else if diff < 0 {
		color.Yellow("  Change: %d parameters removed", diff)
	} else if prev.TotalEntries > 0 {
		fmt.Println("  Change: no change")
	}

	fmt.Printf("  Region: %s\n", current.Region)
	fmt.Printf("  Duration: %.2fs\n", duration.Seconds())
	fmt.Printf("  Cache valid until: %s\n", current.LastRefresh.Add(3*time.Hour).Format(time.RFC3339))

	return nil
}
```

## Acceptance Criteria

- [ ] `clerk refresh` fetches all parameter metadata from AWS
- [ ] Progress bar shows during refresh (plain output)
- [ ] Tags are fetched in parallel using configured concurrency
- [ ] Secret values are NOT cached
- [ ] Cache file is updated with new data
- [ ] Previous cache stats are compared and diff shown
- [ ] Duration is displayed
- [ ] `--output json` outputs JSON format
- [ ] Graceful handling of Ctrl+C during refresh
- [ ] Exit code 0 on success
- [ ] Exit code 2 on AWS error
- [ ] Cache TTL expiry time is displayed

## Example Output

### Plain Output
```
Counting parameters...
Fetching metadata [========================================] 150/150 (25 params/s)

✓ Cache refreshed successfully

Statistics:
  Total parameters: 150
  Change: +5 new parameters
  Region: us-east-1
  Duration: 6.23s
  Cache valid until: 2026-01-02T13:30:00Z
```

### JSON Output
```json
{
  "total_parameters": 150,
  "previous_count": 145,
  "region": "us-east-1",
  "last_refresh": "2026-01-02T10:30:00Z",
  "duration_seconds": 6.23,
  "added": 5
}
```

### Empty Parameter Store
```
Counting parameters...
No parameters found in AWS Parameter Store
```

## Progress Bar Behavior

- Shows parameter count being fetched
- Updates in real-time as tags are fetched in parallel
- Clears on completion (replaced by summary)
- Includes rate indicator (params/s)

## Notes

- Timeout is 5 minutes for large parameter stores
- First call counts parameters, second call fetches metadata+tags
- Tags are fetched in parallel (default 10 concurrent workers)
- Cache includes: name, type, version, last modified date, tags
- Cache excludes: secret values (security)
- Progress callback is thread-safe (uses mutex in cache.Refresh)
- Context cancellation stops ongoing fetches gracefully
