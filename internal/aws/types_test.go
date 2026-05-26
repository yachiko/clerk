package aws

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestParameter_Fields(t *testing.T) {
	now := time.Now()
	p := Parameter{
		Name:             "/test/secret",
		Value:            "secret-value",
		Type:             "SecureString",
		Version:          3,
		LastModifiedDate: now,
		ARN:              "arn:aws:ssm:us-east-1:123456789012:parameter/test/secret",
		DataType:         "text",
		Tags:             map[string]string{"env": "test"},
	}

	assert.Equal(t, "/test/secret", p.Name)
	assert.Equal(t, "secret-value", p.Value)
	assert.Equal(t, "SecureString", p.Type)
	assert.Equal(t, int64(3), p.Version)
	assert.Equal(t, now, p.LastModifiedDate)
	assert.Contains(t, p.ARN, "parameter/test/secret")
	assert.Equal(t, "text", p.DataType)
	assert.Equal(t, "test", p.Tags["env"])
}

func TestParameterMetadata_Fields(t *testing.T) {
	now := time.Now()
	m := ParameterMetadata{
		Name:             "/test/p",
		Type:             "String",
		Version:          1,
		LastModifiedDate: now,
		Tags:             map[string]string{"team": "backend"},
	}

	assert.Equal(t, "/test/p", m.Name)
	assert.Equal(t, int64(1), m.Version)
	assert.Equal(t, "backend", m.Tags["team"])
}

func TestParameterHistory_Fields(t *testing.T) {
	now := time.Now()
	h := ParameterHistory{
		Name:             "/test/p",
		Value:            "old",
		Type:             "SecureString",
		Version:          2,
		LastModifiedDate: now,
		Labels:           []string{"prod", "stable"},
	}

	assert.Equal(t, int64(2), h.Version)
	assert.Len(t, h.Labels, 2)
	assert.Contains(t, h.Labels, "prod")
}

func TestPutParameter_RoundTrip(t *testing.T) {
	in := PutParameterInput{
		Name:      "/prod/secret",
		Value:     "shhh",
		Type:      "SecureString",
		Overwrite: true,
		KMSKeyID:  "alias/my-key",
		Tags:      map[string]string{"env": "prod"},
	}
	assert.Equal(t, "/prod/secret", in.Name)
	assert.True(t, in.Overwrite)
	assert.Equal(t, "alias/my-key", in.KMSKeyID)

	out := PutParameterOutput{Version: 5}
	assert.Equal(t, int64(5), out.Version)
}

func TestLabelParameterTypes(t *testing.T) {
	in := LabelParameterInput{
		Name:    "/p",
		Version: 3,
		Labels:  []string{"prod"},
	}
	assert.Equal(t, int64(3), in.Version)
	assert.Equal(t, []string{"prod"}, in.Labels)

	out := LabelParameterOutput{InvalidLabels: []string{"aws:bad"}, Version: 3}
	assert.Equal(t, []string{"aws:bad"}, out.InvalidLabels)

	un := UnlabelParameterInput{Name: "/p", Version: 3, Labels: []string{"old"}}
	assert.Equal(t, "old", un.Labels[0])
}
