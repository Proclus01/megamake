package doc

import (
	"sort"
	"strings"

	contractartifact "github.com/megamake/megamake/internal/contracts/v1/artifact"
)

type DocModeV1 string

const (
	DocModeLocal DocModeV1 = "local"
	DocModeFetch DocModeV1 = "fetch"
)

type DocImportV1 struct {
	File         string `json:"file"`     // POSIX relpath
	Language     string `json:"language"`
	Raw          string `json:"raw"`
	IsInternal   bool   `json:"isInternal"`
	ResolvedPath string `json:"resolvedPath,omitempty"` // POSIX relpath if internal resolution succeeded
}

type FetchedDocV1 struct {
	URI            string `json:"uri"`
	Title          string `json:"title"`
	ContentPreview string `json:"contentPreview"`
}

// DocReportV1 is the contract for MegaDoc outputs.
// It mirrors the Swift report shape but uses stable POSIX relpaths for internal references.
type DocReportV1 struct {
	GeneratedAt           string         `json:"generatedAt"` // RFC3339Nano UTC
	Mode                  DocModeV1      `json:"mode"`
	RootPath              string         `json:"rootPath"`
	Languages             []string       `json:"languages"`
	DirectoryTree         string         `json:"directoryTree"`
	ImportGraph           string         `json:"importGraph"`
	Imports               []DocImportV1  `json:"imports"`
	ExternalDependencies  map[string]int `json:"externalDependencies"`
	PurposeSummary        string         `json:"purposeSummary"`
	FetchedDocs           []FetchedDocV1 `json:"fetchedDocs"`
	UMLASCII              string         `json:"umlAscii,omitempty"`
	UMLPlantUML           string         `json:"umlPlantUML,omitempty"`
	Warnings              []string       `json:"warnings,omitempty"`
}

// ToXML renders the report as pseudo-XML for stdout.
// This is intentionally "XML-like" (human friendly), not strict XML.
func (r DocReportV1) ToXML() string {
	var parts []string
	parts = append(parts, "<documentation generatedAt=\""+contractartifact.EscapeAttr(r.GeneratedAt)+"\" mode=\""+contractartifact.EscapeAttr(string(r.Mode))+"\">")

	if strings.TrimSpace(r.RootPath) != "" {
		parts = append(parts, "  <root><![CDATA["+r.RootPath+"]]></root>")
	}

	if len(r.Languages) > 0 {
		parts = append(parts, "  <languages>")
		for _, l := range r.Languages {
			parts = append(parts, "    <language name=\""+contractartifact.EscapeAttr(l)+"\"/>")
		}
		parts = append(parts, "  </languages>")
	}

	parts = append(parts, "  <directory_tree><![CDATA[\n"+r.DirectoryTree+"\n]]></directory_tree>")
	parts = append(parts, "  <import_graph><![CDATA[\n"+r.ImportGraph+"\n]]></import_graph>")

	if strings.TrimSpace(r.UMLASCII) != "" {
		parts = append(parts, "  <uml_ascii><![CDATA[\n"+r.UMLASCII+"\n]]></uml_ascii>")
	}
	if strings.TrimSpace(r.UMLPlantUML) != "" {
		parts = append(parts, "  <uml_plantuml><![CDATA[\n"+r.UMLPlantUML+"\n]]></uml_plantuml>")
	}

	if len(r.Imports) > 0 {
		parts = append(parts, "  <imports>")
		for _, i := range r.Imports {
			parts = append(parts, "    <import file=\""+contractartifact.EscapeAttr(i.File)+"\" language=\""+contractartifact.EscapeAttr(i.Language)+"\" internal=\""+boolAttr(i.IsInternal)+"\">")
			parts = append(parts, "      <raw><![CDATA["+i.Raw+"]]></raw>")
			if strings.TrimSpace(i.ResolvedPath) != "" {
				parts = append(parts, "      <resolved_path><![CDATA["+i.ResolvedPath+"]]></resolved_path>")
			}
			parts = append(parts, "    </import>")
		}
		parts = append(parts, "  </imports>")
	}

	if len(r.ExternalDependencies) > 0 {
		parts = append(parts, "  <external_dependencies>")
		keys := make([]string, 0, len(r.ExternalDependencies))
		for k := range r.ExternalDependencies {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, dep := range keys {
			cnt := r.ExternalDependencies[dep]
			parts = append(parts, "    <dep name=\""+contractartifact.EscapeAttr(dep)+"\" count=\""+itoa(cnt)+"\"/>")
		}
		parts = append(parts, "  </external_dependencies>")
	}

	parts = append(parts, "  <purpose><![CDATA["+r.PurposeSummary+"]]></purpose>")

	if len(r.FetchedDocs) > 0 {
		parts = append(parts, "  <fetched_docs>")
		for _, d := range r.FetchedDocs {
			parts = append(parts, "    <doc uri=\""+contractartifact.EscapeAttr(d.URI)+"\" title=\""+contractartifact.EscapeAttr(d.Title)+"\">")
			parts = append(parts, "      <preview><![CDATA["+d.ContentPreview+"]]></preview>")
			parts = append(parts, "    </doc>")
		}
		parts = append(parts, "  </fetched_docs>")
	}

	if len(r.Warnings) > 0 {
		parts = append(parts, "  <warnings>")
		for _, w := range r.Warnings {
			parts = append(parts, "    <warning><![CDATA["+w+"]]></warning>")
		}
		parts = append(parts, "  </warnings>")
	}

	parts = append(parts, "</documentation>")
	return strings.Join(parts, "\n")
}

func boolAttr(v bool) string {
	if v {
		return "true"
	}
	return "false"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	sign := ""
	if n < 0 {
		sign = "-"
		n = -n
	}
	var buf [32]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + (n % 10))
		n /= 10
	}
	return sign + string(buf[i:])
}
