package util

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/atotto/clipboard"
)

// ClipboardManager handles clipboard operations with auto-clear
type ClipboardManager struct {
	clearTimeout time.Duration
	cancelFunc   context.CancelFunc
	mu           sync.Mutex
}

// NewClipboardManager creates a new clipboard manager
func NewClipboardManager(clearTimeout time.Duration) *ClipboardManager {
	return &ClipboardManager{
		clearTimeout: clearTimeout,
	}
}

// Copy copies text to clipboard and schedules auto-clear
func (cm *ClipboardManager) Copy(text string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if cm.cancelFunc != nil {
		cm.cancelFunc()
	}

	if err := clipboard.WriteAll(text); err != nil {
		return fmt.Errorf("failed to copy to clipboard: %w", err)
	}

	if cm.clearTimeout > 0 {
		ctx, cancel := context.WithCancel(context.Background())
		cm.cancelFunc = cancel

		go func() {
			select {
			case <-time.After(cm.clearTimeout):
				_ = cm.Clear()
			case <-ctx.Done():
			}
		}()
	}

	return nil
}

// CopyWithMessage copies text and returns a message
func (cm *ClipboardManager) CopyWithMessage(text string) (string, error) {
	if err := cm.Copy(text); err != nil {
		return "", err
	}

	if cm.clearTimeout > 0 {
		return fmt.Sprintf("Copied to clipboard (will clear in %s)", cm.clearTimeout), nil
	}
	return "Copied to clipboard", nil
}

// Clear clears the clipboard
func (cm *ClipboardManager) Clear() error {
	return clipboard.WriteAll("")
}

// Read reads from clipboard
func (cm *ClipboardManager) Read() (string, error) {
	return clipboard.ReadAll()
}

// IsSupported checks if clipboard operations are supported
func IsClipboardSupported() bool {
	return clipboard.Unsupported == false
}
