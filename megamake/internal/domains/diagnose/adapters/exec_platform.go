package adapters

import (
	"time"

	"github.com/megamake/megamake/internal/domains/diagnose/ports"
	plat "github.com/megamake/megamake/internal/platform/exec"
)

type PlatformExec struct{}

func NewPlatformExec() PlatformExec {
	return PlatformExec{}
}

func (PlatformExec) Which(name string) (string, bool) {
	return plat.Which(name)
}

func (PlatformExec) Run(launchPath string, args []string, cwd string, timeout time.Duration) ports.ExecResult {
	r := plat.Run(launchPath, args, cwd, timeout)
	return ports.ExecResult{
		ExitCode: r.ExitCode,
		Stdout:   r.Stdout,
		Stderr:   r.Stderr,
		TimedOut: r.TimedOut,
	}
}

func (PlatformExec) DevNullPath() string {
	return plat.DevNullPath()
}
