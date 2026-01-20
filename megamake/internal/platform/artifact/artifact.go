package artifact

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/megamake/megamake/internal/contracts/v1/artifact"
	"github.com/megamake/megamake/internal/platform/clock"
	"github.com/megamake/megamake/internal/platform/errors"
)

type WriteRequest struct {
	ArtifactDir string
	ToolPrefix  string // e.g., "MEGAPROMPT", "MEGADOC", "MEGADIAG", "MEGATEST"
	Envelope    artifact.ArtifactEnvelopeV1

	// GeneratedAtUTC, if provided, is used for the artifact filename timestamp.
	// This helps keep filenames aligned with the envelope meta timestamps.
	GeneratedAtUTC *time.Time
}

type Writer struct {
	Clock clock.Clock
}

// WriteToolArtifact writes:
// 1) <ToolPrefix>_YYYYMMDD_HHMMSSZ.txt in ArtifactDir
// 2) <ToolPrefix>_latest.txt pointer file containing the latest artifact filename (one line)
//
// NOTE: Uses UTC timestamps and pointer files (no symlinks) for cross-platform reliability.
func (w Writer) WriteToolArtifact(req WriteRequest) (artifactPath string, latestPointerPath string, err error) {
	if w.Clock == nil {
		return "", "", errors.NewInternal("artifact writer clock is nil", nil)
	}
	if strings.TrimSpace(req.ArtifactDir) == "" {
		return "", "", errors.NewInternal("artifactDir is empty", nil)
	}
	if strings.TrimSpace(req.ToolPrefix) == "" {
		return "", "", errors.NewInternal("toolPrefix is empty", nil)
	}

	now := w.Clock.NowUTC()
	if req.GeneratedAtUTC != nil {
		now = req.GeneratedAtUTC.UTC()
	}

	filename := fmt.Sprintf("%s_%s.txt", req.ToolPrefix, formatUTCForFilename(now))
	fullPath := filepath.Join(req.ArtifactDir, filename)

	if err := os.MkdirAll(req.ArtifactDir, 0o755); err != nil {
		return "", "", errors.New(errors.KindIO, "failed to create artifact directory", err)
	}

	payload := req.Envelope.Render()
	if err := os.WriteFile(fullPath, []byte(payload), 0o644); err != nil {
		return "", "", errors.New(errors.KindIO, "failed to write artifact file", err)
	}

	latestName := fmt.Sprintf("%s_latest.txt", req.ToolPrefix)
	latestPath := filepath.Join(req.ArtifactDir, latestName)
	if err := os.WriteFile(latestPath, []byte(filename+"\n"), 0o644); err != nil {
		return "", "", errors.New(errors.KindIO, "failed to write latest pointer file", err)
	}

	return fullPath, latestPath, nil
}

func formatUTCForFilename(t time.Time) string {
	// Example: 20260120_154233Z
	return t.UTC().Format("20060102_150405Z")
}
