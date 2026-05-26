package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseTags(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    map[string]string
		wantErr string
	}{
		{"empty returns nil", "", nil, ""},
		{"single", "env=prod", map[string]string{"env": "prod"}, ""},
		{"multiple", "env=prod,team=backend", map[string]string{"env": "prod", "team": "backend"}, ""},
		{"trim whitespace", "  env = prod ,  team = backend  ", map[string]string{"env": "prod", "team": "backend"}, ""},
		{"empty pair skipped", "env=prod,,team=be", map[string]string{"env": "prod", "team": "be"}, ""},
		{"value may contain =", "url=https://x?a=b", map[string]string{"url": "https://x?a=b"}, ""},
		{"no equals", "novalue", nil, "invalid tag format"},
		{"empty key", "=value", nil, "empty tag key"},
		{"empty value allowed", "key=", map[string]string{"key": ""}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseTags(tt.input)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestNormalizeSortOption(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"name", "name"},
		{"NAME", "name"},
		{"n", "name"},
		{"created", "created"},
		{"c", "created"},
		{"modified", "modified"},
		{"m", "modified"},
		{"unknown", "name"},
		{"", "name"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.want, normalizeSortOption(tt.input))
		})
	}
}

func TestParseNameVersionLabel(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantName    string
		wantVersion int64
		wantLabel   string
		wantErr     string
	}{
		{"plain name", "/dev/db", "/dev/db", 0, "", ""},
		{"with version", "/dev/db@3", "/dev/db", 3, "", ""},
		{"with @latest", "/dev/db@latest", "/dev/db", 0, "", ""},
		{"with LATEST (case insensitive)", "/dev/db@LATEST", "/dev/db", 0, "", ""},
		{"with label", "/dev/db:prod", "/dev/db", 0, "prod", ""},
		{"version not a number", "/dev/db@abc", "", 0, "", "invalid version"},
		{"version zero", "/dev/db@0", "", 0, "", "positive"},
		{"version negative", "/dev/db@-2", "", 0, "", "positive"},
		{"empty label", "/dev/db:", "", 0, "", "label cannot be empty"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name, version, label, err := parseNameVersionLabel(tt.input)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantName, name)
			assert.Equal(t, tt.wantVersion, version)
			assert.Equal(t, tt.wantLabel, label)
		})
	}
}

func TestMatchPath(t *testing.T) {
	tests := []struct {
		pattern string
		name    string
		want    bool
	}{
		{"", "/anything", true},
		{"/", "/anything", true},
		{"/*", "/anything", true},
		{"/prod/*", "/prod/x", true},
		{"/prod/*", "/prod", true},
		{"/prod/*", "/dev/x", false},
		{"*pass*", "/db/password", true},
		{"*pass*", "/api/key", false},
		{"/prod*", "/prod/x", true},
		{"/prod*", "/dev/x", false},
		{"/exact", "/exact", true},
		{"/exact", "/different", false},
	}
	for _, tt := range tests {
		t.Run(tt.pattern+"_vs_"+tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, matchPath(tt.pattern, tt.name))
		})
	}
}

func TestExtractBasePath(t *testing.T) {
	tests := []struct {
		pattern string
		want    string
	}{
		{"/prod/*", "/prod"},
		{"/prod/db/*", "/prod/db"},
		{"/prod", "/prod"},
		{"/exact/path", "/exact/path"},
		{"*", "*"},  // idx==0 short-circuits; pattern returned as-is
		{"/*", "/"}, // idx==1 → base="/" → trimmed to "" → returns "/"
	}
	for _, tt := range tests {
		t.Run(tt.pattern, func(t *testing.T) {
			assert.Equal(t, tt.want, extractBasePath(tt.pattern))
		})
	}
}

func TestIsValidParamType(t *testing.T) {
	for _, ok := range []string{"String", "StringList", "SecureString"} {
		assert.True(t, isValidParamType(ok), ok)
	}
	for _, bad := range []string{"string", "SECURE", "Number", "", "bool"} {
		assert.False(t, isValidParamType(bad), bad)
	}
}

func TestFormatTags(t *testing.T) {
	// Map iteration order is unstable in Go; test deterministic single-entry
	// case and the multi-entry case via length / contains checks.
	assert.Equal(t, "env=prod", formatTags(map[string]string{"env": "prod"}))
	assert.Equal(t, "", formatTags(nil))

	got := formatTags(map[string]string{"env": "prod", "team": "be"})
	assert.Contains(t, got, "env=prod")
	assert.Contains(t, got, "team=be")
	assert.Contains(t, got, ", ")
}
