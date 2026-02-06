package ports

import contractchat "github.com/megamake/megamake/internal/contracts/v1/chat"

// RunSettingsStore reads/writes per-run settings snapshots.
//
// Path (artifact-dir scoped):
//
//	<artifactDir>/MEGACHAT/runs/<run_name>/settings.json
//
// This is separate from SettingsStore (global project settings at <artifactDir>/MEGACHAT/settings.json).
//
// Secrets must NOT be stored here.
type RunSettingsStore interface {
	ReadRunSettings(req ReadRunSettingsRequest) (settings contractchat.SettingsV1, found bool, err error)
	WriteRunSettings(req WriteRunSettingsRequest) error
}

type ReadRunSettingsRequest struct {
	ArtifactDir string
	RunName     string
}

type WriteRunSettingsRequest struct {
	ArtifactDir string
	RunName     string
	Settings    contractchat.SettingsV1
}
