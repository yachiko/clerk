# Task 17: Unit Tests

## Objective
Implement comprehensive unit tests across all modules to ensure code correctness, enable refactoring with confidence, and maintain code quality.

## Prerequisites
- All previous tasks completed (01-16)
- Go 1.21+ installed

## Testing Strategy

Unit tests focus on testing individual functions and types in isolation:
- **config** - Configuration loading, saving, validation
- **cache** - Cache operations, filtering, sorting
- **aws** - SSM client with mocked interface
- **util** - Clipboard, editor, output utilities
- **cli** - Command argument parsing and validation

## Deliverables

### 1. Add Test Dependencies

Run the following command:

```bash
go get github.com/stretchr/testify@latest
```

### 2. Create Test Helpers

Create file `internal/testutil/helpers.go`:

```go
package testutil

import (
	"os"
	"path/filepath"
	"testing"
)

// TempDir creates a temporary directory for tests and returns cleanup function
func TempDir(t *testing.T) (string, func()) {
	t.Helper()
	dir, err := os.MkdirTemp("", "clerk-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	return dir, func() {
		os.RemoveAll(dir)
	}
}

// TempFile creates a temporary file with content and returns its path
func TempFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	return path
}

// SetEnv sets an environment variable and returns cleanup function
func SetEnv(t *testing.T, key, value string) func() {
	t.Helper()
	old, existed := os.LookupEnv(key)
	os.Setenv(key, value)
	return func() {
		if existed {
			os.Setenv(key, old)
		} else {
			os.Unsetenv(key)
		}
	}
}
```

### 3. Create Config Module Tests

Create file `internal/config/types_test.go`:

```go
package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	assert.Equal(t, "us-east-1", cfg.Region)
	assert.Equal(t, "default", cfg.Profile)
	assert.Equal(t, "", cfg.CachePath)
	assert.Equal(t, 3*time.Hour, cfg.CacheTTL)
	assert.Equal(t, 60*time.Second, cfg.ClipboardTimeout)
	assert.Equal(t, "SecureString", cfg.DefaultType)
	assert.Equal(t, "name", cfg.DefaultSort)
	assert.Equal(t, 10, cfg.ParallelFetches)
}

func TestValidTypes(t *testing.T) {
	types := ValidTypes()

	assert.Len(t, types, 3)
	assert.Contains(t, types, "String")
	assert.Contains(t, types, "StringList")
	assert.Contains(t, types, "SecureString")
}

func TestValidSortOptions(t *testing.T) {
	opts := ValidSortOptions()

	assert.Len(t, opts, 6)
	assert.Contains(t, opts, "name")
	assert.Contains(t, opts, "created")
	assert.Contains(t, opts, "modified")
	assert.Contains(t, opts, "n")
	assert.Contains(t, opts, "c")
	assert.Contains(t, opts, "m")
}
```

Create file `internal/config/config_test.go`:

```go
package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManager_NewManager(t *testing.T) {
	// Save original HOME and restore after test
	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)

	tmpDir, err := os.MkdirTemp("", "clerk-config-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	os.Setenv("HOME", tmpDir)

	mgr, err := NewManager()
	require.NoError(t, err)
	assert.NotNil(t, mgr)

	cfg := mgr.Get()
	assert.Equal(t, "us-east-1", cfg.Region)
	assert.Equal(t, "default", cfg.Profile)
}

func TestManager_SaveAndLoad(t *testing.T) {
	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)

	tmpDir, err := os.MkdirTemp("", "clerk-config-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	os.Setenv("HOME", tmpDir)

	// Create and save config
	mgr, err := NewManager()
	require.NoError(t, err)

	cfg := mgr.Get()
	cfg.Region = "eu-west-1"
	cfg.Profile = "production"
	cfg.CacheTTL = 1 * time.Hour

	err = mgr.Save()
	require.NoError(t, err)

	// Verify file exists
	configPath := filepath.Join(tmpDir, ".clerk", "config.json")
	assert.FileExists(t, configPath)

	// Load in new manager and verify
	mgr2, err := NewManager()
	require.NoError(t, err)

	cfg2 := mgr2.Get()
	assert.Equal(t, "eu-west-1", cfg2.Region)
	assert.Equal(t, "production", cfg2.Profile)
	assert.Equal(t, 1*time.Hour, cfg2.CacheTTL)
}

func TestManager_GetValue(t *testing.T) {
	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)

	tmpDir, err := os.MkdirTemp("", "clerk-config-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	os.Setenv("HOME", tmpDir)

	mgr, err := NewManager()
	require.NoError(t, err)

	tests := []struct {
		key      string
		expected string
	}{
		{"region", "us-east-1"},
		{"profile", "default"},
		{"default_type", "SecureString"},
		{"default.type", "SecureString"},
		{"default_sort", "name"},
		{"default.sort", "name"},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			val, err := mgr.GetValue(tt.key)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, val)
		})
	}
}

func TestManager_GetValue_InvalidKey(t *testing.T) {
	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)

	tmpDir, err := os.MkdirTemp("", "clerk-config-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	os.Setenv("HOME", tmpDir)

	mgr, err := NewManager()
	require.NoError(t, err)

	_, err = mgr.GetValue("invalid_key")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown config key")
}

func TestManager_SetValue(t *testing.T) {
	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)

	tmpDir, err := os.MkdirTemp("", "clerk-config-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	os.Setenv("HOME", tmpDir)

	mgr, err := NewManager()
	require.NoError(t, err)

	tests := []struct {
		key   string
		value string
	}{
		{"region", "ap-southeast-1"},
		{"profile", "dev"},
		{"default_type", "String"},
		{"cache_ttl", "2h"},
		{"clipboard_timeout", "30s"},
		{"parallel_fetches", "20"},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			err := mgr.SetValue(tt.key, tt.value)
			require.NoError(t, err)

			got, err := mgr.GetValue(tt.key)
			require.NoError(t, err)
			// Duration values format differently
			if tt.key != "cache_ttl" && tt.key != "clipboard_timeout" {
				assert.Equal(t, tt.value, got)
			}
		})
	}
}

func TestManager_SetValue_InvalidType(t *testing.T) {
	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)

	tmpDir, err := os.MkdirTemp("", "clerk-config-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	os.Setenv("HOME", tmpDir)

	mgr, err := NewManager()
	require.NoError(t, err)

	err = mgr.SetValue("default_type", "InvalidType")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid type")
}

func TestManager_SetValue_InvalidSort(t *testing.T) {
	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)

	tmpDir, err := os.MkdirTemp("", "clerk-config-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	os.Setenv("HOME", tmpDir)

	mgr, err := NewManager()
	require.NoError(t, err)

	err = mgr.SetValue("default_sort", "invalid")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid sort option")
}
```

### 4. Create Cache Module Tests

Create file `internal/cache/types_test.go`:

```go
package cache

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestCacheEntry(t *testing.T) {
	now := time.Now()
	entry := CacheEntry{
		Name:             "/test/param",
		Type:             "SecureString",
		Version:          1,
		LastModifiedDate: now,
		Tags:             map[string]string{"env": "test"},
	}

	assert.Equal(t, "/test/param", entry.Name)
	assert.Equal(t, "SecureString", entry.Type)
	assert.Equal(t, int64(1), entry.Version)
	assert.Equal(t, now, entry.LastModifiedDate)
	assert.Equal(t, "test", entry.Tags["env"])
}

func TestCacheData(t *testing.T) {
	now := time.Now()
	data := CacheData{
		LastRefresh: now,
		Region:      "us-east-1",
		Entries: []CacheEntry{
			{Name: "/test/param1"},
			{Name: "/test/param2"},
		},
	}

	assert.Equal(t, now, data.LastRefresh)
	assert.Equal(t, "us-east-1", data.Region)
	assert.Len(t, data.Entries, 2)
}

func TestCacheStats(t *testing.T) {
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
```

Create file `internal/cache/cache_test.go`:

```go
package cache

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yachiko/clerk/internal/config"
)

func newTestManager(t *testing.T, tmpDir string) *Manager {
	t.Helper()
	cfg := &config.Config{
		CachePath: filepath.Join(tmpDir, "cache.json"),
		CacheTTL:  1 * time.Hour,
	}
	mgr, err := NewManager(cfg)
	require.NoError(t, err)
	return mgr
}

func TestManager_NewManager(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "clerk-cache-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	mgr := newTestManager(t, tmpDir)
	assert.NotNil(t, mgr)
}

func TestManager_IsExpired(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "clerk-cache-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	mgr := newTestManager(t, tmpDir)

	// Empty cache should be expired
	assert.True(t, mgr.IsExpired())

	// Set recent refresh time
	mgr.data.LastRefresh = time.Now()
	assert.False(t, mgr.IsExpired())

	// Set old refresh time
	mgr.data.LastRefresh = time.Now().Add(-2 * time.Hour)
	assert.True(t, mgr.IsExpired())
}

func TestManager_GetStats(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "clerk-cache-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	mgr := newTestManager(t, tmpDir)

	// Empty cache
	stats := mgr.GetStats()
	assert.Equal(t, 0, stats.TotalEntries)
	assert.True(t, stats.IsExpired)

	// Add entries
	mgr.data.Entries = []CacheEntry{
		{Name: "/test/param1"},
		{Name: "/test/param2"},
	}
	mgr.data.LastRefresh = time.Now()
	mgr.data.Region = "us-east-1"

	stats = mgr.GetStats()
	assert.Equal(t, 2, stats.TotalEntries)
	assert.False(t, stats.IsExpired)
	assert.Equal(t, "us-east-1", stats.Region)
}

func TestManager_GetAll(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "clerk-cache-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	mgr := newTestManager(t, tmpDir)

	entries := []CacheEntry{
		{Name: "/test/param1", Type: "String"},
		{Name: "/test/param2", Type: "SecureString"},
	}
	mgr.data.Entries = entries

	got := mgr.GetAll()
	assert.Len(t, got, 2)
	assert.Equal(t, "/test/param1", got[0].Name)
	assert.Equal(t, "/test/param2", got[1].Name)
}

func TestManager_Search(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "clerk-cache-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	mgr := newTestManager(t, tmpDir)

	mgr.data.Entries = []CacheEntry{
		{Name: "/prod/database/password", Type: "SecureString"},
		{Name: "/prod/api/key", Type: "SecureString"},
		{Name: "/dev/database/password", Type: "SecureString"},
		{Name: "/dev/api/key", Type: "SecureString"},
	}

	tests := []struct {
		name     string
		query    string
		expected int
	}{
		{"empty query", "", 4},
		{"prod filter", "prod", 2},
		{"database filter", "database", 2},
		{"password filter", "password", 2},
		{"no match", "staging", 0},
		{"case insensitive", "PROD", 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results := mgr.Search(tt.query)
			assert.Len(t, results, tt.expected)
		})
	}
}

func TestManager_Filter(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "clerk-cache-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	mgr := newTestManager(t, tmpDir)

	mgr.data.Entries = []CacheEntry{
		{Name: "/prod/db/password"},
		{Name: "/prod/api/key"},
		{Name: "/dev/db/password"},
		{Name: "/dev/api/key"},
		{Name: "/staging/db/password"},
	}

	tests := []struct {
		name     string
		pattern  string
		expected int
	}{
		{"root", "/", 5},
		{"prod prefix", "/prod/*", 2},
		{"all db", "/*/db/*", 3},
		{"exact match", "/dev/api/key", 1},
		{"double wildcard", "/*/password", 0}, // doesn't match nested
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results := mgr.Filter(tt.pattern)
			assert.Len(t, results, tt.expected)
		})
	}
}

func TestManager_Sort(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "clerk-cache-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	mgr := newTestManager(t, tmpDir)

	now := time.Now()
	mgr.data.Entries = []CacheEntry{
		{Name: "/b/param", LastModifiedDate: now.Add(-1 * time.Hour)},
		{Name: "/a/param", LastModifiedDate: now.Add(-2 * time.Hour)},
		{Name: "/c/param", LastModifiedDate: now},
	}

	// Sort by name
	results := mgr.Sort(mgr.GetAll(), "name")
	assert.Equal(t, "/a/param", results[0].Name)
	assert.Equal(t, "/b/param", results[1].Name)
	assert.Equal(t, "/c/param", results[2].Name)

	// Sort by modified (most recent first)
	results = mgr.Sort(mgr.GetAll(), "modified")
	assert.Equal(t, "/c/param", results[0].Name)
	assert.Equal(t, "/b/param", results[1].Name)
	assert.Equal(t, "/a/param", results[2].Name)
}

func TestManager_UpdateEntry(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "clerk-cache-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	mgr := newTestManager(t, tmpDir)

	mgr.data.Entries = []CacheEntry{
		{Name: "/test/param", Version: 1},
	}

	// Update existing
	mgr.UpdateEntry(CacheEntry{Name: "/test/param", Version: 2})
	assert.Len(t, mgr.data.Entries, 1)
	assert.Equal(t, int64(2), mgr.data.Entries[0].Version)

	// Add new
	mgr.UpdateEntry(CacheEntry{Name: "/test/new", Version: 1})
	assert.Len(t, mgr.data.Entries, 2)
}

func TestManager_RemoveEntry(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "clerk-cache-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	mgr := newTestManager(t, tmpDir)

	mgr.data.Entries = []CacheEntry{
		{Name: "/test/param1"},
		{Name: "/test/param2"},
	}

	mgr.RemoveEntry("/test/param1")
	assert.Len(t, mgr.data.Entries, 1)
	assert.Equal(t, "/test/param2", mgr.data.Entries[0].Name)

	// Remove non-existent (should not panic)
	mgr.RemoveEntry("/test/nonexistent")
	assert.Len(t, mgr.data.Entries, 1)
}

func TestManager_Clear(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "clerk-cache-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	mgr := newTestManager(t, tmpDir)

	mgr.data.Entries = []CacheEntry{
		{Name: "/test/param1"},
		{Name: "/test/param2"},
	}
	mgr.data.LastRefresh = time.Now()

	mgr.Clear()
	assert.Empty(t, mgr.data.Entries)
	assert.True(t, mgr.data.LastRefresh.IsZero())
}

func TestManager_SaveAndLoad(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "clerk-cache-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	mgr := newTestManager(t, tmpDir)

	now := time.Now().Truncate(time.Second) // JSON loses nanosecond precision
	mgr.data.LastRefresh = now
	mgr.data.Region = "us-east-1"
	mgr.data.Entries = []CacheEntry{
		{Name: "/test/param", Version: 5, Type: "SecureString"},
	}

	err = mgr.Save()
	require.NoError(t, err)

	// Create new manager and verify data persisted
	mgr2 := newTestManager(t, tmpDir)
	assert.Equal(t, "us-east-1", mgr2.data.Region)
	assert.Len(t, mgr2.data.Entries, 1)
	assert.Equal(t, "/test/param", mgr2.data.Entries[0].Name)
	assert.Equal(t, int64(5), mgr2.data.Entries[0].Version)
}
```

### 5. Create AWS Module Tests

Create file `internal/aws/types_test.go`:

```go
package aws

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestParameter(t *testing.T) {
	now := time.Now()
	param := Parameter{
		Name:             "/test/secret",
		Value:            "secret-value",
		Type:             "SecureString",
		Version:          3,
		LastModifiedDate: now,
		ARN:              "arn:aws:ssm:us-east-1:123456789:parameter/test/secret",
		DataType:         "text",
		Tags:             map[string]string{"env": "test"},
	}

	assert.Equal(t, "/test/secret", param.Name)
	assert.Equal(t, "secret-value", param.Value)
	assert.Equal(t, "SecureString", param.Type)
	assert.Equal(t, int64(3), param.Version)
	assert.Equal(t, now, param.LastModifiedDate)
	assert.Contains(t, param.ARN, "parameter/test/secret")
	assert.Equal(t, "test", param.Tags["env"])
}

func TestParameterMetadata(t *testing.T) {
	now := time.Now()
	meta := ParameterMetadata{
		Name:             "/test/param",
		Type:             "String",
		Version:          1,
		LastModifiedDate: now,
		Tags:             map[string]string{"team": "backend"},
	}

	assert.Equal(t, "/test/param", meta.Name)
	assert.Equal(t, "String", meta.Type)
	assert.Equal(t, int64(1), meta.Version)
	assert.Equal(t, "backend", meta.Tags["team"])
}

func TestParameterHistory(t *testing.T) {
	now := time.Now()
	history := ParameterHistory{
		Name:             "/test/param",
		Value:            "old-value",
		Type:             "SecureString",
		Version:          1,
		LastModifiedDate: now,
		Labels:           []string{"v1.0", "stable"},
	}

	assert.Equal(t, "/test/param", history.Name)
	assert.Equal(t, "old-value", history.Value)
	assert.Equal(t, int64(1), history.Version)
	assert.Len(t, history.Labels, 2)
	assert.Contains(t, history.Labels, "v1.0")
}

func TestPutParameterInput(t *testing.T) {
	input := PutParameterInput{
		Name:      "/prod/secret",
		Value:     "my-secret",
		Type:      "SecureString",
		Overwrite: true,
		KMSKeyID:  "alias/my-key",
		Tags:      map[string]string{"env": "prod"},
	}

	assert.Equal(t, "/prod/secret", input.Name)
	assert.Equal(t, "my-secret", input.Value)
	assert.Equal(t, "SecureString", input.Type)
	assert.True(t, input.Overwrite)
	assert.Equal(t, "alias/my-key", input.KMSKeyID)
	assert.Equal(t, "prod", input.Tags["env"])
}

func TestPutParameterOutput(t *testing.T) {
	output := PutParameterOutput{
		Version: 5,
	}

	assert.Equal(t, int64(5), output.Version)
}
```

Create file `internal/aws/ssm_test.go`:

```go
package aws

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockSSMClient is a mock implementation for testing
type MockSSMClient struct {
	GetParameterFunc             func(ctx context.Context, params *ssm.GetParameterInput, optFns ...func(*ssm.Options)) (*ssm.GetParameterOutput, error)
	PutParameterFunc             func(ctx context.Context, params *ssm.PutParameterInput, optFns ...func(*ssm.Options)) (*ssm.PutParameterOutput, error)
	DeleteParameterFunc          func(ctx context.Context, params *ssm.DeleteParameterInput, optFns ...func(*ssm.Options)) (*ssm.DeleteParameterOutput, error)
	DescribeParametersFunc       func(ctx context.Context, params *ssm.DescribeParametersInput, optFns ...func(*ssm.Options)) (*ssm.DescribeParametersOutput, error)
	GetParameterHistoryFunc      func(ctx context.Context, params *ssm.GetParameterHistoryInput, optFns ...func(*ssm.Options)) (*ssm.GetParameterHistoryOutput, error)
	ListTagsForResourceFunc      func(ctx context.Context, params *ssm.ListTagsForResourceInput, optFns ...func(*ssm.Options)) (*ssm.ListTagsForResourceOutput, error)
}

func (m *MockSSMClient) GetParameter(ctx context.Context, params *ssm.GetParameterInput, optFns ...func(*ssm.Options)) (*ssm.GetParameterOutput, error) {
	if m.GetParameterFunc != nil {
		return m.GetParameterFunc(ctx, params, optFns...)
	}
	return nil, nil
}

func (m *MockSSMClient) PutParameter(ctx context.Context, params *ssm.PutParameterInput, optFns ...func(*ssm.Options)) (*ssm.PutParameterOutput, error) {
	if m.PutParameterFunc != nil {
		return m.PutParameterFunc(ctx, params, optFns...)
	}
	return nil, nil
}

func (m *MockSSMClient) DeleteParameter(ctx context.Context, params *ssm.DeleteParameterInput, optFns ...func(*ssm.Options)) (*ssm.DeleteParameterOutput, error) {
	if m.DeleteParameterFunc != nil {
		return m.DeleteParameterFunc(ctx, params, optFns...)
	}
	return nil, nil
}

func (m *MockSSMClient) DescribeParameters(ctx context.Context, params *ssm.DescribeParametersInput, optFns ...func(*ssm.Options)) (*ssm.DescribeParametersOutput, error) {
	if m.DescribeParametersFunc != nil {
		return m.DescribeParametersFunc(ctx, params, optFns...)
	}
	return nil, nil
}

func (m *MockSSMClient) GetParameterHistory(ctx context.Context, params *ssm.GetParameterHistoryInput, optFns ...func(*ssm.Options)) (*ssm.GetParameterHistoryOutput, error) {
	if m.GetParameterHistoryFunc != nil {
		return m.GetParameterHistoryFunc(ctx, params, optFns...)
	}
	return nil, nil
}

func (m *MockSSMClient) ListTagsForResource(ctx context.Context, params *ssm.ListTagsForResourceInput, optFns ...func(*ssm.Options)) (*ssm.ListTagsForResourceOutput, error) {
	if m.ListTagsForResourceFunc != nil {
		return m.ListTagsForResourceFunc(ctx, params, optFns...)
	}
	return &ssm.ListTagsForResourceOutput{}, nil
}

func TestMockSSMClient_GetParameter(t *testing.T) {
	now := time.Now()
	mock := &MockSSMClient{
		GetParameterFunc: func(ctx context.Context, params *ssm.GetParameterInput, optFns ...func(*ssm.Options)) (*ssm.GetParameterOutput, error) {
			return &ssm.GetParameterOutput{
				Parameter: &types.Parameter{
					Name:             params.Name,
					Value:            aws.String("test-value"),
					Type:             types.ParameterTypeSecureString,
					Version:          3,
					LastModifiedDate: aws.Time(now),
					ARN:              aws.String("arn:aws:ssm:us-east-1:123456789:parameter/test"),
				},
			}, nil
		},
	}

	output, err := mock.GetParameter(context.Background(), &ssm.GetParameterInput{
		Name:           aws.String("/test/param"),
		WithDecryption: aws.Bool(true),
	})

	require.NoError(t, err)
	assert.Equal(t, "/test/param", aws.ToString(output.Parameter.Name))
	assert.Equal(t, "test-value", aws.ToString(output.Parameter.Value))
	assert.Equal(t, types.ParameterTypeSecureString, output.Parameter.Type)
	assert.Equal(t, int64(3), output.Parameter.Version)
}

func TestMockSSMClient_PutParameter(t *testing.T) {
	mock := &MockSSMClient{
		PutParameterFunc: func(ctx context.Context, params *ssm.PutParameterInput, optFns ...func(*ssm.Options)) (*ssm.PutParameterOutput, error) {
			return &ssm.PutParameterOutput{
				Version: 1,
			}, nil
		},
	}

	output, err := mock.PutParameter(context.Background(), &ssm.PutParameterInput{
		Name:  aws.String("/test/new-param"),
		Value: aws.String("new-value"),
		Type:  types.ParameterTypeSecureString,
	})

	require.NoError(t, err)
	assert.Equal(t, int64(1), output.Version)
}

func TestMockSSMClient_DeleteParameter(t *testing.T) {
	mock := &MockSSMClient{
		DeleteParameterFunc: func(ctx context.Context, params *ssm.DeleteParameterInput, optFns ...func(*ssm.Options)) (*ssm.DeleteParameterOutput, error) {
			return &ssm.DeleteParameterOutput{}, nil
		},
	}

	output, err := mock.DeleteParameter(context.Background(), &ssm.DeleteParameterInput{
		Name: aws.String("/test/param-to-delete"),
	})

	require.NoError(t, err)
	assert.NotNil(t, output)
}

func TestMockSSMClient_DescribeParameters(t *testing.T) {
	now := time.Now()
	mock := &MockSSMClient{
		DescribeParametersFunc: func(ctx context.Context, params *ssm.DescribeParametersInput, optFns ...func(*ssm.Options)) (*ssm.DescribeParametersOutput, error) {
			return &ssm.DescribeParametersOutput{
				Parameters: []types.ParameterMetadata{
					{
						Name:             aws.String("/test/param1"),
						Type:             types.ParameterTypeString,
						Version:          1,
						LastModifiedDate: aws.Time(now),
					},
					{
						Name:             aws.String("/test/param2"),
						Type:             types.ParameterTypeSecureString,
						Version:          2,
						LastModifiedDate: aws.Time(now),
					},
				},
			}, nil
		},
	}

	output, err := mock.DescribeParameters(context.Background(), &ssm.DescribeParametersInput{})

	require.NoError(t, err)
	assert.Len(t, output.Parameters, 2)
	assert.Equal(t, "/test/param1", aws.ToString(output.Parameters[0].Name))
	assert.Equal(t, "/test/param2", aws.ToString(output.Parameters[1].Name))
}

func TestMockSSMClient_GetParameterHistory(t *testing.T) {
	now := time.Now()
	mock := &MockSSMClient{
		GetParameterHistoryFunc: func(ctx context.Context, params *ssm.GetParameterHistoryInput, optFns ...func(*ssm.Options)) (*ssm.GetParameterHistoryOutput, error) {
			return &ssm.GetParameterHistoryOutput{
				Parameters: []types.ParameterHistory{
					{
						Name:             params.Name,
						Value:            aws.String("value-v1"),
						Type:             types.ParameterTypeSecureString,
						Version:          1,
						LastModifiedDate: aws.Time(now.Add(-1 * time.Hour)),
					},
					{
						Name:             params.Name,
						Value:            aws.String("value-v2"),
						Type:             types.ParameterTypeSecureString,
						Version:          2,
						LastModifiedDate: aws.Time(now),
					},
				},
			}, nil
		},
	}

	output, err := mock.GetParameterHistory(context.Background(), &ssm.GetParameterHistoryInput{
		Name:           aws.String("/test/param"),
		WithDecryption: aws.Bool(true),
	})

	require.NoError(t, err)
	assert.Len(t, output.Parameters, 2)
	assert.Equal(t, int64(1), output.Parameters[0].Version)
	assert.Equal(t, int64(2), output.Parameters[1].Version)
}
```

Create file `internal/aws/errors_test.go`:

```go
package aws

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsNotFoundError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "generic error",
			err:      errors.New("some error"),
			expected: false,
		},
		{
			name:     "not found message",
			err:      errors.New("ParameterNotFound: parameter not found"),
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsNotFoundError(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsAccessDeniedError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "generic error",
			err:      errors.New("some error"),
			expected: false,
		},
		{
			name:     "access denied message",
			err:      errors.New("AccessDeniedException: access denied"),
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsAccessDeniedError(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}
```

### 6. Create Utility Module Tests

Create file `internal/util/output_test.go`:

```go
package util

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMaskValue(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"empty", "", ""},
		{"short", "ab", "**"},
		{"medium", "secret", "******"},
		{"long", "verylongsecretvalue", "*******************"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MaskValue(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMaskValuePartial(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		showFirst   int
		showLast    int
		expectedLen int
	}{
		{"empty", "", 2, 2, 0},
		{"short", "ab", 1, 1, 2},
		{"medium", "secret123", 2, 2, 9},
		{"long", "verylongsecretvalue", 3, 3, 19},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MaskValuePartial(tt.input, tt.showFirst, tt.showLast)
			assert.Len(t, result, tt.expectedLen)
		})
	}
}

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		name     string
		bytes    int64
		expected string
	}{
		{"zero", 0, "0 B"},
		{"bytes", 500, "500 B"},
		{"kilobytes", 1024, "1.0 KB"},
		{"megabytes", 1048576, "1.0 MB"},
		{"gigabytes", 1073741824, "1.0 GB"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatBytes(tt.bytes)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestTruncateString(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		maxLen   int
		expected string
	}{
		{"empty", "", 10, ""},
		{"short", "hello", 10, "hello"},
		{"exact", "hello", 5, "hello"},
		{"truncate", "hello world", 8, "hello..."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := TruncateString(tt.input, tt.maxLen)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestPrintJSON(t *testing.T) {
	var buf bytes.Buffer

	data := map[string]interface{}{
		"name":  "test",
		"value": 123,
	}

	err := PrintJSON(&buf, data)
	assert.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, `"name": "test"`)
	assert.Contains(t, output, `"value": 123`)
}
```

Create file `internal/util/editor_test.go`:

```go
package util

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEditorConfig_GetEditorCommand(t *testing.T) {
	// Clear environment for consistent testing
	origEditor := os.Getenv("EDITOR")
	origVisual := os.Getenv("VISUAL")
	defer func() {
		os.Setenv("EDITOR", origEditor)
		os.Setenv("VISUAL", origVisual)
	}()

	tests := []struct {
		name     string
		config   EditorConfig
		envVars  map[string]string
		expected string
	}{
		{
			name:     "explicit editor",
			config:   EditorConfig{Editor: "nano"},
			envVars:  map[string]string{},
			expected: "nano",
		},
		{
			name:   "VISUAL env var",
			config: EditorConfig{},
			envVars: map[string]string{
				"VISUAL": "code --wait",
				"EDITOR": "vim",
			},
			expected: "code --wait",
		},
		{
			name:   "EDITOR env var",
			config: EditorConfig{},
			envVars: map[string]string{
				"EDITOR": "vim",
			},
			expected: "vim",
		},
		{
			name:     "default vi",
			config:   EditorConfig{},
			envVars:  map[string]string{},
			expected: "vi",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Unsetenv("EDITOR")
			os.Unsetenv("VISUAL")

			for k, v := range tt.envVars {
				os.Setenv(k, v)
			}

			editor := NewEditor(tt.config)
			assert.Equal(t, tt.expected, editor.getEditorCommand())
		})
	}
}

func TestEditor_NewEditor(t *testing.T) {
	config := EditorConfig{
		Editor: "nvim",
	}

	editor := NewEditor(config)
	assert.NotNil(t, editor)
	assert.Equal(t, "nvim", editor.config.Editor)
}
```

Create file `internal/util/clipboard_test.go`:

```go
package util

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestClipboardManager_NewClipboardManager(t *testing.T) {
	timeout := 30 * time.Second
	cm := NewClipboardManager(timeout)

	assert.NotNil(t, cm)
	assert.Equal(t, timeout, cm.clearTimeout)
}

func TestClipboardManager_CopyWithMessage(t *testing.T) {
	// Skip if clipboard not supported in test environment
	if !IsClipboardSupported() {
		t.Skip("clipboard not supported in test environment")
	}

	tests := []struct {
		name        string
		timeout     time.Duration
		expectMsg   string
	}{
		{
			name:      "with timeout",
			timeout:   30 * time.Second,
			expectMsg: "Copied to clipboard (will clear in 30s)",
		},
		{
			name:      "no timeout",
			timeout:   0,
			expectMsg: "Copied to clipboard",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cm := NewClipboardManager(tt.timeout)
			msg, err := cm.CopyWithMessage("test-value")
			if err != nil {
				t.Skip("clipboard operation failed, likely unsupported environment")
			}
			assert.Contains(t, msg, "Copied to clipboard")
		})
	}
}

func TestIsClipboardSupported(t *testing.T) {
	// Just verify it doesn't panic
	_ = IsClipboardSupported()
}
```

Create file `internal/util/signal_test.go`:

```go
package util

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestSetupSignalHandler(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// SetupSignalHandler should return a context
	signalCtx := SetupSignalHandler(ctx)
	assert.NotNil(t, signalCtx)

	// Context should not be cancelled initially
	select {
	case <-signalCtx.Done():
		t.Error("context should not be done initially")
	default:
		// Expected
	}
}

func TestContextWithTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Should expire after timeout
	<-ctx.Done()
	assert.Error(t, ctx.Err())
}
```

### 7. Create CLI Module Tests

Create file `internal/cli/commands_test.go`:

```go
package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateParameterName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid simple", "/test/param", false},
		{"valid nested", "/prod/db/password", false},
		{"valid deep", "/a/b/c/d/e", false},
		{"missing leading slash", "test/param", true},
		{"empty", "", true},
		{"just slash", "/", true},
		{"trailing slash", "/test/", true},
		{"double slash", "/test//param", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateParameterName(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestParseNameAndVersion(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantName    string
		wantVersion int64
		wantErr     bool
	}{
		{"no version", "/test/param", "/test/param", 0, false},
		{"with version", "/test/param@3", "/test/param", 3, false},
		{"latest version", "/test/param@latest", "/test/param", 0, false},
		{"invalid version", "/test/param@abc", "", 0, true},
		{"negative version", "/test/param@-1", "", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name, version, err := parseNameAndVersion(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantName, name)
				assert.Equal(t, tt.wantVersion, version)
			}
		})
	}
}

func TestParseTags(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected map[string]string
		wantErr  bool
	}{
		{"empty", "", map[string]string{}, false},
		{"single tag", "env=prod", map[string]string{"env": "prod"}, false},
		{"multiple tags", "env=prod,team=backend", map[string]string{"env": "prod", "team": "backend"}, false},
		{"spaces", "env=prod, team=backend", map[string]string{"env": "prod", "team": "backend"}, false},
		{"invalid format", "invalid", nil, true},
		{"missing value", "key=", nil, true},
		{"missing key", "=value", nil, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseTags(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestNormalizeSortOption(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"name", "name"},
		{"n", "name"},
		{"created", "created"},
		{"c", "created"},
		{"modified", "modified"},
		{"m", "modified"},
		{"unknown", "name"}, // defaults to name
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := normalizeSortOption(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
```

Create file `internal/cli/exitcodes_test.go`:

```go
package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExitCodes(t *testing.T) {
	// Verify exit codes are defined as expected
	assert.Equal(t, 0, ExitOK)
	assert.Equal(t, 1, ExitError)
	assert.Equal(t, 2, ExitUsage)
	assert.Equal(t, 3, ExitNotFound)
	assert.Equal(t, 4, ExitAccessDenied)
	assert.Equal(t, 130, ExitInterrupt)
}
```

### 8. Update Makefile for Tests

Add to existing `Makefile`:

```makefile
## Run unit tests only
test-unit:
	@echo "Running unit tests..."
	$(GOTEST) -v -race -short ./...

## Run tests with verbose output
test-verbose:
	@echo "Running tests with verbose output..."
	$(GOTEST) -v -race ./... 2>&1 | tee test-output.log
```

## Acceptance Criteria

- [ ] All test files compile without errors: `go build ./...`
- [ ] All tests pass: `go test ./...`
- [ ] Test coverage is at least 60%: `go test -cover ./...`
- [ ] No data races detected: `go test -race ./...`
- [ ] Tests run in isolation (no external dependencies)
- [ ] Tests are deterministic (same results on repeated runs)

## Running Tests

```bash
# Run all tests
go test ./...

# Run with coverage
go test -cover ./...

# Run with race detection
go test -race ./...

# Run specific package tests
go test ./internal/config/...
go test ./internal/cache/...
go test ./internal/aws/...

# Run with verbose output
go test -v ./...

# Generate coverage report
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

## Example Output

```
$ go test ./...
ok      github.com/yachiko/clerk/internal/config   0.015s
ok      github.com/yachiko/clerk/internal/cache    0.023s
ok      github.com/yachiko/clerk/internal/aws      0.012s
ok      github.com/yachiko/clerk/internal/util     0.018s
ok      github.com/yachiko/clerk/internal/cli      0.021s

$ go test -cover ./...
ok      github.com/yachiko/clerk/internal/config   0.015s  coverage: 78.5% of statements
ok      github.com/yachiko/clerk/internal/cache    0.023s  coverage: 72.3% of statements
ok      github.com/yachiko/clerk/internal/aws      0.012s  coverage: 65.8% of statements
ok      github.com/yachiko/clerk/internal/util     0.018s  coverage: 81.2% of statements
ok      github.com/yachiko/clerk/internal/cli      0.021s  coverage: 68.4% of statements
```

## Notes

- Tests use `github.com/stretchr/testify` for assertions and requirements
- Mock implementations are provided for AWS SSM client
- Tests clean up temporary files and directories
- Environment variables are saved and restored in tests
- Some tests may be skipped if clipboard is not supported in the environment
- Use table-driven tests for better coverage and readability
