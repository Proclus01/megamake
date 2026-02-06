package cli

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/megamake/megamake/internal/app/wiring"
	"github.com/megamake/megamake/internal/platform/console"

	chapp "github.com/megamake/megamake/internal/domains/chat/app"
)

// runChatRunAsync implements:
//
//	megamake chat run_async --run <run_name> --message "..." [--server-url http://...]
//
// Behavior:
//   - If --server-url is provided: sends the request to the running chat server and returns job_id.
//   - If --server-url is NOT provided: starts the job in-process and (by default) follows it until done,
//     because jobs are in-memory and cannot be queried from a different process.
func runChatRunAsync(ctr wiring.Container, artifactDir string, argv []string, stdout io.Writer, stderr io.Writer) int {
	log := console.New(stderr)

	fs := flag.NewFlagSet("chat run_async", flag.ContinueOnError)
	fs.SetOutput(stderr)

	var runName string
	var message string
	var jsonOut bool

	var serverURL string

	// In-process follow mode (only meaningful when serverURL is empty).
	var follow bool
	var pollMS int
	var tailLimit int

	fs.StringVar(&runName, "run", "", "Run name (required).")
	fs.StringVar(&message, "message", "", "User message text. If empty, remaining args are joined.")
	fs.StringVar(&serverURL, "server-url", "", "If set, call a running chat server (enables persistent jobs across CLI invocations).")
	fs.BoolVar(&jsonOut, "json", false, "Output JSON instead of plain job_id.")

	fs.BoolVar(&follow, "follow", true, "In-process only: poll status/tail until done (default true). Ignored when --server-url is set.")
	fs.IntVar(&pollMS, "poll-ms", 650, "In-process only: polling interval in milliseconds.")
	fs.IntVar(&tailLimit, "tail-limit", 16384, "In-process only: bytes to fetch from job tail per poll.")

	fs.Usage = func() { writeChatRunAsyncHelp(stderr) }

	if err := fs.Parse(argv); err != nil {
		writeChatRunAsyncHelp(stderr)
		return exitUsage
	}

	runName = strings.TrimSpace(runName)
	if runName == "" {
		writeChatRunAsyncHelp(stderr)
		log.Error("chat run_async: --run is required")
		return exitUsage
	}

	if strings.TrimSpace(message) == "" {
		rest := fs.Args()
		if len(rest) > 0 {
			message = strings.Join(rest, " ")
		}
	}
	message = strings.TrimSpace(message)
	if message == "" {
		writeChatRunAsyncHelp(stderr)
		log.Error("chat run_async: --message (or trailing args) is required")
		return exitUsage
	}

	serverURL = strings.TrimSpace(serverURL)

	// -------------------------------------------------------------------------
	// Remote/server mode (persistent jobs)
	// -------------------------------------------------------------------------
	if serverURL != "" {
		type reqBody struct {
			RunName string `json:"run_name"`
			Message string `json:"message"`
		}
		type respBody struct {
			OK    bool   `json:"ok"`
			Error string `json:"error,omitempty"`
			JobID string `json:"job_id,omitempty"`
			Run   string `json:"run_name,omitempty"`
			Turn  int    `json:"turn,omitempty"`
		}

		var out respBody
		err := httpPostJSON(serverURL+"/api/chat/run_async", reqBody{RunName: runName, Message: message}, &out)
		if err != nil {
			log.Error(err.Error())
			return exitError
		}
		if !out.OK {
			log.Error("server error: " + emptyDash(out.Error))
			return exitError
		}

		if jsonOut {
			b, _ := json.MarshalIndent(out, "", "  ")
			_, _ = io.WriteString(stdout, string(b)+"\n")
		} else {
			_, _ = io.WriteString(stdout, out.JobID+"\n")
		}

		log.Info("mode: chat run_async (server)")
		log.Info("server: " + serverURL)
		log.Info("run: " + runName)
		log.Info("job: " + out.JobID + " (turn " + itoa(out.Turn) + ")")
		return exitOK
	}

	// -------------------------------------------------------------------------
	// In-process mode (non-persistent jobs)
	// -------------------------------------------------------------------------
	res, err := ctr.Chat.RunAsync(chapp.RunAsyncRequest{
		ArtifactDir:    artifactDir,
		RunName:        runName,
		Message:        message,
		TailLimitBytes: 16384,
	})
	if err != nil {
		log.Error(err.Error())
		return exitError
	}

	if jsonOut {
		b, _ := json.MarshalIndent(res, "", "  ")
		_, _ = io.WriteString(stdout, string(b)+"\n")
	} else {
		// Print job_id for reference
		_, _ = io.WriteString(stdout, res.JobID+"\n")
	}

	log.Info("mode: chat run_async (in-process)")
	log.Info("artifact dir: " + artifactDir)
	log.Info("run: " + runName)
	log.Info("job: " + res.JobID + " (turn " + itoa(res.Turn) + ")")
	log.Warn("note: in-process jobs are not queryable from another CLI invocation; use --server-url or run `chat serve` for persistent job control")

	if !follow {
		return exitOK
	}

	// Follow/poll until done (no duplicate spam: print only deltas).
	if pollMS < 100 {
		pollMS = 100
	}
	if tailLimit <= 0 {
		tailLimit = 16384
	}
	if tailLimit > 2_000_000 {
		tailLimit = 2_000_000
	}

	var lastTail string
	ticker := time.NewTicker(time.Duration(pollMS) * time.Millisecond)
	defer ticker.Stop()

	for {
		st, err := ctr.Chat.JobStatus(chapp.JobStatusRequest{JobID: res.JobID})
		if err != nil {
			log.Error("follow: status error: " + err.Error())
			return exitError
		}

		tail, err := ctr.Chat.JobTail(chapp.JobTailRequest{
			ArtifactDir: artifactDir,
			JobID:       res.JobID,
			Limit:       tailLimit,
		})
		if err != nil {
			log.Error("follow: tail error: " + err.Error())
			return exitError
		}

		// Print delta only.
		if tail.Text != "" && tail.Text != lastTail {
			delta := tail.Text
			if strings.HasPrefix(tail.Text, lastTail) {
				delta = tail.Text[len(lastTail):]
			}
			_, _ = io.WriteString(stdout, delta)
			lastTail = tail.Text
		}

		switch st.Job.Status {
		case "done":
			// Ensure newline at end.
			if lastTail != "" && !strings.HasSuffix(lastTail, "\n") {
				_, _ = io.WriteString(stdout, "\n")
			}
			log.Info("follow: done")
			return exitOK
		case "error":
			log.Error("follow: job error: " + emptyDash(st.Job.Error))
			return exitError
		}

		<-ticker.C
	}
}

// runChatJobs implements:
//
//	megamake chat jobs status --job-id <id> --server-url http://...
//	megamake chat jobs tail   --job-id <id> --server-url http://...
//
// Note: Without --server-url these commands cannot work across invocations because the job queue is in-memory.
func runChatJobs(ctr wiring.Container, artifactDir string, argv []string, stdout io.Writer, stderr io.Writer) int {
	_ = ctr
	_ = artifactDir

	log := console.New(stderr)

	if len(argv) == 0 {
		writeChatJobsHelp(stderr)
		return exitUsage
	}

	sub := argv[0]
	args := argv[1:]

	switch sub {
	case "status":
		fs := flag.NewFlagSet("chat jobs status", flag.ContinueOnError)
		fs.SetOutput(stderr)

		var jobID string
		var jsonOut bool
		var serverURL string

		fs.StringVar(&jobID, "job-id", "", "Job ID (required).")
		fs.StringVar(&serverURL, "server-url", "", "Chat server base URL (required for jobs).")
		fs.BoolVar(&jsonOut, "json", true, "Output JSON (default true).")

		fs.Usage = func() { writeChatJobsStatusHelp(stderr) }

		if err := fs.Parse(args); err != nil {
			writeChatJobsStatusHelp(stderr)
			return exitUsage
		}

		jobID = strings.TrimSpace(jobID)
		serverURL = strings.TrimSpace(serverURL)
		if jobID == "" || serverURL == "" {
			writeChatJobsStatusHelp(stderr)
			if jobID == "" {
				log.Error("chat jobs status: --job-id is required")
			}
			if serverURL == "" {
				log.Error("chat jobs status: --server-url is required (jobs are in-memory on the server)")
			}
			return exitUsage
		}

		// GET JSON
		var out any
		err := httpGetJSON(serverURL+"/api/chat/jobs/status?job_id="+urlQueryEscape(jobID), &out)
		if err != nil {
			log.Error(err.Error())
			return exitError
		}

		if jsonOut {
			b, _ := json.MarshalIndent(out, "", "  ")
			_, _ = io.WriteString(stdout, string(b)+"\n")
		} else {
			// Human mode not implemented here; keep it simple.
			b, _ := json.MarshalIndent(out, "", "  ")
			_, _ = io.WriteString(stdout, string(b)+"\n")
		}

		log.Info("mode: chat jobs status (server)")
		log.Info("server: " + serverURL)
		log.Info("job: " + jobID)
		return exitOK

	case "tail":
		fs := flag.NewFlagSet("chat jobs tail", flag.ContinueOnError)
		fs.SetOutput(stderr)

		var jobID string
		var limit int
		var serverURL string

		fs.StringVar(&jobID, "job-id", "", "Job ID (required).")
		fs.StringVar(&serverURL, "server-url", "", "Chat server base URL (required for jobs).")
		fs.IntVar(&limit, "limit", 16384, "Max bytes to return from tail.")

		fs.Usage = func() { writeChatJobsTailHelp(stderr) }

		if err := fs.Parse(args); err != nil {
			writeChatJobsTailHelp(stderr)
			return exitUsage
		}

		jobID = strings.TrimSpace(jobID)
		serverURL = strings.TrimSpace(serverURL)
		if jobID == "" || serverURL == "" {
			writeChatJobsTailHelp(stderr)
			if jobID == "" {
				log.Error("chat jobs tail: --job-id is required")
			}
			if serverURL == "" {
				log.Error("chat jobs tail: --server-url is required (jobs are in-memory on the server)")
			}
			return exitUsage
		}

		url := serverURL + "/api/chat/jobs/tail?job_id=" + urlQueryEscape(jobID) + "&limit=" + itoa(limit)
		txt, err := httpGetText(url)
		if err != nil {
			log.Error(err.Error())
			return exitError
		}
		_, _ = io.WriteString(stdout, txt)
		if txt != "" && !strings.HasSuffix(txt, "\n") {
			_, _ = io.WriteString(stdout, "\n")
		}

		log.Info("mode: chat jobs tail (server)")
		log.Info("server: " + serverURL)
		log.Info("job: " + jobID)
		return exitOK

	default:
		log.Error("unknown chat jobs subcommand: " + sub)
		writeChatJobsHelp(stderr)
		return exitUsage
	}
}

func writeChatRunAsyncHelp(w io.Writer) {
	help := strings.TrimSpace(`
megamake chat run_async --run <run_name> --message "..." [flags]

Modes:
  - Default (no --server-url):
      Starts a job in-process and (by default) follows it until done.
      NOTE: job IDs are NOT queryable from separate CLI invocations.
  - With --server-url:
      Submits the job to a running server (persistent jobs) and returns job_id.

Flags:
  --run NAME              (required)
  --message TEXT          If omitted, remaining args are joined as message text.
  --server-url URL        Use a running chat server (enables persistent jobs).
  --json                  Output JSON instead of plain job_id.

In-process follow flags (ignored with --server-url):
  --follow=true|false
  --poll-ms N
  --tail-limit N
`)
	_, _ = io.WriteString(w, help+"\n")
}

func writeChatJobsHelp(w io.Writer) {
	help := strings.TrimSpace(`
megamake chat jobs <subcommand>

Subcommands:
  status
  tail

Note:
  These commands require --server-url because jobs are held in memory by the server process.
`)
	_, _ = io.WriteString(w, help+"\n")
}

func writeChatJobsStatusHelp(w io.Writer) {
	help := strings.TrimSpace(`
megamake chat jobs status --job-id <id> --server-url <url> [flags]

Flags:
  --job-id ID         (required)
  --server-url URL    (required)
  --json=true|false
`)
	_, _ = io.WriteString(w, help+"\n")
}

func writeChatJobsTailHelp(w io.Writer) {
	help := strings.TrimSpace(`
megamake chat jobs tail --job-id <id> --server-url <url> [flags]

Flags:
  --job-id ID         (required)
  --server-url URL    (required)
  --limit N           (default: 16384 bytes)
`)
	_, _ = io.WriteString(w, help+"\n")
}

func httpPostJSON(url string, body any, out any) error {
	b, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("http: marshal request: %v", err)
	}
	req, err := http.NewRequest("POST", url, bytes.NewReader(b))
	if err != nil {
		return fmt.Errorf("http: new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("http: post failed: %v", err)
	}
	defer resp.Body.Close()

	rb, _ := io.ReadAll(io.LimitReader(resp.Body, 2_000_000))
	if out != nil {
		_ = json.Unmarshal(rb, out)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("http: %s returned status %d: %s", url, resp.StatusCode, strings.TrimSpace(string(rb)))
	}
	return nil
}

func httpGetJSON(url string, out any) error {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("http: new request: %v", err)
	}
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("http: get failed: %v", err)
	}
	defer resp.Body.Close()

	rb, _ := io.ReadAll(io.LimitReader(resp.Body, 2_000_000))
	if out != nil {
		_ = json.Unmarshal(rb, out)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("http: %s returned status %d: %s", url, resp.StatusCode, strings.TrimSpace(string(rb)))
	}
	return nil
}

func httpGetText(url string) (string, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("http: new request: %v", err)
	}
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("http: get failed: %v", err)
	}
	defer resp.Body.Close()

	rb, _ := io.ReadAll(io.LimitReader(resp.Body, 2_000_000))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("http: %s returned status %d: %s", url, resp.StatusCode, strings.TrimSpace(string(rb)))
	}
	return string(rb), nil
}

// Minimal URL query escaping for job IDs (only needs to handle a small safe subset).
func urlQueryEscape(s string) string {
	// This job id format is already URL-friendly; still escape spaces just in case.
	return strings.ReplaceAll(s, " ", "%20")
}
