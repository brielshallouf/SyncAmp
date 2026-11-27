// Package audit inspects the installation for security-relevant problems.
// It backs both `syncamp doctor` (headless-friendly) and the status warnings.
package audit

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"

	"github.com/brielshallouf/syncamp/internal/auth"
	"github.com/brielshallouf/syncamp/internal/config"
	"github.com/brielshallouf/syncamp/internal/platform"
)

// Level of a finding.
const (
	Fail = "FAIL" // backups will not run / system is unsafe
	Warn = "WARN"
	Info = "INFO"
	Pass = "OK"
)

// Finding is one check result.
type Finding struct {
	Level string `json:"level"`
	Msg   string `json:"msg"`
}

// Check audits the installation rooted at cfgPath.
func Check(cfgPath string, cfg config.Config) []Finding {
	var out []Finding
	add := func(l, f string, a ...any) { out = append(out, Finding{l, fmt.Sprintf(f, a...)}) }

	if err := cfg.Validate(); err != nil {
		add(Fail, "config invalid: %v", err)
	}
	// Client executable: the service runs it with its own (high) privileges.
	switch {
	case cfg.ClientPath == "":
		add(Fail, "Syncrify client not found; set clientPath")
	default:
		if _, err := os.Stat(cfg.ClientPath); err != nil {
			add(Fail, "clientPath %s is not accessible: %v", cfg.ClientPath, err)
		} else if bad, why := platform.UntrustedWritable(cfg.ClientPath); bad {
			l := Fail
			if cfg.AllowInsecureClient {
				l = Warn
			}
			add(l, "client executable can be replaced by non-admins: %s (a service running it hands them its privileges)", why)
		} else if platform.IsWindows() {
			add(Info, "verify manually that %s and its folder are not writable by standard users (icacls)", cfg.ClientPath)
		} else {
			add(Pass, "client executable permissions are safe")
		}
	}
	// Secrets at rest.
	dir := filepath.Dir(cfgPath)
	if !platform.IsWindows() {
		for _, p := range []string{dir, cfgPath, filepath.Join(dir, auth.FileName), filepath.Join(dir, "history.json")} {
			if st, err := os.Stat(p); err == nil && st.Mode().Perm()&0o077 != 0 {
				add(Warn, "%s is accessible by other users (%v); run: chmod go-rwx", p, st.Mode().Perm())
			}
		}
	}
	// Transport.
	if cfg.Server.URL == "" {
		add(Info, "server.url not set: the connection check to your Syncrify server is off")
	} else if u, err := url.Parse(cfg.Server.URL); err == nil && u.Scheme == "http" && !loopback(u.Hostname()) {
		add(Warn, "server.url uses plain http: backup traffic may cross the network unencrypted; use https")
	}
	if cfg.Webhook == "" {
		add(Info, "no webhook configured: failures will only be visible in status/history")
	}
	if cfg.ScanAllUserHomes {
		if platform.IsWindows() {
			add(Warn, "scanAllUserHomes is on: any local user can plant a profile that a SYSTEM backup will run (privileges cannot be dropped on Windows)")
		} else {
			add(Info, "scanAllUserHomes is on: backups run as each profile's owner, not root")
		}
	}
	if cfg.Mode == "scheduler" {
		add(Warn, "scheduler mode: SyncAmp starts scheduled backups. Make sure the Syncrify client's own scheduler / Task Scheduler / cron / launchd is disabled for these profiles, or runs will collide (the client locks a profile, so the second run is reported as a warning)")
	} else {
		add(Info, "monitor mode (default): the Syncrify client keeps its own schedule; SyncAmp watches, verifies and runs on demand")
	}
	if cfg.LogWatch {
		found := false
		for _, d := range append(append([]string{}, cfg.ProfileDirs...), cfg.LogDirs...) {
			if st, err := os.Stat(filepath.Join(d, "logs")); err == nil && st.IsDir() {
				found = true
			}
		}
		for _, d := range cfg.LogDirs {
			if st, err := os.Stat(d); err == nil && st.IsDir() {
				found = true
			}
		}
		if !found {
			add(Warn, "logWatch is on but no client logs folder was found next to your profiles; set logDirs. (Experimental: the client's log layout is unverified.)")
		}
	}
	known := map[string]bool{}
	for _, d := range cfg.ProfileDirs {
		known[filepath.Clean(d)] = true
	}
	for _, d := range platform.SharedDataCandidates() {
		if known[filepath.Clean(d)] {
			continue
		}
		if m, _ := filepath.Glob(filepath.Join(d, "*.syncrify")); len(m) > 0 {
			add(Info, "found %d profile(s) in %s, which may be the Syncrify client's shared data folder. To include it add it to profileDirs, after confirming only administrators can write there (a profile can run scripts).", len(m), d)
		}
	}
	if len(cfg.FailurePatterns) == 0 {
		add(Warn, "no failurePatterns: a client that prints an error but exits 0 will be recorded as success")
	}
	return out
}

func loopback(h string) bool {
	if h == "localhost" {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && ip.IsLoopback()
}
