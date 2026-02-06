package app

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	contractartifact "github.com/megamake/megamake/internal/contracts/v1/artifact"
	contract "github.com/megamake/megamake/internal/contracts/v1/diagnose"
	project "github.com/megamake/megamake/internal/contracts/v1/project"
	"github.com/megamake/megamake/internal/domains/diagnose/domain"
	"github.com/megamake/megamake/internal/domains/diagnose/ports"
	repoapi "github.com/megamake/megamake/internal/domains/repo/api"
	"github.com/megamake/megamake/internal/platform/clock"
)

type Service struct {
	Clock          clock.Clock
	Repo           repoapi.API
	ArtifactWriter ports.ArtifactWriter
	Exec           ports.Exec
}

type DiagnoseRequest struct {
	RootPath       string
	ArtifactDir    string
	Force          bool
	TimeoutSeconds int
	IncludeTests   bool

	MaxFileBytes int64
	IgnoreNames  []string
	IgnoreGlobs  []string

	NetEnabled   bool
	AllowDomains []string
	Args         []string
}

type DiagnoseResult struct {
	Report     contract.DiagnosticsReportV1
	ReportXML  string
	ReportJSON string
	FixPrompt  string

	ArtifactPath string
	LatestPath   string
}

func (s *Service) Diagnose(req DiagnoseRequest) (DiagnoseResult, error) {
	if s.Clock == nil {
		return DiagnoseResult{}, fmt.Errorf("internal error: Clock is nil")
	}
	if s.Repo == nil {
		return DiagnoseResult{}, fmt.Errorf("internal error: Repo is nil")
	}
	if s.ArtifactWriter == nil {
		return DiagnoseResult{}, fmt.Errorf("internal error: ArtifactWriter is nil")
	}
	if s.Exec == nil {
		return DiagnoseResult{}, fmt.Errorf("internal error: Exec is nil")
	}

	if strings.TrimSpace(req.RootPath) == "" {
		req.RootPath = "."
	}
	if strings.TrimSpace(req.ArtifactDir) == "" {
		req.ArtifactDir = req.RootPath
	}
	if req.TimeoutSeconds <= 0 {
		req.TimeoutSeconds = 120
	}
	if req.MaxFileBytes <= 0 {
		req.MaxFileBytes = 1_500_000
	}

	now := s.Clock.NowUTC()

	profile, err := s.Repo.Detect(req.RootPath)
	if err != nil {
		return DiagnoseResult{}, err
	}
	if !profile.IsCodeProject && !req.Force {
		return DiagnoseResult{}, fmt.Errorf(buildSafetyStopMessage(profile))
	}

	// Use repo scan to collect python files under ignore rules, size caps, etc.
	files, err := s.Repo.Scan(req.RootPath, profile, repoapi.ScanOptions{
		MaxFileBytes: req.MaxFileBytes,
		IgnoreNames:  req.IgnoreNames,
		IgnoreGlobs:  req.IgnoreGlobs,
	})
	if err != nil {
		return DiagnoseResult{}, err
	}

	var pyFiles []string
	for _, f := range files {
		if strings.ToLower(filepath.Ext(f.RelPath)) == ".py" {
			// Respect includeTests behavior (best-effort heuristic).
			if !req.IncludeTests && isTestRelPath(f.RelPath) {
				continue
			}
			pyFiles = append(pyFiles, f.RelPath)
		}
	}
	sort.Strings(pyFiles)

	runner := domain.Runner{
		RootPath:     req.RootPath,
		Timeout:      time.Duration(req.TimeoutSeconds) * time.Second,
		IncludeTests: req.IncludeTests,
		IgnoreNames:  req.IgnoreNames,
		IgnoreGlobs:  req.IgnoreGlobs,
		Exec:         s.Exec,
	}

	rep, warnings := runner.Run(profile, pyFiles)
	rep.GeneratedAt = contractartifact.FormatRFC3339NanoUTC(now)
	// Merge warnings from runner into report warnings.
	rep.Warnings = append(rep.Warnings, warnings...)

	fixPrompt := domain.GenerateFixPrompt(rep, req.RootPath)
	xmlOut := rep.ToXML(fixPrompt)

	jsonBytes, _ := json.MarshalIndent(rep, "", "  ")
	jsonOut := string(jsonBytes)

	meta := contractartifact.ArtifactMetaV1{
		Tool:         "megadiagnose",
		Contract:     "v1",
		GeneratedAt:  contractartifact.FormatRFC3339NanoUTC(now),
		RootPath:     req.RootPath,
		Args:         req.Args,
		NetEnabled:   req.NetEnabled,
		AllowDomains: cloneStrings(req.AllowDomains),
		Warnings:     rep.Warnings,
	}

	env := contractartifact.ArtifactEnvelopeV1{
		Meta:   meta,
		XML:    xmlOut,
		JSON:   jsonOut,
		Prompt: fixPrompt,
	}

	artifactPath, latestPath, err := s.ArtifactWriter.WriteToolArtifact(ports.WriteArtifactRequest{
		ArtifactDir:    req.ArtifactDir,
		ToolPrefix:     "MEGADIAG",
		Envelope:       env,
		GeneratedAtUTC: timePtr(now),
	})
	if err != nil {
		return DiagnoseResult{}, err
	}

	return DiagnoseResult{
		Report:       rep,
		ReportXML:    xmlOut,
		ReportJSON:   jsonOut,
		FixPrompt:    fixPrompt,
		ArtifactPath: artifactPath,
		LatestPath:   latestPath,
	}, nil
}

func timePtr(t time.Time) *time.Time { return &t }

func buildSafetyStopMessage(p project.ProjectProfileV1) string {
	var b strings.Builder
	b.WriteString("Safety stop: This directory does not appear to be a code project.\n")
	if len(p.Why) > 0 {
		b.WriteString("Evidence:\n")
		for _, w := range p.Why {
			b.WriteString("  - ")
			b.WriteString(w)
			b.WriteString("\n")
		}
	}
	b.WriteString("If you are certain, re-run with --force.\n")
	return b.String()
}

func cloneStrings(xs []string) []string {
	if len(xs) == 0 {
		return nil
	}
	out := make([]string, 0, len(xs))
	for _, x := range xs {
		x = strings.TrimSpace(x)
		if x == "" {
			continue
		}
		out = append(out, x)
	}
	return out
}

func isTestRelPath(rel string) bool {
	// Local heuristic aligned with earlier Swift/TestHeuristics:
	// filenames like *_test.*, *.test.*, *.spec.* and folders like tests/, test/, __tests__/, spec/
	l := strings.ToLower(rel)
	base := strings.ToLower(filepath.Base(l))

	if strings.Contains(base, ".test.") ||
		strings.Contains(base, ".spec.") ||
		strings.Contains(base, "_test.") ||
		strings.Contains(base, "-test.") ||
		strings.HasPrefix(base, "test_") ||
		strings.Contains(base, "_spec.") {
		return true
	}
	parts := strings.Split(strings.ReplaceAll(l, "\\", "/"), "/")
	for _, p := range parts {
		if p == "test" || p == "tests" || p == "__tests__" || p == "spec" || p == "specs" {
			return true
		}
	}
	return false
}
