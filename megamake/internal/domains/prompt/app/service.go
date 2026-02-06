package app

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	contractartifact "github.com/megamake/megamake/internal/contracts/v1/artifact"
	project "github.com/megamake/megamake/internal/contracts/v1/project"
	contractprompt "github.com/megamake/megamake/internal/contracts/v1/prompt"
	promptdomain "github.com/megamake/megamake/internal/domains/prompt/domain"
	"github.com/megamake/megamake/internal/domains/prompt/ports"
	repoapi "github.com/megamake/megamake/internal/domains/repo/api"
	"github.com/megamake/megamake/internal/platform/clock"
)

type Service struct {
	Clock          clock.Clock
	Repo           repoapi.API
	ArtifactWriter ports.ArtifactWriter
	Clipboard      ports.Clipboard
}

type GenerateRequest struct {
	RootPath     string
	ArtifactDir  string
	Force        bool
	MaxFileBytes int64
	IgnoreNames  []string
	IgnoreGlobs  []string

	// CopyToClipboard is best-effort and must not fail the run if clipboard is unavailable.
	CopyToClipboard bool
}

type GenerateResult struct {
	ContextXML  string
	Report      contractprompt.PromptReportV1
	ReportJSON  string
	AgentPrompt string

	ArtifactPath string
	LatestPath   string
	Copied       bool
}

func (s *Service) Generate(req GenerateRequest) (GenerateResult, error) {
	if s.Clock == nil {
		return GenerateResult{}, fmt.Errorf("internal error: Clock is nil")
	}
	if s.Repo == nil {
		return GenerateResult{}, fmt.Errorf("internal error: Repo is nil")
	}
	if s.ArtifactWriter == nil {
		return GenerateResult{}, fmt.Errorf("internal error: ArtifactWriter is nil")
	}
	if strings.TrimSpace(req.RootPath) == "" {
		req.RootPath = "."
	}
	if strings.TrimSpace(req.ArtifactDir) == "" {
		req.ArtifactDir = req.RootPath
	}
	if req.MaxFileBytes <= 0 {
		req.MaxFileBytes = 1_500_000
	}

	now := s.Clock.NowUTC()

	profile, err := s.Repo.Detect(req.RootPath)
	if err != nil {
		return GenerateResult{}, err
	}
	if !profile.IsCodeProject && !req.Force {
		return GenerateResult{}, fmt.Errorf(buildSafetyStopMessage(profile))
	}

	files, err := s.Repo.Scan(req.RootPath, profile, repoapi.ScanOptions{
		MaxFileBytes: req.MaxFileBytes,
		IgnoreNames:  req.IgnoreNames,
		IgnoreGlobs:  req.IgnoreGlobs,
	})
	if err != nil {
		return GenerateResult{}, err
	}

	var inputs []promptdomain.FileInput
	inputs = make([]promptdomain.FileInput, 0, len(files))

	var totalBytes int64
	var warnings []string

	for _, f := range files {
		totalBytes += f.SizeBytes
		b, err := s.Repo.ReadFileRel(req.RootPath, f.RelPath, req.MaxFileBytes)
		if err != nil {
			warnings = append(warnings, "unable to read "+f.RelPath+": "+err.Error())
			continue
		}
		inputs = append(inputs, promptdomain.FileInput{
			RelPath: f.RelPath,
			Content: b,
		})
	}

	contextXML, buildWarnings := promptdomain.BuildContextBlob(inputs)
	warnings = append(warnings, buildWarnings...)

	report := contractprompt.PromptReportV1{
		GeneratedAt:   contractartifact.FormatRFC3339NanoUTC(now),
		RootPath:      req.RootPath,
		Profile:       profile,
		FilesScanned:  len(files),
		FilesIncluded: len(inputs),
		TotalBytes:    totalBytes,
		Files:         files,
		Warnings:      warnings,
	}

	reportJSONBytes, _ := json.MarshalIndent(report, "", "  ")
	reportJSON := string(reportJSONBytes)

	agentPrompt := strings.Join([]string{
		"You are an expert software engineer.",
		"Using the <context> below (real source files), propose a MegaPatch v1 script.",
		"Rules:",
		"- Do not delete directories. File deletes require explicit user consent.",
		"- Keep changes minimal and correct; preserve existing architecture.",
		"- Return only a single MegaPatch script (no prose).",
	}, "\n")

	meta := contractartifact.ArtifactMetaV1{
		Tool:         "megaprompt",
		Contract:     "v1",
		GeneratedAt:  contractartifact.FormatRFC3339NanoUTC(now),
		RootPath:     req.RootPath,
		Args:         nil,
		NetEnabled:   false,
		AllowDomains: nil,
		Warnings:     warnings,
	}

	envelope := contractartifact.ArtifactEnvelopeV1{
		Meta:   meta,
		XML:    contextXML,
		JSON:   reportJSON,
		Prompt: agentPrompt,
	}

	artifactPath, latestPath, err := s.ArtifactWriter.WriteToolArtifact(ports.WriteArtifactRequest{
		ArtifactDir:    req.ArtifactDir,
		ToolPrefix:     "MEGAPROMPT",
		Envelope:       envelope,
		GeneratedAtUTC: timePtr(now),
	})
	if err != nil {
		return GenerateResult{}, err
	}

	copied := false
	if req.CopyToClipboard && s.Clipboard != nil {
		ok, _ := s.Clipboard.Copy(contextXML)
		copied = ok
	}

	return GenerateResult{
		ContextXML:   contextXML,
		Report:       report,
		ReportJSON:   reportJSON,
		AgentPrompt:  agentPrompt,
		ArtifactPath: artifactPath,
		LatestPath:   latestPath,
		Copied:       copied,
	}, nil
}

func timePtr(t time.Time) *time.Time {
	return &t
}

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
