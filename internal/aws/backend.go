package aws

import (
	"fmt"
	"strings"
)

// Backend identifies a supported secret storage service. BackendAll is valid
// for aggregate discovery, not for operations against one resource.
type Backend string

const (
	BackendAll            Backend = "all"
	BackendSSM            Backend = "ssm"
	BackendSecretsManager Backend = "secretsmanager"
)

// ParseBackend parses a backend selector.
func ParseBackend(value string) (Backend, error) {
	backend := Backend(strings.ToLower(strings.TrimSpace(value)))
	switch backend {
	case BackendAll, BackendSSM, BackendSecretsManager:
		return backend, nil
	default:
		return "", fmt.Errorf("invalid backend %q (valid: all, ssm, secretsmanager)", value)
	}
}
