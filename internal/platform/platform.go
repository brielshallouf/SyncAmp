// Package platform isolates every OS-specific decision (paths, client lookup,
// browser launching) so the rest of SyncAmp stays OS-agnostic.
package platform

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// IsWindows reports whether we are running on Windows.
func IsWindows() bool { return runtime.GOOS == "windows" }

// DataDir is where SyncAmp keeps its config and history.
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

// ProfileDirs lists every directory that may hold Syncrify *.syncrify profiles.
// Syncrify stores profiles in the *user's* home (~/.syncrify), but a Windows
// service runs as SYSTEM, whose home is not the user's. So we scan every
// user home we can see, not just our own.
func ProfileDirs() []string {
	var dirs []string
	add := func(pattern string) {
		m, _ := filepath.Glob(pattern)
		dirs = append(dirs, m...)
	}
	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, filepath.Join(home, ".syncrify"))
	}
	if IsWindows() {
		sd := os.Getenv("SystemDrive")
		if sd == "" {
			sd = "C:"
		}
		add(sd + `\Users\*\.syncrify`)
	} else {
		add("/home/*/.syncrify")
		dirs = append(dirs, "/root/.syncrify")
	}
	return dirs
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

// FindClient locates the Syncrify client launcher, or returns "".
func FindClient() string {
	for _, c := range ClientCandidates() {
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			return c
		}
	}
	for _, n := range []string{"SyncrifyClient", "SyncrifyClient.sh"} {
		if p, err := exec.LookPath(n); err == nil {
			return p
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

// OSName returns a short OS/arch label for display.
func OSName() string { return runtime.GOOS + "/" + runtime.GOARCH }
