package aws

import (
	"context"
	"crypto/rand"
	"fmt"
	"sort"
	"strings"

	sdkaws "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	smtypes "github.com/aws/aws-sdk-go-v2/service/secretsmanager/types"
)

type secretsManagerAPI interface {
	ListSecrets(context.Context, *secretsmanager.ListSecretsInput, ...func(*secretsmanager.Options)) (*secretsmanager.ListSecretsOutput, error)
	DescribeSecret(context.Context, *secretsmanager.DescribeSecretInput, ...func(*secretsmanager.Options)) (*secretsmanager.DescribeSecretOutput, error)
	GetSecretValue(context.Context, *secretsmanager.GetSecretValueInput, ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error)
	ListSecretVersionIds(context.Context, *secretsmanager.ListSecretVersionIdsInput, ...func(*secretsmanager.Options)) (*secretsmanager.ListSecretVersionIdsOutput, error)
	CreateSecret(context.Context, *secretsmanager.CreateSecretInput, ...func(*secretsmanager.Options)) (*secretsmanager.CreateSecretOutput, error)
	PutSecretValue(context.Context, *secretsmanager.PutSecretValueInput, ...func(*secretsmanager.Options)) (*secretsmanager.PutSecretValueOutput, error)
	TagResource(context.Context, *secretsmanager.TagResourceInput, ...func(*secretsmanager.Options)) (*secretsmanager.TagResourceOutput, error)
	UntagResource(context.Context, *secretsmanager.UntagResourceInput, ...func(*secretsmanager.Options)) (*secretsmanager.UntagResourceOutput, error)
	DeleteSecret(context.Context, *secretsmanager.DeleteSecretInput, ...func(*secretsmanager.Options)) (*secretsmanager.DeleteSecretOutput, error)
	RestoreSecret(context.Context, *secretsmanager.RestoreSecretInput, ...func(*secretsmanager.Options)) (*secretsmanager.RestoreSecretOutput, error)
}

// SecretsManagerClient provides focused metadata, value, and mutation access to
// AWS Secrets Manager.
type SecretsManagerClient struct {
	api       secretsManagerAPI
	partition string
	accountID string
	region    string
}

// NewSecretsManagerClient constructs an adapter from an already resolved AWS context.
func NewSecretsManagerClient(resolved *ResolvedContext) (*SecretsManagerClient, error) {
	if resolved == nil {
		return nil, fmt.Errorf("resolved AWS context is required")
	}
	if strings.TrimSpace(resolved.Region) == "" {
		return nil, fmt.Errorf("AWS region is not configured; pass --region, set config region, or configure AWS_REGION/a shared AWS profile")
	}
	return newSecretsManagerClient(resolved, secretsmanager.NewFromConfig(resolved.Config))
}

// DescribeSecret retrieves metadata without retrieving any secret value.
func (c *SecretsManagerClient) DescribeSecret(ctx context.Context, secretID string) (*SecretMetadata, error) {
	if strings.TrimSpace(secretID) == "" {
		return nil, fmt.Errorf("secret ID is required")
	}
	output, err := c.api.DescribeSecret(ctx, &secretsmanager.DescribeSecretInput{SecretId: sdkaws.String(secretID)})
	if err != nil {
		return nil, fmt.Errorf("failed to describe secret: %w", err)
	}
	if output == nil {
		return nil, fmt.Errorf("failed to describe secret: Secrets Manager returned an empty response")
	}
	name, secretARN := sdkaws.ToString(output.Name), sdkaws.ToString(output.ARN)
	if name == "" || secretARN == "" {
		return nil, fmt.Errorf("failed to describe secret: Secrets Manager returned a secret without a name or ARN")
	}
	tags := tagsToMap(output.Tags)
	var rules *SecretRotationRules
	if output.RotationRules != nil {
		rules = &SecretRotationRules{AutomaticallyAfterDays: output.RotationRules.AutomaticallyAfterDays, Duration: sdkaws.ToString(output.RotationRules.Duration), ScheduleExpression: sdkaws.ToString(output.RotationRules.ScheduleExpression)}
	}
	primaryRegion := sdkaws.ToString(output.PrimaryRegion)
	return &SecretMetadata{
		Identity: c.identity(secretARN), Name: name, ARN: secretARN,
		Description: sdkaws.ToString(output.Description), KMSKeyID: sdkaws.ToString(output.KmsKeyId), Tags: tags,
		CreatedDate: output.CreatedDate, LastAccessedDate: output.LastAccessedDate, LastChangedDate: output.LastChangedDate, DeletedDate: output.DeletedDate,
		RotationEnabled: output.RotationEnabled, RotationLambdaARN: sdkaws.ToString(output.RotationLambdaARN), RotationRules: rules,
		LastRotatedDate: output.LastRotatedDate, NextRotationDate: output.NextRotationDate, VersionsToStages: cloneStages(output.VersionIdsToStages),
		PrimaryRegion: primaryRegion, Replica: primaryRegion != "" && primaryRegion != c.region, OwningService: sdkaws.ToString(output.OwningService),
		ExternalSecretType: sdkaws.ToString(output.Type), ExternalRotationRoleARN: sdkaws.ToString(output.ExternalSecretRotationRoleArn),
	}, nil
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

	tags := tagsToMap(entry.Tags)

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

// CreateSecret creates a secret with exactly one text or binary initial value.
func (c *SecretsManagerClient) CreateSecret(ctx context.Context, request CreateSecretRequest) (*CreateSecretResult, error) {
	if strings.TrimSpace(request.Name) == "" {
		return nil, fmt.Errorf("secret name is required")
	}
	if err := validateTags(request.Tags); err != nil {
		return nil, err
	}
	input := &secretsmanager.CreateSecretInput{Name: sdkaws.String(request.Name), Description: optionalString(request.Description), KmsKeyId: optionalString(request.KMSKeyID), Tags: mapToTags(request.Tags)}
	if err := setSecretValue(request.Value, &input.SecretString, &input.SecretBinary); err != nil {
		return nil, err
	}
	token, err := newClientRequestToken()
	if err != nil {
		return nil, fmt.Errorf("failed to generate create secret request token: %w", err)
	}
	input.ClientRequestToken = sdkaws.String(token)
	output, err := c.api.CreateSecret(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to create secret: %w", err)
	}
	if output == nil {
		return nil, fmt.Errorf("failed to create secret: Secrets Manager returned an empty response")
	}
	name, secretARN, versionID := sdkaws.ToString(output.Name), sdkaws.ToString(output.ARN), sdkaws.ToString(output.VersionId)
	if name == "" || secretARN == "" || versionID == "" {
		return nil, fmt.Errorf("failed to create secret: Secrets Manager returned a result without a name, ARN, or version ID")
	}
	return &CreateSecretResult{Identity: c.identity(secretARN), Name: name, ARN: secretARN, VersionID: versionID}, nil
}

// PutSecretValue creates a new immutable text or binary secret version.
func (c *SecretsManagerClient) PutSecretValue(ctx context.Context, request PutSecretValueRequest) (*PutSecretValueResult, error) {
	if strings.TrimSpace(request.SecretID) == "" {
		return nil, fmt.Errorf("secret ID is required")
	}
	for _, stage := range request.VersionStages {
		if strings.TrimSpace(stage) == "" {
			return nil, fmt.Errorf("version stages must not contain empty values")
		}
	}
	input := &secretsmanager.PutSecretValueInput{SecretId: sdkaws.String(request.SecretID), VersionStages: append([]string(nil), request.VersionStages...)}
	if err := setSecretValue(request.Value, &input.SecretString, &input.SecretBinary); err != nil {
		return nil, err
	}
	token, err := newClientRequestToken()
	if err != nil {
		return nil, fmt.Errorf("failed to generate put secret value request token: %w", err)
	}
	input.ClientRequestToken = sdkaws.String(token)
	output, err := c.api.PutSecretValue(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to put secret value: %w", err)
	}
	if output == nil {
		return nil, fmt.Errorf("failed to put secret value: Secrets Manager returned an empty response")
	}
	name, secretARN, versionID := sdkaws.ToString(output.Name), sdkaws.ToString(output.ARN), sdkaws.ToString(output.VersionId)
	if name == "" || secretARN == "" || versionID == "" {
		return nil, fmt.Errorf("failed to put secret value: Secrets Manager returned a result without a name, ARN, or version ID")
	}
	return &PutSecretValueResult{Identity: c.identity(secretARN), Name: name, ARN: secretARN, VersionID: versionID, VersionStages: append([]string(nil), output.VersionStages...)}, nil
}

// TagResource adds or replaces tags on a secret.
func (c *SecretsManagerClient) TagResource(ctx context.Context, request TagSecretRequest) error {
	if strings.TrimSpace(request.SecretID) == "" {
		return fmt.Errorf("secret ID is required")
	}
	if len(request.Tags) == 0 {
		return fmt.Errorf("at least one tag is required")
	}
	if err := validateTags(request.Tags); err != nil {
		return err
	}
	output, err := c.api.TagResource(ctx, &secretsmanager.TagResourceInput{SecretId: sdkaws.String(request.SecretID), Tags: mapToTags(request.Tags)})
	if err != nil {
		return fmt.Errorf("failed to tag secret: %w", err)
	}
	if output == nil {
		return fmt.Errorf("failed to tag secret: Secrets Manager returned an empty response")
	}
	return nil
}

// UntagResource removes tags by key from a secret.
func (c *SecretsManagerClient) UntagResource(ctx context.Context, request UntagSecretRequest) error {
	if strings.TrimSpace(request.SecretID) == "" {
		return fmt.Errorf("secret ID is required")
	}
	if len(request.TagKeys) == 0 {
		return fmt.Errorf("at least one tag key is required")
	}
	for _, key := range request.TagKeys {
		if strings.TrimSpace(key) == "" {
			return fmt.Errorf("tag keys must not contain empty values")
		}
	}
	output, err := c.api.UntagResource(ctx, &secretsmanager.UntagResourceInput{SecretId: sdkaws.String(request.SecretID), TagKeys: append([]string(nil), request.TagKeys...)})
	if err != nil {
		return fmt.Errorf("failed to untag secret: %w", err)
	}
	if output == nil {
		return fmt.Errorf("failed to untag secret: Secrets Manager returned an empty response")
	}
	return nil
}

// DeleteSecret schedules deletion with an explicit recovery window or performs
// an explicitly requested permanent deletion.
func (c *SecretsManagerClient) DeleteSecret(ctx context.Context, request DeleteSecretRequest) (*DeleteSecretResult, error) {
	if strings.TrimSpace(request.SecretID) == "" {
		return nil, fmt.Errorf("secret ID is required")
	}
	input := &secretsmanager.DeleteSecretInput{SecretId: sdkaws.String(request.SecretID)}
	if request.Permanent {
		if request.RecoveryWindowDays != 0 {
			return nil, fmt.Errorf("recovery window cannot be set for permanent deletion")
		}
		input.ForceDeleteWithoutRecovery = sdkaws.Bool(true)
	} else {
		if request.RecoveryWindowDays < 7 || request.RecoveryWindowDays > 30 {
			return nil, fmt.Errorf("recovery window must be between 7 and 30 days")
		}
		input.RecoveryWindowInDays = sdkaws.Int64(request.RecoveryWindowDays)
	}
	output, err := c.api.DeleteSecret(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to delete secret: %w", err)
	}
	if output == nil {
		return nil, fmt.Errorf("failed to delete secret: Secrets Manager returned an empty response")
	}
	name, secretARN := sdkaws.ToString(output.Name), sdkaws.ToString(output.ARN)
	if name == "" || secretARN == "" {
		return nil, fmt.Errorf("failed to delete secret: Secrets Manager returned a result without a name or ARN")
	}
	if !request.Permanent && output.DeletionDate == nil {
		return nil, fmt.Errorf("failed to delete secret: Secrets Manager returned a scheduled deletion without a deletion date")
	}
	return &DeleteSecretResult{Identity: c.identity(secretARN), Name: name, ARN: secretARN, DeletionDate: output.DeletionDate}, nil
}

// RestoreSecret cancels a scheduled deletion.
func (c *SecretsManagerClient) RestoreSecret(ctx context.Context, request RestoreSecretRequest) (*RestoreSecretResult, error) {
	if strings.TrimSpace(request.SecretID) == "" {
		return nil, fmt.Errorf("secret ID is required")
	}
	output, err := c.api.RestoreSecret(ctx, &secretsmanager.RestoreSecretInput{SecretId: sdkaws.String(request.SecretID)})
	if err != nil {
		return nil, fmt.Errorf("failed to restore secret: %w", err)
	}
	if output == nil {
		return nil, fmt.Errorf("failed to restore secret: Secrets Manager returned an empty response")
	}
	name, secretARN := sdkaws.ToString(output.Name), sdkaws.ToString(output.ARN)
	if name == "" || secretARN == "" {
		return nil, fmt.Errorf("failed to restore secret: Secrets Manager returned a result without a name or ARN")
	}
	return &RestoreSecretResult{Identity: c.identity(secretARN), Name: name, ARN: secretARN}, nil
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

func tagsToMap(source []smtypes.Tag) map[string]string {
	if len(source) == 0 {
		return nil
	}
	result := make(map[string]string, len(source))
	for _, tag := range source {
		if tag.Key != nil {
			result[*tag.Key] = sdkaws.ToString(tag.Value)
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func mapToTags(source map[string]string) []smtypes.Tag {
	if len(source) == 0 {
		return nil
	}
	keys := make([]string, 0, len(source))
	for key := range source {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]smtypes.Tag, 0, len(keys))
	for _, key := range keys {
		result = append(result, smtypes.Tag{Key: sdkaws.String(key), Value: sdkaws.String(source[key])})
	}
	return result
}

func optionalString(value string) *string {
	if value == "" {
		return nil
	}
	return sdkaws.String(value)
}

func setSecretValue(value SecretValueInput, text **string, binary *[]byte) error {
	switch value.Kind {
	case ValueText:
		if value.Binary != nil {
			return fmt.Errorf("text secret value must not include binary data")
		}
		*text = sdkaws.String(value.Text)
	case ValueBinary:
		if value.Text != "" {
			return fmt.Errorf("binary secret value must not include text data")
		}
		*binary = append([]byte{}, value.Binary...)
	default:
		return fmt.Errorf("secret value kind must be text or binary")
	}
	return nil
}

func validateTags(tags map[string]string) error {
	for key := range tags {
		if strings.TrimSpace(key) == "" {
			return fmt.Errorf("tag keys must not be empty")
		}
	}
	return nil
}

func newClientRequestToken() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", value[0:4], value[4:6], value[6:8], value[8:10], value[10:16]), nil
}
