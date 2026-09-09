package util

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/atotto/clipboard"
)

type clipboardBackend interface {
	WriteAll(string) error
	ReadAll() (string, error)
}
type systemClipboard struct{}

func (systemClipboard) WriteAll(value string) error { return clipboard.WriteAll(value) }
func (systemClipboard) ReadAll() (string, error)    { return clipboard.ReadAll() }

// ClipboardManager clears only the exact value it last copied. Clipboard ownership is best-effort because desktop clipboards have no atomic compare-and-clear operation.
type ClipboardManager struct {
	clearTimeout time.Duration
	backend      clipboardBackend
	after        func(time.Duration) <-chan time.Time
	cancelFunc   context.CancelFunc
	generation   uint64
	owned        string
	closed       bool
	mu           sync.Mutex
}

func NewClipboardManager(clearTimeout time.Duration) *ClipboardManager {
	return newClipboardManager(clearTimeout, systemClipboard{}, time.After)
}
func newClipboardManager(clearTimeout time.Duration, backend clipboardBackend, after func(time.Duration) <-chan time.Time) *ClipboardManager {
	return &ClipboardManager{clearTimeout: clearTimeout, backend: backend, after: after}
}

// Copy writes a value and schedules an ownership-checked expiry. A failed new write leaves prior ownership intact.
func (cm *ClipboardManager) Copy(text string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	if cm.closed {
		return fmt.Errorf("clipboard manager is closed")
	}
	if err := cm.backend.WriteAll(text); err != nil {
		return fmt.Errorf("failed to copy to clipboard: %w", err)
	}
	if cm.cancelFunc != nil {
		cm.cancelFunc()
	}
	cm.generation++
	cm.owned = text
	generation := cm.generation
	if cm.clearTimeout > 0 {
		ctx, cancel := context.WithCancel(context.Background())
		cm.cancelFunc = cancel
		go func() {
			select {
			case <-cm.after(cm.clearTimeout):
				_ = cm.clearOwned(generation)
			case <-ctx.Done():
			}
		}()
	}
	return nil
}
func (cm *ClipboardManager) CopyWithMessage(text string) (string, error) {
	if err := cm.Copy(text); err != nil {
		return "", err
	}
	if cm.clearTimeout > 0 {
		return fmt.Sprintf("Copied to clipboard (will clear in %s)", cm.clearTimeout), nil
	}
	return "Copied to clipboard", nil
}

func (cm *ClipboardManager) clearOwned(generation uint64) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	if generation != cm.generation || cm.owned == "" {
		return nil
	}
	value, err := cm.backend.ReadAll()
	if err != nil {
		return fmt.Errorf("read clipboard before clear: %w", err)
	}
	if value != cm.owned {
		return nil
	}
	if err := cm.backend.WriteAll(""); err != nil {
		return fmt.Errorf("clear clipboard: %w", err)
	}
	cm.owned = ""
	return nil
}

// Clear clears only a value still owned by this manager.
func (cm *ClipboardManager) Clear() error {
	cm.mu.Lock()
	generation := cm.generation
	cm.mu.Unlock()
	return cm.clearOwned(generation)
}
func (cm *ClipboardManager) Read() (string, error) { return cm.backend.ReadAll() }

// Close cancels expiry and clears Clerk's current value if it still owns the clipboard.
func (cm *ClipboardManager) Close() error {
	cm.mu.Lock()
	if cm.closed {
		cm.mu.Unlock()
		return nil
	}
	cm.closed = true
	if cm.cancelFunc != nil {
		cm.cancelFunc()
	}
	generation := cm.generation
	cm.mu.Unlock()
	return cm.clearOwned(generation)
}
func IsClipboardSupported() bool { return !clipboard.Unsupported }
