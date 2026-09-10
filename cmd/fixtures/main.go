package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/yachiko/clerk/internal/testutil"
)

func main() {
	endpoint := flag.String("endpoint", "http://localhost:5000", "Moto server endpoint")
	region := flag.String("region", "us-east-1", "AWS region")
	count := flag.Int("count", 25, "Target total number of SSM parameters (the deterministic corpus is always included)")
	reset := flag.Bool("reset", false, "Reset all Moto state before creating fixtures")
	flag.Parse()

	if *count < 0 {
		fatalf("count must not be negative")
	}
	endpointValue := strings.TrimRight(*endpoint, "/")
	if *reset {
		if err := testutil.ResetMoto(endpointValue); err != nil {
			fatalf("reset Moto: %v", err)
		}
		fmt.Printf("Reset Moto at %s\n", endpointValue)
	}

	cfg := testutil.DefaultFixtureConfig()
	cfg.Endpoint = endpointValue
	cfg.Region = *region
	cfg.NumParameters = *count
	generator, err := testutil.NewFixtureGenerator(cfg)
	if err != nil {
		fatalf("create fixture generator: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	result, err := generator.Populate(ctx)
	if err != nil {
		fatalf("populate fixtures: %v", err)
	}

	fmt.Printf("Created %d SSM parameters:\n", len(result.Parameters))
	for _, name := range result.Parameters {
		fmt.Printf("  SSM %s\n", name)
	}
	fmt.Printf("Created %d Secrets Manager secrets:\n", len(result.Secrets))
	for _, name := range result.Secrets {
		fmt.Printf("  Secrets Manager %s\n", name)
	}
	for _, warning := range result.Warnings {
		fmt.Printf("Warning: %s\n", warning)
	}

	credentials := fmt.Sprintf("AWS_ENDPOINT_URL=%s AWS_REGION=%s AWS_ACCESS_KEY_ID=testing AWS_SECRET_ACCESS_KEY=testing AWS_SESSION_TOKEN=testing", endpointValue, *region)
	fmt.Printf("\nNext commands:\n  %s go run ./cmd/clerk browse --backend all\n  %s go run ./cmd/clerk list --backend all\n", credentials, credentials)
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "fixtures: "+format+"\n", args...)
	os.Exit(1)
}
