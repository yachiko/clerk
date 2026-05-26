package util

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewClipboardManager(t *testing.T) {
	cm := NewClipboardManager(30 * time.Second)
	require.NotNil(t, cm)
	assert.Equal(t, 30*time.Second, cm.clearTimeout)
}

func TestIsClipboardSupported_DoesNotPanic(t *testing.T) {
	// In headless CI the answer is "false"; locally it may be true. Either is
	// acceptable — we just verify the call returns a bool without panicking.
	_ = IsClipboardSupported()
}

func TestClipboardManager_CopyWithMessage(t *testing.T) {
	if !IsClipboardSupported() {
		t.Skip("clipboard not supported in this environment")
	}

	t.Run("with timeout", func(t *testing.T) {
		cm := NewClipboardManager(30 * time.Second)
		msg, err := cm.CopyWithMessage("test-value")
		if err != nil {
			t.Skipf("clipboard write failed (likely no display): %v", err)
		}
		assert.True(t, strings.HasPrefix(msg, "Copied to clipboard"))
		assert.Contains(t, msg, "30s")
	})

	t.Run("no timeout", func(t *testing.T) {
		cm := NewClipboardManager(0)
		msg, err := cm.CopyWithMessage("test-value")
		if err != nil {
			t.Skipf("clipboard write failed: %v", err)
		}
		assert.Equal(t, "Copied to clipboard", msg)
	})
}
