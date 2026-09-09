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

// EditorSession separates secure file preparation from process execution. A
// terminal UI can pass Command to tea.ExecProcess, which releases its terminal
// while the editor owns stdin/stdout.
type EditorSession struct {
	path    string
	command *exec.Cmd
	editor  *Editor
}

func (e *Editor) Prepare(content, extension string) (*EditorSession, error) {
	path, err := e.createSecureTempFile(content, extension)
	if err != nil {
		return nil, fmt.Errorf("failed to create temp file: %w", err)
	}
	editor := e.getEditor()
	if editor == "" {
		_ = e.secureDelete(path)
		return nil, fmt.Errorf("no editor found: set $EDITOR environment variable")
	}
	parts, err := parseEditorCommand(editor)
	if err != nil {
		_ = e.secureDelete(path)
		return nil, err
	}
	cmd := exec.Command(parts[0], append(parts[1:], path)...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return &EditorSession{path: path, command: cmd, editor: e}, nil
}

func (s *EditorSession) Command() *exec.Cmd    { return s.command }
func (s *EditorSession) Read() (string, error) { b, err := os.ReadFile(s.path); return string(b), err }
func (s *EditorSession) Close() error          { return s.editor.secureDelete(s.path) }

// NewEditor creates a new editor utility
func NewEditor(config EditorConfig) *Editor {
	return &Editor{config: config}
}

// Edit opens content in an external editor and returns the modified content
func (e *Editor) Edit(content string, extension string) (string, error) {
	session, err := e.Prepare(content, extension)
	if err != nil {
		return "", err
	}
	defer func() { _ = session.Close() }()
	if err := session.Command().Run(); err != nil {
		return "", fmt.Errorf("failed to open editor: %w", err)
	}
	modified, err := session.Read()
	if err != nil {
		return "", fmt.Errorf("failed to read modified content: %w", err)
	}

	return modified, nil
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

// parseEditorCommand handles quoting for command paths and arguments while
// deliberately avoiding a shell. Shell expansion would make $EDITOR input an
// execution surface; this parser only recognizes quotes and backslash escapes.
func parseEditorCommand(command string) ([]string, error) {
	var args []string
	var current strings.Builder
	var quote rune
	escaped := false
	flush := func() {
		if current.Len() > 0 {
			args = append(args, current.String())
			current.Reset()
		}
	}
	for _, r := range command {
		if escaped {
			current.WriteRune(r)
			escaped = false
			continue
		}
		if r == '\\' && quote != '\'' {
			escaped = true
			continue
		}
		if quote != 0 {
			if r == quote {
				quote = 0
			} else {
				current.WriteRune(r)
			}
			continue
		}
		if r == '\'' || r == '"' {
			quote = r
			continue
		}
		if r == ' ' || r == '\t' {
			flush()
			continue
		}
		current.WriteRune(r)
	}
	if escaped || quote != 0 {
		return nil, fmt.Errorf("invalid editor command: unmatched quote or escape")
	}
	flush()
	if len(args) == 0 {
		return nil, fmt.Errorf("empty editor command")
	}
	return args, nil
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
