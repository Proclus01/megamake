package app

import (
	"fmt"
	"strings"
	"time"

	contractartifact "github.com/megamake/megamake/internal/contracts/v1/artifact"
	contractchat "github.com/megamake/megamake/internal/contracts/v1/chat"
	"github.com/megamake/megamake/internal/domains/chat/domain"
	"github.com/megamake/megamake/internal/domains/chat/ports"
)

// GetRunSettingsRequest loads the effective settings for a run.
//
// Semantics:
// - If per-run settings.json exists => return it (Found=true).
// - Else fall back to global settings.json => return it (Found=false, Source="global").
// - Else return defaultSettings() (Found=false, Source="default").
type GetRunSettingsRequest struct {
	ArtifactDir string
	RunName     string
}

type GetRunSettingsResult struct {
	RunName  string                  `json:"run_name"`
	Settings contractchat.SettingsV1 `json:"settings"`
	Found    bool                    `json:"found"`  // true if per-run settings existed
	Source   string                  `json:"source"` // "run" | "global" | "default"
}

type SetRunSettingsRequest struct {
	ArtifactDir string
	RunName     string
	Settings    contractchat.SettingsV1
}

type SetRunSettingsResult struct {
	RunName string `json:"run_name"`
	OK      bool   `json:"ok"`
}

// GetRunSettings returns run settings (if any), else falls back to global settings, else defaults.
func (s *Service) GetRunSettings(req GetRunSettingsRequest) (GetRunSettingsResult, error) {
	if s.RunSettings == nil {
		return GetRunSettingsResult{}, fmt.Errorf("internal error: chat RunSettings store is nil")
	}
	artifactDir := strings.TrimSpace(req.ArtifactDir)
	if artifactDir == "" {
		artifactDir = "."
	}
	runName := strings.TrimSpace(req.RunName)
	if runName == "" {
		return GetRunSettingsResult{}, fmt.Errorf("chat run settings get: run_name is required")
	}
	if !domain.IsValidRunName(runName) {
		return GetRunSettingsResult{}, fmt.Errorf("chat run settings get: invalid run_name: %s", runName)
	}

	// 1) per-run
	rs, found, err := s.RunSettings.ReadRunSettings(ports.ReadRunSettingsRequest{
		ArtifactDir: artifactDir,
		RunName:     runName,
	})
	if err != nil {
		return GetRunSettingsResult{}, err
	}
	if found {
		rs = normalizeSettings(rs)
		return GetRunSettingsResult{RunName: runName, Settings: rs, Found: true, Source: "run"}, nil
	}

	// 2) global settings
	if s.Settings != nil {
		gs, gfound, _ := s.Settings.Read(ports.ReadSettingsRequest{ArtifactDir: artifactDir})
		if gfound {
			gs = normalizeSettings(gs)
			return GetRunSettingsResult{RunName: runName, Settings: gs, Found: false, Source: "global"}, nil
		}
	}

	// 3) defaults
	def := normalizeSettings(defaultSettings())
	return GetRunSettingsResult{RunName: runName, Settings: def, Found: false, Source: "default"}, nil
}

// SetRunSettings writes per-run settings.json for a run (full replace) AND keeps meta.json consistent.
//
// Policy / semantics:
//   - Provider/Model/SystemText/DeveloperText are treated as optional overrides.
//   - If a field is non-empty in Settings, we also copy it into meta.json.
//     This makes the run list/header consistent with “what you configured”.
//   - If a field is empty, we do NOT clear meta; meta remains as-is.
func (s *Service) SetRunSettings(req SetRunSettingsRequest) (SetRunSettingsResult, error) {
	if s.RunSettings == nil {
		return SetRunSettingsResult{}, fmt.Errorf("internal error: chat RunSettings store is nil")
	}
	if s.Store == nil {
		return SetRunSettingsResult{}, fmt.Errorf("internal error: chat Store is nil (needed to validate run exists and update meta)")
	}

	artifactDir := strings.TrimSpace(req.ArtifactDir)
	if artifactDir == "" {
		artifactDir = "."
	}
	runName := strings.TrimSpace(req.RunName)
	if runName == "" {
		return SetRunSettingsResult{}, fmt.Errorf("chat run settings set: run_name is required")
	}
	if !domain.IsValidRunName(runName) {
		return SetRunSettingsResult{}, fmt.Errorf("chat run settings set: invalid run_name: %s", runName)
	}

	// Ensure run exists by reading meta first.
	meta, err := s.Store.ReadMeta(ports.ReadMetaRequest{
		ArtifactDir: artifactDir,
		RunName:     runName,
	})
	if err != nil {
		return SetRunSettingsResult{}, err
	}

	sv := normalizeSettings(req.Settings)

	now := time.Now().UTC()
	nowTS := contractartifact.FormatRFC3339NanoUTC(now)

	// Best-effort UpdatedTS for auditability.
	if strings.TrimSpace(sv.UpdatedTS) == "" {
		sv.UpdatedTS = nowTS
	}

	// Write per-run settings snapshot
	if err := s.RunSettings.WriteRunSettings(ports.WriteRunSettingsRequest{
		ArtifactDir: artifactDir,
		RunName:     runName,
		Settings:    sv,
	}); err != nil {
		return SetRunSettingsResult{}, err
	}

	// Keep meta consistent with non-empty overrides.
	//
	// This makes the UI list/header show the current configured provider/model/system/developer.
	meta.UpdatedTS = nowTS

	if strings.TrimSpace(sv.Provider) != "" {
		meta.Provider = strings.TrimSpace(sv.Provider)
	}
	if strings.TrimSpace(sv.Model) != "" {
		meta.Model = strings.TrimSpace(sv.Model)
	}
	if strings.TrimSpace(sv.SystemText) != "" {
		meta.SystemText = sv.SystemText
	}
	if strings.TrimSpace(sv.DeveloperText) != "" {
		meta.DeveloperText = sv.DeveloperText
	}

	// Also clear last_error on explicit settings save (optional, but feels nicer in UI).
	// (If you prefer to keep last_error until next successful turn, remove this.)
	// meta.LastError = ""

	if err := s.Store.WriteMeta(ports.WriteMetaRequest{
		ArtifactDir: artifactDir,
		RunName:     runName,
		Meta:        meta,
	}); err != nil {
		return SetRunSettingsResult{}, err
	}

	return SetRunSettingsResult{RunName: runName, OK: true}, nil
}

func normalizeSettings(s contractchat.SettingsV1) contractchat.SettingsV1 {
	// IMPORTANT:
	// For per-run settings, Provider/Model are optional overrides.
	// If empty, they should stay empty so run meta / global settings can remain authoritative.
	//
	// We only normalize "behavior" fields so they are never empty/invalid.
	if s.TextFormat == "" {
		s.TextFormat = contractchat.TextFormatText
	}
	if s.Verbosity == "" {
		s.Verbosity = contractchat.VerbosityHigh
	}
	if s.Effort == "" {
		s.Effort = contractchat.EffortHigh
	}
	if s.MaxOutputTokens <= 0 {
		s.MaxOutputTokens = 999999
	}
	return s
}
