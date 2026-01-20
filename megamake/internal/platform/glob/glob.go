package glob

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Match checks if a POSIX-style relPath matches a glob pattern supporting *, **, ?.
// - *  matches within a path segment (no '/')
// - ** matches across directories (may include '/')
// - ?  matches a single character (including '/'; consistent with common glob semantics here)
func Match(relPath string, pattern string) bool {
	rel := filepath.ToSlash(relPath)
	pat := filepath.ToSlash(pattern)
	re := globToRegex(pat)
	ok, err := regexp.MatchString(re, rel)
	if err != nil {
		return false
	}
	return ok
}

// FindMatches returns filesystem paths under root that match the given pattern.
// If the pattern contains no glob metacharacters, it checks direct existence.
func FindMatches(root string, pattern string) ([]string, error) {
	pat := filepath.ToSlash(pattern)
	if !strings.ContainsAny(pat, "*?") {
		cand := filepath.Join(root, filepath.FromSlash(pat))
		if _, err := os.Stat(cand); err == nil {
			return []string{cand}, nil
		}
		return nil, nil
	}

	var out []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			// Best-effort: ignore unreadable entries.
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		rel = strings.TrimPrefix(rel, "./")
		if rel == "." {
			return nil
		}
		if Match(rel, pat) {
			out = append(out, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func globToRegex(glob string) string {
	// Convert glob to anchored regex.
	// We escape everything except glob operators, then translate:
	// ** -> .*
	// *  -> [^/]* (segment)
	// ?  -> .
	var b strings.Builder
	b.WriteString("^")

	runes := []rune(glob)
	for i := 0; i < len(runes); i++ {
		ch := runes[i]
		if ch == '*' {
			// ** case
			if i+1 < len(runes) && runes[i+1] == '*' {
				b.WriteString(".*")
				i++
				continue
			}
			b.WriteString("[^/]*")
			continue
		}
		if ch == '?' {
			b.WriteString(".")
			continue
		}
		// Escape regex metacharacters.
		b.WriteString(regexp.QuoteMeta(string(ch)))
	}

	b.WriteString("$")
	return b.String()
}
