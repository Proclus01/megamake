package httpserver

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	contractchat "github.com/megamake/megamake/internal/contracts/v1/chat"
	chatapi "github.com/megamake/megamake/internal/domains/chat/api"
	chatapp "github.com/megamake/megamake/internal/domains/chat/app"
)

// Server is the HTTP transport adapter for the chat domain.
type Server struct {
	Chat        chatapi.API
	ArtifactDir string

	NetEnabled   bool
	AllowDomains []string
}

func (s Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":            true,
			"time":          time.Now().UTC().Format(time.RFC3339Nano),
			"net_enabled":   s.NetEnabled,
			"allow_domains": s.AllowDomains,
		})
	})

	// Runs
	mux.HandleFunc("POST /api/chat/new", s.handleNew)
	mux.HandleFunc("GET /api/chat/list", s.handleList)
	mux.HandleFunc("GET /api/chat/get", s.handleGet)

	// Per-run settings
	mux.HandleFunc("GET /api/chat/run/settings", s.handleRunSettingsGet)
	mux.HandleFunc("POST /api/chat/run/settings", s.handleRunSettingsSet)

	// Jobs
	mux.HandleFunc("POST /api/chat/run_async", s.handleRunAsync)
	mux.HandleFunc("GET /api/chat/jobs/status", s.handleJobStatus)
	mux.HandleFunc("GET /api/chat/jobs/tail", s.handleJobTail)
	mux.HandleFunc("POST /api/chat/jobs/cancel", s.handleJobCancel)

	// Providers
	mux.HandleFunc("GET /api/chat/providers/list", s.handleProvidersList)
	mux.HandleFunc("POST /api/chat/providers/verify", s.handleProviderVerify)
	mux.HandleFunc("GET /api/chat/providers/models", s.handleProviderModels)

	registerUI(mux)
	return mux
}

func (s Server) handleNew(w http.ResponseWriter, r *http.Request) {
	if s.Chat == nil {
		writeJSON(w, 500, map[string]any{"ok": false, "error": "server misconfigured: Chat API is nil"})
		return
	}
	type reqBody struct {
		Title         string `json:"title"`
		Provider      string `json:"provider,omitempty"`
		Model         string `json:"model,omitempty"`
		SystemText    string `json:"systemText,omitempty"`
		DeveloperText string `json:"developerText,omitempty"`
	}
	var body reqBody
	if err := readJSON(r, &body); err != nil {
		writeJSON(w, 400, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	res, err := s.Chat.NewRun(chatapp.NewRunRequest{
		ArtifactDir:   s.ArtifactDir,
		Title:         body.Title,
		Provider:      body.Provider,
		Model:         body.Model,
		SystemText:    body.SystemText,
		DeveloperText: body.DeveloperText,
		EnvFile:       "",
		EnvOverwrite:  false,
	})
	if err != nil {
		writeJSON(w, 500, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	writeJSON(w, 200, map[string]any{"ok": true, "run_name": res.RunName, "meta": res.Meta})
}

func (s Server) handleList(w http.ResponseWriter, r *http.Request) {
	if s.Chat == nil {
		writeJSON(w, 500, map[string]any{"ok": false, "error": "server misconfigured: Chat API is nil"})
		return
	}
	limit := intFromQuery(r, "limit", 200)
	if limit <= 0 {
		limit = 200
	}
	if limit > 2000 {
		limit = 2000
	}

	res, err := s.Chat.ListRuns(chatapp.ListRunsRequest{ArtifactDir: s.ArtifactDir, Limit: limit})
	if err != nil {
		writeJSON(w, 500, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "items": res.Items})
}

func (s Server) handleGet(w http.ResponseWriter, r *http.Request) {
	if s.Chat == nil {
		writeJSON(w, 500, map[string]any{"ok": false, "error": "server misconfigured: Chat API is nil"})
		return
	}
	runName := strings.TrimSpace(r.URL.Query().Get("run_name"))
	if runName == "" {
		writeJSON(w, 400, map[string]any{"ok": false, "error": "missing run_name"})
		return
	}
	tail := intFromQuery(r, "tail", 500)
	if tail <= 0 {
		tail = 200
	}
	if tail > 5000 {
		tail = 5000
	}

	res, err := s.Chat.GetRun(chatapp.GetRunRequest{ArtifactDir: s.ArtifactDir, RunName: runName, Tail: tail})
	if err != nil {
		writeJSON(w, 500, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "meta": res.Meta, "events": res.Events})
}

func (s Server) handleRunSettingsGet(w http.ResponseWriter, r *http.Request) {
	if s.Chat == nil {
		writeJSON(w, 500, map[string]any{"ok": false, "error": "server misconfigured: Chat API is nil"})
		return
	}
	runName := strings.TrimSpace(r.URL.Query().Get("run_name"))
	if runName == "" {
		writeJSON(w, 400, map[string]any{"ok": false, "error": "missing run_name"})
		return
	}

	res, err := s.Chat.GetRunSettings(chatapp.GetRunSettingsRequest{
		ArtifactDir: s.ArtifactDir,
		RunName:     runName,
	})
	if err != nil {
		writeJSON(w, 500, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	writeJSON(w, 200, map[string]any{"ok": true, "result": res})
}

func (s Server) handleRunSettingsSet(w http.ResponseWriter, r *http.Request) {
	if s.Chat == nil {
		writeJSON(w, 500, map[string]any{"ok": false, "error": "server misconfigured: Chat API is nil"})
		return
	}

	type reqBody struct {
		RunName  string                  `json:"run_name"`
		Settings contractchat.SettingsV1 `json:"settings"`
	}

	var body reqBody
	if err := readJSON(r, &body); err != nil {
		writeJSON(w, 400, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	runName := strings.TrimSpace(body.RunName)
	if runName == "" {
		writeJSON(w, 400, map[string]any{"ok": false, "error": "missing run_name"})
		return
	}

	res, err := s.Chat.SetRunSettings(chatapp.SetRunSettingsRequest{
		ArtifactDir: s.ArtifactDir,
		RunName:     runName,
		Settings:    body.Settings,
	})
	if err != nil {
		writeJSON(w, 500, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	writeJSON(w, 200, map[string]any{"ok": true, "result": res})
}

func (s Server) handleRunAsync(w http.ResponseWriter, r *http.Request) {
	if s.Chat == nil {
		writeJSON(w, 500, map[string]any{"ok": false, "error": "server misconfigured: Chat API is nil"})
		return
	}
	type reqBody struct {
		RunName string `json:"run_name"`
		Message string `json:"message"`
	}
	var body reqBody
	if err := readJSON(r, &body); err != nil {
		writeJSON(w, 400, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	res, err := s.Chat.RunAsync(chatapp.RunAsyncRequest{
		ArtifactDir:    s.ArtifactDir,
		RunName:        body.RunName,
		Message:        body.Message,
		TailLimitBytes: 16384,
		NetEnabled:     s.NetEnabled,
		AllowDomains:   cloneStrings(s.AllowDomains),
	})
	if err != nil {
		writeJSON(w, 500, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "job_id": res.JobID, "run_name": res.RunName, "turn": res.Turn})
}

func (s Server) handleJobStatus(w http.ResponseWriter, r *http.Request) {
	if s.Chat == nil {
		writeJSON(w, 500, map[string]any{"ok": false, "error": "server misconfigured: Chat API is nil"})
		return
	}
	jobID := strings.TrimSpace(r.URL.Query().Get("job_id"))
	if jobID == "" {
		writeJSON(w, 400, map[string]any{"ok": false, "error": "missing job_id"})
		return
	}
	res, err := s.Chat.JobStatus(chatapp.JobStatusRequest{JobID: jobID})
	if err != nil {
		writeJSON(w, 404, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "job": res.Job})
}

func (s Server) handleJobTail(w http.ResponseWriter, r *http.Request) {
	if s.Chat == nil {
		writeText(w, 500, "server misconfigured: Chat API is nil\n")
		return
	}
	jobID := strings.TrimSpace(r.URL.Query().Get("job_id"))
	if jobID == "" {
		writeText(w, 400, "missing job_id\n")
		return
	}
	limit := intFromQuery(r, "limit", 16384)
	if limit <= 0 {
		limit = 16384
	}
	if limit > 2_000_000 {
		limit = 2_000_000
	}
	res, err := s.Chat.JobTail(chatapp.JobTailRequest{ArtifactDir: s.ArtifactDir, JobID: jobID, Limit: limit})
	if err != nil {
		writeText(w, 404, err.Error()+"\n")
		return
	}
	writeText(w, 200, res.Text)
}

func (s Server) handleJobCancel(w http.ResponseWriter, r *http.Request) {
	if s.Chat == nil {
		writeJSON(w, 500, map[string]any{"ok": false, "error": "server misconfigured: Chat API is nil"})
		return
	}
	type reqBody struct {
		JobID string `json:"job_id"`
	}
	var body reqBody
	if err := readJSON(r, &body); err != nil {
		writeJSON(w, 400, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	jobID := strings.TrimSpace(body.JobID)
	if jobID == "" {
		writeJSON(w, 400, map[string]any{"ok": false, "error": "missing job_id"})
		return
	}
	res, err := s.Chat.CancelJob(chatapp.CancelJobRequest{JobID: jobID})
	if err != nil {
		writeJSON(w, 404, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "job": res.Job})
}

func (s Server) handleProvidersList(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{"ok": true, "providers": []string{"stub", "openai"}})
}

func (s Server) handleProviderVerify(w http.ResponseWriter, r *http.Request) {
	if s.Chat == nil {
		writeJSON(w, 500, map[string]any{"ok": false, "error": "server misconfigured: Chat API is nil"})
		return
	}
	type reqBody struct {
		Provider       string `json:"provider"`
		TimeoutSeconds int    `json:"timeout_seconds,omitempty"`
	}
	var body reqBody
	if err := readJSON(r, &body); err != nil {
		writeJSON(w, 400, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	res, err := s.Chat.VerifyProvider(chatapp.VerifyProviderRequest{
		Provider:       body.Provider,
		TimeoutSeconds: body.TimeoutSeconds,
		NetEnabled:     s.NetEnabled,
		AllowDomains:   cloneStrings(s.AllowDomains),
	})
	if err != nil {
		writeJSON(w, 400, map[string]any{"ok": false, "error": err.Error(), "result": res})
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "result": res})
}

func (s Server) handleProviderModels(w http.ResponseWriter, r *http.Request) {
	if s.Chat == nil {
		writeJSON(w, 500, map[string]any{"ok": false, "error": "server misconfigured: Chat API is nil"})
		return
	}
	provider := strings.TrimSpace(r.URL.Query().Get("provider"))
	limit := intFromQuery(r, "limit", 200)
	timeoutSeconds := intFromQuery(r, "timeout_seconds", 25)
	cacheTTLSeconds := intFromQuery(r, "cache_ttl_seconds", 300)
	noCache := boolFromQuery(r, "no_cache", false)

	res, err := s.Chat.ListModels(chatapp.ListModelsRequest{
		Provider:        provider,
		Limit:           limit,
		TimeoutSeconds:  timeoutSeconds,
		NetEnabled:      s.NetEnabled,
		AllowDomains:    cloneStrings(s.AllowDomains),
		CacheTTLSeconds: cacheTTLSeconds,
		NoCache:         noCache,
	})
	if err != nil {
		writeJSON(w, 400, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "result": res})
}

////////////////////////////////////////////////////////////////////////////////
// Helpers
////////////////////////////////////////////////////////////////////////////////

func readJSON(r *http.Request, dst any) error {
	if r == nil || r.Body == nil {
		return fmt.Errorf("empty request body")
	}
	defer r.Body.Close()

	const maxBytes = 1_000_000
	b, err := io.ReadAll(io.LimitReader(r.Body, maxBytes))
	if err != nil {
		return fmt.Errorf("failed reading request body: %v", err)
	}
	if len(strings.TrimSpace(string(b))) == 0 {
		b = []byte("{}")
	}
	if err := json.Unmarshal(b, dst); err != nil {
		return fmt.Errorf("invalid json: %v", err)
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	if w == nil {
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	b, err := json.Marshal(v)
	if err != nil {
		_, _ = w.Write([]byte(`{"ok":false,"error":"failed to marshal json"}`))
		return
	}
	_, _ = w.Write(append(b, '\n'))
}

func writeText(w http.ResponseWriter, status int, s string) {
	if w == nil {
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(s))
}

func intFromQuery(r *http.Request, key string, def int) int {
	if r == nil || r.URL == nil {
		return def
	}
	raw := strings.TrimSpace(r.URL.Query().Get(key))
	if raw == "" {
		return def
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return def
	}
	return n
}

func boolFromQuery(r *http.Request, key string, def bool) bool {
	if r == nil || r.URL == nil {
		return def
	}
	raw := strings.TrimSpace(strings.ToLower(r.URL.Query().Get(key)))
	if raw == "" {
		return def
	}
	switch raw {
	case "1", "true", "yes", "y", "on":
		return true
	case "0", "false", "no", "n", "off":
		return false
	default:
		return def
	}
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
