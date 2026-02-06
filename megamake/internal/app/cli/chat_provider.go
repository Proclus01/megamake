package cli

import (
	"encoding/json"
	"flag"
	"io"
	"strings"

	"github.com/megamake/megamake/internal/app/wiring"
	"github.com/megamake/megamake/internal/platform/console"
	"github.com/megamake/megamake/internal/platform/policy"

	chapp "github.com/megamake/megamake/internal/domains/chat/app"
)

// runChatVerify implements:
//
//	megamake chat verify [--provider openai|stub] [--env-file PATH]
func runChatVerify(ctr wiring.Container, pol policy.Policy, artifactDir string, argv []string, stdout io.Writer, stderr io.Writer) int {
	log := console.New(stderr)

	fs := flag.NewFlagSet("chat verify", flag.ContinueOnError)
	fs.SetOutput(stderr)

	var providerName string
	var envFile string
	var timeoutSeconds int
	var jsonOut bool

	fs.StringVar(&providerName, "provider", "", "Provider name (e.g., openai, stub). If empty, uses saved settings provider.")
	fs.StringVar(&envFile, "env-file", "", "Override dotenv path. Default: <artifactDir>/MEGACHAT/.env")
	fs.IntVar(&timeoutSeconds, "timeout-seconds", 20, "Timeout in seconds for provider verification.")
	fs.BoolVar(&jsonOut, "json", true, "Output JSON (default true).")

	fs.Usage = func() { writeChatVerifyHelp(stderr) }

	if err := fs.Parse(argv); err != nil {
		writeChatVerifyHelp(stderr)
		return exitUsage
	}

	// Ensure dotenv is loaded (best-effort) by calling ConfigGet (it loads env by design).
	cfg, err := ctr.Chat.ConfigGet(chapp.ConfigGetRequest{
		ArtifactDir: artifactDir,
		EnvFile:     envFile,
	})
	if err != nil {
		log.Error(err.Error())
		return exitError
	}

	prov := strings.TrimSpace(providerName)
	if prov == "" {
		prov = strings.TrimSpace(cfg.Settings.Provider)
	}
	if prov == "" {
		prov = "stub"
	}

	res, err := ctr.Chat.VerifyProvider(chapp.VerifyProviderRequest{
		Provider:       prov,
		TimeoutSeconds: timeoutSeconds,
		NetEnabled:     pol.NetEnabled,
		AllowDomains:   pol.AllowDomains,
	})
	if err != nil {
		if jsonOut {
			b, _ := json.MarshalIndent(res, "", "  ")
			_, _ = io.WriteString(stdout, string(b)+"\n")
		} else {
			_, _ = io.WriteString(stdout, "provider: "+res.Provider+"\n")
			_, _ = io.WriteString(stdout, "ok: "+boolString(res.OK)+"\n")
			if strings.TrimSpace(res.Message) != "" {
				_, _ = io.WriteString(stdout, "message: "+res.Message+"\n")
			}
		}
		log.Error(err.Error())
		return exitError
	}

	if jsonOut {
		b, _ := json.MarshalIndent(res, "", "  ")
		_, _ = io.WriteString(stdout, string(b)+"\n")
	} else {
		_, _ = io.WriteString(stdout, "provider: "+res.Provider+"\n")
		_, _ = io.WriteString(stdout, "ok: "+boolString(res.OK)+"\n")
		if strings.TrimSpace(res.Message) != "" {
			_, _ = io.WriteString(stdout, "message: "+res.Message+"\n")
		}
	}

	log.Info("mode: chat verify")
	log.Info("provider: " + res.Provider)
	return exitOK
}

// runChatModels implements:
//
//	megamake chat models [--provider openai|stub] [--limit N] [--env-file PATH] [--no-cache] [--cache-ttl-seconds N]
func runChatModels(ctr wiring.Container, pol policy.Policy, artifactDir string, argv []string, stdout io.Writer, stderr io.Writer) int {
	log := console.New(stderr)

	fs := flag.NewFlagSet("chat models", flag.ContinueOnError)
	fs.SetOutput(stderr)

	var providerName string
	var envFile string
	var limit int
	var timeoutSeconds int
	var jsonOut bool

	var noCache bool
	var cacheTTLSeconds int

	fs.StringVar(&providerName, "provider", "", "Provider name (e.g., openai, stub). If empty, uses saved settings provider.")
	fs.StringVar(&envFile, "env-file", "", "Override dotenv path. Default: <artifactDir>/MEGACHAT/.env")
	fs.IntVar(&limit, "limit", 200, "Max models to return.")
	fs.IntVar(&timeoutSeconds, "timeout-seconds", 25, "Timeout in seconds for provider model listing.")
	fs.BoolVar(&jsonOut, "json", true, "Output JSON (default true).")

	fs.BoolVar(&noCache, "no-cache", false, "Bypass server-side model cache (force refresh).")
	fs.IntVar(&cacheTTLSeconds, "cache-ttl-seconds", 300, "Model cache TTL in seconds (server-side).")

	fs.Usage = func() { writeChatModelsHelp(stderr) }

	if err := fs.Parse(argv); err != nil {
		writeChatModelsHelp(stderr)
		return exitUsage
	}

	// Load dotenv best-effort via ConfigGet.
	cfg, err := ctr.Chat.ConfigGet(chapp.ConfigGetRequest{
		ArtifactDir: artifactDir,
		EnvFile:     envFile,
	})
	if err != nil {
		log.Error(err.Error())
		return exitError
	}

	prov := strings.TrimSpace(providerName)
	if prov == "" {
		prov = strings.TrimSpace(cfg.Settings.Provider)
	}
	if prov == "" {
		prov = "stub"
	}

	res, err := ctr.Chat.ListModels(chapp.ListModelsRequest{
		Provider:        prov,
		Limit:           limit,
		TimeoutSeconds:  timeoutSeconds,
		NetEnabled:      pol.NetEnabled,
		AllowDomains:    pol.AllowDomains,
		CacheTTLSeconds: cacheTTLSeconds,
		NoCache:         noCache,
	})
	if err != nil {
		log.Error(err.Error())
		return exitError
	}

	if jsonOut {
		b, _ := json.MarshalIndent(res, "", "  ")
		_, _ = io.WriteString(stdout, string(b)+"\n")
	} else {
		for _, m := range res.Models {
			_, _ = io.WriteString(stdout, m.ID+"\n")
		}
	}

	log.Info("mode: chat models")
	log.Info("provider: " + res.Provider)
	log.Info("models: " + itoa(len(res.Models)))
	if res.Cached {
		log.Info("models cache: hit (age_s " + itoa(res.CacheAgeS) + ")")
	} else {
		log.Info("models cache: miss/refresh")
	}
	return exitOK
}

func writeChatVerifyHelp(w io.Writer) {
	help := strings.TrimSpace(`
megamake chat verify [flags]

Flags:
  --provider NAME
  --env-file PATH
  --timeout-seconds N
  --json=true|false

Network policy:
  - Providers that require HTTP(S) also require global --net
  - If global --allow-domain is set, provider hosts must be allowed

Notes:
  - Loads <artifactDir>/MEGACHAT/.env by default (or --env-file override) before verifying.
`)
	_, _ = io.WriteString(w, help+"\n")
}

func writeChatModelsHelp(w io.Writer) {
	help := strings.TrimSpace(`
megamake chat models [flags]

Flags:
  --provider NAME
  --env-file PATH
  --limit N
  --timeout-seconds N
  --json=true|false
  --no-cache
  --cache-ttl-seconds N

Network policy:
  - Providers that require HTTP(S) also require global --net
  - If global --allow-domain is set, provider hosts must be allowed

Notes:
  - Loads <artifactDir>/MEGACHAT/.env by default (or --env-file override) before listing models.
  - Cache is server-side when using a running server; in-process CLI calls also benefit while the process runs.
`)
	_, _ = io.WriteString(w, help+"\n")
}
