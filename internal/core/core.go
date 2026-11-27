// Package core is the SyncAmp engine: it ties discovery, scheduling, running,
// history, monitoring and notifications together. The service, CLI and API all
// drive this one type.
package core

import (
	"context"
	"errors"
	"fmt"
	"log"
	"path/filepath"
	"sync"
	"time"

	"github.com/brielshallouf/syncamp/internal/audit"
	"github.com/brielshallouf/syncamp/internal/config"
	"github.com/brielshallouf/syncamp/internal/monitor"
	"github.com/brielshallouf/syncamp/internal/notify"
	"github.com/brielshallouf/syncamp/internal/platform"
	"github.com/brielshallouf/syncamp/internal/profiles"
	"github.com/brielshallouf/syncamp/internal/runner"
	"github.com/brielshallouf/syncamp/internal/scheduler"
	"github.com/brielshallouf/syncamp/internal/store"
)

// Engine is the running SyncAmp core.
type Engine struct {
	cfgPath string
	hist    *store.Store

	cfgMu   sync.Mutex // serialises config file read-modify-write
	mu      sync.RWMutex
	cfg     config.Config
	profs   []profiles.Profile
	skipped []profiles.Skipped
	running map[string]bool
	lastRun map[string]time.Time
	health  *monitor.Health
	wasUp   *bool
	started time.Time
	baseCtx context.Context
	wg      sync.WaitGroup
}

// New builds an engine from the config at cfgPath.
func New(cfgPath string) (*Engine, error) {
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	hist, err := store.Open(filepath.Join(filepath.Dir(cfgPath), "history.json"), cfg.HistoryLimit)
	if err != nil {
		return nil, fmt.Errorf("open history: %w", err)
	}
	e := &Engine{cfgPath: cfgPath, cfg: cfg, hist: hist, running: map[string]bool{}, lastRun: map[string]time.Time{}, started: time.Now()}
	e.Reload()
	return e, nil
}

// Reload re-reads config and re-scans profiles (picks up profiles created in the Syncrify GUI).
func (e *Engine) Reload() {
	cfg, err := config.Load(e.cfgPath)
	e.mu.Lock()
	defer e.mu.Unlock()
	if err == nil {
		e.cfg = cfg
	} else {
		log.Printf("reload config: %v (keeping previous)", err)
	}
	e.profs, e.skipped = profiles.DiscoverReport(e.cfg.ProfileDirs)
	for _, p := range e.profs {
		if r, ok := e.hist.Last(p.Name); ok {
			e.lastRun[p.Name] = r.End
		}
	}
}

// Start launches background loops; they stop when ctx is cancelled.
func (e *Engine) Start(ctx context.Context) {
	e.baseCtx = ctx
	e.wg.Add(2)
	go func() { defer e.wg.Done(); e.scheduleLoop(ctx) }()
	go func() { defer e.wg.Done(); e.monitorLoop(ctx) }()
}

// Wait blocks until background loops and in-flight backups finish.
func (e *Engine) Wait() { e.wg.Wait() }

func (e *Engine) scheduleLoop(ctx context.Context) {
	prev := time.Now()
	t := time.NewTicker(15 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			if now.Sub(prev) > time.Minute {
				e.Reload() // also covers resume-from-sleep
			}
			e.mu.RLock()
			profs := append([]profiles.Profile(nil), e.profs...)
			e.mu.RUnlock()
			for _, p := range profs {
				spec, ok := e.specFor(p)
				if !ok {
					continue
				}
				e.mu.RLock()
				last := e.lastRun[p.Name]
				e.mu.RUnlock()
				if last.IsZero() && spec.Interval > 0 {
					last = e.started
				}
				if spec.Due(prev, now, last) {
					if err := e.RunProfile(p.Name, "schedule"); err != nil {
						log.Printf("schedule %s: %v", p.Name, err)
					}
				}
			}
			prev = now
		}
	}
}

// specFor merges profile schedule and config override. ok=false => manual only.
func (e *Engine) specFor(p profiles.Profile) (scheduler.Spec, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.specForLocked(p)
}

// specForLocked requires e.mu to be held (read or write). ok=false means "do
// not auto-run" (manual-only, or paused); the returned spec is still the
// configured schedule so the UI can show what resumes when un-paused.
func (e *Engine) specForLocked(p profiles.Profile) (scheduler.Spec, bool) {
	ov := e.cfg.Profiles[p.Name]
	paused := ov.Enabled != nil && !*ov.Enabled
	spec := scheduler.Spec{Days: p.Days, Times: e.cfg.DefaultTimes}
	if d := config.Dur(ov.Interval, 0); ov.Interval != "" && d > 0 {
		return scheduler.Spec{Interval: d}, !paused
	}
	if len(ov.Days) > 0 {
		if d, err := scheduler.ParseDays(ov.Days); err == nil {
			spec.Days = d
		}
	}
	if len(ov.Times) > 0 {
		spec.Times = ov.Times
	}
	return spec, !paused && len(spec.Days) > 0 && len(spec.Times) > 0
}

// RunProfile starts a backup asynchronously. Returns an error if the profile
// is unknown or already running (a profile never runs twice concurrently).
func (e *Engine) RunProfile(name, trigger string) error {
	e.mu.Lock()
	var prof *profiles.Profile
	for i := range e.profs {
		if e.profs[i].Name == name {
			prof = &e.profs[i]
		}
	}
	if prof == nil {
		e.mu.Unlock()
		return fmt.Errorf("unknown profile %q", name)
	}
	if e.running[name] {
		e.mu.Unlock()
		return fmt.Errorf("profile %q is already running", name)
	}
	e.running[name] = true
	cfg, path := e.cfg, prof.Path
	ctx := e.baseCtx
	if ctx == nil {
		ctx = context.Background()
	}
	e.wg.Add(1)
	e.mu.Unlock()

	go func() {
		defer e.wg.Done()
		log.Printf("backup %q started (%s)", name, trigger)
		opts := runner.Options{
			ClientPath: cfg.ClientPath, Args: cfg.ClientArgs,
			Timeout: config.Dur(cfg.JobTimeout, 12*time.Hour), FailRegex: runner.Compile(cfg.FailurePatterns),
		}
		// Least privilege: a root service runs each backup as the profile's
		// owner, so a profile can never cause files to be read that its owner
		// could not read themselves.
		if platform.IsRoot() {
			if uid, gid, ok := platform.FileOwner(path); ok && uid != 0 {
				opts.RunAs = &runner.Cred{UID: uid, GID: gid}
			}
		}
		var res store.Run
		if bad, why := platform.UntrustedWritable(cfg.ClientPath); bad && !cfg.AllowInsecureClient {
			res = store.Run{Profile: name, Start: time.Now(), End: time.Now(), Trigger: trigger, Status: "failed",
				Error: "refusing to run client: " + why}
		} else {
			res = runner.Run(ctx, name, path, trigger, opts)
		}
		if err := e.hist.Add(res); err != nil {
			log.Printf("save history: %v", err)
		}
		e.mu.Lock()
		delete(e.running, name)
		e.lastRun[name] = res.End
		e.mu.Unlock()
		log.Printf("backup %q finished: %s %s", name, res.Status, res.Error)
		if res.Status != "success" {
			_ = notify.Send(context.Background(), cfg.Webhook, fmt.Sprintf("Backup %q %s: %s", name, res.Status, res.Error))
		}
	}()
	return nil
}

// RunAll triggers every profile; returns how many started.
func (e *Engine) RunAll(trigger string) int {
	n := 0
	for _, p := range e.Profiles() {
		if e.RunProfile(p.Name, trigger) == nil {
			n++
		}
	}
	return n
}

func (e *Engine) monitorLoop(ctx context.Context) {
	for {
		e.mu.RLock()
		url, iv := e.cfg.Server.URL, config.Dur(e.cfg.Server.CheckInterval, time.Minute)
		webhook := e.cfg.Webhook
		e.mu.RUnlock()
		if url != "" {
			h := monitor.Check(ctx, url)
			e.mu.Lock()
			e.health = &h
			changed := e.wasUp != nil && *e.wasUp != h.Up
			up := h.Up
			e.wasUp = &up
			e.mu.Unlock()
			if changed {
				msg := "Connection to the Syncrify server is restored"
				if !h.Up {
					msg = "Can't reach the Syncrify server: " + h.Error
				}
				_ = notify.Send(ctx, webhook, msg)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(iv):
		}
	}
}

// Profiles returns the discovered profiles.
func (e *Engine) Profiles() []profiles.Profile {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return append([]profiles.Profile(nil), e.profs...)
}

// History returns run history, oldest first.
func (e *Engine) History() []store.Run { return e.hist.All() }

// ProfileStatus is the per-profile view for the UI.
type ProfileStatus struct {
	Name      string     `json:"name"`
	Path      string     `json:"path"`
	Running   bool       `json:"running"`
	Scheduled bool       `json:"scheduled"`
	Paused    bool       `json:"paused"`   // explicitly disabled in config
	Schedule  string     `json:"schedule"` // human summary, e.g. "Every 6h"
	Preset    string     `json:"preset"`   // hourly | every6h | daily | profile (profile's own / custom)
	Last      *store.Run `json:"last,omitempty"`
}

// Status is the whole-system summary.
type Status struct {
	Overall  string             `json:"overall"` // ok | warn | error | running | idle
	Summary  string             `json:"summary"`
	OS       string             `json:"os"`
	Client   string             `json:"client"`
	ClientOK bool               `json:"clientFound"`
	Uptime   string             `json:"uptime"`
	Server   *monitor.Health    `json:"server,omitempty"`
	Profiles []ProfileStatus    `json:"profiles"`
	Skipped  []profiles.Skipped `json:"skippedProfiles,omitempty"`
	Findings []audit.Finding    `json:"findings,omitempty"` // WARN/FAIL security findings
}

// Status computes the current summary. Rules: any running => running; server
// down or any profile's last run failed => error; no profiles/no client => warn.
func (e *Engine) Status() Status {
	profs := e.Profiles()
	e.mu.RLock()
	defer e.mu.RUnlock()
	st := Status{OS: platform.OSName(), Client: e.cfg.ClientPath, ClientOK: e.cfg.ClientPath != "",
		Uptime: time.Since(e.started).Round(time.Second).String(), Server: e.health}
	st.Skipped = e.skipped
	for _, f := range audit.Check(e.cfgPath, e.cfg) {
		if f.Level == audit.Warn || f.Level == audit.Fail {
			st.Findings = append(st.Findings, f)
		}
	}
	st.Profiles = []ProfileStatus{} // JSON [] not null, for every client
	failed, running := 0, 0
	for _, p := range profs {
		ps := ProfileStatus{Name: p.Name, Path: p.Path, Running: e.running[p.Name]}
		spec, sched := e.specForLocked(p)
		ps.Scheduled = sched
		if ov := e.cfg.Profiles[p.Name]; ov.Enabled != nil && !*ov.Enabled {
			ps.Paused = true
		}
		ps.Schedule, ps.Preset = "Manual", "profile"
		if sched || (ps.Paused && (spec.Interval > 0 || (len(spec.Days) > 0 && len(spec.Times) > 0))) {
			ps.Schedule = spec.Describe()
			switch {
			case spec.Interval == time.Hour:
				ps.Preset = "hourly"
			case spec.Interval == 6*time.Hour:
				ps.Preset = "every6h"
			case spec.Interval == 0 && len(spec.Days) == 7 && len(e.cfg.Profiles[p.Name].Times) == 0:
				ps.Preset = "daily"
			}
		}
		if r, ok := e.hist.Last(p.Name); ok {
			rr := r
			ps.Last = &rr
			if r.Status != "success" {
				failed++
			}
		}
		if ps.Running {
			running++
		}
		st.Profiles = append(st.Profiles, ps)
	}
	switch {
	case running > 0:
		st.Overall, st.Summary = "running", fmt.Sprintf("%d backup(s) running", running)
	case failed > 0:
		st.Overall, st.Summary = "error", fmt.Sprintf("%d profile(s) failed on last run", failed)
	case st.Server != nil && !st.Server.Up:
		st.Overall, st.Summary = "error", "Can't reach the Syncrify server"
	case hasLevel(st.Findings, audit.Fail):
		st.Overall, st.Summary = "error", "configuration problem: run `syncamp doctor`"
	case !st.ClientOK:
		st.Overall, st.Summary = "warn", "Syncrify client not found"
	case len(profs) == 0:
		st.Overall, st.Summary = "warn", "No Syncrify profiles found"
	case hasLevel(st.Findings, audit.Warn):
		st.Overall, st.Summary = "warn", "security warnings: run `syncamp doctor`"
	default:
		st.Overall, st.Summary = "ok", fmt.Sprintf("%d profile(s) healthy", len(profs))
	}
	return st
}

func hasLevel(fs []audit.Finding, l string) bool {
	for _, f := range fs {
		if f.Level == l {
			return true
		}
	}
	return false
}

// Config returns a copy of the effective configuration.
func (e *Engine) Config() config.Config {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.cfg
}

// CfgPath returns the config file path.
func (e *Engine) CfgPath() string { return e.cfgPath }

// ErrUnknownProfile is returned when a name matches no discovered profile.
var ErrUnknownProfile = errors.New("unknown profile")

// ErrBadPreset is returned for an unsupported schedule preset.
var ErrBadPreset = errors.New("unknown preset (use hourly, every6h, daily or profile)")

// UpdateSchedule changes a profile's override in the config file and reloads.
// enabled (optional) pauses/resumes automatic runs; preset (optional) picks a
// schedule: hourly, every6h, daily (every day at the default time), or
// profile (revert to the schedule stored in the Syncrify profile itself).
// Only discovered profiles can be edited, so the API cannot grow arbitrary config.
func (e *Engine) UpdateSchedule(name string, enabled *bool, preset string) error {
	known := false
	for _, p := range e.Profiles() {
		known = known || p.Name == name
	}
	if !known {
		return ErrUnknownProfile
	}
	e.cfgMu.Lock()
	defer e.cfgMu.Unlock()
	cfg, err := config.LoadRaw(e.cfgPath)
	if err != nil {
		return fmt.Errorf("read config: %w", err)
	}
	if cfg.Profiles == nil {
		cfg.Profiles = map[string]config.Override{}
	}
	ov := cfg.Profiles[name]
	if enabled != nil {
		ov.Enabled = enabled
	}
	switch preset {
	case "":
	case "hourly":
		ov.Interval, ov.Days, ov.Times = "1h", nil, nil
	case "every6h":
		ov.Interval, ov.Days, ov.Times = "6h", nil, nil
	case "daily":
		ov.Interval, ov.Times = "", nil
		ov.Days = []string{"sun", "mon", "tue", "wed", "thu", "fri", "sat"}
	case "profile":
		ov.Interval, ov.Days, ov.Times = "", nil, nil
	default:
		return ErrBadPreset
	}
	cfg.Profiles[name] = ov
	if err := config.Save(e.cfgPath, cfg); err != nil {
		return err
	}
	e.Reload()
	return nil
}
