# Task 04: Cache Module

## Objective
Implement the local caching system for parameter metadata to improve browse and search performance.

## Prerequisites
- Task 01 completed (project setup)
- Task 02 completed (configuration module)
- Task 03 completed (AWS SSM client)

## Deliverables

### 1. Create Cache Types

Create file `internal/cache/types.go`:

```go
package cache

import "time"

// CacheEntry represents a cached parameter
type CacheEntry struct {
	Name             string            `json:"name"`
	Type             string            `json:"type"`
	Version          int64             `json:"version"`
	LastModifiedDate time.Time         `json:"last_modified_date"`
	Tags             map[string]string `json:"tags,omitempty"`
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
```

### 2. Create Cache Manager

Create file `internal/cache/cache.go`:

```go
package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/yachiko/clerk/internal/aws"
	"github.com/yachiko/clerk/internal/config"
)

// Manager handles cache operations
type Manager struct {
	cachePath string
	ttl       time.Duration
	data      *CacheData
	mu        sync.RWMutex
	lockFile  string
}

// NewManager creates a new cache manager
func NewManager(cfg *config.Config) (*Manager, error) {
	cachePath := cfg.CachePath
	if cachePath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get home directory: %w", err)
		}
		cachePath = filepath.Join(home, ".clerk", "cache.json")
	}

	m := &Manager{
		cachePath: cachePath,
		ttl:       cfg.CacheTTL,
		lockFile:  cachePath + ".lock",
		data:      &CacheData{Entries: []CacheEntry{}},
	}

	// Try to load existing cache
	_ = m.load()

	return m, nil
}

// load reads cache from disk
func (m *Manager) load() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	data, err := os.ReadFile(m.cachePath)
	if err != nil {
		return err
	}

	return json.Unmarshal(data, m.data)
}

// save writes cache to disk with file locking
func (m *Manager) save() error {
	// Acquire file lock
	if err := m.acquireLock(); err != nil {
		return fmt.Errorf("failed to acquire cache lock: %w", err)
	}
	defer m.releaseLock()

	dir := filepath.Dir(m.cachePath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create cache directory: %w", err)
	}

	data, err := json.MarshalIndent(m.data, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal cache: %w", err)
	}

	if err := os.WriteFile(m.cachePath, data, 0600); err != nil {
		return fmt.Errorf("failed to write cache: %w", err)
	}

	return nil
}

// acquireLock creates a lock file
func (m *Manager) acquireLock() error {
	for i := 0; i < 10; i++ {
		f, err := os.OpenFile(m.lockFile, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err == nil {
			f.Close()
			return nil
		}
		if os.IsExist(err) {
			// Check if lock is stale (older than 5 minutes)
			info, statErr := os.Stat(m.lockFile)
			if statErr == nil && time.Since(info.ModTime()) > 5*time.Minute {
				os.Remove(m.lockFile)
				continue
			}
			time.Sleep(100 * time.Millisecond)
			continue
		}
		return err
	}
	return fmt.Errorf("failed to acquire lock after retries")
}

// releaseLock removes the lock file
func (m *Manager) releaseLock() {
	os.Remove(m.lockFile)
}

// IsExpired checks if the cache is expired
func (m *Manager) IsExpired() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.data.LastRefresh.IsZero() {
		return true
	}
	return time.Since(m.data.LastRefresh) > m.ttl
}

// GetStats returns cache statistics
func (m *Manager) GetStats() CacheStats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return CacheStats{
		TotalEntries: len(m.data.Entries),
		LastRefresh:  m.data.LastRefresh,
		IsExpired:    m.IsExpired(),
		Region:       m.data.Region,
	}
}

// GetAll returns all cached entries
func (m *Manager) GetAll() []CacheEntry {
	m.mu.RLock()
	defer m.mu.RUnlock()

	entries := make([]CacheEntry, len(m.data.Entries))
	copy(entries, m.data.Entries)
	return entries
}

// Search searches cache entries by glob pattern
func (m *Manager) Search(pattern string) []CacheEntry {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var results []CacheEntry
	for _, entry := range m.data.Entries {
		if matchGlob(pattern, entry.Name) {
			results = append(results, entry)
		}
	}
	return results
}

// SearchByTag searches cache entries by tag
func (m *Manager) SearchByTag(key, value string) []CacheEntry {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var results []CacheEntry
	for _, entry := range m.data.Entries {
		if v, ok := entry.Tags[key]; ok {
			if value == "" || v == value {
				results = append(results, entry)
			}
		}
	}
	return results
}

// Get retrieves a single entry by name
func (m *Manager) Get(name string) (*CacheEntry, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, entry := range m.data.Entries {
		if entry.Name == name {
			return &entry, true
		}
	}
	return nil, false
}

// Update updates or adds a single entry
func (m *Manager) Update(entry CacheEntry) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Find and update existing entry
	for i, e := range m.data.Entries {
		if e.Name == entry.Name {
			m.data.Entries[i] = entry
			return m.save()
		}
	}

	// Add new entry
	m.data.Entries = append(m.data.Entries, entry)
	return m.save()
}

// Delete removes an entry from cache
func (m *Manager) Delete(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for i, e := range m.data.Entries {
		if e.Name == name {
			m.data.Entries = append(m.data.Entries[:i], m.data.Entries[i+1:]...)
			return m.save()
		}
	}
	return nil
}

// RefreshProgressCallback is called during refresh with progress updates
type RefreshProgressCallback func(current, total int)

// Refresh updates the entire cache from AWS
func (m *Manager) Refresh(ctx context.Context, client *aws.Client, region string, parallel int, progressCb RefreshProgressCallback) error {
	// Fetch all parameter metadata
	params, err := client.DescribeAllParameters(ctx)
	if err != nil {
		return fmt.Errorf("failed to fetch parameters: %w", err)
	}

	total := len(params)
	entries := make([]CacheEntry, total)

	// Fetch tags in parallel
	sem := make(chan struct{}, parallel)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var fetchErr error
	completed := 0

	for i, p := range params {
		wg.Add(1)
		go func(idx int, param aws.ParameterMetadata) {
			defer wg.Done()

			sem <- struct{}{}
			defer func() { <-sem }()

			// Check for context cancellation
			if ctx.Err() != nil {
				return
			}

			// Check for previous errors
			mu.Lock()
			if fetchErr != nil {
				mu.Unlock()
				return
			}
			mu.Unlock()

			entry := CacheEntry{
				Name:             param.Name,
				Type:             param.Type,
				Version:          param.Version,
				LastModifiedDate: param.LastModifiedDate,
			}

			// Fetch tags
			tags, err := client.GetParameterTags(ctx, param.Name)
			if err == nil {
				entry.Tags = tags
			}

			mu.Lock()
			entries[idx] = entry
			completed++
			if progressCb != nil {
				progressCb(completed, total)
			}
			mu.Unlock()
		}(i, p)
	}

	wg.Wait()

	if fetchErr != nil {
		return fetchErr
	}

	if ctx.Err() != nil {
		return ctx.Err()
	}

	// Update cache
	m.mu.Lock()
	m.data = &CacheData{
		LastRefresh: time.Now(),
		Region:      region,
		Entries:     entries,
	}
	m.mu.Unlock()

	return m.save()
}

// Sort sorts entries by the given criteria
func (m *Manager) Sort(entries []CacheEntry, by string) []CacheEntry {
	sorted := make([]CacheEntry, len(entries))
	copy(sorted, entries)

	switch by {
	case "name", "n":
		sort.Slice(sorted, func(i, j int) bool {
			return sorted[i].Name < sorted[j].Name
		})
	case "created", "c":
		sort.Slice(sorted, func(i, j int) bool {
			return sorted[i].LastModifiedDate.Before(sorted[j].LastModifiedDate)
		})
	case "modified", "m":
		sort.Slice(sorted, func(i, j int) bool {
			return sorted[i].LastModifiedDate.After(sorted[j].LastModifiedDate)
		})
	}

	return sorted
}

// matchGlob performs glob pattern matching
func matchGlob(pattern, name string) bool {
	if pattern == "" || pattern == "*" || pattern == "/*" {
		return true
	}

	// Handle /path/* pattern
	if strings.HasSuffix(pattern, "/*") {
		prefix := strings.TrimSuffix(pattern, "/*")
		if prefix == "" {
			return true
		}
		return strings.HasPrefix(name, prefix+"/") || name == prefix
	}

	// Handle *pattern* (contains)
	if strings.HasPrefix(pattern, "*") && strings.HasSuffix(pattern, "*") {
		substr := strings.Trim(pattern, "*")
		return strings.Contains(strings.ToLower(name), strings.ToLower(substr))
	}

	// Handle pattern* (prefix)
	if strings.HasSuffix(pattern, "*") {
		prefix := strings.TrimSuffix(pattern, "*")
		return strings.HasPrefix(name, prefix)
	}

	// Handle *pattern (suffix)
	if strings.HasPrefix(pattern, "*") {
		suffix := strings.TrimPrefix(pattern, "*")
		return strings.HasSuffix(name, suffix)
	}

	// Case-insensitive contains for simple search
	return strings.Contains(strings.ToLower(name), strings.ToLower(pattern))
}
```

## Acceptance Criteria

- [ ] Cache file is created at configured location
- [ ] Cache loads on startup if file exists
- [ ] `Refresh` fetches all parameters from AWS
- [ ] `Refresh` fetches tags in parallel (configurable concurrency)
- [ ] `Refresh` calls progress callback with updates
- [ ] `Search` returns entries matching glob pattern
- [ ] `SearchByTag` returns entries with matching tags
- [ ] `Update` modifies single entries
- [ ] `Delete` removes entries from cache
- [ ] `IsExpired` correctly checks TTL
- [ ] File locking prevents concurrent writes
- [ ] Stale locks (>5 min) are automatically cleaned
- [ ] `Sort` works for name, created, and modified

## Notes

- Cache stores metadata only, never secret values
- File permissions are 0600 for security
- Lock file mechanism prevents corruption from concurrent CLI invocations
- Glob matching is case-insensitive for search convenience
- Tags are fetched in parallel to speed up refresh
- Progress callback enables UI updates during refresh
