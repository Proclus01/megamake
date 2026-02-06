package adapters

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	contractchat "github.com/megamake/megamake/internal/contracts/v1/chat"
	"github.com/megamake/megamake/internal/domains/chat/domain"
	"github.com/megamake/megamake/internal/domains/chat/ports"
)

// FSRunSettingsStore reads/writes per-run settings snapshots:
//
//	<artifactDir>/MEGACHAT/runs/<run_name>/settings.json
//
// Secrets must NOT be stored here.
type FSRunSettingsStore struct{}

func NewFSRunSettingsStore() FSRunSettingsStore { return FSRunSettingsStore{} }

func (FSRunSettingsStore) ReadRunSettings(req ports.ReadRunSettingsRequest) (contractchat.SettingsV1, bool, error) {
	artifactDir := strings.TrimSpace(req.ArtifactDir)
	runName := strings.TrimSpace(req.RunName)
	if artifactDir == "" {
		return contractchat.SettingsV1{}, false, fmt.Errorf("chat run settings: ArtifactDir is empty")
	}
	if runName == "" {
		return contractchat.SettingsV1{}, false, fmt.Errorf("chat run settings: RunName is empty")
	}
	if !domain.IsValidRunName(runName) {
		return contractchat.SettingsV1{}, false, fmt.Errorf("chat run settings: invalid run_name: %s", runName)
	}

	p := filepath.Join(artifactDir, "MEGACHAT", "runs", runName, "settings.json")
	b, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return contractchat.SettingsV1{}, false, nil
		}
		return contractchat.SettingsV1{}, false, fmt.Errorf("chat run settings: failed to read settings.json: %v", err)
	}

	var s contractchat.SettingsV1
	if err := json.Unmarshal(b, &s); err != nil {
		return contractchat.SettingsV1{}, false, fmt.Errorf("chat run settings: failed to parse settings.json: %v", err)
	}
	return s, true, nil
}

func (FSRunSettingsStore) WriteRunSettings(req ports.WriteRunSettingsRequest) error {
	artifactDir := strings.TrimSpace(req.ArtifactDir)
	runName := strings.TrimSpace(req.RunName)
	if artifactDir == "" {
		return fmt.Errorf("chat run settings: ArtifactDir is empty")
	}
	if runName == "" {
		return fmt.Errorf("chat run settings: RunName is empty")
	}
	if !domain.IsValidRunName(runName) {
		return fmt.Errorf("chat run settings: invalid run_name: %s", runName)
	}

	// Best-effort: ensure UpdatedTS is present for auditability.
	if strings.TrimSpace(req.Settings.UpdatedTS) == "" {
		req.Settings.UpdatedTS = time.Now().UTC().Format(time.RFC3339Nano)
	}

	p := filepath.Join(artifactDir, "MEGACHAT", "runs", runName, "settings.json")
	return writeJSONAtomicRunSettings(p, req.Settings)
}

func writeJSONAtomicRunSettings(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("chat run settings: failed to marshal json for %s: %v", path, err)
	}
	if len(b) == 0 || b[len(b)-1] != '\n' {
		b = append(b, '\n')
	}
	return writeFileAtomicRunSettings(path, b, 0o644)
}

func writeFileAtomicRunSettings(path string, data []byte, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("chat run settings: failed to ensure dir for %s: %v", path, err)
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, perm); err != nil {
		return fmt.Errorf("chat run settings: failed to write temp file %s: %v", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("chat run settings: failed to rename temp file to %s: %v", path, err)
	}
	return nil
}
