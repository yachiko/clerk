package cache

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestCacheEntry_Fields(t *testing.T) {
	now := time.Now()
	entry := CacheEntry{
		Name:             "/test/param",
		Type:             "SecureString",
		Version:          1,
		LastModifiedDate: now,
		Tags:             map[string]string{"env": "test"},
		VersionHistory: []VersionHistoryEntry{
			{Version: 1, Modified: now},
		},
	}

	assert.Equal(t, "/test/param", entry.Name)
	assert.Equal(t, "SecureString", entry.Type)
	assert.Equal(t, int64(1), entry.Version)
	assert.Equal(t, now, entry.LastModifiedDate)
	assert.Equal(t, "test", entry.Tags["env"])
	assert.Len(t, entry.VersionHistory, 1)
}

func TestCacheData_Fields(t *testing.T) {
	now := time.Now()
	data := CacheData{
		LastRefresh: now,
		Region:      "us-east-1",
		Entries: []CacheEntry{
			{Name: "/test/p1"},
			{Name: "/test/p2"},
		},
	}

	assert.Equal(t, now, data.LastRefresh)
	assert.Equal(t, "us-east-1", data.Region)
	assert.Len(t, data.Entries, 2)
}

func TestCacheStats_Fields(t *testing.T) {
	now := time.Now()
	stats := CacheStats{
		TotalEntries: 100,
		LastRefresh:  now,
		IsExpired:    false,
		Region:       "us-west-2",
	}

	assert.Equal(t, 100, stats.TotalEntries)
	assert.Equal(t, now, stats.LastRefresh)
	assert.False(t, stats.IsExpired)
	assert.Equal(t, "us-west-2", stats.Region)
}
