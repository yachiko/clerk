package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	assert.Equal(t, "us-east-1", cfg.Region)
	assert.Equal(t, "", cfg.Profile) // empty = SDK default (avoids forcing shared-config lookup)
	assert.Equal(t, "", cfg.CachePath)
	assert.Equal(t, 3*time.Hour, cfg.CacheTTL)
	assert.Equal(t, 60*time.Second, cfg.ClipboardTimeout)
	assert.Equal(t, "SecureString", cfg.DefaultType)
	assert.Equal(t, "name", cfg.DefaultSort)
	assert.Equal(t, 10, cfg.ParallelFetches)
	assert.True(t, cfg.SearchSlashPrefix)
	assert.Equal(t, int32(50), cfg.DescribePageSize)
	assert.Equal(t, int32(0), cfg.DescribeMaxItems)
	assert.Equal(t, 10, cfg.DescribeVersionBatchSize)
	assert.True(t, cfg.DecryptByDefault)
	assert.True(t, cfg.BrowseAutoRefresh)
	assert.Equal(t, 5*time.Minute, cfg.BrowseRefreshCooldown)
}

func TestValidTypes(t *testing.T) {
	types := ValidTypes()
	assert.ElementsMatch(t, []string{"String", "StringList", "SecureString"}, types)
}

func TestValidSortOptions(t *testing.T) {
	opts := ValidSortOptions()
	assert.ElementsMatch(t, []string{"name", "created", "modified", "n", "c", "m"}, opts)
}

func TestIsValidType(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"String", "String", true},
		{"SecureString", "SecureString", true},
		{"StringList", "StringList", true},
		{"case-insensitive", "securestring", true},
		{"invalid", "Number", false},
		{"empty", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isValidType(tt.input))
		})
	}
}

func TestIsValidSort(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"name", true},
		{"n", true},
		{"created", true},
		{"c", true},
		{"modified", true},
		{"m", true},
		{"NAME", true},
		{"version", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.want, isValidSort(tt.input))
		})
	}
}
