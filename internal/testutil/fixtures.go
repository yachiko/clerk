package testutil

import (
	"context"
	"fmt"
	"math/rand"
	"strings"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	smtypes "github.com/aws/aws-sdk-go-v2/service/secretsmanager/types"
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

// FixtureResult describes resources created by Populate.
type FixtureResult struct {
	Parameters []string
	Secrets    []string
	Warnings   []string
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
	client  *ssm.Client
	secrets *secretsmanager.Client
	config  *FixtureConfig
	rng     *rand.Rand
	rngMu   sync.Mutex
}

// NewFixtureGenerator builds a generator pointed at cfg.Endpoint.
func NewFixtureGenerator(cfg *FixtureConfig) (*FixtureGenerator, error) {
	if cfg == nil {
		cfg = DefaultFixtureConfig()
	}
	if cfg.NumParameters < 0 {
		return nil, fmt.Errorf("NumParameters must not be negative")
	}
	if cfg.Endpoint == "" {
		cfg.Endpoint = "http://localhost:5000"
	}
	if cfg.Region == "" {
		cfg.Region = "us-east-1"
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
	secretsClient := secretsmanager.NewFromConfig(awsCfg, func(o *secretsmanager.Options) {
		o.BaseEndpoint = aws.String(cfg.Endpoint)
	})

	return &FixtureGenerator{
		client:  client,
		secrets: secretsClient,
		config:  cfg,
		rng:     rand.New(rand.NewSource(1)),
	}, nil
}

// Client exposes the underlying SSM client for advanced setup/verification.
func (g *FixtureGenerator) Client() *ssm.Client { return g.client }

// GenerateParameters creates NumParameters random parameters in moto using
// bounded concurrency. Returns the list of names actually created.
func (g *FixtureGenerator) GenerateParameters(ctx context.Context) ([]string, error) {
	return g.generateParameters(ctx, g.config.NumParameters)
}

func (g *FixtureGenerator) generateParameters(ctx context.Context, count int) ([]string, error) {
	names := make(chan string, count)
	errCh := make(chan error, 1)

	sem := make(chan struct{}, g.config.Parallel)
	var wg sync.WaitGroup

	for i := 0; i < count; i++ {
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

type secretFixture struct {
	name      string
	value     string
	binary    []byte
	tags      map[string]string
	versioned bool
}

func fixtureSecrets() []secretFixture {
	return []secretFixture{
		{name: "/dev/database/password", value: "secret-manager-dev-password", tags: map[string]string{"env": "dev", "team": "backend"}, versioned: true},
		{name: "/test/plain/text", value: "plain secret text", tags: map[string]string{"kind": "text"}},
		{name: "/test/json/unicode", value: `{"message":"こんにちは, café","emoji":"🔐","enabled":true}`, tags: map[string]string{"kind": "json", "encoding": "unicode"}},
		{name: "/test/binary/certificate", binary: []byte{0x00, 0x01, 0x02, 0xfe, 0xff, 'f', 'i', 'x'}, tags: map[string]string{"kind": "binary"}},
	}
}

// Populate creates the deterministic browsing corpus and NumParameters filler
// parameters. Secret creation uses the same endpoint and credentials as SSM.
func (g *FixtureGenerator) Populate(ctx context.Context) (FixtureResult, error) {
	parameters, err := g.GenerateSpecificParameters(ctx)
	if err != nil {
		return FixtureResult{Parameters: parameters}, err
	}
	remaining := g.config.NumParameters - len(parameters)
	if remaining < 0 {
		remaining = 0
	}
	filler, err := g.generateParameters(ctx, remaining)
	parameters = append(parameters, filler...)
	if err != nil {
		return FixtureResult{Parameters: parameters}, err
	}

	result := FixtureResult{Parameters: parameters}
	for _, fixture := range fixtureSecrets() {
		input := &secretsmanager.CreateSecretInput{Name: aws.String(fixture.name), Tags: secretTags(fixture.tags)}
		if fixture.binary != nil {
			input.SecretBinary = fixture.binary
		} else {
			input.SecretString = aws.String(fixture.value)
		}
		created, createErr := g.secrets.CreateSecret(ctx, input)
		if createErr != nil {
			return result, fmt.Errorf("CreateSecret %s: %w", fixture.name, createErr)
		}
		result.Secrets = append(result.Secrets, fixture.name)
		if !fixture.versioned {
			continue
		}
		version, versionErr := g.secrets.PutSecretValue(ctx, &secretsmanager.PutSecretValueInput{
			SecretId:           aws.String(fixture.name),
			ClientRequestToken: aws.String("fixture-version-2"),
			SecretString:       aws.String(`{"password":"rotated-secret","version":2}`),
			VersionStages:      []string{"AWSCURRENT", "fixture-rotated"},
		})
		if versionErr != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("%s: version creation unavailable: %v", fixture.name, versionErr))
			continue
		}
		if created.VersionId != nil && version.VersionId != nil {
			_, labelErr := g.secrets.UpdateSecretVersionStage(ctx, &secretsmanager.UpdateSecretVersionStageInput{
				SecretId: aws.String(fixture.name), VersionStage: aws.String("fixture-initial"), MoveToVersionId: created.VersionId,
			})
			if labelErr != nil {
				result.Warnings = append(result.Warnings, fmt.Sprintf("%s: custom staging label unavailable: %v", fixture.name, labelErr))
			}
		}
	}
	return result, nil
}

func secretTags(tags map[string]string) []smtypes.Tag {
	result := make([]smtypes.Tag, 0, len(tags))
	for key, value := range tags {
		result = append(result, smtypes.Tag{Key: aws.String(key), Value: aws.String(value)})
	}
	return result
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
