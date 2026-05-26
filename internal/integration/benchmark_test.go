//go:build integration

package integration

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/yachiko/clerk/internal/testutil"
)

// benchmarkSetup is the b.Helper equivalent of setupTest. Because benchmarks
// don't have testing.T, we accept a *testing.B and skip via b.Skip.
func benchmarkSetup(b *testing.B) (*testutil.IntegrationTestConfig, string) {
	b.Helper()
	cfg := testutil.DefaultIntegrationConfig()
	if !testutil.IsMotoAvailable(cfg.MotoEndpoint) {
		b.Skipf("moto server not reachable at %s", cfg.MotoEndpoint)
	}
	if err := testutil.ResetMoto(cfg.MotoEndpoint); err != nil {
		b.Fatalf("reset moto: %v", err)
	}

	// Build clerk via the same once-only helper. The helper takes *testing.T,
	// so we adapt with a tiny shim.
	t := &testing.T{}
	testutil.BuildClerk(t)
	if t.Failed() {
		b.Fatal("failed to build clerk")
	}

	home, err := os.MkdirTemp("", "clerk-bench-home-*")
	if err != nil {
		b.Fatalf("mkdtemp: %v", err)
	}
	b.Cleanup(func() { os.RemoveAll(home) })
	return cfg, home
}

func BenchmarkIntegration_Get(b *testing.B) {
	cfg, home := benchmarkSetup(b)
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
	cfg, home := benchmarkSetup(b)
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
	cfg, home := benchmarkSetup(b)
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
	cfg, home := benchmarkSetup(b)
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
