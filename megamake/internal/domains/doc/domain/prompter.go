package domain

import (
	"sort"
	"strings"

	contractdoc "github.com/megamake/megamake/internal/contracts/v1/doc"
)

// GenerateDocPrompt produces an agent-oriented prompt with structure, deps, UML, and/or fetched docs.
func GenerateDocPrompt(r contractdoc.DocReportV1) string {
	var lines []string
	lines = append(lines, "You are a documentation-aware agent. Use the structure and imports to understand this codebase and/or the fetched docs.")
	lines = append(lines, "")
	lines = append(lines, "Mode: "+string(r.Mode))

	if len(r.Languages) > 0 {
		langs := append([]string(nil), r.Languages...)
		sort.Strings(langs)
		lines = append(lines, "Languages: "+strings.Join(langs, ", "))
	}

	if r.Mode == contractdoc.DocModeLocal {
		lines = append(lines, "")
		lines = append(lines, "Directory tree:")
		lines = append(lines, r.DirectoryTree)
		lines = append(lines, "")
		lines = append(lines, "Import/dependency graph:")
		lines = append(lines, r.ImportGraph)

		if strings.TrimSpace(r.UMLASCII) != "" {
			lines = append(lines, "")
			lines = append(lines, "UML (ASCII):")
			lines = append(lines, r.UMLASCII)
		}

		if len(r.ExternalDependencies) > 0 {
			lines = append(lines, "")
			lines = append(lines, "External dependencies (approximate):")
			keys := make([]string, 0, len(r.ExternalDependencies))
			for k := range r.ExternalDependencies {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, dep := range keys {
				lines = append(lines, "  - "+dep+": "+itoa(r.ExternalDependencies[dep])+" reference(s)")
			}
		}

		lines = append(lines, "")
		lines = append(lines, "Purpose summary:")
		lines = append(lines, r.PurposeSummary)

		lines = append(lines, "")
		lines = append(lines, "Instructions:")
		lines = append(lines, "- Extract architectural overview, key modules, and responsibilities.")
		lines = append(lines, "- Use UML to identify entrypoints, service boundaries, and data sources.")
		lines = append(lines, "- Relate external dependencies to specific modules and features.")
		lines = append(lines, "- Return a concise outline plus follow-up questions if crucial information is missing.")
		return strings.Join(lines, "\n")
	}

	// Fetch mode
	lines = append(lines, "")
	lines = append(lines, "Fetched docs:")
	if len(r.FetchedDocs) == 0 {
		lines = append(lines, "(none)")
	} else {
		for i, d := range r.FetchedDocs {
			if i >= 12 {
				break
			}
			lines = append(lines, "- "+d.Title+" ["+d.URI+"]")
		}
		if len(r.FetchedDocs) > 12 {
			lines = append(lines, "... "+itoa(len(r.FetchedDocs)-12)+" more")
		}
	}

	lines = append(lines, "")
	lines = append(lines, "Summary:")
	lines = append(lines, r.PurposeSummary)

	lines = append(lines, "")
	lines = append(lines, "Instructions:")
	lines = append(lines, "- Summarize the fetched docs and explain their relevance to the requested topic.")
	lines = append(lines, "- If the docs are incomplete, identify missing areas and suggest next URIs to fetch (respecting network policy).")
	lines = append(lines, "- Return a concise outline plus follow-up questions if crucial information is missing.")
	return strings.Join(lines, "\n")
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
