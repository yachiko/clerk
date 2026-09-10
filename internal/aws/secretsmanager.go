package aws

import (
	"context"
	"encoding/base64"
	"fmt"

	awsSDK "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/yachiko/clerk/internal/parammatch"
)

var errReadOnlySecretsManager = fmt.Errorf("the secretsmanager backend is read-only; put, delete, cp, and mv are not supported")

type secretsManagerAPI interface {
	DescribeSecret(context.Context, *secretsmanager.DescribeSecretInput, ...func(*secretsmanager.Options)) (*secretsmanager.DescribeSecretOutput, error)
	GetSecretValue(context.Context, *secretsmanager.GetSecretValueInput, ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error)
	ListSecrets(context.Context, *secretsmanager.ListSecretsInput, ...func(*secretsmanager.Options)) (*secretsmanager.ListSecretsOutput, error)
}

func newSecretsManagerClient(cfg awsSDK.Config) secretsManagerAPI {
	return secretsmanager.NewFromConfig(cfg)
}

func (c *Client) getSecret(ctx context.Context, name, stage, versionID string) (*Parameter, error) {
	in := &secretsmanager.GetSecretValueInput{SecretId: awsSDK.String(name)}
	if stage != "" {
		in.VersionStage = awsSDK.String(stage)
	}
	if versionID != "" {
		in.VersionId = awsSDK.String(versionID)
	}
	out, err := c.secretsManager.GetSecretValue(ctx, in)
	if err != nil {
		return nil, fmt.Errorf("failed to get secret: %w", err)
	}
	p := &Parameter{Name: name, Type: "Secret", ARN: awsSDK.ToString(out.ARN), VersionID: awsSDK.ToString(out.VersionId), LastModifiedDate: awsSDK.ToTime(out.CreatedDate)}
	if out.SecretString != nil {
		p.Value = *out.SecretString
	} else {
		p.Value = base64.StdEncoding.EncodeToString(out.SecretBinary)
		p.Binary = true
	}
	return p, nil
}

// GetSecret retrieves a Secrets Manager secret with an optional version stage
// or opaque version ID. Callers must ensure the selectors are mutually exclusive.
func (c *Client) GetSecret(ctx context.Context, name, stage, versionID string) (*Parameter, error) {
	if c.backend != "secretsmanager" {
		return nil, fmt.Errorf("GetSecret requires the secretsmanager backend")
	}
	return c.getSecret(ctx, name, stage, versionID)
}

func (c *Client) getSecretTags(ctx context.Context, name string) (map[string]string, error) {
	out, err := c.secretsManager.DescribeSecret(ctx, &secretsmanager.DescribeSecretInput{SecretId: awsSDK.String(name)})
	if err != nil {
		return nil, fmt.Errorf("failed to describe secret tags: %w", err)
	}
	tags := make(map[string]string, len(out.Tags))
	for _, tag := range out.Tags {
		tags[awsSDK.ToString(tag.Key)] = awsSDK.ToString(tag.Value)
	}
	return tags, nil
}

func (c *Client) listSecrets(ctx context.Context, pattern string) ([]ParameterMetadata, error) {
	var params []ParameterMetadata
	paginator := secretsmanager.NewListSecretsPaginator(c.secretsManager, &secretsmanager.ListSecretsInput{})
	for paginator.HasMorePages() {
		out, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list secrets: %w", err)
		}
		for _, secret := range out.SecretList {
			name := awsSDK.ToString(secret.Name)
			ok, err := parammatch.Match(pattern, name)
			if err != nil {
				return nil, err
			}
			if !ok {
				continue
			}
			tags := make(map[string]string, len(secret.Tags))
			for _, tag := range secret.Tags {
				tags[awsSDK.ToString(tag.Key)] = awsSDK.ToString(tag.Value)
			}
			params = append(params, ParameterMetadata{Name: name, Type: "Secret", LastModifiedDate: awsSDK.ToTime(secret.LastChangedDate), Tags: tags})
		}
	}
	return params, nil
}
