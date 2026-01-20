package domain

import (
	"strings"
	"unicode/utf8"
)

// FileInput is a single file to include in the <context> blob.
type FileInput struct {
	RelPath string // POSIX-style relative path
	Content []byte // raw bytes, expected to be UTF-8
}

// BuildContextBlob builds the pseudo-XML <context> blob:
//
// <context>
// <rel/path/file.ext>
// <![CDATA[
// ...content...
// ]]>
// </rel/path/file.ext>
// </context>
//
// Notes:
// - This is intentionally pseudo-XML to keep it copy/paste-friendly for LLM prompts.
// - Tag names are relative paths, which is not strict XML.
// - We defensively make CDATA safe by splitting any "]]>" sequences.
func BuildContextBlob(files []FileInput) (blob string, warnings []string) {
	var b strings.Builder
	b.Grow(1024 * 32)

	b.WriteString("<context>\n")

	for _, f := range files {
		if strings.TrimSpace(f.RelPath) == "" {
			warnings = append(warnings, "skipping file with empty relpath")
			continue
		}
		if !utf8.Valid(f.Content) {
			warnings = append(warnings, "skipping non-UTF8 file: "+f.RelPath)
			continue
		}

		content := string(f.Content)
		// Ensure CDATA cannot be prematurely terminated.
		content = strings.ReplaceAll(content, "]]>", "]]]]><![CDATA[>")

		b.WriteString("<")
		b.WriteString(f.RelPath)
		b.WriteString(">\n")
		b.WriteString("<![CDATA[\n")
		b.WriteString(content)
		if !strings.HasSuffix(content, "\n") {
			b.WriteString("\n")
		}
		b.WriteString("]]>\n")
		b.WriteString("</")
		b.WriteString(f.RelPath)
		b.WriteString(">\n")
	}

	b.WriteString("</context>\n")
	return b.String(), warnings
}
