# Task 03: AWS SSM Client Module

## Objective
Implement the AWS Systems Manager (SSM) Parameter Store client wrapper that handles all AWS API interactions.

## Prerequisites
- Task 01 completed (project setup)
- Task 02 completed (configuration module)

## Deliverables

### 1. Create SSM Types

Create file `internal/aws/types.go`:

```go
package aws

import "time"

// Parameter represents a secret/parameter from AWS Parameter Store
type Parameter struct {
	Name             string            `json:"name"`
	Value            string            `json:"value,omitempty"`
	Type             string            `json:"type"`
	Version          int64             `json:"version"`
	LastModifiedDate time.Time         `json:"last_modified_date"`
	ARN              string            `json:"arn,omitempty"`
	DataType         string            `json:"data_type,omitempty"`
	Tags             map[string]string `json:"tags,omitempty"`
}

// ParameterMetadata represents metadata without the value
type ParameterMetadata struct {
	Name             string            `json:"name"`
	Type             string            `json:"type"`
	Version          int64             `json:"version"`
	LastModifiedDate time.Time         `json:"last_modified_date"`
	Tags             map[string]string `json:"tags,omitempty"`
}

// ParameterHistory represents a historical version of a parameter
type ParameterHistory struct {
	Name             string    `json:"name"`
	Value            string    `json:"value,omitempty"`
	Type             string    `json:"type"`
	Version          int64     `json:"version"`
	LastModifiedDate time.Time `json:"last_modified_date"`
	Labels           []string  `json:"labels,omitempty"`
}

// PutParameterInput represents input for creating/updating a parameter
type PutParameterInput struct {
	Name      string
	Value     string
	Type      string // String, StringList, SecureString
	Overwrite bool
	KMSKeyID  string            // Optional, for SecureString
	Tags      map[string]string // Only applied on create, not update
}

// PutParameterOutput represents output from put operation
type PutParameterOutput struct {
	Version int64
}
```

### 2. Create SSM Client

Create file `internal/aws/ssm.go`:

```go
package aws

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
)

// Client wraps the AWS SSM client
type Client struct {
	ssm    *ssm.Client
	region string
}

// ClientOptions contains options for creating a new client
type ClientOptions struct {
	Region  string
	Profile string
}

// NewClient creates a new AWS SSM client
func NewClient(ctx context.Context, opts ClientOptions) (*Client, error) {
	var cfgOpts []func(*config.LoadOptions) error

	if opts.Region != "" {
		cfgOpts = append(cfgOpts, config.WithRegion(opts.Region))
	}
	if opts.Profile != "" {
		cfgOpts = append(cfgOpts, config.WithSharedConfigProfile(opts.Profile))
	}

	cfg, err := config.LoadDefaultConfig(ctx, cfgOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	return &Client{
		ssm:    ssm.NewFromConfig(cfg),
		region: cfg.Region,
	}, nil
}

// GetParameter retrieves a parameter by name
func (c *Client) GetParameter(ctx context.Context, name string, withDecryption bool) (*Parameter, error) {
	input := &ssm.GetParameterInput{
		Name:           aws.String(name),
		WithDecryption: aws.Bool(withDecryption),
	}

	output, err := c.ssm.GetParameter(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to get parameter: %w", err)
	}

	p := output.Parameter
	param := &Parameter{
		Name:             aws.ToString(p.Name),
		Value:            aws.ToString(p.Value),
		Type:             string(p.Type),
		Version:          p.Version,
		LastModifiedDate: aws.ToTime(p.LastModifiedDate),
		ARN:              aws.ToString(p.ARN),
		DataType:         aws.ToString(p.DataType),
	}

	// Fetch tags separately
	tags, err := c.GetParameterTags(ctx, name)
	if err == nil {
		param.Tags = tags
	}

	return param, nil
}

// GetParameterByVersion retrieves a specific version of a parameter
func (c *Client) GetParameterByVersion(ctx context.Context, name string, version int64, withDecryption bool) (*Parameter, error) {
	versionedName := fmt.Sprintf("%s:%d", name, version)
	input := &ssm.GetParameterInput{
		Name:           aws.String(versionedName),
		WithDecryption: aws.Bool(withDecryption),
	}

	output, err := c.ssm.GetParameter(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to get parameter version: %w", err)
	}

	p := output.Parameter
	return &Parameter{
		Name:             aws.ToString(p.Name),
		Value:            aws.ToString(p.Value),
		Type:             string(p.Type),
		Version:          p.Version,
		LastModifiedDate: aws.ToTime(p.LastModifiedDate),
		ARN:              aws.ToString(p.ARN),
	}, nil
}

// GetParameterTags retrieves tags for a parameter
func (c *Client) GetParameterTags(ctx context.Context, name string) (map[string]string, error) {
	input := &ssm.ListTagsForResourceInput{
		ResourceType: types.ResourceTypeForTaggingParameter,
		ResourceId:   aws.String(name),
	}

	output, err := c.ssm.ListTagsForResource(ctx, input)
	if err != nil {
		return nil, err
	}

	tags := make(map[string]string)
	for _, tag := range output.TagList {
		tags[aws.ToString(tag.Key)] = aws.ToString(tag.Value)
	}

	return tags, nil
}

// PutParameter creates or updates a parameter
func (c *Client) PutParameter(ctx context.Context, input *PutParameterInput) (*PutParameterOutput, error) {
	ssmInput := &ssm.PutParameterInput{
		Name:      aws.String(input.Name),
		Value:     aws.String(input.Value),
		Type:      types.ParameterType(input.Type),
		Overwrite: aws.Bool(input.Overwrite),
	}

	if input.KMSKeyID != "" && input.Type == "SecureString" {
		ssmInput.KeyId = aws.String(input.KMSKeyID)
	}

	// Add tags only if provided and not overwriting
	if len(input.Tags) > 0 && !input.Overwrite {
		var tags []types.Tag
		for k, v := range input.Tags {
			tags = append(tags, types.Tag{
				Key:   aws.String(k),
				Value: aws.String(v),
			})
		}
		ssmInput.Tags = tags
	}

	output, err := c.ssm.PutParameter(ctx, ssmInput)
	if err != nil {
		return nil, fmt.Errorf("failed to put parameter: %w", err)
	}

	return &PutParameterOutput{
		Version: output.Version,
	}, nil
}

// DeleteParameter deletes a parameter
func (c *Client) DeleteParameter(ctx context.Context, name string) error {
	input := &ssm.DeleteParameterInput{
		Name: aws.String(name),
	}

	_, err := c.ssm.DeleteParameter(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to delete parameter: %w", err)
	}

	return nil
}

// GetParameterHistory retrieves version history for a parameter
func (c *Client) GetParameterHistory(ctx context.Context, name string, maxResults int32, withDecryption bool) ([]ParameterHistory, error) {
	input := &ssm.GetParameterHistoryInput{
		Name:           aws.String(name),
		WithDecryption: aws.Bool(withDecryption),
		MaxResults:     aws.Int32(maxResults),
	}

	var history []ParameterHistory
	paginator := ssm.NewGetParameterHistoryPaginator(c.ssm, input)

	for paginator.HasMorePages() {
		output, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to get parameter history: %w", err)
		}

		for _, h := range output.Parameters {
			history = append(history, ParameterHistory{
				Name:             aws.ToString(h.Name),
				Value:            aws.ToString(h.Value),
				Type:             string(h.Type),
				Version:          h.Version,
				LastModifiedDate: aws.ToTime(h.LastModifiedDate),
				Labels:           h.Labels,
			})
		}

		if int32(len(history)) >= maxResults {
			break
		}
	}

	return history, nil
}

// ListParameters lists all parameters matching a path
func (c *Client) ListParameters(ctx context.Context, path string, recursive bool) ([]ParameterMetadata, error) {
	var params []ParameterMetadata

	// Use DescribeParameters for glob patterns, GetParametersByPath for path prefix
	if strings.Contains(path, "*") {
		return c.listParametersWithFilter(ctx, path)
	}

	input := &ssm.GetParametersByPathInput{
		Path:           aws.String(path),
		Recursive:      aws.Bool(recursive),
		WithDecryption: aws.Bool(false),
	}

	paginator := ssm.NewGetParametersByPathPaginator(c.ssm, input)

	for paginator.HasMorePages() {
		output, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list parameters: %w", err)
		}

		for _, p := range output.Parameters {
			params = append(params, ParameterMetadata{
				Name:             aws.ToString(p.Name),
				Type:             string(p.Type),
				Version:          p.Version,
				LastModifiedDate: aws.ToTime(p.LastModifiedDate),
			})
		}
	}

	return params, nil
}

// listParametersWithFilter uses DescribeParameters with filters
func (c *Client) listParametersWithFilter(ctx context.Context, pattern string) ([]ParameterMetadata, error) {
	var params []ParameterMetadata

	input := &ssm.DescribeParametersInput{}

	paginator := ssm.NewDescribeParametersPaginator(c.ssm, input)

	for paginator.HasMorePages() {
		output, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to describe parameters: %w", err)
		}

		for _, p := range output.Parameters {
			name := aws.ToString(p.Name)
			if matchGlob(pattern, name) {
				params = append(params, ParameterMetadata{
					Name:             name,
					Type:             string(p.Type),
					Version:          p.Version,
					LastModifiedDate: aws.ToTime(p.LastModifiedDate),
				})
			}
		}
	}

	return params, nil
}

// DescribeAllParameters retrieves metadata for all parameters (for cache refresh)
func (c *Client) DescribeAllParameters(ctx context.Context) ([]ParameterMetadata, error) {
	var params []ParameterMetadata

	input := &ssm.DescribeParametersInput{}
	paginator := ssm.NewDescribeParametersPaginator(c.ssm, input)

	for paginator.HasMorePages() {
		output, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to describe parameters: %w", err)
		}

		for _, p := range output.Parameters {
			params = append(params, ParameterMetadata{
				Name:             aws.ToString(p.Name),
				Type:             string(p.Type),
				Version:          p.Version,
				LastModifiedDate: aws.ToTime(p.LastModifiedDate),
			})
		}
	}

	return params, nil
}

// matchGlob performs simple glob pattern matching
func matchGlob(pattern, name string) bool {
	// Simple glob: * matches any sequence of characters
	if pattern == "*" || pattern == "/*" {
		return true
	}

	// Convert glob to prefix match for /path/*
	if strings.HasSuffix(pattern, "/*") {
		prefix := strings.TrimSuffix(pattern, "/*")
		return strings.HasPrefix(name, prefix+"/")
	}

	// Exact match
	return pattern == name
}
```

### 3. Create AWS Error Handling

Create file `internal/aws/errors.go`:

```go
package aws

import (
	"errors"

	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
)

// IsParameterNotFoundError checks if the error is a parameter not found error
func IsParameterNotFoundError(err error) bool {
	var pnf *types.ParameterNotFound
	return errors.As(err, &pnf)
}

// IsParameterAlreadyExistsError checks if the error is a parameter already exists error
func IsParameterAlreadyExistsError(err error) bool {
	var pae *types.ParameterAlreadyExists
	return errors.As(err, &pae)
}

// IsAccessDeniedError checks if the error is an access denied error
func IsAccessDeniedError(err error) bool {
	// Check for common access denied error patterns
	if err == nil {
		return false
	}
	errStr := err.Error()
	return contains(errStr, "AccessDenied") || contains(errStr, "UnauthorizedAccess")
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsImpl(s, substr))
}

func containsImpl(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
```

## Acceptance Criteria

- [ ] Client can be created with optional region and profile
- [ ] `GetParameter` retrieves a parameter with decryption
- [ ] `GetParameterByVersion` retrieves a specific version
- [ ] `PutParameter` creates new parameters
- [ ] `PutParameter` updates existing parameters with `Overwrite: true`
- [ ] `DeleteParameter` removes a parameter
- [ ] `GetParameterHistory` retrieves version history
- [ ] `ListParameters` lists parameters by path
- [ ] Glob pattern matching works for listing (`/dev/*`)
- [ ] Tags are retrieved and included in Parameter struct
- [ ] Error types are correctly identified

## Testing Commands (Manual)

```bash
# After implementing, test with real AWS account:
# Requires AWS credentials configured

# Test put
clerk put "/test/secret" "test-value"

# Test get
clerk get "/test/secret"

# Test delete
clerk delete "/test/secret" --force
```

## Notes

- Use `aws.ToString()` and `aws.ToTime()` for safe pointer dereferencing
- Always use context for cancellation support
- Pagination is handled automatically for list operations
- Tags are only applied during creation, not update (AWS limitation)
- For SecureString parameters, `WithDecryption: true` is needed to see the actual value
