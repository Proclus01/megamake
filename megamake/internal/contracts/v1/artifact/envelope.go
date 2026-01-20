package artifact

import "strings"

// ArtifactEnvelopeV1 is the unified artifact envelope stored in MEGA* .txt files.
// It embeds three human- and machine-consumable views:
// - XML: pseudo-XML report (canonical/human)
// - JSON: machine JSON report
// - Prompt: agent-facing instruction text
type ArtifactEnvelopeV1 struct {
	Meta   ArtifactMetaV1
	XML    string
	JSON   string
	Prompt string
}

// Render returns a pseudo-XML-ish envelope suitable for storing in a .txt artifact.
// This is intentionally not strict XML, but is structured to be readable and robust.
func (e ArtifactEnvelopeV1) Render() string {
	var b strings.Builder

	tool := EscapeAttr(e.Meta.Tool)
	contract := EscapeAttr(e.Meta.Contract)
	generatedAt := EscapeAttr(e.Meta.GeneratedAt)

	b.WriteString("<megamake_artifact tool=\"")
	b.WriteString(tool)
	b.WriteString("\" contract=\"")
	b.WriteString(contract)
	b.WriteString("\" generatedAt=\"")
	b.WriteString(generatedAt)
	b.WriteString("\">\n")

	// XML block
	b.WriteString("  <xml><![CDATA[\n")
	if e.XML != "" {
		b.WriteString(e.XML)
		if !strings.HasSuffix(e.XML, "\n") {
			b.WriteString("\n")
		}
	}
	b.WriteString("  ]]></xml>\n\n")

	// JSON block
	b.WriteString("  <json><![CDATA[\n")
	if e.JSON != "" {
		b.WriteString(e.JSON)
		if !strings.HasSuffix(e.JSON, "\n") {
			b.WriteString("\n")
		}
	}
	b.WriteString("  ]]></json>\n\n")

	// Prompt block
	b.WriteString("  <prompt><![CDATA[\n")
	if e.Prompt != "" {
		b.WriteString(e.Prompt)
		if !strings.HasSuffix(e.Prompt, "\n") {
			b.WriteString("\n")
		}
	}
	b.WriteString("  ]]></prompt>\n")

	b.WriteString("</megamake_artifact>\n")
	return b.String()
}
