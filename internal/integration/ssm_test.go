//go:build integration

package integration

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yachiko/clerk/internal/testutil"
)

// setupTest is called from every test. It builds clerk (once per process),
// resets the moto server to a clean state, and returns a per-test HOME and the
// shared integration config. Tests that need to share state across multiple
// RunClerk invocations should pass `home` to RunClerkInHome.
func setupTest(t *testing.T) (*testutil.IntegrationTestConfig, string) {
	t.Helper()
	testutil.SkipIfNoMoto(t)
	testutil.BuildClerk(t)

	cfg := testutil.DefaultIntegrationConfig()
	require.NoError(t, testutil.ResetMoto(cfg.MotoEndpoint))

	return cfg, t.TempDir()
}

func TestIntegration_PutAndGet(t *testing.T) {
	cfg, home := setupTest(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	stdout, stderr, err := testutil.RunClerkInHome(ctx, cfg, home, "put", "/test/integration/secret", "my-secret-value")
	require.NoErrorf(t, err, "stderr: %s", stderr)
	assert.Contains(t, stdout, "Created")
	assert.Contains(t, stdout, "/test/integration/secret")

	stdout, stderr, err = testutil.RunClerkInHome(ctx, cfg, home, "get", "/test/integration/secret", "--value")
	require.NoErrorf(t, err, "stderr: %s", stderr)
	assert.Equal(t, "my-secret-value\n", stdout)
}

func TestIntegration_PutUpdateBumpsVersion(t *testing.T) {
	cfg, home := setupTest(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, stderr, err := testutil.RunClerkInHome(ctx, cfg, home, "put", "/test/v/secret", "v1")
	require.NoErrorf(t, err, "stderr: %s", stderr)

	stdout, stderr, err := testutil.RunClerkInHome(ctx, cfg, home, "put", "/test/v/secret", "v2")
	require.NoErrorf(t, err, "stderr: %s", stderr)
	assert.Contains(t, stdout, "Updated")
	assert.Contains(t, stdout, "version 2")

	stdout, _, err = testutil.RunClerkInHome(ctx, cfg, home, "get", "/test/v/secret", "--value")
	require.NoError(t, err)
	assert.Equal(t, "v2\n", stdout)
}

func TestIntegration_PutWithTags(t *testing.T) {
	cfg, home := setupTest(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, stderr, err := testutil.RunClerkInHome(ctx, cfg, home, "put", "/test/tagged/secret", "v", "--tags", "env=test,team=backend")
	require.NoErrorf(t, err, "stderr: %s", stderr)

	stdout, _, err := testutil.RunClerkInHome(ctx, cfg, home, "get", "/test/tagged/secret", "--output", "json")
	require.NoError(t, err)
	assert.Contains(t, stdout, `"env"`)
	assert.Contains(t, stdout, `"test"`)
	assert.Contains(t, stdout, `"team"`)
	assert.Contains(t, stdout, `"backend"`)
}

func TestIntegration_GetVersion(t *testing.T) {
	cfg, home := setupTest(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, _, err := testutil.RunClerkInHome(ctx, cfg, home, "put", "/test/versions/secret", "version-1")
	require.NoError(t, err)
	_, _, err = testutil.RunClerkInHome(ctx, cfg, home, "put", "/test/versions/secret", "version-2")
	require.NoError(t, err)

	stdout, _, err := testutil.RunClerkInHome(ctx, cfg, home, "get", "/test/versions/secret@1", "--value")
	require.NoError(t, err)
	assert.Equal(t, "version-1\n", stdout)

	stdout, _, err = testutil.RunClerkInHome(ctx, cfg, home, "get", "/test/versions/secret", "--value")
	require.NoError(t, err)
	assert.Equal(t, "version-2\n", stdout)
}

func TestIntegration_GetWithMask(t *testing.T) {
	cfg, home := setupTest(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, _, err := testutil.RunClerkInHome(ctx, cfg, home, "put", "/test/mask/secret", "sensitive-data-123")
	require.NoError(t, err)

	stdout, _, err := testutil.RunClerkInHome(ctx, cfg, home, "get", "/test/mask/secret", "--mask")
	require.NoError(t, err)
	assert.NotContains(t, stdout, "sensitive-data-123")
	assert.Contains(t, stdout, "*")
}

func TestIntegration_Delete(t *testing.T) {
	cfg, home := setupTest(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, _, err := testutil.RunClerkInHome(ctx, cfg, home, "put", "/test/del/secret", "bye")
	require.NoError(t, err)

	stdout, stderr, err := testutil.RunClerkInHome(ctx, cfg, home, "delete", "/test/del/secret", "--force")
	require.NoErrorf(t, err, "stderr: %s", stderr)
	assert.Contains(t, strings.ToLower(stdout), "deleted")

	_, stderr, err = testutil.RunClerkInHome(ctx, cfg, home, "get", "/test/del/secret")
	assert.Error(t, err, "get should fail after delete; stderr=%s", stderr)
}

func TestIntegration_List(t *testing.T) {
	cfg, home := setupTest(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	gen, err := testutil.NewFixtureGenerator(&testutil.FixtureConfig{
		Endpoint: cfg.MotoEndpoint,
		Region:   cfg.MotoRegion,
	})
	require.NoError(t, err)
	created, err := gen.GenerateSpecificParameters(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { _ = gen.CleanupParameters(context.Background(), created) })

	// List all under /dev/* — should not include /prod/*
	stdout, stderr, err := testutil.RunClerkInHome(ctx, cfg, home, "list", "/dev/*")
	require.NoErrorf(t, err, "stderr: %s", stderr)
	assert.Contains(t, stdout, "/dev/")
	assert.NotContains(t, stdout, "/prod/")

	// List all under /prod/* — should not include /dev/*
	stdout, _, err = testutil.RunClerkInHome(ctx, cfg, home, "list", "/prod/*")
	require.NoError(t, err)
	assert.Contains(t, stdout, "/prod/")
	assert.NotContains(t, stdout, "/dev/")

	// Default (no path) returns everything
	stdout, _, err = testutil.RunClerkInHome(ctx, cfg, home, "list")
	require.NoError(t, err)
	assert.Contains(t, stdout, "/dev/")
	assert.Contains(t, stdout, "/prod/")
	assert.Contains(t, stdout, "/staging/")
}

func TestIntegration_Copy(t *testing.T) {
	cfg, home := setupTest(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Use plain String so the round-trip value matches verbatim. SecureString
	// works against real AWS (opaque ciphertext that decrypts back), but moto's
	// mock encryption prepends "kms:alias/aws/ssm:" which leaks when clerk's
	// cp reads with withDecryption=false.
	_, _, err := testutil.RunClerkInHome(ctx, cfg, home, "put", "/test/cp/src", "the-value", "--type", "String")
	require.NoError(t, err)

	stdout, stderr, err := testutil.RunClerkInHome(ctx, cfg, home, "cp", "/test/cp/src", "/test/cp/dst")
	require.NoErrorf(t, err, "stderr: %s", stderr)
	assert.Contains(t, strings.ToLower(stdout), "copied")

	// Both should now be readable with the same value
	for _, name := range []string{"/test/cp/src", "/test/cp/dst"} {
		out, _, err := testutil.RunClerkInHome(ctx, cfg, home, "get", name, "--value")
		require.NoError(t, err)
		assert.Equal(t, "the-value\n", out)
	}
}

func TestIntegration_Move(t *testing.T) {
	cfg, home := setupTest(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Same moto-SecureString caveat as TestIntegration_Copy — use String.
	_, _, err := testutil.RunClerkInHome(ctx, cfg, home, "put", "/test/mv/src", "movee", "--type", "String")
	require.NoError(t, err)

	stdout, stderr, err := testutil.RunClerkInHome(ctx, cfg, home, "mv", "/test/mv/src", "/test/mv/dst", "--force")
	require.NoErrorf(t, err, "stderr: %s", stderr)
	assert.Contains(t, strings.ToLower(stdout), "moved")

	out, _, err := testutil.RunClerkInHome(ctx, cfg, home, "get", "/test/mv/dst", "--value")
	require.NoError(t, err)
	assert.Equal(t, "movee\n", out)

	_, _, err = testutil.RunClerkInHome(ctx, cfg, home, "get", "/test/mv/src")
	assert.Error(t, err, "source must be gone after mv")
}

func TestIntegration_Refresh(t *testing.T) {
	cfg, home := setupTest(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	gen, err := testutil.NewFixtureGenerator(&testutil.FixtureConfig{
		Endpoint: cfg.MotoEndpoint,
		Region:   cfg.MotoRegion,
	})
	require.NoError(t, err)
	created, err := gen.GenerateSpecificParameters(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { _ = gen.CleanupParameters(context.Background(), created) })

	stdout, stderr, err := testutil.RunClerkInHome(ctx, cfg, home, "refresh")
	require.NoErrorf(t, err, "stderr: %s", stderr)
	assert.Contains(t, strings.ToLower(stdout), "refresh")
}

func TestIntegration_JSONOutput(t *testing.T) {
	cfg, home := setupTest(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, _, err := testutil.RunClerkInHome(ctx, cfg, home, "put", "/test/json/secret", "json-value")
	require.NoError(t, err)

	stdout, _, err := testutil.RunClerkInHome(ctx, cfg, home, "get", "/test/json/secret", "--output", "json")
	require.NoError(t, err)
	assert.Contains(t, stdout, `"name"`)
	assert.Contains(t, stdout, `"/test/json/secret"`)
	assert.Contains(t, stdout, `"value"`)
}

func TestIntegration_ErrorCases(t *testing.T) {
	cfg, home := setupTest(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	t.Run("get non-existent", func(t *testing.T) {
		_, stderr, err := testutil.RunClerkInHome(ctx, cfg, home, "get", "/never/created")
		assert.Error(t, err)
		assert.NotEmpty(t, stderr)
	})

	t.Run("delete non-existent", func(t *testing.T) {
		_, _, err := testutil.RunClerkInHome(ctx, cfg, home, "delete", "/never/created", "--force")
		assert.Error(t, err)
	})

	t.Run("get with bad version", func(t *testing.T) {
		_, _, err := testutil.RunClerkInHome(ctx, cfg, home, "put", "/test/badver/secret", "value")
		require.NoError(t, err)
		_, _, err = testutil.RunClerkInHome(ctx, cfg, home, "get", "/test/badver/secret@99")
		assert.Error(t, err, "version 99 doesn't exist")
	})
}
