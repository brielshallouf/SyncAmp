// Package autostart registers the tray to launch at user login.
package autostart

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

const name = "SyncAmpTray"

// Enable makes `exe tray` start at login for the current user.
func Enable(exe string) error {
	switch runtime.GOOS {
	case "windows":
		return exec.Command("reg", "add", `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`,
			"/v", name, "/t", "REG_SZ", "/d", fmt.Sprintf(`"%s" tray`, exe), "/f").Run()
	default:
		dir, err := os.UserConfigDir()
		if err != nil {
			return err
		}
		dir = filepath.Join(dir, "autostart")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		entry := fmt.Sprintf("[Desktop Entry]\nType=Application\nName=SyncAmp Tray\nExec=%q tray\nX-GNOME-Autostart-enabled=true\n", exe)
		return os.WriteFile(filepath.Join(dir, "syncamp-tray.desktop"), []byte(entry), 0o644)
	}
}

// Disable removes the login entry.
func Disable() error {
	if runtime.GOOS == "windows" {
		return exec.Command("reg", "delete", `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`, "/v", name, "/f").Run()
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return err
	}
	err = os.Remove(filepath.Join(dir, "autostart", "syncamp-tray.desktop"))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
