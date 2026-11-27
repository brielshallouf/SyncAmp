// Package platform isolates every OS-specific decision (paths, client lookup,
// file permissions) so the rest of SyncAmp stays OS-agnostic.
package platform

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// IsWindows reports whether we are running on Windows.
func IsWindows() bool { return runtime.GOOS == "windows" }

// OSName returns a short OS/arch label for display.
func OSName() string { return runtime.GOOS + "/" + runtime.GOARCH }

// DataDir is where SyncAmp keeps its config, token and history.
// Windows service (LocalSystem): %ProgramData%\SyncAmp
// Linux root/service:            /var/lib/syncamp
// Linux/other normal user:       ~/.local/share/syncamp
func DataDir() string {
	if IsWindows() {
		base := os.Getenv("ProgramData")
		if base == "" {
			base = `C:\ProgramData`
		}
		return filepath.Join(base, "SyncAmp")
	}
	if os.Geteuid() == 0 {
		return "/var/lib/syncamp"
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "syncamp")
}

// ConfigPath returns the default config file location.
func ConfigPath() string { return filepath.Join(DataDir(), "config.json") }

// OwnProfileDirs is the running account's own ~/.syncrify. This is the only
// directory scanned by default: scanning other users' homes from a privileged
// service lets any local user plant a profile that a SYSTEM/root backup then
// executes (see Config.ScanAllUserHomes).
func OwnProfileDirs() []string {
	if home, err := os.UserHomeDir(); err == nil {
		return []string{filepath.Join(home, ".syncrify")}
	}
	return nil
}

// AllUserProfileDirs lists every visible user's ~/.syncrify. Opt-in only.
func AllUserProfileDirs() []string {
	pattern, extra := "/home/*/.syncrify", []string{"/root/.syncrify"}
	if IsWindows() {
		sd := os.Getenv("SystemDrive")
		if sd == "" {
			sd = "C:"
		}
		pattern, extra = sd+`\Users\*\.syncrify`, nil
	}
	m, _ := filepath.Glob(pattern)
	return append(m, extra...)
}

// ClientCandidates returns likely Syncrify client launcher paths, best first.
func ClientCandidates() []string {
	if IsWindows() {
		var out []string
		for _, env := range []string{"ProgramFiles", "ProgramFiles(x86)"} {
			base := os.Getenv(env)
			if base == "" {
				continue
			}
			for _, d := range []string{"Syncrify Client", "Syncrify", "SyncrifyClient"} {
				for _, exe := range []string{"SyncrifyClient.exe", "BackupClient.exe", "SyncrifyBackupClient.exe"} {
					out = append(out, filepath.Join(base, d, exe))
				}
			}
		}
		return out
	}
	return []string{
		"/opt/Syncrify/SyncrifyClient.sh", "/opt/SyncrifyClient/SyncrifyClient.sh",
		"/usr/local/Syncrify/SyncrifyClient.sh", "/usr/local/bin/SyncrifyClient.sh",
	}
}

// FindClient locates the Syncrify client launcher, or returns "". PATH lookup
// is deliberately not used: a service must not execute whatever a user put
// earlier on a search path.
func FindClient() string {
	for _, c := range ClientCandidates() {
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			return c
		}
	}
	return ""
}

// OpenBrowser opens url in the user's default browser.
func OpenBrowser(url string) error {
	switch runtime.GOOS {
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		return exec.Command("open", url).Start()
	default:
		return exec.Command("xdg-open", url).Start()
	}
}
