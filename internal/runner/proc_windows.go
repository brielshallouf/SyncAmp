//go:build windows

package runner

import (
	"os/exec"
	"strconv"
	"syscall"
	"time"
)

// prepare hides the console window the service would otherwise flash, and
// makes cancellation kill the whole tree (the .exe launcher spawns java).
func prepare(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000} // CREATE_NO_WINDOW
	cmd.Cancel = func() error {
		kill := exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid))
		kill.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
		return kill.Run()
	}
	cmd.WaitDelay = 5 * time.Second
}
