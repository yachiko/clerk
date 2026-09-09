package cli

import (
	"fmt"
	"strings"
)

// validateParameterIdentifier accepts both SSM paths and valid flat names.
// ARNs are safe for read operations but are deliberately not accepted for
// mutations, where a name is less ambiguous to review at the prompt.
func validateParameterIdentifier(value string, allowARN bool) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("parameter name cannot be empty")
	}
	if strings.HasPrefix(value, "arn:") {
		if !allowARN {
			return fmt.Errorf("parameter ARN is not supported for this mutation; use the parameter name")
		}
		return nil
	}
	return nil
}
