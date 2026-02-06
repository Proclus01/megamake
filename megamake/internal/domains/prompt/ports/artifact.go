package ports

import (
	"time"

	contractartifact "github.com/megamake/megamake/internal/contracts/v1/artifact"
)

// WriteArtifactRequest is a tool-agnostic request for writing a .txt artifact and latest pointer.
type WriteArtifactRequest struct {
	ArtifactDir    string
	ToolPrefix     string
	Envelope       contractartifact.ArtifactEnvelopeV1
	GeneratedAtUTC *time.Time
}

type ArtifactWriter interface {
	WriteToolArtifact(req WriteArtifactRequest) (artifactPath string, latestPointerPath string, err error)
}
