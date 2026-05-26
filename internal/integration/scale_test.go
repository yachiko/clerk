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

// largeScaleFixture creates `count` parameters in moto and registers cleanup.
// Skipped automatically in -short mode.
func largeScaleFixture(t *testing.T, cfg *testutil.IntegrationTestConfig, count int) []string {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping large-scale test in -short mode")
	}

	fixtureCfg := testutil.DefaultFixtureConfig()
	fixtureCfg.Endpoint = cfg.MotoEndpoint
	fixtureCfg.Region = cfg.MotoRegion
	fixtureCfg.NumParameters = count

	gen, err := testutil.NewFixtureGenerator(fixtureCfg)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	created, err := gen.GenerateParameters(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, created, "fixture generated zero parameters")

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		_ = gen.CleanupParameters(ctx, created)
	})
	t.Logf("generated %d parameters", len(created))
	return created
}

func TestIntegration_LargeScale_List(t *testing.T) {
	cfg, home := setupTest(t)
	created := largeScaleFixture(t, cfg, 200)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	stdout, stderr, err := testutil.RunClerkInHome(ctx, cfg, home, "list")
	require.NoErrorf(t, err, "stderr: %s", stderr)

	// Count distinct parameter paths in the output. Output format includes
	// headers and ANSI styling; the assertion is "saw most of what we made".
	nameCount := 0
	for _, line := range strings.Split(stdout, "\n") {
		if strings.Contains(line, "/") && !strings.Contains(line, "─") {
			nameCount++
		}
	}
	// Random name generation can collide; require >= 80% of created visible.
	assert.GreaterOrEqual(t, nameCount, len(created)*8/10,
		"list output (%d lines with /) should cover ~all %d created params", nameCount, len(created))
}

func TestIntegration_LargeScale_Refresh(t *testing.T) {
	cfg, home := setupTest(t)
	largeScaleFixture(t, cfg, 300)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	start := time.Now()
	stdout, stderr, err := testutil.RunClerkInHome(ctx, cfg, home, "refresh")
	duration := time.Since(start)
	require.NoErrorf(t, err, "stderr: %s", stderr)
	assert.Contains(t, strings.ToLower(stdout), "refresh")
	t.Logf("refresh of 300 params completed in %v", duration)
}

func TestIntegration_LargeScale_FilterPerformance(t *testing.T) {
	cfg, home := setupTest(t)
	largeScaleFixture(t, cfg, 500)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	for _, pattern := range []string{"/dev/*", "/prod/*", "/staging/*", "/qa/*"} {
		t.Run("filter_"+pattern, func(t *testing.T) {
			start := time.Now()
			_, stderr, err := testutil.RunClerkInHome(ctx, cfg, home, "list", pattern)
			duration := time.Since(start)
			require.NoErrorf(t, err, "stderr: %s", stderr)
			t.Logf("filter %s took %v", pattern, duration)
			assert.Less(t, duration, 30*time.Second, "filter on 500 params should be fast")
		})
	}
}
