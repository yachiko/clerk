package cache

import (
	"time"

	"github.com/yachiko/clerk/internal/aws"
)

const SchemaVersion = 2

// VersionHistoryEntry represents a single version in history
type VersionHistoryEntry struct {
	Version  int64     `json:"version"`
	Modified time.Time `json:"modified"`
}

// CacheEntry represents cached resource metadata. Values must never be added to
// this type because cache files are durable plaintext metadata.
type CacheEntry struct {
	Identity         aws.ResourceIdentity  `json:"identity"`
	Name             string                `json:"name"`
	Type             string                `json:"type"`
	Version          int64                 `json:"version"`
	LastModifiedDate time.Time             `json:"last_modified_date"`
	Tags             map[string]string     `json:"tags,omitempty"`
	TagsFetchedAt    time.Time             `json:"tags_fetched_at,omitempty"`
	TagsComplete     bool                  `json:"tags_complete,omitempty"`
	TagsError        string                `json:"tags_error,omitempty"`
	VersionHistory   []VersionHistoryEntry `json:"version_history,omitempty"`
}

// CacheData represents the entire cache file structure
type CacheData struct {
	SchemaVersion int          `json:"schema_version"`
	Partition     string       `json:"partition"`
	AccountID     string       `json:"account_id"`
	Region        string       `json:"region"`
	Backend       aws.Backend  `json:"backend"`
	LastRefresh   time.Time    `json:"last_refresh"`
	Entries       []CacheEntry `json:"entries"`
	Complete      bool         `json:"complete"`
	// Incomplete is true when a discovery cap stopped a refresh. Such a cache
	// may be useful for known entries but must never imply absent parameters.
	Incomplete       bool   `json:"incomplete,omitempty"`
	LastRefreshError string `json:"last_refresh_error,omitempty"`
}

// CacheStats provides cache statistics
type CacheStats struct {
	TotalEntries     int
	LastRefresh      time.Time
	IsExpired        bool
	Partition        string
	AccountID        string
	Region           string
	Backend          aws.Backend
	Complete         bool
	LastRefreshError string
}
