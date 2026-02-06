package app

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	contractartifact "github.com/megamake/megamake/internal/contracts/v1/artifact"
	contractchat "github.com/megamake/megamake/internal/contracts/v1/chat"
	"github.com/megamake/megamake/internal/domains/chat/domain"
	"github.com/megamake/megamake/internal/domains/chat/ports"
)

// RunAsyncRequest starts an asynchronous chat turn.
type RunAsyncRequest struct {
	ArtifactDir string
	RunName     string

	// Message is the user message for the next turn.
	Message string

	// TailLimitBytes is a convenience default for tailing partial output.
	TailLimitBytes int

	// Network policy for provider calls during this job.
	// (These are passed by the server based on global flags.)
	NetEnabled   bool
	AllowDomains []string
}

type RunAsyncResult struct {
	JobID   string `json:"job_id"`
	RunName string `json:"run_name"`
	Turn    int    `json:"turn"`
}

type JobStatusRequest struct {
	JobID string
}

type JobStatusResult struct {
	Job ports.JobState `json:"job"`
}

type JobTailRequest struct {
	ArtifactDir string
	JobID       string
	Limit       int // bytes; if <=0, default (16384)
}

type JobTailResult struct {
	Text string `json:"text"`
}

func (s *Service) RunAsync(req RunAsyncRequest) (RunAsyncResult, error) {
	if s.Store == nil {
		return RunAsyncResult{}, fmt.Errorf("internal error: chat Store is nil")
	}
	if s.Jobs == nil {
		return RunAsyncResult{}, fmt.Errorf("internal error: chat Jobs is nil (job queue not wired)")
	}

	artifactDir := strings.TrimSpace(req.ArtifactDir)
	if artifactDir == "" {
		artifactDir = "."
	}
	runName := strings.TrimSpace(req.RunName)
	if runName == "" {
		return RunAsyncResult{}, fmt.Errorf("chat run_async: run_name is required")
	}
	if !domain.IsValidRunName(runName) {
		return RunAsyncResult{}, fmt.Errorf("chat run_async: invalid run_name: %s", runName)
	}
	msg := strings.TrimSpace(req.Message)
	if msg == "" {
		return RunAsyncResult{}, fmt.Errorf("chat run_async: message is required")
	}

	turn, err := s.Store.NextTurnNumber(ports.NextTurnNumberRequest{
		ArtifactDir: artifactDir,
		RunName:     runName,
	})
	if err != nil {
		return RunAsyncResult{}, err
	}

	jobID, _, err := s.Jobs.Create(ports.CreateJobRequest{
		RunName: runName,
		Turn:    turn,
		Message: "queued",
	})
	if err != nil {
		return RunAsyncResult{}, err
	}

	// Pre-write user artifacts synchronously.
	now := s.nowUTC()
	nowTS := contractartifact.FormatRFC3339NanoUTC(now)

	if err := s.Store.WriteUserTurnText(ports.WriteUserTurnTextRequest{
		ArtifactDir: artifactDir,
		RunName:     runName,
		Turn:        turn,
		Text:        msg,
	}); err != nil {
		_ = s.failJob(jobID, err)
		return RunAsyncResult{}, err
	}

	_ = s.Store.AppendTranscriptEvent(ports.AppendTranscriptEventRequest{
		ArtifactDir: artifactDir,
		RunName:     runName,
		Event: contractchat.TranscriptEventV1{
			Role: contractchat.RoleUser,
			Text: msg,
			TS:   nowTS,
			Turn: turn,
		},
	})

	// Update meta immediately after user message (monotonic and structurally correct).
	if meta, err := s.Store.ReadMeta(ports.ReadMetaRequest{ArtifactDir: artifactDir, RunName: runName}); err == nil {
		meta.UpdatedTS = nowTS
		meta.TurnsN = maxInt(meta.TurnsN, turn)
		meta.MessagesN = maxInt(meta.MessagesN, 2*turn-1)
		meta.LastError = ""
		_ = s.Store.WriteMeta(ports.WriteMetaRequest{ArtifactDir: artifactDir, RunName: runName, Meta: meta})
	}

	_ = s.updateJob(jobID, ports.JobRunning, intPtr(1), strPtr("running"), nil)

	// Start async worker.
	go s.runAsyncWorker(context.Background(), artifactDir, runName, turn, msg, jobID, req.NetEnabled, req.AllowDomains)

	return RunAsyncResult{
		JobID:   jobID,
		RunName: runName,
		Turn:    turn,
	}, nil
}

func (s *Service) JobStatus(req JobStatusRequest) (JobStatusResult, error) {
	if s.Jobs == nil {
		return JobStatusResult{}, fmt.Errorf("internal error: chat Jobs is nil (job queue not wired)")
	}
	id := strings.TrimSpace(req.JobID)
	if id == "" {
		return JobStatusResult{}, fmt.Errorf("chat jobs status: job_id is required")
	}
	st, ok := s.Jobs.Get(id)
	if !ok {
		return JobStatusResult{}, fmt.Errorf("chat jobs status: unknown job_id: %s", id)
	}
	return JobStatusResult{Job: st}, nil
}

func (s *Service) JobTail(req JobTailRequest) (JobTailResult, error) {
	if s.Jobs == nil {
		return JobTailResult{}, fmt.Errorf("internal error: chat Jobs is nil (job queue not wired)")
	}

	artifactDir := strings.TrimSpace(req.ArtifactDir)
	if artifactDir == "" {
		artifactDir = "."
	}

	id := strings.TrimSpace(req.JobID)
	if id == "" {
		return JobTailResult{}, fmt.Errorf("chat jobs tail: job_id is required")
	}

	st, ok := s.Jobs.Get(id)
	if !ok {
		return JobTailResult{}, fmt.Errorf("chat jobs tail: unknown job_id: %s", id)
	}

	limit := req.Limit
	if limit <= 0 {
		limit = 16384
	}
	if limit > 2_000_000 {
		limit = 2_000_000
	}

	files := domain.TurnFiles(st.Turn)
	runDir := filepath.Join(artifactDir, "MEGACHAT", "runs", st.RunName)

	partialPath := filepath.Join(runDir, files.AssistantPartialTextFile)
	finalPath := filepath.Join(runDir, files.AssistantTextFile)

	if txt, ok := readTailFile(partialPath, limit); ok {
		return JobTailResult{Text: txt}, nil
	}
	if txt, ok := readTailFile(finalPath, limit); ok {
		return JobTailResult{Text: txt}, nil
	}
	return JobTailResult{Text: ""}, nil
}

////////////////////////////////////////////////////////////////////////////////
// Worker (provider streaming)
////////////////////////////////////////////////////////////////////////////////

func (s *Service) runAsyncWorker(parentCtx context.Context, artifactDir string, runName string, turn int, userMsg string, jobID string, netEnabled bool, allowDomains []string) {
	started := s.nowUTC()
	startedTS := contractartifact.FormatRFC3339NanoUTC(started)

	// Resolve run meta.
	meta, err := s.Store.ReadMeta(ports.ReadMetaRequest{ArtifactDir: artifactDir, RunName: runName})
	if err != nil {
		_ = s.markJobError(jobID, err)
		return
	}

	// Load global settings once (defaults -> global overlay).
	global := defaultSettings()
	if s.Settings != nil {
		if gs, found, _ := s.Settings.Read(ports.ReadSettingsRequest{ArtifactDir: artifactDir}); found {
			// Provider/model/system/developer can come from global settings.
			if strings.TrimSpace(gs.Provider) != "" {
				global.Provider = strings.TrimSpace(gs.Provider)
			}
			if strings.TrimSpace(gs.Model) != "" {
				global.Model = strings.TrimSpace(gs.Model)
			}
			if strings.TrimSpace(gs.SystemText) != "" {
				global.SystemText = gs.SystemText
			}
			if strings.TrimSpace(gs.DeveloperText) != "" {
				global.DeveloperText = gs.DeveloperText
			}

			// Behavior/tool fields (best-effort)
			if gs.TextFormat != "" {
				global.TextFormat = gs.TextFormat
			}
			if gs.Verbosity != "" {
				global.Verbosity = gs.Verbosity
			}
			if gs.Effort != "" {
				global.Effort = gs.Effort
			}
			global.SummaryAuto = gs.SummaryAuto
			if gs.MaxOutputTokens > 0 {
				global.MaxOutputTokens = gs.MaxOutputTokens
			}
			global.Tools = gs.Tools
		}
	}

	// Load per-run settings once (optional overrides + behavior snapshot).
	var runSettings contractchat.SettingsV1
	runFound := false
	if s.RunSettings != nil {
		if rs, found, _ := s.RunSettings.ReadRunSettings(ports.ReadRunSettingsRequest{
			ArtifactDir: artifactDir,
			RunName:     runName,
		}); found {
			runSettings = rs
			runFound = true
		}
	}

	// Provider registry required.
	if s.Providers == nil {
		_ = s.markJobError(jobID, fmt.Errorf("internal error: Providers registry is nil"))
		return
	}

	// Provider selection precedence:
	//   1) per-run settings provider override (if non-empty)
	//   2) run meta provider
	//   3) global settings provider (if non-empty)
	//   4) registry default
	providerName := strings.TrimSpace(meta.Provider)
	if runFound && strings.TrimSpace(runSettings.Provider) != "" {
		providerName = strings.TrimSpace(runSettings.Provider)
	}
	if providerName == "" && strings.TrimSpace(global.Provider) != "" {
		providerName = strings.TrimSpace(global.Provider)
	}

	prov, ok := s.Providers.Get(providerName)
	if !ok || prov == nil {
		prov = s.Providers.Default()
	}

	// Enforce network policy for this provider.
	if err := requireProviderAllowed(netEnabled, allowDomains, prov); err != nil {
		_ = s.markJobError(jobID, err)
		s.updateMetaOnError(artifactDir, runName, turn, err.Error(), startedTS)
		return
	}

	// Build conversation messages from transcript tail (includes the user message we already appended).
	evs, _ := s.Store.ReadTranscriptTail(ports.ReadTranscriptTailRequest{
		ArtifactDir: artifactDir,
		RunName:     runName,
		Limit:       2000,
	})
	msgs := transcriptToChatMessages(evs)

	// Resolve effective settings now (global baseline + run behavior snapshot if present).
	effective := global

	// Run overrides (optional):
	runModelOverride := ""
	runSystemOverride := ""
	runDeveloperOverride := ""

	if runFound {
		// Normalize behavior fields (but keep provider/model optional).
		rs := runSettings
		rs = normalizeSettings(rs)

		if strings.TrimSpace(rs.Model) != "" {
			runModelOverride = strings.TrimSpace(rs.Model)
		}
		if strings.TrimSpace(rs.SystemText) != "" {
			runSystemOverride = rs.SystemText
		}
		if strings.TrimSpace(rs.DeveloperText) != "" {
			runDeveloperOverride = rs.DeveloperText
		}

		// Treat run behavior fields as authoritative snapshot.
		if rs.TextFormat != "" {
			effective.TextFormat = rs.TextFormat
		}
		if rs.Verbosity != "" {
			effective.Verbosity = rs.Verbosity
		}
		if rs.Effort != "" {
			effective.Effort = rs.Effort
		}
		effective.SummaryAuto = rs.SummaryAuto
		if rs.MaxOutputTokens > 0 {
			effective.MaxOutputTokens = rs.MaxOutputTokens
		}
		effective.Tools = rs.Tools
	}

	// Playground-like resolution for model/system/developer:
	// Model: run override > meta > global/default
	model := strings.TrimSpace(meta.Model)
	if model == "" {
		model = strings.TrimSpace(effective.Model)
	}
	if runModelOverride != "" {
		model = runModelOverride
	}

	// System/Developer: run override > meta > global/default
	sys := strings.TrimSpace(meta.SystemText)
	if runSystemOverride != "" {
		sys = runSystemOverride
	} else if sys == "" {
		sys = effective.SystemText
	}
	dev := strings.TrimSpace(meta.DeveloperText)
	if runDeveloperOverride != "" {
		dev = runDeveloperOverride
	} else if dev == "" {
		dev = effective.DeveloperText
	}

	req := ports.ChatRequest{
		Model:           model,
		SystemText:      sys,
		DeveloperText:   dev,
		Messages:        msgs,
		TextFormat:      effective.TextFormat,
		Verbosity:       effective.Verbosity,
		Effort:          effective.Effort,
		SummaryAuto:     effective.SummaryAuto,
		MaxOutputTokens: effective.MaxOutputTokens,
		Tools:           effective.Tools,
	}

	// Snapshot effective (non-secret) settings used for this turn.
	turnSettings := contractchat.SettingsV1{
		Provider:        prov.Name(),
		Model:           req.Model,
		SystemText:      req.SystemText,
		DeveloperText:   req.DeveloperText,
		TextFormat:      req.TextFormat,
		Verbosity:       req.Verbosity,
		Effort:          req.Effort,
		SummaryAuto:     req.SummaryAuto,
		MaxOutputTokens: req.MaxOutputTokens,
		Tools:           req.Tools,
		UpdatedTS:       startedTS,
	}
	settingsSnapshot := &turnSettings

	// Cancellation: cancel request context when job is canceled.
	ctx := parentCtx
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	isCanceled := func() bool {
		select {
		case <-ctx.Done():
			return true
		default:
		}
		if s.Jobs == nil {
			return false
		}
		st, ok := s.Jobs.Get(jobID)
		if !ok {
			return false
		}
		return st.Status == ports.JobCanceled
	}

	var firstByteAt time.Time
	var usageProvider *contractchat.TokenUsageV1
	var providerReqID string
	var out strings.Builder

	handler := &workerStreamHandler{
		s:           s,
		artifactDir: artifactDir,
		runName:     runName,
		turn:        turn,
		jobID:       jobID,
		cancel:      cancel,
		isCanceled:  isCanceled,
		out:         &out,
		firstByteAt: &firstByteAt,
		usage:       &usageProvider,
		requestID:   &providerReqID,
		started:     started,
	}

	resp, callErr := prov.StreamChat(ctx, req, handler)

	// Capture response metadata if present
	if strings.TrimSpace(resp.ProviderRequestID) != "" {
		providerReqID = resp.ProviderRequestID
	}
	if resp.UsageProvider != nil {
		usageProvider = resp.UsageProvider
	}

	if isCanceled() {
		s.finishCanceledTurn(artifactDir, runName, turn, jobID, started, startedTS, firstByteAt, userMsg, out.String(), usageProvider, providerReqID, settingsSnapshot)
		return
	}

	if callErr != nil {
		_ = s.markJobError(jobID, callErr)
		s.finishErroredTurn(artifactDir, runName, turn, started, startedTS, firstByteAt, userMsg, out.String(), usageProvider, providerReqID, settingsSnapshot, callErr)
		return
	}

	// Success: commit assistant output
	completed := s.nowUTC()
	completedTS := contractartifact.FormatRFC3339NanoUTC(completed)

	assistantText := out.String()
	if strings.TrimSpace(assistantText) == "" {
		assistantText = resp.Text
	}

	_ = s.Store.WriteAssistantFinalText(ports.WriteAssistantFinalTextRequest{
		ArtifactDir: artifactDir,
		RunName:     runName,
		Turn:        turn,
		Text:        assistantText,
	})

	// Internal token usage estimate (best-effort)
	var usageInternal *contractchat.TokenUsageV1
	if s.TokenCounter != nil {
		inText := buildInputText(req)
		u := s.TokenCounter.Count(ports.TokenCountRequest{
			Provider:   prov.Name(),
			Model:      req.Model,
			InputText:  inText,
			OutputText: assistantText,
		})
		usageInternal = &u
	}

	// Append transcript assistant event (commit) with usage if available
	_ = s.Store.AppendTranscriptEvent(ports.AppendTranscriptEventRequest{
		ArtifactDir: artifactDir,
		RunName:     runName,
		Event: contractchat.TranscriptEventV1{
			Role:          contractchat.RoleAssistant,
			Text:          assistantText,
			TS:            completedTS,
			Turn:          turn,
			Provider:      prov.Name(),
			Model:         req.Model,
			UsageProvider: usageProvider,
			UsageInternal: usageInternal,
		},
	})

	// Durations
	var ttfbMs *int
	if !firstByteAt.IsZero() {
		v := int(firstByteAt.Sub(started).Milliseconds())
		ttfbMs = &v
	}
	totalMsV := int(completed.Sub(started).Milliseconds())
	totalMs := &totalMsV

	metrics := contractchat.TurnMetricsV1{
		Turn:              turn,
		RunName:           runName,
		Provider:          prov.Name(),
		Model:             req.Model,
		Settings:          settingsSnapshot,
		StartedTS:         startedTS,
		CompletedTS:       completedTS,
		TTFBMs:            ttfbMs,
		TotalMs:           totalMs,
		UsageProvider:     usageProvider,
		UsageInternal:     usageInternal,
		ProviderRequestID: providerReqID,
	}
	if !firstByteAt.IsZero() {
		metrics.FirstByteTS = contractartifact.FormatRFC3339NanoUTC(firstByteAt)
	}

	_ = s.Store.WriteTurnMetrics(ports.WriteTurnMetricsRequest{
		ArtifactDir: artifactDir,
		RunName:     runName,
		Metrics:     metrics,
	})

	// Update meta
	if meta2, err := s.Store.ReadMeta(ports.ReadMetaRequest{ArtifactDir: artifactDir, RunName: runName}); err == nil {
		meta2.UpdatedTS = completedTS
		meta2.TurnsN = maxInt(meta2.TurnsN, turn)
		meta2.MessagesN = maxInt(meta2.MessagesN, 2*turn)

		meta2.LastTTFBMs = ttfbMs
		meta2.LastTotalMs = totalMs
		meta2.LastError = ""

		meta2.LastUsageProvider = usageProvider
		meta2.LastUsageInternal = usageInternal

		_ = s.Store.WriteMeta(ports.WriteMetaRequest{ArtifactDir: artifactDir, RunName: runName, Meta: meta2})
	}

	doneMsg := "done"
	p100 := 100
	_ = s.updateJob(jobID, ports.JobDone, &p100, &doneMsg, nil)
}

// workerStreamHandler writes streaming deltas to assistant partial file and updates job state.
type workerStreamHandler struct {
	s           *Service
	artifactDir string
	runName     string
	turn        int
	jobID       string

	cancel     context.CancelFunc
	isCanceled func() bool

	out         *strings.Builder
	firstByteAt *time.Time

	usage     **contractchat.TokenUsageV1
	requestID *string

	started time.Time
}

func (h *workerStreamHandler) OnStart() {}

func (h *workerStreamHandler) OnDelta(delta string) {
	if strings.TrimSpace(delta) == "" {
		return
	}
	if h.isCanceled != nil && h.isCanceled() {
		if h.cancel != nil {
			h.cancel()
		}
		return
	}

	now := time.Now().UTC()
	if h.firstByteAt != nil && h.firstByteAt.IsZero() {
		*h.firstByteAt = now
	}

	if h.out != nil {
		h.out.WriteString(delta)
	}

	// Write assistant partial snapshot (overwrite entire file).
	if h.s != nil && h.s.Store != nil {
		_ = h.s.Store.WriteAssistantPartialText(ports.WriteAssistantPartialTextRequest{
			ArtifactDir: h.artifactDir,
			RunName:     h.runName,
			Turn:        h.turn,
			Text:        h.out.String(),
		})
	}

	// Update job (percent best-effort)
	if h.s != nil {
		pct := 50
		msg := "streaming"
		_ = h.s.updateJob(h.jobID, ports.JobRunning, &pct, &msg, nil)
	}
}

func (h *workerStreamHandler) OnUsage(u contractchat.TokenUsageV1) {
	if h.usage != nil {
		uu := u
		*h.usage = &uu
	}
}

func (h *workerStreamHandler) OnError(err error) {
	_ = err
}

func (h *workerStreamHandler) OnDone() {}

////////////////////////////////////////////////////////////////////////////////
// Turn finalization helpers
////////////////////////////////////////////////////////////////////////////////

func (s *Service) finishCanceledTurn(artifactDir, runName string, turn int, jobID string, started time.Time, startedTS string, firstByteAt time.Time, userMsg string, assistantPartial string, usageProvider *contractchat.TokenUsageV1, providerReqID string, settingsSnapshot *contractchat.SettingsV1) {
	completed := s.nowUTC()
	completedTS := contractartifact.FormatRFC3339NanoUTC(completed)

	// internal usage best-effort
	var usageInternal *contractchat.TokenUsageV1
	if s.TokenCounter != nil {
		u := s.TokenCounter.Count(ports.TokenCountRequest{
			Provider:   "",
			Model:      "",
			InputText:  userMsg,
			OutputText: assistantPartial,
		})
		usageInternal = &u
	}

	var ttfbMs *int
	if !firstByteAt.IsZero() {
		v := int(firstByteAt.Sub(started).Milliseconds())
		ttfbMs = &v
	}
	totalMsV := int(completed.Sub(started).Milliseconds())
	totalMs := &totalMsV

	metrics := contractchat.TurnMetricsV1{
		Turn:              turn,
		RunName:           runName,
		Settings:          settingsSnapshot,
		StartedTS:         startedTS,
		CompletedTS:       completedTS,
		TTFBMs:            ttfbMs,
		TotalMs:           totalMs,
		UsageProvider:     usageProvider,
		UsageInternal:     usageInternal,
		ProviderRequestID: providerReqID,
		Error:             "canceled",
	}
	if !firstByteAt.IsZero() {
		metrics.FirstByteTS = contractartifact.FormatRFC3339NanoUTC(firstByteAt)
	}
	_ = s.Store.WriteTurnMetrics(ports.WriteTurnMetricsRequest{ArtifactDir: artifactDir, RunName: runName, Metrics: metrics})

	// Meta: user message exists, assistant not committed => messages >= 2*turn-1.
	if meta, err := s.Store.ReadMeta(ports.ReadMetaRequest{ArtifactDir: artifactDir, RunName: runName}); err == nil {
		meta.UpdatedTS = completedTS
		meta.TurnsN = maxInt(meta.TurnsN, turn)
		meta.MessagesN = maxInt(meta.MessagesN, 2*turn-1)

		meta.LastTTFBMs = ttfbMs
		meta.LastTotalMs = totalMs
		meta.LastError = "canceled"
		meta.LastUsageProvider = usageProvider
		meta.LastUsageInternal = usageInternal

		_ = s.Store.WriteMeta(ports.WriteMetaRequest{ArtifactDir: artifactDir, RunName: runName, Meta: meta})
	}

	// Ensure job is marked canceled (Cancel() already sets it, but be defensive).
	msg := "canceled"
	p100 := 100
	_ = s.updateJob(jobID, ports.JobCanceled, &p100, &msg, nil)
}

func (s *Service) finishErroredTurn(artifactDir, runName string, turn int, started time.Time, startedTS string, firstByteAt time.Time, userMsg string, assistantPartial string, usageProvider *contractchat.TokenUsageV1, providerReqID string, settingsSnapshot *contractchat.SettingsV1, err error) {

	completed := s.nowUTC()
	completedTS := contractartifact.FormatRFC3339NanoUTC(completed)

	var usageInternal *contractchat.TokenUsageV1
	if s.TokenCounter != nil {
		u := s.TokenCounter.Count(ports.TokenCountRequest{
			Provider:   "",
			Model:      "",
			InputText:  userMsg,
			OutputText: assistantPartial,
		})
		usageInternal = &u
	}

	var ttfbMs *int
	if !firstByteAt.IsZero() {
		v := int(firstByteAt.Sub(started).Milliseconds())
		ttfbMs = &v
	}
	totalMsV := int(completed.Sub(started).Milliseconds())
	totalMs := &totalMsV

	metrics := contractchat.TurnMetricsV1{
		Turn:              turn,
		RunName:           runName,
		Settings:          settingsSnapshot,
		StartedTS:         startedTS,
		CompletedTS:       completedTS,
		TTFBMs:            ttfbMs,
		TotalMs:           totalMs,
		UsageProvider:     usageProvider,
		UsageInternal:     usageInternal,
		ProviderRequestID: providerReqID,
		Error:             err.Error(),
	}
	if !firstByteAt.IsZero() {
		metrics.FirstByteTS = contractartifact.FormatRFC3339NanoUTC(firstByteAt)
	}

	_ = s.Store.WriteTurnMetrics(ports.WriteTurnMetricsRequest{ArtifactDir: artifactDir, RunName: runName, Metrics: metrics})

	// Meta: user message exists, assistant not committed => messages >= 2*turn-1.
	if meta, err2 := s.Store.ReadMeta(ports.ReadMetaRequest{ArtifactDir: artifactDir, RunName: runName}); err2 == nil {
		meta.UpdatedTS = completedTS
		meta.TurnsN = maxInt(meta.TurnsN, turn)
		meta.MessagesN = maxInt(meta.MessagesN, 2*turn-1)

		meta.LastTTFBMs = ttfbMs
		meta.LastTotalMs = totalMs
		meta.LastError = err.Error()
		meta.LastUsageProvider = usageProvider
		meta.LastUsageInternal = usageInternal

		_ = s.Store.WriteMeta(ports.WriteMetaRequest{ArtifactDir: artifactDir, RunName: runName, Meta: meta})
	}
}

func (s *Service) updateMetaOnError(artifactDir, runName string, turn int, errMsg string, nowTS string) {
	if meta, err := s.Store.ReadMeta(ports.ReadMetaRequest{ArtifactDir: artifactDir, RunName: runName}); err == nil {
		meta.UpdatedTS = nowTS
		meta.TurnsN = maxInt(meta.TurnsN, turn)
		meta.MessagesN = maxInt(meta.MessagesN, 2*turn-1)
		meta.LastError = errMsg
		_ = s.Store.WriteMeta(ports.WriteMetaRequest{ArtifactDir: artifactDir, RunName: runName, Meta: meta})
	}
}

func buildInputText(req ports.ChatRequest) string {
	var b strings.Builder
	if strings.TrimSpace(req.SystemText) != "" {
		b.WriteString("system:\n")
		b.WriteString(req.SystemText)
		b.WriteString("\n\n")
	}
	if strings.TrimSpace(req.DeveloperText) != "" {
		b.WriteString("developer:\n")
		b.WriteString(req.DeveloperText)
		b.WriteString("\n\n")
	}
	for _, m := range req.Messages {
		r := strings.TrimSpace(m.Role)
		if r == "" {
			r = "user"
		}
		b.WriteString(r)
		b.WriteString(":\n")
		b.WriteString(m.Text)
		b.WriteString("\n\n")
	}
	return b.String()
}

func transcriptToChatMessages(evs []contractchat.TranscriptEventV1) []ports.ChatMessage {
	var out []ports.ChatMessage
	for _, ev := range evs {
		role := strings.ToLower(strings.TrimSpace(string(ev.Role)))
		if role == "" {
			role = "user"
		}
		// Only keep common roles.
		switch role {
		case "user", "assistant", "system", "developer":
		default:
			role = "user"
		}
		txt := ev.Text
		if strings.TrimSpace(txt) == "" {
			continue
		}
		out = append(out, ports.ChatMessage{Role: role, Text: txt})
	}
	return out
}

////////////////////////////////////////////////////////////////////////////////
// Existing helpers (errors + tail reader + job updates)
////////////////////////////////////////////////////////////////////////////////

func (s *Service) failJob(jobID string, err error) error {
	msg := "error"
	e := err.Error()
	_, _ = s.Jobs.Update(ports.UpdateJobRequest{
		JobID:   jobID,
		Status:  ports.JobError,
		Message: &msg,
		Error:   &e,
	})
	return err
}

func (s *Service) markJobError(jobID string, err error) error {
	msg := "error"
	e := err.Error()
	_, _ = s.Jobs.Update(ports.UpdateJobRequest{
		JobID:   jobID,
		Status:  ports.JobError,
		Message: &msg,
		Error:   &e,
	})
	return err
}

func (s *Service) updateJob(jobID string, st ports.JobStatus, pct *int, msg *string, errStr *string) error {
	if s.Jobs == nil {
		return nil
	}
	_, _ = s.Jobs.Update(ports.UpdateJobRequest{
		JobID:   jobID,
		Status:  st,
		Percent: pct,
		Message: msg,
		Error:   errStr,
	})
	return nil
}

func (s *Service) nowUTC() time.Time {
	if s.Clock != nil {
		return s.Clock.NowUTC()
	}
	return time.Now().UTC()
}

// readTailFile reads up to the last limit bytes of a file.
// Returns ok=false if file doesn't exist or cannot be read.
func readTailFile(path string, limit int) (text string, ok bool) {
	if strings.TrimSpace(path) == "" {
		return "", false
	}
	f, err := os.Open(path)
	if err != nil {
		return "", false
	}
	defer f.Close()

	st, err := f.Stat()
	if err != nil {
		return "", false
	}
	size := st.Size()
	if size <= 0 {
		return "", true
	}
	if int64(limit) <= 0 {
		limit = 16384
	}

	start := int64(0)
	if size > int64(limit) {
		start = size - int64(limit)
	}
	if _, err := f.Seek(start, 0); err != nil {
		return "", false
	}
	b, err := ioReadAllLimit(f, int64(limit))
	if err != nil {
		return "", false
	}
	return string(b), true
}

// ioReadAllLimit reads up to max bytes.
func ioReadAllLimit(r *os.File, max int64) ([]byte, error) {
	if max <= 0 {
		max = 16384
	}
	buf := make([]byte, 0, minInt64(max, 64*1024))
	tmp := make([]byte, 4096)
	var nread int64
	for {
		if nread >= max {
			break
		}
		want := int64(len(tmp))
		if max-nread < want {
			want = max - nread
		}
		n, err := r.Read(tmp[:want])
		if n > 0 {
			buf = append(buf, tmp[:n]...)
			nread += int64(n)
		}
		if err != nil {
			break
		}
	}
	return buf, nil
}

func minInt64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func intPtr(v int) *int       { return &v }
func strPtr(s string) *string { return &s }

// Ensure io is linked (used above)
var _ = io.EOF
