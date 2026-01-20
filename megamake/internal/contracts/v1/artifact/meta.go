package artifact

import (
	"strings"
	"time"
)

// ArtifactMetaV1 is the shared metadata header for all tools.
type ArtifactMetaV1 struct {
	Tool         string   `json:"tool"`
	Contract     string   `json:"contract"`
	GeneratedAt  string   `json:"generatedAt"` // RFC3339Nano UTC
	RootPath     string   `json:"rootPath"`
	Args         []string `json:"args,omitempty"`
	NetEnabled   bool     `json:"netEnabled"`
	AllowDomains []string `json:"allowDomains,omitempty"`
	Warnings     []string `json:"warnings,omitempty"`
}

func FormatRFC3339NanoUTC(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

// EscapeAttr escapes a string for safe use inside pseudo-XML attributes.
func EscapeAttr(s string) string {
	repl := strings.NewReplacer(
		"&", "&amp;",
		"\"", "&quot;",
		"<", "&lt;",
		">", "&gt;",
	)
	return repl.Replace(s)
}
