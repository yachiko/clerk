package cache

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yachiko/clerk/internal/config"
	"github.com/yachiko/clerk/internal/testutil"
)

func newTestManager(t *testing.T) (*Manager, string) {
	t.Helper()
	home := testutil.IsolateHome(t)
	cfg := &config.Config{CacheTTL: 1 * time.Hour}
	mgr, err := NewManager(cfg, "us-east-1", "123456789012")
	require.NoError(t, err)
	return mgr, home
}

func TestNewManager_PathLayout(t *testing.T) {
	mgr, home := newTestManager(t)
	expected := filepath.Join(home, ".clerk", "cache", "123456789012", "us-east-1.json")
	assert.Equal(t, expected, mgr.cachePath)
}

func TestManager_IsExpired(t *testing.T) {
	mgr, _ := newTestManager(t)

	// Fresh cache (LastRefresh zero) is considered expired.
	assert.True(t, mgr.IsExpired())

	mgr.data.LastRefresh = time.Now()
	assert.False(t, mgr.IsExpired())

	mgr.data.LastRefresh = time.Now().Add(-2 * time.Hour)
	assert.True(t, mgr.IsExpired())
}

func TestManager_GetAge(t *testing.T) {
	mgr, _ := newTestManager(t)

	// Zero LastRefresh → zero age (not the wall-clock-since-epoch duration)
	assert.Equal(t, time.Duration(0), mgr.GetAge())

	mgr.data.LastRefresh = time.Now().Add(-10 * time.Minute)
	age := mgr.GetAge()
	assert.Greater(t, age, 9*time.Minute)
	assert.Less(t, age, 11*time.Minute)
}

func TestManager_GetStats(t *testing.T) {
	mgr, _ := newTestManager(t)

	stats := mgr.GetStats()
	assert.Equal(t, 0, stats.TotalEntries)
	assert.True(t, stats.IsExpired)

	mgr.data.Entries = []CacheEntry{{Name: "/a"}, {Name: "/b"}}
	mgr.data.LastRefresh = time.Now()
	mgr.data.Region = "us-east-1"

	stats = mgr.GetStats()
	assert.Equal(t, 2, stats.TotalEntries)
	assert.False(t, stats.IsExpired)
	assert.Equal(t, "us-east-1", stats.Region)
}

func TestManager_GetAll_ReturnsCopy(t *testing.T) {
	mgr, _ := newTestManager(t)
	mgr.data.Entries = []CacheEntry{{Name: "/a"}, {Name: "/b"}}

	got := mgr.GetAll()
	assert.Len(t, got, 2)

	// Mutating the returned slice must not affect the cache.
	got[0].Name = "/mutated"
	assert.Equal(t, "/a", mgr.data.Entries[0].Name)
}

func TestManager_Get(t *testing.T) {
	mgr, _ := newTestManager(t)
	mgr.data.Entries = []CacheEntry{{Name: "/found", Version: 3}}

	entry, ok := mgr.Get("/found")
	require.True(t, ok)
	assert.Equal(t, int64(3), entry.Version)

	_, ok = mgr.Get("/missing")
	assert.False(t, ok)
}

func TestManager_Search_Glob(t *testing.T) {
	mgr, _ := newTestManager(t)
	mgr.data.Entries = []CacheEntry{
		{Name: "/prod/db/password"},
		{Name: "/prod/api/key"},
		{Name: "/dev/db/password"},
		{Name: "/dev/api/key"},
	}

	tests := []struct {
		name    string
		pattern string
		wantLen int
	}{
		{"empty", "", 4},
		{"all-slash", "/", 4},
		{"all-glob", "/*", 4},
		{"prod prefix", "/prod/*", 2},
		{"suffix glob", "*password", 2},
		{"contains", "*db*", 2},
		{"prefix only", "/prod", 2},
		{"no match", "/staging/*", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results := mgr.Search(tt.pattern)
			assert.Len(t, results, tt.wantLen)
		})
	}
}

func TestManager_SearchByTag(t *testing.T) {
	mgr, _ := newTestManager(t)
	mgr.data.Entries = []CacheEntry{
		{Name: "/a", Tags: map[string]string{"env": "prod", "team": "be"}},
		{Name: "/b", Tags: map[string]string{"env": "dev"}},
		{Name: "/c", Tags: map[string]string{"team": "fe"}},
	}

	assert.Len(t, mgr.SearchByTag("env", ""), 2)
	assert.Len(t, mgr.SearchByTag("env", "prod"), 1)
	assert.Len(t, mgr.SearchByTag("env", "missing"), 0)
	assert.Len(t, mgr.SearchByTag("team", ""), 2)
	assert.Len(t, mgr.SearchByTag("nope", ""), 0)
}

func TestManager_Sort(t *testing.T) {
	mgr, _ := newTestManager(t)
	now := time.Now()
	entries := []CacheEntry{
		{Name: "/b", LastModifiedDate: now.Add(-1 * time.Hour)},
		{Name: "/a", LastModifiedDate: now.Add(-2 * time.Hour)},
		{Name: "/c", LastModifiedDate: now},
	}

	byName := mgr.Sort(entries, "name")
	assert.Equal(t, []string{"/a", "/b", "/c"}, names(byName))

	byShortName := mgr.Sort(entries, "n")
	assert.Equal(t, []string{"/a", "/b", "/c"}, names(byShortName))

	byModified := mgr.Sort(entries, "modified")
	assert.Equal(t, []string{"/c", "/b", "/a"}, names(byModified))

	byCreated := mgr.Sort(entries, "created")
	assert.Equal(t, []string{"/a", "/b", "/c"}, names(byCreated))

	// Unknown criterion: leaves input untouched (the returned slice is a copy).
	unknown := mgr.Sort(entries, "wat")
	assert.Equal(t, []string{"/b", "/a", "/c"}, names(unknown))
}

func TestManager_Update_AddAndModify(t *testing.T) {
	mgr, _ := newTestManager(t)

	require.NoError(t, mgr.Update(CacheEntry{Name: "/x", Version: 1}))
	require.NoError(t, mgr.Update(CacheEntry{Name: "/y", Version: 1}))
	assert.Len(t, mgr.data.Entries, 2)

	// Update existing — Version changes, count stays the same.
	require.NoError(t, mgr.Update(CacheEntry{Name: "/x", Version: 5}))
	assert.Len(t, mgr.data.Entries, 2)
	got, ok := mgr.Get("/x")
	require.True(t, ok)
	assert.Equal(t, int64(5), got.Version)
}

func TestManager_Delete(t *testing.T) {
	mgr, _ := newTestManager(t)
	require.NoError(t, mgr.Update(CacheEntry{Name: "/a"}))
	require.NoError(t, mgr.Update(CacheEntry{Name: "/b"}))

	require.NoError(t, mgr.Delete("/a"))
	assert.Len(t, mgr.data.Entries, 1)
	_, ok := mgr.Get("/a")
	assert.False(t, ok)

	// Deleting a missing entry is a no-op, not an error.
	require.NoError(t, mgr.Delete("/never-existed"))
	assert.Len(t, mgr.data.Entries, 1)
}

func TestManager_Persistence(t *testing.T) {
	// Update() calls save() internally; reloading a new manager with the same
	// HOME/region/account must see the persisted entries.
	cfg := &config.Config{CacheTTL: 1 * time.Hour}
	testutil.IsolateHome(t)

	mgr1, err := NewManager(cfg, "us-east-1", "111")
	require.NoError(t, err)
	require.NoError(t, mgr1.Update(CacheEntry{Name: "/persist/me", Version: 7, Type: "SecureString"}))

	mgr2, err := NewManager(cfg, "us-east-1", "111")
	require.NoError(t, err)
	got, ok := mgr2.Get("/persist/me")
	require.True(t, ok)
	assert.Equal(t, int64(7), got.Version)
	assert.Equal(t, "SecureString", got.Type)
}

func TestManager_RegionAndAccountIsolation(t *testing.T) {
	// Different region/account combos must use separate cache files.
	cfg := &config.Config{CacheTTL: 1 * time.Hour}
	testutil.IsolateHome(t)

	mgrEast, err := NewManager(cfg, "us-east-1", "111")
	require.NoError(t, err)
	require.NoError(t, mgrEast.Update(CacheEntry{Name: "/east-only"}))

	mgrWest, err := NewManager(cfg, "us-west-2", "111")
	require.NoError(t, err)

	mgrOtherAcct, err := NewManager(cfg, "us-east-1", "222")
	require.NoError(t, err)

	// West region in the same account: doesn't see east entries.
	_, ok := mgrWest.Get("/east-only")
	assert.False(t, ok)

	// Same region different account: doesn't see east entries either.
	_, ok = mgrOtherAcct.Get("/east-only")
	assert.False(t, ok)

	// Original east cache is still intact.
	mgrEast2, err := NewManager(cfg, "us-east-1", "111")
	require.NoError(t, err)
	_, ok = mgrEast2.Get("/east-only")
	assert.True(t, ok)
}

func TestMatchGlob(t *testing.T) {
	tests := []struct {
		pattern string
		name    string
		want    bool
	}{
		{"", "/anything", true},
		{"*", "/anything", true},
		{"/*", "/anything", true},
		{"/prod/*", "/prod/x", true},
		{"/prod/*", "/prod", true},
		{"/prod/*", "/dev/x", false},
		{"*pass*", "/db/password", true},
		{"*pass*", "/api/key", false},
		{"/prod*", "/prod/x", true},
		{"*key", "/api/key", true},
		{"*key", "/api/secret", false},
		{"plain", "/no-plain", true}, // contains
		{"plain", "/no-other", false},
	}
	for _, tt := range tests {
		t.Run(tt.pattern+"_vs_"+tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, matchGlob(tt.pattern, tt.name))
		})
	}
}

func TestManager_LockLifecycle(t *testing.T) {
	mgr, _ := newTestManager(t)
	// Lock file is computed from cache path; the cache directory may not exist
	// yet so we create it for the lock test.
	require.NoError(t, os.MkdirAll(filepath.Dir(mgr.lockFile), 0700))

	require.NoError(t, mgr.acquireLock())
	// Second acquire while held should fail-eventually (10 retries × 100ms).
	defer mgr.releaseLock()
	assert.FileExists(t, mgr.lockFile)
}

func TestManager_LoadIgnoresMissingFile(t *testing.T) {
	// NewManager calls load() and swallows missing-file errors. Verify that a
	// brand-new manager simply starts empty rather than failing.
	cfg := &config.Config{CacheTTL: time.Hour}
	testutil.IsolateHome(t)
	mgr, err := NewManager(cfg, "us-east-1", "999")
	require.NoError(t, err)
	assert.Empty(t, mgr.GetAll())
}

func names(entries []CacheEntry) []string {
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = e.Name
	}
	return out
}
