package aws

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/yachiko/clerk/internal/parammatch"
)

// Client wraps the AWS SSM client
type Client struct {
	ssm              *ssm.Client
	sts              *sts.Client
	region           string
	accountID        string
	describePageSize int32
	describeMaxItems int32
}

// ClientOptions contains options for creating a new client
type ClientOptions struct {
	Region           string
	Profile          string
	DescribePageSize int32
	DescribeMaxItems int32
}

// NewClient creates a new AWS SSM client
func NewClient(ctx context.Context, opts ClientOptions) (*Client, error) {
	var cfgOpts []func(*config.LoadOptions) error

	if opts.Region != "" {
		cfgOpts = append(cfgOpts, config.WithRegion(opts.Region))
	}
	// Skip WithSharedConfigProfile for the literal "default" profile: passing
	// it explicitly forces the SDK to validate the profile exists in a config
	// file, which fails on machines that rely purely on AWS_* env vars and
	// have no ~/.aws/config. The SDK's intrinsic behavior already picks the
	// "default" profile when none is specified.
	if opts.Profile != "" && opts.Profile != "default" {
		cfgOpts = append(cfgOpts, config.WithSharedConfigProfile(opts.Profile))
	}

	cfg, err := config.LoadDefaultConfig(ctx, cfgOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	pageSize := opts.DescribePageSize
	if pageSize == 0 {
		pageSize = 50 // Default to maximum
	}

	// Create STS client to get account ID
	stsClient := sts.NewFromConfig(cfg)

	// Get account ID
	identity, err := stsClient.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return nil, fmt.Errorf("failed to get AWS account ID: %w", err)
	}

	return &Client{
		ssm:              ssm.NewFromConfig(cfg),
		sts:              stsClient,
		region:           cfg.Region,
		accountID:        aws.ToString(identity.Account),
		describePageSize: pageSize,
		describeMaxItems: opts.DescribeMaxItems,
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

	return param, nil
}

func (c *Client) getParameterMetadata(ctx context.Context, name string) (*Parameter, error) {
	output, err := c.ssm.DescribeParameters(ctx, &ssm.DescribeParametersInput{ParameterFilters: []types.ParameterStringFilter{{Key: aws.String("Name"), Values: []string{name}}}})
	if err != nil {
		return nil, fmt.Errorf("failed to describe parameter: %w", err)
	}
	for _, p := range output.Parameters {
		if aws.ToString(p.Name) != name {
			continue
		}
		policies, err := json.Marshal(p.Policies)
		if err != nil {
			return nil, fmt.Errorf("failed to encode parameter policies: %w", err)
		}
		return &Parameter{Name: aws.ToString(p.Name), Type: string(p.Type), ARN: aws.ToString(p.ARN), DataType: aws.ToString(p.DataType), Description: aws.ToString(p.Description), KMSKeyID: aws.ToString(p.KeyId), Tier: string(p.Tier), AllowedPattern: aws.ToString(p.AllowedPattern), Policies: string(policies)}, nil
	}
	return nil, fmt.Errorf("parameter metadata not found: %s", name)
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
	if input.Description != "" {
		ssmInput.Description = aws.String(input.Description)
	}
	if input.Tier != "" {
		ssmInput.Tier = types.ParameterTier(input.Tier)
	}
	if input.AllowedPattern != "" {
		ssmInput.AllowedPattern = aws.String(input.AllowedPattern)
	}
	if input.Policies != "" {
		ssmInput.Policies = aws.String(input.Policies)
	}
	if input.DataType != "" {
		ssmInput.DataType = aws.String(input.DataType)
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
	// PutParameter ignores tags on an overwrite. Apply supplied tags only after
	// the value write succeeds so callers can report a truthful partial result.
	if input.Overwrite && len(input.Tags) > 0 {
		var tags []types.Tag
		for k, v := range input.Tags {
			tags = append(tags, types.Tag{Key: aws.String(k), Value: aws.String(v)})
		}
		if _, err := c.ssm.AddTagsToResource(ctx, &ssm.AddTagsToResourceInput{ResourceType: types.ResourceTypeForTaggingParameter, ResourceId: aws.String(input.Name), Tags: tags}); err != nil {
			return nil, fmt.Errorf("parameter value was written but tags were not updated: %w", err)
		}
	}

	return &PutParameterOutput{
		Version: output.Version,
	}, nil
}

// Transfer copies decrypted source bytes and supported destination metadata. It
// never deletes a destination as rollback; callers receive a partial outcome.
func (c *Client) Transfer(ctx context.Context, input TransferInput) (TransferResult, error) {
	result := TransferResult{}
	if canonicalParameterName(input.Source) == canonicalParameterName(input.Destination) {
		return result, fmt.Errorf("source and destination identify the same parameter")
	}
	source, err := c.GetParameter(ctx, input.Source, true)
	if err != nil {
		return result, fmt.Errorf("read source (including decryption): %w", err)
	}
	// Metadata and tags are part of a faithful transfer. Unlike ordinary reads,
	// do not silently drop them when authorization is missing.
	metadata, err := c.getParameterMetadata(ctx, source.Name)
	if err != nil {
		return result, fmt.Errorf("read source metadata: %w", err)
	}
	source.Description, source.KMSKeyID, source.Tier = metadata.Description, metadata.KMSKeyID, metadata.Tier
	source.AllowedPattern, source.Policies, source.DataType = metadata.AllowedPattern, metadata.Policies, metadata.DataType
	source.Tags, err = c.GetParameterTags(ctx, source.Name)
	if err != nil {
		return result, fmt.Errorf("read source tags: %w", err)
	}
	result.Source = source
	if source.Name == canonicalParameterName(input.Destination) {
		return result, fmt.Errorf("source and destination identify the same parameter")
	}
	destination, err := c.GetParameter(ctx, input.Destination, false)
	if err == nil && !input.Overwrite {
		return result, fmt.Errorf("destination already exists: %s (use --overwrite to replace it)", input.Destination)
	}
	if err != nil && !IsParameterNotFoundError(err) {
		return result, fmt.Errorf("check destination: %w", err)
	}
	put := &PutParameterInput{Name: input.Destination, Value: source.Value, Type: source.Type, Overwrite: destination != nil, KMSKeyID: source.KMSKeyID, Tags: source.Tags, Description: source.Description, Tier: source.Tier, AllowedPattern: source.AllowedPattern, Policies: source.Policies, DataType: source.DataType}
	if _, err := c.PutParameter(ctx, put); err != nil {
		return result, fmt.Errorf("write destination: %w", err)
	}
	result.DestinationWritten = true
	verified, err := c.GetParameter(ctx, input.Destination, true)
	if err != nil {
		return result, fmt.Errorf("destination was written but could not be verified: %w", err)
	}
	result.Destination = verified
	if verified.Type != source.Type || verified.Value != source.Value {
		return result, fmt.Errorf("destination was written but verification failed; source retained")
	}
	if !input.Move {
		return result, nil
	}
	current, err := c.GetParameter(ctx, input.Source, true)
	if err != nil {
		return result, fmt.Errorf("destination verified but source could not be rechecked; source retained: %w", err)
	}
	if current.Version != source.Version || current.Value != source.Value {
		return result, fmt.Errorf("destination verified but source changed during transfer; source retained")
	}
	if err := c.DeleteParameter(ctx, input.Source); err != nil {
		return result, fmt.Errorf("destination verified but failed to delete source; both parameters remain: %w", err)
	}
	result.SourceDeleted = true
	return result, nil
}

func canonicalParameterName(name string) string {
	if i := strings.Index(name, ":parameter/"); i >= 0 {
		return "/" + strings.TrimPrefix(name[i+len(":parameter/"):], "/")
	}
	if i := strings.LastIndex(name, ":"); i > 0 {
		return name[:i]
	}
	return name
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

// GetParameterHistory retrieves all available version history. pageSize controls
// the service page size only; it is deliberately not a total-result cap.
func (c *Client) GetParameterHistory(ctx context.Context, name string, pageSize int32, withDecryption bool) ([]ParameterHistory, error) {
	if pageSize <= 0 {
		pageSize = 50
	}
	input := &ssm.GetParameterHistoryInput{
		Name:           aws.String(name),
		WithDecryption: aws.Bool(withDecryption),
		MaxResults:     aws.Int32(pageSize),
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

	}

	return history, nil
}

// ListParameters lists metadata only. DescribeParameters avoids returning
// ordinary String values as an incidental result of inventory browsing.
func (c *Client) ListParameters(ctx context.Context, path string, recursive bool) ([]ParameterMetadata, error) {
	var params []ParameterMetadata
	_ = recursive // retained for source compatibility
	input := &ssm.DescribeParametersInput{MaxResults: aws.Int32(c.describePageSize)}
	paginator := ssm.NewDescribeParametersPaginator(c.ssm, input)

	for paginator.HasMorePages() {
		output, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to describe parameters: %w", err)
		}

		for _, p := range output.Parameters {
			ok, matchErr := parammatch.Match(path, aws.ToString(p.Name))
			if matchErr != nil {
				return nil, matchErr
			}
			if !ok {
				continue
			}
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

// ListParametersByPath is the explicit compatibility path for installations
// granted only ssm:GetParametersByPath. It is intentionally separate from the
// inventory API because SSM includes String values in this response even when
// WithDecryption is false. Callers must treat it as a scoped-read operation.
func (c *Client) ListParametersByPath(ctx context.Context, path string, recursive bool) ([]ParameterMetadata, error) {
	input := &ssm.GetParametersByPathInput{Path: aws.String(path), Recursive: aws.Bool(recursive), WithDecryption: aws.Bool(false)}
	paginator := ssm.NewGetParametersByPathPaginator(c.ssm, input)
	var params []ParameterMetadata
	for paginator.HasMorePages() {
		output, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list parameters by path: %w", err)
		}
		for _, p := range output.Parameters {
			params = append(params, ParameterMetadata{Name: aws.ToString(p.Name), Type: string(p.Type), Version: p.Version, LastModifiedDate: aws.ToTime(p.LastModifiedDate)})
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
			if ok, _ := parammatch.Match(pattern, name); ok {
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

	input := &ssm.DescribeParametersInput{
		MaxResults: aws.Int32(c.describePageSize),
	}
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

			// Check max-items limit
			if c.describeMaxItems > 0 && int32(len(params)) >= c.describeMaxItems {
				return params[:c.describeMaxItems], nil
			}
		}
	}

	return params, nil
}

// DescribeParametersStream sends parameters to a channel as they're discovered
func (c *Client) DescribeParametersStream(ctx context.Context, ch chan<- ParameterMetadata) (DescribeResult, error) {
	defer close(ch)

	input := &ssm.DescribeParametersInput{
		MaxResults: aws.Int32(c.describePageSize),
	}
	paginator := ssm.NewDescribeParametersPaginator(c.ssm, input)

	var count int32
	for paginator.HasMorePages() {
		if ctx.Err() != nil {
			return DescribeResult{}, ctx.Err()
		}

		output, err := paginator.NextPage(ctx)
		if err != nil {
			return DescribeResult{}, fmt.Errorf("failed to describe parameters: %w", err)
		}

		for _, p := range output.Parameters {
			// Check max-items limit before sending
			if c.describeMaxItems > 0 && count >= c.describeMaxItems {
				return DescribeResult{Truncated: true}, nil
			}

			select {
			case <-ctx.Done():
				return DescribeResult{}, ctx.Err()
			case ch <- ParameterMetadata{
				Name:             aws.ToString(p.Name),
				Type:             string(p.Type),
				Version:          p.Version,
				LastModifiedDate: aws.ToTime(p.LastModifiedDate),
			}:
				count++
			}
		}
	}

	return DescribeResult{Complete: true}, nil
}

// GetRegion returns the AWS region for this client
func (c *Client) GetRegion() string {
	return c.region
}

// GetAccountID returns the AWS account ID for this client
func (c *Client) GetAccountID() string {
	return c.accountID
}

// LabelParameterVersion adds or moves labels to a parameter version
// If a label already exists on another version, it will be moved
func (c *Client) LabelParameterVersion(ctx context.Context, input *LabelParameterInput) (*LabelParameterOutput, error) {
	ssmInput := &ssm.LabelParameterVersionInput{
		Name:             aws.String(input.Name),
		ParameterVersion: aws.Int64(input.Version),
		Labels:           input.Labels,
	}

	output, err := c.ssm.LabelParameterVersion(ctx, ssmInput)
	if err != nil {
		return nil, fmt.Errorf("failed to label parameter version: %w", err)
	}

	return &LabelParameterOutput{
		InvalidLabels: output.InvalidLabels,
		Version:       input.Version,
	}, nil
}

// UnlabelParameterVersion removes labels from a parameter version
func (c *Client) UnlabelParameterVersion(ctx context.Context, input *UnlabelParameterInput) error {
	ssmInput := &ssm.UnlabelParameterVersionInput{
		Name:             aws.String(input.Name),
		ParameterVersion: aws.Int64(input.Version),
		Labels:           input.Labels,
	}

	_, err := c.ssm.UnlabelParameterVersion(ctx, ssmInput)
	if err != nil {
		return fmt.Errorf("failed to unlabel parameter version: %w", err)
	}

	return nil
}

// AddTagsToResource adds tags to a parameter
func (c *Client) AddTagsToResource(ctx context.Context, name string, tags map[string]string) error {
	var tagList []types.Tag
	for k, v := range tags {
		tagList = append(tagList, types.Tag{
			Key:   aws.String(k),
			Value: aws.String(v),
		})
	}

	input := &ssm.AddTagsToResourceInput{
		ResourceType: types.ResourceTypeForTaggingParameter,
		ResourceId:   aws.String(name),
		Tags:         tagList,
	}

	_, err := c.ssm.AddTagsToResource(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to add tags: %w", err)
	}

	return nil
}

// RemoveTagsFromResource removes tags from a parameter by key
func (c *Client) RemoveTagsFromResource(ctx context.Context, name string, tagKeys []string) error {
	input := &ssm.RemoveTagsFromResourceInput{
		ResourceType: types.ResourceTypeForTaggingParameter,
		ResourceId:   aws.String(name),
		TagKeys:      tagKeys,
	}

	_, err := c.ssm.RemoveTagsFromResource(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to remove tags: %w", err)
	}

	return nil
}

// GetParameterByLabel retrieves a parameter version by label
func (c *Client) GetParameterByLabel(ctx context.Context, name, label string, withDecryption bool) (*Parameter, error) {
	labeledName := fmt.Sprintf("%s:%s", name, label)
	input := &ssm.GetParameterInput{
		Name:           aws.String(labeledName),
		WithDecryption: aws.Bool(withDecryption),
	}

	output, err := c.ssm.GetParameter(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to get parameter by label: %w", err)
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

// FindLabelVersion finds which version has a specific label
func (c *Client) FindLabelVersion(ctx context.Context, name, label string) (int64, error) {
	param, err := c.GetParameterByLabel(ctx, name, label, false)
	if err != nil {
		return 0, err
	}
	return param.Version, nil
}
