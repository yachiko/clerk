package util

import (
	"os"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yachiko/clerk/internal/testutil"
)

func TestNewEditor(t *testing.T) {
	e := NewEditor(EditorConfig{PreferredEditor: "nvim"})
	assert.NotNil(t, e)
	assert.Equal(t, "nvim", e.config.PreferredEditor)
}

func TestEditor_getEditor_PreferenceOrder(t *testing.T) {
	// Strip both env vars so the test starts from a clean slate.
	os.Unsetenv("EDITOR")
	os.Unsetenv("VISUAL")

	t.Run("explicit preferred wins over env", func(t *testing.T) {
		testutil.SetEnv(t, "EDITOR", "vim")
		testutil.SetEnv(t, "VISUAL", "code")
		e := NewEditor(EditorConfig{PreferredEditor: "nano"})
		assert.Equal(t, "nano", e.getEditor())
	})

	t.Run("EDITOR env honored when no preferred", func(t *testing.T) {
		testutil.SetEnv(t, "EDITOR", "vim")
		os.Unsetenv("VISUAL")
		e := NewEditor(EditorConfig{})
		assert.Equal(t, "vim", e.getEditor())
	})

	t.Run("VISUAL falls through when EDITOR unset", func(t *testing.T) {
		os.Unsetenv("EDITOR")
		testutil.SetEnv(t, "VISUAL", "code --wait")
		e := NewEditor(EditorConfig{})
		assert.Equal(t, "code --wait", e.getEditor())
	})
}

func TestEditor_TempFile_CreateAndSecureDelete(t *testing.T) {
	tmp := testutil.TempDir(t)
	e := NewEditor(EditorConfig{TempDir: tmp})

	path, err := e.createSecureTempFile("hello world", ".txt")
	if err != nil {
		t.Fatalf("createSecureTempFile: %v", err)
	}
	assert.FileExists(t, path)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	assert.Equal(t, os.FileMode(0600), info.Mode().Perm())

	// secureDelete should remove the file.
	if err := e.secureDelete(path); err != nil {
		t.Fatalf("secureDelete: %v", err)
	}
	_, err = os.Stat(path)
	assert.True(t, os.IsNotExist(err), "file must be gone after secureDelete")
}

func TestEditor_Edit_RoundTripWithStubEditor(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("relies on /bin/true existing on PATH")
	}
	tmp := testutil.TempDir(t)
	// /bin/true exits 0 without modifying the file → Edit returns the original
	// content unchanged.
	e := NewEditor(EditorConfig{PreferredEditor: "/bin/true", TempDir: tmp})
	got, err := e.Edit("payload", ".txt")
	if err != nil {
		t.Fatalf("Edit: %v", err)
	}
	assert.Equal(t, "payload", got)
}

func TestEditor_GetEditorName(t *testing.T) {
	os.Unsetenv("EDITOR")
	os.Unsetenv("VISUAL")

	t.Run("strips path and args", func(t *testing.T) {
		e := NewEditor(EditorConfig{PreferredEditor: "/usr/bin/nvim --headless"})
		assert.Equal(t, "nvim", e.GetEditorName())
	})

	t.Run("none when no editor available", func(t *testing.T) {
		if runtime.GOOS != "linux" {
			t.Skip("linux-only: fallback logic looks up nano/vi on PATH")
		}
		// Empty PATH ensures the linux fallback (nano/vi LookPath) finds nothing.
		testutil.SetEnv(t, "PATH", "")
		e := NewEditor(EditorConfig{})
		assert.Equal(t, "none", e.GetEditorName())
	})
}
