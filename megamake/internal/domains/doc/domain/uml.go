package domain

import (
	"regexp"
	"sort"
	"strings"

	contractdoc "github.com/megamake/megamake/internal/contracts/v1/doc"
)

type UMLGranularity string

const (
	GranularityFile    UMLGranularity = "file"
	GranularityModule  UMLGranularity = "module"
	GranularityPackage UMLGranularity = "package"
)

type UMLNodeKind string

const (
	NodeModule     UMLNodeKind = "module"
	NodeExternal   UMLNodeKind = "external"
	NodeDatasource UMLNodeKind = "datasource"
	NodeEndpoint   UMLNodeKind = "endpoint"
	NodeMain       UMLNodeKind = "main"
)

type UMLEdgeRel string

const (
	RelImports UMLEdgeRel = "imports"
	RelUses    UMLEdgeRel = "uses"
	RelServes  UMLEdgeRel = "serves"
)

type UMLNode struct {
	ID    string
	Label string
	Kind  UMLNodeKind
	Group string
}

type UMLEdge struct {
	FromID string
	ToID   string
	Rel    UMLEdgeRel
	Label  string
}

type UMLDiagram struct {
	Nodes  []UMLNode
	Edges  []UMLEdge
	Legend string
}

type UMLBuildOptions struct {
	Granularity      UMLGranularity
	MaxNodes         int
	IncludeIO        bool
	IncludeEndpoints bool
}

func BuildUML(relPaths []string, fileContents map[string]string, imports []contractdoc.DocImportV1, externalCounts map[string]int, opt UMLBuildOptions) UMLDiagram {
	if opt.Granularity == "" {
		opt.Granularity = GranularityModule
	}
	if opt.MaxNodes <= 0 {
		opt.MaxNodes = 120
	}

	// Map file -> module
	fileToModule := map[string]string{}
	for _, rel := range relPaths {
		fileToModule[rel] = moduleName(rel, opt.Granularity)
	}

	nodes := map[string]UMLNode{}
	edges := map[string]UMLEdge{}

	ensureNode := func(id string, label string, kind UMLNodeKind, group string) {
		if _, ok := nodes[id]; ok {
			return
		}
		nodes[id] = UMLNode{ID: id, Label: label, Kind: kind, Group: group}
	}

	addEdge := func(fromID, toID string, rel UMLEdgeRel, label string) {
		key := fromID + "|" + toID + "|" + string(rel) + "|" + label
		edges[key] = UMLEdge{FromID: fromID, ToID: toID, Rel: rel, Label: label}
	}

	// Build module-to-module import edges and external edges.
	for _, di := range imports {
		fromModule := fileToModule[di.File]
		if fromModule == "" {
			fromModule = moduleName(di.File, opt.Granularity)
		}
		fromID := nodeID("module", fromModule)
		ensureNode(fromID, fromModule, NodeModule, groupFor(fromModule))

		if di.IsInternal && di.ResolvedPath != "" {
			toModule := fileToModule[di.ResolvedPath]
			if toModule == "" {
				toModule = moduleName(di.ResolvedPath, opt.Granularity)
			}
			toID := nodeID("module", toModule)
			ensureNode(toID, toModule, NodeModule, groupFor(toModule))
			addEdge(fromID, toID, RelImports, "")
		}
	}

	// External collapsing
	topExternalCount := max(5, min(15, opt.MaxNodes/6))
	type kv struct {
		k string
		v int
	}
	var externals []kv
	for k, v := range externalCounts {
		externals = append(externals, kv{k: k, v: v})
	}
	sort.Slice(externals, func(i, j int) bool {
		if externals[i].v == externals[j].v {
			return externals[i].k < externals[j].k
		}
		return externals[i].v > externals[j].v
	})
	kept := map[string]bool{}
	for i := 0; i < len(externals) && i < topExternalCount; i++ {
		kept[externals[i].k] = true
	}
	collapsedCount := 0
	if len(externals) > len(kept) {
		collapsedCount = len(externals) - len(kept)
	}

	if collapsedCount > 0 {
		collID := nodeID("external", "external/*")
		ensureNode(collID, "[external/*] ("+itoa(collapsedCount)+" more)", NodeExternal, "external")
		// Add a single incoming edge from every module that imports a collapsed external
		for _, di := range imports {
			if di.IsInternal {
				continue
			}
			if kept[di.Raw] {
				continue
			}
			fromModule := fileToModule[di.File]
			fromID := nodeID("module", fromModule)
			ensureNode(fromID, fromModule, NodeModule, groupFor(fromModule))
			addEdge(fromID, collID, RelImports, "")
		}
	}

	for _, di := range imports {
		if di.IsInternal {
			continue
		}
		if !kept[di.Raw] {
			continue
		}
		extLabel := "ext:" + di.Raw
		extID := nodeID("external", extLabel)
		ensureNode(extID, extLabel, NodeExternal, "external")
		fromModule := fileToModule[di.File]
		fromID := nodeID("module", fromModule)
		ensureNode(fromID, fromModule, NodeModule, groupFor(fromModule))
		addEdge(fromID, extID, RelImports, "")
	}

	// IO + endpoints + main
	if opt.IncludeIO || opt.IncludeEndpoints {
		// module -> IO flags
		moduleFlags := map[string]ioFlags{}
		var mainFiles []string

		for _, rel := range relPaths {
			text := fileContents[rel]
			lower := strings.ToLower(text)
			mod := fileToModule[rel]
			if mod == "" {
				mod = moduleName(rel, opt.Granularity)
			}
			if opt.IncludeIO {
				cur := moduleFlags[mod]
				cur = cur.merged(detectIO(lower))
				moduleFlags[mod] = cur
			}
			if opt.IncludeEndpoints {
				lang := languageForRel(rel)
				for _, ep := range detectEndpoints(text, lang) {
					epLabel := "(" + ep.method + " " + ep.path + ")"
					epID := nodeID("endpoint", "endpoint:"+ep.method+" "+ep.path)
					ensureNode(epID, epLabel, NodeEndpoint, "endpoint")
					modID := nodeID("module", mod)
					ensureNode(modID, mod, NodeModule, groupFor(mod))
					addEdge(epID, modID, RelServes, "")
				}
			}
			if isMainFile(rel, lower) {
				mainFiles = append(mainFiles, rel)
			}
		}

		if opt.IncludeIO {
			for mod, fl := range moduleFlags {
				modID := nodeID("module", mod)
				ensureNode(modID, mod, NodeModule, groupFor(mod))
				if fl.db {
					dbName := fl.dbKind
					if dbName == "" {
						dbName = "db"
					}
					dsID := nodeID("datasource", "db:"+dbName)
					ensureNode(dsID, "db: "+dbName, NodeDatasource, "datasource")
					addEdge(modID, dsID, RelUses, "uses")
				}
				if fl.fsRead || fl.fsWrite {
					dsID := nodeID("datasource", "fs")
					ensureNode(dsID, "fs", NodeDatasource, "datasource")
					lbl := "reads"
					if fl.fsWrite {
						lbl = "reads/writes"
					}
					addEdge(modID, dsID, RelUses, lbl)
				}
				if fl.env {
					dsID := nodeID("datasource", "env")
					ensureNode(dsID, "env", NodeDatasource, "datasource")
					addEdge(modID, dsID, RelUses, "reads")
				}
				if fl.network {
					dsID := nodeID("datasource", "http:external")
					ensureNode(dsID, "http: external", NodeDatasource, "datasource")
					addEdge(modID, dsID, RelUses, "calls")
				}
			}
		}

		if len(mainFiles) > 0 {
			sort.Strings(mainFiles)
			mf := mainFiles[0]
			mainID := nodeID("main", "main")
			ensureNode(mainID, "main", NodeMain, "main")

			selfMod := fileToModule[mf]
			if selfMod == "" {
				selfMod = moduleName(mf, opt.Granularity)
			}
			selfID := nodeID("module", selfMod)
			ensureNode(selfID, selfMod, NodeModule, groupFor(selfMod))
			addEdge(mainID, selfID, RelImports, "")

			// Connect main -> imported modules (internal only)
			for _, di := range imports {
				if di.File != mf || !di.IsInternal || di.ResolvedPath == "" {
					continue
				}
				tm := fileToModule[di.ResolvedPath]
				if tm == "" {
					tm = moduleName(di.ResolvedPath, opt.Granularity)
				}
				toID := nodeID("module", tm)
				ensureNode(toID, tm, NodeModule, groupFor(tm))
				addEdge(mainID, toID, RelImports, "")
			}
		}
	}

	legend := "Legend:\n" +
		"- [module] components (grouped by " + string(opt.Granularity) + ")\n" +
		"- ext:* are external libraries\n" +
		"- db:/fs/env/http:external are data sources\n" +
		"- (METHOD /path) are HTTP endpoints\n" +
		"- main is the top-level entrypoint (if detected)\n" +
		"Arrows:\n" +
		"- imports: [a] --> [b]\n" +
		"- uses:    [a] ..> [ds]\n" +
		"- serves:  (endpoint) --> [handler module]\n"

	// Finalize nodes/edges arrays sorted deterministically.
	nodeList := make([]UMLNode, 0, len(nodes))
	for _, n := range nodes {
		nodeList = append(nodeList, n)
	}
	sort.Slice(nodeList, func(i, j int) bool {
		if nodeList[i].Kind != nodeList[j].Kind {
			return nodeList[i].Kind < nodeList[j].Kind
		}
		return nodeList[i].ID < nodeList[j].ID
	})

	edgeList := make([]UMLEdge, 0, len(edges))
	for _, e := range edges {
		edgeList = append(edgeList, e)
	}
	sort.Slice(edgeList, func(i, j int) bool {
		a := edgeList[i].FromID + "|" + edgeList[i].ToID + "|" + string(edgeList[i].Rel) + "|" + edgeList[i].Label
		b := edgeList[j].FromID + "|" + edgeList[j].ToID + "|" + string(edgeList[j].Rel) + "|" + edgeList[j].Label
		return a < b
	})

	return UMLDiagram{
		Nodes:  nodeList,
		Edges:  edgeList,
		Legend: legend,
	}
}

func ToUMLASCII(d UMLDiagram) string {
	var lines []string
	lines = append(lines, d.Legend)

	labelFor := func(id string) string {
		for _, n := range d.Nodes {
			if n.ID == id {
				if n.Kind == NodeEndpoint {
					return n.Label
				}
				return "[" + n.Label + "]"
			}
		}
		return id
	}

	for _, e := range d.Edges {
		from := labelFor(e.FromID)
		to := labelFor(e.ToID)
		switch e.Rel {
		case RelImports:
			lines = append(lines, from+" --> "+to)
		case RelUses:
			if strings.TrimSpace(e.Label) != "" {
				lines = append(lines, from+" ..> "+to+" <<"+e.Label+">>")
			} else {
				lines = append(lines, from+" ..> "+to)
			}
		case RelServes:
			lines = append(lines, from+" --> "+to)
		}
	}
	return strings.Join(lines, "\n")
}

func ToPlantUML(d UMLDiagram) string {
	var lines []string
	lines = append(lines, "@startuml")
	lines = append(lines, "skinparam componentStyle rectangle")

	pumlID := func(s string) string {
		re := regexp.MustCompile(`[^A-Za-z0-9_]`)
		return re.ReplaceAllString(s, "_")
	}
	esc := func(s string) string {
		return strings.ReplaceAll(s, `"`, `\"`)
	}

	for _, n := range d.Nodes {
		id := pumlID(n.ID)
		switch n.Kind {
		case NodeModule:
			lines = append(lines, "rectangle \""+esc(n.Label)+"\" as "+id)
		case NodeExternal:
			lines = append(lines, "component \""+esc(n.Label)+"\" as "+id)
		case NodeDatasource:
			if strings.HasPrefix(strings.ToLower(n.Label), "db:") {
				lines = append(lines, "database \""+esc(n.Label)+"\" as "+id)
			} else {
				lines = append(lines, "queue \""+esc(n.Label)+"\" as "+id)
			}
		case NodeEndpoint:
			lines = append(lines, "usecase \""+esc(n.Label)+"\" as "+id)
		case NodeMain:
			lines = append(lines, "rectangle \"main\" as "+id)
		}
	}

	for _, e := range d.Edges {
		from := pumlID(e.FromID)
		to := pumlID(e.ToID)
		switch e.Rel {
		case RelImports:
			lines = append(lines, from+" --> "+to)
		case RelUses:
			if strings.TrimSpace(e.Label) != "" {
				lines = append(lines, from+" ..> "+to+" : "+esc(e.Label))
			} else {
				lines = append(lines, from+" ..> "+to)
			}
		case RelServes:
			lines = append(lines, from+" --> "+to)
		}
	}

	lines = append(lines, "@enduml")
	return strings.Join(lines, "\n")
}

func nodeID(kind string, label string) string {
	raw := kind + "_" + label
	raw = strings.ReplaceAll(raw, " ", "_")
	raw = strings.ReplaceAll(raw, "/", "_")
	raw = strings.ReplaceAll(raw, ":", "_")
	// Strip remaining non-safe chars.
	re := regexp.MustCompile(`[^A-Za-z0-9_]+`)
	return re.ReplaceAllString(raw, "")
}

func moduleName(rel string, gran UMLGranularity) string {
	parts := strings.Split(rel, "/")
	if len(parts) == 0 {
		return rel
	}
	switch gran {
	case GranularityFile:
		return rel
	case GranularityPackage:
		return parts[0]
	case GranularityModule:
		anchors := map[string]bool{"src": true, "lib": true, "pkg": true, "app": true, "cmd": true, "internal": true}
		if len(parts) >= 2 && anchors[parts[0]] {
			return parts[0] + "/" + parts[1]
		}
		return parts[0]
	default:
		return parts[0]
	}
}

func groupFor(label string) string {
	if strings.HasPrefix(label, "src/") {
		return "src"
	}
	if strings.HasPrefix(label, "pkg/") {
		return "pkg"
	}
	if strings.HasPrefix(label, "lib/") {
		return "lib"
	}
	if strings.HasPrefix(label, "app/") {
		return "app"
	}
	if strings.HasPrefix(label, "cmd/") {
		return "cmd"
	}
	if strings.HasPrefix(label, "internal/") {
		return "internal"
	}
	return "root"
}

type ioFlags struct {
	fsRead      bool
	fsWrite     bool
	network     bool
	db          bool
	env         bool
	concurrency bool
	dbKind      string
}

func (a ioFlags) merged(b ioFlags) ioFlags {
	out := a
	out.fsRead = out.fsRead || b.fsRead
	out.fsWrite = out.fsWrite || b.fsWrite
	out.network = out.network || b.network
	out.db = out.db || b.db
	out.env = out.env || b.env
	out.concurrency = out.concurrency || b.concurrency
	if out.dbKind == "" {
		out.dbKind = b.dbKind
	}
	return out
}

func detectIO(lower string) ioFlags {
	var f ioFlags
	if strings.Contains(lower, "fs.") || strings.Contains(lower, "open(") || strings.Contains(lower, "os.open") || strings.Contains(lower, "filemanager") || strings.Contains(lower, "pathlib") {
		f.fsRead = true
	}
	if strings.Contains(lower, "writefile") || strings.Contains(lower, "fs.write") || strings.Contains(lower, "os.create") || strings.Contains(lower, "os.write") || strings.Contains(lower, "filemanager.default.create") {
		f.fsWrite = true
	}
	if strings.Contains(lower, "http.") || strings.Contains(lower, "fetch(") || strings.Contains(lower, "urlsession") || strings.Contains(lower, "requests.") || strings.Contains(lower, "reqwest") || strings.Contains(lower, "net/http") {
		f.network = true
	}
	if strings.Contains(lower, "process.env") || strings.Contains(lower, "os.environ") || strings.Contains(lower, "getenv(") || strings.Contains(lower, "environment.") {
		f.env = true
	}
	if strings.Contains(lower, "async") || strings.Contains(lower, "await") || strings.Contains(lower, "goroutine") || strings.Contains(lower, " chan") || strings.Contains(lower, "dispatchqueue") || strings.Contains(lower, "tokio") || strings.Contains(lower, "spawn") {
		f.concurrency = true
	}
	if strings.Contains(lower, "sqlalchemy") || strings.Contains(lower, "psycopg2") || strings.Contains(lower, "gorm") || strings.Contains(lower, "database/sql") ||
		strings.Contains(lower, "entitymanager") || strings.Contains(lower, "jpa") || strings.Contains(lower, "mongoose") || strings.Contains(lower, "redis") {
		f.db = true
		if strings.Contains(lower, "postgres") || strings.Contains(lower, "psycopg2") {
			f.dbKind = "postgres"
		} else if strings.Contains(lower, "mysql") {
			f.dbKind = "mysql"
		} else if strings.Contains(lower, "sqlite") {
			f.dbKind = "sqlite"
		} else if strings.Contains(lower, "mongo") {
			f.dbKind = "mongo"
		} else if strings.Contains(lower, "redis") {
			f.dbKind = "redis"
		}
	}
	return f
}

type endpoint struct {
	method string
	path   string
}

func detectEndpoints(text string, lang string) []endpoint {
	var out []endpoint
	add := func(method, p string) {
		m := strings.ToUpper(strings.TrimSpace(method))
		pp := strings.TrimSpace(p)
		if m == "" || pp == "" {
			return
		}
		out = append(out, endpoint{method: m, path: pp})
	}

	switch lang {
	case "javascript", "typescript":
		re := regexp.MustCompile(`(?m)\b(app|router|server|fastify)\.(get|post|put|delete|patch)\(\s*['"]([^'"]+)['"]`)
		for _, m := range re.FindAllStringSubmatch(text, -1) {
			if len(m) >= 4 {
				add(m[2], m[3])
			}
		}
		re2 := regexp.MustCompile(`(?m)new\s+Router\(\)\.(get|post|put|delete|patch)\(\s*['"]([^'"]+)['"]`)
		for _, m := range re2.FindAllStringSubmatch(text, -1) {
			if len(m) >= 3 {
				add(m[1], m[2])
			}
		}

	case "python":
		re := regexp.MustCompile(`(?m)^\s*@(?:app|router)\.(get|post|put|delete|patch)\(\s*['"]([^'"]+)['"]`)
		for _, m := range re.FindAllStringSubmatch(text, -1) {
			if len(m) >= 3 {
				add(m[1], m[2])
			}
		}

	case "go":
		re := regexp.MustCompile(`(?m)\.(GET|POST|PUT|DELETE|PATCH)\(\s*["']([^"']+)["']`)
		for _, m := range re.FindAllStringSubmatch(text, -1) {
			if len(m) >= 3 {
				add(m[1], m[2])
			}
		}
		re2 := regexp.MustCompile(`(?m)http\.HandleFunc\(\s*["']([^"']+)["']`)
		for _, m := range re2.FindAllStringSubmatch(text, -1) {
			if len(m) >= 2 {
				add("ANY", m[1])
			}
		}

	case "rust":
		re := regexp.MustCompile(`(?m)^\s*#\[\s*(get|post|put|delete|patch)\s*\(\s*["']([^"']+)['"]`)
		for _, m := range re.FindAllStringSubmatch(text, -1) {
			if len(m) >= 3 {
				add(m[1], m[2])
			}
		}

	case "java", "kotlin":
		re := regexp.MustCompile(`(?m)^\s*@((?:Get|Post|Put|Delete|Patch)Mapping)\(\s*["']([^"']+)['"]`)
		for _, m := range re.FindAllStringSubmatch(text, -1) {
			if len(m) >= 3 {
				add(methodFromMapping(m[1]), m[2])
			}
		}
		re2 := regexp.MustCompile(`(?m)^\s*@RequestMapping\([^)]*value\s*=\s*["']([^"']+)["'][^)]*method\s*=\s*RequestMethod\.([A-Z]+)[^)]*\)`)
		for _, m := range re2.FindAllStringSubmatch(text, -1) {
			if len(m) >= 3 {
				add(m[2], m[1])
			}
		}
	}

	return out
}

func methodFromMapping(mapping string) string {
	m := strings.ToLower(mapping)
	if strings.HasPrefix(m, "get") {
		return "GET"
	}
	if strings.HasPrefix(m, "post") {
		return "POST"
	}
	if strings.HasPrefix(m, "put") {
		return "PUT"
	}
	if strings.HasPrefix(m, "delete") {
		return "DELETE"
	}
	if strings.HasPrefix(m, "patch") {
		return "PATCH"
	}
	return "GET"
}

func isMainFile(rel string, lower string) bool {
	rl := strings.ToLower(rel)
	base := rl
	if strings.Contains(rl, "/") {
		parts := strings.Split(rl, "/")
		base = parts[len(parts)-1]
	}
	if rl == "src/main.rs" || base == "main.swift" {
		return true
	}
	if strings.HasSuffix(rl, "/main.ts") || strings.HasSuffix(rl, "/main.js") || strings.HasSuffix(rl, "/server.ts") || strings.HasSuffix(rl, "/server.js") ||
		rl == "src/index.ts" || rl == "src/index.js" || rl == "index.ts" || rl == "index.js" {
		return true
	}
	if strings.Contains(lower, "package main") && strings.Contains(lower, "func main(") {
		return true
	}
	if strings.Contains(lower, "@main") {
		return true
	}
	if strings.Contains(lower, "public static void main(") {
		return true
	}
	if strings.Contains(lower, "fun main(") {
		return true
	}
	return false
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// func itoa(n int) string {
// 	if n == 0 {
// 		return "0"
// 	}
// 	sign := ""
// 	if n < 0 {
// 		sign = "-"
// 		n = -n
// 	}
// 	var buf [32]byte
// 	i := len(buf)
// 	for n > 0 {
// 		i--
// 		buf[i] = byte('0' + (n % 10))
// 		n /= 10
// 	}
// 	return sign + string(buf[i:])
// }
