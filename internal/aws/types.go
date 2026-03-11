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
	Type      string
	Overwrite bool
	KMSKeyID  string
	Tags      map[string]string
}

// PutParameterOutput represents output from put operation
type PutParameterOutput struct {
	Version int64
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
