package config

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"
)

// Config represents the application configuration
type Config struct {
	Region                   string        `json:"region"`
	Profile                  string        `json:"profile"`
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
		// An empty region lets the AWS SDK resolve AWS_REGION and shared config.
		Region:                   "",
		Profile:                  "", // empty = let AWS SDK pick (honors AWS_PROFILE or "default" profile if present)
		CacheTTL:                 3 * time.Hour,
		ClipboardTimeout:         60 * time.Second,
		DefaultType:              "SecureString",
		DefaultSort:              "name",
		ParallelFetches:          10,
		SearchSlashPrefix:        true,
		DescribePageSize:         50,
		DescribeMaxItems:         0, // 0 = unlimited
		DescribeVersionBatchSize: 10,
		DecryptByDefault:         false,
		BrowseAutoRefresh:        true,
		BrowseRefreshCooldown:    5 * time.Minute,
	}
}

// UnmarshalJSON accepts the legacy numeric nanosecond representation as well
// as the documented Go duration strings.
func (c *Config) UnmarshalJSON(data []byte) error {
	type configJSON struct {
		Region                   string          `json:"region"`
		Profile                  string          `json:"profile"`
		CacheTTL                 json.RawMessage `json:"cache_ttl"`
		ClipboardTimeout         json.RawMessage `json:"clipboard_timeout"`
		DefaultType              string          `json:"default_type"`
		DefaultSort              string          `json:"default_sort"`
		ParallelFetches          int             `json:"parallel_fetches"`
		SearchSlashPrefix        bool            `json:"search_slash_prefix"`
		DescribePageSize         int64           `json:"describe_page_size"`
		DescribeMaxItems         int64           `json:"describe_max_items"`
		DescribeVersionBatchSize int             `json:"describe_version_batch_size"`
		DecryptByDefault         bool            `json:"decrypt_by_default"`
		BrowseAutoRefresh        bool            `json:"browse_auto_refresh"`
		BrowseRefreshCooldown    json.RawMessage `json:"browse_refresh_cooldown"`
	}
	var raw configJSON
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	parseDuration := func(field string, value json.RawMessage, fallback time.Duration) (time.Duration, error) {
		if len(value) == 0 || string(value) == "null" {
			return fallback, nil
		}
		var text string
		if err := json.Unmarshal(value, &text); err == nil {
			d, err := time.ParseDuration(text)
			if err != nil {
				return 0, fmt.Errorf("%s: invalid duration %q: %w", field, text, err)
			}
			return d, nil
		}
		var numeric int64
		if err := json.Unmarshal(value, &numeric); err != nil {
			return 0, fmt.Errorf("%s must be a duration string or legacy integer nanoseconds", field)
		}
		return time.Duration(numeric), nil
	}
	var err error
	if c.CacheTTL, err = parseDuration("cache_ttl", raw.CacheTTL, c.CacheTTL); err != nil {
		return err
	}
	if c.ClipboardTimeout, err = parseDuration("clipboard_timeout", raw.ClipboardTimeout, c.ClipboardTimeout); err != nil {
		return err
	}
	if c.BrowseRefreshCooldown, err = parseDuration("browse_refresh_cooldown", raw.BrowseRefreshCooldown, c.BrowseRefreshCooldown); err != nil {
		return err
	}
	if _, ok := fields["region"]; ok {
		c.Region = raw.Region
	}
	if _, ok := fields["profile"]; ok {
		c.Profile = raw.Profile
	}
	if _, ok := fields["default_type"]; ok {
		c.DefaultType = raw.DefaultType
	}
	if _, ok := fields["default_sort"]; ok {
		c.DefaultSort = raw.DefaultSort
	}
	if _, ok := fields["parallel_fetches"]; ok {
		c.ParallelFetches = raw.ParallelFetches
	}
	if _, ok := fields["search_slash_prefix"]; ok {
		c.SearchSlashPrefix = raw.SearchSlashPrefix
	}
	if _, ok := fields["describe_version_batch_size"]; ok {
		c.DescribeVersionBatchSize = raw.DescribeVersionBatchSize
	}
	if raw.DescribePageSize > math.MaxInt32 || raw.DescribePageSize < math.MinInt32 {
		return fmt.Errorf("describe_page_size is outside int32 range")
	}
	if raw.DescribeMaxItems > math.MaxInt32 || raw.DescribeMaxItems < math.MinInt32 {
		return fmt.Errorf("describe_max_items is outside int32 range")
	}
	if _, ok := fields["describe_page_size"]; ok {
		c.DescribePageSize = int32(raw.DescribePageSize)
	}
	if _, ok := fields["describe_max_items"]; ok {
		c.DescribeMaxItems = int32(raw.DescribeMaxItems)
	}
	if _, ok := fields["decrypt_by_default"]; ok {
		c.DecryptByDefault = raw.DecryptByDefault
	}
	if _, ok := fields["browse_auto_refresh"]; ok {
		c.BrowseAutoRefresh = raw.BrowseAutoRefresh
	}
	return c.Validate()
}

func (c Config) MarshalJSON() ([]byte, error) {
	type configJSON struct {
		Region                   string `json:"region"`
		Profile                  string `json:"profile"`
		CacheTTL                 string `json:"cache_ttl"`
		ClipboardTimeout         string `json:"clipboard_timeout"`
		DefaultType              string `json:"default_type"`
		DefaultSort              string `json:"default_sort"`
		ParallelFetches          int    `json:"parallel_fetches"`
		SearchSlashPrefix        bool   `json:"search_slash_prefix"`
		DescribePageSize         int32  `json:"describe_page_size"`
		DescribeMaxItems         int32  `json:"describe_max_items"`
		DescribeVersionBatchSize int    `json:"describe_version_batch_size"`
		DecryptByDefault         bool   `json:"decrypt_by_default"`
		BrowseAutoRefresh        bool   `json:"browse_auto_refresh"`
		BrowseRefreshCooldown    string `json:"browse_refresh_cooldown"`
	}
	return json.Marshal(configJSON{c.Region, c.Profile, c.CacheTTL.String(), c.ClipboardTimeout.String(), c.DefaultType, c.DefaultSort, c.ParallelFetches, c.SearchSlashPrefix, c.DescribePageSize, c.DescribeMaxItems, c.DescribeVersionBatchSize, c.DecryptByDefault, c.BrowseAutoRefresh, c.BrowseRefreshCooldown.String()})
}

// Validate canonicalizes values used by AWS and rejects unsafe runtime values.
func (c *Config) Validate() error {
	c.Region, c.Profile = strings.TrimSpace(c.Region), strings.TrimSpace(c.Profile)
	if c.CacheTTL < 0 {
		return fmt.Errorf("cache_ttl must be >= 0")
	}
	if c.ClipboardTimeout < 0 {
		return fmt.Errorf("clipboard_timeout must be >= 0")
	}
	if c.BrowseRefreshCooldown < 0 {
		return fmt.Errorf("browse_refresh_cooldown must be >= 0")
	}
	if c.ParallelFetches < 1 || c.ParallelFetches > 50 {
		return fmt.Errorf("parallel_fetches must be between 1 and 50")
	}
	if c.DescribePageSize < 1 || c.DescribePageSize > 50 {
		return fmt.Errorf("describe_page_size must be between 1 and 50")
	}
	if c.DescribeMaxItems < 0 {
		return fmt.Errorf("describe_max_items must be >= 0 (0 = unlimited)")
	}
	if c.DescribeVersionBatchSize < 1 || c.DescribeVersionBatchSize > 50 {
		return fmt.Errorf("describe_version_batch_size must be between 1 and 50")
	}
	if !isValidType(c.DefaultType) {
		return fmt.Errorf("default_type: invalid type %q", c.DefaultType)
	}
	for _, valid := range ValidTypes() {
		if strings.EqualFold(c.DefaultType, valid) {
			c.DefaultType = valid
			break
		}
	}
	if !isValidSort(c.DefaultSort) {
		return fmt.Errorf("default_sort: invalid sort option %q", c.DefaultSort)
	}
	c.DefaultSort = strings.ToLower(c.DefaultSort)
	return nil
}

// ValidTypes returns valid parameter types
func ValidTypes() []string {
	return []string{"String", "StringList", "SecureString"}
}

// ValidSortOptions returns valid sort options
func ValidSortOptions() []string {
	return []string{"name", "created", "modified", "n", "c", "m"}
}
