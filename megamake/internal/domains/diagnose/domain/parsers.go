package domain

import (
	"regexp"
	"strings"

	contract "github.com/megamake/megamake/internal/contracts/v1/diagnose"
)

func ParseSwift(stdout string, stderr string) []contract.DiagnosticV1 {
	tool := "swift build"
	var out []contract.DiagnosticV1
	combined := stdout + "\n" + stderr
	re := regexp.MustCompile(`(?m)^(.+?):(\d+):(\d+):\s+(error|warning):\s+(.+)$`)
	matches := re.FindAllStringSubmatch(combined, -1)
	for _, m := range matches {
		if len(m) < 6 {
			continue
		}
		file := m[1]
		line := atoiPtr(m[2])
		col := atoiPtr(m[3])
		sev := contract.SeverityError
		if strings.ToLower(m[4]) == "warning" {
			sev = contract.SeverityWarning
		}
		msg := m[5]
		out = append(out, contract.DiagnosticV1{
			Tool:     tool,
			Language: "swift",
			File:     file,
			Line:     line,
			Column:   col,
			Code:     "",
			Severity: sev,
			Message:  msg,
		})
	}
	return out
}

func ParseTypeScript(stdout string, stderr string, language string, tool string) []contract.DiagnosticV1 {
	var out []contract.DiagnosticV1
	combined := stdout + "\n" + stderr
	// Accept .ts/.tsx/.js/.jsx outputs.
	re := regexp.MustCompile(`(?m)^(.+?\.(?:ts|tsx|js|jsx|mjs|cjs)):(\d+):(\d+)\s*-\s*(error|warning)\s*TS(\d+):\s*(.+)$`)
	matches := re.FindAllStringSubmatch(combined, -1)
	for _, m := range matches {
		if len(m) < 7 {
			continue
		}
		file := m[1]
		line := atoiPtr(m[2])
		col := atoiPtr(m[3])
		sev := contract.SeverityError
		if strings.ToLower(m[4]) == "warning" {
			sev = contract.SeverityWarning
		}
		code := "TS" + m[5]
		msg := m[6]
		out = append(out, contract.DiagnosticV1{
			Tool:     tool,
			Language: language,
			File:     file,
			Line:     line,
			Column:   col,
			Code:     code,
			Severity: sev,
			Message:  msg,
		})
	}
	return out
}

func ParseGo(stdout string, stderr string) []contract.DiagnosticV1 {
	tool := "go build"
	var out []contract.DiagnosticV1
	combined := stdout + "\n" + stderr
	re := regexp.MustCompile(`(?m)^(.+?\.go):(\d+)(?::(\d+))?:\s+(.*)$`)
	matches := re.FindAllStringSubmatch(combined, -1)
	for _, m := range matches {
		if len(m) < 5 {
			continue
		}
		file := m[1]
		line := atoiPtr(m[2])
		col := atoiPtr(m[3])
		msg := m[4]
		out = append(out, contract.DiagnosticV1{
			Tool:     tool,
			Language: "go",
			File:     file,
			Line:     line,
			Column:   col,
			Code:     "",
			Severity: contract.SeverityError,
			Message:  msg,
		})
	}
	return out
}

func ParseJava(stdout string, stderr string) []contract.DiagnosticV1 {
	tool := "javac/maven"
	var out []contract.DiagnosticV1
	combined := stdout + "\n" + stderr
	re := regexp.MustCompile(`(?m)^(.+?\.java):(\d+):\s+(error|warning):\s+(.+)$`)
	matches := re.FindAllStringSubmatch(combined, -1)
	for _, m := range matches {
		if len(m) < 5 {
			continue
		}
		file := m[1]
		line := atoiPtr(m[2])
		sev := contract.SeverityError
		if strings.ToLower(m[3]) == "warning" {
			sev = contract.SeverityWarning
		}
		msg := m[4]
		out = append(out, contract.DiagnosticV1{
			Tool:     tool,
			Language: "java",
			File:     file,
			Line:     line,
			Column:   nil,
			Code:     "",
			Severity: sev,
			Message:  msg,
		})
	}
	return out
}

func ParseLean(stdout string, stderr string) []contract.DiagnosticV1 {
	tool := "lake build"
	var out []contract.DiagnosticV1
	combined := stdout + "\n" + stderr
	re := regexp.MustCompile(`(?m)^(.+?\.lean):(\d+):(\d+):\s+(error|warning):\s+(.+)$`)
	matches := re.FindAllStringSubmatch(combined, -1)
	for _, m := range matches {
		if len(m) < 6 {
			continue
		}
		file := m[1]
		line := atoiPtr(m[2])
		col := atoiPtr(m[3])
		sev := contract.SeverityError
		if strings.ToLower(m[4]) == "warning" {
			sev = contract.SeverityWarning
		}
		msg := m[5]
		out = append(out, contract.DiagnosticV1{
			Tool:     tool,
			Language: "lean",
			File:     file,
			Line:     line,
			Column:   col,
			Code:     "",
			Severity: sev,
			Message:  msg,
		})
	}
	return out
}

func ParseUnixStyle(stdout string, stderr string, language string, tool string) []contract.DiagnosticV1 {
	var out []contract.DiagnosticV1
	combined := stdout + "\n" + stderr
	re := regexp.MustCompile(`(?m)^(.+?):(\d+):(\d+):\s*(.+)$`)
	matches := re.FindAllStringSubmatch(combined, -1)
	for _, m := range matches {
		if len(m) < 5 {
			continue
		}
		file := m[1]
		line := atoiPtr(m[2])
		col := atoiPtr(m[3])
		msg := m[4]
		out = append(out, contract.DiagnosticV1{
			Tool:     tool,
			Language: language,
			File:     file,
			Line:     line,
			Column:   col,
			Code:     "",
			Severity: contract.SeverityWarning,
			Message:  msg,
		})
	}
	return out
}

func ParsePython(stdout string, stderr string) []contract.DiagnosticV1 {
	tool := "python -m py_compile"
	var out []contract.DiagnosticV1
	combined := stdout + "\n" + stderr
	lines := strings.Split(combined, "\n")

	fileRe := regexp.MustCompile(`^\s*File\s+"(.+?)",\s+line\s+(\d+).*$`)
	errRe := regexp.MustCompile(`^(SyntaxError|IndentationError|NameError|TypeError):\s*(.+)$`)

	for i := 0; i < len(lines); i++ {
		line := lines[i]
		m := fileRe.FindStringSubmatch(line)
		if len(m) < 3 {
			continue
		}
		file := m[1]
		lineN := atoiPtr(m[2])

		code := "SyntaxError"
		msg := "syntax error"

		for j := i + 1; j < len(lines); j++ {
			em := errRe.FindStringSubmatch(strings.TrimSpace(lines[j]))
			if len(em) >= 3 {
				code = em[1]
				msg = em[2]
				break
			}
		}

		out = append(out, contract.DiagnosticV1{
			Tool:     tool,
			Language: "python",
			File:     file,
			Line:     lineN,
			Column:   nil,
			Code:     code,
			Severity: contract.SeverityError,
			Message:  msg,
		})
	}
	return out
}

func ParseRust(stdout string, stderr string) []contract.DiagnosticV1 {
	tool := "cargo check"
	var out []contract.DiagnosticV1
	combined := stdout + "\n" + stderr
	lines := strings.Split(combined, "\n")

	errHead := regexp.MustCompile(`^error(?:\[(E\d+)\])?:\s*(.+)$`)
	warnHead := regexp.MustCompile(`^warning:\s*(.+)$`)
	locRe := regexp.MustCompile(`^\s*-->\s+(.+?):(\d+):(\d+)\s*$`)

	for i := 0; i < len(lines); i++ {
		line := lines[i]

		if m := errHead.FindStringSubmatch(strings.TrimSpace(line)); len(m) >= 3 {
			code := strings.TrimSpace(m[1])
			msg := strings.TrimSpace(m[2])
			var file string
			var ln, col *int

			for j := i + 1; j < len(lines); j++ {
				if lm := locRe.FindStringSubmatch(lines[j]); len(lm) >= 4 {
					file = lm[1]
					ln = atoiPtr(lm[2])
					col = atoiPtr(lm[3])
					break
				}
				if strings.HasPrefix(strings.TrimSpace(lines[j]), "error") {
					break
				}
			}

			out = append(out, contract.DiagnosticV1{
				Tool:     tool,
				Language: "rust",
				File:     file,
				Line:     ln,
				Column:   col,
				Code:     code,
				Severity: contract.SeverityError,
				Message:  msg,
			})
			continue
		}

		if m := warnHead.FindStringSubmatch(strings.TrimSpace(line)); len(m) >= 2 {
			msg := strings.TrimSpace(m[1])
			var file string
			var ln, col *int
			for j := i + 1; j < len(lines); j++ {
				if lm := locRe.FindStringSubmatch(lines[j]); len(lm) >= 4 {
					file = lm[1]
					ln = atoiPtr(lm[2])
					col = atoiPtr(lm[3])
					break
				}
				t := strings.TrimSpace(lines[j])
				if strings.HasPrefix(t, "warning") || strings.HasPrefix(t, "error") {
					break
				}
			}
			out = append(out, contract.DiagnosticV1{
				Tool:     tool,
				Language: "rust",
				File:     file,
				Line:     ln,
				Column:   col,
				Code:     "",
				Severity: contract.SeverityWarning,
				Message:  msg,
			})
		}
	}

	return out
}

func atoiPtr(s string) *int {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	n := 0
	sign := 1
	i := 0
	if strings.HasPrefix(s, "-") {
		sign = -1
		i++
	}
	for ; i < len(s); i++ {
		c := s[i]
		if c < '0' || c > '9' {
			break
		}
		n = n*10 + int(c-'0')
	}
	n = n * sign
	return &n
}
