package domain

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	contract "github.com/megamake/megamake/internal/contracts/v1/diagnose"
	project "github.com/megamake/megamake/internal/contracts/v1/project"
	"github.com/megamake/megamake/internal/domains/diagnose/ports"
	"github.com/megamake/megamake/internal/platform/glob"
)

type Runner struct {
	RootPath        string
	Timeout         time.Duration
	IncludeTests    bool
	IgnoreNames     []string
	IgnoreGlobs     []string
	Exec            ports.Exec
}

func (r Runner) Run(profile project.ProjectProfileV1, pyRelFiles []string) (contract.DiagnosticsReportV1, []string) {
	var warnings []string
	var langs []contract.LanguageDiagnosticsV1

	// Keep "attempted languages" consistent (include empty buckets).
	if fileExists(filepath.Join(r.RootPath, "Package.swift")) {
		langs = append(langs, r.runSwift(&warnings))
	}
	if fileExists(filepath.Join(r.RootPath, "tsconfig.json")) || fileExists(filepath.Join(r.RootPath, "package.json")) {
		langs = append(langs, r.runTypeScriptOrJS(&warnings))
	}
	if fileExists(filepath.Join(r.RootPath, "go.mod")) {
		langs = append(langs, r.runGo(&warnings))
	}
	if fileExists(filepath.Join(r.RootPath, "Cargo.toml")) {
		langs = append(langs, r.runRust(&warnings))
	}
	if len(pyRelFiles) > 0 {
		langs = append(langs, r.runPython(pyRelFiles, &warnings))
	}
	if fileExists(filepath.Join(r.RootPath, "pom.xml")) ||
		fileExists(filepath.Join(r.RootPath, "build.gradle")) ||
		fileExists(filepath.Join(r.RootPath, "build.gradle.kts")) {
		langs = append(langs, r.runJava(&warnings))
	}
	if fileExists(filepath.Join(r.RootPath, "lakefile.lean")) || fileExists(filepath.Join(r.RootPath, "lean-toolchain")) {
		langs = append(langs, r.runLean(&warnings))
	}

	// If we detected no markers but profile languages exist, still emit empty buckets for consistency.
	if len(langs) == 0 && len(profile.Languages) > 0 {
		for _, l := range profile.Languages {
			langs = append(langs, contract.LanguageDiagnosticsV1{Name: l, Tool: "", Issues: nil})
		}
	}

	// Filter ignored paths.
	filtered := make([]contract.LanguageDiagnosticsV1, 0, len(langs))
	for _, ld := range langs {
		ld.Issues = r.filterIssues(ld.Issues)
		// Stable ordering within a language
		ld.Issues = SortedIssuesByFile(ld.Issues)
		filtered = append(filtered, ld)
	}

	// Stable ordering of language buckets
	sort.Slice(filtered, func(i, j int) bool { return filtered[i].Name < filtered[j].Name })

	rep := contract.DiagnosticsReportV1{
		Languages:   filtered,
		GeneratedAt: "", // filled by app/service using clock
		Warnings:    warnings,
	}
	return rep, warnings
}

func (r Runner) runSwift(warnings *[]string) contract.LanguageDiagnosticsV1 {
	tool := "swift build"
	var issues []contract.DiagnosticV1
	p, ok := r.Exec.Which("swift")
	if !ok {
		*warnings = append(*warnings, "swift not found in PATH; skipping Swift diagnostics")
		return contract.LanguageDiagnosticsV1{Name: "swift", Tool: tool, Issues: nil}
	}
	args := []string{"build", "-c", "debug"}
	if r.IncludeTests {
		args = append(args, "--build-tests")
	}
	res := r.Exec.Run(p, args, r.RootPath, r.Timeout)
	issues = append(issues, ParseSwift(res.Stdout, res.Stderr)...)
	return contract.LanguageDiagnosticsV1{Name: "swift", Tool: tool, Issues: issues}
}

func (r Runner) runLean(warnings *[]string) contract.LanguageDiagnosticsV1 {
	tool := "lake build"
	var issues []contract.DiagnosticV1
	p, ok := r.Exec.Which("lake")
	if !ok {
		*warnings = append(*warnings, "lake not found in PATH; skipping Lean diagnostics (install Lean 4 via elan, which provides lake)")
		return contract.LanguageDiagnosticsV1{Name: "lean", Tool: tool, Issues: nil}
	}
	res := r.Exec.Run(p, []string{"build"}, r.RootPath, r.Timeout)
	issues = append(issues, ParseLean(res.Stdout, res.Stderr)...)
	return contract.LanguageDiagnosticsV1{Name: "lean", Tool: tool, Issues: issues}
}

func (r Runner) runTypeScriptOrJS(warnings *[]string) contract.LanguageDiagnosticsV1 {
	hasTS := fileExists(filepath.Join(r.RootPath, "tsconfig.json"))
	lang := "javascript"
	if hasTS {
		lang = "typescript"
	}
	usedTool := ""

	var issues []contract.DiagnosticV1

	tryTSC := func(args []string) bool {
		if npx, ok := r.Exec.Which("npx"); ok {
			usedTool = "npx tsc"
			res := r.Exec.Run(npx, append([]string{"-y", "tsc"}, args...), r.RootPath, r.Timeout)
			diags := ParseTypeScript(res.Stdout, res.Stderr, lang, usedTool)
			issues = append(issues, diags...)
			return len(diags) > 0 || res.ExitCode != 0
		}
		if tsc, ok := r.Exec.Which("tsc"); ok {
			usedTool = "tsc"
			res := r.Exec.Run(tsc, args, r.RootPath, r.Timeout)
			diags := ParseTypeScript(res.Stdout, res.Stderr, lang, usedTool)
			issues = append(issues, diags...)
			return len(diags) > 0 || res.ExitCode != 0
		}
		return false
	}

	if hasTS {
		_ = tryTSC([]string{"-p", ".", "--noEmit"})
	} else {
		// JS-only: attempt tsc in checkJs mode, then eslint unix.
		ok := tryTSC([]string{"--allowJs", "--checkJs", "--noEmit"})
		if !ok || len(issues) == 0 {
			if npx, ok2 := r.Exec.Which("npx"); ok2 {
				usedTool = "eslint -f unix"
				res := r.Exec.Run(npx, []string{"-y", "eslint", "-f", "unix", "."}, r.RootPath, r.Timeout)
				issues = append(issues, ParseUnixStyle(res.Stdout, res.Stderr, "javascript", "eslint")...)
			} else if eslint, ok2 := r.Exec.Which("eslint"); ok2 {
				usedTool = "eslint -f unix"
				res := r.Exec.Run(eslint, []string{"-f", "unix", "."}, r.RootPath, r.Timeout)
				issues = append(issues, ParseUnixStyle(res.Stdout, res.Stderr, "javascript", "eslint")...)
			} else {
				*warnings = append(*warnings, "tsc and eslint not found; skipping JS/TS diagnostics")
			}
		}
	}

	// When including tests, run eslint over common test globs if available.
	if r.IncludeTests {
		globs := []string{}
		if hasTS {
			globs = []string{"**/*.test.ts", "**/*.spec.ts", "**/*.test.tsx", "**/*.spec.tsx"}
		} else {
			globs = []string{"**/*.test.js", "**/*.spec.js", "**/*.test.jsx", "**/*.spec.jsx"}
		}
		if npx, ok := r.Exec.Which("npx"); ok {
			res := r.Exec.Run(npx, append([]string{"-y", "eslint", "-f", "unix"}, globs...), r.RootPath, r.Timeout)
			issues = append(issues, ParseUnixStyle(res.Stdout, res.Stderr, lang, "eslint")...)
			if usedTool == "" {
				usedTool = "eslint -f unix"
			}
		} else if eslint, ok := r.Exec.Which("eslint"); ok {
			res := r.Exec.Run(eslint, append([]string{"-f", "unix"}, globs...), r.RootPath, r.Timeout)
			issues = append(issues, ParseUnixStyle(res.Stdout, res.Stderr, lang, "eslint")...)
			if usedTool == "" {
				usedTool = "eslint -f unix"
			}
		}
	}

	if usedTool == "" {
		usedTool = "js/ts diagnostics"
	}

	return contract.LanguageDiagnosticsV1{
		Name:   lang,
		Tool:   usedTool,
		Issues: issues,
	}
}

func (r Runner) runGo(warnings *[]string) contract.LanguageDiagnosticsV1 {
	tool := "go build"
	var issues []contract.DiagnosticV1
	goPath, ok := r.Exec.Which("go")
	if !ok {
		*warnings = append(*warnings, "go not found in PATH; skipping Go diagnostics")
		return contract.LanguageDiagnosticsV1{Name: "go", Tool: tool, Issues: nil}
	}

	// Global build
	res := r.Exec.Run(goPath, []string{"build", "-gcflags=all=-e", "./..."}, r.RootPath, r.Timeout)
	issues = append(issues, ParseGo(res.Stdout, res.Stderr)...)

	// Per-package build (best-effort)
	pkgs := r.listGoPackages(goPath)
	for _, pkg := range pkgs {
		res2 := r.Exec.Run(goPath, []string{"build", "-gcflags=all=-e", pkg}, r.RootPath, r.Timeout)
		issues = append(issues, ParseGo(res2.Stdout, res2.Stderr)...)

		if r.IncludeTests {
			devNull := r.Exec.DevNullPath()
			resT := r.Exec.Run(goPath, []string{"test", "-c", "-o", devNull, pkg}, r.RootPath, r.Timeout)
			issues = append(issues, ParseGo(resT.Stdout, resT.Stderr)...)
		}
	}

	return contract.LanguageDiagnosticsV1{Name: "go", Tool: "go build (-gcflags=all=-e)", Issues: issues}
}

func (r Runner) listGoPackages(goPath string) []string {
	res := r.Exec.Run(goPath, []string{"list", "./..."}, r.RootPath, r.Timeout)
	combined := res.Stdout + "\n" + res.Stderr
	lines := strings.Split(combined, "\n")
	set := map[string]bool{}
	for _, ln := range lines {
		s := strings.TrimSpace(ln)
		if s == "" {
			continue
		}
		if s == "std" {
			continue
		}
		set[s] = true
	}
	var out []string
	for k := range set {
		// Skip vendor-ish packages.
		if strings.Contains(k, "/vendor/") || strings.HasSuffix(k, "/vendor") {
			continue
		}
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func (r Runner) runRust(warnings *[]string) contract.LanguageDiagnosticsV1 {
	tool := "cargo check"
	var issues []contract.DiagnosticV1
	cargo, ok := r.Exec.Which("cargo")
	if !ok {
		*warnings = append(*warnings, "cargo not found in PATH; skipping Rust diagnostics")
		return contract.LanguageDiagnosticsV1{Name: "rust", Tool: tool, Issues: nil}
	}
	res := r.Exec.Run(cargo, []string{"check", "--color", "never"}, r.RootPath, r.Timeout)
	issues = append(issues, ParseRust(res.Stdout, res.Stderr)...)

	if r.IncludeTests {
		resT := r.Exec.Run(cargo, []string{"test", "--no-run", "--color", "never"}, r.RootPath, r.Timeout)
		issues = append(issues, ParseRust(resT.Stdout, resT.Stderr)...)
	}
	return contract.LanguageDiagnosticsV1{Name: "rust", Tool: tool, Issues: issues}
}

func (r Runner) runPython(pyRelFiles []string, warnings *[]string) contract.LanguageDiagnosticsV1 {
	tool := "python -m py_compile"
	var issues []contract.DiagnosticV1
	py := ""
	if p, ok := r.Exec.Which("python3"); ok {
		py = p
	} else if p, ok := r.Exec.Which("python"); ok {
		py = p
	}
	if py == "" {
		*warnings = append(*warnings, "python3/python not found in PATH; skipping Python diagnostics")
		return contract.LanguageDiagnosticsV1{Name: "python", Tool: tool, Issues: nil}
	}

	for _, rel := range pyRelFiles {
		abs := filepath.Join(r.RootPath, filepath.FromSlash(rel))
		res := r.Exec.Run(py, []string{"-m", "py_compile", abs}, r.RootPath, minDuration(r.Timeout, 30*time.Second))
		issues = append(issues, ParsePython(res.Stdout, res.Stderr)...)
	}
	return contract.LanguageDiagnosticsV1{Name: "python", Tool: tool, Issues: issues}
}

func (r Runner) runJava(warnings *[]string) contract.LanguageDiagnosticsV1 {
	// Maven preferred, then Gradle/Gradle wrapper.
	if fileExists(filepath.Join(r.RootPath, "pom.xml")) {
		if mvn, ok := r.Exec.Which("mvn"); ok {
			args := []string{"-q", "-DskipTests", "compile"}
			tool := "mvn compile"
			if r.IncludeTests {
				args = []string{"-q", "-DskipTests", "test-compile"}
				tool = "mvn test-compile"
			}
			res := r.Exec.Run(mvn, args, r.RootPath, r.Timeout)
			issues := ParseJava(res.Stdout, res.Stderr)
			return contract.LanguageDiagnosticsV1{Name: "java", Tool: tool, Issues: issues}
		}
		*warnings = append(*warnings, "mvn not found; skipping Maven diagnostics")
	}

	gradlePath := ""
	// Prefer local wrapper if present.
	if fileExists(filepath.Join(r.RootPath, "gradlew")) {
		gradlePath = filepath.Join(r.RootPath, "gradlew")
	}
	if gradlePath == "" {
		if g, ok := r.Exec.Which("gradle"); ok {
			gradlePath = g
		}
	}
	if gradlePath != "" {
		task := "classes"
		if r.IncludeTests {
			task = "testClasses"
		}
		res := r.Exec.Run(gradlePath, []string{"-q", task}, r.RootPath, r.Timeout)
		issues := ParseJava(res.Stdout, res.Stderr)
		return contract.LanguageDiagnosticsV1{Name: "java", Tool: "gradle "+task, Issues: issues}
	}

	*warnings = append(*warnings, "no Maven/Gradle found; skipping Java diagnostics")
	return contract.LanguageDiagnosticsV1{Name: "java", Tool: "javac/maven", Issues: nil}
}

func (r Runner) filterIssues(issues []contract.DiagnosticV1) []contract.DiagnosticV1 {
	if len(r.IgnoreNames) == 0 && len(r.IgnoreGlobs) == 0 {
		return issues
	}

	ignoreNames := map[string]bool{}
	for _, n := range r.IgnoreNames {
		n = strings.TrimSpace(n)
		if n == "" {
			continue
		}
		ignoreNames[n] = true
		ignoreNames[strings.ToLower(n)] = true
	}

	ignoreGlobs := []string{}
	for _, g := range r.IgnoreGlobs {
		g = strings.TrimSpace(g)
		if g == "" {
			continue
		}
		ignoreGlobs = append(ignoreGlobs, filepathToSlash(g))
	}

	var out []contract.DiagnosticV1
	for _, d := range issues {
		if strings.TrimSpace(d.File) == "" {
			out = append(out, d)
			continue
		}
		if r.isIgnoredPath(d.File, ignoreNames, ignoreGlobs) {
			continue
		}
		out = append(out, d)
	}
	return out
}

func (r Runner) isIgnoredPath(filePath string, ignoreNames map[string]bool, ignoreGlobs []string) bool {
	// Attempt to compute a relpath under root; fall back to last path component if needed.
	rel := filePath
	rootAbs, _ := filepath.Abs(r.RootPath)
	fpAbs, err := filepath.Abs(filePath)
	if err == nil {
		ra := filepathToSlash(rootAbs)
		fa := filepathToSlash(fpAbs)
		if strings.HasPrefix(fa, ra+"/") {
			rel = strings.TrimPrefix(fa, ra+"/")
		} else {
			rel = filepath.Base(filePath)
		}
	} else {
		rel = filepath.Base(filePath)
	}
	rel = filepathToSlash(rel)

	parts := strings.Split(rel, "/")
	for _, p := range parts {
		if p == "" {
			continue
		}
		if ignoreNames[p] || ignoreNames[strings.ToLower(p)] {
			return true
		}
	}

	for _, pat := range ignoreGlobs {
		if glob.Match(rel, pat) {
			return true
		}
	}
	return false
}

func filepathToSlash(p string) string {
	return strings.ReplaceAll(p, "\\", "/")
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func minDuration(a, b time.Duration) time.Duration {
	if a <= b {
		return a
	}
	return b
}

// Ensure we can parse "A..B" style args safely if needed later.
var _ = regexp.MustCompile
