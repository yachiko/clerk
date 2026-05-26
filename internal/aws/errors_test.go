package aws

import (
	"errors"
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/stretchr/testify/assert"
)

func TestIsParameterNotFoundError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"generic", errors.New("boom"), false},
		{"typed direct", &types.ParameterNotFound{}, true},
		{"typed wrapped", fmt.Errorf("get failed: %w", &types.ParameterNotFound{}), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsParameterNotFoundError(tt.err))
		})
	}
}

func TestIsParameterAlreadyExistsError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"generic", errors.New("boom"), false},
		{"typed direct", &types.ParameterAlreadyExists{}, true},
		{"typed wrapped", fmt.Errorf("put failed: %w", &types.ParameterAlreadyExists{}), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsParameterAlreadyExistsError(tt.err))
		})
	}
}

func TestIsAccessDeniedError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"generic", errors.New("boom"), false},
		{"access denied", errors.New("AccessDeniedException: ..."), true},
		{"unauthorized", errors.New("UnauthorizedAccess: ..."), true},
		{"wrapped access denied", fmt.Errorf("rpc: %w", errors.New("AccessDenied")), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsAccessDeniedError(tt.err))
		})
	}
}
