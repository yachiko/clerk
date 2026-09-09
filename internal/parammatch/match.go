// Package parammatch defines the name-selection syntax shared by cache, CLI,
// and SSM metadata listing. A star matches any characters, including '/'.
package parammatch

import (
	"fmt"
	"path"
	"strings"
)

// Match reports whether name matches pattern. A pattern without a wildcard is
// an exact, case-sensitive parameter name. Empty, /, and /* select all names.
func Match(pattern, name string) (bool, error) {
	if pattern == "" || pattern == "/" || pattern == "*" || pattern == "/*" {
		return true, nil
	}
	if strings.ContainsAny(pattern, "[]\\") {
		return false, fmt.Errorf("invalid parameter glob %q: character classes and escapes are not supported", pattern)
	}
	if !strings.Contains(pattern, "*") {
		return pattern == name, nil
	}
	// Hierarchy shorthand includes the hierarchy node itself, matching the
	// historical CLI behavior while still using one matcher everywhere.
	if strings.HasSuffix(pattern, "/*") && name == strings.TrimSuffix(pattern, "/*") {
		return true, nil
	}
	return glob(pattern, name), nil
}

func glob(pattern, name string) bool {
	// path.Match deliberately is not used: its * does not cross a slash.
	parts := strings.Split(pattern, "*")
	pos := 0
	if parts[0] != "" {
		if !strings.HasPrefix(name, parts[0]) {
			return false
		}
		pos = len(parts[0])
	}
	for _, part := range parts[1 : len(parts)-1] {
		idx := strings.Index(name[pos:], part)
		if idx < 0 {
			return false
		}
		pos += idx + len(part)
	}
	last := parts[len(parts)-1]
	return last == "" || strings.HasSuffix(name[pos:], last)
}

// BasePath returns the longest complete hierarchy prefix safe for a
// GetParametersByPath request. Callers still must locally Match every result.
func BasePath(pattern string) string {
	if strings.Contains(pattern, "*") {
		return "/"
	}
	if pattern == "" || pattern == "/" {
		return "/"
	}
	if strings.HasPrefix(pattern, "/") {
		// Exact names are not a hierarchy request. The caller should use
		// DescribeParameters in that case, so / is the safe broad fallback.
		return path.Dir(pattern)
	}
	return "/"
}
