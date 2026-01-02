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

// DescribeAllParameters retrieves metadata for all parameters
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
	if pattern == "*" || pattern == "/*" {
		return true
	}

	if strings.HasSuffix(pattern, "/*") {
		prefix := strings.TrimSuffix(pattern, "/*")
		return strings.HasPrefix(name, prefix+"/")
	}

	return pattern == name
}
