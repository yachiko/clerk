package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"github.com/yachiko/clerk/internal/aws"
	"github.com/yachiko/clerk/internal/cache"
	"github.com/yachiko/clerk/internal/config"
	"github.com/yachiko/clerk/internal/parammatch"
)

var (
	listSort     string
	listShowTags bool
)

type legacySSMListEntry struct {
	Name             string                      `json:"name"`
	Type             string                      `json:"type"`
	Version          int64                       `json:"version"`
	LastModifiedDate time.Time                   `json:"last_modified_date"`
	Tags             map[string]string           `json:"tags,omitempty"`
	TagsFetchedAt    time.Time                   `json:"tags_fetched_at,omitempty"`
	TagsComplete     bool                        `json:"tags_complete,omitempty"`
	TagsError        string                      `json:"tags_error,omitempty"`
	VersionHistory   []cache.VersionHistoryEntry `json:"version_history,omitempty"`
}

type metadataProvider struct {
	list func(context.Context, string, bool) ([]cache.CacheEntry, error)
}

// InitListCommand initializes the LIST command.
func InitListCommand() *cobra.Command {
	listCmd := &cobra.Command{
		Use:   "list [pattern]",
		Short: "List metadata from one secret backend",
		Long: `List secret metadata from the selected backend without retrieving values.

Parameter Store is used by default. SSM output preserves the legacy format.

Examples:
  clerk list
  clerk list "/dev/*" --backend ssm
  clerk list "*database*" --backend secretsmanager`,
		Args: cobra.MaximumNArgs(1),
		PreRunE: func(cmd *cobra.Command, _ []string) error {
			_, err := selectedBackend(cmd)
			return err
		},
		RunE: runList,
	}
	listCmd.Flags().StringVarP(&listSort, "sort", "s", "", "Sort by: name (n), created (c), modified (m)")
	listCmd.Flags().BoolVar(&listShowTags, "tags", false, "Show tags in output")
	return listCmd
}

func runList(cmd *cobra.Command, args []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	pattern := "/*"
	if len(args) > 0 {
		pattern = args[0]
	}
	if _, err := parammatch.Match(pattern, ""); err != nil {
		return err
	}
	backend, err := selectedBackend(cmd)
	if err != nil {
		return err
	}
	cfgMgr, err := config.NewManager()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	cfg := cfgMgr.Get()
	sortBy := normalizeSortOption(listSort)
	if listSort == "" {
		sortBy = normalizeSortOption(cfg.DefaultSort)
	}
	awsOpts, err := resolveAWSOptions(cmd, cfg)
	if err != nil {
		return err
	}
	resolved, err := aws.ResolveContext(ctx, awsOpts)
	if err != nil {
		return fmt.Errorf("failed to resolve AWS context: %w", err)
	}
	provider, err := listProvider(resolved, awsOpts, cfg, backend)
	if err != nil {
		return err
	}
	items, err := provider.list(ctx, pattern, listShowTags)
	if err != nil {
		return fmt.Errorf("failed to list %s metadata: %w", backend, err)
	}
	sortListEntries(items, sortBy)
	return outputList(items, backend, listShowTags)
}

func listProvider(resolved *aws.ResolvedContext, opts aws.ClientOptions, cfg *config.Config, selected aws.Backend) (metadataProvider, error) {
	if selected == aws.BackendSSM {
		client, err := aws.NewClientFromContext(resolved, opts)
		if err != nil {
			return metadataProvider{}, fmt.Errorf("failed to create SSM client: %w", err)
		}
		manager, err := cache.NewManagerForBackend(cfg, resolved.Partition, resolved.Region, resolved.AccountID, aws.BackendSSM)
		if err != nil {
			return metadataProvider{}, fmt.Errorf("failed to initialize SSM cache: %w", err)
		}
		return metadataProvider{list: func(ctx context.Context, pattern string, showTags bool) ([]cache.CacheEntry, error) {
			if !manager.IsExpired() {
				entries := manager.Search(pattern)
				if showTags {
					for i := range entries {
						if entries[i].TagsComplete && !entries[i].TagsFetchedAt.IsZero() && time.Since(entries[i].TagsFetchedAt) < 15*time.Minute {
							continue
						}
						entries[i].TagsFetchedAt = time.Now()
						tags, tagErr := client.GetParameterTags(ctx, entries[i].Name)
						if tagErr != nil {
							entries[i].TagsError = tagErr.Error()
							entries[i].TagsComplete = false
						} else {
							entries[i].Tags = tags
							entries[i].TagsError = ""
							entries[i].TagsComplete = true
						}
						_ = manager.Update(entries[i])
					}
				}
				return entries, nil
			}
			params, err := client.ListParameters(ctx, "/*", true)
			if err != nil {
				return nil, err
			}
			entries := make([]cache.CacheEntry, 0, len(params))
			for _, parameter := range params {
				entry := cache.CacheEntry{Identity: aws.ResourceIdentity{Partition: resolved.Partition, AccountID: resolved.AccountID, Region: resolved.Region, Backend: aws.BackendSSM, CanonicalID: parameter.Name}, Name: parameter.Name, Type: parameter.Type, Version: parameter.Version, LastModifiedDate: parameter.LastModifiedDate}
				if showTags && matchPath(pattern, parameter.Name) {
					entry.TagsFetchedAt = time.Now()
					tags, tagErr := client.GetParameterTags(ctx, parameter.Name)
					if tagErr != nil {
						entry.TagsError = tagErr.Error()
					} else {
						entry.Tags = tags
						entry.TagsComplete = true
					}
				}
				entries = append(entries, entry)
			}
			if err := manager.ReplaceSnapshot(entries); err != nil {
				return nil, fmt.Errorf("persist SSM metadata snapshot: %w", err)
			}
			return filterListEntries(entries, pattern), nil
		}}, nil
	}
	if selected == aws.BackendSecretsManager {
		client, err := aws.NewSecretsManagerClient(resolved)
		if err != nil {
			return metadataProvider{}, fmt.Errorf("failed to create Secrets Manager client: %w", err)
		}
		manager, err := cache.NewManagerForBackend(cfg, resolved.Partition, resolved.Region, resolved.AccountID, aws.BackendSecretsManager)
		if err != nil {
			return metadataProvider{}, fmt.Errorf("failed to initialize Secrets Manager cache: %w", err)
		}
		return metadataProvider{list: func(ctx context.Context, pattern string, _ bool) ([]cache.CacheEntry, error) {
			if !manager.IsExpired() {
				return manager.Search(pattern), nil
			}
			secrets, err := client.ListSecrets(ctx)
			if err != nil {
				return nil, err
			}
			entries := make([]cache.CacheEntry, 0, len(secrets))
			for _, secret := range secrets {
				modified := secret.LastChangedDate
				if modified == nil {
					modified = secret.CreatedDate
				}
				entry := cache.CacheEntry{Identity: secret.Identity, Name: secret.Name, Tags: secret.Tags, TagsComplete: true}
				if modified != nil {
					entry.LastModifiedDate = *modified
				}
				entries = append(entries, entry)
			}
			if err := manager.ReplaceSnapshot(entries); err != nil {
				return nil, fmt.Errorf("persist Secrets Manager metadata snapshot: %w", err)
			}
			return filterListEntries(entries, pattern), nil
		}}, nil
	}
	return metadataProvider{}, fmt.Errorf("unsupported backend %q", selected)
}

func filterListEntries(entries []cache.CacheEntry, pattern string) []cache.CacheEntry {
	result := make([]cache.CacheEntry, 0, len(entries))
	for _, entry := range entries {
		if matchPath(pattern, entry.Name) {
			result = append(result, entry)
		}
	}
	return result
}

func sortListEntries(entries []cache.CacheEntry, by string) {
	sort.SliceStable(entries, func(i, j int) bool {
		var less bool
		switch by {
		case "created":
			less = entries[i].LastModifiedDate.Before(entries[j].LastModifiedDate)
		case "modified":
			less = entries[i].LastModifiedDate.After(entries[j].LastModifiedDate)
		default:
			less = entries[i].Name < entries[j].Name
		}
		if entries[i].Name == entries[j].Name && entries[i].LastModifiedDate.Equal(entries[j].LastModifiedDate) {
			return entries[i].Identity.Backend < entries[j].Identity.Backend
		}
		return less
	})
}

func normalizeSortOption(value string) string {
	switch strings.ToLower(value) {
	case "c", "created":
		return "created"
	case "m", "modified":
		return "modified"
	default:
		return "name"
	}
}

func matchPath(pattern, name string) bool {
	ok, err := parammatch.Match(pattern, name)
	return err == nil && ok
}

func extractBasePath(pattern string) string {
	if index := strings.Index(pattern, "*"); index > 0 {
		base := strings.TrimSuffix(pattern[:index], "/")
		if base == "" {
			return "/"
		}
		return base
	}
	return pattern
}

func outputList(entries []cache.CacheEntry, selected aws.Backend, showTags bool) error {
	return outputListTo(os.Stdout, os.Stderr, entries, selected, showTags)
}

func outputListTo(stdout, stderr io.Writer, entries []cache.CacheEntry, selected aws.Backend, showTags bool) error {
	if entries == nil {
		entries = []cache.CacheEntry{}
	}
	if globalOpts.Output == "json" {
		if !showTags {
			entries = withoutListTags(entries)
		}
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		if selected == aws.BackendSSM {
			return encoder.Encode(legacySSMListEntries(entries))
		}
		return encoder.Encode(entries)
	}
	if len(entries) == 0 {
		if selected == aws.BackendSSM {
			_, _ = fmt.Fprintln(stderr, "No parameters found")
		} else {
			_, _ = fmt.Fprintln(stderr, "No secrets found")
		}
		return nil
	}
	w := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
	header := "NAME\tTYPE\tVERSION\tMODIFIED"
	if showTags {
		header += "\tTAGS"
	}
	_, _ = fmt.Fprintln(w, header)
	_, _ = fmt.Fprintln(w, strings.Repeat("-", 85))
	for _, entry := range entries {
		modified := entry.LastModifiedDate.Format("2006-01-02 15:04")
		entryType := entry.Type
		version := "-"
		if entry.Identity.Backend == aws.BackendSSM {
			version = fmt.Sprintf("%d", entry.Version)
		}
		if entryType == "" {
			entryType = "-"
		}
		if showTags {
			_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", entry.Name, entryType, version, modified, formatListTags(entry))
		} else {
			_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", entry.Name, entryType, version, modified)
		}
	}
	_ = w.Flush()
	if selected == aws.BackendSSM {
		fmt.Fprintf(stderr, "\nTotal: %d parameters\n", len(entries))
	} else {
		fmt.Fprintf(stderr, "\nTotal: %d secrets\n", len(entries))
	}
	return nil
}

func legacySSMListEntries(entries []cache.CacheEntry) []legacySSMListEntry {
	result := make([]legacySSMListEntry, len(entries))
	for i, entry := range entries {
		result[i].Name = entry.Name
		result[i].Type = entry.Type
		result[i].Version = entry.Version
		result[i].LastModifiedDate = entry.LastModifiedDate
		result[i].Tags = entry.Tags
		result[i].TagsFetchedAt = entry.TagsFetchedAt
		result[i].TagsComplete = entry.TagsComplete
		result[i].TagsError = entry.TagsError
		result[i].VersionHistory = entry.VersionHistory
	}
	return result
}

func withoutListTags(entries []cache.CacheEntry) []cache.CacheEntry {
	result := append([]cache.CacheEntry(nil), entries...)
	for i := range result {
		result[i].Tags = nil
		result[i].TagsFetchedAt = time.Time{}
		result[i].TagsComplete = false
		result[i].TagsError = ""
	}
	return result
}

func formatListTags(entry cache.CacheEntry) string {
	if entry.TagsError != "" {
		return "<unavailable: " + entry.TagsError + ">"
	}
	if !entry.TagsComplete {
		return "<unavailable>"
	}
	if len(entry.Tags) == 0 {
		return "-"
	}
	parts := make([]string, 0, len(entry.Tags))
	for key, value := range entry.Tags {
		parts = append(parts, key+"="+value)
	}
	sort.Strings(parts)
	return strings.Join(parts, ",")
}
