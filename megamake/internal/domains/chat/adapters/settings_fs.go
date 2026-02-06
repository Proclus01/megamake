package adapters

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	contractchat "github.com/megamake/megamake/internal/contracts/v1/chat"
	"github.com/megamake/megamake/internal/domains/chat/ports"
)

// FSSettingsStore persists chat settings to the artifact-dir scoped path:
//
//	<artifactDir>/MEGACHAT/settings.json
//
// Secrets (API keys) must NOT be stored here.
type FSSettingsStore struct{}

func NewFSSettingsStore() FSSettingsStore { return FSSettingsStore{} }

func (FSSettingsStore) Read(req ports.ReadSettingsRequest) (contractchat.SettingsV1, bool, error) {
	artifactDir := strings.TrimSpace(req.ArtifactDir)
	if artifactDir == "" {
		return contractchat.SettingsV1{}, false, fmt.Errorf("chat settings: ArtifactDir is empty")
	}

	p := filepath.Join(artifactDir, "MEGACHAT", "settings.json")
	b, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return contractchat.SettingsV1{}, false, nil
		}
		return contractchat.SettingsV1{}, false, fmt.Errorf("chat settings: failed to read settings.json: %v", err)
	}

	var s contractchat.SettingsV1
	if err := json.Unmarshal(b, &s); err != nil {
		return contractchat.SettingsV1{}, false, fmt.Errorf("chat settings: failed to parse settings.json: %v", err)
	}

	return s, true, nil
}

func (FSSettingsStore) Write(req ports.WriteSettingsRequest) error {
	artifactDir := strings.TrimSpace(req.ArtifactDir)
	if artifactDir == "" {
		return fmt.Errorf("chat settings: ArtifactDir is empty")
	}

	// Best-effort: ensure UpdatedTS is present for auditability.
	if strings.TrimSpace(req.Settings.UpdatedTS) == "" {
		req.Settings.UpdatedTS = time.Now().UTC().Format(time.RFC3339Nano)
	}

	p := filepath.Join(artifactDir, "MEGACHAT", "settings.json")
	return writeJSONAtomicSettings(p, req.Settings)
}

func writeJSONAtomicSettings(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("chat settings: failed to marshal json for %s: %v", path, err)
	}
	if len(b) == 0 || b[len(b)-1] != '\n' {
		b = append(b, '\n')
	}
	return writeFileAtomicSettings(path, b, 0o644)
}

func writeFileAtomicSettings(path string, data []byte, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("chat settings: failed to ensure dir for %s: %v", path, err)
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, perm); err != nil {
		return fmt.Errorf("chat settings: failed to write temp file %s: %v", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("chat settings: failed to rename temp file to %s: %v", path, err)
	}
	return nil
}
