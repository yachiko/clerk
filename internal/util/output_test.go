package util

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMaskValue(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"empty", "", ""},
		{"two", "ab", "**"},
		{"eight (fully masked)", "12345678", "********"},
		{"nine (partial)", "123456789", "12*****89"},
		{"long", "verylongsecretvalue", "ve***************ue"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MaskValue(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestMaskValueFull(t *testing.T) {
	assert.Equal(t, "", MaskValueFull(""))
	assert.Equal(t, "*****", MaskValueFull("hello"))
	assert.Equal(t, "********", MaskValueFull("12345678"))
}

func TestFormatter_NewFormatter_DefaultsToPlain(t *testing.T) {
	f := NewFormatter("unrecognized", &bytes.Buffer{})
	assert.Equal(t, OutputPlain, f.format)

	f = NewFormatter("JSON", &bytes.Buffer{})
	assert.Equal(t, OutputJSON, f.format)

	f = NewFormatter("plain", &bytes.Buffer{})
	assert.Equal(t, OutputPlain, f.format)
}

func TestFormatter_Print_JSON(t *testing.T) {
	var buf bytes.Buffer
	f := NewFormatter("json", &buf)

	data := map[string]any{"name": "test", "value": 123}
	require.NoError(t, f.Print(data))

	// Output must be valid JSON containing both fields.
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &decoded))
	assert.Equal(t, "test", decoded["name"])
	assert.EqualValues(t, 123, decoded["value"])
}

func TestFormatter_Print_Plain(t *testing.T) {
	var buf bytes.Buffer
	f := NewFormatter("plain", &buf)

	require.NoError(t, f.Print("hello"))
	assert.Equal(t, "hello\n", buf.String())
}

func TestFormatter_StyledHelpers_SuppressedInJSON(t *testing.T) {
	// In JSON mode, the colored helpers must write nothing — they would corrupt
	// the JSON stream otherwise. They use stdout directly via fatih/color, so we
	// can only assert that the call returns without panic and that the buffer is
	// untouched.
	var buf bytes.Buffer
	f := NewFormatter("json", &buf)

	f.PrintSuccess("ok %s", "yes")
	f.PrintError("nope %d", 1)
	f.PrintWarning("watch")
	f.PrintInfo("fyi")

	assert.Empty(t, buf.Bytes())
}
