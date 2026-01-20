package ports

import "time"

type ExecResult struct {
	ExitCode int
	Stdout   string
	Stderr   string
	TimedOut bool
}

type Exec interface {
	Which(name string) (string, bool)
	Run(launchPath string, args []string, cwd string, timeout time.Duration) ExecResult
	DevNullPath() string
}
