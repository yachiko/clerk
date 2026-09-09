package aws

import (
	"context"
	"fmt"
	"strings"

	sdkaws "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	smtypes "github.com/aws/aws-sdk-go-v2/service/secretsmanager/types"
)

type secretsManagerAPI interface {
	ListSecrets(context.Context, *secretsmanager.ListSecretsInput, ...func(*secretsmanager.Options)) (*secretsmanager.ListSecretsOutput, error)
	GetSecretValue(context.Context, *secretsmanager.GetSecretValueInput, ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error)
	ListSecretVersionIds(context.Context, *secretsmanager.ListSecretVersionIdsInput, ...func(*secretsmanager.Options)) (*secretsmanager.ListSecretVersionIdsOutput, error)
}

// SecretsManagerClient provides read-only access to AWS Secrets Manager.
type SecretsManagerClient struct {
	api       secretsManagerAPI
	partition string
	accountID string
	region    string
}

// NewSecretsManagerClient constructs a read-only adapter from an already
// resolved AWS context.
func NewSecretsManagerClient(resolved *ResolvedContext) (*SecretsManagerClient, error) {
	if resolved == nil {
		return nil, fmt.Errorf("resolved AWS context is required")
	}
	if strings.TrimSpace(resolved.Region) == "" {
		return nil, fmt.Errorf("AWS region is not configured; pass --region, set config region, or configure AWS_REGION/a shared AWS profile")
	}
	return newSecretsManagerClient(resolved, secretsmanager.NewFromConfig(resolved.Config))
}

func newSecretsManagerClient(resolved *ResolvedContext, api secretsManagerAPI) (*SecretsManagerClient, error) {
	if resolved == nil {
		return nil, fmt.Errorf("resolved AWS context is required")
	}
	if api == nil {
		return nil, fmt.Errorf("Secrets Manager API is required")
	}
	return &SecretsManagerClient{api: api, partition: resolved.Partition, accountID: resolved.AccountID, region: resolved.Region}, nil
}

// ListSecrets returns complete metadata inventory without retrieving any secret
// values. Planned deletions are requested so callers can represent deletion state.
func (c *SecretsManagerClient) ListSecrets(ctx context.Context) ([]SecretMetadata, error) {
	input := &secretsmanager.ListSecretsInput{IncludePlannedDeletion: sdkaws.Bool(true)}
	var result []SecretMetadata
	seenTokens := make(map[string]struct{})

	for {
		output, err := c.api.ListSecrets(ctx, input)
		if err != nil {
			return nil, fmt.Errorf("failed to list secrets: %w", err)
		}
		if output == nil {
			return nil, fmt.Errorf("failed to list secrets: Secrets Manager returned an empty response")
		}
		for _, entry := range output.SecretList {
			metadata, err := c.secretMetadata(entry)
			if err != nil {
				return nil, err
			}
			result = append(result, metadata)
		}

		if output.NextToken == nil || *output.NextToken == "" {
			return result, nil
		}
		token := *output.NextToken
		if _, duplicate := seenTokens[token]; duplicate {
			return nil, fmt.Errorf("failed to list secrets: Secrets Manager returned duplicate pagination token %q", token)
		}
		seenTokens[token] = struct{}{}
		input.NextToken = sdkaws.String(token)
	}
}

func (c *SecretsManagerClient) secretMetadata(entry smtypes.SecretListEntry) (SecretMetadata, error) {
	name, secretARN := sdkaws.ToString(entry.Name), sdkaws.ToString(entry.ARN)
	if name == "" || secretARN == "" {
		return SecretMetadata{}, fmt.Errorf("failed to list secrets: Secrets Manager returned a secret without a name or ARN")
	}

	tags := make(map[string]string, len(entry.Tags))
	for _, tag := range entry.Tags {
		if tag.Key != nil {
			tags[*tag.Key] = sdkaws.ToString(tag.Value)
		}
	}
	if len(tags) == 0 {
		tags = nil
	}

	var rules *SecretRotationRules
	if entry.RotationRules != nil {
		rules = &SecretRotationRules{
			AutomaticallyAfterDays: entry.RotationRules.AutomaticallyAfterDays,
			Duration:               sdkaws.ToString(entry.RotationRules.Duration),
			ScheduleExpression:     sdkaws.ToString(entry.RotationRules.ScheduleExpression),
		}
	}
	primaryRegion := sdkaws.ToString(entry.PrimaryRegion)
	return SecretMetadata{
		Identity:                c.identity(secretARN),
		Name:                    name,
		ARN:                     secretARN,
		Description:             sdkaws.ToString(entry.Description),
		KMSKeyID:                sdkaws.ToString(entry.KmsKeyId),
		Tags:                    tags,
		CreatedDate:             entry.CreatedDate,
		LastAccessedDate:        entry.LastAccessedDate,
		LastChangedDate:         entry.LastChangedDate,
		DeletedDate:             entry.DeletedDate,
		RotationEnabled:         entry.RotationEnabled,
		RotationLambdaARN:       sdkaws.ToString(entry.RotationLambdaARN),
		RotationRules:           rules,
		LastRotatedDate:         entry.LastRotatedDate,
		NextRotationDate:        entry.NextRotationDate,
		VersionsToStages:        cloneStages(entry.SecretVersionsToStages),
		PrimaryRegion:           primaryRegion,
		Replica:                 primaryRegion != "" && primaryRegion != c.region,
		OwningService:           sdkaws.ToString(entry.OwningService),
		ExternalSecretType:      sdkaws.ToString(entry.Type),
		ExternalRotationRoleARN: sdkaws.ToString(entry.ExternalSecretRotationRoleArn),
	}, nil
}

// GetSecretValue retrieves AWSCURRENT by default, or a version selected by one
// explicit stage or opaque version ID.
func (c *SecretsManagerClient) GetSecretValue(ctx context.Context, secretID string, selector SecretValueSelector) (*SecretDetail, error) {
	if strings.TrimSpace(secretID) == "" {
		return nil, fmt.Errorf("secret ID is required")
	}
	if selector.VersionStage != "" && selector.VersionID != "" {
		return nil, fmt.Errorf("version stage and version ID are mutually exclusive")
	}
	input := &secretsmanager.GetSecretValueInput{SecretId: sdkaws.String(secretID)}
	switch {
	case selector.VersionID != "":
		input.VersionId = sdkaws.String(selector.VersionID)
	case selector.VersionStage != "":
		input.VersionStage = sdkaws.String(selector.VersionStage)
	default:
		input.VersionStage = sdkaws.String("AWSCURRENT")
	}

	output, err := c.api.GetSecretValue(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to get secret value: %w", err)
	}
	if output == nil {
		return nil, fmt.Errorf("failed to get secret value: Secrets Manager returned an empty response")
	}
	name, secretARN := sdkaws.ToString(output.Name), sdkaws.ToString(output.ARN)
	if name == "" || secretARN == "" {
		return nil, fmt.Errorf("failed to get secret value: Secrets Manager returned a value without a name or ARN")
	}
	if output.SecretString != nil && output.SecretBinary != nil {
		return nil, fmt.Errorf("failed to get secret value: Secrets Manager returned both text and binary values")
	}
	identity := c.identity(secretARN)
	var value ResourceValue
	if output.SecretString != nil {
		value = NewTextValue(identity, *output.SecretString)
	} else if output.SecretBinary != nil {
		value = NewBinaryValue(identity, output.SecretBinary)
	} else {
		return nil, fmt.Errorf("failed to get secret value: Secrets Manager returned no value")
	}

	return &SecretDetail{
		Identity:      identity,
		Name:          name,
		ARN:           secretARN,
		Value:         value,
		VersionID:     sdkaws.ToString(output.VersionId),
		VersionStages: append([]string(nil), output.VersionStages...),
		CreatedDate:   output.CreatedDate,
	}, nil
}

// ListSecretVersionIds returns every version, including deprecated versions with
// no stages, in service order. Version IDs remain opaque strings.
func (c *SecretsManagerClient) ListSecretVersionIds(ctx context.Context, secretID string) ([]SecretVersion, error) {
	if strings.TrimSpace(secretID) == "" {
		return nil, fmt.Errorf("secret ID is required")
	}
	input := &secretsmanager.ListSecretVersionIdsInput{SecretId: sdkaws.String(secretID), IncludeDeprecated: sdkaws.Bool(true)}
	var result []SecretVersion
	seenTokens := make(map[string]struct{})
	for {
		output, err := c.api.ListSecretVersionIds(ctx, input)
		if err != nil {
			return nil, fmt.Errorf("failed to list secret versions: %w", err)
		}
		if output == nil {
			return nil, fmt.Errorf("failed to list secret versions: Secrets Manager returned an empty response")
		}
		for _, version := range output.Versions {
			versionID := sdkaws.ToString(version.VersionId)
			if versionID == "" {
				return nil, fmt.Errorf("failed to list secret versions: Secrets Manager returned a version without an ID")
			}
			result = append(result, SecretVersion{
				VersionID:        versionID,
				VersionStages:    append([]string(nil), version.VersionStages...),
				KMSKeyIDs:        append([]string(nil), version.KmsKeyIds...),
				CreatedDate:      version.CreatedDate,
				LastAccessedDate: version.LastAccessedDate,
			})
		}
		if output.NextToken == nil || *output.NextToken == "" {
			return result, nil
		}
		token := *output.NextToken
		if _, duplicate := seenTokens[token]; duplicate {
			return nil, fmt.Errorf("failed to list secret versions: Secrets Manager returned duplicate pagination token %q", token)
		}
		seenTokens[token] = struct{}{}
		input.NextToken = sdkaws.String(token)
	}
}

func (c *SecretsManagerClient) identity(secretARN string) ResourceIdentity {
	return ResourceIdentity{Partition: c.partition, AccountID: c.accountID, Region: c.region, Backend: BackendSecretsManager, CanonicalID: secretARN}
}

func cloneStages(source map[string][]string) map[string][]string {
	if len(source) == 0 {
		return nil
	}
	result := make(map[string][]string, len(source))
	for versionID, stages := range source {
		result[versionID] = append([]string(nil), stages...)
	}
	return result
}
