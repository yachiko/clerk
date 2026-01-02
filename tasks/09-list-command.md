# Task 09: LIST Command

## Objective
Implement the `clerk list` command for listing secrets from AWS Parameter Store.

## Prerequisites
- Task 01 completed (project setup)
- Task 02 completed (configuration module)
- Task 03 completed (AWS SSM client)
- Task 04 completed (cache module)

## Command Specification

```
clerk list [path] [flags]

Arguments:
  path     Path pattern to list (default "/")
           Supports glob patterns: /dev/*, /*/db_password

Flags:
  --sort string    Sort by: name (n), created (c), modified (m) (default from config)
  --tags           Show tags in output
  --output string  Output format: plain, json (default "plain")
  -h, --help       Help for list

Global Flags:
  --profile string  AWS profile to use
  --region string   AWS region to use
```

## Deliverables

### 1. Create LIST Command

Create file `internal/cli/list.go`:

```go
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
	listOutput   string
)

func init() {
	rootCmd.AddCommand(listCmd)

	listCmd.Flags().StringVarP(&listSort, "sort", "s", "", "Sort by: name (n), created (c), modified (m)")
	listCmd.Flags().BoolVar(&listShowTags, "tags", false, "Show tags in output")
	listCmd.Flags().StringVarP(&listOutput, "output", "o", "plain", "Output format: plain, json")
}

var listCmd = &cobra.Command{
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

	// Try to use cache first
	cacheMgr, err := cache.NewManager(cfg)
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

	// Determine if we should use path-based or filter-based listing
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

	// Fetch tags if requested
	if listShowTags {
		entries = fetchTagsForEntries(ctx, client, entries)
	}

	// Sort
	entries = cacheMgr.Sort(entries, sortBy)

	return outputList(entries, sortBy)
}

// extractBasePath extracts the base path for API call
func extractBasePath(pattern string) string {
	// For /dev/* return /dev
	// For /* return /
	if idx := strings.Index(pattern, "*"); idx > 0 {
		basePath := pattern[:idx]
		// Remove trailing /
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

	// Handle /path/* pattern
	if strings.HasSuffix(pattern, "/*") {
		prefix := strings.TrimSuffix(pattern, "/*")
		return strings.HasPrefix(name, prefix+"/") || name == prefix
	}

	// Handle *pattern* (contains)
	if strings.HasPrefix(pattern, "*") && strings.HasSuffix(pattern, "*") {
		substr := strings.Trim(pattern, "*")
		return strings.Contains(name, substr)
	}

	// Handle pattern* (prefix)
	if strings.HasSuffix(pattern, "*") {
		prefix := strings.TrimSuffix(pattern, "*")
		return strings.HasPrefix(name, prefix)
	}

	// Exact match
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

// fetchTagsForEntries fetches tags for each entry
func fetchTagsForEntries(ctx context.Context, client *aws.Client, entries []cache.CacheEntry) []cache.CacheEntry {
	for i := range entries {
		tags, err := client.GetParameterTags(ctx, entries[i].Name)
		if err == nil {
			entries[i].Tags = tags
		}
	}
	return entries
}

// outputList outputs the list of entries
func outputList(entries []cache.CacheEntry, sortBy string) error {
	if len(entries) == 0 {
		if listOutput == "json" {
			fmt.Println("[]")
		} else {
			color.Yellow("No parameters found")
		}
		return nil
	}

	if listOutput == "json" {
		return outputListJSON(entries)
	}

	return outputListPlain(entries, sortBy)
}

// outputListJSON outputs entries as JSON
func outputListJSON(entries []cache.CacheEntry) error {
	type jsonEntry struct {
		Name             string            `json:"name"`
		Type             string            `json:"type"`
		Version          int64             `json:"version"`
		LastModifiedDate string            `json:"last_modified_date"`
		Tags             map[string]string `json:"tags,omitempty"`
	}

	output := make([]jsonEntry, len(entries))
	for i, e := range entries {
		output[i] = jsonEntry{
			Name:             e.Name,
			Type:             e.Type,
			Version:          e.Version,
			LastModifiedDate: e.LastModifiedDate.Format(time.RFC3339),
		}
		if listShowTags && len(e.Tags) > 0 {
			output[i].Tags = e.Tags
		}
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(output)
}

// outputListPlain outputs entries in plain text table format
func outputListPlain(entries []cache.CacheEntry, sortBy string) error {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)

	// Header
	header := "NAME\tTYPE\tVERSION\tMODIFIED"
	if listShowTags {
		header += "\tTAGS"
	}
	color.New(color.Bold).Fprintln(w, header)

	// Rows
	for _, e := range entries {
		row := fmt.Sprintf("%s\t%s\t%d\t%s",
			e.Name,
			e.Type,
			e.Version,
			formatRelativeTime(e.LastModifiedDate),
		)
		if listShowTags {
			row += "\t" + formatTagsCompact(e.Tags)
		}
		fmt.Fprintln(w, row)
	}

	w.Flush()

	// Footer
	fmt.Println()
	color.Cyan("Total: %d parameters (sorted by %s)", len(entries), sortBy)

	return nil
}

// formatRelativeTime formats time as relative (e.g., "2 hours ago")
func formatRelativeTime(t time.Time) string {
	duration := time.Since(t)

	if duration < time.Minute {
		return "just now"
	}
	if duration < time.Hour {
		mins := int(duration.Minutes())
		if mins == 1 {
			return "1 minute ago"
		}
		return fmt.Sprintf("%d minutes ago", mins)
	}
	if duration < 24*time.Hour {
		hours := int(duration.Hours())
		if hours == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", hours)
	}
	if duration < 30*24*time.Hour {
		days := int(duration.Hours() / 24)
		if days == 1 {
			return "1 day ago"
		}
		return fmt.Sprintf("%d days ago", days)
	}

	return t.Format("2006-01-02")
}

// formatTagsCompact formats tags compactly for table display
func formatTagsCompact(tags map[string]string) string {
	if len(tags) == 0 {
		return "-"
	}

	var pairs []string
	for k, v := range tags {
		pairs = append(pairs, fmt.Sprintf("%s=%s", k, v))
	}

	result := strings.Join(pairs, ", ")
	if len(result) > 40 {
		return result[:37] + "..."
	}
	return result
}
```

## Acceptance Criteria

- [ ] `clerk list` lists all parameters
- [ ] `clerk list "/dev/*"` lists parameters under /dev/
- [ ] `clerk list` uses cache when available and not expired
- [ ] `--sort name` sorts alphabetically by name
- [ ] `--sort modified` (or `-s m`) sorts by modification date (newest first)
- [ ] `--sort created` (or `-s c`) sorts by creation date
- [ ] `--tags` shows tags in output
- [ ] `--output json` outputs JSON array
- [ ] Empty results show appropriate message
- [ ] Table output is properly aligned
- [ ] Relative time formatting works (e.g., "2 hours ago")
- [ ] Exit code 0 on success
- [ ] Exit code 2 on AWS error

## Example Output

### Plain Output
```
NAME                    TYPE          VERSION  MODIFIED
/dev/db_password        SecureString  3        2 hours ago
/dev/api_key            SecureString  1        5 days ago
/dev/config             String        2        2024-12-15

Total: 3 parameters (sorted by name)
```

### Plain Output with Tags
```
NAME                    TYPE          VERSION  MODIFIED       TAGS
/dev/db_password        SecureString  3        2 hours ago    env=dev, team=backend
/dev/api_key            SecureString  1        5 days ago     env=dev
/dev/config             String        2        2024-12-15     -

Total: 3 parameters (sorted by name)
```

### JSON Output
```json
[
  {
    "name": "/dev/db_password",
    "type": "SecureString",
    "version": 3,
    "last_modified_date": "2026-01-02T10:30:00Z"
  },
  {
    "name": "/dev/api_key",
    "type": "SecureString",
    "version": 1,
    "last_modified_date": "2025-12-28T15:00:00Z"
  }
]
```

## Notes

- Uses cache first to avoid slow API calls
- Falls back to AWS API when cache is expired or empty
- Tags are only fetched when `--tags` flag is used (performance optimization)
- Relative time formatting makes output more readable
- Long tags are truncated in table view (full tags in JSON output)
- Sort aliases allow quick shortcuts (n, c, m)
