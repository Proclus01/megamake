package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/megamake/megamake/internal/app/wiring"
	"github.com/megamake/megamake/internal/platform/console"
	"github.com/megamake/megamake/internal/platform/policy"

	chatapp "github.com/megamake/megamake/internal/domains/chat/app"
	chathttp "github.com/megamake/megamake/internal/domains/chat/transport/httpserver"
)

func runChatServe(ctr wiring.Container, pol policy.Policy, artifactDir string, argv []string, stdout io.Writer, stderr io.Writer) int {
	log := console.New(stderr)

	fs := flag.NewFlagSet("chat serve", flag.ContinueOnError)
	fs.SetOutput(stderr)

	var listen string
	var envFile string

	fs.StringVar(&listen, "listen", "127.0.0.1:8082", "Listen address (host:port).")
	fs.StringVar(&envFile, "env-file", "", "Override dotenv path. Default: <artifactDir>/MEGACHAT/.env")

	fs.Usage = func() { writeChatServeHelp(stderr) }

	if err := fs.Parse(argv); err != nil {
		writeChatServeHelp(stderr)
		log.Error(fmt.Sprintf("failed to parse chat serve flags: %v", err))
		return exitUsage
	}

	if ctr.Chat == nil {
		log.Error("internal error: Chat API is nil")
		return exitError
	}

	// IMPORTANT: Load dotenv into the server process environment at startup (best-effort).
	// This ensures provider adapters that read os.Getenv(...) (like OpenAI) will work.
	_, err := ctr.Chat.ConfigGet(chatapp.ConfigGetRequest{
		ArtifactDir: artifactDir,
		EnvFile:     envFile,
	})
	if err != nil {
		log.Error("failed to load chat config/env: " + err.Error())
		return exitError
	}

	h := chathttp.Server{
		Chat:         ctr.Chat,
		ArtifactDir:  artifactDir,
		NetEnabled:   pol.NetEnabled,
		AllowDomains: pol.AllowDomains,
	}.Handler()

	srv := &http.Server{
		Addr:              listen,
		Handler:           h,
		ReadHeaderTimeout: 10 * time.Second,
	}

	log.Info("mode: chat serve")
	log.Info("artifact dir: " + artifactDir)
	if envFile != "" {
		log.Info("env file: " + envFile)
	} else {
		log.Info("env file: <artifactDir>/MEGACHAT/.env (default)")
	}
	log.Info("net enabled: " + boolString(pol.NetEnabled))
	if len(pol.AllowDomains) > 0 {
		log.Info("allow domains: " + stringsJoin(pol.AllowDomains, ", "))
	}
	log.Info("listening: http://" + listen)
	log.Info("ui: http://" + listen + "/ui")
	log.Info("health: http://" + listen + "/health")

	errCh := make(chan error, 1)
	go func() { errCh <- srv.ListenAndServe() }()

	sigCh := make(chan os.Signal, 2)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	select {
	case sig := <-sigCh:
		log.Warn("shutdown signal: " + sig.String())
	case err := <-errCh:
		if err != nil && err != http.ErrServerClosed {
			log.Error("server error: " + err.Error())
			return exitError
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Error("shutdown error: " + err.Error())
		return exitError
	}

	log.Info("server stopped")
	_, _ = io.WriteString(stdout, "ok\n")
	return exitOK
}

func writeChatServeHelp(w io.Writer) {
	help := `
megamake chat serve [flags]

Flags:
  --listen host:port    (default: 127.0.0.1:8082)
  --env-file PATH       Override dotenv path (default: <artifactDir>/MEGACHAT/.env)

Network policy (global flags):
  --net
  --allow-domain DOMAIN   (repeatable)

Notes:
  - Serves the chat API over HTTP.
  - Loads dotenv into the server process on startup so provider adapters can read API keys from env.
  - Artifacts are stored under: <artifactDir>/MEGACHAT/runs/<run_name>/
  - UI endpoint: /ui
`
	_, _ = io.WriteString(w, stringsTrimSpace(help)+"\n")
}

// tiny helpers (avoid importing strings in this file)
func stringsTrimSpace(s string) string {
	start := 0
	end := len(s)
	for start < end {
		c := s[start]
		if c != ' ' && c != '\n' && c != '\t' && c != '\r' {
			break
		}
		start++
	}
	for end > start {
		c := s[end-1]
		if c != ' ' && c != '\n' && c != '\t' && c != '\r' {
			break
		}
		end--
	}
	return s[start:end]
}

func stringsJoin(items []string, sep string) string {
	if len(items) == 0 {
		return ""
	}
	out := ""
	for i, it := range items {
		if i > 0 {
			out += sep
		}
		out += it
	}
	return out
}
