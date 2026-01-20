package prompt

import (
	project "github.com/megamake/megamake/internal/contracts/v1/project"
)

// PromptReportV1 is the v1 contract for MegaPrompt output.
// The full <context> blob may be large; this report provides a structured inventory.
type PromptReportV1 struct {
	GeneratedAt string                 `json:"generatedAt"` // RFC3339Nano UTC
	RootPath    string                 `json:"rootPath"`
	Profile     project.ProjectProfileV1 `json:"profile"`

	FilesScanned  int               `json:"filesScanned"`
	FilesIncluded int               `json:"filesIncluded"`
	TotalBytes    int64             `json:"totalBytes"`
	Files         []project.FileRefV1 `json:"files"`

	Warnings []string `json:"warnings,omitempty"`
}
