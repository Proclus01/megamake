package domain

import (
	"path/filepath"
	"strings"
)

// IsTestFile returns true if a path matches common test naming conventions.
// It is applied to both base names and directories to mimic the Swift behavior.
func IsTestFile(relOrPath string) bool {
	p := filepath.ToSlash(relOrPath)
	base := strings.ToLower(filepath.Base(p))

	if strings.Contains(base, ".test.") ||
		strings.Contains(base, ".spec.") ||
		strings.Contains(base, "_test.") ||
		strings.Contains(base, "-test.") ||
		strings.HasPrefix(base, "test_") ||
		strings.Contains(base, "_spec.") {
		return true
	}

	parts := strings.Split(p, "/")
	for _, part := range parts {
		x := strings.ToLower(strings.TrimSpace(part))
		if x == "test" || x == "tests" || x == "__tests__" || x == "spec" || x == "specs" {
			return true
		}
	}
	return false
}
