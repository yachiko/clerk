//go:build integration

package integration

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/yachiko/clerk/internal/testutil"
)

// Benchmarks stay as standard Go benchmarks. Ginkgo's spec runner doesn't
// drive `testing.B`, so these are reached via `go test -bench=. -tags=integration`.

// bSetup prepares a moto-connected fixture for a single benchmark.
func bSetup(b *testing.B) (*testutil.IntegrationTestConfig, string) {
	b.Helper()
	cfg := testutil.DefaultIntegrationConfig()
	if !testutil.IsMotoAvailable(cfg.MotoEndpoint) {
		b.Skipf("moto server not reachable at %s", cfg.MotoEndpoint)
	}
	if err := testutil.ResetMoto(cfg.MotoEndpoint); err != nil {
		b.Fatalf("reset moto: %v", err)
	}

	// BuildClerk takes our minimal T interface; *testing.B satisfies it via
	// its standard Helper/Cleanup/Fatalf methods.
	testutil.BuildClerk(b)

	home, err := os.MkdirTemp("", "clerk-bench-home-*")
	if err != nil {
		b.Fatalf("mkdtemp: %v", err)
	}
	b.Cleanup(func() { os.RemoveAll(home) })
	return cfg, home
}

func BenchmarkIntegration_Get(b *testing.B) {
	cfg, home := bSetup(b)
	ctx := context.Background()

	if _, _, err := testutil.RunClerkInHome(ctx, cfg, home, "put", "/bench/get/param", "v", "--type", "String"); err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, _, err := testutil.RunClerkInHome(ctx, cfg, home, "get", "/bench/get/param", "--value"); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkIntegration_Put(b *testing.B) {
	cfg, home := bSetup(b)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		name := fmt.Sprintf("/bench/put/param-%d", i)
		if _, _, err := testutil.RunClerkInHome(ctx, cfg, home, "put", name, "v", "--type", "String"); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkIntegration_List(b *testing.B) {
	cfg, home := bSetup(b)
	ctx := context.Background()

	fixtureCfg := testutil.DefaultFixtureConfig()
	fixtureCfg.Endpoint = cfg.MotoEndpoint
	fixtureCfg.Region = cfg.MotoRegion
	fixtureCfg.NumParameters = 100
	gen, err := testutil.NewFixtureGenerator(fixtureCfg)
	if err != nil {
		b.Fatal(err)
	}
	if _, err := gen.GenerateParameters(ctx); err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, _, err := testutil.RunClerkInHome(ctx, cfg, home, "list"); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkIntegration_Refresh(b *testing.B) {
	cfg, home := bSetup(b)
	ctx := context.Background()

	fixtureCfg := testutil.DefaultFixtureConfig()
	fixtureCfg.Endpoint = cfg.MotoEndpoint
	fixtureCfg.Region = cfg.MotoRegion
	fixtureCfg.NumParameters = 100
	gen, err := testutil.NewFixtureGenerator(fixtureCfg)
	if err != nil {
		b.Fatal(err)
	}
	if _, err := gen.GenerateParameters(ctx); err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, _, err := testutil.RunClerkInHome(ctx, cfg, home, "refresh"); err != nil {
			b.Fatal(err)
		}
	}
}
