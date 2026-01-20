package adapters

import (
	artifactwriter "github.com/megamake/megamake/internal/platform/artifact"

	"github.com/megamake/megamake/internal/domains/diagnose/ports"
)

type PlatformArtifactWriter struct {
	Writer artifactwriter.Writer
}

func NewPlatformArtifactWriter(w artifactwriter.Writer) PlatformArtifactWriter {
	return PlatformArtifactWriter{Writer: w}
}

func (p PlatformArtifactWriter) WriteToolArtifact(req ports.WriteArtifactRequest) (string, string, error) {
	return p.Writer.WriteToolArtifact(artifactwriter.WriteRequest{
		ArtifactDir:    req.ArtifactDir,
		ToolPrefix:     req.ToolPrefix,
		Envelope:       req.Envelope,
		GeneratedAtUTC: req.GeneratedAtUTC,
	})
}
