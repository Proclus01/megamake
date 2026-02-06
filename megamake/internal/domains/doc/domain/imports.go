package domain

import (
	"path"
	"regexp"
	"sort"
	"strings"

	contractdoc "github.com/megamake/megamake/internal/contracts/v1/doc"
)

// importEdge is a stable, file-scope edge type for import graph rendering.
// (Do not re-declare a same-shaped type inside BuildImportGraph; Go treats them as distinct named types.)
type importEdge struct {
	from       string
	to         string
	isInternal bool
}

// importTarget is the per-source target edge used for rendering and de-duping.
type importTarget struct {
	to         string
	isInternal bool
}

// BuildImportGraph parses imports and returns:
// - imports: a flat list of imports
// - asciiGraph: a human-friendly import graph grouped by source file
// - externalCounts: counts of external dependency strings
func BuildImportGraph(relPaths []string, fileContents map[string]string) (imports []contractdoc.DocImportV1, asciiGraph string, externalCounts map[string]int) {
	externalCounts = map[string]int{}

	// Index for internal-by-stem resolution.
	stemIndex := buildStemIndex(relPaths)
	exists := make(map[string]bool, len(relPaths))
	for _, p := range relPaths {
		exists[p] = true
	}

	var edges []importEdge

	for _, rel := range relPaths {
		content, ok := fileContents[rel]
		if !ok {
			continue
		}
		lang := languageForRel(rel)
		if lang == "" {
			continue
		}
		raws := parseImports(content, lang)
		for _, raw := range raws {
			isInternal, resolved := resolveImport(rel, raw, lang, exists, stemIndex)
			di := contractdoc.DocImportV1{
				File:       rel,
				Language:   lang,
				Raw:        raw,
				IsInternal: isInternal,
			}
			if resolved != "" {
				di.ResolvedPath = resolved
			}
			imports = append(imports, di)

			target := raw
			if isInternal {
				if resolved != "" {
					target = resolved
				}
			} else {
				externalCounts[raw] = externalCounts[raw] + 1
			}
			edges = append(edges, importEdge{from: rel, to: target, isInternal: isInternal})
		}
	}

	asciiGraph = renderASCII(edges)
	return imports, asciiGraph, externalCounts
}

func languageForRel(rel string) string {
	ext := strings.ToLower(path.Ext(rel))
	switch ext {
	case ".ts", ".tsx":
		return "typescript"
	case ".js", ".jsx", ".mjs", ".cjs":
		return "javascript"
	case ".py":
		return "python"
	case ".go":
		return "go"
	case ".rs":
		return "rust"
	case ".swift":
		return "swift"
	case ".java":
		return "java"
	case ".kt", ".kts":
		return "kotlin"
	case ".lean":
		return "lean"
	default:
		return ""
	}
}

func parseImports(content string, lang string) []string {
	var out []string
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s != "" {
			out = append(out, s)
		}
	}

	switch lang {
	case "typescript", "javascript":
		re1 := regexp.MustCompile(`(?m)^\s*import\s+(?:[^'"]*\s+from\s+)?['"]([^'"]+)['"]`)
		re2 := regexp.MustCompile(`(?m)require\(\s*['"]([^'"]+)['"]\s*\)`)
		re3 := regexp.MustCompile(`(?m)import\(\s*['"]([^'"]+)['"]\s*\)`)
		for _, m := range re1.FindAllStringSubmatch(content, -1) {
			if len(m) >= 2 {
				add(m[1])
			}
		}
		for _, m := range re2.FindAllStringSubmatch(content, -1) {
			if len(m) >= 2 {
				add(m[1])
			}
		}
		for _, m := range re3.FindAllStringSubmatch(content, -1) {
			if len(m) >= 2 {
				add(m[1])
			}
		}

	case "python":
		re1 := regexp.MustCompile(`(?m)^\s*import\s+([A-Za-z0-9_\.]+)`)
		re2 := regexp.MustCompile(`(?m)^\s*from\s+([A-Za-z0-9_\.]+)\s+import\s+`)
		for _, m := range re1.FindAllStringSubmatch(content, -1) {
			if len(m) >= 2 {
				add(m[1])
			}
		}
		for _, m := range re2.FindAllStringSubmatch(content, -1) {
			if len(m) >= 2 {
				add(m[1])
			}
		}

	case "go":
		reLine := regexp.MustCompile(`(?m)^\s*import\s+"([^"]+)"`)
		for _, m := range reLine.FindAllStringSubmatch(content, -1) {
			if len(m) >= 2 {
				add(m[1])
			}
		}
		reBlock := regexp.MustCompile(`(?s)import\s*\(\s*([^\)]+)\s*\)`)
		blocks := reBlock.FindAllStringSubmatch(content, -1)
		reQ := regexp.MustCompile(`(?m)"([^"]+)"`)
		for _, b := range blocks {
			if len(b) >= 2 {
				for _, m := range reQ.FindAllStringSubmatch(b[1], -1) {
					if len(m) >= 2 {
						add(m[1])
					}
				}
			}
		}

	case "rust":
		re := regexp.MustCompile(`(?m)^\s*use\s+([A-Za-z0-9_:]+)`)
		for _, m := range re.FindAllStringSubmatch(content, -1) {
			if len(m) >= 2 {
				add(m[1])
			}
		}

	case "swift":
		re := regexp.MustCompile(`(?m)^\s*import\s+([A-Za-z0-9_]+)`)
		for _, m := range re.FindAllStringSubmatch(content, -1) {
			if len(m) >= 2 {
				add(m[1])
			}
		}

	case "java", "kotlin":
		re := regexp.MustCompile(`(?m)^\s*import\s+([A-Za-z0-9_\.]+)`)
		for _, m := range re.FindAllStringSubmatch(content, -1) {
			if len(m) >= 2 {
				add(m[1])
			}
		}

	case "lean":
		re := regexp.MustCompile(`(?m)^\s*import\s+(.+)$`)
		for _, m := range re.FindAllStringSubmatch(content, -1) {
			if len(m) >= 2 {
				parts := strings.Fields(m[1])
				for _, p := range parts {
					add(p)
				}
			}
		}
	}

	return compact(out)
}

func compact(xs []string) []string {
	m := map[string]bool{}
	var out []string
	for _, x := range xs {
		x = strings.TrimSpace(x)
		if x == "" {
			continue
		}
		if m[x] {
			continue
		}
		m[x] = true
		out = append(out, x)
	}
	return out
}

func buildStemIndex(relPaths []string) map[string][]string {
	idx := map[string][]string{}
	for _, rel := range relPaths {
		base := path.Base(rel)
		stem := strings.TrimSuffix(base, path.Ext(base))
		idx[stem] = append(idx[stem], rel)
		idx[base] = append(idx[base], rel)
	}
	return idx
}

func resolveImport(fromRel string, raw string, lang string, exists map[string]bool, stemIndex map[string][]string) (isInternal bool, resolved string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false, ""
	}

	if strings.HasPrefix(raw, "./") || strings.HasPrefix(raw, "../") {
		res := resolveRelative(fromRel, raw, exists)
		if res != "" {
			return true, res
		}
		return true, ""
	}

	if strings.Contains(raw, "://") {
		return false, ""
	}

	if lang == "lean" {
		cand := strings.ReplaceAll(raw, ".", "/") + ".lean"
		if exists[cand] {
			return true, cand
		}
		return false, ""
	}

	if isLikelyExternal(raw, lang) {
		return false, ""
	}

	last := raw
	if strings.Contains(raw, "/") {
		parts := strings.Split(raw, "/")
		last = parts[len(parts)-1]
	}
	hits := stemIndex[last]
	if len(hits) == 1 {
		return true, hits[0]
	}

	return false, ""
}

func isLikelyExternal(raw string, lang string) bool {
	switch lang {
	case "typescript", "javascript":
		if strings.HasPrefix(raw, "@") {
			return true
		}
		if strings.HasPrefix(raw, "node:") {
			return true
		}
		if strings.Contains(raw, "/") {
			return true
		}
		return false
	case "go":
		return strings.Contains(raw, ".") && strings.Contains(raw, "/")
	default:
		return false
	}
}

func resolveRelative(fromRel string, raw string, exists map[string]bool) string {
	dir := path.Dir(fromRel)
	if dir == "." {
		dir = ""
	}
	cand := path.Clean(path.Join(dir, raw))
	cand = strings.TrimPrefix(cand, "./")

	if exists[cand] {
		return cand
	}

	exts := []string{".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs", ".py", ".go", ".rs", ".swift", ".java", ".kt", ".kts", ".lean"}
	for _, e := range exts {
		if exists[cand+e] {
			return cand + e
		}
	}

	indexes := []string{"index.ts", "index.tsx", "index.js", "mod.rs", "lib.rs"}
	for _, idx := range indexes {
		p := path.Join(cand, idx)
		if exists[p] {
			return p
		}
	}
	return ""
}

func renderASCII(edges []importEdge) string {
	by := map[string][]importTarget{}
	for _, e := range edges {
		by[e.from] = append(by[e.from], importTarget{to: e.to, isInternal: e.isInternal})
	}

	sources := make([]string, 0, len(by))
	for k := range by {
		sources = append(sources, k)
	}
	sort.Strings(sources)

	var lines []string
	for _, src := range sources {
		lines = append(lines, src)
		ts := uniqueTargets(by[src])
		for _, t := range ts {
			tag := "(external)"
			if t.isInternal {
				tag = "(internal)"
			}
			lines = append(lines, "  └─> "+t.to+" "+tag)
		}
	}
	return strings.Join(lines, "\n")
}

func uniqueTargets(targets []importTarget) []importTarget {
	seen := map[string]bool{}
	var out []importTarget
	for _, t := range targets {
		key := t.to
		if t.isInternal {
			key = "i:" + key
		} else {
			key = "e:" + key
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].isInternal != out[j].isInternal {
			return out[i].isInternal && !out[j].isInternal
		}
		return out[i].to < out[j].to
	})
	return out
}
