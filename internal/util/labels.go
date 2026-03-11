package util

import (
	"fmt"
	"regexp"
	"strings"
)

const (
	MaxLabelLength      = 100
	MaxLabelsPerVersion = 10
)

var (
	// labelRegex matches valid label characters: a-zA-Z0-9_.-
	labelRegex = regexp.MustCompile(`^[a-zA-Z0-9_.-]+$`)

	// reservedPrefix that cannot be used
	reservedPrefix = "aws:"
)

// ValidateLabel validates a label string
func ValidateLabel(label string) error {
	if label == "" {
		return fmt.Errorf("label cannot be empty")
	}

	if len(label) > MaxLabelLength {
		return fmt.Errorf("label exceeds maximum length of %d characters", MaxLabelLength)
	}

	if strings.HasPrefix(strings.ToLower(label), reservedPrefix) {
		return fmt.Errorf("label cannot start with reserved prefix %q", reservedPrefix)
	}

	if !labelRegex.MatchString(label) {
		return fmt.Errorf("label contains invalid characters (allowed: a-zA-Z0-9_.-)")
	}

	return nil
}

// ValidateLabels validates a slice of labels
func ValidateLabels(labels []string) error {
	if len(labels) > MaxLabelsPerVersion {
		return fmt.Errorf("cannot add more than %d labels per version", MaxLabelsPerVersion)
	}

	seen := make(map[string]bool)
	for _, label := range labels {
		if err := ValidateLabel(label); err != nil {
			return err
		}
		if seen[label] {
			return fmt.Errorf("duplicate label: %s", label)
		}
		seen[label] = true
	}

	return nil
}

// SuggestLabels returns common label suggestions
func SuggestLabels() []string {
	return []string{
		"prod",
		"staging",
		"dev",
		"test",
		"current",
		"previous",
		"rollback-point",
		"last-known-good",
		"deprecated",
	}
}
