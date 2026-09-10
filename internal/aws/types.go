package aws

import "time"

// Parameter represents a secret/parameter from the selected AWS backend.
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
	VersionID        string            `json:"version_id,omitempty"`
	Binary           bool              `json:"binary,omitempty"`
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
