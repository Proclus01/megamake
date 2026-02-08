package cli

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
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
		if err == flag.ErrHelp {
			// The flag package already triggered Usage output.
			// Treat help as a successful exit.
			return exitOK
		}

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
	case "chat":
		return runChat(ctr, pol, artifactDir, args, stdout, stderr)
	default:
		log.Error("unknown command: " + cmd)
		writeRootHelp(stderr)
		return exitUsage
	}
}

func runPrompt(ctr wiring.Container, pol policy.Policy, globalArtifactDir string, argv []string, stdout io.Writer, stderr io.Writer) int {
	log := console.New(stderr)

	leadingPos, flagArgs := splitLeadingPositionals(argv)

	fs := flag.NewFlagSet("prompt", flag.ContinueOnError)
	fs.SetOutput(stderr)

	var jsonOut string
	var promptOut string
	var force bool
	var maxFileBytes int64
	var copyToClipboard bool
	var showSummary bool

	var ignores stringListFlag
	fs.Var(&ignores, "ignore", "Directory names or glob paths to ignore (repeatable). Use quotes in zsh: --ignore 'megamake/artifacts/**'")
	fs.Var(&ignores, "I", "Alias for --ignore (repeatable).")
	fs.StringVar(&jsonOut, "json-out", "", "Write JSON report to this path (optional).")
	fs.StringVar(&promptOut, "prompt-out", "", "Write agent prompt text to this path (optional).")
	fs.BoolVar(&force, "force", false, "Force run even if the directory does not look like a code project.")
	fs.Int64Var(&maxFileBytes, "max-file-bytes", 1_500_000, "Skip files larger than this many bytes during scanning.")
	fs.BoolVar(&copyToClipboard, "copy", false, "Best-effort: also copy the generated <context> blob to clipboard.")
	fs.BoolVar(&showSummary, "show-summary", true, "Print a brief summary to stderr.")
	fs.Usage = func() { writePromptHelp(stderr) }

	argsToParse := argv
	if len(leadingPos) > 0 {
		argsToParse = flagArgs
	}
	if err := fs.Parse(argsToParse); err != nil {
		writePromptHelp(stderr)
		log.Error(fmt.Sprintf("failed to parse prompt flags: %v", err))
		return exitUsage
	}

	rootPath := "."
	if len(leadingPos) > 0 {
		if len(leadingPos) != 1 {
			log.Error("prompt: expected at most one positional path")
			writePromptHelp(stderr)
			return exitUsage
		}
		rootPath = leadingPos[0]
	} else {
		rest := fs.Args()
		if len(rest) >= 1 {
			rootPath = rest[0]
			rest = rest[1:]
		}
		if len(rest) > 0 {
			log.Error("prompt: unexpected extra arguments: " + strings.Join(rest, " "))
			writePromptHelp(stderr)
			return exitUsage
		}
	}

	// Resolve root relative to invocation directory (supports cd’ing aliases via MEGAMAKE_CALLER_PWD).
	rootPath = resolveRootPathFromInvocation(rootPath)

	artifactRoot := artifactDirForLocalTools(globalArtifactDir, log)

	ignoreNames, ignoreGlobs := splitIgnores(ignores.values)
	ignoreGlobs = append(ignoreGlobs, defaultLocalArtifactsIgnoreGlobs(rootPath)...)
	ignoreNames = dedupeStrings(ignoreNames)
	ignoreGlobs = dedupeStrings(ignoreGlobs)

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
		log.Info("invocation cwd: " + invocationCWD())
		if cwd, err := os.Getwd(); err == nil {
			log.Info("process cwd: " + cwd)
		}
		log.Info("root: " + rootPath)
		log.Info("artifact dir: " + artifactRoot)
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

		fs.Var(&ignores, "ignore", "Directory names or glob paths to ignore (repeatable). Use quotes in zsh: --ignore 'megamake/artifacts/**'")
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

		leadingPos, flagArgs := splitLeadingPositionals(args)

		argsToParse := args
		if len(leadingPos) > 0 {
			argsToParse = flagArgs
		}
		if err := fs.Parse(argsToParse); err != nil {
			writeDocCreateHelp(stderr)
			log.Error(fmt.Sprintf("failed to parse doc create flags: %v", err))
			return exitUsage
		}

		rootPath := "."
		if len(leadingPos) > 0 {
			if len(leadingPos) != 1 {
				log.Error("doc create: expected at most one positional path")
				writeDocCreateHelp(stderr)
				return exitUsage
			}
			rootPath = leadingPos[0]
		} else {
			if rest := fs.Args(); len(rest) >= 1 {
				rootPath = rest[0]
				rest = rest[1:]
				if len(rest) > 0 {
					log.Error("doc create: unexpected extra arguments: " + strings.Join(rest, " "))
					writeDocCreateHelp(stderr)
					return exitUsage
				}
			}
		}

		artifactRoot := artifactDirForLocalTools(globalArtifactDir, log)

		ignoreNames, ignoreGlobs := splitIgnores(ignores.values)
		ignoreGlobs = append(ignoreGlobs, defaultLocalArtifactsIgnoreGlobs(rootPath)...)
		ignoreNames = dedupeStrings(ignoreNames)
		ignoreGlobs = dedupeStrings(ignoreGlobs)

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

		leadingURIs, flagArgs := splitLeadingPositionals(args)

		// If URIs-first, parse flags from the tail; else parse from all args.
		argsToParse := args
		if len(leadingURIs) > 0 && len(flagArgs) > 0 {
			argsToParse = flagArgs
		}

		if err := fs.Parse(argsToParse); err != nil {
			writeDocGetHelp(stderr)
			log.Error(fmt.Sprintf("failed to parse doc get flags: %v", err))
			return exitUsage
		}

		uris := []string{}
		uris = append(uris, leadingURIs...)
		uris = append(uris, fs.Args()...) // supports URIs after flags too

		// Clean URIs
		var cleaned []string
		for _, u := range uris {
			u = strings.TrimSpace(u)
			if u != "" {
				cleaned = append(cleaned, u)
			}
		}
		uris = cleaned

		if len(uris) == 0 {
			writeDocGetHelp(stderr)
			log.Error("doc get: at least one URI/path is required")
			return exitUsage
		}

		artifactRoot := artifactDirForLocalTools(globalArtifactDir, log)

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

	leadingPos, flagArgs := splitLeadingPositionals(argv)

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
	fs.Var(&ignores, "ignore", "Directory names or glob paths to ignore (repeatable). Use quotes in zsh: --ignore 'megamake/artifacts/**'")
	fs.Var(&ignores, "I", "Alias for --ignore (repeatable).")

	fs.StringVar(&jsonOut, "json-out", "", "Write JSON output to this file.")
	fs.StringVar(&promptOut, "prompt-out", "", "Write fix prompt text to this file.")

	fs.Usage = func() { writeDiagnoseHelp(stderr) }

	argsToParse := argv
	if len(leadingPos) > 0 {
		argsToParse = flagArgs
	}
	if err := fs.Parse(argsToParse); err != nil {
		writeDiagnoseHelp(stderr)
		log.Error(fmt.Sprintf("failed to parse diagnose flags: %v", err))
		return exitUsage
	}

	rootPath := "."
	if len(leadingPos) > 0 {
		if len(leadingPos) != 1 {
			log.Error("diagnose: expected at most one positional path")
			writeDiagnoseHelp(stderr)
			return exitUsage
		}
		rootPath = leadingPos[0]
	} else {
		rest := fs.Args()
		if len(rest) >= 1 {
			rootPath = rest[0]
			rest = rest[1:]
		}
		if len(rest) > 0 {
			log.Error("diagnose: unexpected extra arguments: " + strings.Join(rest, " "))
			writeDiagnoseHelp(stderr)
			return exitUsage
		}
	}

	artifactRoot := artifactDirForLocalTools(globalArtifactDir, log)
	ignoreNames, ignoreGlobs := splitIgnores(ignores.values)
	ignoreGlobs = append(ignoreGlobs, defaultLocalArtifactsIgnoreGlobs(rootPath)...)
	ignoreNames = dedupeStrings(ignoreNames)
	ignoreGlobs = dedupeStrings(ignoreGlobs)

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

	leadingPos, flagArgs := splitLeadingPositionals(argv)

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

	fs.Var(&ignores, "ignore", "Directory names or glob paths to ignore (repeatable). Use quotes in zsh: --ignore 'megamake/artifacts/**'")
	fs.Var(&ignores, "I", "Alias for --ignore (repeatable).")

	fs.StringVar(&jsonOut, "json-out", "", "Write JSON output to this file.")
	fs.StringVar(&promptOut, "prompt-out", "", "Write test prompt text to this file.")

	fs.Usage = func() { writeTestHelp(stderr) }

	argsToParse := argv
	if len(leadingPos) > 0 {
		argsToParse = flagArgs
	}
	if err := fs.Parse(argsToParse); err != nil {
		writeTestHelp(stderr)
		log.Error(fmt.Sprintf("failed to parse test flags: %v", err))
		return exitUsage
	}

	rootPath := "."
	if len(leadingPos) > 0 {
		if len(leadingPos) != 1 {
			log.Error("test: expected at most one positional path")
			writeTestHelp(stderr)
			return exitUsage
		}
		rootPath = leadingPos[0]
	} else {
		rest := fs.Args()
		if len(rest) >= 1 {
			rootPath = rest[0]
			rest = rest[1:]
		}
		if len(rest) > 0 {
			log.Error("test: unexpected extra arguments: " + strings.Join(rest, " "))
			writeTestHelp(stderr)
			return exitUsage
		}
	}

	artifactRoot := artifactDirForLocalTools(globalArtifactDir, log)

	ignoreNames, ignoreGlobs := splitIgnores(ignores.values)
	ignoreGlobs = append(ignoreGlobs, defaultLocalArtifactsIgnoreGlobs(rootPath)...)
	ignoreNames = dedupeStrings(ignoreNames)
	ignoreGlobs = dedupeStrings(ignoreGlobs)

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
		log.Info("artifact dir: " + artifactRoot)
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
	_ = repoPath // intentionally unused; artifacts should default to invocation CWD

	if strings.TrimSpace(globalArtifactDir) != "" {
		return globalArtifactDir
	}

	cwd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return cwd
}

func splitIgnores(items []string) (ignoreNames []string, ignoreGlobs []string) {
	normalize := func(s string) string {
		s = strings.TrimSpace(s)
		if s == "" {
			return ""
		}
		s = strings.ReplaceAll(s, "\\", "/")
		for strings.HasPrefix(s, "./") {
			s = strings.TrimPrefix(s, "./")
		}
		s = strings.TrimPrefix(s, "/")
		for strings.HasSuffix(s, "/") {
			s = strings.TrimSuffix(s, "/")
		}
		return strings.TrimSpace(s)
	}

	for _, raw := range items {
		s := normalize(raw)
		if s == "" {
			continue
		}

		if strings.ContainsAny(s, "*?") {
			ignoreGlobs = append(ignoreGlobs, s)
			continue
		}
		if strings.Contains(s, "/") {
			ignoreGlobs = append(ignoreGlobs, s)
			continue
		}
		ignoreNames = append(ignoreNames, s)
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

func isFlagToken(s string) bool {
	s = strings.TrimSpace(s)
	return strings.HasPrefix(s, "-") && s != "-" && s != "--"
}

func invocationCWD() string {
	// If a wrapper/alias changes directories before running megamake,
	// it can preserve the "real" working directory by setting this env var.
	if v := strings.TrimSpace(os.Getenv("MEGAMAKE_CALLER_PWD")); v != "" {
		// Prefer absolute for predictable artifact output paths.
		if abs, err := filepath.Abs(v); err == nil {
			return abs
		}
		return v
	}

	if cwd, err := os.Getwd(); err == nil && strings.TrimSpace(cwd) != "" {
		return cwd
	}
	return "."
}

func resolveRootPathFromInvocation(rootPath string) string {
	rootPath = strings.TrimSpace(rootPath)
	if rootPath == "" {
		return invocationCWD()
	}

	// If absolute already, keep it.
	if filepath.IsAbs(rootPath) {
		return rootPath
	}

	// Otherwise interpret relative paths against the invocation CWD (not process CWD).
	base := invocationCWD()
	return filepath.Clean(filepath.Join(base, rootPath))
}

func artifactDirForLocalTools(globalArtifactDir string, log console.Logger) string {
	// Your requirement: prompt/doc/diagnose/test artifacts should be written to the directory
	// where the command was invoked.
	//
	// So: ALWAYS use invocationCWD().
	//
	// We ignore global --artifact-dir for these tools (but warn to reduce confusion).
	if strings.TrimSpace(globalArtifactDir) != "" {
		log.Warn("note: global --artifact-dir is ignored for this command; writing tool artifacts to invocation directory instead")
	}
	return invocationCWD()
}

// splitLeadingPositionals splits argv into:
// - positionals: all leading tokens until the first flag token
// - flagArgs: remaining tokens (starting at first flag token)
func splitLeadingPositionals(argv []string) (positionals []string, flagArgs []string) {
	for i := 0; i < len(argv); i++ {
		if argv[i] == "--" {
			// Everything after -- is positional
			positionals = append(positionals, argv[i+1:]...)
			return positionals, nil
		}
		if isFlagToken(argv[i]) {
			return positionals, argv[i:]
		}
		positionals = append(positionals, argv[i])
	}
	return positionals, nil
}

// defaultLocalArtifactsIgnoreGlobs returns ignore patterns for the canonical local artifacts dirs,
// but ONLY if those dirs exist under rootPath.
//
// Covers your canonical layout:
// - repo root:      megamake/artifacts
// - inside megamake: artifacts
func defaultLocalArtifactsIgnoreGlobs(rootPath string) []string {
	if strings.TrimSpace(rootPath) == "" {
		rootPath = "."
	}

	candidates := []string{
		"megamake/artifacts",
		"artifacts",
	}

	var out []string
	for _, rel := range candidates {
		p := filepath.Join(rootPath, filepath.FromSlash(rel))
		st, err := os.Stat(p)
		if err != nil || st == nil || !st.IsDir() {
			continue
		}
		out = append(out, rel)
		out = append(out, rel+"/**")
	}
	return dedupeStrings(out)
}

func dedupeStrings(xs []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, x := range xs {
		x = strings.TrimSpace(x)
		if x == "" || seen[x] {
			continue
		}
		seen[x] = true
		out = append(out, x)
	}
	return out
}

func writeRootHelp(w io.Writer) {
	help := strings.TrimSpace(`
megamake — cross-platform orchestration platform (patch6)

Usage:
  megamake [global flags] <command> [args] [command flags]

Global flags:
  --artifact-dir <dir>     Directory where MEGA* artifacts and *_latest pointer files are written.
                           Default: current working directory (where you run the command).
  --net                    Enable network access (deny-by-default otherwise).
  --allow-domain <domain>  Allowed domain when --net is set (repeatable). If none provided, all domains allowed when --net is set.

Commands:
  prompt   [path] [flags]   (also accepts flags after path)
  doc      <subcommand>
  diagnose [path] [flags]   (also accepts flags after path)
  test     [path] [flags]   (also accepts flags after path)
  chat     <subcommand>

Notes:
  - prompt/doc/diagnose/test automatically ignore local artifacts directories (if present):
      - megamake/artifacts/**
      - artifacts/**
  - If you use zsh and pass glob patterns to --ignore, quote them:
      --ignore 'megamake/artifacts/**'
`)
	_, _ = io.WriteString(w, help+"\n")
}

func writePromptHelp(w io.Writer) {
	help := strings.TrimSpace(`
megamake prompt [path] [flags]
megamake prompt [flags] [path]

Generates a <context> blob from the codebase.

Flags:
  --ignore X / -I X           Ignore directory name OR path/glob (repeatable).
                              Examples:
                                --ignore artifacts
                                --ignore megamake/artifacts
                                --ignore 'docs/generated/**'
                              zsh note: quote globs or zsh may expand/raise "no matches found".
  --max-file-bytes N          Skip files larger than N bytes (default: 1500000).
  --force                     Run even if directory does not look like a code project.
  --copy                      Best-effort: copy the generated <context> to clipboard.
  --json-out PATH             Write JSON report to PATH (optional).
  --prompt-out PATH           Write agent prompt text to PATH (optional).
  --show-summary=true|false   Print a brief summary to stderr (default: true).

Defaults:
  - Artifact output directory: current working directory
    (override with global --artifact-dir).
  - Automatically ignores local artifacts directories if present:
      - megamake/artifacts/**
      - artifacts/**
`)
	_, _ = io.WriteString(w, help+"\n")
}

func writeDocHelp(w io.Writer) {
	help := strings.TrimSpace(`
megamake doc <subcommand>

Subcommands:
  create   Create documentation report from a local repo directory.
  get      Fetch and summarize docs from URIs/paths (HTTP(S) requires --net).

Notes:
  - doc create accepts flags after the path.
  - doc get accepts flags before/after URIs (see doc get --help).
`)
	_, _ = io.WriteString(w, help+"\n")
}

func writeDocCreateHelp(w io.Writer) {
	help := strings.TrimSpace(`
megamake doc create [path] [flags]
megamake doc create [flags] [path]

Generates a documentation report (tree, import graph, purpose, optional UML).

Common flags:
  --ignore X / -I X           Ignore directory name OR path/glob (repeatable).
                              Examples:
                                --ignore artifacts
                                --ignore megamake/artifacts
                                --ignore 'docs/generated/**'
                              zsh note: quote globs or zsh may expand/raise "no matches found".
  --force
  --show-summary=true|false
  --max-file-bytes N
  --max-analyze-bytes N
  --tree-depth N

UML flags:
  --uml ascii,plantuml|ascii|plantuml|none
  --uml-granularity file|module|package
  --uml-max-nodes N
  --uml-include-io=true|false
  --uml-include-endpoints=true|false
  --uml-out PATH

Output flags:
  --json-out PATH
  --prompt-out PATH

Defaults:
  - Artifact output directory: current working directory
    (override with global --artifact-dir).
  - Automatically ignores local artifacts directories if present:
      - megamake/artifacts/**
      - artifacts/**
`)
	_, _ = io.WriteString(w, help+"\n")
}

func writeDocGetHelp(w io.Writer) {
	help := strings.TrimSpace(`
megamake doc get <uri>... [flags]
megamake doc get [flags] <uri>...

Fetches and summarizes docs from one or more URIs/paths.

Flags:
  --crawl-depth N             Crawl depth for HTTP fetch (default: 1).
  --json-out PATH             Write JSON report to PATH (optional).
  --prompt-out PATH           Write agent prompt text to PATH (optional).
  --show-summary=true|false   Print a brief summary to stderr (default: true).

Flag ordering:
  - Flags may appear before OR after URIs.
    Examples:
      megamake doc get https://example.com --crawl-depth 2
      megamake doc get --crawl-depth 2 https://example.com
  - Use "--" to force the rest to be treated as positional URIs:
      megamake doc get --crawl-depth 2 -- https://example.com?x=--weird--

Network policy:
  - HTTP(S) requires global --net
  - If --net and --allow-domain is provided, only those domains (and subdomains) are allowed
  - Local paths are always allowed

Defaults:
  - Artifact output directory: current working directory
    (override with global --artifact-dir).
`)
	_, _ = io.WriteString(w, help+"\n")
}

func writeDiagnoseHelp(w io.Writer) {
	help := strings.TrimSpace(`
megamake diagnose [path] [flags]
megamake diagnose [flags] [path]

Runs multi-language diagnostics (best-effort) and emits a report + fix prompt.

Flags:
  --force
  --include-tests
  --timeout-seconds N
  --max-file-bytes N
  --ignore X / -I X           Ignore directory name OR path/glob (repeatable).
                              Examples:
                                --ignore artifacts
                                --ignore megamake/artifacts
                                --ignore 'docs/generated/**'
                              zsh note: quote globs or zsh may expand/raise "no matches found".
  --json-out PATH
  --prompt-out PATH
  --show-summary=true|false

Defaults:
  - Artifact output directory: current working directory
    (override with global --artifact-dir).
  - Automatically ignores local artifacts directories if present:
      - megamake/artifacts/**
      - artifacts/**
`)
	_, _ = io.WriteString(w, help+"\n")
}

func writeTestHelp(w io.Writer) {
	help := strings.TrimSpace(`
megamake test [path] [flags]
megamake test [flags] [path]

Builds a test plan (subjects + scenarios) for the repo.

Flags:
  --force
  --limit-subjects N
  --levels csv               (smoke,unit,integration,e2e,regression) (default: all)
  --max-file-bytes N
  --max-analyze-bytes N
  --ignore X / -I X           Ignore directory name OR path/glob (repeatable).
                              Examples:
                                --ignore artifacts
                                --ignore megamake/artifacts
                                --ignore 'docs/generated/**'
                              zsh note: quote globs or zsh may expand/raise "no matches found".
  --regression-since REF
  --regression-range A..B
  --no-regression
  --json-out PATH
  --prompt-out PATH
  --show-summary=true|false

Defaults:
  - Artifact output directory: current working directory
    (override with global --artifact-dir).
  - Automatically ignores local artifacts directories if present:
      - megamake/artifacts/**
      - artifacts/**
`)
	_, _ = io.WriteString(w, help+"\n")
}
