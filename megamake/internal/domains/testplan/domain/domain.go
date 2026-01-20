package domain

import (
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	contract "github.com/megamake/megamake/internal/contracts/v1/testplan"
	project "github.com/megamake/megamake/internal/contracts/v1/project"
)

// -------------------------
// Framework detection
// -------------------------

func DetectFrameworks(rootRelRead func(rel string, maxBytes int64) (string, bool)) map[string][]string {
	byLang := map[string][]string{}

	// JS/TS: package.json
	if s, ok := rootRelRead("package.json", 2_000_000); ok {
		l := strings.ToLower(s)
		var f []string
		if strings.Contains(l, "jest") {
			f = append(f, "jest")
		}
		if strings.Contains(l, "vitest") {
			f = append(f, "vitest")
		}
		if strings.Contains(l, "mocha") {
			f = append(f, "mocha")
		}
		if strings.Contains(l, "playwright") {
			f = append(f, "playwright")
		}
		if strings.Contains(l, "cypress") {
			f = append(f, "cypress")
		}
		byLang["javascript"] = uniq(f)
		byLang["typescript"] = uniq(f)
	}

	// Python: pyproject/requirements
	for _, p := range []string{"pyproject.toml", "requirements.txt", "Pipfile"} {
		if s, ok := rootRelRead(p, 2_000_000); ok {
			l := strings.ToLower(s)
			f := byLang["python"]
			if strings.Contains(l, "pytest") {
				f = append(f, "pytest")
			}
			if strings.Contains(l, "unittest") {
				f = append(f, "unittest")
			}
			if strings.Contains(l, "behave") {
				f = append(f, "behave")
			}
			byLang["python"] = uniq(f)
		}
	}

	// Go, Rust, Swift, Java, Lean
	byLang["go"] = []string{"go test"}
	byLang["rust"] = []string{"cargo test"}
	byLang["swift"] = []string{"XCTest (swift test)"}
	byLang["java"] = []string{"JUnit"}
	byLang["kotlin"] = []string{"JUnit"}
	byLang["lean"] = []string{"Lake (Lean 4): lake build"}

	return byLang
}

// -------------------------
// Subject analysis
// -------------------------

func LanguageForRel(rel string) string {
	ext := strings.ToLower(filepath.Ext(rel))
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

func AnalyzeFile(rel string, content string, lang string) []contract.TestSubjectV1 {
	switch lang {
	case "typescript", "javascript":
		return analyzeJS(rel, content, lang)
	case "python":
		return analyzePython(rel, content)
	case "go":
		return analyzeGo(rel, content)
	case "rust":
		return analyzeRust(rel, content)
	case "swift":
		return analyzeSwift(rel, content)
	case "java":
		return analyzeJava(rel, content)
	case "kotlin":
		return analyzeKotlin(rel, content)
	case "lean":
		return analyzeLean(rel, content)
	default:
		return nil
	}
}

func analyzeJS(rel string, content string, lang string) []contract.TestSubjectV1 {
	var out []contract.TestSubjectV1

	lower := strings.ToLower(content)
	risk, factors, io := scoreRisk(lower)

	// export function foo(...)
	fnRe := regexp.MustCompile(`(?m)^\s*(?:export\s+(?:default\s+)?)?(?:async\s+)?function\s+([A-Za-z_]\w*)\s*\(([^)]*)\)`)
	for _, m := range fnRe.FindAllStringSubmatch(content, -1) {
		name := m[1]
		sig := "function " + name + "(" + m[2] + ")"
		exported := strings.Contains(content, "export function "+name) || strings.Contains(content, "export default function "+name)
		out = append(out, contract.TestSubjectV1{
			ID:        rel + "#fn:" + name,
			Kind:      contract.KindFunction,
			Language:  lang,
			Name:      name,
			Path:      rel,
			Signature: sig,
			Exported:  exported,
			Params:    parseParamsColon(m[2]),
			RiskScore: risk,
			RiskFactors: factors,
			IO:        io,
			Meta:      map[string]string{},
		})
	}

	// export const foo = (...) =>
	arrowRe := regexp.MustCompile(`(?m)^\s*export\s+(?:default\s+)?(?:const|let|var)\s+([A-Za-z_]\w*)\s*=\s*(?:async\s+)?\(([^)]*)\)\s*=>`)
	for _, m := range arrowRe.FindAllStringSubmatch(content, -1) {
		name := m[1]
		out = append(out, contract.TestSubjectV1{
			ID:        rel + "#fn:" + name,
			Kind:      contract.KindFunction,
			Language:  lang,
			Name:      name,
			Path:      rel,
			Signature: "const " + name + " = (" + m[2] + ") =>",
			Exported:  true,
			Params:    parseParamsColon(m[2]),
			RiskScore: risk,
			RiskFactors: factors,
			IO:        io,
			Meta:      map[string]string{},
		})
	}

	clsRe := regexp.MustCompile(`(?m)^\s*(?:export\s+(?:default\s+)?|)class\s+([A-Za-z_]\w*)`)
	for _, m := range clsRe.FindAllStringSubmatch(content, -1) {
		name := m[1]
		exported := strings.Contains(content, "export class "+name) || strings.Contains(content, "export default class "+name)
		out = append(out, contract.TestSubjectV1{
			ID:        rel + "#class:" + name,
			Kind:      contract.KindClass,
			Language:  lang,
			Name:      name,
			Path:      rel,
			Signature: "class " + name,
			Exported:  exported,
			RiskScore: risk,
			RiskFactors: factors,
			IO:        io,
			Meta:      map[string]string{},
		})
	}

	routeRe := regexp.MustCompile(`(?m)\b(app|router|server|fastify)\.(get|post|put|delete|patch)\(\s*['"]([^'"]+)['"]`)
	for _, m := range routeRe.FindAllStringSubmatch(content, -1) {
		method := strings.ToUpper(m[2])
		path := m[3]
		out = append(out, contract.TestSubjectV1{
			ID:        rel + "#endpoint:" + method + " " + path,
			Kind:      contract.KindEndpoint,
			Language:  lang,
			Name:      method + " " + path,
			Path:      rel,
			Signature: "",
			Exported:  true,
			RiskScore: maxInt(risk, 4),
			RiskFactors: append([]string{"http route"}, factors...),
			IO:        io,
			Meta:      map[string]string{"method": method, "path": path},
		})
	}

	_ = lower
	return out
}

func analyzePython(rel string, content string) []contract.TestSubjectV1 {
	var out []contract.TestSubjectV1
	lower := strings.ToLower(content)
	risk, factors, io := scoreRisk(lower)

	fnRe := regexp.MustCompile(`(?m)^\s*def\s+([A-Za-z_]\w*)\s*\(([^)]*)\)\s*:`)
	for _, m := range fnRe.FindAllStringSubmatch(content, -1) {
		name := m[1]
		out = append(out, contract.TestSubjectV1{
			ID:        rel + "#fn:" + name,
			Kind:      contract.KindFunction,
			Language:  "python",
			Name:      name,
			Path:      rel,
			Signature: "def " + name + "(" + m[2] + "):",
			Exported:  !strings.HasPrefix(name, "_"),
			Params:    parseParamsPython(m[2]),
			RiskScore: risk,
			RiskFactors: factors,
			IO:        io,
			Meta:      map[string]string{},
		})
	}

	clsRe := regexp.MustCompile(`(?m)^\s*class\s+([A-Za-z_]\w*)`)
	for _, m := range clsRe.FindAllStringSubmatch(content, -1) {
		name := m[1]
		out = append(out, contract.TestSubjectV1{
			ID:        rel + "#class:" + name,
			Kind:      contract.KindClass,
			Language:  "python",
			Name:      name,
			Path:      rel,
			Signature: "class " + name,
			Exported:  true,
			RiskScore: risk,
			RiskFactors: factors,
			IO:        io,
			Meta:      map[string]string{},
		})
	}

	routeRe := regexp.MustCompile(`(?m)^\s*@(?:app|router)\.(get|post|put|delete|patch)\(\s*['"]([^'"]+)['"]`)
	for _, m := range routeRe.FindAllStringSubmatch(content, -1) {
		method := strings.ToUpper(m[1])
		path := m[2]
		out = append(out, contract.TestSubjectV1{
			ID:        rel + "#endpoint:" + method + " " + path,
			Kind:      contract.KindEndpoint,
			Language:  "python",
			Name:      method + " " + path,
			Path:      rel,
			Exported:  true,
			RiskScore: maxInt(risk, 4),
			RiskFactors: append([]string{"http route"}, factors...),
			IO:        io,
			Meta:      map[string]string{"method": method, "path": path},
		})
	}

	return out
}

func analyzeGo(rel string, content string) []contract.TestSubjectV1 {
	var out []contract.TestSubjectV1
	lower := strings.ToLower(content)
	risk, factors, io := scoreRisk(lower)

	fnRe := regexp.MustCompile(`(?m)^\s*func\s*(?:\([^)]+\)\s*)?([A-Za-z_]\w*)\s*\(([^)]*)\)`)
	for _, m := range fnRe.FindAllStringSubmatch(content, -1) {
		name := m[1]
		exported := len(name) > 0 && strings.ToUpper(name[:1]) == name[:1]
		out = append(out, contract.TestSubjectV1{
			ID:        rel + "#fn:" + name,
			Kind:      contract.KindFunction,
			Language:  "go",
			Name:      name,
			Path:      rel,
			Signature: "func " + name + "(" + m[2] + ")",
			Exported:  exported,
			Params:    parseParamsGo(m[2]),
			RiskScore: risk,
			RiskFactors: factors,
			IO:        io,
			Meta:      map[string]string{},
		})
	}

	routeRe := regexp.MustCompile(`(?m)\.(GET|POST|PUT|DELETE|PATCH)\(\s*["']([^"']+)["']`)
	for _, m := range routeRe.FindAllStringSubmatch(content, -1) {
		method := strings.ToUpper(m[1])
		path := m[2]
		out = append(out, contract.TestSubjectV1{
			ID:        rel + "#endpoint:" + method + " " + path,
			Kind:      contract.KindEndpoint,
			Language:  "go",
			Name:      method + " " + path,
			Path:      rel,
			Exported:  true,
			RiskScore: maxInt(risk, 4),
			RiskFactors: append([]string{"http route"}, factors...),
			IO:        io,
			Meta:      map[string]string{"method": method, "path": path},
		})
	}

	handleRe := regexp.MustCompile(`(?m)http\.HandleFunc\(\s*["']([^"']+)["']`)
	for _, m := range handleRe.FindAllStringSubmatch(content, -1) {
		path := m[1]
		out = append(out, contract.TestSubjectV1{
			ID:        rel + "#endpoint:ANY " + path,
			Kind:      contract.KindEndpoint,
			Language:  "go",
			Name:      "ANY " + path,
			Path:      rel,
			Exported:  true,
			RiskScore: maxInt(risk, 4),
			RiskFactors: append([]string{"http route"}, factors...),
			IO:        io,
			Meta:      map[string]string{"method": "ANY", "path": path},
		})
	}

	return out
}

func analyzeRust(rel string, content string) []contract.TestSubjectV1 {
	var out []contract.TestSubjectV1
	lower := strings.ToLower(content)
	risk, factors, io := scoreRisk(lower)

	fnRe := regexp.MustCompile(`(?m)^\s*pub\s+fn\s+([A-Za-z_]\w*)\s*\(([^)]*)\)`)
	for _, m := range fnRe.FindAllStringSubmatch(content, -1) {
		name := m[1]
		out = append(out, contract.TestSubjectV1{
			ID:        rel + "#fn:" + name,
			Kind:      contract.KindFunction,
			Language:  "rust",
			Name:      name,
			Path:      rel,
			Signature: "pub fn " + name + "(" + m[2] + ")",
			Exported:  true,
			Params:    parseParamsRust(m[2]),
			RiskScore: risk,
			RiskFactors: factors,
			IO:        io,
			Meta:      map[string]string{},
		})
	}

	routeRe := regexp.MustCompile(`(?m)^\s*#\[\s*(get|post|put|delete|patch)\s*\(\s*["']([^"']+)['"]`)
	for _, m := range routeRe.FindAllStringSubmatch(content, -1) {
		method := strings.ToUpper(m[1])
		path := m[2]
		out = append(out, contract.TestSubjectV1{
			ID:        rel + "#endpoint:" + method + " " + path,
			Kind:      contract.KindEndpoint,
			Language:  "rust",
			Name:      method + " " + path,
			Path:      rel,
			Exported:  true,
			RiskScore: maxInt(risk, 4),
			RiskFactors: append([]string{"http route"}, factors...),
			IO:        io,
			Meta:      map[string]string{"method": method, "path": path},
		})
	}

	return out
}

func analyzeSwift(rel string, content string) []contract.TestSubjectV1 {
	var out []contract.TestSubjectV1
	lower := strings.ToLower(content)
	risk, factors, io := scoreRisk(lower)

	fnRe := regexp.MustCompile(`(?m)^\s*(?:public|open|internal|fileprivate|private)?\s*func\s+([A-Za-z_]\w*)\s*\(([^)]*)\)`)
	for _, m := range fnRe.FindAllStringSubmatch(content, -1) {
		name := m[1]
		out = append(out, contract.TestSubjectV1{
			ID:        rel + "#fn:" + name,
			Kind:      contract.KindFunction,
			Language:  "swift",
			Name:      name,
			Path:      rel,
			Signature: "func " + name + "(" + m[2] + ")",
			Exported:  true,
			Params:    parseParamsSwift(m[2]),
			RiskScore: risk,
			RiskFactors: factors,
			IO:        io,
			Meta:      map[string]string{},
		})
	}

	typeRe := regexp.MustCompile(`(?m)^\s*(?:public|open|internal|fileprivate|private)?\s*(class|struct|enum)\s+([A-Za-z_]\w*)`)
	for _, m := range typeRe.FindAllStringSubmatch(content, -1) {
		name := m[2]
		out = append(out, contract.TestSubjectV1{
			ID:        rel + "#class:" + name,
			Kind:      contract.KindClass,
			Language:  "swift",
			Name:      name,
			Path:      rel,
			Signature: m[1] + " " + name,
			Exported:  true,
			RiskScore: risk,
			RiskFactors: factors,
			IO:        io,
			Meta:      map[string]string{},
		})
	}

	// init(...) (entrypoint-ish)
	initRe := regexp.MustCompile(`(?m)^\s*(?:public|open|internal|fileprivate|private)?\s*init\s*\(([^)]*)\)`)
	for range initRe.FindAllStringSubmatch(content, -1) {
		out = append(out, contract.TestSubjectV1{
			ID:        rel + "#init",
			Kind:      contract.KindFunction,
			Language:  "swift",
			Name:      "init",
			Path:      rel,
			Signature: "init(...)",
			Exported:  true,
			RiskScore: risk,
			RiskFactors: factors,
			IO:        io,
			Meta:      map[string]string{},
		})
		break
	}

	return out
}

func analyzeJava(rel string, content string) []contract.TestSubjectV1 {
	var out []contract.TestSubjectV1
	lower := strings.ToLower(content)
	risk, factors, io := scoreRisk(lower)

	clsRe := regexp.MustCompile(`(?m)^\s*public\s+class\s+([A-Za-z_]\w*)`)
	for _, m := range clsRe.FindAllStringSubmatch(content, -1) {
		name := m[1]
		out = append(out, contract.TestSubjectV1{
			ID:        rel + "#class:" + name,
			Kind:      contract.KindClass,
			Language:  "java",
			Name:      name,
			Path:      rel,
			Signature: "public class " + name,
			Exported:  true,
			RiskScore: risk,
			RiskFactors: factors,
			IO:        io,
			Meta:      map[string]string{},
		})
	}

	fnRe := regexp.MustCompile(`(?m)^\s*(?:public|protected|private)\s+(?:static\s+)?[A-Za-z0-9_<>\[\]]+\s+([A-Za-z_]\w*)\s*\(([^)]*)\)\s*\{`)
	for _, m := range fnRe.FindAllStringSubmatch(content, -1) {
		name := m[1]
		out = append(out, contract.TestSubjectV1{
			ID:        rel + "#fn:" + name,
			Kind:      contract.KindFunction,
			Language:  "java",
			Name:      name,
			Path:      rel,
			Signature: "method " + name + "(" + m[2] + ")",
			Exported:  true,
			Params:    parseParamsJava(m[2]),
			RiskScore: risk,
			RiskFactors: factors,
			IO:        io,
			Meta:      map[string]string{},
		})
	}

	routeRe := regexp.MustCompile(`(?m)^\s*@(?:GetMapping|PostMapping|PutMapping|DeleteMapping|PatchMapping)\(\s*["']([^"']+)["']`)
	for _, m := range routeRe.FindAllStringSubmatch(content, -1) {
		path := m[1]
		method := detectSpringMethod(m[0])
		out = append(out, contract.TestSubjectV1{
			ID:        rel + "#endpoint:" + method + " " + path,
			Kind:      contract.KindEndpoint,
			Language:  "java",
			Name:      method + " " + path,
			Path:      rel,
			Exported:  true,
			RiskScore: maxInt(risk, 4),
			RiskFactors: append([]string{"http route"}, factors...),
			IO:        io,
			Meta:      map[string]string{"method": method, "path": path},
		})
	}

	return out
}

func analyzeKotlin(rel string, content string) []contract.TestSubjectV1 {
	var out []contract.TestSubjectV1
	lower := strings.ToLower(content)
	risk, factors, io := scoreRisk(lower)

	typeRe := regexp.MustCompile(`(?m)^\s*(data\s+class|class|object)\s+([A-Za-z_]\w*)`)
	for _, m := range typeRe.FindAllStringSubmatch(content, -1) {
		name := m[2]
		out = append(out, contract.TestSubjectV1{
			ID:        rel + "#class:" + name,
			Kind:      contract.KindClass,
			Language:  "kotlin",
			Name:      name,
			Path:      rel,
			Signature: m[1] + " " + name,
			Exported:  true,
			RiskScore: risk,
			RiskFactors: factors,
			IO:        io,
			Meta:      map[string]string{},
		})
	}

	fnRe := regexp.MustCompile(`(?m)^\s*(?:public\s+)?fun\s+([A-Za-z_]\w*)\s*\(([^)]*)\)`)
	for _, m := range fnRe.FindAllStringSubmatch(content, -1) {
		name := m[1]
		out = append(out, contract.TestSubjectV1{
			ID:        rel + "#fn:" + name,
			Kind:      contract.KindFunction,
			Language:  "kotlin",
			Name:      name,
			Path:      rel,
			Signature: "fun " + name + "(" + m[2] + ")",
			Exported:  true,
			Params:    parseParamsKt(m[2]),
			RiskScore: risk,
			RiskFactors: factors,
			IO:        io,
			Meta:      map[string]string{},
		})
	}

	routeRe := regexp.MustCompile(`(?m)^\s*@(?:GetMapping|PostMapping|PutMapping|DeleteMapping|PatchMapping)\(\s*["']([^"']+)["']`)
	for _, m := range routeRe.FindAllStringSubmatch(content, -1) {
		path := m[1]
		method := detectSpringMethod(m[0])
		out = append(out, contract.TestSubjectV1{
			ID:        rel + "#endpoint:" + method + " " + path,
			Kind:      contract.KindEndpoint,
			Language:  "kotlin",
			Name:      method + " " + path,
			Path:      rel,
			Exported:  true,
			RiskScore: maxInt(risk, 4),
			RiskFactors: append([]string{"http route"}, factors...),
			IO:        io,
			Meta:      map[string]string{"method": method, "path": path},
		})
	}

	ktorRe := regexp.MustCompile(`(?m)\b(get|post|put|delete|patch)\(\s*["']([^"']+)["']`)
	for _, m := range ktorRe.FindAllStringSubmatch(content, -1) {
		method := strings.ToUpper(m[1])
		path := m[2]
		out = append(out, contract.TestSubjectV1{
			ID:        rel + "#endpoint:" + method + " " + path,
			Kind:      contract.KindEndpoint,
			Language:  "kotlin",
			Name:      method + " " + path,
			Path:      rel,
			Exported:  true,
			RiskScore: maxInt(risk, 4),
			RiskFactors: append([]string{"http route"}, factors...),
			IO:        io,
			Meta:      map[string]string{"method": method, "path": path},
		})
	}

	return out
}

func analyzeLean(rel string, content string) []contract.TestSubjectV1 {
	var out []contract.TestSubjectV1
	re := regexp.MustCompile(`(?m)^\s*(def|abbrev|theorem|lemma|structure|inductive|class)\s+([A-Za-z_][A-Za-z0-9_']*)\b`)
	for _, m := range re.FindAllStringSubmatch(content, -1) {
		kindTok := m[1]
		name := m[2]
		k := contract.KindFunction
		if kindTok == "structure" || kindTok == "inductive" || kindTok == "class" {
			k = contract.KindClass
		}
		out = append(out, contract.TestSubjectV1{
			ID:        rel + "#lean:" + kindTok + ":" + name,
			Kind:      k,
			Language:  "lean",
			Name:      name,
			Path:      rel,
			Signature: kindTok + " " + name,
			Exported:  true,
			RiskScore: 2,
			RiskFactors: []string{"lean declaration: " + kindTok},
			IO:        contract.IOCapabilitiesV1{},
			Meta:      map[string]string{"lean_decl": kindTok},
		})
	}
	return out
}

func detectSpringMethod(annotationLine string) string {
	if strings.Contains(annotationLine, "@GetMapping") {
		return "GET"
	}
	if strings.Contains(annotationLine, "@PostMapping") {
		return "POST"
	}
	if strings.Contains(annotationLine, "@PutMapping") {
		return "PUT"
	}
	if strings.Contains(annotationLine, "@DeleteMapping") {
		return "DELETE"
	}
	if strings.Contains(annotationLine, "@PatchMapping") {
		return "PATCH"
	}
	return "GET"
}

// -------------------------
// Params parsing helpers (simple heuristics)
// -------------------------

func parseParamsColon(plist string) []contract.SubjectParamV1 {
	parts := splitCSV(plist)
	var out []contract.SubjectParamV1
	for _, p := range parts {
		if p == "" {
			continue
		}
		// name?: type = default
		namePart := p
		typeHint := ""
		if strings.Contains(p, ":") {
			ss := strings.SplitN(p, ":", 2)
			namePart = strings.TrimSpace(ss[0])
			typeHint = strings.TrimSpace(strings.SplitN(ss[1], "=", 2)[0])
		}
		optional := strings.HasSuffix(namePart, "?") || strings.Contains(p, "=")
		name := strings.TrimSuffix(namePart, "?")
		out = append(out, contract.SubjectParamV1{Name: name, TypeHint: typeHint, Optional: optional})
	}
	return out
}

func parseParamsPython(plist string) []contract.SubjectParamV1 {
	parts := splitCSV(plist)
	var out []contract.SubjectParamV1
	for _, p := range parts {
		if p == "" {
			continue
		}
		// name: type = default
		name := strings.TrimSpace(strings.SplitN(p, "=", 2)[0])
		typeHint := ""
		if strings.Contains(name, ":") {
			ss := strings.SplitN(name, ":", 2)
			name = strings.TrimSpace(ss[0])
			typeHint = strings.TrimSpace(ss[1])
		}
		optional := strings.Contains(p, "=") || strings.Contains(typeHint, "None")
		out = append(out, contract.SubjectParamV1{Name: name, TypeHint: typeHint, Optional: optional})
	}
	return out
}

func parseParamsGo(plist string) []contract.SubjectParamV1 {
	parts := splitCSV(plist)
	var out []contract.SubjectParamV1
	for _, p := range parts {
		toks := strings.Fields(p)
		if len(toks) == 0 {
			continue
		}
		name := toks[0]
		typeHint := strings.Join(toks[1:], " ")
		out = append(out, contract.SubjectParamV1{Name: name, TypeHint: typeHint, Optional: false})
	}
	return out
}

func parseParamsRust(plist string) []contract.SubjectParamV1 {
	parts := splitCSV(plist)
	var out []contract.SubjectParamV1
	for _, p := range parts {
		if p == "" {
			continue
		}
		ss := strings.SplitN(p, ":", 2)
		name := strings.TrimSpace(ss[0])
		typeHint := ""
		if len(ss) == 2 {
			typeHint = strings.TrimSpace(ss[1])
		}
		out = append(out, contract.SubjectParamV1{Name: name, TypeHint: typeHint, Optional: false})
	}
	return out
}

func parseParamsSwift(plist string) []contract.SubjectParamV1 {
	parts := splitCSV(plist)
	var out []contract.SubjectParamV1
	for _, p := range parts {
		if p == "" {
			continue
		}
		// external internal: Type = default
		namePart := strings.TrimSpace(strings.SplitN(p, ":", 2)[0])
		nameFields := strings.Fields(namePart)
		name := namePart
		if len(nameFields) > 0 {
			name = nameFields[len(nameFields)-1]
		}
		typeHint := ""
		if strings.Contains(p, ":") {
			typeHint = strings.TrimSpace(strings.SplitN(strings.SplitN(p, ":", 2)[1], "=", 2)[0])
		}
		optional := strings.Contains(typeHint, "?") || strings.Contains(p, "=")
		out = append(out, contract.SubjectParamV1{Name: name, TypeHint: typeHint, Optional: optional})
	}
	return out
}

func parseParamsJava(plist string) []contract.SubjectParamV1 {
	parts := splitCSV(plist)
	var out []contract.SubjectParamV1
	for _, p := range parts {
		toks := strings.Fields(p)
		if len(toks) < 2 {
			continue
		}
		name := toks[len(toks)-1]
		typeHint := strings.Join(toks[:len(toks)-1], " ")
		out = append(out, contract.SubjectParamV1{Name: name, TypeHint: typeHint, Optional: false})
	}
	return out
}

func parseParamsKt(plist string) []contract.SubjectParamV1 {
	parts := splitCSV(plist)
	var out []contract.SubjectParamV1
	for _, p := range parts {
		if p == "" {
			continue
		}
		ss := strings.SplitN(p, ":", 2)
		name := strings.TrimSpace(ss[0])
		typeHint := ""
		if len(ss) == 2 {
			typeHint = strings.TrimSpace(ss[1])
		}
		optional := strings.Contains(p, "=")
		out = append(out, contract.SubjectParamV1{Name: name, TypeHint: typeHint, Optional: optional})
	}
	return out
}

func splitCSV(s string) []string {
	raw := strings.Split(s, ",")
	var out []string
	for _, r := range raw {
		x := strings.TrimSpace(r)
		if x != "" {
			out = append(out, x)
		}
	}
	return out
}

// -------------------------
// Risk scoring and IO detection
// -------------------------

func scoreRisk(lower string) (score int, factors []string, io contract.IOCapabilitiesV1) {
	score = 1

	branches := countAny(lower, []string{" if", " else", " switch", " case", " for", " while", " try", " catch", " guard", " defer", " when", " match"})
	if branches > 0 {
		score += minInt(5, branches/2)
		factors = append(factors, "branches ~"+itoa(branches))
	}

	conc := countAny(lower, []string{"async", "await", "goroutine", " chan", "thread", "dispatchqueue", "tokio", "spawn", "executor"})
	if conc > 0 {
		score += 2
		factors = append(factors, "concurrency hints")
		io.Concurrency = true
	}

	io.ReadsFS = containsAny(lower, []string{"readfile", "open(", "os.open", "filemanager", "io.open", "files.read", "pathlib"})
	io.WritesFS = containsAny(lower, []string{"writefile", "fs.write", "os.create", "os.write", "files.write", "filemanager.default.create"})
	io.Network = containsAny(lower, []string{"http.", "fetch(", "urlsession", "requests.", "axios", "reqwest", "net/http"})
	io.DB = containsAny(lower, []string{"database/sql", "gorm", "sqlalchemy", "psycopg2", "mongo", "mongoose", "redis", "jpa", "entitymanager"})
	io.Env = containsAny(lower, []string{"process.env", "os.environ", "getenv", "environment."})

	ioFlags := []string{}
	if io.ReadsFS || io.WritesFS {
		ioFlags = append(ioFlags, "fs")
		score++
	}
	if io.Network {
		ioFlags = append(ioFlags, "network")
		score++
	}
	if io.DB {
		ioFlags = append(ioFlags, "db")
		score += 2
	}
	if io.Env {
		ioFlags = append(ioFlags, "env")
		score++
	}
	if len(ioFlags) > 0 {
		factors = append(factors, "io: "+strings.Join(ioFlags, ","))
	}

	if score < 1 {
		score = 1
	}
	if score > 10 {
		score = 10
	}
	return score, factors, io
}

func countAny(s string, needles []string) int {
	total := 0
	for _, n := range needles {
		if n == "" {
			continue
		}
		parts := strings.Split(s, n)
		if len(parts) > 1 {
			total += len(parts) - 1
		}
	}
	return total
}

func containsAny(s string, needles []string) bool {
	for _, n := range needles {
		if n != "" && strings.Contains(s, n) {
			return true
		}
	}
	return false
}

// -------------------------
// Coverage assessment
// -------------------------

func AssessCoverage(subjects []contract.TestSubjectV1, testFiles []project.FileRefV1, readRel func(rel string, maxBytes int64) (string, bool), maxAnalyzeBytes int64) map[string]contract.CoverageV1 {
	if len(subjects) == 0 || len(testFiles) == 0 {
		return map[string]contract.CoverageV1{}
	}
	if maxAnalyzeBytes <= 0 {
		maxAnalyzeBytes = 200_000
	}

	type testBlob struct {
		rel   string
		text  string
		lower string
	}
	var blobs []testBlob
	for _, tf := range testFiles {
		s, ok := readRel(tf.RelPath, maxAnalyzeBytes)
		if !ok {
			continue
		}
		blobs = append(blobs, testBlob{rel: tf.RelPath, text: s, lower: strings.ToLower(s)})
	}

	results := map[string]contract.CoverageV1{}

	edgeKeywords := []string{
		"empty", "nil", "null", "undefined", "invalid", "error", "throws", "throw", "exception",
		"large", "huge", "max", "min", "boundary", "timeout", "retry", "concurrent", "race",
		"unauthorized", "forbidden", "denied", "overflow", "underflow",
	}

	for _, subj := range subjects {
		name := strings.TrimSpace(subj.Name)
		if name == "" {
			results[subj.ID] = missingCoverage()
			continue
		}
		re := regexp.MustCompile(`\b` + regexp.QuoteMeta(name) + `\b`)

		totalHits := 0
		var ev []contract.CoverageEvidenceV1
		foundKW := map[string]bool{}

		for _, tb := range blobs {
			hits := len(re.FindAllStringIndex(tb.text, -1))
			if hits > 0 {
				totalHits += hits
				ev = append(ev, contract.CoverageEvidenceV1{File: tb.rel, Hits: hits})
				for _, kw := range edgeKeywords {
					if strings.Contains(tb.lower, kw) {
						foundKW[kw] = true
					}
				}
			}
		}

		if totalHits == 0 {
			results[subj.ID] = missingCoverage()
			continue
		}

		score := 0
		if totalHits >= 10 {
			score += 3
		} else if totalHits >= 5 {
			score += 2
		} else {
			score += 1
		}
		if len(foundKW) >= 2 {
			score += 1
		}
		for _, e := range ev {
			low := strings.ToLower(e.File)
			if strings.Contains(low, "integration") || strings.Contains(low, "e2e") || strings.Contains(low, "end2end") {
				score += 1
				break
			}
		}

		flag := contract.CoverageRed
		status := "MISSING"
		if score >= 4 {
			flag = contract.CoverageGreen
			status = "DONE"
		} else if score >= 2 {
			flag = contract.CoverageYellow
			status = "PARTIAL"
		}

		sort.Slice(ev, func(i, j int) bool { return ev[i].Hits > ev[j].Hits })
		if len(ev) > 5 {
			ev = ev[:5]
		}

		notes := []string{
			"hits=" + itoa(totalHits),
			"edge_keywords=" + joinKeys(foundKW),
		}

		results[subj.ID] = contract.CoverageV1{
			Flag:     flag,
			Status:   status,
			Score:    score,
			Evidence: ev,
			Notes:    notes,
		}
	}

	return results
}

func missingCoverage() contract.CoverageV1 {
	return contract.CoverageV1{
		Flag:     contract.CoverageRed,
		Status:   "MISSING",
		Score:    0,
		Evidence: nil,
		Notes:    []string{"no tests found"},
	}
}

func joinKeys(m map[string]bool) string {
	var keys []string
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return strings.Join(keys, ",")
}

// -------------------------
// Scenario generation
// -------------------------

func BuildScenarios(sub contract.TestSubjectV1, levels contract.LevelSetV1) []contract.ScenarioSuggestionV1 {
	var out []contract.ScenarioSuggestionV1
	if levels.Has(contract.LevelUnit) {
		out = append(out, unitScenario(sub))
	}
	if levels.Has(contract.LevelIntegration) && (sub.IO.Network || sub.IO.DB || sub.IO.ReadsFS || sub.IO.WritesFS || sub.IO.Env) {
		out = append(out, integrationScenario(sub))
	}
	if levels.Has(contract.LevelSmoke) && isEntrypointish(sub) {
		out = append(out, smokeScenario(sub))
	}
	if levels.Has(contract.LevelE2E) && sub.Kind == contract.KindEndpoint {
		out = append(out, e2eScenario(sub))
	}
	return out
}

func RegressionScenario(sub contract.TestSubjectV1, desc string) contract.ScenarioSuggestionV1 {
	isEndpoint := sub.Kind == contract.KindEndpoint
	inputs := []string{}
	if isEndpoint {
		method := sub.Meta["method"]
		path := sub.Meta["path"]
		if method == "" {
			method = "GET"
		}
		if path == "" {
			path = "/"
		}
		inputs = []string{
			method + " " + path + " with minimal valid payload",
			method + " " + path + " missing required fields",
			method + " " + path + " unauthorized/forbidden",
			method + " " + path + " oversized body and invalid JSON",
		}
	} else {
		inputs = fuzzInputs(sub.Params)
	}

	title := "Regression for " + sub.Name
	if strings.TrimSpace(desc) != "" {
		title += " (" + desc + ")"
	}

	steps := []string{
		"Invoke with edge/negative inputs and boundary values",
		"Assert no crashes/panics/exceptions",
		"Assert correct error handling and stable outputs",
	}
	if isEndpoint {
		steps = []string{
			"Start the service with a disposable backing store",
			"Exercise the changed endpoint with edge and invalid payloads",
			"Verify status codes, response schema, and side effects",
		}
	}

	assertions := []string{
		"No crash or unhandled exception",
		"Correct behavior at boundary values and for invalid inputs",
		"Idempotency (re-running does not corrupt state)",
	}
	if sub.IO.Concurrency {
		assertions = append(assertions, "No race conditions or deadlocks under concurrent calls")
	}

	return contract.ScenarioSuggestionV1{
		Level:      contract.LevelRegression,
		Title:      title,
		Rationale:  "Protect against reintroduction of recent issues by testing boundary/negative cases near the change.",
		Steps:      steps,
		Inputs:     take(inputs, 10),
		Assertions: assertions,
	}
}

func unitScenario(s contract.TestSubjectV1) contract.ScenarioSuggestionV1 {
	return contract.ScenarioSuggestionV1{
		Level:     contract.LevelUnit,
		Title:     "Unit tests for " + s.Name,
		Rationale: "Validate core logic, boundary conditions, and error paths. Risk score " + itoa(s.RiskScore) + ".",
		Steps: []string{
			"Isolate the unit and mock external effects.",
			"Cover happy-path plus edge cases below.",
		},
		Inputs: fuzzInputs(s.Params),
		Assertions: []string{
			"Correct outputs for valid inputs",
			"Throws/returns errors for invalid inputs",
			"Idempotency and no state leakage",
			"Handles large inputs within time limits",
		},
	}
}

func integrationScenario(s contract.TestSubjectV1) contract.ScenarioSuggestionV1 {
	var steps []string
	if s.IO.DB {
		steps = append(steps, "Use a disposable DB for read/write/transaction tests")
	}
	if s.IO.Network {
		steps = append(steps, "Mock/stub external HTTP endpoints and cover retries/timeouts")
	}
	if s.IO.ReadsFS || s.IO.WritesFS {
		steps = append(steps, "Use a temp directory for FS reads/writes; test missing paths and permissions")
	}
	if s.IO.Env {
		steps = append(steps, "Vary environment variables; test unset/malformed values")
	}
	if s.IO.Concurrency {
		steps = append(steps, "Run concurrent invocations to detect races and locking issues")
	}
	return contract.ScenarioSuggestionV1{
		Level:     contract.LevelIntegration,
		Title:     "Integration tests for " + s.Name,
		Rationale: "Covers real I/O and cross-module boundaries indicated by IO capabilities.",
		Steps:     steps,
		Assertions: []string{
			"Correct behavior under network/DB errors",
			"Resource cleanup (connections, files)",
			"Retry/backoff adherence (if applicable)",
			"No deadlocks or race conditions",
		},
	}
}

func smokeScenario(s contract.TestSubjectV1) contract.ScenarioSuggestionV1 {
	return contract.ScenarioSuggestionV1{
		Level:     contract.LevelSmoke,
		Title:     "Smoke test for " + s.Name,
		Rationale: "Ensure the primary entrypoint boots and responds.",
		Steps: []string{
			"Build/start the service or executable",
			"Probe /health or a trivial endpoint (if applicable)",
			"Run CLI --help / basic command returns 0",
		},
		Assertions: []string{
			"Process exits 0 or keeps running",
			"Boot completes within a short timeout",
			"Basic route returns HTTP 200 (if applicable)",
		},
	}
}

func e2eScenario(s contract.TestSubjectV1) contract.ScenarioSuggestionV1 {
	path := s.Meta["path"]
	method := s.Meta["method"]
	if path == "" {
		path = "/"
	}
	if method == "" {
		method = "GET"
	}
	inputs := []string{
		method + " " + path + " with minimal valid payload",
		method + " " + path + " missing required fields",
		method + " " + path + " with extra fields",
		method + " " + path + " unauthorized/forbidden",
		method + " " + path + " oversized body and invalid JSON",
	}
	return contract.ScenarioSuggestionV1{
		Level:     contract.LevelE2E,
		Title:     "E2E for " + method + " " + path,
		Rationale: "Validate the full request/response path and persistence side effects.",
		Steps: []string{
			"Start the service with a disposable backing store",
			"Issue requests with the payloads below",
			"Follow-on GET/queries to verify persisted state",
		},
		Inputs: inputs,
		Assertions: []string{
			"Status codes and response schemas",
			"Auth/permissions if applicable",
			"Idempotency and invariants across requests",
		},
	}
}

func isEntrypointish(s contract.TestSubjectV1) bool {
	if s.Kind == contract.KindEntrypoint {
		return true
	}
	n := strings.ToLower(s.Name)
	return n == "main" || strings.Contains(n, "start") || strings.Contains(n, "run") || strings.Contains(n, "boot")
}

func fuzzInputs(params []contract.SubjectParamV1) []string {
	if len(params) == 0 {
		return []string{"No inputs: call with defaults; expect not to crash and return sane output"}
	}
	var cases []string
	for _, p := range params {
		n := strings.ToLower(p.Name)
		t := strings.ToLower(p.TypeHint)
		if strings.Contains(t, "int") || strings.Contains(t, "float") || strings.Contains(t, "double") || strings.Contains(t, "number") || strings.Contains(n, "count") || strings.Contains(n, "limit") {
			cases = append(cases, p.Name+"=0,1,-1")
			cases = append(cases, p.Name+"=very large value")
		} else if strings.Contains(t, "string") || strings.Contains(n, "name") || strings.Contains(n, "id") || strings.Contains(n, "path") || strings.Contains(n, "url") {
			cases = append(cases, p.Name+"=\"\" (empty), whitespace-only")
			cases = append(cases, p.Name+" very long (10k chars), unicode")
			cases = append(cases, p.Name+" with injection-like content ('; DROP, ../../, <script>)")
		} else if strings.Contains(t, "bool") || strings.HasPrefix(n, "is") || strings.HasPrefix(n, "has") {
			cases = append(cases, p.Name+"=true and "+p.Name+"=false")
		} else {
			cases = append(cases, p.Name+" nominal valid value")
			cases = append(cases, p.Name+" invalid/malformed value")
		}
		if p.Optional {
			cases = append(cases, p.Name+"=nil/undefined")
		}
		if len(cases) >= 24 {
			break
		}
	}
	return take(cases, 24)
}

func take(xs []string, n int) []string {
	if n <= 0 || len(xs) == 0 {
		return nil
	}
	if len(xs) <= n {
		return xs
	}
	return xs[:n]
}

// -------------------------
// Small utils
// -------------------------

func uniq(xs []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, x := range xs {
		x = strings.TrimSpace(x)
		if x == "" || seen[x] {
			continue
		}
		seen[x] = true
		out = append(out, x)
	}
	return out
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

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
