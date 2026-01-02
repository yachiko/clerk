package aws

import (
	"errors"

	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
)

// IsParameterNotFoundError checks if the error is a parameter not found error
func IsParameterNotFoundError(err error) bool {
	var pnf *types.ParameterNotFound
	return errors.As(err, &pnf)
}

// IsParameterAlreadyExistsError checks if the error is a parameter already exists error
func IsParameterAlreadyExistsError(err error) bool {
	var pae *types.ParameterAlreadyExists
	return errors.As(err, &pae)
}

// IsAccessDeniedError checks if the error is an access denied error
func IsAccessDeniedError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return contains(errStr, "AccessDenied") || contains(errStr, "UnauthorizedAccess")
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsImpl(s, substr))
}

func containsImpl(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
