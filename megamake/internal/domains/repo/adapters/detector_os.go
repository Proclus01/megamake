package adapters

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/megamake/megamake/internal/contracts/v1/project"
	"github.com/megamake/megamake/internal/domains/repo/domain"
	"github.com/megamake/megamake/internal/platform/glob"
)

type OSDetector struct{}

func NewOSDetector() OSDetector {
	return OSDetector{}
}

func (d OSDetector) Detect(rootPath string) (project.ProjectProfileV1, error) {
	rootAbs, err := filepath.Abs(rootPath)
	if err != nil {
		return project.ProjectProfileV1{}, err
	}

	info, err := os.Stat(rootAbs)
	if err != nil {
		return project.ProjectProfileV1{}, err
	}
	if !info.IsDir() {
		return project.ProjectProfileV1{
			RootPath:      rootPath,
			Languages:     nil,
			Markers:       nil,
			IsCodeProject: false,
			Why:           []string{"path is not a directory"},
		}, nil
	}

	// Markers (close to Swift ProjectDetector)
	markerFiles := map[string][]string{
		"typescript": {"tsconfig.json"},
		"javascript": {"package.json"},
		"python":     {"pyproject.toml", "requirements.txt", "Pipfile", "setup.py", "setup.cfg", "tox.ini"},
		"go":         {"go.mod"},
		"rust":       {"Cargo.toml"},
		"java":       {"pom.xml", "build.gradle", "build.gradle.kts", "settings.gradle", "settings.gradle.kts"},
		"kotlin":     {"build.gradle.kts"},
		"csharp":     {"*.sln", "*.csproj"},
		"cpp":        {"CMakeLists.txt"},
		"php":        {"composer.json"},
		"ruby":       {"Gemfile"},
		"swift":      {"Package.swift", "*.xcodeproj"},
		"terraform":  {"*.tf"},
		"docker":     {"Dockerfile"},
		"lean":       {"lakefile.lean", "lean-toolchain", "*.lean"},
		"latex":      {"latexmkrc", "*.tex"},
	}

	languageSet := map[string]bool{}
	markersSet := map[string]bool{}
	var why []string

	for lang, patterns := range markerFiles {
		for _, pat := range patterns {
			hits, _ := glob.FindMatches(rootAbs, pat)
			if len(hits) == 0 {
				continue
			}
			languageSet[lang] = true
			for _, h := range hits {
				rel, err := filepath.Rel(rootAbs, h)
				if err != nil {
					continue
				}
				rel = filepath.ToSlash(rel)
				rel = strings.TrimPrefix(rel, "./")
				if rel == "." {
					continue
				}
				markersSet[rel] = true
				why = append(why, lang+" marker: "+rel)
			}
		}
	}

	// Source file heuristic (>= 8 recognizable source files).
	sourceExtToLang := map[string]string{
		".ts": "typescript", ".tsx": "typescript",
		".js": "javascript", ".jsx": "javascript", ".mjs": "javascript", ".cjs": "javascript",
		".py":   "python",
		".go":   "go",
		".rs":   "rust",
		".java": "java",
		".kt":   "kotlin", ".kts": "kotlin",
		".c": "cpp", ".cc": "cpp", ".cpp": "cpp", ".cxx": "cpp", ".h": "cpp", ".hpp": "cpp", ".hh": "cpp",
		".cs":    "csharp",
		".php":   "php",
		".rb":    "ruby",
		".swift": "swift",
		".tf":    "terraform",
		".lean":  "lean",
		".tex":   "latex", ".cls": "latex", ".sty": "latex", ".bib": "latex",
	}

	sourceFileCount := 0

	_ = filepath.WalkDir(rootAbs, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		// Avoid descending into .git and other heavy pruned directories.
		if entry.IsDir() {
			name := entry.Name()
			if name == ".git" || name == ".hg" || name == ".svn" {
				return fs.SkipDir
			}
			// Reuse pruning knowledge to avoid expensive traversals.
			// This is conservative: it improves performance and matches later scanner
			// behavior for common build/vendor/cache dirs.
			rules := domain.BuildRules(languageSet)
			low := strings.ToLower(name)
			if rules.PruneDirs[name] || rules.PruneDirs[low] {
				return fs.SkipDir
			}
		}

		if entry.Type().IsRegular() {
			ext := strings.ToLower(filepath.Ext(path))
			if lang, ok := sourceExtToLang[ext]; ok {
				languageSet[lang] = true
				sourceFileCount++
			}
		}
		return nil
	})

	isProject := len(markersSet) > 0 || sourceFileCount >= 8
	if !isProject {
		why = append(why, "source file count: "+itoa(sourceFileCount))
	}

	langs := keysSorted(languageSet)
	markers := keysSorted(markersSet)

	return project.ProjectProfileV1{
		RootPath:      rootPath,
		Languages:     langs,
		Markers:       markers,
		IsCodeProject: isProject,
		Why:           why,
	}, nil
}

func keysSorted(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		if strings.TrimSpace(k) == "" {
			continue
		}
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func itoa(n int) string {
	// Small, allocation-light conversion (no strconv import for this file).
	if n == 0 {
		return "0"
	}
	sign := ""
	if n < 0 {
		sign = "-"
		n = -n
	}
	var buf [32]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + (n % 10))
		n /= 10
	}
	return sign + string(buf[i:])
}
