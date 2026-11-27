// Package config loads, validates and saves SyncAmp's JSON configuration.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/brielshallouf/syncamp/internal/platform"
)

// Override customises one Syncrify profile (matched by profile name).
type Override struct {
	Enabled  *bool    `json:"enabled,omitempty"`  // false = never auto-run
	Days     []string `json:"days,omitempty"`     // "sun".."sat"; replaces the profile's schedulerDays
	Times    []string `json:"times,omitempty"`    // "HH:MM" local time
	Interval string   `json:"interval,omitempty"` // e.g. "4h"; alternative to days/times
	// StaleAfter: raise an "overdue" alert if no successful backup is seen within this
	// long ("auto" or empty = derive from the profile schedule / global default).
	StaleAfter string `json:"staleAfter,omitempty"`
}

// Server is the Syncrify server this client backs up to. It is only used for a
// connection check (reachability from this machine); SyncAmp never administers it.
type Server struct {
	URL           string `json:"url,omitempty"` // e.g. https://backup.lan:5800
	CheckInterval string `json:"checkInterval,omitempty"`
}

// Config is the on-disk configuration. It is trusted input: whoever can write
// it controls what the privileged service executes, so it is stored 0600 /
// Administrators-only and validated on every load.
type Config struct {
	ListenAddr      string   `json:"listenAddr"`      // must be loopback
	ClientPath      string   `json:"clientPath"`      // Syncrify client launcher; auto-detected if empty
	ClientArgs      []string `json:"clientArgs"`      // {profile} is replaced by the profile path
	ProfileDirs     []string `json:"profileDirs"`     // extra dirs holding *.syncrify
	DefaultTimes    []string `json:"defaultTimes"`    // used when a profile has days but no time
	JobTimeout      string   `json:"jobTimeout"`      // per backup, e.g. "12h"
	FailurePatterns []string `json:"failurePatterns"` // regexes in client output that mean failure
	WarnPatterns    []string `json:"warnPatterns"`    // regexes that mean "finished, but with errors" (not a failure)
	// Mode: "monitor" (default) - observe, verify and run on demand while the Syncrify
	// client keeps its own schedule; "scheduler" - SyncAmp also starts scheduled backups
	// (use only if the client's own scheduler / Task Scheduler / cron are disabled).
	Mode string `json:"mode"`
	// StaleAfter is the global "no successful backup within" threshold ("" = derive per
	// profile from its weekday schedule).
	StaleAfter string `json:"staleAfter,omitempty"`
	// LogWatch (experimental, off by default) reads the Syncrify client's HTML logs in
	// <data folder>/logs to learn about backups SyncAmp did not start itself.
	LogWatch        bool                `json:"logWatch,omitempty"`
	LogDirs         []string            `json:"logDirs,omitempty"`         // extra folders to scan for logs
	SuccessPatterns []string            `json:"successPatterns,omitempty"` // log lines meaning a backup finished OK
	Webhook         string              `json:"webhook,omitempty"`
	HistoryLimit    int                 `json:"historyLimit"`
	Server          Server              `json:"server"`
	Profiles        map[string]Override `json:"profiles,omitempty"`

	// Dangerous opt-ins, all default false.
	ScanAllUserHomes     bool `json:"scanAllUserHomes,omitempty"`     // see platform.OwnProfileDirs
	AllowInsecureWebhook bool `json:"allowInsecureWebhook,omitempty"` // permit http:// webhooks
	AllowInsecureClient  bool `json:"allowInsecureClient,omitempty"`  // run client even if it is user-writable
}

// Default returns a config with sensible, safe defaults.
func Default() Config {
	return Config{
		ListenAddr:   "127.0.0.1:5801",
		ClientArgs:   []string{"-console", "{profile}"},
		DefaultTimes: []string{"02:00"},
		JobTimeout:   "12h",
		// Taken from the message strings shipped in Syncrify's own resource bundle
		// (SyncrifyBundle_en.properties). Whether the console client prints each of
		// them to stdout (vs only its log file) is unverified; tune against real output.
		FailurePatterns: []string{
			`(?i)backup failed to run`,
			`(?i)but encountered one or more errors`,
			`(?i)unable to connect to the server`,
			`(?i)authentication failed`,
			`(?i)unable to backup `,
			`(?i)unable to read backup profile`,
			`(?i)ran out of client licenses`,
			`(?i)internal server error`,
			`(?i)unexpected error occurred when connecting`,
			`(?i)connection refused`,
		},
		// Syncrify's own "Success with errors" outcome, and its profile lock (another
		// run - e.g. the client's built-in scheduler - already holds the profile).
		WarnPatterns: []string{
			`(?i)completed with one or more error`,
			`(?i)unable to lock profile`,
			`(?i)has locked the current profile`,
		},
		Mode:            "monitor",
		SuccessPatterns: []string{`(?i)backup completed in`},
		HistoryLimit:    500,
		Server:          Server{CheckInterval: "1m"},
	}
}

var hhmm = regexp.MustCompile(`^([01]\d|2[0-3]):[0-5]\d$`)

// Validate rejects unsafe or malformed settings. The API listener MUST be
// loopback: SyncAmp has no TLS, so exposing it would send the API token and
// backup status in clear text.
func (c Config) Validate() error {
	host, _, err := net.SplitHostPort(c.ListenAddr)
	if err != nil {
		return fmt.Errorf("listenAddr: %w", err)
	}
	if host != "localhost" {
		ip := net.ParseIP(host)
		if ip == nil || !ip.IsLoopback() {
			return fmt.Errorf("listenAddr %q must be a loopback address (127.0.0.1, ::1 or localhost)", c.ListenAddr)
		}
	}
	if c.ClientPath != "" && !filepath.IsAbs(c.ClientPath) {
		return errors.New("clientPath must be an absolute path (no PATH lookup for privileged execution)")
	}
	if c.Mode != "" && c.Mode != "monitor" && c.Mode != "scheduler" {
		return fmt.Errorf("mode must be \"monitor\" or \"scheduler\", got %q", c.Mode)
	}
	if err := validStale(c.StaleAfter); err != nil {
		return fmt.Errorf("staleAfter: %w", err)
	}
	for _, p := range append(append(append([]string{}, c.FailurePatterns...), c.WarnPatterns...), c.SuccessPatterns...) {
		if _, err := regexp.Compile(p); err != nil {
			return fmt.Errorf("pattern %q: %w", p, err)
		}
	}
	for _, t := range c.DefaultTimes {
		if !hhmm.MatchString(t) {
			return fmt.Errorf("defaultTimes: %q is not HH:MM", t)
		}
	}
	for n, o := range c.Profiles {
		for _, t := range o.Times {
			if !hhmm.MatchString(t) {
				return fmt.Errorf("profiles[%q].times: %q is not HH:MM", n, t)
			}
		}
		if err := validStale(o.StaleAfter); err != nil {
			return fmt.Errorf("profiles[%q].staleAfter: %w", n, err)
		}
		if o.Interval != "" {
			if d, err := time.ParseDuration(o.Interval); err != nil || d < time.Minute {
				return fmt.Errorf("profiles[%q].interval %q must be a duration >= 1m", n, o.Interval)
			}
		}
	}
	if c.Webhook != "" {
		u, err := url.Parse(c.Webhook)
		if err != nil || u.Host == "" {
			return errors.New("webhook is not a valid URL")
		}
		if u.Scheme != "https" && !(u.Scheme == "http" && c.AllowInsecureWebhook) {
			return errors.New("webhook must be https:// (set allowInsecureWebhook to override)")
		}
	}
	if c.Server.URL != "" {
		if u, err := url.Parse(c.Server.URL); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return errors.New("server.url must be an http(s) URL")
		}
	}
	return nil
}

// Load reads path; a missing file yields defaults (and is not an error).
// The result is validated.
func Load(path string) (Config, error) {
	c := Default()
	b, err := os.ReadFile(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
	case err != nil:
		return c, err
	default:
		dec := json.NewDecoder(bytesReader(b))
		dec.DisallowUnknownFields() // a typo in a security setting must not be silently ignored
		if err := dec.Decode(&c); err != nil {
			return Default().finish(), fmt.Errorf("parse %s: %w", path, err)
		}
	}
	c = c.finish()
	return c, c.Validate()
}

func (c Config) finish() Config {
	if c.ClientPath == "" {
		c.ClientPath = platform.FindClient()
	}
	dirs := append([]string{}, c.ProfileDirs...)
	dirs = append(dirs, platform.OwnProfileDirs()...)
	if c.ScanAllUserHomes {
		dirs = append(dirs, platform.AllUserProfileDirs()...)
	}
	c.ProfileDirs = dirs
	return c
}

// Save validates and writes the config atomically with owner-only permissions.
func Save(path string, c Config) error {
	if err := c.Validate(); err != nil {
		return err
	}
	if err := platform.SecureDir(filepath.Dir(path)); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(c, "", "  ")
	return platform.WriteSecret(path, b)
}

// Dur parses a duration, returning def on empty/invalid input.
func Dur(s string, def time.Duration) time.Duration {
	if d, err := time.ParseDuration(s); err == nil && d > 0 {
		return d
	}
	return def
}

// LoadRaw reads the stored file exactly as written (no auto-detected paths
// merged in) so CLI edits don't persist derived values. Missing file => defaults.
func LoadRaw(path string) (Config, error) {
	c := Default()
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return c, nil
	}
	if err != nil {
		return c, err
	}
	dec := json.NewDecoder(bytesReader(b))
	dec.DisallowUnknownFields()
	return c, dec.Decode(&c)
}

// validStale accepts "", "auto" or a duration of at least one hour.
func validStale(s string) error {
	if s == "" || s == "auto" {
		return nil
	}
	if d, err := time.ParseDuration(s); err != nil || d < time.Hour {
		return fmt.Errorf("%q must be \"auto\" or a duration >= 1h", s)
	}
	return nil
}
