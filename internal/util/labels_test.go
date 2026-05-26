package util

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateLabel(t *testing.T) {
	cases := []struct {
		name    string
		label   string
		wantErr string
	}{
		{"empty", "", "empty"},
		{"valid simple", "prod", ""},
		{"valid with dash", "rollback-point", ""},
		{"valid with dot", "v1.2.3", ""},
		{"valid with underscore", "last_known_good", ""},
		{"reserved prefix lower", "aws:foo", "reserved prefix"},
		{"reserved prefix mixed case", "AWS:foo", "reserved prefix"},
		{"invalid char space", "bad label", "invalid characters"},
		{"invalid char slash", "bad/label", "invalid characters"},
		{"too long", strings.Repeat("a", MaxLabelLength+1), "maximum length"},
		{"max length exact", strings.Repeat("a", MaxLabelLength), ""},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateLabel(tt.label)
			if tt.wantErr == "" {
				assert.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			}
		})
	}
}

func TestValidateLabels(t *testing.T) {
	assert.NoError(t, ValidateLabels([]string{"prod", "stable"}))

	// Duplicate
	err := ValidateLabels([]string{"prod", "prod"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate")

	// Too many
	tooMany := make([]string, MaxLabelsPerVersion+1)
	for i := range tooMany {
		tooMany[i] = "label" + string(rune('a'+i))
	}
	err = ValidateLabels(tooMany)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "more than")

	// Inner label invalid: ValidateLabel error surfaces.
	err = ValidateLabels([]string{"ok", "aws:bad"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reserved prefix")
}

func TestSuggestLabels(t *testing.T) {
	got := SuggestLabels()
	assert.NotEmpty(t, got)
	assert.Contains(t, got, "prod")
	assert.Contains(t, got, "staging")
	// Every suggestion must itself be a valid label.
	for _, l := range got {
		assert.NoError(t, ValidateLabel(l), "suggested label %q must validate", l)
	}
}
