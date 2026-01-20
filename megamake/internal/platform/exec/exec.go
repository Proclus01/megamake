package exec

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type Result struct {
	ExitCode int
	Stdout   string
	Stderr   string
	TimedOut bool
}

func Which(name string) (string, bool) {
	// Prefer exec.LookPath (respects PATH), but also try some common defaults.
	p, err := exec.LookPath(name)
	if err == nil && strings.TrimSpace(p) != "" {
		return p, true
	}
	return "", false
}

// Run executes a process with a timeout and captures stdout/stderr.
// - launchPath must be an executable path (no shell expansion).
// - exitCode=124 is used for timeout, matching common conventions.
func Run(launchPath string, args []string, cwd string, timeout time.Duration) Result {
	if timeout <= 0 {
		timeout = 120 * time.Second
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, launchPath, args...)
	if strings.TrimSpace(cwd) != "" {
		cmd.Dir = cwd
	}

	var outBuf bytes.Buffer
	var errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	startErr := cmd.Start()
	if startErr != nil {
		return Result{
			ExitCode: -1,
			Stdout:   "",
			Stderr:   "Failed to start " + launchPath + ": " + startErr.Error(),
			TimedOut: false,
		}
	}

	waitErr := cmd.Wait()

	timedOut := ctx.Err() == context.DeadlineExceeded
	if timedOut {
		// Best effort: ensure process is dead.
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	}

	exitCode := 0
	if timedOut {
		exitCode = 124
	} else if waitErr != nil {
		// Extract exit code if possible.
		if ee, ok := waitErr.(*exec.ExitError); ok && ee.ProcessState != nil {
			exitCode = ee.ProcessState.ExitCode()
		} else {
			exitCode = 1
		}
	} else if cmd.ProcessState != nil {
		exitCode = cmd.ProcessState.ExitCode()
	}

	return Result{
		ExitCode: exitCode,
		Stdout:   outBuf.String(),
		Stderr:   errBuf.String(),
		TimedOut: timedOut,
	}
}

func DevNullPath() string {
	if runtime.GOOS == "windows" {
		return "NUL"
	}
	return "/dev/null"
}

func IsExecutable(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	if info.IsDir() {
		return false
	}
	// On Unix, executable bit matters; on Windows, this is heuristic.
	mode := info.Mode()
	if runtime.GOOS == "windows" {
		ext := strings.ToLower(filepath.Ext(path))
		return ext == ".exe" || ext == ".cmd" || ext == ".bat"
	}
	return mode&0o111 != 0
}
