// Package config loads and saves SyncAmp's JSON configuration.
package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/brielshallouf/syncamp/internal/platform"
)

// Override customises one Syncrify profile (matched by profile name).
type Override struct {
	Enabled  *bool    `json:"enabled,omitempty"`  // false = never auto-run
	Days     []string `json:"days,omitempty"`     // "sun".."sat"; replaces the profile's schedulerDays
	Times    []string `json:"times,omitempty"`    // "HH:MM" local time
	Interval string   `json:"interval,omitempty"` // e.g. "4h"; alternative to days/times
}

// Server describes the Syncrify *server* to health-check (optional).
type Server struct {
	URL           string `json:"url,omitempty"` // e.g. http://backup.lan:5800
	CheckInterval string `json:"checkInterval,omitempty"`
}

// Config is the on-disk configuration.
type Config struct {
	ListenAddr      string              `json:"listenAddr"`      // loopback only
	ClientPath      string              `json:"clientPath"`      // Syncrify client launcher; auto-detected if empty
	ClientArgs      []string            `json:"clientArgs"`      // {profile} is replaced by the profile path
	ProfileDirs     []string            `json:"profileDirs"`     // extra dirs holding *.syncrify
	DefaultTimes    []string            `json:"defaultTimes"`    // used when a profile has days but no time
	JobTimeout      string              `json:"jobTimeout"`      // per backup, e.g. "12h"
	FailurePatterns []string            `json:"failurePatterns"` // regexes in client output that mean failure
	Webhook         string              `json:"webhook,omitempty"`
	HistoryLimit    int                 `json:"historyLimit"`
	Server          Server              `json:"server"`
	Profiles        map[string]Override `json:"profiles,omitempty"`
}

// Default returns a config with sensible defaults.
func Default() Config {
	return Config{
		ListenAddr:   "127.0.0.1:5801",
		ClientArgs:   []string{"-console", "{profile}"},
		DefaultTimes: []string{"02:00"},
		JobTimeout:   "12h",
		FailurePatterns: []string{
			`(?i)backup failed`, `(?i)unable to connect`, `(?i)authentication failed`,
			`(?i)connection refused`, `(?i)login failed`,
		},
		HistoryLimit: 500,
		Server:       Server{CheckInterval: "1m"},
	}
}

// Load reads path; a missing file yields defaults (and is not an error).
func Load(path string) (Config, error) {
	c := Default()
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return c.finish(), nil
	}
	if err != nil {
		return c, err
	}
	if err := json.Unmarshal(b, &c); err != nil {
		return c, err
	}
	return c.finish(), nil
}

func (c Config) finish() Config {
	if c.ClientPath == "" {
		c.ClientPath = platform.FindClient()
	}
	c.ProfileDirs = append(append([]string{}, c.ProfileDirs...), platform.ProfileDirs()...)
	return c
}

// Save writes the config atomically.
func Save(path string, c Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(c, "", "  ")
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Dur parses a duration, returning def on empty/invalid input.
func Dur(s string, def time.Duration) time.Duration {
	if d, err := time.ParseDuration(s); err == nil && d > 0 {
		return d
	}
	return def
}
