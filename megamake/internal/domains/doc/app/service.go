package app

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	contractartifact "github.com/megamake/megamake/internal/contracts/v1/artifact"
	contractdoc "github.com/megamake/megamake/internal/contracts/v1/doc"
	project "github.com/megamake/megamake/internal/contracts/v1/project"
	docdomain "github.com/megamake/megamake/internal/domains/doc/domain"
	"github.com/megamake/megamake/internal/domains/doc/ports"
	repoapi "github.com/megamake/megamake/internal/domains/repo/api"
	"github.com/megamake/megamake/internal/platform/clock"
	"github.com/megamake/megamake/internal/platform/policy"
)

type Service struct {
	Clock          clock.Clock
	Repo           repoapi.API
	ArtifactWriter ports.ArtifactWriter
}

type CreateRequest struct {
	RootPath        string
	ArtifactDir     string
	Force           bool
	MaxFileBytes    int64
	MaxAnalyzeBytes int64
	TreeDepth       int

	// UML options (local create mode)
	UMLFormats          string // csv: ascii,plantuml|ascii|plantuml|none
	UMLGranularity      string // file|module|package
	UMLMaxNodes         int
	UMLIncludeIO        bool
	UMLIncludeEndpoints bool

	IgnoreNames []string
	IgnoreGlobs []string

	// Included for audit metadata parity with global flags (even if unused in local mode).
	NetEnabled   bool
	AllowDomains []string

	// Raw args for audit (optional; may be nil).
	Args []string
}

type CreateResult struct {
	Report     contractdoc.DocReportV1
	ReportXML  string
	ReportJSON string
	PromptText string

	ArtifactPath string
	LatestPath   string
}

// GetRequest drives `megamake doc get`.
// URIs can be http(s), file://, absolute paths, or relative paths.
type GetRequest struct {
	URIs        []string
	ArtifactDir string

	CrawlDepth int

	NetEnabled   bool
	AllowDomains []string

	Args []string
}

type GetResult struct {
	Report     contractdoc.DocReportV1
	ReportXML  string
	ReportJSON string
	PromptText string

	ArtifactPath string
	LatestPath   string
}

func (s *Service) Create(req CreateRequest) (CreateResult, error) {
	if s.Clock == nil {
		return CreateResult{}, fmt.Errorf("internal error: Clock is nil")
	}
	if s.Repo == nil {
		return CreateResult{}, fmt.Errorf("internal error: Repo is nil")
	}
	if s.ArtifactWriter == nil {
		return CreateResult{}, fmt.Errorf("internal error: ArtifactWriter is nil")
	}

	if strings.TrimSpace(req.RootPath) == "" {
		req.RootPath = "."
	}
	if strings.TrimSpace(req.ArtifactDir) == "" {
		req.ArtifactDir = req.RootPath
	}
	if req.MaxFileBytes <= 0 {
		req.MaxFileBytes = 1_500_000
	}
	if req.MaxAnalyzeBytes <= 0 {
		req.MaxAnalyzeBytes = 200_000
	}
	if req.TreeDepth <= 0 {
		req.TreeDepth = 6
	}
	if strings.TrimSpace(req.UMLFormats) == "" {
		req.UMLFormats = "ascii,plantuml"
	}
	if strings.TrimSpace(req.UMLGranularity) == "" {
		req.UMLGranularity = "module"
	}
	if req.UMLMaxNodes <= 0 {
		req.UMLMaxNodes = 120
	}

	now := s.Clock.NowUTC()

	profile, err := s.Repo.Detect(req.RootPath)
	if err != nil {
		return CreateResult{}, err
	}
	if !profile.IsCodeProject && !req.Force {
		return CreateResult{}, fmt.Errorf(buildSafetyStopMessage(profile))
	}

	files, err := s.Repo.Scan(req.RootPath, profile, repoapi.ScanOptions{
		MaxFileBytes: req.MaxFileBytes,
		IgnoreNames:  req.IgnoreNames,
		IgnoreGlobs:  req.IgnoreGlobs,
	})
	if err != nil {
		return CreateResult{}, err
	}

	relPaths := make([]string, 0, len(files))
	for _, f := range files {
		relPaths = append(relPaths, f.RelPath)
	}
	sort.Strings(relPaths)

	fileContents := map[string]string{}
	var warnings []string
	var sampleTexts []string

	for _, rel := range relPaths {
		b, err := s.Repo.ReadFileRel(req.RootPath, rel, req.MaxFileBytes)
		if err != nil {
			warnings = append(warnings, "unable to read "+rel+": "+err.Error())
			continue
		}
		if int64(len(b)) > req.MaxAnalyzeBytes {
			b = b[:req.MaxAnalyzeBytes]
		}
		if !isLikelyUTF8(b) {
			warnings = append(warnings, "skipping non-UTF8 analysis for: "+rel)
			continue
		}
		txt := string(b)
		fileContents[rel] = txt
		if len(sampleTexts) < 40 {
			sampleTexts = append(sampleTexts, txt)
		}
	}

	rootName := baseNameForRoot(req.RootPath)
	dirTree := docdomain.BuildDirectoryTreeFromFiles(rootName, relPaths, req.TreeDepth)

	imps, asciiGraph, externalCounts := docdomain.BuildImportGraph(relPaths, fileContents)
	readmeText := findReadmeText(relPaths, fileContents)
	purpose := docdomain.GuessPurpose(docdomain.PurposeInput{
		ReadmeText: readmeText,
		Languages:  profile.Languages,
		SampleText: sampleTexts,
	})

	umlAscii := ""
	umlPlant := ""
	wantAscii, wantPlant, includeUML := parseUMLFormats(req.UMLFormats)
	if includeUML {
		gran := parseGranularity(req.UMLGranularity)
		diagram := docdomain.BuildUML(relPaths, fileContents, imps, externalCounts, docdomain.UMLBuildOptions{
			Granularity:      gran,
			MaxNodes:         req.UMLMaxNodes,
			IncludeIO:        req.UMLIncludeIO,
			IncludeEndpoints: req.UMLIncludeEndpoints,
		})
		if wantAscii {
			umlAscii = docdomain.ToUMLASCII(diagram)
		}
		if wantPlant {
			umlPlant = docdomain.ToPlantUML(diagram)
		}
	}

	report := contractdoc.DocReportV1{
		GeneratedAt:          contractartifact.FormatRFC3339NanoUTC(now),
		Mode:                 contractdoc.DocModeLocal,
		RootPath:             req.RootPath,
		Languages:            sortedStrings(profile.Languages),
		DirectoryTree:        dirTree,
		ImportGraph:          asciiGraph,
		Imports:              imps,
		ExternalDependencies: externalCounts,
		PurposeSummary:       purpose,
		FetchedDocs:          nil,
		UMLASCII:             umlAscii,
		UMLPlantUML:          umlPlant,
		Warnings:             warnings,
	}

	xmlOut := report.ToXML()
	jsonBytes, _ := json.MarshalIndent(report, "", "  ")
	jsonOut := string(jsonBytes)
	promptText := docdomain.GenerateDocPrompt(report)

	meta := contractartifact.ArtifactMetaV1{
		Tool:         "megadoc",
		Contract:     "v1",
		GeneratedAt:  contractartifact.FormatRFC3339NanoUTC(now),
		RootPath:     req.RootPath,
		Args:         req.Args,
		NetEnabled:   req.NetEnabled,
		AllowDomains: cloneStrings(req.AllowDomains),
		Warnings:     warnings,
	}

	env := contractartifact.ArtifactEnvelopeV1{Meta: meta, XML: xmlOut, JSON: jsonOut, Prompt: promptText}

	artifactPath, latestPath, err := s.ArtifactWriter.WriteToolArtifact(ports.WriteArtifactRequest{
		ArtifactDir:    req.ArtifactDir,
		ToolPrefix:     "MEGADOC",
		Envelope:       env,
		GeneratedAtUTC: timePtr(now),
	})
	if err != nil {
		return CreateResult{}, err
	}

	return CreateResult{
		Report:       report,
		ReportXML:    xmlOut,
		ReportJSON:   jsonOut,
		PromptText:   promptText,
		ArtifactPath: artifactPath,
		LatestPath:   latestPath,
	}, nil
}

func (s *Service) Get(req GetRequest) (GetResult, error) {
	if s.Clock == nil {
		return GetResult{}, fmt.Errorf("internal error: Clock is nil")
	}
	if s.ArtifactWriter == nil {
		return GetResult{}, fmt.Errorf("internal error: ArtifactWriter is nil")
	}
	if len(req.URIs) == 0 {
		return GetResult{}, fmt.Errorf("no URIs provided")
	}
	if strings.TrimSpace(req.ArtifactDir) == "" {
		// Caller should set; fallback to cwd.
		req.ArtifactDir = "."
	}
	if req.CrawlDepth <= 0 {
		req.CrawlDepth = 1
	}

	now := s.Clock.NowUTC()

	pol := policy.Policy{
		NetEnabled:   req.NetEnabled,
		AllowDomains: cloneStrings(req.AllowDomains),
	}

	fetcher := newDocFetcher(pol, req.CrawlDepth)
	var docs []contractdoc.FetchedDocV1
	for _, u := range req.URIs {
		u = strings.TrimSpace(u)
		if u == "" {
			continue
		}
		got, err := fetcher.fetch(u)
		if err != nil {
			// Non-fatal: include warning and continue.
			docs = append(docs, contractdoc.FetchedDocV1{
				URI:            u,
				Title:          "(error)",
				ContentPreview: err.Error(),
			})
			continue
		}
		docs = append(docs, got...)
	}

	// Deduplicate by URI (stable: first wins), then sort by URI for determinism.
	seen := map[string]bool{}
	var uniq []contractdoc.FetchedDocV1
	for _, d := range docs {
		if strings.TrimSpace(d.URI) == "" {
			continue
		}
		if seen[d.URI] {
			continue
		}
		seen[d.URI] = true
		uniq = append(uniq, d)
	}
	sort.Slice(uniq, func(i, j int) bool {
		if uniq[i].URI == uniq[j].URI {
			return uniq[i].Title < uniq[j].Title
		}
		return uniq[i].URI < uniq[j].URI
	})

	summary := summarizeFetched(uniq)

	report := contractdoc.DocReportV1{
		GeneratedAt:          contractartifact.FormatRFC3339NanoUTC(now),
		Mode:                 contractdoc.DocModeFetch,
		RootPath:             "",
		Languages:            nil,
		DirectoryTree:        "fetch mode: no directory tree",
		ImportGraph:          "fetch mode: no import graph",
		Imports:              nil,
		ExternalDependencies: map[string]int{},
		PurposeSummary:       summary,
		FetchedDocs:          uniq,
		UMLASCII:             "",
		UMLPlantUML:          "",
		Warnings:             nil,
	}

	xmlOut := report.ToXML()
	jsonBytes, _ := json.MarshalIndent(report, "", "  ")
	jsonOut := string(jsonBytes)
	promptText := docdomain.GenerateDocPrompt(report)

	meta := contractartifact.ArtifactMetaV1{
		Tool:         "megadoc",
		Contract:     "v1",
		GeneratedAt:  contractartifact.FormatRFC3339NanoUTC(now),
		RootPath:     "",
		Args:         req.Args,
		NetEnabled:   req.NetEnabled,
		AllowDomains: cloneStrings(req.AllowDomains),
		Warnings:     nil,
	}

	env := contractartifact.ArtifactEnvelopeV1{Meta: meta, XML: xmlOut, JSON: jsonOut, Prompt: promptText}

	artifactPath, latestPath, err := s.ArtifactWriter.WriteToolArtifact(ports.WriteArtifactRequest{
		ArtifactDir:    req.ArtifactDir,
		ToolPrefix:     "MEGADOC",
		Envelope:       env,
		GeneratedAtUTC: timePtr(now),
	})
	if err != nil {
		return GetResult{}, err
	}

	return GetResult{
		Report:       report,
		ReportXML:    xmlOut,
		ReportJSON:   jsonOut,
		PromptText:   promptText,
		ArtifactPath: artifactPath,
		LatestPath:   latestPath,
	}, nil
}

func timePtr(t time.Time) *time.Time { return &t }

func buildSafetyStopMessage(p project.ProjectProfileV1) string {
	var b strings.Builder
	b.WriteString("Safety stop: This directory does not appear to be a code project.\n")
	if len(p.Why) > 0 {
		b.WriteString("Evidence:\n")
		for _, w := range p.Why {
			b.WriteString("  - ")
			b.WriteString(w)
			b.WriteString("\n")
		}
	}
	b.WriteString("If you are certain, re-run with --force.\n")
	return b.String()
}

func baseNameForRoot(rootPath string) string {
	abs, err := filepath.Abs(rootPath)
	if err != nil {
		return filepath.Base(rootPath)
	}
	return filepath.Base(abs)
}

func findReadmeText(relPaths []string, fileContents map[string]string) string {
	candidates := map[string]bool{
		"readme.md": true,
		"readme":    true,
	}
	for _, rel := range relPaths {
		base := strings.ToLower(filepath.Base(rel))
		if candidates[base] {
			if s, ok := fileContents[rel]; ok {
				return s
			}
		}
	}
	return ""
}

func parseUMLFormats(csv string) (wantASCII bool, wantPlant bool, include bool) {
	items := strings.Split(csv, ",")
	set := map[string]bool{}
	for _, it := range items {
		x := strings.ToLower(strings.TrimSpace(it))
		if x == "" {
			continue
		}
		set[x] = true
	}
	if set["none"] {
		return false, false, false
	}
	wantASCII = set["ascii"] || len(set) == 0
	wantPlant = set["plantuml"] || len(set) == 0
	include = wantASCII || wantPlant
	return wantASCII, wantPlant, include
}

func parseGranularity(s string) docdomain.UMLGranularity {
	x := strings.ToLower(strings.TrimSpace(s))
	switch x {
	case "file":
		return docdomain.GranularityFile
	case "package":
		return docdomain.GranularityPackage
	default:
		return docdomain.GranularityModule
	}
}

func sortedStrings(xs []string) []string {
	out := append([]string(nil), xs...)
	sort.Strings(out)
	return out
}

func cloneStrings(xs []string) []string {
	if len(xs) == 0 {
		return nil
	}
	out := make([]string, 0, len(xs))
	for _, x := range xs {
		x = strings.TrimSpace(x)
		if x == "" {
			continue
		}
		out = append(out, x)
	}
	return out
}

func isLikelyUTF8(b []byte) bool {
	// Fast heuristic: reject if any NUL bytes appear; otherwise assume text-ish.
	if len(b) == 0 {
		return true
	}
	for _, c := range b {
		if c == 0 {
			return false
		}
	}
	return true
}

func summarizeFetched(docs []contractdoc.FetchedDocV1) string {
	if len(docs) == 0 {
		return "No docs fetched."
	}
	var lines []string
	lines = append(lines, "Fetched "+itoa(len(docs))+" doc(s):")
	for i, d := range docs {
		if i >= 12 {
			break
		}
		lines = append(lines, "- "+d.Title+" ["+d.URI+"]")
	}
	if len(docs) > 12 {
		lines = append(lines, "... "+itoa(len(docs)-12)+" more")
	}
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

///////////////////////////////////////////////////////////////////////////////
// Fetcher (internal to doc app): local files + policy-gated HTTP with crawl
///////////////////////////////////////////////////////////////////////////////

type docFetcher struct {
	policy   policy.Policy
	maxDepth int
	client   *http.Client
}

func newDocFetcher(pol policy.Policy, maxDepth int) *docFetcher {
	if maxDepth < 1 {
		maxDepth = 1
	}
	return &docFetcher{
		policy:   pol,
		maxDepth: maxDepth,
		client: &http.Client{
			Timeout: 30 * time.Second,
			// Follow redirects, but enforce policy per redirect.
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if req == nil || req.URL == nil {
					return fmt.Errorf("redirect to invalid URL")
				}
				return pol.RequireNetworkAllowed(req.URL.Host)
			},
		},
	}
}

func (f *docFetcher) fetch(uri string) ([]contractdoc.FetchedDocV1, error) {
	uri = strings.TrimSpace(uri)
	if uri == "" {
		return nil, nil
	}

	// Local heuristics: file://, absolute, relative, ./, ../
	if isLocalURI(uri) {
		return f.fetchLocal(uri)
	}

	u, err := url.Parse(uri)
	if err != nil {
		return nil, fmt.Errorf("invalid uri: %v", err)
	}
	if u.Scheme == "http" || u.Scheme == "https" {
		return f.fetchHTTP(u)
	}

	// Default: treat as local path
	return f.fetchLocal(uri)
}

func isLocalURI(uri string) bool {
	if strings.HasPrefix(uri, "file://") {
		return true
	}
	if strings.HasPrefix(uri, "/") || strings.HasPrefix(uri, "./") || strings.HasPrefix(uri, "../") {
		return true
	}
	// Windows-like paths are not expected on macOS, but we keep conservative.
	if len(uri) >= 2 && (uri[1] == ':' && (uri[2:3] == "\\" || uri[2:3] == "/")) {
		return true
	}
	return false
}

func (f *docFetcher) fetchLocal(uri string) ([]contractdoc.FetchedDocV1, error) {
	pathStr := uri
	if strings.HasPrefix(uri, "file://") {
		u, err := url.Parse(uri)
		if err == nil && u.Path != "" {
			pathStr = u.Path
		} else {
			pathStr = strings.TrimPrefix(uri, "file://")
		}
	}

	fi, err := os.Stat(pathStr)
	if err != nil {
		return nil, fmt.Errorf("local path not found: %s", uri)
	}

	if fi.IsDir() {
		var out []contractdoc.FetchedDocV1
		_ = filepath.WalkDir(pathStr, func(p string, d os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return nil
			}
			if d.IsDir() {
				// Skip hidden directories for safety/noise.
				if strings.HasPrefix(d.Name(), ".") {
					return filepath.SkipDir
				}
				return nil
			}
			if strings.HasPrefix(d.Name(), ".") {
				return nil
			}
			if !isDocFileName(strings.ToLower(d.Name())) {
				return nil
			}
			b, err := os.ReadFile(p)
			if err != nil {
				return nil
			}
			out = append(out, contractdoc.FetchedDocV1{
				URI:            p,
				Title:          filepath.Base(p),
				ContentPreview: previewText(string(b)),
			})
			return nil
		})
		// Deterministic order.
		sort.Slice(out, func(i, j int) bool {
			return out[i].URI < out[j].URI
		})
		return out, nil
	}

	// Single file
	b, err := os.ReadFile(pathStr)
	if err != nil {
		return nil, fmt.Errorf("failed to read local file: %v", err)
	}
	return []contractdoc.FetchedDocV1{
		{
			URI:            pathStr,
			Title:          filepath.Base(pathStr),
			ContentPreview: previewText(string(b)),
		},
	}, nil
}

func (f *docFetcher) fetchHTTP(start *url.URL) ([]contractdoc.FetchedDocV1, error) {
	if start == nil {
		return nil, fmt.Errorf("nil url")
	}
	if start.Scheme != "http" && start.Scheme != "https" {
		return nil, fmt.Errorf("unsupported scheme: %s", start.Scheme)
	}

	// Policy gate
	if err := f.policy.RequireNetworkAllowed(start.Host); err != nil {
		return nil, err
	}

	visited := map[string]bool{}
	type item struct {
		u     *url.URL
		depth int
	}
	queue := []item{{u: start, depth: 1}}

	var out []contractdoc.FetchedDocV1

	for len(queue) > 0 {
		it := queue[0]
		queue = queue[1:]

		u := canonicalURL(it.u)
		key := u.String()
		if visited[key] {
			continue
		}
		visited[key] = true

		// Policy gate each fetch (and redirects enforced by client CheckRedirect)
		if err := f.policy.RequireNetworkAllowed(u.Host); err != nil {
			// Record as a doc with error preview to keep audit trail.
			out = append(out, contractdoc.FetchedDocV1{
				URI:            key,
				Title:          "(blocked)",
				ContentPreview: err.Error(),
			})
			continue
		}

		body, title, err := f.httpGetText(u)
		if err != nil {
			out = append(out, contractdoc.FetchedDocV1{
				URI:            key,
				Title:          "(error)",
				ContentPreview: err.Error(),
			})
			continue
		}

		out = append(out, contractdoc.FetchedDocV1{
			URI:            key,
			Title:          title,
			ContentPreview: previewText(body),
		})

		if it.depth < f.maxDepth {
			links := extractLinks(body, u)
			links = filterSameHost(links, u.Host)
			links = uniqueURLs(links)
			sort.Slice(links, func(i, j int) bool { return links[i].String() < links[j].String() })
			for _, v := range links {
				queue = append(queue, item{u: v, depth: it.depth + 1})
			}
		}
	}

	return out, nil
}

func canonicalURL(u *url.URL) *url.URL {
	if u == nil {
		return &url.URL{}
	}
	c := *u
	c.Fragment = ""
	return &c
}

func (f *docFetcher) httpGetText(u *url.URL) (body string, title string, err error) {
	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Accept", "text/html, text/plain; q=0.8")

	resp, err := f.client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	// Read a bounded amount (avoid huge pages). Keep enough for title + first previews + links.
	const maxBytes = 1_000_000
	r := io.LimitReader(resp.Body, maxBytes)
	b, err := io.ReadAll(r)
	if err != nil {
		return "", "", err
	}
	s := string(b)

	t := extractTitle(s)
	if t == "" {
		t = u.Host
	}

	return s, t, nil
}

func extractTitle(html string) string {
	re := regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)
	m := re.FindStringSubmatch(html)
	if len(m) >= 2 {
		t := strings.TrimSpace(m[1])
		// Collapse whitespace
		t = regexp.MustCompile(`\s+`).ReplaceAllString(t, " ")
		return t
	}
	return ""
}

func extractLinks(html string, base *url.URL) []*url.URL {
	re := regexp.MustCompile(`(?i)<a\s+[^>]*href=['"]([^'"]+)['"]`)
	matches := re.FindAllStringSubmatch(html, -1)
	var out []*url.URL
	for _, m := range matches {
		if len(m) < 2 {
			continue
		}
		href := strings.TrimSpace(m[1])
		if href == "" {
			continue
		}
		// Skip mailto/javascript anchors
		hl := strings.ToLower(href)
		if strings.HasPrefix(hl, "mailto:") || strings.HasPrefix(hl, "javascript:") {
			continue
		}
		u, err := url.Parse(href)
		if err != nil {
			continue
		}
		if base != nil {
			u = base.ResolveReference(u)
		}
		out = append(out, canonicalURL(u))
	}
	return out
}

func filterSameHost(urls []*url.URL, host string) []*url.URL {
	var out []*url.URL
	for _, u := range urls {
		if u == nil {
			continue
		}
		if u.Host == host && (u.Scheme == "http" || u.Scheme == "https") {
			out = append(out, u)
		}
	}
	return out
}

func uniqueURLs(urls []*url.URL) []*url.URL {
	seen := map[string]bool{}
	var out []*url.URL
	for _, u := range urls {
		if u == nil {
			continue
		}
		key := u.String()
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, u)
	}
	return out
}

func isDocFileName(lowerName string) bool {
	exts := []string{".md", ".rst", ".adoc", ".txt", ".html", ".htm", ".mdx"}
	for _, e := range exts {
		if strings.HasSuffix(lowerName, e) {
			return true
		}
	}
	if lowerName == "readme" || lowerName == "readme.md" {
		return true
	}
	return false
}

func previewText(s string) string {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return ""
	}
	lines := strings.Split(trimmed, "\n")
	if len(lines) > 40 {
		lines = lines[:40]
	}
	return strings.Join(lines, "\n")
}
