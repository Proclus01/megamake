package cli

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/megamake/megamake/internal/app/wiring"
	"github.com/megamake/megamake/internal/platform/console"
	"github.com/megamake/megamake/internal/platform/policy"

	diagapp "github.com/megamake/megamake/internal/domains/diagnose/app"
	docapp "github.com/megamake/megamake/internal/domains/doc/app"
	promptapp "github.com/megamake/megamake/internal/domains/prompt/app"
	tpapp "github.com/megamake/megamake/internal/domains/testplan/app"
)

const (
	exitOK    = 0
	exitUsage = 2
	exitError = 1
)

type stringListFlag struct {
	values []string
}

func (s *stringListFlag) String() string {
	if s == nil || len(s.values) == 0 {
		return ""
	}
	return strings.Join(s.values, ",")
}

func (s *stringListFlag) Set(v string) error {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil
	}
	s.values = append(s.values, v)
	return nil
}

func Run(argv []string) int {
	stdout := os.Stdout
	stderr := os.Stderr
	log := console.New(stderr)
	ctr := wiring.New()

	// Global flags
	var allowDomains stringListFlag
	global := flag.NewFlagSet("megamake", flag.ContinueOnError)
	global.SetOutput(stderr)

	var artifactDir string
	var netEnabled bool
	global.StringVar(&artifactDir, "artifact-dir", "", "Directory where MEGA* artifacts and *_latest pointer files are written.")
	global.BoolVar(&netEnabled, "net", false, "Enable network access (deny-by-default otherwise).")
	global.Var(&allowDomains, "allow-domain", "Allowed domain when --net is set (repeatable). If none provided, all domains allowed when --net is set.")
	global.Usage = func() { writeRootHelp(stderr) }

	if err := global.Parse(argv[1:]); err != nil {
		writeRootHelp(stderr)
		log.Error(fmt.Sprintf("failed to parse global flags: %v", err))
		return exitUsage
	}

	rest := global.Args()
	if len(rest) == 0 {
		writeRootHelp(stderr)
		return exitUsage
	}

	pol := policy.Policy{
		NetEnabled:   netEnabled,
		AllowDomains: allowDomains.values,
	}

	cmd := rest[0]
	args := rest[1:]

	switch cmd {
	case "help", "-h", "--help":
		writeRootHelp(stdout)
		return exitOK
	case "prompt":
		return runPrompt(ctr, pol, artifactDir, args, stdout, stderr)
	case "doc":
		return runDoc(ctr, pol, artifactDir, args, stdout, stderr)
	case "diagnose":
		return runDiagnose(ctr, pol, artifactDir, args, stdout, stderr)
	case "test":
		return runTest(ctr, pol, artifactDir, args, stdout, stderr)
	case "secure", "patch", "make":
		log.Warn(cmd + ": not implemented yet")
		return exitUsage
	default:
		log.Error("unknown command: " + cmd)
		writeRootHelp(stderr)
		return exitUsage
	}
}

func runPrompt(ctr wiring.Container, pol policy.Policy, globalArtifactDir string, argv []string, stdout io.Writer, stderr io.Writer) int {
	log := console.New(stderr)

	fs := flag.NewFlagSet("prompt", flag.ContinueOnError)
	fs.SetOutput(stderr)

	var jsonOut string
	var promptOut string
	var force bool
	var maxFileBytes int64
	var copyToClipboard bool
	var showSummary bool

	var ignores stringListFlag
	fs.Var(&ignores, "ignore", "Directory names or glob paths to ignore (repeatable). Example: --ignore build --ignore docs/generated/**")
	fs.Var(&ignores, "I", "Alias for --ignore (repeatable).")
	fs.StringVar(&jsonOut, "json-out", "", "Write JSON report to this path (optional).")
	fs.StringVar(&promptOut, "prompt-out", "", "Write agent prompt text to this path (optional).")
	fs.BoolVar(&force, "force", false, "Force run even if the directory does not look like a code project.")
	fs.Int64Var(&maxFileBytes, "max-file-bytes", 1_500_000, "Skip files larger than this many bytes during scanning.")
	fs.BoolVar(&copyToClipboard, "copy", false, "Best-effort: also copy the generated <context> blob to clipboard.")
	fs.BoolVar(&showSummary, "show-summary", true, "Print a brief summary to stderr.")
	fs.Usage = func() { writePromptHelp(stderr) }

	if err := fs.Parse(argv); err != nil {
		writePromptHelp(stderr)
		log.Error(fmt.Sprintf("failed to parse prompt flags: %v", err))
		return exitUsage
	}

	rootPath := "."
	if rest := fs.Args(); len(rest) >= 1 {
		rootPath = rest[0]
	}

	artifactRoot := computeArtifactDir(globalArtifactDir, rootPath)
	ignoreNames, ignoreGlobs := splitIgnores(ignores.values)

	res, err := ctr.Prompt.Generate(promptapp.GenerateRequest{
		RootPath:        rootPath,
		ArtifactDir:     artifactRoot,
		Force:           force,
		MaxFileBytes:    maxFileBytes,
		IgnoreNames:     ignoreNames,
		IgnoreGlobs:     ignoreGlobs,
		CopyToClipboard: copyToClipboard,
	})
	if err != nil {
		log.Error(err.Error())
		return exitError
	}

	if _, err := io.WriteString(stdout, res.ContextXML); err != nil {
		log.Error(fmt.Sprintf("failed writing to stdout: %v", err))
		return exitError
	}

	if jsonOut != "" {
		if err := os.WriteFile(jsonOut, []byte(res.ReportJSON+"\n"), 0o644); err != nil {
			log.Error(fmt.Sprintf("failed writing --json-out: %v", err))
			return exitError
		}
	}
	if promptOut != "" {
		if err := os.WriteFile(promptOut, []byte(res.AgentPrompt+"\n"), 0o644); err != nil {
			log.Error(fmt.Sprintf("failed writing --prompt-out: %v", err))
			return exitError
		}
	}

	if showSummary {
		log.Info("mode: prompt")
		log.Info("root: " + rootPath)
		log.Info("files scanned: " + itoa(res.Report.FilesScanned) + ", included: " + itoa(res.Report.FilesIncluded))
		log.Info("artifact: " + res.ArtifactPath)
		log.Info("latest pointer: " + res.LatestPath)
		if copyToClipboard {
			log.Info("clipboard copied: " + boolString(res.Copied))
		}
		if len(ignoreNames) > 0 {
			log.Info("ignore names: " + strings.Join(ignoreNames, ", "))
		}
		if len(ignoreGlobs) > 0 {
			log.Info("ignore globs: " + strings.Join(ignoreGlobs, ", "))
		}
	}

	return exitOK
}

func runDoc(ctr wiring.Container, pol policy.Policy, globalArtifactDir string, argv []string, stdout io.Writer, stderr io.Writer) int {
	log := console.New(stderr)

	if len(argv) == 0 {
		writeDocHelp(stderr)
		return exitUsage
	}

	sub := argv[0]
	args := argv[1:]

	switch sub {
	case "create":
		fs := flag.NewFlagSet("doc create", flag.ContinueOnError)
		fs.SetOutput(stderr)

		var jsonOut string
		var promptOut string
		var umlOut string

		var force bool
		var showSummary bool
		var maxFileBytes int64
		var maxAnalyzeBytes int64
		var treeDepth int

		var umlFormats string
		var umlGranularity string
		var umlMaxNodes int
		var umlIncludeIO bool
		var umlIncludeEndpoints bool

		var ignores stringListFlag

		fs.Var(&ignores, "ignore", "Directory names or glob paths to ignore (repeatable).")
		fs.Var(&ignores, "I", "Alias for --ignore (repeatable).")
		fs.BoolVar(&force, "force", false, "Force run even if directory does not look like a code project.")
		fs.BoolVar(&showSummary, "show-summary", true, "Print a brief summary to stderr.")
		fs.Int64Var(&maxFileBytes, "max-file-bytes", 1_500_000, "Skip files larger than this many bytes during scanning.")
		fs.Int64Var(&maxAnalyzeBytes, "max-analyze-bytes", 200_000, "Analyze at most this many bytes of each file.")
		fs.IntVar(&treeDepth, "tree-depth", 6, "Limit directory tree depth.")
		fs.StringVar(&umlFormats, "uml", "ascii,plantuml", "UML formats: ascii,plantuml|ascii|plantuml|none.")
		fs.StringVar(&umlGranularity, "uml-granularity", "module", "UML granularity: file|module|package.")
		fs.IntVar(&umlMaxNodes, "uml-max-nodes", 120, "UML soft max nodes before collapsing externals.")
		fs.BoolVar(&umlIncludeIO, "uml-include-io", true, "Include IO nodes in UML.")
		fs.BoolVar(&umlIncludeEndpoints, "uml-include-endpoints", true, "Include endpoint nodes in UML.")
		fs.StringVar(&umlOut, "uml-out", "", "Write UML text to this file (optional).")

		fs.StringVar(&jsonOut, "json-out", "", "Write JSON report to this path (optional).")
		fs.StringVar(&promptOut, "prompt-out", "", "Write agent prompt text to this path (optional).")
		fs.Usage = func() { writeDocCreateHelp(stderr) }

		if err := fs.Parse(args); err != nil {
			writeDocCreateHelp(stderr)
			log.Error(fmt.Sprintf("failed to parse doc create flags: %v", err))
			return exitUsage
		}

		rootPath := "."
		if rest := fs.Args(); len(rest) >= 1 {
			rootPath = rest[0]
		}
		artifactRoot := computeArtifactDir(globalArtifactDir, rootPath)
		ignoreNames, ignoreGlobs := splitIgnores(ignores.values)

		res, err := ctr.Doc.Create(docapp.CreateRequest{
			RootPath:            rootPath,
			ArtifactDir:         artifactRoot,
			Force:               force,
			MaxFileBytes:        maxFileBytes,
			MaxAnalyzeBytes:     maxAnalyzeBytes,
			TreeDepth:           treeDepth,
			UMLFormats:          umlFormats,
			UMLGranularity:      umlGranularity,
			UMLMaxNodes:         umlMaxNodes,
			UMLIncludeIO:        umlIncludeIO,
			UMLIncludeEndpoints: umlIncludeEndpoints,
			IgnoreNames:         ignoreNames,
			IgnoreGlobs:         ignoreGlobs,
			NetEnabled:          pol.NetEnabled,
			AllowDomains:        pol.AllowDomains,
			Args:                nil,
		})
		if err != nil {
			log.Error(err.Error())
			return exitError
		}

		if _, err := io.WriteString(stdout, res.ReportXML+"\n"); err != nil {
			log.Error(fmt.Sprintf("failed writing to stdout: %v", err))
			return exitError
		}

		if jsonOut != "" {
			if err := os.WriteFile(jsonOut, []byte(res.ReportJSON+"\n"), 0o644); err != nil {
				log.Error(fmt.Sprintf("failed writing --json-out: %v", err))
				return exitError
			}
		}
		if promptOut != "" {
			if err := os.WriteFile(promptOut, []byte(res.PromptText+"\n"), 0o644); err != nil {
				log.Error(fmt.Sprintf("failed writing --prompt-out: %v", err))
				return exitError
			}
		}
		if umlOut != "" {
			var blob []string
			if strings.TrimSpace(res.Report.UMLASCII) != "" {
				blob = append(blob, "# UML ASCII")
				blob = append(blob, res.Report.UMLASCII)
				blob = append(blob, "")
			}
			if strings.TrimSpace(res.Report.UMLPlantUML) != "" {
				blob = append(blob, "# UML PlantUML")
				blob = append(blob, res.Report.UMLPlantUML)
			}
			if len(blob) == 0 {
				blob = append(blob, "# UML")
				blob = append(blob, "(no UML emitted; check --uml flag)")
			}
			if err := os.WriteFile(umlOut, []byte(strings.Join(blob, "\n")+"\n"), 0o644); err != nil {
				log.Error(fmt.Sprintf("failed writing --uml-out: %v", err))
				return exitError
			}
		}

		if showSummary {
			log.Info("mode: doc create")
			log.Info("root: " + rootPath)
			log.Info("artifact: " + res.ArtifactPath)
			log.Info("latest pointer: " + res.LatestPath)
			if len(res.Report.Languages) > 0 {
				log.Info("languages: " + strings.Join(res.Report.Languages, ", "))
			}
			log.Info("imports: " + itoa(len(res.Report.Imports)) + ", external deps: " + itoa(len(res.Report.ExternalDependencies)))
			if strings.TrimSpace(res.Report.UMLASCII) != "" {
				log.Info("uml: ascii included")
			}
			if strings.TrimSpace(res.Report.UMLPlantUML) != "" {
				log.Info("uml: plantuml included")
			}
			if len(ignoreNames) > 0 {
				log.Info("ignore names: " + strings.Join(ignoreNames, ", "))
			}
			if len(ignoreGlobs) > 0 {
				log.Info("ignore globs: " + strings.Join(ignoreGlobs, ", "))
			}
			if len(res.Report.Warnings) > 0 {
				log.Warn("warnings: " + itoa(len(res.Report.Warnings)) + " (see JSON/artifact for details)")
			}
		}

		return exitOK

	case "get":
		fs := flag.NewFlagSet("doc get", flag.ContinueOnError)
		fs.SetOutput(stderr)

		var jsonOut string
		var promptOut string
		var showSummary bool
		var crawlDepth int

		fs.IntVar(&crawlDepth, "crawl-depth", 1, "Crawl depth for HTTP fetch (default: 1 = fetch the URI only).")
		fs.StringVar(&jsonOut, "json-out", "", "Write JSON report to this path (optional).")
		fs.StringVar(&promptOut, "prompt-out", "", "Write agent prompt text to this path (optional).")
		fs.BoolVar(&showSummary, "show-summary", true, "Print a brief summary to stderr.")
		fs.Usage = func() { writeDocGetHelp(stderr) }

		if err := fs.Parse(args); err != nil {
			writeDocGetHelp(stderr)
			log.Error(fmt.Sprintf("failed to parse doc get flags: %v", err))
			return exitUsage
		}

		uris := fs.Args()
		if len(uris) == 0 {
			writeDocGetHelp(stderr)
			log.Error("doc get: at least one URI/path is required")
			return exitUsage
		}

		// In fetch mode (no repo root), default artifact dir is cwd unless --artifact-dir provided.
		artifactRoot := computeArtifactDir(globalArtifactDir, "")

		res, err := ctr.Doc.Get(docapp.GetRequest{
			URIs:         uris,
			ArtifactDir:  artifactRoot,
			CrawlDepth:   crawlDepth,
			NetEnabled:   pol.NetEnabled,
			AllowDomains: pol.AllowDomains,
			Args:         nil,
		})
		if err != nil {
			log.Error(err.Error())
			return exitError
		}

		if _, err := io.WriteString(stdout, res.ReportXML+"\n"); err != nil {
			log.Error(fmt.Sprintf("failed writing to stdout: %v", err))
			return exitError
		}

		if jsonOut != "" {
			if err := os.WriteFile(jsonOut, []byte(res.ReportJSON+"\n"), 0o644); err != nil {
				log.Error(fmt.Sprintf("failed writing --json-out: %v", err))
				return exitError
			}
		}
		if promptOut != "" {
			if err := os.WriteFile(promptOut, []byte(res.PromptText+"\n"), 0o644); err != nil {
				log.Error(fmt.Sprintf("failed writing --prompt-out: %v", err))
				return exitError
			}
		}

		if showSummary {
			log.Info("mode: doc get")
			log.Info("artifact: " + res.ArtifactPath)
			log.Info("latest pointer: " + res.LatestPath)
			log.Info("net enabled: " + boolString(pol.NetEnabled))
			if len(pol.AllowDomains) > 0 {
				log.Info("allow domains: " + strings.Join(pol.AllowDomains, ", "))
			}
			log.Info("crawl depth: " + itoa(crawlDepth))
			log.Info("docs fetched: " + itoa(len(res.Report.FetchedDocs)))
		}

		return exitOK

	default:
		log.Error("unknown doc subcommand: " + sub)
		writeDocHelp(stderr)
		return exitUsage
	}
}

func runDiagnose(ctr wiring.Container, pol policy.Policy, globalArtifactDir string, argv []string, stdout io.Writer, stderr io.Writer) int {
	log := console.New(stderr)

	fs := flag.NewFlagSet("diagnose", flag.ContinueOnError)
	fs.SetOutput(stderr)

	var jsonOut string
	var promptOut string
	var force bool
	var includeTests bool
	var timeoutSeconds int
	var showSummary bool
	var maxFileBytes int64
	var ignores stringListFlag

	fs.BoolVar(&force, "force", false, "Force run even if directory does not look like a code project.")
	fs.BoolVar(&includeTests, "include-tests", false, "Also compile/analyze tests for diagnostics without running them.")
	fs.IntVar(&timeoutSeconds, "timeout-seconds", 120, "Timeout in seconds per tool invocation.")
	fs.Int64Var(&maxFileBytes, "max-file-bytes", 1_500_000, "Skip files larger than this many bytes during scanning.")
	fs.BoolVar(&showSummary, "show-summary", true, "Print a brief summary to stderr.")
	fs.Var(&ignores, "ignore", "Directory names or glob paths to ignore (repeatable).")
	fs.Var(&ignores, "I", "Alias for --ignore (repeatable).")

	fs.StringVar(&jsonOut, "json-out", "", "Write JSON output to this file.")
	fs.StringVar(&promptOut, "prompt-out", "", "Write fix prompt text to this file.")

	fs.Usage = func() { writeDiagnoseHelp(stderr) }

	if err := fs.Parse(argv); err != nil {
		writeDiagnoseHelp(stderr)
		log.Error(fmt.Sprintf("failed to parse diagnose flags: %v", err))
		return exitUsage
	}

	rootPath := "."
	if rest := fs.Args(); len(rest) >= 1 {
		rootPath = rest[0]
	}

	artifactRoot := computeArtifactDir(globalArtifactDir, rootPath)
	ignoreNames, ignoreGlobs := splitIgnores(ignores.values)

	res, err := ctr.Diagnose.Diagnose(diagapp.DiagnoseRequest{
		RootPath:       rootPath,
		ArtifactDir:    artifactRoot,
		Force:          force,
		TimeoutSeconds: timeoutSeconds,
		IncludeTests:   includeTests,
		MaxFileBytes:   maxFileBytes,
		IgnoreNames:    ignoreNames,
		IgnoreGlobs:    ignoreGlobs,
		NetEnabled:     pol.NetEnabled,
		AllowDomains:   pol.AllowDomains,
		Args:           nil,
	})
	if err != nil {
		log.Error(err.Error())
		return exitError
	}

	if _, err := io.WriteString(stdout, res.ReportXML+"\n"); err != nil {
		log.Error(fmt.Sprintf("failed writing to stdout: %v", err))
		return exitError
	}

	if jsonOut != "" {
		if err := os.WriteFile(jsonOut, []byte(res.ReportJSON+"\n"), 0o644); err != nil {
			log.Error(fmt.Sprintf("failed writing --json-out: %v", err))
			return exitError
		}
	}
	if promptOut != "" {
		if err := os.WriteFile(promptOut, []byte(res.FixPrompt+"\n"), 0o644); err != nil {
			log.Error(fmt.Sprintf("failed writing --prompt-out: %v", err))
			return exitError
		}
	}

	if showSummary {
		log.Info("mode: diagnose")
		log.Info("root: " + rootPath)
		log.Info("artifact: " + res.ArtifactPath)
		log.Info("latest pointer: " + res.LatestPath)
		log.Info("include tests: " + boolString(includeTests))
		log.Info("timeout seconds: " + itoa(timeoutSeconds))
		if len(ignoreNames) > 0 {
			log.Info("ignore names: " + strings.Join(ignoreNames, ", "))
		}
		if len(ignoreGlobs) > 0 {
			log.Info("ignore globs: " + strings.Join(ignoreGlobs, ", "))
		}
		totalIssues := 0
		totalErrs := 0
		totalWarns := 0
		for _, ld := range res.Report.Languages {
			totalIssues += len(ld.Issues)
			for _, d := range ld.Issues {
				if string(d.Severity) == "error" {
					totalErrs++
				} else if string(d.Severity) == "warning" {
					totalWarns++
				}
			}
		}
		log.Info("issues: " + itoa(totalIssues) + " (errors: " + itoa(totalErrs) + ", warnings: " + itoa(totalWarns) + ")")
		if len(res.Report.Warnings) > 0 {
			log.Warn("warnings: " + itoa(len(res.Report.Warnings)) + " (see artifact for details)")
		}
	}

	return exitOK
}

func runTest(ctr wiring.Container, pol policy.Policy, globalArtifactDir string, argv []string, stdout io.Writer, stderr io.Writer) int {
	log := console.New(stderr)

	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	fs.SetOutput(stderr)

	var jsonOut string
	var promptOut string
	var force bool
	var showSummary bool

	var limitSubjects int
	var levels string
	var maxFileBytes int64
	var maxAnalyzeBytes int64

	var ignores stringListFlag

	var regressionSince string
	var regressionRange string
	var noRegression bool

	fs.BoolVar(&force, "force", false, "Force run even if directory does not look like a code project.")
	fs.BoolVar(&showSummary, "show-summary", true, "Print a brief summary to stderr.")
	fs.IntVar(&limitSubjects, "limit-subjects", 500, "Limit number of subjects analyzed (default: 500).")
	fs.StringVar(&levels, "levels", "", "Comma-separated levels: smoke,unit,integration,e2e,regression (default: all).")
	fs.Int64Var(&maxFileBytes, "max-file-bytes", 1_500_000, "Skip files larger than this many bytes during scanning.")
	fs.Int64Var(&maxAnalyzeBytes, "max-analyze-bytes", 200_000, "Analyze at most this many bytes of each file.")

	fs.StringVar(&regressionSince, "regression-since", "", "Enable regression suggestions by diffing against this git ref (e.g., origin/main, HEAD~1).")
	fs.StringVar(&regressionRange, "regression-range", "", "Enable regression suggestions by diffing this git range A..B (e.g., HEAD~3..HEAD).")
	fs.BoolVar(&noRegression, "no-regression", false, "Disable regression scenarios entirely.")

	fs.Var(&ignores, "ignore", "Directory names or glob paths to ignore (repeatable).")
	fs.Var(&ignores, "I", "Alias for --ignore (repeatable).")

	fs.StringVar(&jsonOut, "json-out", "", "Write JSON output to this file.")
	fs.StringVar(&promptOut, "prompt-out", "", "Write test prompt text to this file.")

	fs.Usage = func() { writeTestHelp(stderr) }

	if err := fs.Parse(argv); err != nil {
		writeTestHelp(stderr)
		log.Error(fmt.Sprintf("failed to parse test flags: %v", err))
		return exitUsage
	}

	rootPath := "."
	if rest := fs.Args(); len(rest) >= 1 {
		rootPath = rest[0]
	}

	artifactRoot := computeArtifactDir(globalArtifactDir, rootPath)
	ignoreNames, ignoreGlobs := splitIgnores(ignores.values)

	regMode := tpapp.RegressionMode{
		Disabled: noRegression,
		SinceRef: strings.TrimSpace(regressionSince),
		Range:    strings.TrimSpace(regressionRange),
	}
	// Range wins if both provided.
	if regMode.Range != "" {
		regMode.SinceRef = ""
	}

	res, err := ctr.TestPlan.Build(tpapp.BuildRequest{
		RootPath:        rootPath,
		ArtifactDir:     artifactRoot,
		Force:           force,
		LimitSubjects:   limitSubjects,
		LevelsCSV:       levels,
		MaxFileBytes:    maxFileBytes,
		MaxAnalyzeBytes: maxAnalyzeBytes,
		IgnoreNames:     ignoreNames,
		IgnoreGlobs:     ignoreGlobs,
		Regression:      regMode,
		NetEnabled:      pol.NetEnabled,
		AllowDomains:    pol.AllowDomains,
		Args:            nil,
	})
	if err != nil {
		log.Error(err.Error())
		return exitError
	}

	// Stdout: pseudo-XML test plan
	if _, err := io.WriteString(stdout, res.ReportXML+"\n"); err != nil {
		log.Error(fmt.Sprintf("failed writing to stdout: %v", err))
		return exitError
	}

	if jsonOut != "" {
		if err := os.WriteFile(jsonOut, []byte(res.ReportJSON+"\n"), 0o644); err != nil {
			log.Error(fmt.Sprintf("failed writing --json-out: %v", err))
			return exitError
		}
	}
	if promptOut != "" {
		if err := os.WriteFile(promptOut, []byte(res.TestPrompt+"\n"), 0o644); err != nil {
			log.Error(fmt.Sprintf("failed writing --prompt-out: %v", err))
			return exitError
		}
	}

	if showSummary {
		log.Info("mode: test")
		log.Info("root: " + rootPath)
		log.Info("artifact: " + res.ArtifactPath)
		log.Info("latest pointer: " + res.LatestPath)
		log.Info("subjects: " + itoa(res.Report.Summary.TotalSubjects) + ", scenarios: " + itoa(res.Report.Summary.TotalScenarios))
		if noRegression {
			log.Info("regression mode: disabled")
		} else if regMode.Range != "" {
			log.Info("regression mode: " + regMode.Range)
		} else if regMode.SinceRef != "" {
			log.Info("regression mode: since " + regMode.SinceRef)
		} else {
			log.Info("regression mode: off")
		}
		if len(ignoreNames) > 0 {
			log.Info("ignore names: " + strings.Join(ignoreNames, ", "))
		}
		if len(ignoreGlobs) > 0 {
			log.Info("ignore globs: " + strings.Join(ignoreGlobs, ", "))
		}
	}

	return exitOK
}

func computeArtifactDir(globalArtifactDir string, repoPath string) string {
	if strings.TrimSpace(globalArtifactDir) != "" {
		return globalArtifactDir
	}
	if strings.TrimSpace(repoPath) != "" {
		return repoPath
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return cwd
}

func splitIgnores(items []string) (ignoreNames []string, ignoreGlobs []string) {
	for _, raw := range items {
		s := strings.TrimSpace(raw)
		if s == "" {
			continue
		}
		if strings.Contains(s, "/") || strings.Contains(s, "*") || strings.Contains(s, "?") {
			ignoreGlobs = append(ignoreGlobs, s)
		} else {
			ignoreNames = append(ignoreNames, s)
		}
	}
	return ignoreNames, ignoreGlobs
}

func boolString(v bool) string {
	if v {
		return "true"
	}
	return "false"
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

func writeRootHelp(w io.Writer) {
	help := strings.TrimSpace(`
megamake — cross-platform orchestration platform (patch6)

Usage:
  megamake [global flags] <command> [args] [command flags]

Global flags:
  --artifact-dir <dir>
  --net
  --allow-domain <domain>

Commands:
  prompt [path]
  doc create [path]
  doc get <uri>...
  diagnose [path]
  test [path]
`)
	_, _ = io.WriteString(w, help+"\n")
}

func writePromptHelp(w io.Writer) {
	help := strings.TrimSpace(`
megamake prompt [path] [flags]
Use --help on the binary for the full help text.
`)
	_, _ = io.WriteString(w, help+"\n")
}

func writeDocHelp(w io.Writer) {
	help := strings.TrimSpace(`
megamake doc <subcommand>
Subcommands: create, get
`)
	_, _ = io.WriteString(w, help+"\n")
}

func writeDocCreateHelp(w io.Writer) {
	help := strings.TrimSpace(`
megamake doc create [path] [flags]
(see previous patch help text; options include --uml, --tree-depth, --ignore, etc.)
`)
	_, _ = io.WriteString(w, help+"\n")
}

func writeDocGetHelp(w io.Writer) {
	help := strings.TrimSpace(`
megamake doc get <uri>... [flags]

Flags:
  --crawl-depth N
  --json-out PATH
  --prompt-out PATH
  --show-summary=true|false

Network policy:
  - HTTP(S) requires --net
  - If --net and --allow-domain is provided, only those domains (and subdomains) are allowed
  - Local paths are always allowed
`)
	_, _ = io.WriteString(w, help+"\n")
}

func writeDiagnoseHelp(w io.Writer) {
	help := strings.TrimSpace(`
megamake diagnose [path] [flags]

Flags:
  --force
  --include-tests
  --timeout-seconds N
  --max-file-bytes N
  --ignore X / -I X
  --json-out PATH
  --prompt-out PATH
  --show-summary=true|false
`)
	_, _ = io.WriteString(w, help+"\n")
}

func writeTestHelp(w io.Writer) {
	help := strings.TrimSpace(`
megamake test [path] [flags]

Flags:
  --force
  --limit-subjects N
  --levels csv               (smoke,unit,integration,e2e,regression)
  --max-file-bytes N
  --max-analyze-bytes N
  --ignore X / -I X
  --regression-since REF
  --regression-range A..B
  --no-regression
  --json-out PATH
  --prompt-out PATH
  --show-summary=true|false
`)
	_, _ = io.WriteString(w, help+"\n")
}
