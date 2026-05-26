package testutil

import (
	"context"
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
)

// FixtureConfig configures test fixture generation.
type FixtureConfig struct {
	Endpoint      string
	Region        string
	NumParameters int
	Environments  []string
	Services      []string
	SecretTypes   []string
	// Parallel controls the upper bound on concurrent PutParameter calls.
	Parallel int
}

// DefaultFixtureConfig returns a sensible default fixture configuration.
func DefaultFixtureConfig() *FixtureConfig {
	return &FixtureConfig{
		Endpoint:      "http://localhost:5000",
		Region:        "us-east-1",
		NumParameters: 500,
		Environments:  []string{"dev", "staging", "prod", "qa", "uat"},
		Services:      []string{"api", "web", "worker", "scheduler", "auth", "payment", "notification", "analytics", "search", "cache"},
		SecretTypes:   []string{"db_password", "api_key", "secret_key", "token", "connection_string", "certificate", "private_key", "webhook_secret", "encryption_key", "access_token"},
		Parallel:      10,
	}
}

// FixtureGenerator drives PutParameter calls against an SSM endpoint (usually moto).
type FixtureGenerator struct {
	client *ssm.Client
	config *FixtureConfig
	rng    *rand.Rand
	rngMu  sync.Mutex
}

// NewFixtureGenerator builds a generator pointed at cfg.Endpoint.
func NewFixtureGenerator(cfg *FixtureConfig) (*FixtureGenerator, error) {
	if cfg == nil {
		cfg = DefaultFixtureConfig()
	}
	if cfg.Parallel <= 0 {
		cfg.Parallel = 10
	}

	awsCfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithRegion(cfg.Region),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("testing", "testing", "testing")),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	client := ssm.NewFromConfig(awsCfg, func(o *ssm.Options) {
		o.BaseEndpoint = aws.String(cfg.Endpoint)
	})

	return &FixtureGenerator{
		client: client,
		config: cfg,
		rng:    rand.New(rand.NewSource(time.Now().UnixNano())),
	}, nil
}

// Client exposes the underlying SSM client for advanced setup/verification.
func (g *FixtureGenerator) Client() *ssm.Client { return g.client }

// GenerateParameters creates NumParameters random parameters in moto using
// bounded concurrency. Returns the list of names actually created.
func (g *FixtureGenerator) GenerateParameters(ctx context.Context) ([]string, error) {
	names := make(chan string, g.config.NumParameters)
	errCh := make(chan error, 1)

	sem := make(chan struct{}, g.config.Parallel)
	var wg sync.WaitGroup

	for i := 0; i < g.config.NumParameters; i++ {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()

			if ctx.Err() != nil {
				return
			}

			name := g.generateParameterName()
			value := g.generateParameterValue()
			ptype := g.randomParameterType()

			input := &ssm.PutParameterInput{
				Name:  aws.String(name),
				Value: aws.String(value),
				Type:  ptype,
			}
			g.rngMu.Lock()
			addTags := g.rng.Float32() > 0.5
			g.rngMu.Unlock()
			if addTags {
				input.Tags = g.generateTags()
			}

			if _, err := g.client.PutParameter(ctx, input); err != nil {
				// Duplicate names are expected when N is large vs. the search space.
				if strings.Contains(err.Error(), "ParameterAlreadyExists") {
					return
				}
				select {
				case errCh <- fmt.Errorf("PutParameter %s: %w", name, err):
				default:
				}
				return
			}
			names <- name
		}()
	}

	wg.Wait()
	close(names)
	close(errCh)

	if err := <-errCh; err != nil {
		return nil, err
	}

	var created []string
	for n := range names {
		created = append(created, n)
	}
	return created, nil
}

// GenerateSpecificParameters creates a deterministic set used by table-driven tests.
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
			for k, v := range p.tags {
				input.Tags = append(input.Tags, types.Tag{
					Key:   aws.String(k),
					Value: aws.String(v),
				})
			}
		}
		if _, err := g.client.PutParameter(ctx, input); err != nil {
			return created, fmt.Errorf("PutParameter %s: %w", p.name, err)
		}
		created = append(created, p.name)
	}
	return created, nil
}

// CleanupParameters deletes the given parameters. Missing parameters are
// ignored so callers can use this in defer chains without race conditions.
func (g *FixtureGenerator) CleanupParameters(ctx context.Context, names []string) error {
	for _, name := range names {
		_, err := g.client.DeleteParameter(ctx, &ssm.DeleteParameterInput{
			Name: aws.String(name),
		})
		if err != nil && !strings.Contains(err.Error(), "ParameterNotFound") {
			return fmt.Errorf("DeleteParameter %s: %w", name, err)
		}
	}
	return nil
}

func (g *FixtureGenerator) generateParameterName() string {
	g.rngMu.Lock()
	defer g.rngMu.Unlock()

	env := g.config.Environments[g.rng.Intn(len(g.config.Environments))]
	service := g.config.Services[g.rng.Intn(len(g.config.Services))]
	secretType := g.config.SecretTypes[g.rng.Intn(len(g.config.SecretTypes))]
	if g.rng.Float32() > 0.7 {
		subs := []string{"primary", "replica", "backup", "external", "internal"}
		sub := subs[g.rng.Intn(len(subs))]
		// Append a small disambiguator to keep collisions rare across larger Ns.
		return fmt.Sprintf("/%s/%s/%s/%s-%d", env, service, sub, secretType, g.rng.Intn(10000))
	}
	return fmt.Sprintf("/%s/%s/%s-%d", env, service, secretType, g.rng.Intn(10000))
}

func (g *FixtureGenerator) generateParameterValue() string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	g.rngMu.Lock()
	defer g.rngMu.Unlock()
	length := g.rng.Intn(48) + 16
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[g.rng.Intn(len(charset))]
	}
	return string(b)
}

func (g *FixtureGenerator) randomParameterType() types.ParameterType {
	g.rngMu.Lock()
	defer g.rngMu.Unlock()
	roll := g.rng.Float32()
	switch {
	case roll < 0.7:
		return types.ParameterTypeSecureString
	case roll < 0.9:
		return types.ParameterTypeString
	default:
		return types.ParameterTypeStringList
	}
}

func (g *FixtureGenerator) generateTags() []types.Tag {
	tagValues := map[string][]string{
		"team":        {"backend", "frontend", "devops", "data", "security"},
		"cost-center": {"engineering", "operations", "infrastructure"},
		"project":     {"main-app", "microservices", "data-pipeline", "ml-platform"},
		"owner":       {"alice", "bob", "charlie", "david", "eve"},
		"criticality": {"high", "medium", "low"},
	}
	tagKeys := []string{"team", "cost-center", "project", "owner", "criticality"}

	g.rngMu.Lock()
	defer g.rngMu.Unlock()

	numTags := g.rng.Intn(3) + 1
	used := make(map[string]bool, numTags)
	var tags []types.Tag
	for i := 0; i < numTags; i++ {
		key := tagKeys[g.rng.Intn(len(tagKeys))]
		if used[key] {
			continue
		}
		used[key] = true
		vals := tagValues[key]
		tags = append(tags, types.Tag{
			Key:   aws.String(key),
			Value: aws.String(vals[g.rng.Intn(len(vals))]),
		})
	}
	return tags
}
