package ports

import (
	contractchat "github.com/megamake/megamake/internal/contracts/v1/chat"
)

// SettingsStore persists non-secret settings for chat.
//
// By your decision, settings are artifact-dir scoped (project-local), stored at:
//
//	<artifactDir>/MEGACHAT/settings.json
//
// Secrets (API keys) must not be stored here.
type SettingsStore interface {
	Read(req ReadSettingsRequest) (contractchat.SettingsV1, bool, error)
	Write(req WriteSettingsRequest) error
}

type ReadSettingsRequest struct {
	ArtifactDir string
}

type WriteSettingsRequest struct {
	ArtifactDir string
	Settings    contractchat.SettingsV1
}
