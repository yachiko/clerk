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
	PreferredEditor string
	TempDir         string
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
func (e *Editor) Edit(content string, extension string) (string, error) {
	tempPath, err := e.createSecureTempFile(content, extension)
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}

	defer func() { _ = e.secureDelete(tempPath) }()

	editor := e.getEditor()
	if editor == "" {
		return "", fmt.Errorf("no editor found: set $EDITOR environment variable")
	}

	if err := e.openEditor(editor, tempPath); err != nil {
		return "", fmt.Errorf("failed to open editor: %w", err)
	}

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

	randBytes := make([]byte, 16)
	if _, err := rand.Read(randBytes); err != nil {
		return "", err
	}
	filename := fmt.Sprintf("clerk-%s%s", hex.EncodeToString(randBytes), extension)
	tempPath := filepath.Join(tempDir, filename)

	f, err := os.OpenFile(tempPath, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0600)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()

	if _, err := f.WriteString(content); err != nil {
		_ = os.Remove(tempPath)
		return "", err
	}

	return tempPath, nil
}

// secureDelete overwrites and removes a file. The zero-overwrite is
// best-effort: on modern journaling/COW filesystems and SSDs it doesn't
// reach the underlying blocks. Errors from the overwrite phase are
// discarded — the unlink at the end is what actually matters.
func (e *Editor) secureDelete(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}

	f, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		return os.Remove(path)
	}

	zeros := make([]byte, info.Size())
	_, _ = f.Write(zeros)
	_ = f.Sync()
	_ = f.Close()

	return os.Remove(path)
}

// getEditor returns the editor command to use
func (e *Editor) getEditor() string {
	if e.config.PreferredEditor != "" {
		return e.config.PreferredEditor
	}

	if editor := os.Getenv("EDITOR"); editor != "" {
		return editor
	}
	if editor := os.Getenv("VISUAL"); editor != "" {
		return editor
	}

	switch runtime.GOOS {
	case "darwin":
		if _, err := exec.LookPath("code"); err == nil {
			return "code --wait"
		}
		return "nano"
	case "windows":
		return "notepad"
	default:
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
