package config

import "time"

// Config represents the application configuration
type Config struct {
	Region                   string        `json:"region"`
	Profile                  string        `json:"profile"`
	CachePath                string        `json:"cache_path"`
	CacheTTL                 time.Duration `json:"cache_ttl"`
	ClipboardTimeout         time.Duration `json:"clipboard_timeout"`
	DefaultType              string        `json:"default_type"`
	DefaultSort              string        `json:"default_sort"`
	ParallelFetches          int           `json:"parallel_fetches"`
	SearchSlashPrefix        bool          `json:"search_slash_prefix"`
	DescribePageSize         int32         `json:"describe_page_size"`
	DescribeMaxItems         int32         `json:"describe_max_items"`
	DescribeVersionBatchSize int           `json:"describe_version_batch_size"`
	DecryptByDefault         bool          `json:"decrypt_by_default"`
	BrowseAutoRefresh        bool          `json:"browse_auto_refresh"`
	BrowseRefreshCooldown    time.Duration `json:"browse_refresh_cooldown"`
}

// DefaultConfig returns a Config with default values
func DefaultConfig() *Config {
	return &Config{
		Region:                   "us-east-1",
		Profile:                  "default",
		CachePath:                "", // Will be set to $HOME/.clerk/cache.json
		CacheTTL:                 3 * time.Hour,
		ClipboardTimeout:         60 * time.Second,
		DefaultType:              "SecureString",
		DefaultSort:              "name",
		ParallelFetches:          10,
		SearchSlashPrefix:        true,
		DescribePageSize:         50,
		DescribeMaxItems:         0, // 0 = unlimited
		DescribeVersionBatchSize: 10,
		DecryptByDefault:         true,
		BrowseAutoRefresh:        true,
		BrowseRefreshCooldown:    5 * time.Minute,
	}
}

// ValidTypes returns valid parameter types
func ValidTypes() []string {
	return []string{"String", "StringList", "SecureString"}
}

// ValidSortOptions returns valid sort options
func ValidSortOptions() []string {
	return []string{"name", "created", "modified", "n", "c", "m"}
}
