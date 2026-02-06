package app

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	contractartifact "github.com/megamake/megamake/internal/contracts/v1/artifact"
	project "github.com/megamake/megamake/internal/contracts/v1/project"
	contract "github.com/megamake/megamake/internal/contracts/v1/testplan"
	repoapi "github.com/megamake/megamake/internal/domains/repo/api"
	"github.com/megamake/megamake/internal/domains/testplan/domain"
	"github.com/megamake/megamake/internal/domains/testplan/ports"
	"github.com/megamake/megamake/internal/platform/clock"
)

type Service struct {
	Clock          clock.Clock
	Repo           repoapi.API
	ArtifactWriter ports.ArtifactWriter
	Git            ports.Git
}

type RegressionMode struct {
	Disabled bool
	SinceRef string
	Range    string
}

type BuildRequest struct {
	RootPath    string
	ArtifactDir string
	Force       bool

	LimitSubjects int
	LevelsCSV     string

	MaxFileBytes    int64
	MaxAnalyzeBytes int64
	IgnoreNames     []string
	IgnoreGlobs     []string

	Regression RegressionMode

	NetEnabled   bool
	AllowDomains []string
	Args         []string
}

type BuildResult struct {
	Report     contract.TestPlanReportV1
	ReportXML  string
	ReportJSON string
	TestPrompt string

	ArtifactPath string
	LatestPath   string
}

func (s *Service) Build(req BuildRequest) (BuildResult, error) {
	if s.Clock == nil {
		return BuildResult{}, fmt.Errorf("internal error: Clock is nil")
	}
	if s.Repo == nil {
		return BuildResult{}, fmt.Errorf("internal error: Repo is nil")
	}
	if s.ArtifactWriter == nil {
		return BuildResult{}, fmt.Errorf("internal error: ArtifactWriter is nil")
	}

	if strings.TrimSpace(req.RootPath) == "" {
		req.RootPath = "."
	}
	if strings.TrimSpace(req.ArtifactDir) == "" {
		req.ArtifactDir = req.RootPath
	}
	if req.LimitSubjects <= 0 {
		req.LimitSubjects = 500
	}
	if req.LimitSubjects < 50 {
		req.LimitSubjects = 50
	}
	if req.MaxFileBytes <= 0 {
		req.MaxFileBytes = 1_500_000
	}
	if req.MaxAnalyzeBytes <= 0 {
		req.MaxAnalyzeBytes = 200_000
	}

	now := s.Clock.NowUTC()

	profile, err := s.Repo.Detect(req.RootPath)
	if err != nil {
		return BuildResult{}, err
	}
	if !profile.IsCodeProject && !req.Force {
		return BuildResult{}, fmt.Errorf(buildSafetyStopMessage(profile))
	}

	files, err := s.Repo.Scan(req.RootPath, profile, repoapi.ScanOptions{
		MaxFileBytes: req.MaxFileBytes,
		IgnoreNames:  req.IgnoreNames,
		IgnoreGlobs:  req.IgnoreGlobs,
	})
	if err != nil {
		return BuildResult{}, err
	}

	// Separate test files for coverage; exclude from subjects.
	var testFiles []project.FileRefV1
	var codeFiles []project.FileRefV1
	for _, f := range files {
		if f.IsTest {
			testFiles = append(testFiles, f)
		} else {
			codeFiles = append(codeFiles, f)
		}
	}
	sort.Slice(testFiles, func(i, j int) bool { return testFiles[i].RelPath < testFiles[j].RelPath })
	sort.Slice(codeFiles, func(i, j int) bool { return codeFiles[i].RelPath < codeFiles[j].RelPath })

	// Read helper
	readRel := func(rel string, maxBytes int64) (string, bool) {
		b, err := s.Repo.ReadFileRel(req.RootPath, rel, maxBytes)
		if err != nil {
			return "", false
		}
		if maxBytes > 0 && int64(len(b)) > maxBytes {
			b = b[:maxBytes]
		}
		// Assume text; if binary, it will produce odd subjects but won't crash.
		return string(b), true
	}

	frameworks := domain.DetectFrameworks(func(rel string, maxBytes int64) (string, bool) {
		return readRel(rel, maxBytes)
	})

	// Regression impacted files
	impacted := map[string]bool{}
	regressionDesc := ""
	if !req.Regression.Disabled && s.Git != nil {
		if strings.TrimSpace(req.Regression.Range) != "" {
			for _, p := range s.Git.ChangedFilesInRange(req.RootPath, req.Regression.Range) {
				impacted[p] = true
			}
			regressionDesc = req.Regression.Range
		} else if strings.TrimSpace(req.Regression.SinceRef) != "" {
			for _, p := range s.Git.ChangedFilesSince(req.RootPath, req.Regression.SinceRef) {
				impacted[p] = true
			}
			regressionDesc = "since " + req.Regression.SinceRef
		}
	}

	levels := contract.ParseLevelSetV1(req.LevelsCSV)

	// Analyze subjects round-robin per language for fairness.
	queues := map[string][]project.FileRefV1{}
	for _, f := range codeFiles {
		lang := domain.LanguageForRel(f.RelPath)
		if lang == "" {
			continue
		}
		queues[lang] = append(queues[lang], f)
	}
	langKeys := make([]string, 0, len(queues))
	for k := range queues {
		langKeys = append(langKeys, k)
	}
	sort.Strings(langKeys)

	perLangSubjects := map[string][]contract.TestSubjectV1{}
	totalSubjects := 0

	progressed := true
	for progressed && totalSubjects < req.LimitSubjects {
		progressed = false
		for _, lang := range langKeys {
			q := queues[lang]
			if len(q) == 0 {
				continue
			}
			f := q[0]
			queues[lang] = q[1:]
			progressed = true

			text, ok := readRel(f.RelPath, req.MaxAnalyzeBytes)
			if !ok {
				continue
			}
			subs := domain.AnalyzeFile(f.RelPath, text, lang)
			if len(subs) == 0 {
				continue
			}

			remaining := req.LimitSubjects - totalSubjects
			if remaining <= 0 {
				break
			}
			if len(subs) > remaining {
				subs = subs[:remaining]
			}
			perLangSubjects[lang] = append(perLangSubjects[lang], subs...)
			totalSubjects += len(subs)
			if totalSubjects >= req.LimitSubjects {
				break
			}
		}
	}

	// Flatten subjects for coverage
	var allSubjects []contract.TestSubjectV1
	for _, lang := range langKeys {
		allSubjects = append(allSubjects, perLangSubjects[lang]...)
	}

	coverageMap := domain.AssessCoverage(allSubjects, testFiles, func(rel string, maxBytes int64) (string, bool) {
		return readRel(rel, maxBytes)
	}, req.MaxAnalyzeBytes)

	// Test files count per language
	perLangTestCount := map[string]int{}
	for _, tf := range testFiles {
		lang := domain.LanguageForRel(tf.RelPath)
		if lang != "" {
			perLangTestCount[lang]++
		}
	}

	// Build plans
	var languagePlans []contract.LanguagePlanV1
	totalScenarios := 0

	for _, lang := range langKeys {
		subs := perLangSubjects[lang]
		sort.Slice(subs, func(i, j int) bool { return subs[i].ID < subs[j].ID })

		var subjectPlans []contract.SubjectPlanV1
		for _, subj := range subs {
			cov, ok := coverageMap[subj.ID]
			if !ok {
				cov = contract.CoverageV1{Flag: contract.CoverageRed, Status: "MISSING", Score: 0, Notes: []string{"no tests found"}}
			}

			scenarios := domain.BuildScenarios(subj, levels)

			// Regression add-on
			if impacted[subj.Path] {
				scenarios = append(scenarios, domain.RegressionScenario(subj, regressionDesc))
			}

			// Suppress non-regression for DONE items
			if cov.Flag == contract.CoverageGreen {
				var keep []contract.ScenarioSuggestionV1
				for _, sc := range scenarios {
					if sc.Level == contract.LevelRegression {
						keep = append(keep, sc)
					}
				}
				scenarios = keep
			}

			totalScenarios += len(scenarios)
			subjectPlans = append(subjectPlans, contract.SubjectPlanV1{
				Subject:   subj,
				Scenarios: scenarios,
				Coverage:  cov,
			})
		}

		fw := frameworks[lang]
		languagePlans = append(languagePlans, contract.LanguagePlanV1{
			Name:           lang,
			Frameworks:     fw,
			Subjects:       subjectPlans,
			TestFilesFound: perLangTestCount[lang],
		})
	}

	report := contract.TestPlanReportV1{
		Languages:   languagePlans,
		GeneratedAt: contractartifact.FormatRFC3339NanoUTC(now),
		Summary: contract.PlanSummaryV1{
			TotalLanguages: len(languagePlans),
			TotalSubjects:  totalSubjects,
			TotalScenarios: totalScenarios,
		},
		Warnings: nil,
	}

	reportXML := report.ToXML()
	jb, _ := json.MarshalIndent(report, "", "  ")
	reportJSON := string(jb)

	testPrompt := GenerateTestPrompt(report, levels, req.RootPath)

	meta := contractartifact.ArtifactMetaV1{
		Tool:         "megatest",
		Contract:     "v1",
		GeneratedAt:  contractartifact.FormatRFC3339NanoUTC(now),
		RootPath:     req.RootPath,
		Args:         req.Args,
		NetEnabled:   req.NetEnabled,
		AllowDomains: cloneStrings(req.AllowDomains),
		Warnings:     report.Warnings,
	}

	env := contractartifact.ArtifactEnvelopeV1{
		Meta:   meta,
		XML:    reportXML,
		JSON:   reportJSON,
		Prompt: testPrompt,
	}

	artifactPath, latestPath, err := s.ArtifactWriter.WriteToolArtifact(ports.WriteArtifactRequest{
		ArtifactDir:    req.ArtifactDir,
		ToolPrefix:     "MEGATEST",
		Envelope:       env,
		GeneratedAtUTC: timePtr(now),
	})
	if err != nil {
		return BuildResult{}, err
	}

	return BuildResult{
		Report:       report,
		ReportXML:    reportXML,
		ReportJSON:   reportJSON,
		TestPrompt:   testPrompt,
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

// GenerateTestPrompt produces an agent prompt that mirrors the Swift TestPrompter intent.
func GenerateTestPrompt(plan contract.TestPlanReportV1, levels contract.LevelSetV1, root string) string {
	var lines []string
	lines = append(lines, "You are an expert test developer. Create tests according to this plan without rewriting architecture.")
	lines = append(lines, "")
	lines = append(lines, "Context:")
	langs := []string{}
	for _, lp := range plan.Languages {
		langs = append(langs, lp.Name)
	}
	sort.Strings(langs)
	lines = append(lines, "- Languages: "+strings.Join(langs, ", "))
	lines = append(lines, "- Subjects: "+itoa(plan.Summary.TotalSubjects)+", Scenarios: "+itoa(plan.Summary.TotalScenarios))
	lines = append(lines, "")

	lines = append(lines, "Instructions:")
	lines = append(lines, "- Write tests for levels: "+levelsCSV(levels))
	lines = append(lines, "- For [DONE], do not re-add tests; only add regression if required.")
	lines = append(lines, "- For [PARTIAL], improve edge coverage and negative paths; for [MISSING], create focused tests.")
	lines = append(lines, "- Prefer deterministic, hermetic tests; stub/mock external I/O.")
	lines = append(lines, "")

	lines = append(lines, "Plan overview:")
	for _, lp := range plan.Languages {
		lines = append(lines, "- "+lp.Name+" (frameworks: "+strings.Join(lp.Frameworks, ", ")+")")
		limit := 25
		if len(lp.Subjects) < limit {
			limit = len(lp.Subjects)
		}
		for i := 0; i < limit; i++ {
			sp := lp.Subjects[i]
			s := sp.Subject
			tag := tagFor(sp.Coverage.Flag)
			lines = append(lines, "  • ["+tag+"] "+string(s.Kind)+" "+s.Name+" @ "+s.Path+" [risk "+itoa(s.RiskScore)+"]")
			if sp.Coverage.Flag != contract.CoverageGreen {
				for _, sc := range sp.Scenarios {
					lines = append(lines, "     - ["+string(sc.Level)+"] "+sc.Title)
				}
			} else {
				for _, ev := range sp.Coverage.Evidence {
					lines = append(lines, "     - DONE in "+ev.File+" (hits "+itoa(ev.Hits)+")")
				}
			}
		}
		if len(lp.Subjects) > limit {
			lines = append(lines, "  • ... "+itoa(len(lp.Subjects)-limit)+" more")
		}
	}
	return strings.Join(lines, "\n")
}

func levelsCSV(ls contract.LevelSetV1) string {
	var out []string
	all := []contract.TestLevelV1{contract.LevelSmoke, contract.LevelUnit, contract.LevelIntegration, contract.LevelE2E, contract.LevelRegression}
	for _, l := range all {
		if ls.Has(l) {
			out = append(out, string(l))
		}
	}
	return strings.Join(out, ", ")
}

func tagFor(flag contract.CoverageFlagV1) string {
	switch flag {
	case contract.CoverageGreen:
		return "DONE"
	case contract.CoverageYellow:
		return "PARTIAL"
	default:
		return "MISSING"
	}
}

func itoa(n int) string {
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

// Ensure filepath is linked (used by earlier logic).
var _ = filepath.Base
