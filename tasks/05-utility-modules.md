# Task 05: Utility Modules (Clipboard & Editor)

## Objective
Implement utility modules for clipboard operations with auto-clear and external editor integration.

## Prerequisites
- Task 01 completed (project setup)
- Task 02 completed (configuration module)

## Deliverables

### 1. Create Clipboard Utility

Create file `internal/util/clipboard.go`:

```go
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

	// Cancel any pending clear operation
	if cm.cancelFunc != nil {
		cm.cancelFunc()
	}

	// Copy to clipboard
	if err := clipboard.WriteAll(text); err != nil {
		return fmt.Errorf("failed to copy to clipboard: %w", err)
	}

	// Schedule auto-clear
	if cm.clearTimeout > 0 {
		ctx, cancel := context.WithCancel(context.Background())
		cm.cancelFunc = cancel

		go func() {
			select {
			case <-time.After(cm.clearTimeout):
				cm.Clear()
			case <-ctx.Done():
				// Cancelled, don't clear
			}
		}()
	}

	return nil
}

// CopyWithMessage copies text and returns a message indicating clear timeout
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
```

### 2. Create Editor Utility

Create file `internal/util/editor.go`:

```go
package util

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// EditorConfig configures the editor
type EditorConfig struct {
	PreferredEditor string // Override for $EDITOR
	TempDir         string // Custom temp directory
}

// Editor handles external editor operations
type Editor struct {
	config EditorConfig
}

// NewEditor creates a new editor utility
func NewEditor(config EditorConfig) *Editor {
	return &Editor{config: config}
}

// Edit opens content in an external editor and returns the modified content
// The temp file is securely deleted after editing
func (e *Editor) Edit(content string, extension string) (string, error) {
	// Create secure temp file
	tempPath, err := e.createSecureTempFile(content, extension)
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}

	// Ensure cleanup
	defer e.secureDelete(tempPath)

	// Get editor command
	editor := e.getEditor()
	if editor == "" {
		return "", fmt.Errorf("no editor found: set $EDITOR environment variable")
	}

	// Open editor
	if err := e.openEditor(editor, tempPath); err != nil {
		return "", fmt.Errorf("failed to open editor: %w", err)
	}

	// Read modified content
	modified, err := os.ReadFile(tempPath)
	if err != nil {
		return "", fmt.Errorf("failed to read modified content: %w", err)
	}

	return string(modified), nil
}

// createSecureTempFile creates a temp file with restricted permissions
func (e *Editor) createSecureTempFile(content string, extension string) (string, error) {
	tempDir := e.config.TempDir
	if tempDir == "" {
		tempDir = os.TempDir()
	}

	// Generate random filename
	randBytes := make([]byte, 16)
	if _, err := rand.Read(randBytes); err != nil {
		return "", err
	}
	filename := fmt.Sprintf("clerk-%s%s", hex.EncodeToString(randBytes), extension)
	tempPath := filepath.Join(tempDir, filename)

	// Create file with restricted permissions (owner read/write only)
	f, err := os.OpenFile(tempPath, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0600)
	if err != nil {
		return "", err
	}
	defer f.Close()

	if _, err := f.WriteString(content); err != nil {
		os.Remove(tempPath)
		return "", err
	}

	return tempPath, nil
}

// secureDelete overwrites and removes a file
func (e *Editor) secureDelete(path string) error {
	// Get file info
	info, err := os.Stat(path)
	if err != nil {
		return err
	}

	// Overwrite with zeros
	f, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		return os.Remove(path) // At least try to remove
	}

	zeros := make([]byte, info.Size())
	f.Write(zeros)
	f.Sync()
	f.Close()

	// Remove file
	return os.Remove(path)
}

// getEditor returns the editor command to use
func (e *Editor) getEditor() string {
	if e.config.PreferredEditor != "" {
		return e.config.PreferredEditor
	}

	// Check environment variables
	if editor := os.Getenv("EDITOR"); editor != "" {
		return editor
	}
	if editor := os.Getenv("VISUAL"); editor != "" {
		return editor
	}

	// Platform defaults
	switch runtime.GOOS {
	case "darwin":
		// Check for VS Code
		if _, err := exec.LookPath("code"); err == nil {
			return "code --wait"
		}
		return "nano"
	case "windows":
		return "notepad"
	default:
		// Linux/other Unix
		if _, err := exec.LookPath("nano"); err == nil {
			return "nano"
		}
		if _, err := exec.LookPath("vi"); err == nil {
			return "vi"
		}
		return ""
	}
}

// openEditor opens the editor and waits for it to close
func (e *Editor) openEditor(editor, filePath string) error {
	parts := strings.Fields(editor)
	if len(parts) == 0 {
		return fmt.Errorf("empty editor command")
	}

	cmdName := parts[0]
	args := append(parts[1:], filePath)

	cmd := exec.Command(cmdName, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

// GetEditorName returns the name of the editor that will be used
func (e *Editor) GetEditorName() string {
	editor := e.getEditor()
	if editor == "" {
		return "none"
	}
	parts := strings.Fields(editor)
	return filepath.Base(parts[0])
}
```

### 3. Create Output Formatter

Create file `internal/util/output.go`:

```go
package util

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/fatih/color"
)

// OutputFormat represents the output format
type OutputFormat string

const (
	OutputPlain OutputFormat = "plain"
	OutputJSON  OutputFormat = "json"
)

// Formatter handles output formatting
type Formatter struct {
	format OutputFormat
	writer io.Writer
}

// NewFormatter creates a new output formatter
func NewFormatter(format string, writer io.Writer) *Formatter {
	f := OutputFormat(strings.ToLower(format))
	if f != OutputJSON {
		f = OutputPlain
	}
	return &Formatter{
		format: f,
		writer: writer,
	}
}

// Print outputs data in the configured format
func (f *Formatter) Print(data any) error {
	if f.format == OutputJSON {
		return f.printJSON(data)
	}
	return f.printPlain(data)
}

func (f *Formatter) printJSON(data any) error {
	encoder := json.NewEncoder(f.writer)
	encoder.SetIndent("", "  ")
	return encoder.Encode(data)
}

func (f *Formatter) printPlain(data any) error {
	_, err := fmt.Fprintln(f.writer, data)
	return err
}

// PrintSuccess prints a success message
func (f *Formatter) PrintSuccess(format string, args ...any) {
	if f.format == OutputJSON {
		return // Skip decorative messages in JSON mode
	}
	color.Green(format, args...)
}

// PrintError prints an error message
func (f *Formatter) PrintError(format string, args ...any) {
	if f.format == OutputJSON {
		return // Skip decorative messages in JSON mode
	}
	color.Red(format, args...)
}

// PrintWarning prints a warning message
func (f *Formatter) PrintWarning(format string, args ...any) {
	if f.format == OutputJSON {
		return
	}
	color.Yellow(format, args...)
}

// PrintInfo prints an info message
func (f *Formatter) PrintInfo(format string, args ...any) {
	if f.format == OutputJSON {
		return
	}
	color.Cyan(format, args...)
}

// MaskValue masks a secret value, showing only first and last 2 characters
func MaskValue(value string) string {
	if len(value) <= 8 {
		return strings.Repeat("*", len(value))
	}
	return value[:2] + strings.Repeat("*", len(value)-4) + value[len(value)-2:]
}

// MaskValueFull returns a fully masked value
func MaskValueFull(value string) string {
	return strings.Repeat("*", len(value))
}
```

### 4. Create Signal Handler

Create file `internal/util/signal.go`:

```go
package util

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"syscall"
)

// CleanupFunc is a function to run during cleanup
type CleanupFunc func()

// SignalHandler manages graceful shutdown
type SignalHandler struct {
	cleanupFuncs []CleanupFunc
	mu           sync.Mutex
	ctx          context.Context
	cancel       context.CancelFunc
}

// NewSignalHandler creates a new signal handler
func NewSignalHandler() *SignalHandler {
	ctx, cancel := context.WithCancel(context.Background())
	sh := &SignalHandler{
		ctx:    ctx,
		cancel: cancel,
	}
	sh.setup()
	return sh
}

// setup sets up signal handling
func (sh *SignalHandler) setup() {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigChan
		sh.cleanup()
		os.Exit(1)
	}()
}

// Context returns the context that will be cancelled on signal
func (sh *SignalHandler) Context() context.Context {
	return sh.ctx
}

// RegisterCleanup registers a cleanup function to run on shutdown
func (sh *SignalHandler) RegisterCleanup(fn CleanupFunc) {
	sh.mu.Lock()
	defer sh.mu.Unlock()
	sh.cleanupFuncs = append(sh.cleanupFuncs, fn)
}

// cleanup runs all registered cleanup functions
func (sh *SignalHandler) cleanup() {
	sh.cancel()
	sh.mu.Lock()
	defer sh.mu.Unlock()

	// Run in reverse order (LIFO)
	for i := len(sh.cleanupFuncs) - 1; i >= 0; i-- {
		sh.cleanupFuncs[i]()
	}
}

// Cleanup manually triggers cleanup (for normal exit)
func (sh *SignalHandler) Cleanup() {
	sh.cleanup()
}
```

## Acceptance Criteria

- [ ] Clipboard copy works on macOS, Linux, and Windows
- [ ] Clipboard auto-clears after configured timeout
- [ ] Subsequent copies cancel pending clear operations
- [ ] Editor opens with content and returns modified content
- [ ] Temp files have restricted permissions (0600)
- [ ] Temp files are securely deleted (overwritten before removal)
- [ ] Editor detection works ($EDITOR, VS Code, nano fallback)
- [ ] Output formatter supports plain and JSON formats
- [ ] MaskValue correctly masks secrets
- [ ] Signal handler runs cleanup functions on Ctrl+C
- [ ] Cleanup functions run in LIFO order

## Notes

- Clipboard support depends on platform:
  - macOS: uses `pbcopy/pbpaste`
  - Linux: requires `xclip` or `xsel`
  - Windows: uses native API
- Editor with `--wait` flag (like VS Code) is required for proper behavior
- Secure delete is best-effort; SSDs may not actually overwrite
- Signal handler ensures temp files are cleaned on Ctrl+C
