package paths

import (
	"path/filepath"
	"strings"

	"github.com/megamake/megamake/internal/platform/errors"
)

// ToPosixRel returns a clean, POSIX-style relative path from base to target.
// It is intended to keep internal paths stable across OSes.
func ToPosixRel(baseDir string, targetPath string) (string, error) {
	if strings.TrimSpace(baseDir) == "" {
		return "", errors.NewInternal("baseDir is empty", nil)
	}
	if strings.TrimSpace(targetPath) == "" {
		return "", errors.NewInternal("targetPath is empty", nil)
	}

	rel, err := filepath.Rel(baseDir, targetPath)
	if err != nil {
		return "", errors.NewInternal("failed to compute relative path", err)
	}

	rel = filepath.Clean(rel)
	rel = filepath.ToSlash(rel)
	rel = strings.TrimPrefix(rel, "./")

	return rel, nil
}
