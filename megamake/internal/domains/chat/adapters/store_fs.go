package adapters

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	contractchat "github.com/megamake/megamake/internal/contracts/v1/chat"
	"github.com/megamake/megamake/internal/domains/chat/domain"
	"github.com/megamake/megamake/internal/domains/chat/ports"
)

// FSRunStore implements the chat RunStore port using the local filesystem.
//
// Layout (artifact-dir scoped):
//
//	<artifactDir>/MEGACHAT/
//	  runs/<run_name>/args.json
//	  runs/<run_name>/meta.json
//	  runs/<run_name>/settings.json          (optional snapshot)
//	  runs/<run_name>/transcript.jsonl
//	  runs/<run_name>/user_turn_001.txt
//	  runs/<run_name>/assistant_turn_001.partial.txt
//	  runs/<run_name>/assistant_turn_001.txt
//	  runs/<run_name>/turn_001.json
//
// Plus pointer:
//
//	<artifactDir>/MEGACHAT_latest.txt        (contains latest run_name)
type FSRunStore struct{}

func NewFSRunStore() FSRunStore { return FSRunStore{} }

func (FSRunStore) CreateRun(req ports.CreateRunRequest) error {
	artifactDir := strings.TrimSpace(req.ArtifactDir)
	runName := strings.TrimSpace(req.RunName)
	if artifactDir == "" {
		return fmt.Errorf("chat store: ArtifactDir is empty")
	}
	if runName == "" {
		return fmt.Errorf("chat store: RunName is empty")
	}
	// App/service should validate, but store enforces defensively.
	if !domain.IsValidRunName(runName) {
		return fmt.Errorf("chat store: invalid run_name: %s", runName)
	}

	runDir := runDirPath(artifactDir, runName)

	// Ensure parent directories exist.
	if err := os.MkdirAll(filepath.Dir(runDir), 0o755); err != nil {
		return fmt.Errorf("chat store: failed to create runs dir: %v", err)
	}

	// Refuse to overwrite an existing run directory.
	if st, err := os.Stat(runDir); err == nil && st.IsDir() {
		return fmt.Errorf("chat store: run already exists: %s", runName)
	}

	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return fmt.Errorf("chat store: failed to create run dir: %v", err)
	}

	// Write args.json, meta.json atomically.
	if err := writeJSONAtomic(filepath.Join(runDir, "args.json"), req.Args); err != nil {
		return err
	}
	if err := writeJSONAtomic(filepath.Join(runDir, "meta.json"), req.Meta); err != nil {
		return err
	}

	// Optional per-run settings snapshot.
	if req.SettingsSnapshot != nil {
		if err := writeJSONAtomic(filepath.Join(runDir, "settings.json"), *req.SettingsSnapshot); err != nil {
			return err
		}
	}

	// Create transcript.jsonl if missing.
	transcriptPath := filepath.Join(runDir, "transcript.jsonl")
	if err := ensureFileExists(transcriptPath); err != nil {
		return err
	}

	// Update latest pointer (best-effort, but should usually succeed).
	if err := os.MkdirAll(chatRootDir(artifactDir), 0o755); err != nil {
		return fmt.Errorf("chat store: failed to create MEGACHAT dir: %v", err)
	}
	if err := os.WriteFile(latestPointerPath(artifactDir), []byte(runName+"\n"), 0o644); err != nil {
		return fmt.Errorf("chat store: failed to write latest pointer: %v", err)
	}

	return nil
}

func (FSRunStore) ListRuns(req ports.ListRunsRequest) ([]contractchat.MetaV1, error) {
	artifactDir := strings.TrimSpace(req.ArtifactDir)
	if artifactDir == "" {
		return nil, fmt.Errorf("chat store: ArtifactDir is empty")
	}

	limit := req.Limit
	if limit <= 0 {
		limit = 200
	}
	if limit > 2000 {
		limit = 2000
	}

	runsDir := runsRootDir(artifactDir)
	ents, err := os.ReadDir(runsDir)
	if err != nil {
		// If runs dir doesn't exist yet, return empty list (not an error).
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("chat store: failed to read runs dir: %v", err)
	}

	type item struct {
		meta contractchat.MetaV1
		t    time.Time
	}
	var items []item

	for _, e := range ents {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if !domain.IsValidRunName(name) {
			continue
		}

		metaPath := filepath.Join(runsDir, name, "meta.json")
		b, err := os.ReadFile(metaPath)
		if err != nil {
			continue
		}
		var m contractchat.MetaV1
		if err := json.Unmarshal(b, &m); err != nil {
			continue
		}
		tt := parseRFC3339NanoSafe(m.UpdatedTS)
		items = append(items, item{meta: m, t: tt})
	}

	// Sort most recently updated first; tie-break by run_name desc (newer names sort later lexicographically).
	sort.Slice(items, func(i, j int) bool {
		if !items[i].t.Equal(items[j].t) {
			return items[i].t.After(items[j].t)
		}
		return items[i].meta.RunName > items[j].meta.RunName
	})

	var out []contractchat.MetaV1
	for _, it := range items {
		out = append(out, it.meta)
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (FSRunStore) ReadMeta(req ports.ReadMetaRequest) (contractchat.MetaV1, error) {
	artifactDir := strings.TrimSpace(req.ArtifactDir)
	runName := strings.TrimSpace(req.RunName)
	if artifactDir == "" {
		return contractchat.MetaV1{}, fmt.Errorf("chat store: ArtifactDir is empty")
	}
	if runName == "" {
		return contractchat.MetaV1{}, fmt.Errorf("chat store: RunName is empty")
	}

	runDir := runDirPath(artifactDir, runName)
	b, err := os.ReadFile(filepath.Join(runDir, "meta.json"))
	if err != nil {
		return contractchat.MetaV1{}, fmt.Errorf("chat store: failed to read meta.json: %v", err)
	}
	var m contractchat.MetaV1
	if err := json.Unmarshal(b, &m); err != nil {
		return contractchat.MetaV1{}, fmt.Errorf("chat store: failed to parse meta.json: %v", err)
	}
	return m, nil
}

func (FSRunStore) WriteMeta(req ports.WriteMetaRequest) error {
	artifactDir := strings.TrimSpace(req.ArtifactDir)
	runName := strings.TrimSpace(req.RunName)
	if artifactDir == "" {
		return fmt.Errorf("chat store: ArtifactDir is empty")
	}
	if runName == "" {
		return fmt.Errorf("chat store: RunName is empty")
	}

	runDir := runDirPath(artifactDir, runName)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return fmt.Errorf("chat store: failed to ensure run dir: %v", err)
	}
	return writeJSONAtomic(filepath.Join(runDir, "meta.json"), req.Meta)
}

func (FSRunStore) AppendTranscriptEvent(req ports.AppendTranscriptEventRequest) error {
	artifactDir := strings.TrimSpace(req.ArtifactDir)
	runName := strings.TrimSpace(req.RunName)
	if artifactDir == "" {
		return fmt.Errorf("chat store: ArtifactDir is empty")
	}
	if runName == "" {
		return fmt.Errorf("chat store: RunName is empty")
	}

	runDir := runDirPath(artifactDir, runName)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return fmt.Errorf("chat store: failed to ensure run dir: %v", err)
	}

	p := filepath.Join(runDir, "transcript.jsonl")
	f, err := os.OpenFile(p, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("chat store: failed to open transcript.jsonl: %v", err)
	}
	defer f.Close()

	lineBytes, err := json.Marshal(req.Event)
	if err != nil {
		return fmt.Errorf("chat store: failed to marshal transcript event: %v", err)
	}

	if _, err := f.Write(append(lineBytes, '\n')); err != nil {
		return fmt.Errorf("chat store: failed to append transcript event: %v", err)
	}
	return nil
}

func (FSRunStore) ReadTranscriptTail(req ports.ReadTranscriptTailRequest) ([]contractchat.TranscriptEventV1, error) {
	artifactDir := strings.TrimSpace(req.ArtifactDir)
	runName := strings.TrimSpace(req.RunName)
	if artifactDir == "" {
		return nil, fmt.Errorf("chat store: ArtifactDir is empty")
	}
	if runName == "" {
		return nil, fmt.Errorf("chat store: RunName is empty")
	}

	limit := req.Limit
	if limit <= 0 {
		limit = 200
	}
	if limit > 5000 {
		limit = 5000
	}

	p := filepath.Join(runDirPath(artifactDir, runName), "transcript.jsonl")
	f, err := os.Open(p)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("chat store: failed to open transcript.jsonl: %v", err)
	}
	defer f.Close()

	// Keep only last N lines (events) using a ring buffer-like slice.
	lines := make([]string, 0, limit)
	r := bufio.NewReader(f)

	for {
		ln, readErr := r.ReadString('\n')
		if ln != "" {
			ln = strings.TrimSpace(ln)
			if ln != "" {
				if len(lines) < limit {
					lines = append(lines, ln)
				} else {
					// shift-left (limit is modest; OK for v1)
					copy(lines, lines[1:])
					lines[len(lines)-1] = ln
				}
			}
		}
		if readErr != nil {
			// EOF or real error; we treat any error as end-of-file.
			break
		}
	}

	var out []contractchat.TranscriptEventV1
	for _, ln := range lines {
		var ev contractchat.TranscriptEventV1
		if err := json.Unmarshal([]byte(ln), &ev); err != nil {
			return nil, fmt.Errorf("chat store: invalid transcript line: %v", err)
		}
		out = append(out, ev)
	}
	return out, nil
}

func (FSRunStore) NextTurnNumber(req ports.NextTurnNumberRequest) (int, error) {
	artifactDir := strings.TrimSpace(req.ArtifactDir)
	runName := strings.TrimSpace(req.RunName)
	if artifactDir == "" {
		return 0, fmt.Errorf("chat store: ArtifactDir is empty")
	}
	if runName == "" {
		return 0, fmt.Errorf("chat store: RunName is empty")
	}

	runDir := runDirPath(artifactDir, runName)

	ents, err := os.ReadDir(runDir)
	if err != nil {
		return 0, fmt.Errorf("chat store: failed to read run dir: %v", err)
	}

	// Determine max existing user turn number, then +1.
	re := regexp.MustCompile(`^user_turn_(\d{3})\.txt$`)
	maxTurn := 0
	for _, e := range ents {
		if e.IsDir() {
			continue
		}
		m := re.FindStringSubmatch(e.Name())
		if len(m) != 2 {
			continue
		}
		n := atoi3(m[1])
		if n > maxTurn {
			maxTurn = n
		}
	}
	return maxTurn + 1, nil
}

func (FSRunStore) WriteUserTurnText(req ports.WriteUserTurnTextRequest) error {
	return writeTurnText(req.ArtifactDir, req.RunName, req.Turn, true, req.Text)
}

func (FSRunStore) WriteAssistantPartialText(req ports.WriteAssistantPartialTextRequest) error {
	return writeTurnText(req.ArtifactDir, req.RunName, req.Turn, false, req.Text, true)
}

func (FSRunStore) WriteAssistantFinalText(req ports.WriteAssistantFinalTextRequest) error {
	return writeTurnText(req.ArtifactDir, req.RunName, req.Turn, false, req.Text, false)
}

func (FSRunStore) WriteTurnMetrics(req ports.WriteTurnMetricsRequest) error {
	artifactDir := strings.TrimSpace(req.ArtifactDir)
	runName := strings.TrimSpace(req.RunName)
	if artifactDir == "" {
		return fmt.Errorf("chat store: ArtifactDir is empty")
	}
	if runName == "" {
		return fmt.Errorf("chat store: RunName is empty")
	}
	if req.Metrics.Turn <= 0 {
		return fmt.Errorf("chat store: Metrics.Turn must be >= 1")
	}

	runDir := runDirPath(artifactDir, runName)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return fmt.Errorf("chat store: failed to ensure run dir: %v", err)
	}

	files := domain.TurnFiles(req.Metrics.Turn)
	p := filepath.Join(runDir, files.TurnMetricsFile)
	return writeJSONAtomic(p, req.Metrics)
}

////////////////////////////////////////////////////////////////////////////////
// Paths + helpers
////////////////////////////////////////////////////////////////////////////////

func chatRootDir(artifactDir string) string {
	return filepath.Join(artifactDir, "MEGACHAT")
}

func runsRootDir(artifactDir string) string {
	return filepath.Join(chatRootDir(artifactDir), "runs")
}

func runDirPath(artifactDir string, runName string) string {
	return filepath.Join(runsRootDir(artifactDir), runName)
}

func latestPointerPath(artifactDir string) string {
	return filepath.Join(artifactDir, "MEGACHAT_latest.txt")
}

func ensureFileExists(p string) error {
	// Ensure parent exists.
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return fmt.Errorf("chat store: failed to ensure parent dir for %s: %v", p, err)
	}
	// Create only if missing.
	f, err := os.OpenFile(p, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		if os.IsExist(err) {
			return nil
		}
		return fmt.Errorf("chat store: failed to create file %s: %v", p, err)
	}
	_ = f.Close()
	return nil
}

func writeJSONAtomic(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("chat store: failed to marshal json for %s: %v", path, err)
	}
	// Ensure newline for nicer diffs.
	if len(b) == 0 || b[len(b)-1] != '\n' {
		b = append(b, '\n')
	}
	return writeFileAtomic(path, b, 0o644)
}

func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("chat store: failed to ensure dir for %s: %v", path, err)
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, perm); err != nil {
		return fmt.Errorf("chat store: failed to write temp file %s: %v", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("chat store: failed to rename temp file to %s: %v", path, err)
	}
	return nil
}

// writeTurnText writes either user or assistant text files.
// For assistant, partial controls whether the ".partial.txt" name is used.
func writeTurnText(artifactDir string, runName string, turn int, isUser bool, text string, assistantPartial ...bool) error {
	artifactDir = strings.TrimSpace(artifactDir)
	runName = strings.TrimSpace(runName)
	if artifactDir == "" {
		return fmt.Errorf("chat store: ArtifactDir is empty")
	}
	if runName == "" {
		return fmt.Errorf("chat store: RunName is empty")
	}
	if turn <= 0 {
		return fmt.Errorf("chat store: turn must be >= 1")
	}

	files := domain.TurnFiles(turn)

	filename := ""
	if isUser {
		filename = files.UserTextFile
	} else {
		partial := false
		if len(assistantPartial) > 0 {
			partial = assistantPartial[0]
		}
		if partial {
			filename = files.AssistantPartialTextFile
		} else {
			filename = files.AssistantTextFile
		}
	}

	runDir := runDirPath(artifactDir, runName)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return fmt.Errorf("chat store: failed to ensure run dir: %v", err)
	}

	// Keep output stable: always end with newline for text files.
	if text != "" && !strings.HasSuffix(text, "\n") {
		text = text + "\n"
	}

	p := filepath.Join(runDir, filename)
	if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
		return fmt.Errorf("chat store: failed to write %s: %v", filename, err)
	}
	return nil
}

func parseRFC3339NanoSafe(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		// For robustness, fall back to zero time (sorting will then be stable via run_name tie-breaks).
		return time.Time{}
	}
	return t
}

func atoi3(s string) int {
	// s must be exactly 3 digits; returns 0 on invalid.
	if len(s) != 3 {
		return 0
	}
	n := 0
	for i := 0; i < 3; i++ {
		c := s[i]
		if c < '0' || c > '9' {
			return 0
		}
		n = n*10 + int(c-'0')
	}
	return n
}
