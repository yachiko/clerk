package aws

import "time"

// ResourceIdentity uniquely qualifies a resource across AWS scopes and
// backends. Display names are deliberately not part of identity.
type ResourceIdentity struct {
	Partition   string  `json:"partition"`
	AccountID   string  `json:"account_id"`
	Region      string  `json:"region"`
	Backend     Backend `json:"backend"`
	CanonicalID string  `json:"canonical_id"`
}

// ResourceMetadata contains the backend-neutral identity and presentation name.
// Provider-specific detail remains on provider-specific models.
type ResourceMetadata struct {
	Identity    ResourceIdentity `json:"identity"`
	DisplayName string           `json:"display_name"`
}

// ValueKind identifies which member of ResourceValue contains the value.
type ValueKind string

const (
	ValueText   ValueKind = "text"
	ValueBinary ValueKind = "binary"
)

// ResourceValue represents a retrieved value without coercing binary data to
// text. Kind remains explicit even when the selected value is empty.
type ResourceValue struct {
	Identity ResourceIdentity `json:"identity"`
	Kind     ValueKind        `json:"kind"`
	Text     string           `json:"text,omitempty"`
	Binary   []byte           `json:"binary,omitempty"`
}

// SecretMetadata is the metadata returned by a Secrets Manager inventory scan.
// It intentionally contains only fields available without retrieving a value.
type SecretMetadata struct {
	Identity                ResourceIdentity     `json:"identity"`
	Name                    string               `json:"name"`
	ARN                     string               `json:"arn"`
	Description             string               `json:"description,omitempty"`
	KMSKeyID                string               `json:"kms_key_id,omitempty"`
	Tags                    map[string]string    `json:"tags,omitempty"`
	CreatedDate             *time.Time           `json:"created_date,omitempty"`
	LastAccessedDate        *time.Time           `json:"last_accessed_date,omitempty"`
	LastChangedDate         *time.Time           `json:"last_changed_date,omitempty"`
	DeletedDate             *time.Time           `json:"deleted_date,omitempty"`
	RotationEnabled         *bool                `json:"rotation_enabled,omitempty"`
	RotationLambdaARN       string               `json:"rotation_lambda_arn,omitempty"`
	RotationRules           *SecretRotationRules `json:"rotation_rules,omitempty"`
	LastRotatedDate         *time.Time           `json:"last_rotated_date,omitempty"`
	NextRotationDate        *time.Time           `json:"next_rotation_date,omitempty"`
	VersionsToStages        map[string][]string  `json:"versions_to_stages,omitempty"`
	PrimaryRegion           string               `json:"primary_region,omitempty"`
	Replica                 bool                 `json:"replica,omitempty"`
	OwningService           string               `json:"owning_service,omitempty"`
	ExternalSecretType      string               `json:"external_secret_type,omitempty"`
	ExternalRotationRoleARN string               `json:"external_rotation_role_arn,omitempty"`
}

// SecretRotationRules describes the configured Secrets Manager rotation schedule.
type SecretRotationRules struct {
	AutomaticallyAfterDays *int64 `json:"automatically_after_days,omitempty"`
	Duration               string `json:"duration,omitempty"`
	ScheduleExpression     string `json:"schedule_expression,omitempty"`
}

// SecretDetail is a retrieved Secrets Manager value and its version details.
type SecretDetail struct {
	Identity      ResourceIdentity `json:"identity"`
	Name          string           `json:"name"`
	ARN           string           `json:"arn"`
	Value         ResourceValue    `json:"value"`
	VersionID     string           `json:"version_id"`
	VersionStages []string         `json:"version_stages,omitempty"`
	CreatedDate   *time.Time       `json:"created_date,omitempty"`
}

// SecretVersion is metadata for one opaque Secrets Manager version ID.
type SecretVersion struct {
	VersionID        string     `json:"version_id"`
	VersionStages    []string   `json:"version_stages,omitempty"`
	KMSKeyIDs        []string   `json:"kms_key_ids,omitempty"`
	CreatedDate      *time.Time `json:"created_date,omitempty"`
	LastAccessedDate *time.Time `json:"last_accessed_date,omitempty"`
}

// SecretValueSelector chooses a version by stage or opaque ID. With neither
// field set, GetSecretValue selects AWSCURRENT.
type SecretValueSelector struct {
	VersionStage string
	VersionID    string
}

// NewTextValue constructs a text resource value.
func NewTextValue(identity ResourceIdentity, value string) ResourceValue {
	return ResourceValue{Identity: identity, Kind: ValueText, Text: value}
}

// NewBinaryValue constructs a binary resource value and owns a copy of value.
func NewBinaryValue(identity ResourceIdentity, value []byte) ResourceValue {
	return ResourceValue{Identity: identity, Kind: ValueBinary, Binary: append([]byte(nil), value...)}
}

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
	Description      string            `json:"description,omitempty"`
	KMSKeyID         string            `json:"kms_key_id,omitempty"`
	Tier             string            `json:"tier,omitempty"`
	AllowedPattern   string            `json:"allowed_pattern,omitempty"`
	Policies         string            `json:"policies,omitempty"`
}

// ParameterMetadata represents metadata without the value
type ParameterMetadata struct {
	Name             string            `json:"name"`
	Type             string            `json:"type"`
	Version          int64             `json:"version"`
	LastModifiedDate time.Time         `json:"last_modified_date"`
	Tags             map[string]string `json:"tags,omitempty"`
}

// DescribeResult records whether a metadata scan observed the whole account.
type DescribeResult struct {
	Complete  bool
	Truncated bool
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
	Name           string
	Value          string
	Type           string
	Overwrite      bool
	KMSKeyID       string
	Tags           map[string]string
	Description    string
	Tier           string
	AllowedPattern string
	Policies       string
	DataType       string
}

// PutParameterOutput represents output from put operation
type PutParameterOutput struct {
	Version int64
}

// TransferInput describes a safe copy or move. Transfers create destinations by
// default; replacing an existing parameter requires Overwrite.
type TransferInput struct {
	Source      string
	Destination string
	Move        bool
	Overwrite   bool
}

// TransferResult records the completed remote steps. A non-nil error can be
// returned with DestinationWritten true when a move could not delete its source.
type TransferResult struct {
	Source             *Parameter
	Destination        *Parameter
	DestinationWritten bool
	SourceDeleted      bool
}

// LabelParameterInput represents input for labeling a parameter version
type LabelParameterInput struct {
	Name    string
	Version int64
	Labels  []string
}

// LabelParameterOutput represents output from label operation
type LabelParameterOutput struct {
	InvalidLabels []string // Labels that couldn't be applied
	Version       int64
}

// UnlabelParameterInput represents input for removing labels
type UnlabelParameterInput struct {
	Name    string
	Version int64
	Labels  []string
}
