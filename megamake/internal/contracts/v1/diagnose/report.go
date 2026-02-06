package diagnose

import (
	"sort"
	"strings"

	contractartifact "github.com/megamake/megamake/internal/contracts/v1/artifact"
)

type SeverityV1 string

const (
	SeverityError   SeverityV1 = "error"
	SeverityWarning SeverityV1 = "warning"
	SeverityInfo    SeverityV1 = "info"
)

type DiagnosticV1 struct {
	Tool     string     `json:"tool"`
	Language string     `json:"language"`
	File     string     `json:"file"`
	Line     *int       `json:"line,omitempty"`
	Column   *int       `json:"column,omitempty"`
	Code     string     `json:"code,omitempty"`
	Severity SeverityV1 `json:"severity"`
	Message  string     `json:"message"`
}

type LanguageDiagnosticsV1 struct {
	Name   string         `json:"name"`
	Tool   string         `json:"tool"`
	Issues []DiagnosticV1 `json:"issues"`
}

type DiagnosticsReportV1 struct {
	Languages   []LanguageDiagnosticsV1 `json:"languages"`
	GeneratedAt string                  `json:"generatedAt"` // RFC3339Nano UTC
	Warnings    []string                `json:"warnings,omitempty"`
}

// ToXML renders pseudo-XML diagnostics output and embeds the fix prompt text.
func (r DiagnosticsReportV1) ToXML(fixPrompt string) string {
	var parts []string
	parts = append(parts, "<diagnostics generatedAt=\""+contractartifact.EscapeAttr(r.GeneratedAt)+"\">")

	for _, ld := range r.Languages {
		parts = append(parts, "  <language name=\""+contractartifact.EscapeAttr(ld.Name)+"\" tool=\""+contractartifact.EscapeAttr(ld.Tool)+"\">")
		for _, d := range ld.Issues {
			line := ""
			col := ""
			if d.Line != nil {
				line = itoa(*d.Line)
			}
			if d.Column != nil {
				col = itoa(*d.Column)
			}
			code := d.Code
			parts = append(parts,
				"    <issue file=\""+contractartifact.EscapeAttr(d.File)+"\" line=\""+contractartifact.EscapeAttr(line)+"\" column=\""+contractartifact.EscapeAttr(col)+"\" severity=\""+contractartifact.EscapeAttr(string(d.Severity))+"\" code=\""+contractartifact.EscapeAttr(code)+"\">")
			parts = append(parts, "      <![CDATA["+d.Message+"]]>")
			parts = append(parts, "    </issue>")
		}
		errs := 0
		warns := 0
		for _, d := range ld.Issues {
			if d.Severity == SeverityError {
				errs++
			}
			if d.Severity == SeverityWarning {
				warns++
			}
		}
		parts = append(parts, "    <summary count=\""+itoa(len(ld.Issues))+"\" errors=\""+itoa(errs)+"\" warnings=\""+itoa(warns)+"\" />")
		parts = append(parts, "  </language>")
	}

	totalIssues := 0
	for _, ld := range r.Languages {
		totalIssues += len(ld.Issues)
	}
	parts = append(parts, "  <summary total_languages=\""+itoa(len(r.Languages))+"\" total_issues=\""+itoa(totalIssues)+"\" />")

	if len(r.Warnings) > 0 {
		parts = append(parts, "  <warnings>")
		for _, w := range r.Warnings {
			parts = append(parts, "    <warning><![CDATA["+w+"]]></warning>")
		}
		parts = append(parts, "  </warnings>")
	}

	parts = append(parts, "  <fix_prompt>")
	parts = append(parts, "    <![CDATA["+fixPrompt+"]]>")
	parts = append(parts, "  </fix_prompt>")
	parts = append(parts, "</diagnostics>")

	return strings.Join(parts, "\n")
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

func SortedLanguageNames(r DiagnosticsReportV1) []string {
	set := map[string]bool{}
	for _, ld := range r.Languages {
		if strings.TrimSpace(ld.Name) != "" {
			set[ld.Name] = true
		}
	}
	var out []string
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
