package testutil

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// IntegrationTestConfig holds configuration for end-to-end tests that drive
// the clerk binary against a moto server.
type IntegrationTestConfig struct {
	MotoEndpoint string
	MotoRegion   string
	BinaryPath   string
}

// DefaultIntegrationConfig reads MOTO_ENDPOINT from the environment (default
// http://localhost:5000) and resolves the binary path relative to the repo
// root, so callers don't have to thread cwd through.
func DefaultIntegrationConfig() *IntegrationTestConfig {
	endpoint := os.Getenv("MOTO_ENDPOINT")
	if endpoint == "" {
		endpoint = "http://localhost:5000"
	}
	return &IntegrationTestConfig{
		MotoEndpoint: endpoint,
		MotoRegion:   "us-east-1",
		BinaryPath:   repoBinaryPath(),
	}
}

// SkipIfNoMoto is the entry point for every integration test: it short-circuits
// the test if the moto endpoint isn't reachable.
// skippableT extends our minimal T with Skipf, used only by SkipIfNoMoto.
type skippableT interface {
	T
	Skipf(format string, args ...any)
}

// SkipIfNoMoto short-circuits the test if the moto endpoint isn't reachable.
func SkipIfNoMoto(t skippableT) {
	t.Helper()
	cfg := DefaultIntegrationConfig()
	if !IsMotoAvailable(cfg.MotoEndpoint) {
		t.Skipf("moto server not reachable at %s — start it with `moto_server -p 5000` or `make moto-start`", cfg.MotoEndpoint)
	}
}

// IsMotoAvailable returns true if moto is responding at the given endpoint.
func IsMotoAvailable(endpoint string) bool {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(endpoint + "/moto-api/")
	if err != nil {
		return false
	}
	_ = resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// WaitForMoto blocks until moto becomes reachable or timeout elapses.
func WaitForMoto(endpoint string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if IsMotoAvailable(endpoint) {
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf("moto server not available at %s after %v", endpoint, timeout)
}

// ResetMoto wipes all moto state so each test starts from a clean slate.
func ResetMoto(endpoint string) error {
	client := &http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequest(http.MethodPost, endpoint+"/moto-api/reset", nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("moto reset returned status %d", resp.StatusCode)
	}
	return nil
}

// RunClerk executes the clerk binary with the given args, routed at the moto
// endpoint, with an isolated HOME so cache/config writes don't escape the test.
//
// Returns (stdout, stderr, err). exit-code-non-zero shows up as err.
func RunClerk(ctx context.Context, cfg *IntegrationTestConfig, args ...string) (string, string, error) {
	return RunClerkInHome(ctx, cfg, "", args...)
}

// RunClerkInHome is like RunClerk but lets the caller pin HOME to a specific
// directory so cache state survives across invocations (useful for testing
// list-after-put). Pass "" to get a fresh per-call temp HOME.
func RunClerkInHome(ctx context.Context, cfg *IntegrationTestConfig, home string, args ...string) (string, string, error) {
	if home == "" {
		tmp, err := os.MkdirTemp("", "clerk-int-home-*")
		if err != nil {
			return "", "", fmt.Errorf("MkdirTemp: %w", err)
		}
		defer func() { _ = os.RemoveAll(tmp) }()
		home = tmp
	}

	env := append([]string{}, sanitizedEnviron()...)
	env = append(env,
		"AWS_ENDPOINT_URL="+cfg.MotoEndpoint,
		"AWS_REGION="+cfg.MotoRegion,
		"AWS_DEFAULT_REGION="+cfg.MotoRegion,
		"AWS_ACCESS_KEY_ID=testing",
		"AWS_SECRET_ACCESS_KEY=testing",
		"AWS_SESSION_TOKEN=testing",
		"HOME="+home,
	)

	cmd := exec.CommandContext(ctx, cfg.BinaryPath, args...)
	cmd.Env = env

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

// sanitizedEnviron returns os.Environ() minus AWS_* and HOME entries so they
// can be replaced with deterministic test values without interference from the
// developer's real AWS config.
func sanitizedEnviron() []string {
	prefixes := []string{"AWS_", "HOME="}
	in := os.Environ()
	out := make([]string, 0, len(in))
outer:
	for _, kv := range in {
		for _, p := range prefixes {
			if strings.HasPrefix(kv, p) {
				continue outer
			}
		}
		out = append(out, kv)
	}
	return out
}

var (
	buildOnce sync.Once
	buildErr  error
	buildPath string
)

// BuildClerk builds the clerk binary into bin/clerk once per test process.
// Subsequent calls return the cached path. Accepts our minimal T so it works
// with *testing.T, *testing.B, and ginkgo.GinkgoT().
func BuildClerk(t T) string {
	t.Helper()
	buildOnce.Do(func() {
		root := repoRoot()
		buildPath = filepath.Join(root, "bin", "clerk")
		if err := os.MkdirAll(filepath.Join(root, "bin"), 0755); err != nil {
			buildErr = err
			return
		}
		cmd := exec.Command("go", "build", "-o", buildPath, "./cmd/clerk")
		cmd.Dir = root
		cmd.Stderr = os.Stderr
		buildErr = cmd.Run()
	})
	if buildErr != nil {
		t.Fatalf("failed to build clerk: %v", buildErr)
	}
	return buildPath
}

// repoRoot finds the repo root by walking up from this file's location to find
// the go.mod file. Falls back to cwd if not found.
func repoRoot() string {
	_, file, _, _ := runtime.Caller(0)
	dir := filepath.Dir(file)
	for i := 0; i < 10; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	cwd, _ := os.Getwd()
	return cwd
}

// repoBinaryPath returns the absolute path to bin/clerk in the repo root.
func repoBinaryPath() string {
	return filepath.Join(repoRoot(), "bin", "clerk")
}
