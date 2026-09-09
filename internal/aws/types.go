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
