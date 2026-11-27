//go:build !windows

package runner

import (
	"os/exec"
	"syscall"
	"time"
)

// prepare puts the client in its own process group so cancellation kills the
// launcher script *and* the java process it spawned.
func prepare(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = 5 * time.Second
}
