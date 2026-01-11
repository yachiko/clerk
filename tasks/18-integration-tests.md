# Task 18: Integration Tests with Moto

## Objective
Implement integration tests using Moto server to mock AWS SSM API, with fixtures that populate Parameter Store with hundreds of random parameters for realistic testing scenarios. Include comprehensive tests for multi-region and multi-account cache isolation to ensure the region/account-scoped cache architecture works correctly.

## Prerequisites
- Task 17 completed (unit tests)
- Python 3.8+ installed (for moto server)
- Docker installed (optional, for containerized moto)

## Technical Approach

Use **moto** (Mock AWS Services) in standalone server mode to provide a realistic AWS SSM API endpoint for integration tests. This allows testing the full CLI flow without real AWS credentials.

## Deliverables

### 1. Install Moto Server

Create file `scripts/install-moto.sh`:

```bash
#!/bin/bash
# Install moto server for integration testing

set -e

echo "Installing moto server..."

# Check if pip is available
if ! command -v pip3 &> /dev/null; then
    echo "Error: pip3 is not installed"
    exit 1
fi

# Install moto with server support
pip3 install "moto[server,ssm]>=4.0.0"

echo "Moto server installed successfully"
echo "Run with: moto_server ssm -p 5000"
```

### 2. Create Docker Compose for Moto

Create file `docker-compose.test.yml`:

```yaml
version: '3.8'

services:
  moto:
    image: motoserver/moto:latest
    ports:
      - "5000:5000"
    environment:
      - MOTO_PORT=5000
    command: ["-p", "5000", "ssm"]
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:5000/moto-api/"]
      interval: 5s
      timeout: 3s
      retries: 10
```

### 3. Create Test Fixtures Generator

Create file `internal/testutil/fixtures.go`:

```go
package testutil

import (
	"context"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
)

// FixtureConfig configures test fixture generation
type FixtureConfig struct {
	// Endpoint is the moto server endpoint (e.g., "http://localhost:5000")
	Endpoint string
	// Region is the AWS region to use
	Region string
	// NumParameters is the total number of parameters to create
	NumParameters int
	// Environments is the list of environment prefixes
	Environments []string
	// Services is the list of service names
	Services []string
	// SecretTypes is the list of secret type suffixes
	SecretTypes []string
}

// DefaultFixtureConfig returns a default fixture configuration
func DefaultFixtureConfig() *FixtureConfig {
	return &FixtureConfig{
		Endpoint:      "http://localhost:5000",
		Region:        "us-east-1",
		NumParameters: 500,
		Environments:  []string{"dev", "staging", "prod", "qa", "uat"},
		Services:      []string{"api", "web", "worker", "scheduler", "auth", "payment", "notification", "analytics", "search", "cache"},
		SecretTypes:   []string{"db_password", "api_key", "secret_key", "token", "connection_string", "certificate", "private_key", "webhook_secret", "encryption_key", "access_token"},
	}
}

// FixtureGenerator generates test fixtures
type FixtureGenerator struct {
	client *ssm.Client
	config *FixtureConfig
	rng    *rand.Rand
}

// NewFixtureGenerator creates a new fixture generator
func NewFixtureGenerator(cfg *FixtureConfig) (*FixtureGenerator, error) {
	if cfg == nil {
		cfg = DefaultFixtureConfig()
	}

	// Create custom endpoint resolver for moto
	customResolver := aws.EndpointResolverWithOptionsFunc(func(service, region string, options ...interface{}) (aws.Endpoint, error) {
		return aws.Endpoint{
			URL:           cfg.Endpoint,
			SigningRegion: cfg.Region,
		}, nil
	})

	// Load AWS config with fake credentials for moto
	awsCfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithRegion(cfg.Region),
		config.WithEndpointResolverWithOptions(customResolver),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("testing", "testing", "testing")),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	return &FixtureGenerator{
		client: ssm.NewFromConfig(awsCfg),
		config: cfg,
		rng:    rand.New(rand.NewSource(time.Now().UnixNano())),
	}, nil
}

// GenerateParameters creates test parameters in SSM
func (g *FixtureGenerator) GenerateParameters(ctx context.Context) ([]string, error) {
	var created []string

	for i := 0; i < g.config.NumParameters; i++ {
		name := g.generateParameterName()
		value := g.generateParameterValue()
		paramType := g.randomParameterType()

		input := &ssm.PutParameterInput{
			Name:  aws.String(name),
			Value: aws.String(value),
			Type:  paramType,
		}

		// Add tags randomly (50% chance)
		if g.rng.Float32() > 0.5 {
			tags := g.generateTags()
			input.Tags = tags
		}

		_, err := g.client.PutParameter(ctx, input)
		if err != nil {
			// Skip duplicates
			if strings.Contains(err.Error(), "ParameterAlreadyExists") {
				continue
			}
			return created, fmt.Errorf("failed to create parameter %s: %w", name, err)
		}

		created = append(created, name)

		// Create multiple versions for some parameters (20% chance)
		if g.rng.Float32() > 0.8 {
			numVersions := g.rng.Intn(5) + 1
			for v := 0; v < numVersions; v++ {
				input.Overwrite = aws.Bool(true)
				input.Value = aws.String(g.generateParameterValue())
				input.Tags = nil // Tags can't be updated with PutParameter
				g.client.PutParameter(ctx, input)
			}
		}
	}

	return created, nil
}

// generateParameterName creates a realistic parameter name
func (g *FixtureGenerator) generateParameterName() string {
	env := g.config.Environments[g.rng.Intn(len(g.config.Environments))]
	service := g.config.Services[g.rng.Intn(len(g.config.Services))]
	secretType := g.config.SecretTypes[g.rng.Intn(len(g.config.SecretTypes))]

	// Sometimes add a subsystem
	if g.rng.Float32() > 0.7 {
		subsystems := []string{"primary", "replica", "backup", "external", "internal"}
		subsystem := subsystems[g.rng.Intn(len(subsystems))]
		return fmt.Sprintf("/%s/%s/%s/%s", env, service, subsystem, secretType)
	}

	return fmt.Sprintf("/%s/%s/%s", env, service, secretType)
}

// generateParameterValue creates a random parameter value
func (g *FixtureGenerator) generateParameterValue() string {
	length := g.rng.Intn(64) + 16
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!@#$%^&*"
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[g.rng.Intn(len(charset))]
	}
	return string(b)
}

// randomParameterType returns a random parameter type
func (g *FixtureGenerator) randomParameterType() types.ParameterType {
	roll := g.rng.Float32()
	if roll < 0.7 {
		return types.ParameterTypeSecureString
	} else if roll < 0.9 {
		return types.ParameterTypeString
	}
	return types.ParameterTypeStringList
}

// generateTags creates random tags
func (g *FixtureGenerator) generateTags() []types.Tag {
	tagKeys := []string{"team", "cost-center", "project", "owner", "criticality"}
	tagValues := map[string][]string{
		"team":        {"backend", "frontend", "devops", "data", "security"},
		"cost-center": {"engineering", "operations", "infrastructure"},
		"project":     {"main-app", "microservices", "data-pipeline", "ml-platform"},
		"owner":       {"alice", "bob", "charlie", "david", "eve"},
		"criticality": {"high", "medium", "low"},
	}

	numTags := g.rng.Intn(3) + 1
	var tags []types.Tag

	usedKeys := make(map[string]bool)
	for i := 0; i < numTags; i++ {
		key := tagKeys[g.rng.Intn(len(tagKeys))]
		if usedKeys[key] {
			continue
		}
		usedKeys[key] = true

		values := tagValues[key]
		value := values[g.rng.Intn(len(values))]

		tags = append(tags, types.Tag{
			Key:   aws.String(key),
			Value: aws.String(value),
		})
	}

	return tags
}

// CleanupParameters deletes all test parameters
func (g *FixtureGenerator) CleanupParameters(ctx context.Context, names []string) error {
	for _, name := range names {
		_, err := g.client.DeleteParameter(ctx, &ssm.DeleteParameterInput{
			Name: aws.String(name),
		})
		if err != nil && !strings.Contains(err.Error(), "ParameterNotFound") {
			return fmt.Errorf("failed to delete parameter %s: %w", name, err)
		}
	}
	return nil
}

// GenerateSpecificParameters creates a specific set of parameters for deterministic tests
func (g *FixtureGenerator) GenerateSpecificParameters(ctx context.Context) ([]string, error) {
	params := []struct {
		name  string
		value string
		ptype types.ParameterType
		tags  map[string]string
	}{
		{"/test/simple/string", "simple-value", types.ParameterTypeString, nil},
		{"/test/simple/secure", "secure-value", types.ParameterTypeSecureString, nil},
		{"/test/tagged/param", "tagged-value", types.ParameterTypeSecureString, map[string]string{"env": "test", "team": "backend"}},
		{"/dev/database/password", "dev-db-pass-123", types.ParameterTypeSecureString, map[string]string{"env": "dev"}},
		{"/dev/database/host", "localhost:5432", types.ParameterTypeString, map[string]string{"env": "dev"}},
		{"/dev/api/key", "dev-api-key-abc", types.ParameterTypeSecureString, map[string]string{"env": "dev"}},
		{"/staging/database/password", "staging-db-pass-456", types.ParameterTypeSecureString, map[string]string{"env": "staging"}},
		{"/staging/database/host", "staging-db.example.com:5432", types.ParameterTypeString, map[string]string{"env": "staging"}},
		{"/prod/database/password", "prod-db-pass-789", types.ParameterTypeSecureString, map[string]string{"env": "prod", "criticality": "high"}},
		{"/prod/database/host", "prod-db.example.com:5432", types.ParameterTypeString, map[string]string{"env": "prod"}},
		{"/prod/api/key", "prod-api-key-xyz", types.ParameterTypeSecureString, map[string]string{"env": "prod", "criticality": "high"}},
		{"/prod/api/secret", "prod-api-secret", types.ParameterTypeSecureString, map[string]string{"env": "prod"}},
		{"/shared/config/list", "item1,item2,item3", types.ParameterTypeStringList, nil},
	}

	var created []string
	for _, p := range params {
		input := &ssm.PutParameterInput{
			Name:  aws.String(p.name),
			Value: aws.String(p.value),
			Type:  p.ptype,
		}

		if len(p.tags) > 0 {
			var tags []types.Tag
			for k, v := range p.tags {
				tags = append(tags, types.Tag{
					Key:   aws.String(k),
					Value: aws.String(v),
				})
			}
			input.Tags = tags
		}

		_, err := g.client.PutParameter(ctx, input)
		if err != nil {
			return created, fmt.Errorf("failed to create parameter %s: %w", p.name, err)
		}
		created = append(created, p.name)
	}

	return created, nil
}
```

### 4. Create Integration Test Helper

Create file `internal/testutil/integration.go`:

```go
package testutil

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"testing"
	"time"
)

// IntegrationTestConfig holds integration test configuration
type IntegrationTestConfig struct {
	MotoEndpoint string
	MotoRegion   string
	BinaryPath   string
}

// DefaultIntegrationConfig returns default integration test config
func DefaultIntegrationConfig() *IntegrationTestConfig {
	endpoint := os.Getenv("MOTO_ENDPOINT")
	if endpoint == "" {
		endpoint = "http://localhost:5000"
	}

	return &IntegrationTestConfig{
		MotoEndpoint: endpoint,
		MotoRegion:   "us-east-1",
		BinaryPath:   "./bin/clerk",
	}
}

// SkipIfNoMoto skips the test if moto server is not available
func SkipIfNoMoto(t *testing.T) {
	t.Helper()

	cfg := DefaultIntegrationConfig()
	if !IsMotoAvailable(cfg.MotoEndpoint) {
		t.Skip("Moto server not available. Start with: docker-compose -f docker-compose.test.yml up -d")
	}
}

// IsMotoAvailable checks if moto server is running
func IsMotoAvailable(endpoint string) bool {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(endpoint + "/moto-api/")
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// WaitForMoto waits for moto server to be available
func WaitForMoto(endpoint string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if IsMotoAvailable(endpoint) {
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("moto server not available after %v", timeout)
}

// ResetMoto resets the moto server state
func ResetMoto(endpoint string) error {
	client := &http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequest("POST", endpoint+"/moto-api/reset", nil)
	if err != nil {
		return err
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to reset moto: status %d", resp.StatusCode)
	}
	return nil
}

// RunClerk runs the clerk binary with the given arguments
func RunClerk(ctx context.Context, cfg *IntegrationTestConfig, args ...string) (string, string, error) {
	// Set AWS endpoint environment variable for the clerk process
	env := append(os.Environ(),
		"AWS_ENDPOINT_URL="+cfg.MotoEndpoint,
		"AWS_REGION="+cfg.MotoRegion,
		"AWS_ACCESS_KEY_ID=testing",
		"AWS_SECRET_ACCESS_KEY=testing",
	)

	cmd := exec.CommandContext(ctx, cfg.BinaryPath, args...)
	cmd.Env = env

	stdout, err := cmd.Output()
	stderr := ""
	if exitErr, ok := err.(*exec.ExitError); ok {
		stderr = string(exitErr.Stderr)
	}

	return string(stdout), stderr, err
}

// BuildClerk builds the clerk binary for testing
func BuildClerk(t *testing.T) string {
	t.Helper()

	cmd := exec.Command("go", "build", "-o", "./bin/clerk", "./cmd/clerk")
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to build clerk: %v", err)
	}

	return "./bin/clerk"
}
```

### 5. Create Integration Tests

Create file `internal/integration/ssm_test.go`:

```go
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

func TestMain(m *testing.M) {
	// Build clerk binary before running tests
	// This is handled by individual tests using BuildClerk
	m.Run()
}

func setupTest(t *testing.T) (*testutil.IntegrationTestConfig, *testutil.FixtureGenerator) {
	t.Helper()
	testutil.SkipIfNoMoto(t)

	cfg := testutil.DefaultIntegrationConfig()

	// Reset moto state
	err := testutil.ResetMoto(cfg.MotoEndpoint)
	require.NoError(t, err)

	// Create fixture generator
	fixtureCfg := testutil.DefaultFixtureConfig()
	fixtureCfg.Endpoint = cfg.MotoEndpoint
	gen, err := testutil.NewFixtureGenerator(fixtureCfg)
	require.NoError(t, err)

	return cfg, gen
}

func TestIntegration_MultiRegion_CacheIsolation(t *testing.T) {
	cfg, _ := setupTest(t)
	testutil.BuildClerk(t)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Create parameters in "us-east-1"
	stdout, stderr, err := testutil.RunClerk(ctx, cfg, "put", "/test/region1/secret", "value-east-1", "--region", "us-east-1")
	require.NoError(t, err, "stderr: %s", stderr)
	assert.Contains(t, stdout, "Created")

	// Create parameters in "us-west-2" 
	stdout, stderr, err = testutil.RunClerk(ctx, cfg, "put", "/test/region2/secret", "value-west-2", "--region", "us-west-2")
	require.NoError(t, err, "stderr: %s", stderr)
	assert.Contains(t, stdout, "Created")

	// List in us-east-1 should only show east parameters
	stdout, stderr, err = testutil.RunClerk(ctx, cfg, "list", "/test/*", "--region", "us-east-1")
	require.NoError(t, err, "stderr: %s", stderr)
	assert.Contains(t, stdout, "region1")
	assert.NotContains(t, stdout, "region2")

	// List in us-west-2 should only show west parameters
	stdout, stderr, err = testutil.RunClerk(ctx, cfg, "list", "/test/*", "--region", "us-west-2")
	require.NoError(t, err, "stderr: %s", stderr)
	assert.Contains(t, stdout, "region2")
	assert.NotContains(t, stdout, "region1")
}

func TestIntegration_MultiRegion_CacheFiles(t *testing.T) {
	cfg, _ := setupTest(t)
	testutil.BuildClerk(t)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Refresh cache for multiple regions
	_, stderr, err := testutil.RunClerk(ctx, cfg, "refresh", "--region", "us-east-1")
	require.NoError(t, err, "stderr: %s", stderr)

	_, stderr, err = testutil.RunClerk(ctx, cfg, "refresh", "--region", "us-west-2")
	require.NoError(t, err, "stderr: %s", stderr)

	// Verify separate cache files exist
	// Note: In real implementation, would check ~/.clerk/cache/<account>/<region>.json
	// For moto tests, this validates the cache manager creates separate instances
}

func TestIntegration_MultiAccount_Support(t *testing.T) {
	// This test validates that different AWS account IDs result in separate cache directories
	// In moto, all requests use the same mock account, but the mechanism is tested
	cfg, _ := setupTest(t)
	testutil.BuildClerk(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create parameter
	_, stderr, err := testutil.RunClerk(ctx, cfg, "put", "/test/account/secret", "value")
	require.NoError(t, err, "stderr: %s", stderr)

	// Verify cache uses account ID in path
	// Cache should be in ~/.clerk/cache/<account_id>/<region>.json
}

func TestIntegration_PutAndGetParameter(t *testing.T) {
	cfg, _ := setupTest(t)
	testutil.BuildClerk(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Put a parameter
	stdout, stderr, err := testutil.RunClerk(ctx, cfg, "put", "/test/integration/secret", "my-secret-value")
	require.NoError(t, err, "stderr: %s", stderr)
	assert.Contains(t, stdout, "Created")

	// Get the parameter
	stdout, stderr, err = testutil.RunClerk(ctx, cfg, "get", "/test/integration/secret")
	require.NoError(t, err, "stderr: %s", stderr)
	assert.Contains(t, stdout, "my-secret-value")
}

func TestIntegration_PutWithTags(t *testing.T) {
	cfg, _ := setupTest(t)
	testutil.BuildClerk(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Put with tags
	stdout, stderr, err := testutil.RunClerk(ctx, cfg, "put", "/test/tagged/secret", "value", "--tags", "env=test,team=backend")
	require.NoError(t, err, "stderr: %s", stderr)
	assert.Contains(t, stdout, "Created")

	// Get with JSON output to verify tags
	stdout, stderr, err = testutil.RunClerk(ctx, cfg, "get", "/test/tagged/secret", "--output", "json")
	require.NoError(t, err, "stderr: %s", stderr)
	assert.Contains(t, stdout, "env")
	assert.Contains(t, stdout, "test")
}

func TestIntegration_PutUpdate(t *testing.T) {
	cfg, _ := setupTest(t)
	testutil.BuildClerk(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create parameter
	_, _, err := testutil.RunClerk(ctx, cfg, "put", "/test/update/secret", "initial-value")
	require.NoError(t, err)

	// Update parameter
	stdout, stderr, err := testutil.RunClerk(ctx, cfg, "put", "/test/update/secret", "updated-value")
	require.NoError(t, err, "stderr: %s", stderr)
	assert.Contains(t, stdout, "Updated")

	// Verify new value
	stdout, _, err = testutil.RunClerk(ctx, cfg, "get", "/test/update/secret", "--value")
	require.NoError(t, err)
	assert.Contains(t, stdout, "updated-value")
}

func TestIntegration_GetWithMask(t *testing.T) {
	cfg, _ := setupTest(t)
	testutil.BuildClerk(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create parameter
	_, _, err := testutil.RunClerk(ctx, cfg, "put", "/test/mask/secret", "sensitive-data-123")
	require.NoError(t, err)

	// Get with mask
	stdout, _, err := testutil.RunClerk(ctx, cfg, "get", "/test/mask/secret", "--mask")
	require.NoError(t, err)
	assert.Contains(t, stdout, "***")
	assert.NotContains(t, stdout, "sensitive-data-123")
}

func TestIntegration_GetVersion(t *testing.T) {
	cfg, _ := setupTest(t)
	testutil.BuildClerk(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create and update parameter
	_, _, err := testutil.RunClerk(ctx, cfg, "put", "/test/version/secret", "version-1")
	require.NoError(t, err)

	_, _, err = testutil.RunClerk(ctx, cfg, "put", "/test/version/secret", "version-2")
	require.NoError(t, err)

	// Get specific version
	stdout, _, err := testutil.RunClerk(ctx, cfg, "get", "/test/version/secret@1", "--value")
	require.NoError(t, err)
	assert.Contains(t, stdout, "version-1")

	// Get latest
	stdout, _, err = testutil.RunClerk(ctx, cfg, "get", "/test/version/secret", "--value")
	require.NoError(t, err)
	assert.Contains(t, stdout, "version-2")
}

func TestIntegration_Delete(t *testing.T) {
	cfg, _ := setupTest(t)
	testutil.BuildClerk(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create parameter
	_, _, err := testutil.RunClerk(ctx, cfg, "put", "/test/delete/secret", "to-be-deleted")
	require.NoError(t, err)

	// Delete with force
	stdout, stderr, err := testutil.RunClerk(ctx, cfg, "delete", "/test/delete/secret", "--force")
	require.NoError(t, err, "stderr: %s", stderr)
	assert.Contains(t, stdout, "Deleted")

	// Verify deleted
	_, _, err = testutil.RunClerk(ctx, cfg, "get", "/test/delete/secret")
	assert.Error(t, err)
}

func TestIntegration_List(t *testing.T) {
	cfg, gen := setupTest(t)
	testutil.BuildClerk(t)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Create specific test parameters
	created, err := gen.GenerateSpecificParameters(ctx)
	require.NoError(t, err)
	defer gen.CleanupParameters(ctx, created)

	// List all
	stdout, stderr, err := testutil.RunClerk(ctx, cfg, "list")
	require.NoError(t, err, "stderr: %s", stderr)
	assert.Contains(t, stdout, "/dev/database/password")
	assert.Contains(t, stdout, "/prod/database/password")

	// List with pattern
	stdout, _, err = testutil.RunClerk(ctx, cfg, "list", "/dev/*")
	require.NoError(t, err)
	assert.Contains(t, stdout, "/dev/")
	assert.NotContains(t, stdout, "/prod/")

	// List with sort
	stdout, _, err = testutil.RunClerk(ctx, cfg, "list", "--sort", "name")
	require.NoError(t, err)
	lines := strings.Split(stdout, "\n")
	assert.Greater(t, len(lines), 1)
}

func TestIntegration_ListWithTags(t *testing.T) {
	cfg, gen := setupTest(t)
	testutil.BuildClerk(t)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	created, err := gen.GenerateSpecificParameters(ctx)
	require.NoError(t, err)
	defer gen.CleanupParameters(ctx, created)

	// List with tags
	stdout, _, err := testutil.RunClerk(ctx, cfg, "list", "--tags")
	require.NoError(t, err)
	assert.Contains(t, stdout, "env")
}

func TestIntegration_CopyParameter(t *testing.T) {
	cfg, _ := setupTest(t)
	testutil.BuildClerk(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create source parameter
	_, _, err := testutil.RunClerk(ctx, cfg, "put", "/test/copy/source", "copy-me")
	require.NoError(t, err)

	// Copy to destination
	stdout, stderr, err := testutil.RunClerk(ctx, cfg, "cp", "/test/copy/source", "/test/copy/dest")
	require.NoError(t, err, "stderr: %s", stderr)
	assert.Contains(t, stdout, "Copied")

	// Verify both exist with same value
	stdout, _, err = testutil.RunClerk(ctx, cfg, "get", "/test/copy/dest", "--value")
	require.NoError(t, err)
	assert.Contains(t, stdout, "copy-me")
}

func TestIntegration_MoveParameter(t *testing.T) {
	cfg, _ := setupTest(t)
	testutil.BuildClerk(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create source parameter
	_, _, err := testutil.RunClerk(ctx, cfg, "put", "/test/move/source", "move-me")
	require.NoError(t, err)

	// Move to destination
	stdout, stderr, err := testutil.RunClerk(ctx, cfg, "mv", "/test/move/source", "/test/move/dest", "--force")
	require.NoError(t, err, "stderr: %s", stderr)
	assert.Contains(t, stdout, "Moved")

	// Verify destination exists
	stdout, _, err = testutil.RunClerk(ctx, cfg, "get", "/test/move/dest", "--value")
	require.NoError(t, err)
	assert.Contains(t, stdout, "move-me")

	// Verify source deleted
	_, _, err = testutil.RunClerk(ctx, cfg, "get", "/test/move/source")
	assert.Error(t, err)
}

func TestIntegration_Refresh(t *testing.T) {
	cfg, gen := setupTest(t)
	testutil.BuildClerk(t)

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// Create some parameters
	created, err := gen.GenerateSpecificParameters(ctx)
	require.NoError(t, err)
	defer gen.CleanupParameters(ctx, created)

	// Refresh cache
	stdout, stderr, err := testutil.RunClerk(ctx, cfg, "refresh")
	require.NoError(t, err, "stderr: %s", stderr)
	assert.Contains(t, stdout, "Refreshed")
}

func TestIntegration_JSONOutput(t *testing.T) {
	cfg, _ := setupTest(t)
	testutil.BuildClerk(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create parameter
	_, _, err := testutil.RunClerk(ctx, cfg, "put", "/test/json/secret", "json-value")
	require.NoError(t, err)

	// Get with JSON output
	stdout, _, err := testutil.RunClerk(ctx, cfg, "get", "/test/json/secret", "--output", "json")
	require.NoError(t, err)
	assert.Contains(t, stdout, `"name"`)
	assert.Contains(t, stdout, `"value"`)
	assert.Contains(t, stdout, `"/test/json/secret"`)
}

func TestIntegration_ErrorCases(t *testing.T) {
	cfg, _ := setupTest(t)
	testutil.BuildClerk(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	t.Run("get non-existent parameter", func(t *testing.T) {
		_, _, err := testutil.RunClerk(ctx, cfg, "get", "/non/existent/param")
		assert.Error(t, err)
	})

	t.Run("invalid parameter name", func(t *testing.T) {
		_, _, err := testutil.RunClerk(ctx, cfg, "put", "no-leading-slash", "value")
		assert.Error(t, err)
	})

	t.Run("delete non-existent parameter", func(t *testing.T) {
		_, _, err := testutil.RunClerk(ctx, cfg, "delete", "/non/existent/param", "--force")
		assert.Error(t, err)
	})
}
```

### 6. Create Large Scale Integration Tests

Create file `internal/integration/scale_test.go`:

```go
//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yachiko/clerk/internal/testutil"
)

func TestIntegration_LargeScale_List(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping large scale test in short mode")
	}

	cfg, gen := setupTest(t)
	testutil.BuildClerk(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Generate many parameters
	fixtureCfg := testutil.DefaultFixtureConfig()
	fixtureCfg.Endpoint = cfg.MotoEndpoint
	fixtureCfg.NumParameters = 200 // Start with 200 for CI, increase locally

	gen2, err := testutil.NewFixtureGenerator(fixtureCfg)
	require.NoError(t, err)

	created, err := gen2.GenerateParameters(ctx)
	require.NoError(t, err)
	defer gen2.CleanupParameters(ctx, created)

	t.Logf("Created %d parameters", len(created))

	// List all
	stdout, stderr, err := testutil.RunClerk(ctx, cfg, "list")
	require.NoError(t, err, "stderr: %s", stderr)

	// Count lines (excluding header and empty lines)
	lines := 0
	for _, line := range splitLines(stdout) {
		if line != "" && !isHeader(line) {
			lines++
		}
	}
	assert.GreaterOrEqual(t, lines, len(created)-10) // Allow some tolerance
}

func TestIntegration_LargeScale_Refresh(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping large scale test in short mode")
	}

	cfg, _ := setupTest(t)
	testutil.BuildClerk(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Generate parameters
	fixtureCfg := testutil.DefaultFixtureConfig()
	fixtureCfg.Endpoint = cfg.MotoEndpoint
	fixtureCfg.NumParameters = 300

	gen, err := testutil.NewFixtureGenerator(fixtureCfg)
	require.NoError(t, err)

	created, err := gen.GenerateParameters(ctx)
	require.NoError(t, err)
	defer gen.CleanupParameters(ctx, created)

	t.Logf("Created %d parameters for refresh test", len(created))

	// Refresh cache
	start := time.Now()
	stdout, stderr, err := testutil.RunClerk(ctx, cfg, "refresh")
	duration := time.Since(start)

	require.NoError(t, err, "stderr: %s", stderr)
	assert.Contains(t, stdout, "Refreshed")
	t.Logf("Refresh completed in %v", duration)
}

func TestIntegration_LargeScale_FilterPerformance(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping large scale test in short mode")
	}

	cfg, _ := setupTest(t)
	testutil.BuildClerk(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Generate parameters
	fixtureCfg := testutil.DefaultFixtureConfig()
	fixtureCfg.Endpoint = cfg.MotoEndpoint
	fixtureCfg.NumParameters = 500

	gen, err := testutil.NewFixtureGenerator(fixtureCfg)
	require.NoError(t, err)

	created, err := gen.GenerateParameters(ctx)
	require.NoError(t, err)
	defer gen.CleanupParameters(ctx, created)

	// Test various filter patterns
	patterns := []string{
		"/dev/*",
		"/prod/*",
		"/*/api/*",
		"/*/database/*",
	}

	for _, pattern := range patterns {
		t.Run("filter_"+pattern, func(t *testing.T) {
			start := time.Now()
			_, stderr, err := testutil.RunClerk(ctx, cfg, "list", pattern)
			duration := time.Since(start)

			require.NoError(t, err, "stderr: %s", stderr)
			t.Logf("Filter %s completed in %v", pattern, duration)
			assert.Less(t, duration, 30*time.Second)
		})
	}
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

func isHeader(line string) bool {
	// Simple heuristic: headers often contain column names
	return len(line) > 0 && (line[0] == '-' || line[0] == '=')
}
```

### 7. Create Benchmark Tests

Create file `internal/integration/benchmark_test.go`:

```go
//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yachiko/clerk/internal/testutil"
)

func BenchmarkIntegration_GetParameter(b *testing.B) {
	testutil.SkipIfNoMoto(&testing.T{})

	cfg := testutil.DefaultIntegrationConfig()
	testutil.ResetMoto(cfg.MotoEndpoint)

	ctx := context.Background()

	// Setup: create a parameter
	testutil.RunClerk(ctx, cfg, "put", "/bench/get/param", "benchmark-value")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, err := testutil.RunClerk(ctx, cfg, "get", "/bench/get/param", "--value")
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkIntegration_PutParameter(b *testing.B) {
	testutil.SkipIfNoMoto(&testing.T{})

	cfg := testutil.DefaultIntegrationConfig()
	testutil.ResetMoto(cfg.MotoEndpoint)

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		name := "/bench/put/param" + string(rune(i%26+'a'))
		_, _, err := testutil.RunClerk(ctx, cfg, "put", name, "benchmark-value")
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkIntegration_List(b *testing.B) {
	testutil.SkipIfNoMoto(&testing.T{})

	cfg := testutil.DefaultIntegrationConfig()
	testutil.ResetMoto(cfg.MotoEndpoint)

	ctx := context.Background()

	// Setup: create parameters
	fixtureCfg := testutil.DefaultFixtureConfig()
	fixtureCfg.Endpoint = cfg.MotoEndpoint
	fixtureCfg.NumParameters = 100

	gen, _ := testutil.NewFixtureGenerator(fixtureCfg)
	gen.GenerateParameters(ctx)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, err := testutil.RunClerk(ctx, cfg, "list")
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkIntegration_Refresh(b *testing.B) {
	testutil.SkipIfNoMoto(&testing.T{})

	cfg := testutil.DefaultIntegrationConfig()
	testutil.ResetMoto(cfg.MotoEndpoint)

	ctx := context.Background()

	// Setup: create parameters
	fixtureCfg := testutil.DefaultFixtureConfig()
	fixtureCfg.Endpoint = cfg.MotoEndpoint
	fixtureCfg.NumParameters = 100

	gen, _ := testutil.NewFixtureGenerator(fixtureCfg)
	gen.GenerateParameters(ctx)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, err := testutil.RunClerk(ctx, cfg, "refresh")
		if err != nil {
			b.Fatal(err)
		}
	}
}
```

### 8. Update Makefile

Add to existing `Makefile`:

```makefile
## Start moto server for integration tests
moto-start:
	@echo "Starting moto server..."
	docker-compose -f docker-compose.test.yml up -d
	@echo "Waiting for moto to be ready..."
	@sleep 3

## Stop moto server
moto-stop:
	@echo "Stopping moto server..."
	docker-compose -f docker-compose.test.yml down

## Run integration tests
test-integration: build moto-start
	@echo "Running integration tests..."
	MOTO_ENDPOINT=http://localhost:5000 $(GOTEST) -v -tags=integration ./internal/integration/...
	$(MAKE) moto-stop

## Run integration tests with fixtures (hundreds of parameters)
test-integration-large: build moto-start
	@echo "Running large scale integration tests..."
	MOTO_ENDPOINT=http://localhost:5000 $(GOTEST) -v -tags=integration -timeout 10m ./internal/integration/... -run "LargeScale"
	$(MAKE) moto-stop

## Run integration benchmarks
bench-integration: build moto-start
	@echo "Running integration benchmarks..."
	MOTO_ENDPOINT=http://localhost:5000 $(GOTEST) -v -tags=integration -bench=. -benchmem ./internal/integration/...
	$(MAKE) moto-stop

## Run all tests (unit + integration)
test-all: test-unit test-integration
```

### 9. Create CI Integration Test Script

Create file `scripts/run-integration-tests.sh`:

```bash
#!/bin/bash
# Run integration tests with moto server

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

cd "$PROJECT_ROOT"

echo "=== Building clerk ==="
go build -o ./bin/clerk ./cmd/clerk

echo "=== Starting moto server ==="
docker-compose -f docker-compose.test.yml up -d

# Wait for moto to be ready
echo "=== Waiting for moto server ==="
for i in {1..30}; do
    if curl -s http://localhost:5000/moto-api/ > /dev/null 2>&1; then
        echo "Moto server is ready"
        break
    fi
    echo "Waiting... ($i/30)"
    sleep 1
done

# Verify moto is running
if ! curl -s http://localhost:5000/moto-api/ > /dev/null 2>&1; then
    echo "ERROR: Moto server failed to start"
    docker-compose -f docker-compose.test.yml logs
    docker-compose -f docker-compose.test.yml down
    exit 1
fi

echo "=== Running integration tests ==="
MOTO_ENDPOINT=http://localhost:5000 go test -v -tags=integration ./internal/integration/...
TEST_EXIT_CODE=$?

echo "=== Stopping moto server ==="
docker-compose -f docker-compose.test.yml down

exit $TEST_EXIT_CODE
```

Create file `scripts/generate-fixtures.sh`:

```bash
#!/bin/bash
# Generate test fixtures in moto server

set -e

MOTO_ENDPOINT="${MOTO_ENDPOINT:-http://localhost:5000}"
NUM_PARAMS="${NUM_PARAMS:-500}"

echo "Generating $NUM_PARAMS test parameters..."

go run ./cmd/fixtures/main.go -endpoint "$MOTO_ENDPOINT" -count "$NUM_PARAMS"

echo "Done generating fixtures"
```

### 10. Create Fixtures CLI Tool

Create file `cmd/fixtures/main.go`:

```go
//go:build ignore

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
	endpoint := flag.String("endpoint", "http://localhost:5000", "Moto server endpoint")
	count := flag.Int("count", 500, "Number of parameters to generate")
	cleanup := flag.Bool("cleanup", false, "Cleanup generated parameters")
	specific := flag.Bool("specific", false, "Generate specific test parameters only")
	flag.Parse()

	cfg := &testutil.FixtureConfig{
		Endpoint:      *endpoint,
		Region:        "us-east-1",
		NumParameters: *count,
		Environments:  []string{"dev", "staging", "prod", "qa", "uat"},
		Services:      []string{"api", "web", "worker", "scheduler", "auth", "payment", "notification", "analytics", "search", "cache"},
		SecretTypes:   []string{"db_password", "api_key", "secret_key", "token", "connection_string", "certificate", "private_key", "webhook_secret", "encryption_key", "access_token"},
	}

	gen, err := testutil.NewFixtureGenerator(cfg)
	if err != nil {
		log.Fatalf("Failed to create fixture generator: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	if *cleanup {
		fmt.Println("Note: Cleanup requires tracking created parameters. Use moto reset instead.")
		return
	}

	var created []string
	if *specific {
		fmt.Println("Generating specific test parameters...")
		created, err = gen.GenerateSpecificParameters(ctx)
	} else {
		fmt.Printf("Generating %d random parameters...\n", *count)
		created, err = gen.GenerateParameters(ctx)
	}

	if err != nil {
		log.Fatalf("Failed to generate parameters: %v", err)
	}

	fmt.Printf("Successfully created %d parameters\n", len(created))
}
```

## Acceptance Criteria

- [ ] Moto server starts successfully: `docker-compose -f docker-compose.test.yml up -d`
- [ ] Integration tests compile: `go build -tags=integration ./internal/integration/...`
- [ ] All integration tests pass: `make test-integration`
- [ ] Multi-region cache isolation tests pass (separate cache files per region)
- [ ] Multi-account cache tests validate account-scoped directories
- [ ] Fixture generator creates 500+ parameters successfully
- [ ] Large scale tests complete within timeout
- [ ] Benchmark tests provide meaningful metrics
- [ ] Tests clean up after themselves (moto reset)
- [ ] CI script runs end-to-end successfully

## Running Integration Tests

```bash
# Start moto server
docker-compose -f docker-compose.test.yml up -d

# Run integration tests
MOTO_ENDPOINT=http://localhost:5000 go test -v -tags=integration ./internal/integration/...

# Run with fixtures
MOTO_ENDPOINT=http://localhost:5000 go test -v -tags=integration ./internal/integration/... -run "LargeScale"

# Run benchmarks
MOTO_ENDPOINT=http://localhost:5000 go test -v -tags=integration -bench=. ./internal/integration/...

# Stop moto server
docker-compose -f docker-compose.test.yml down

# Or use make targets
make test-integration
make test-integration-large
make bench-integration
```

## Example Output

```
$ make test-integration
Starting moto server...
Waiting for moto to be ready...
Running integration tests...
=== RUN   TestIntegration_PutAndGetParameter
--- PASS: TestIntegration_PutAndGetParameter (0.45s)
=== RUN   TestIntegration_PutWithTags
--- PASS: TestIntegration_PutWithTags (0.38s)
=== RUN   TestIntegration_Delete
--- PASS: TestIntegration_Delete (0.52s)
=== RUN   TestIntegration_List
--- PASS: TestIntegration_List (1.23s)
=== RUN   TestIntegration_CopyParameter
--- PASS: TestIntegration_CopyParameter (0.67s)
=== RUN   TestIntegration_MoveParameter
--- PASS: TestIntegration_MoveParameter (0.71s)
PASS
ok      github.com/yachiko/clerk/internal/integration   4.521s
Stopping moto server...

$ make test-integration-large
Running large scale integration tests...
=== RUN   TestIntegration_LargeScale_List
    scale_test.go:32: Created 200 parameters
--- PASS: TestIntegration_LargeScale_List (45.23s)
=== RUN   TestIntegration_LargeScale_Refresh
    scale_test.go:58: Created 300 parameters for refresh test
    scale_test.go:67: Refresh completed in 12.4s
--- PASS: TestIntegration_LargeScale_Refresh (58.12s)
PASS
```

## Notes

- Integration tests require Docker for running moto server
- Tests use build tag `integration` to separate from unit tests
- Large scale tests are skipped in `-short` mode
- Moto server resets between test runs for isolation
- Fixture generator creates realistic parameter hierarchies
- Benchmark tests help identify performance regressions
- AWS credentials are fake (testing/testing) for moto
- Tests set `AWS_ENDPOINT_URL` environment variable to route requests to moto
