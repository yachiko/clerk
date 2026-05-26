// Package testutil provides shared test helpers.
package testutil

import (
	"os"
	"path/filepath"
	"testing"
)

// TempDir creates a temporary directory for tests and registers automatic cleanup.
func TempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "clerk-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

// TempFile writes content to a file inside dir and returns its path.
func TempFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	return path
}

// SetEnv sets an environment variable for the duration of the test and
// restores the prior value on cleanup.
func SetEnv(t *testing.T, key, value string) {
	t.Helper()
	old, existed := os.LookupEnv(key)
	if err := os.Setenv(key, value); err != nil {
		t.Fatalf("failed to set env %s: %v", key, err)
	}
	t.Cleanup(func() {
		if existed {
			os.Setenv(key, old)
		} else {
			os.Unsetenv(key)
		}
	})
}

// IsolateHome points HOME at a fresh temp directory so config/cache writes
// don't touch the real user home. Returns the temp dir.
func IsolateHome(t *testing.T) string {
	t.Helper()
	dir := TempDir(t)
	SetEnv(t, "HOME", dir)
	return dir
}
