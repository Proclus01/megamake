package adapters

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/megamake/megamake/internal/contracts/v1/project"
	"github.com/megamake/megamake/internal/domains/repo/domain"
	"github.com/megamake/megamake/internal/domains/repo/ports"
	"github.com/megamake/megamake/internal/platform/glob"
)

var _ = os.PathSeparator

type OSScanner struct{}

func NewOSScanner() OSScanner {
	return OSScanner{}
}

func (s OSScanner) Scan(req ports.ScanRequest) ([]project.FileRefV1, error) {
	rootAbs, err := filepath.Abs(req.RootPath)
	if err != nil {
		return nil, err
	}

	// Build a language set for rules.
	langSet := map[string]bool{}
	for _, l := range req.Profile.Languages {
		langSet[strings.TrimSpace(strings.ToLower(l))] = true
	}
	// The Swift detector uses "typescript" literal; preserve that.
	if contains(req.Profile.Languages, "typescript") {
		langSet["typescript"] = true
	}

	rules := domain.BuildRules(langSet)

	ignoreNames := toSet(req.IgnoreNames)
	ignoreGlobs := normalizeGlobs(req.IgnoreGlobs)

	maxBytes := req.MaxFileBytes
	if maxBytes <= 0 {
		maxBytes = 1_500_000
	}

	var files []project.FileRefV1

	_ = filepath.WalkDir(rootAbs, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}

		rel, relErr := filepath.Rel(rootAbs, path)
		if relErr != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		rel = strings.TrimPrefix(rel, "./")
		if rel == "." {
			return nil
		}

		if entry.IsDir() {
			name := entry.Name()
			low := strings.ToLower(name)

			// prune by built-in names + user ignore names
			if rules.PruneDirs[name] || rules.PruneDirs[low] || ignoreNames[name] || ignoreNames[low] {
				return fs.SkipDir
			}

			// prune by ignore globs (if rel path matches a glob, skip entire dir)
			if matchAnyGlob(rel, ignoreGlobs) {
				return fs.SkipDir
			}

			return nil
		}

		// file-level ignore by name segments/globs
		if isInIgnoredPath(rel, rules.PruneDirs, ignoreNames, ignoreGlobs) {
			return nil
		}

		info, err := entry.Info()
		if err != nil {
			return nil
		}
		if !info.Mode().IsRegular() {
			return nil
		}

		base := info.Name()
		baseLower := strings.ToLower(base)
		ext := strings.ToLower(filepath.Ext(baseLower))

		// Auto-ignore megamake tool artifacts (since we now default to writing them into CWD).
		// Examples:
		//   MEGAPROMPT_20260207_225153Z.txt
		//   MEGAPROMPT_latest.txt
		//   MEGADOC_....txt, MEGATEST_....txt, MEGADIAG_....txt, etc.
		if strings.HasPrefix(baseLower, "mega") && strings.HasSuffix(baseLower, ".txt") {
			return nil
		}

		// explicit noisy / secret-ish excludes
		if strings.HasPrefix(baseLower, ".env") {
			return nil
		}
		if strings.HasSuffix(baseLower, ".min.js") {
			return nil
		}
		if rules.ExcludeNames[baseLower] {
			return nil
		}
		if rules.ExcludeExts[ext] {
			return nil
		}

		// size cap
		if info.Size() > maxBytes {
			return nil
		}

		if shouldIncludeFile(rel, baseLower, ext, rules) {
			files = append(files, project.FileRefV1{
				RelPath:   rel,
				SizeBytes: info.Size(),
				IsTest:    domain.IsTestFile(rel),
			})
		}
		return nil
	})

	sort.Slice(files, func(i, j int) bool {
		return files[i].RelPath < files[j].RelPath
	})
	return files, nil
}

func shouldIncludeFile(rel string, baseLower string, ext string, rules domain.IncludeRules) bool {
	// Force include by name
	if rules.ForceIncludeNames[baseLower] {
		return true
	}

	// Force include by glob patterns (e.g., CI)
	if matchAnyGlob(rel, rules.ForceIncludeGlobs) {
		return true
	}

	// Config without extension
	if ext == "" && (baseLower == "dockerfile" || baseLower == "makefile") {
		return true
	}

	// Standard extension allow-list
	if rules.AllowedExts[ext] {
		return true
	}

	// README files
	if baseLower == "readme" || baseLower == "readme.md" {
		return true
	}

	// Tests treated as code if their extension is otherwise allowed
	if domain.IsTestFile(rel) {
		return rules.AllowedExts[ext]
	}

	return false
}

func isInIgnoredPath(rel string, pruneDirs map[string]bool, ignoreNames map[string]bool, ignoreGlobs []string) bool {
	parts := strings.Split(rel, "/")
	for _, p := range parts {
		low := strings.ToLower(p)
		if pruneDirs[p] || pruneDirs[low] || ignoreNames[p] || ignoreNames[low] {
			return true
		}
	}
	if matchAnyGlob(rel, ignoreGlobs) {
		return true
	}
	return false
}

func matchAnyGlob(rel string, patterns []string) bool {
	if len(patterns) == 0 {
		return false
	}
	for _, pat := range patterns {
		if strings.TrimSpace(pat) == "" {
			continue
		}
		if glob.Match(rel, pat) {
			return true
		}
	}
	return false
}

func toSet(items []string) map[string]bool {
	out := map[string]bool{}
	for _, s := range items {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		out[s] = true
		out[strings.ToLower(s)] = true
	}
	return out
}

func normalizeGlobs(items []string) []string {
	var out []string
	for _, s := range items {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		out = append(out, filepath.ToSlash(s))
	}
	return out
}

func contains(xs []string, target string) bool {
	for _, x := range xs {
		if x == target {
			return true
		}
	}
	return false
}
