package adapters

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/megamake/megamake/internal/domains/chat/ports"
)

// FSEnvLoader loads dotenv-style KEY=VALUE pairs from a file.
//
// Parsing rules (intentionally minimal and predictable):
// - Lines starting with '#' are comments.
// - Blank lines ignored.
// - KEY=VALUE required.
// - KEY is trimmed.
// - VALUE is trimmed; surrounding single or double quotes are stripped if present.
// - "export KEY=VALUE" is supported.
// - No variable expansion is performed.
// - Invalid lines are ignored with a warning.
// - If Overwrite=false, existing env vars are not overwritten.
type FSEnvLoader struct{}

func NewFSEnvLoader() FSEnvLoader { return FSEnvLoader{} }

func (FSEnvLoader) Load(req ports.LoadEnvRequest) (ports.LoadEnvResult, error) {
	path := strings.TrimSpace(req.Path)
	if path == "" {
		return ports.LoadEnvResult{Loaded: false}, nil
	}

	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ports.LoadEnvResult{Loaded: false}, nil
		}
		return ports.LoadEnvResult{}, fmt.Errorf("env: failed to open %s: %v", path, err)
	}
	defer f.Close()

	var warnings []string
	var keysSet []string

	sc := bufio.NewScanner(f)
	lineN := 0
	for sc.Scan() {
		lineN++
		raw := sc.Text()
		s := strings.TrimSpace(raw)
		if s == "" || strings.HasPrefix(s, "#") {
			continue
		}
		if strings.HasPrefix(s, "export ") {
			s = strings.TrimSpace(strings.TrimPrefix(s, "export "))
		}

		// KEY=VALUE
		idx := strings.Index(s, "=")
		if idx <= 0 {
			warnings = append(warnings, fmtLine(path, lineN, "ignoring invalid line (expected KEY=VALUE)"))
			continue
		}

		key := strings.TrimSpace(s[:idx])
		val := strings.TrimSpace(s[idx+1:])

		if key == "" {
			warnings = append(warnings, fmtLine(path, lineN, "ignoring empty key"))
			continue
		}

		// Strip surrounding quotes if present.
		if len(val) >= 2 {
			if (val[0] == '"' && val[len(val)-1] == '"') || (val[0] == '\'' && val[len(val)-1] == '\'') {
				val = val[1 : len(val)-1]
			}
		}

		if !req.Overwrite {
			if _, ok := os.LookupEnv(key); ok {
				continue
			}
		}

		if err := os.Setenv(key, val); err != nil {
			warnings = append(warnings, fmtLine(path, lineN, "failed to set env var "+key))
			continue
		}
		keysSet = append(keysSet, key)
	}

	if err := sc.Err(); err != nil {
		return ports.LoadEnvResult{}, fmt.Errorf("env: failed reading %s: %v", path, err)
	}

	return ports.LoadEnvResult{
		Loaded:   true,
		KeysSet:  keysSet,
		Warnings: warnings,
	}, nil
}

func fmtLine(path string, line int, msg string) string {
	return path + ":" + itoa(line) + ": " + msg
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
