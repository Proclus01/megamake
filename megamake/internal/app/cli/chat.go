package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/megamake/megamake/internal/app/wiring"
	"github.com/megamake/megamake/internal/platform/console"
	"github.com/megamake/megamake/internal/platform/policy"

	contractchat "github.com/megamake/megamake/internal/contracts/v1/chat"
	chapp "github.com/megamake/megamake/internal/domains/chat/app"
)

func runChat(ctr wiring.Container, pol policy.Policy, globalArtifactDir string, argv []string, stdout io.Writer, stderr io.Writer) int {
	_ = pol // provider networking comes later; keep signature consistent with other commands.

	log := console.New(stderr)

	if len(argv) == 0 {
		writeChatHelp(stderr)
		return exitUsage
	}

	sub := argv[0]
	args := argv[1:]

	// Chat is not repo-root based; default artifact dir is cwd unless --artifact-dir provided.
	artifactRoot := computeArtifactDir(globalArtifactDir, "")

	switch sub {
	case "help", "-h", "--help":
		writeChatHelp(stdout)
		return exitOK

	case "serve":
		return runChatServe(ctr, pol, artifactRoot, args, stdout, stderr)

	case "run_async", "run-async":
		return runChatRunAsync(ctr, artifactRoot, args, stdout, stderr)

	case "jobs":
		return runChatJobs(ctr, artifactRoot, args, stdout, stderr)

	case "verify":
		return runChatVerify(ctr, pol, artifactRoot, args, stdout, stderr)
	case "models":
		return runChatModels(ctr, pol, artifactRoot, args, stdout, stderr)

	case "new":
		fs := flag.NewFlagSet("chat new", flag.ContinueOnError)
		fs.SetOutput(stderr)

		var title string
		var provider string
		var model string
		var systemText string
		var developerText string

		var envFile string
		var envOverwrite bool

		var jsonOut bool

		fs.StringVar(&title, "title", "Untitled Conversation", "Conversation title.")
		fs.StringVar(&provider, "provider", "", "Provider name (e.g., openai). If empty, uses saved settings/defaults.")
		fs.StringVar(&model, "model", "", "Model name (provider-specific). If empty, uses saved settings/defaults.")
		fs.StringVar(&systemText, "system", "", "System message text (Playground-style). Overrides saved settings if non-empty.")
		fs.StringVar(&developerText, "developer", "", "Developer message text (Playground-style). Overrides saved settings if non-empty.")
		fs.StringVar(&envFile, "env-file", "", "Override dotenv path. Default: <artifactDir>/MEGACHAT/.env")
		fs.BoolVar(&envOverwrite, "env-overwrite", false, "If set, dotenv variables overwrite existing environment variables.")
		fs.BoolVar(&jsonOut, "json", false, "Output JSON instead of plain run_name.")

		fs.Usage = func() { writeChatNewHelp(stderr) }

		if err := fs.Parse(args); err != nil {
			// flag package already writes errors; provide help.
			writeChatNewHelp(stderr)
			return exitUsage
		}

		req := chapp.NewRunRequest{
			ArtifactDir:   artifactRoot,
			Title:         title,
			Provider:      provider,
			Model:         model,
			SystemText:    systemText,
			DeveloperText: developerText,
			EnvFile:       envFile,
			EnvOverwrite:  envOverwrite,
		}

		res, err := ctr.Chat.NewRun(req)
		if err != nil {
			log.Error(err.Error())
			return exitError
		}

		if jsonOut {
			b, _ := json.MarshalIndent(res, "", "  ")
			_, _ = io.WriteString(stdout, string(b)+"\n")
		} else {
			// Script-friendly: print only run_name to stdout.
			_, _ = io.WriteString(stdout, res.RunName+"\n")
		}

		log.Info("mode: chat new")
		log.Info("artifact dir: " + artifactRoot)
		log.Info("run: " + res.RunName)
		log.Info("provider: " + emptyDash(res.Settings.Provider) + ", model: " + emptyDash(res.Settings.Model))
		return exitOK

	case "list":
		fs := flag.NewFlagSet("chat list", flag.ContinueOnError)
		fs.SetOutput(stderr)

		var limit int
		var jsonOut bool
		fs.IntVar(&limit, "limit", 200, "Limit number of runs returned.")
		fs.BoolVar(&jsonOut, "json", false, "Output JSON instead of human text.")

		fs.Usage = func() { writeChatListHelp(stderr) }

		if err := fs.Parse(args); err != nil {
			writeChatListHelp(stderr)
			return exitUsage
		}

		res, err := ctr.Chat.ListRuns(chapp.ListRunsRequest{
			ArtifactDir: artifactRoot,
			Limit:       limit,
		})
		if err != nil {
			log.Error(err.Error())
			return exitError
		}

		if jsonOut {
			b, _ := json.MarshalIndent(res, "", "  ")
			_, _ = io.WriteString(stdout, string(b)+"\n")
		} else {
			// Human output: tab-separated (run_name, title, model, updated)
			for _, m := range res.Items {
				_, _ = io.WriteString(stdout,
					m.RunName+"\t"+safeOneLine(m.Title)+"\t"+safeOneLine(m.Model)+"\t"+safeOneLine(m.UpdatedTS)+"\n")
			}
		}

		log.Info("mode: chat list")
		log.Info("artifact dir: " + artifactRoot)
		log.Info("runs: " + itoa(len(res.Items)))
		return exitOK

	case "get":
		fs := flag.NewFlagSet("chat get", flag.ContinueOnError)
		fs.SetOutput(stderr)

		var runName string
		var tail int
		var jsonOut bool

		fs.StringVar(&runName, "run", "", "Run name (required).")
		fs.IntVar(&tail, "tail", 200, "Transcript tail size (events).")
		fs.BoolVar(&jsonOut, "json", true, "Output JSON (default true).")

		fs.Usage = func() { writeChatGetHelp(stderr) }

		if err := fs.Parse(args); err != nil {
			writeChatGetHelp(stderr)
			return exitUsage
		}
		runName = strings.TrimSpace(runName)
		if runName == "" {
			writeChatGetHelp(stderr)
			log.Error("chat get: --run is required")
			return exitUsage
		}

		res, err := ctr.Chat.GetRun(chapp.GetRunRequest{
			ArtifactDir: artifactRoot,
			RunName:     runName,
			Tail:        tail,
		})
		if err != nil {
			log.Error(err.Error())
			return exitError
		}

		if jsonOut {
			b, _ := json.MarshalIndent(res, "", "  ")
			_, _ = io.WriteString(stdout, string(b)+"\n")
		} else {
			// Minimal human view
			_, _ = io.WriteString(stdout, "run: "+res.Meta.RunName+"\n")
			_, _ = io.WriteString(stdout, "title: "+res.Meta.Title+"\n")
			_, _ = io.WriteString(stdout, "provider/model: "+emptyDash(res.Meta.Provider)+"/"+emptyDash(res.Meta.Model)+"\n")
			_, _ = io.WriteString(stdout, "turns/messages: "+itoa(res.Meta.TurnsN)+"/"+itoa(res.Meta.MessagesN)+"\n")
			_, _ = io.WriteString(stdout, "updated: "+res.Meta.UpdatedTS+"\n")
			_, _ = io.WriteString(stdout, "\ntranscript tail:\n")
			for _, ev := range res.Events {
				_, _ = io.WriteString(stdout, fmt.Sprintf("- [%s] t=%d %s\n", ev.Role, ev.Turn, safeOneLine(ev.Text)))
			}
		}

		log.Info("mode: chat get")
		log.Info("artifact dir: " + artifactRoot)
		log.Info("run: " + runName)
		log.Info("events returned: " + itoa(len(res.Events)))
		return exitOK

	case "config":
		if len(args) == 0 {
			writeChatConfigHelp(stderr)
			return exitUsage
		}
		sub2 := args[0]
		args2 := args[1:]

		switch sub2 {
		case "get":
			fs := flag.NewFlagSet("chat config get", flag.ContinueOnError)
			fs.SetOutput(stderr)

			var envFile string
			var jsonOut bool

			fs.StringVar(&envFile, "env-file", "", "Override dotenv path. Default: <artifactDir>/MEGACHAT/.env")
			fs.BoolVar(&jsonOut, "json", true, "Output JSON (default true).")
			fs.Usage = func() { writeChatConfigGetHelp(stderr) }

			if err := fs.Parse(args2); err != nil {
				writeChatConfigGetHelp(stderr)
				return exitUsage
			}

			res, err := ctr.Chat.ConfigGet(chapp.ConfigGetRequest{
				ArtifactDir: artifactRoot,
				EnvFile:     envFile,
			})
			if err != nil {
				log.Error(err.Error())
				return exitError
			}

			if jsonOut {
				b, _ := json.MarshalIndent(res, "", "  ")
				_, _ = io.WriteString(stdout, string(b)+"\n")
			} else {
				_, _ = io.WriteString(stdout, "found: "+boolString(res.Found)+"\n")
				_, _ = io.WriteString(stdout, "provider: "+emptyDash(res.Settings.Provider)+"\n")
				_, _ = io.WriteString(stdout, "model: "+emptyDash(res.Settings.Model)+"\n")
			}

			log.Info("mode: chat config get")
			log.Info("artifact dir: " + artifactRoot)
			log.Info("found: " + boolString(res.Found))
			return exitOK

		case "set":
			fs := flag.NewFlagSet("chat config set", flag.ContinueOnError)
			fs.SetOutput(stderr)

			// Minimal set of settings fields for now (expand later).
			var provider string
			var model string
			var systemText string
			var developerText string

			var textFormat string
			var verbosity string
			var effort string
			var summaryAuto bool
			var maxOutputTokens int

			var toolWebSearch bool
			var toolCodeInterpreter bool
			var toolFileSearch bool
			var toolImageGeneration bool

			fs.StringVar(&provider, "provider", "", "Provider name (e.g., openai).")
			fs.StringVar(&model, "model", "", "Model name (provider-specific).")
			fs.StringVar(&systemText, "system", "", "Default system text.")
			fs.StringVar(&developerText, "developer", "", "Default developer text.")

			fs.StringVar(&textFormat, "text-format", "", "text|markdown|json (optional).")
			fs.StringVar(&verbosity, "verbosity", "", "low|medium|high (optional).")
			fs.StringVar(&effort, "effort", "", "minimal|low|medium|high (optional).")
			fs.BoolVar(&summaryAuto, "summary-auto", true, "If true, enable summary auto (best-effort).")
			fs.IntVar(&maxOutputTokens, "max-output-tokens", 999999, "Default max output tokens (best-effort).")

			fs.BoolVar(&toolWebSearch, "tool-web-search", false, "Enable web_search tool (best-effort).")
			fs.BoolVar(&toolCodeInterpreter, "tool-code-interpreter", false, "Enable code_interpreter tool (best-effort).")
			fs.BoolVar(&toolFileSearch, "tool-file-search", false, "Enable file_search tool (best-effort).")
			fs.BoolVar(&toolImageGeneration, "tool-image-generation", false, "Enable image_generation tool (best-effort).")

			fs.Usage = func() { writeChatConfigSetHelp(stderr) }

			if err := fs.Parse(args2); err != nil {
				writeChatConfigSetHelp(stderr)
				return exitUsage
			}

			// Start from existing settings if any, else defaults (via ConfigGet).
			got, err := ctr.Chat.ConfigGet(chapp.ConfigGetRequest{
				ArtifactDir: artifactRoot,
				EnvFile:     "",
			})
			if err != nil {
				log.Error(err.Error())
				return exitError
			}

			s := got.Settings
			if strings.TrimSpace(provider) != "" {
				s.Provider = strings.TrimSpace(provider)
			}
			if strings.TrimSpace(model) != "" {
				s.Model = strings.TrimSpace(model)
			}
			if systemText != "" {
				s.SystemText = systemText
			}
			if developerText != "" {
				s.DeveloperText = developerText
			}

			// Optional enums (only apply if set).
			if strings.TrimSpace(textFormat) != "" {
				switch strings.ToLower(strings.TrimSpace(textFormat)) {
				case "text":
					s.TextFormat = contractchat.TextFormatText
				case "markdown":
					s.TextFormat = contractchat.TextFormatMarkdown
				case "json":
					s.TextFormat = contractchat.TextFormatJSON
				default:
					log.Error("invalid --text-format (expected: text|markdown|json)")
					return exitUsage
				}
			}
			if strings.TrimSpace(verbosity) != "" {
				switch strings.ToLower(strings.TrimSpace(verbosity)) {
				case "low":
					s.Verbosity = contractchat.VerbosityLow
				case "medium":
					s.Verbosity = contractchat.VerbosityMedium
				case "high":
					s.Verbosity = contractchat.VerbosityHigh
				default:
					log.Error("invalid --verbosity (expected: low|medium|high)")
					return exitUsage
				}
			}
			if strings.TrimSpace(effort) != "" {
				switch strings.ToLower(strings.TrimSpace(effort)) {
				case "minimal":
					s.Effort = contractchat.EffortMinimal
				case "low":
					s.Effort = contractchat.EffortLow
				case "medium":
					s.Effort = contractchat.EffortMedium
				case "high":
					s.Effort = contractchat.EffortHigh
				default:
					log.Error("invalid --effort (expected: minimal|low|medium|high)")
					return exitUsage
				}
			}

			s.SummaryAuto = summaryAuto
			if maxOutputTokens > 0 {
				s.MaxOutputTokens = maxOutputTokens
			}

			s.Tools = contractchat.ToolsV1{
				WebSearch:       toolWebSearch,
				CodeInterpreter: toolCodeInterpreter,
				FileSearch:      toolFileSearch,
				ImageGeneration: toolImageGeneration,
			}

			if _, err := ctr.Chat.ConfigSet(chapp.ConfigSetRequest{
				ArtifactDir: artifactRoot,
				Settings:    s,
			}); err != nil {
				log.Error(err.Error())
				return exitError
			}

			_, _ = io.WriteString(stdout, "ok\n")
			log.Info("mode: chat config set")
			log.Info("artifact dir: " + artifactRoot)
			return exitOK

		default:
			log.Error("unknown chat config subcommand: " + sub2)
			writeChatConfigHelp(stderr)
			return exitUsage
		}

	default:
		log.Error("unknown chat subcommand: " + sub)
		writeChatHelp(stderr)
		return exitUsage
	}
}

func writeChatHelp(w io.Writer) {
	help := strings.TrimSpace(`
megamake chat <subcommand>

Subcommands:
  new
  serve
  list
  get
  config get
  config set
  run_async
  jobs
  verify
  models

Notes:
  - Chat artifacts are stored under: <artifactDir>/MEGACHAT/runs/<run_name>/
  - API keys are loaded from <artifactDir>/MEGACHAT/.env by default, or --env-file overrides (for commands that use it).
`)
	_, _ = io.WriteString(w, help+"\n")
}

func writeChatNewHelp(w io.Writer) {
	help := strings.TrimSpace(`
megamake chat new [flags]

Flags:
  --title STR
  --provider NAME
  --model NAME
  --system TEXT
  --developer TEXT
  --env-file PATH
  --env-overwrite
  --json
`)
	_, _ = io.WriteString(w, help+"\n")
}

func writeChatListHelp(w io.Writer) {
	help := strings.TrimSpace(`
megamake chat list [flags]

Flags:
  --limit N
  --json
`)
	_, _ = io.WriteString(w, help+"\n")
}

func writeChatGetHelp(w io.Writer) {
	help := strings.TrimSpace(`
megamake chat get --run <run_name> [flags]

Flags:
  --run NAME        (required)
  --tail N
  --json=true|false
`)
	_, _ = io.WriteString(w, help+"\n")
}

func writeChatConfigHelp(w io.Writer) {
	help := strings.TrimSpace(`
megamake chat config <subcommand>

Subcommands:
  get
  set
`)
	_, _ = io.WriteString(w, help+"\n")
}

func writeChatConfigGetHelp(w io.Writer) {
	help := strings.TrimSpace(`
megamake chat config get [flags]

Flags:
  --env-file PATH
  --json=true|false
`)
	_, _ = io.WriteString(w, help+"\n")
}

func writeChatConfigSetHelp(w io.Writer) {
	help := strings.TrimSpace(`
megamake chat config set [flags]

Flags:
  --provider NAME
  --model NAME
  --system TEXT
  --developer TEXT
  --text-format text|markdown|json
  --verbosity low|medium|high
  --effort minimal|low|medium|high
  --summary-auto=true|false
  --max-output-tokens N
  --tool-web-search
  --tool-code-interpreter
  --tool-file-search
  --tool-image-generation
`)
	_, _ = io.WriteString(w, help+"\n")
}

func safeOneLine(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	// keep it reasonably short
	if len(s) > 140 {
		return s[:140] + "…"
	}
	return s
}

func emptyDash(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "—"
	}
	return s
}
