package app

import (
	"crypto/rand"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	contractartifact "github.com/megamake/megamake/internal/contracts/v1/artifact"
	contractchat "github.com/megamake/megamake/internal/contracts/v1/chat"
	"github.com/megamake/megamake/internal/domains/chat/domain"
	"github.com/megamake/megamake/internal/domains/chat/ports"
	"github.com/megamake/megamake/internal/platform/clock"
)

// Service implements the chat application use-cases (runs + settings).
//
// It orchestrates ports:
// - RunStore (filesystem persistence)
// - SettingsStore (artifact-dir scoped settings)
// - EnvLoader (.env loader)
type Service struct {
	Clock    clock.Clock
	Store    ports.RunStore
	Settings ports.SettingsStore
	Env      ports.EnvLoader

	Jobs         ports.JobQueue
	TokenCounter ports.TokenCounter

	Providers  ports.ProviderRegistry
	ModelCache ports.ModelCache

	RunSettings ports.RunSettingsStore
}

type NewRunRequest struct {
	ArtifactDir string

	Title string

	// Optional overrides. If empty, service will fall back to settings (if present).
	Provider      string
	Model         string
	SystemText    string
	DeveloperText string

	// EnvFile overrides default .env path resolution.
	// If empty, the service may load <artifactDir>/MEGACHAT/.env (best-effort).
	EnvFile string

	// If true, loaded env vars overwrite existing process env vars.
	// Recommended default: false.
	EnvOverwrite bool
}

type NewRunResult struct {
	RunName  string
	Args     contractchat.ArgsV1
	Meta     contractchat.MetaV1
	Settings contractchat.SettingsV1
}

type ListRunsRequest struct {
	ArtifactDir string
	Limit       int
}

type ListRunsResult struct {
	Items []contractchat.MetaV1
}

type GetRunRequest struct {
	ArtifactDir string
	RunName     string

	Tail int // transcript tail events; if <=0 service uses default
}

type GetRunResult struct {
	Meta   contractchat.MetaV1
	Events []contractchat.TranscriptEventV1
}

type ConfigGetRequest struct {
	ArtifactDir string

	// EnvFile override (optional)
	EnvFile string
}

type ConfigGetResult struct {
	Settings contractchat.SettingsV1
	Found    bool
}

type ConfigSetRequest struct {
	ArtifactDir string

	// Full settings replacement (simple + explicit).
	// (We can add partial patch semantics later if you want.)
	Settings contractchat.SettingsV1
}

type ConfigSetResult struct{}

// NewRun creates a new chat run under <artifactDir>/MEGACHAT/runs/<run_name>/...
func (s *Service) NewRun(req NewRunRequest) (NewRunResult, error) {
	if s.Store == nil {
		return NewRunResult{}, fmt.Errorf("internal error: chat Store is nil")
	}
	if s.Settings == nil {
		return NewRunResult{}, fmt.Errorf("internal error: chat Settings is nil")
	}
	// Env loader is optional; provider work happens later, but we still support loading.
	// If s.Env is nil, we simply skip env load.

	artifactDir := strings.TrimSpace(req.ArtifactDir)
	if artifactDir == "" {
		artifactDir = "."
	}

	now := time.Now().UTC()
	if s.Clock != nil {
		now = s.Clock.NowUTC()
	}
	nowTS := contractartifact.FormatRFC3339NanoUTC(now)

	// Best-effort env load (so later provider calls can rely on env being loaded).
	envPath := strings.TrimSpace(req.EnvFile)
	if envPath == "" {
		envPath = filepath.Join(artifactDir, "MEGACHAT", ".env")
	}
	if s.Env != nil {
		_, _ = s.Env.Load(ports.LoadEnvRequest{Path: envPath, Overwrite: req.EnvOverwrite})
	}

	// Base settings from store (if any)
	baseSettings, found, err := s.Settings.Read(ports.ReadSettingsRequest{ArtifactDir: artifactDir})
	if err != nil {
		return NewRunResult{}, err
	}
	if !found {
		baseSettings = defaultSettings()
	}

	// Apply overrides from request (only when non-empty).
	effective := baseSettings
	if strings.TrimSpace(req.Provider) != "" {
		effective.Provider = strings.TrimSpace(req.Provider)
	}
	if strings.TrimSpace(req.Model) != "" {
		effective.Model = strings.TrimSpace(req.Model)
	}
	if req.SystemText != "" {
		effective.SystemText = req.SystemText
	}
	if req.DeveloperText != "" {
		effective.DeveloperText = req.DeveloperText
	}

	// Ensure minimal sensible defaults.
	if strings.TrimSpace(effective.Provider) == "" {
		effective.Provider = "openai"
	}
	if strings.TrimSpace(effective.Model) == "" {
		effective.Model = "gpt-5"
	}
	if effective.TextFormat == "" {
		effective.TextFormat = contractchat.TextFormatText
	}
	if effective.Verbosity == "" {
		effective.Verbosity = contractchat.VerbosityHigh
	}
	if effective.Effort == "" {
		effective.Effort = contractchat.EffortHigh
	}
	if effective.MaxOutputTokens == 0 {
		effective.MaxOutputTokens = 4096
	}

	// Build run name.
	sfx := make([]byte, 4)
	_, _ = rand.Read(sfx) // best-effort; even if it fails, suffix will be zeros
	runName := domain.BuildRunName(now, sfx)

	// Create args + meta snapshot.
	title := strings.TrimSpace(req.Title)
	if title == "" {
		title = "Untitled Conversation"
	}

	args := contractchat.ArgsV1{
		Title:         title,
		Provider:      effective.Provider,
		Model:         effective.Model,
		SystemText:    effective.SystemText,
		DeveloperText: effective.DeveloperText,
		Timestamp:     nowTS,
	}

	meta := contractchat.MetaV1{
		RunName:       runName,
		Title:         title,
		Provider:      effective.Provider,
		Model:         effective.Model,
		SystemText:    effective.SystemText,
		DeveloperText: effective.DeveloperText,
		CreatedTS:     nowTS,
		UpdatedTS:     nowTS,
		MessagesN:     0,
		TurnsN:        0,
	}

	// Persist run.
	snap := effective // snapshot per run (non-secret)
	if err := s.Store.CreateRun(ports.CreateRunRequest{
		ArtifactDir:      artifactDir,
		RunName:          runName,
		Args:             args,
		Meta:             meta,
		SettingsSnapshot: &snap,
	}); err != nil {
		return NewRunResult{}, err
	}

	return NewRunResult{
		RunName:  runName,
		Args:     args,
		Meta:     meta,
		Settings: effective,
	}, nil
}

func (s *Service) ListRuns(req ListRunsRequest) (ListRunsResult, error) {
	if s.Store == nil {
		return ListRunsResult{}, fmt.Errorf("internal error: chat Store is nil")
	}
	artifactDir := strings.TrimSpace(req.ArtifactDir)
	if artifactDir == "" {
		artifactDir = "."
	}
	items, err := s.Store.ListRuns(ports.ListRunsRequest{
		ArtifactDir: artifactDir,
		Limit:       req.Limit,
	})
	if err != nil {
		return ListRunsResult{}, err
	}
	return ListRunsResult{Items: items}, nil
}

func (s *Service) GetRun(req GetRunRequest) (GetRunResult, error) {
	if s.Store == nil {
		return GetRunResult{}, fmt.Errorf("internal error: chat Store is nil")
	}
	artifactDir := strings.TrimSpace(req.ArtifactDir)
	if artifactDir == "" {
		artifactDir = "."
	}
	runName := strings.TrimSpace(req.RunName)
	if runName == "" {
		return GetRunResult{}, fmt.Errorf("chat get: run_name is required")
	}
	if !domain.IsValidRunName(runName) {
		return GetRunResult{}, fmt.Errorf("chat get: invalid run_name: %s", runName)
	}

	meta, err := s.Store.ReadMeta(ports.ReadMetaRequest{
		ArtifactDir: artifactDir,
		RunName:     runName,
	})
	if err != nil {
		return GetRunResult{}, err
	}

	evs, err := s.Store.ReadTranscriptTail(ports.ReadTranscriptTailRequest{
		ArtifactDir: artifactDir,
		RunName:     runName,
		Limit:       req.Tail,
	})
	if err != nil {
		return GetRunResult{}, err
	}

	return GetRunResult{Meta: meta, Events: evs}, nil
}

func (s *Service) ConfigGet(req ConfigGetRequest) (ConfigGetResult, error) {
	if s.Settings == nil {
		return ConfigGetResult{}, fmt.Errorf("internal error: chat Settings is nil")
	}

	artifactDir := strings.TrimSpace(req.ArtifactDir)
	if artifactDir == "" {
		artifactDir = "."
	}

	// Best-effort load env as part of config operations (so verify/models later work predictably).
	// This does not error if the file is missing.
	envPath := strings.TrimSpace(req.EnvFile)
	if envPath == "" {
		envPath = filepath.Join(artifactDir, "MEGACHAT", ".env")
	}
	if s.Env != nil {
		_, _ = s.Env.Load(ports.LoadEnvRequest{Path: envPath, Overwrite: false})
	}

	sv, found, err := s.Settings.Read(ports.ReadSettingsRequest{ArtifactDir: artifactDir})
	if err != nil {
		return ConfigGetResult{}, err
	}
	if !found {
		return ConfigGetResult{Settings: defaultSettings(), Found: false}, nil
	}
	return ConfigGetResult{Settings: sv, Found: true}, nil
}

func (s *Service) ConfigSet(req ConfigSetRequest) (ConfigSetResult, error) {
	if s.Settings == nil {
		return ConfigSetResult{}, fmt.Errorf("internal error: chat Settings is nil")
	}
	artifactDir := strings.TrimSpace(req.ArtifactDir)
	if artifactDir == "" {
		artifactDir = "."
	}
	// Settings adapter will fill UpdatedTS if missing, but we can normalize here too.
	if strings.TrimSpace(req.Settings.Provider) == "" {
		req.Settings.Provider = "openai"
	}
	if strings.TrimSpace(req.Settings.Model) == "" {
		req.Settings.Model = "gpt-5"
	}
	if req.Settings.TextFormat == "" {
		req.Settings.TextFormat = contractchat.TextFormatText
	}
	if req.Settings.Verbosity == "" {
		req.Settings.Verbosity = contractchat.VerbosityHigh
	}
	if req.Settings.Effort == "" {
		req.Settings.Effort = contractchat.EffortHigh
	}
	if req.Settings.MaxOutputTokens <= 0 {
		req.Settings.MaxOutputTokens = 999999
	}

	if err := s.Settings.Write(ports.WriteSettingsRequest{
		ArtifactDir: artifactDir,
		Settings:    req.Settings,
	}); err != nil {
		return ConfigSetResult{}, err
	}
	return ConfigSetResult{}, nil
}

func defaultSettings() contractchat.SettingsV1 {
	return contractchat.SettingsV1{
		Provider:        "openai",
		Model:           "gpt-5",
		TextFormat:      contractchat.TextFormatText,
		Verbosity:       contractchat.VerbosityHigh,
		Effort:          contractchat.EffortHigh,
		SummaryAuto:     true,
		MaxOutputTokens: 999999,
		Tools: contractchat.ToolsV1{
			WebSearch:       false,
			CodeInterpreter: false,
			FileSearch:      false,
			ImageGeneration: false,
		},
	}
}
