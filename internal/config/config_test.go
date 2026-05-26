package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yachiko/clerk/internal/testutil"
)

func TestNewManager_FreshHome(t *testing.T) {
	home := testutil.IsolateHome(t)

	mgr, err := NewManager()
	require.NoError(t, err)
	require.NotNil(t, mgr)

	cfg := mgr.Get()
	assert.Equal(t, "us-east-1", cfg.Region)
	assert.Equal(t, "", cfg.Profile)
	// Default cache path is computed relative to HOME
	assert.Equal(t, filepath.Join(home, ".clerk", "cache.json"), cfg.CachePath)
}

func TestManager_SaveAndReload(t *testing.T) {
	home := testutil.IsolateHome(t)

	mgr, err := NewManager()
	require.NoError(t, err)

	cfg := mgr.Get()
	cfg.Region = "eu-west-1"
	cfg.Profile = "production"
	cfg.CacheTTL = 1 * time.Hour
	cfg.ParallelFetches = 25

	require.NoError(t, mgr.Save())

	configPath := filepath.Join(home, ".clerk", "config.json")
	assert.FileExists(t, configPath)
	info, err := os.Stat(configPath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0600), info.Mode().Perm())

	// Reload in a fresh manager
	mgr2, err := NewManager()
	require.NoError(t, err)

	cfg2 := mgr2.Get()
	assert.Equal(t, "eu-west-1", cfg2.Region)
	assert.Equal(t, "production", cfg2.Profile)
	assert.Equal(t, 1*time.Hour, cfg2.CacheTTL)
	assert.Equal(t, 25, cfg2.ParallelFetches)
}

func TestManager_GetValue(t *testing.T) {
	testutil.IsolateHome(t)
	mgr, err := NewManager()
	require.NoError(t, err)

	tests := []struct {
		key      string
		expected string
	}{
		{"region", "us-east-1"},
		{"profile", ""},
		{"default_type", "SecureString"},
		{"default.type", "SecureString"},
		{"default_sort", "name"},
		{"default.sort", "name"},
		{"parallel_fetches", "10"},
		{"cache_ttl", "3h0m0s"},
		{"clipboard_timeout", "1m0s"},
		{"search_slash_prefix", "true"},
		{"decrypt_by_default", "true"},
		{"describe_page_size", "50"},
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			got, err := mgr.GetValue(tt.key)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestManager_GetValue_UnknownKey(t *testing.T) {
	testutil.IsolateHome(t)
	mgr, err := NewManager()
	require.NoError(t, err)

	_, err = mgr.GetValue("nope")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown configuration key")
}

func TestManager_SetValue(t *testing.T) {
	testutil.IsolateHome(t)
	mgr, err := NewManager()
	require.NoError(t, err)

	tests := []struct {
		name     string
		key      string
		value    string
		checkKey string
		want     string
	}{
		{"region", "region", "ap-southeast-1", "region", "ap-southeast-1"},
		{"profile", "profile", "dev", "profile", "dev"},
		{"default_type", "default_type", "String", "default_type", "String"},
		{"cache_ttl", "cache_ttl", "2h", "cache_ttl", "2h0m0s"},
		{"clipboard_timeout", "clipboard_timeout", "30s", "clipboard_timeout", "30s"},
		{"parallel_fetches", "parallel_fetches", "20", "parallel_fetches", "20"},
		{"search_slash_prefix false", "search_slash_prefix", "false", "search_slash_prefix", "false"},
		{"decrypt_by_default false", "decrypt_by_default", "false", "decrypt_by_default", "false"},
		{"describe_page_size", "describe_page_size", "25", "describe_page_size", "25"},
		{"describe_max_items unlimited", "describe_max_items", "0", "describe_max_items", "0"},
		{"describe_version_batch_size", "describe_version_batch_size", "5", "describe_version_batch_size", "5"},
		{"sort alias", "default_sort", "modified", "default_sort", "modified"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.NoError(t, mgr.SetValue(tt.key, tt.value))
			got, err := mgr.GetValue(tt.checkKey)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestManager_SetValue_Validation(t *testing.T) {
	testutil.IsolateHome(t)
	mgr, err := NewManager()
	require.NoError(t, err)

	cases := []struct {
		name   string
		key    string
		value  string
		errSub string
	}{
		{"invalid type", "default_type", "Number", "invalid type"},
		{"invalid sort", "default_sort", "size", "invalid sort option"},
		{"bad duration", "cache_ttl", "forever", "invalid duration format"},
		{"bad bool", "search_slash_prefix", "maybe", "invalid boolean"},
		{"parallel too high", "parallel_fetches", "999", "between 1 and 50"},
		{"parallel zero", "parallel_fetches", "0", "between 1 and 50"},
		{"page size zero", "describe_page_size", "0", "between 1 and 50"},
		{"max items negative", "describe_max_items", "-1", "must be >= 0"},
		{"batch size zero", "describe_version_batch_size", "0", "must be >= 1"},
		{"unknown key", "blarg", "x", "unknown configuration key"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			err := mgr.SetValue(tt.key, tt.value)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.errSub)
		})
	}
}

func TestManager_ListKeys(t *testing.T) {
	testutil.IsolateHome(t)
	mgr, err := NewManager()
	require.NoError(t, err)

	keys := mgr.ListKeys()
	// Every key returned must be readable via GetValue
	for _, k := range keys {
		_, err := mgr.GetValue(k)
		assert.NoError(t, err, "key %q listed by ListKeys must be readable", k)
	}
	// And every settable key in our SetValue table should appear here.
	assert.Contains(t, keys, "region")
	assert.Contains(t, keys, "default_type")
	assert.Contains(t, keys, "describe_version_batch_size")
}
