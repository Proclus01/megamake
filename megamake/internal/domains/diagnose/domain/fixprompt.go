package domain

import (
	"sort"
	"strings"

	contract "github.com/megamake/megamake/internal/contracts/v1/diagnose"
)

func GenerateFixPrompt(report contract.DiagnosticsReportV1, rootPath string) string {
	var lines []string
	lines = append(lines, "You are an expert software engineer. Apply fixes across the project to resolve the following diagnostics.")
	lines = append(lines, "")
	lines = append(lines, "Context:")
	langs := contract.SortedLanguageNames(report)
	lines = append(lines, "- Languages analyzed: "+strings.Join(langs, ", "))

	total := 0
	errs := 0
	warns := 0
	for _, ld := range report.Languages {
		total += len(ld.Issues)
		for _, d := range ld.Issues {
			if d.Severity == contract.SeverityError {
				errs++
			}
			if d.Severity == contract.SeverityWarning {
				warns++
			}
		}
	}
	lines = append(lines, "- Total issues: "+itoa(total)+" ("+itoa(errs)+" errors, "+itoa(warns)+" warnings)")
	lines = append(lines, "")
	lines = append(lines, "Top issues by language:")

	for _, ld := range report.Languages {
		e := 0
		w := 0
		for _, d := range ld.Issues {
			if d.Severity == contract.SeverityError {
				e++
			}
			if d.Severity == contract.SeverityWarning {
				w++
			}
		}
		lines = append(lines, "- "+ld.Name+": "+itoa(e)+" errors, "+itoa(w)+" warnings")
		limit := 5
		if len(ld.Issues) < limit {
			limit = len(ld.Issues)
		}
		for i := 0; i < limit; i++ {
			d := ld.Issues[i]
			loc := locationString(d, rootPath)
			code := strings.TrimSpace(d.Code)
			if code != "" {
				code = " " + code
			}
			lines = append(lines, "  • "+loc+code+": "+d.Message)
		}
		if len(ld.Issues) > 5 {
			lines = append(lines, "  • ... "+itoa(len(ld.Issues)-5)+" more")
		}
	}

	lines = append(lines, "")
	lines = append(lines, "Instructions:")
	lines = append(lines, "- Produce minimal, correct fixes for each issue.")
	lines = append(lines, "- Maintain existing architecture and conventions.")
	lines = append(lines, "- Include tests or adjustments to tests as needed.")
	lines = append(lines, "- If a tool was unavailable, suggest installation steps.")
	lines = append(lines, "")
	lines = append(lines, "Return patches as a set of unified diffs or a patch.sh script that overwrites the relevant files using heredocs with single-quoted EOF delimiters.")
	return strings.Join(lines, "\n")
}

func locationString(d contract.DiagnosticV1, rootPath string) string {
	path := d.File
	if strings.TrimSpace(rootPath) != "" {
		// Best-effort relativization: if file begins with rootPath, strip it.
		rp := strings.TrimSuffix(rootPath, "/") + "/"
		if strings.HasPrefix(path, rp) {
			path = strings.TrimPrefix(path, rp)
		}
	}
	var parts []string
	parts = append(parts, path)
	if d.Line != nil {
		parts = append(parts, itoa(*d.Line))
	}
	if d.Column != nil {
		parts = append(parts, itoa(*d.Column))
	}
	return strings.Join(parts, ":")
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

func SortedIssuesByFile(issues []contract.DiagnosticV1) []contract.DiagnosticV1 {
	out := append([]contract.DiagnosticV1(nil), issues...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].File == out[j].File {
			li := 0
			lj := 0
			if out[i].Line != nil {
				li = *out[i].Line
			}
			if out[j].Line != nil {
				lj = *out[j].Line
			}
			return li < lj
		}
		return out[i].File < out[j].File
	})
	return out
}
