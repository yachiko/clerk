// Command fixtures populates a moto endpoint with SSM and Secrets Manager data
// for local Clerk exploration.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/yachiko/clerk/internal/testutil"
)

func main() {
	endpoint := flag.String("endpoint", "http://localhost:5000", "moto server endpoint")
	region := flag.String("region", "us-east-1", "AWS region")
	parameterCount := flag.Int("count", 500, "number of random Parameter Store fixtures")
	secretCount := flag.Int("secrets", 50, "number of Secrets Manager fixtures")
	flag.Parse()

	if *parameterCount < 0 || *secretCount < 0 {
		log.Fatal("-count and -secrets must not be negative")
	}

	cfg := testutil.DefaultFixtureConfig()
	cfg.Endpoint = *endpoint
	cfg.Region = *region
	cfg.NumParameters = *parameterCount

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	parameters, err := testutil.NewFixtureGenerator(cfg)
	if err != nil {
		log.Fatalf("create Parameter Store fixture generator: %v", err)
	}
	secrets, err := testutil.NewSecretsManagerFixtureGenerator(cfg)
	if err != nil {
		log.Fatalf("create Secrets Manager fixture generator: %v", err)
	}

	createdParameters, err := parameters.GenerateParameters(ctx)
	if err != nil {
		log.Fatalf("generate Parameter Store fixtures: %v", err)
	}
	createdSecrets, err := secrets.GenerateSecrets(ctx, *secretCount)
	if err != nil {
		log.Fatalf("generate Secrets Manager fixtures: %v", err)
	}

	fmt.Printf("Created %d Parameter Store fixtures and %d Secrets Manager fixtures at %s in %s.\n", len(createdParameters), len(createdSecrets), *endpoint, *region)
}
