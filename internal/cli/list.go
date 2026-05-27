package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"github.com/yachiko/clerk/internal/aws"
	"github.com/yachiko/clerk/internal/cache"
	"github.com/yachiko/clerk/internal/config"
)

var (
	listSort     string
	listShowTags bool
)

// InitListCommand initializes the LIST command
func InitListCommand() *cobra.Command {
	listCmd := &cobra.Command{
		Use:   "list [path]",
		Short: "List secrets in AWS Parameter Store",
		Long: `List secrets stored in AWS Parameter Store.

If no path is provided, all parameters are listed.
Supports glob patterns for filtering.

Examples:
  # List all parameters
  clerk list

  # List all parameters under /dev/
  clerk list "/dev/*"

  # List and sort by modification date
  clerk list --sort modified

  # List with tags
  clerk list "/dev/*" --tags

  # List as JSON
  clerk list --output json`,
		Args: cobra.MaximumNArgs(1),
		RunE: runList,
	}

	listCmd.Flags().StringVarP(&listSort, "sort", "s", "", "Sort by: name (n), created (c), modified (m)")
	listCmd.Flags().BoolVar(&listShowTags, "tags", false, "Show tags in output")

	return listCmd
}

func runList(cmd *cobra.Command, args []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	path := "/*"
	if len(args) > 0 {
		path = args[0]
	}

	// Load config
	cfgMgr, err := config.NewManager()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	cfg := cfgMgr.Get()

	// Determine sort order
	sortBy := listSort
	if sortBy == "" {
		sortBy = cfg.DefaultSort
	}
	sortBy = normalizeSortOption(sortBy)

	// Create AWS client first (needed for cache manager)
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

	// Initialize cache with region and account ID
	cacheMgr, err := cache.NewManager(cfg, client.GetRegion(), client.GetAccountID())
	if err != nil {
		return fmt.Errorf("failed to initialize cache: %w", err)
	}

	var entries []cache.CacheEntry

	// Check if cache is valid
	if !cacheMgr.IsExpired() {
		entries = cacheMgr.Search(path)
		if len(entries) > 0 {
			entries = cacheMgr.Sort(entries, sortBy)
			return outputList(entries, sortBy)
		}
	}

	// Cache miss or expired - fetch from AWS
	basePath := extractBasePath(path)
	params, err := client.ListParameters(ctx, basePath, true)
	if err != nil {
		return fmt.Errorf("failed to list parameters: %w", err)
	}

	// Convert to cache entries and filter
	entries = make([]cache.CacheEntry, 0, len(params))
	for _, p := range params {
		if matchPath(path, p.Name) {
			entry := cache.CacheEntry{
				Name:             p.Name,
				Type:             p.Type,
				Version:          p.Version,
				LastModifiedDate: p.LastModifiedDate,
				Tags:             p.Tags,
			}
			entries = append(entries, entry)
		}
	}

	// Sort
	entries = cacheMgr.Sort(entries, sortBy)

	return outputList(entries, sortBy)
}

// extractBasePath extracts the base path for API call
func extractBasePath(pattern string) string {
	if idx := strings.Index(pattern, "*"); idx > 0 {
		basePath := pattern[:idx]
		basePath = strings.TrimSuffix(basePath, "/")
		if basePath == "" {
			return "/"
		}
		return basePath
	}
	return pattern
}

// matchPath checks if a name matches the path pattern
func matchPath(pattern, name string) bool {
	if pattern == "" || pattern == "/*" || pattern == "/" {
		return true
	}

	if strings.HasSuffix(pattern, "/*") {
		prefix := strings.TrimSuffix(pattern, "/*")
		return strings.HasPrefix(name, prefix+"/") || name == prefix
	}

	if strings.HasPrefix(pattern, "*") && strings.HasSuffix(pattern, "*") {
		substr := strings.Trim(pattern, "*")
		return strings.Contains(name, substr)
	}

	if strings.HasSuffix(pattern, "*") {
		prefix := strings.TrimSuffix(pattern, "*")
		return strings.HasPrefix(name, prefix)
	}

	return pattern == name
}

// normalizeSortOption normalizes sort option aliases
func normalizeSortOption(sort string) string {
	switch strings.ToLower(sort) {
	case "n", "name":
		return "name"
	case "c", "created":
		return "created"
	case "m", "modified":
		return "modified"
	default:
		return "name"
	}
}

// outputList outputs the parameter list
func outputList(entries []cache.CacheEntry, sortBy string) error {
	if globalOpts.Output == "json" {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(entries)
	}

	// Plain output
	if len(entries) == 0 {
		color.Yellow("No parameters found")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "NAME\tTYPE\tVERSION\tMODIFIED")
	_, _ = fmt.Fprintln(w, strings.Repeat("-", 70))

	for _, entry := range entries {
		modified := entry.LastModifiedDate.Format("2006-01-02 15:04")
		_, _ = fmt.Fprintf(w, "%s\t%s\t%d\t%s\n", entry.Name, entry.Type, entry.Version, modified)
	}

	_ = w.Flush()

	fmt.Fprintf(os.Stderr, "\nTotal: %d parameters\n", len(entries))
	return nil
}
