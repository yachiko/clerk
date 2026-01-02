package cache

import "time"

// VersionHistoryEntry represents a single version in history
type VersionHistoryEntry struct {
	Version  int64     `json:"version"`
	Modified time.Time `json:"modified"`
}

// CacheEntry represents a cached parameter
type CacheEntry struct {
	Name             string                `json:"name"`
	Type             string                `json:"type"`
	Version          int64                 `json:"version"`
	LastModifiedDate time.Time             `json:"last_modified_date"`
	Tags             map[string]string     `json:"tags,omitempty"`
	VersionHistory   []VersionHistoryEntry `json:"version_history,omitempty"`
}

// CacheData represents the entire cache file structure
type CacheData struct {
	LastRefresh time.Time    `json:"last_refresh"`
	Region      string       `json:"region"`
	Entries     []CacheEntry `json:"entries"`
}

// CacheStats provides cache statistics
type CacheStats struct {
	TotalEntries int
	LastRefresh  time.Time
	IsExpired    bool
	Region       string
}
