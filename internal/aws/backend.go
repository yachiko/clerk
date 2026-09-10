package aws

import (
	"fmt"
	"strings"
)

// Backend identifies a supported secret storage service.
type Backend string

const (
	BackendSSM            Backend = "ssm"
	BackendSecretsManager Backend = "secretsmanager"
)

// ParseBackend parses a backend selector.
func ParseBackend(value string) (Backend, error) {
	backend := Backend(strings.ToLower(strings.TrimSpace(value)))
	switch backend {
	case BackendSSM, BackendSecretsManager:
		return backend, nil
	default:
		return "", fmt.Errorf("invalid backend %q (valid: ssm, secretsmanager)", value)
	}
}
