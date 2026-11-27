package core

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/brielshallouf/syncamp/internal/monitor"
	"github.com/brielshallouf/syncamp/internal/store"
)

func newTestEngine(t *testing.T, extraCfg string, profiles map[string]string) (*Engine, string) {
	t.Helper()
	dir := t.TempDir()
	pd := filepath.Join(dir, "profiles")
	_ = os.MkdirAll(pd, 0o755)
	for n, days := range profiles {
		_ = os.WriteFile(filepath.Join(pd, n+".syncrify"), []byte("<p><schedulerDays>"+days+"</schedulerDays></p>"), 0o644)
	}
	cfg := `{"clientPath":"/bin/echo","profileDirs":["` + pd + `"]` + extraCfg + `}`
	cp := filepath.Join(dir, "config.json")
	_ = os.WriteFile(cp, []byte(cfg), 0o600)
	e, err := New(cp)
	if err != nil {
		t.Fatal(err)
	}
	return e, dir
}

func ago(h float64) time.Time { return time.Now().Add(-time.Duration(h * float64(time.Hour))) }

func addRun(t *testing.T, e *Engine, profile, status string, hoursAgo float64) {
	t.Helper()
	end := ago(hoursAgo)
	if err := e.hist.Add(store.Run{Profile: profile, Start: end, End: end, Status: status, Trigger: "manual"}); err != nil {
		t.Fatal(err)
	}
}

func prof(st Status, name string) ProfileStatus {
	for _, p := range st.Profiles {
		if p.Name == name {
			return p
		}
	}
	return ProfileStatus{}
}

func TestDefaultsAreMonitorMode(t *testing.T) {
	e, _ := newTestEngine(t, "", map[string]string{"Docs": "127"})
	if m := e.Status().Mode; m != "monitor" {
		t.Errorf("mode = %q", m)
	}
}

func TestOverdueDerivedFromProfileSchedule(t *testing.T) {
	// daily (127): threshold 24h + 12h grace = 36h. Mon-Fri (62): Fri->Mon gap 3d + 12h.
	e, _ := newTestEngine(t, "", map[string]string{"Daily": "127", "Weekdays": "62", "NoSched": "0"})
	addRun(t, e, "Daily", "success", 10)    // fresh
	addRun(t, e, "Weekdays", "success", 60) // 2.5d: fine across a weekend
	addRun(t, e, "NoSched", "success", 500) // no expectation known: never "overdue"
	st := e.Status()
	if prof(st, "Daily").Overdue || prof(st, "Weekdays").Overdue || prof(st, "NoSched").Overdue {
		t.Errorf("none should be overdue: %+v", st.Profiles)
	}
	if got := prof(st, "Daily").StaleAfter; got != "36h" {
		t.Errorf("daily threshold = %q", got)
	}

	addRun(t, e, "Daily", "failed", 1) // a failure is not a success: freshness still based on the 10h-old success
	if prof(e.Status(), "Daily").Overdue {
		t.Error("failure must not reset or trigger freshness")
	}

	e2, _ := newTestEngine(t, "", map[string]string{"Daily": "127"})
	addRun(t, e2, "Daily", "success", 40) // > 36h
	st2 := e2.Status()
	if !prof(st2, "Daily").Overdue || st2.Overall != "error" || !strings.Contains(st2.Summary, "overdue") {
		t.Errorf("40h-old success on a daily profile must be overdue: %s / %s", st2.Overall, st2.Summary)
	}
	addRun(t, e2, "Daily", "warning", 1) // "success with errors" still counts as completed
	if prof(e2.Status(), "Daily").Overdue {
		t.Error("a warning outcome is a completed backup")
	}
}

func TestNoSignalMeansNoAlarm(t *testing.T) {
	e, _ := newTestEngine(t, "", map[string]string{"Daily": "127"})
	st := e.Status() // never seen a backup: we must not cry wolf
	if prof(st, "Daily").Overdue || st.Overall == "error" || strings.Contains(st.Summary, "overdue") {
		t.Errorf("%s %+v", st.Overall, prof(st, "Daily"))
	}
}

func TestExplicitStaleAfterAndPaused(t *testing.T) {
	e, _ := newTestEngine(t, `,"profiles":{"Docs":{"staleAfter":"2h"},"Off":{"staleAfter":"2h","enabled":false}}`,
		map[string]string{"Docs": "0", "Off": "0"})
	addRun(t, e, "Docs", "success", 3)
	addRun(t, e, "Off", "success", 3)
	st := e.Status()
	if !prof(st, "Docs").Overdue {
		t.Error("explicit 2h with a 3h-old success is overdue")
	}
	if prof(st, "Off").Overdue {
		t.Error("a paused profile is never overdue")
	}
	if prof(st, "Docs").StaleSetting != "2h" {
		t.Errorf("setting = %q", prof(st, "Docs").StaleSetting)
	}
	// Explicit threshold, nothing ever succeeded, running longer than the threshold.
	e3, _ := newTestEngine(t, `,"profiles":{"Docs":{"staleAfter":"1h"}}`, map[string]string{"Docs": "0"})
	e3.started = ago(2)
	if !prof(e3.Status(), "Docs").Overdue {
		t.Error("explicit threshold with no success since start must be overdue")
	}
}

func TestMonitorModeNeverStartsBackups(t *testing.T) {
	e, _ := newTestEngine(t, `,"defaultTimes":["00:00"]`, map[string]string{"Docs": "127"})
	// Drive the scheduler decision directly: in monitor mode scheduleLoop must not fire.
	if e.Config().Mode == "scheduler" {
		t.Fatal("default must be monitor")
	}
	ctx, cancel := context.WithCancel(context.Background())
	e.Start(ctx)
	time.Sleep(200 * time.Millisecond)
	cancel()
	e.Wait()
	if n := len(e.History()); n != 0 {
		t.Errorf("monitor mode started %d backups", n)
	}
}

func TestScanLogsRecordsOnceAndSkipsOwnRuns(t *testing.T) {
	e, dir := newTestEngine(t, `,"logWatch":true`, map[string]string{"Docs": "127", "Photos": "127"})
	logs := filepath.Join(dir, "profiles", "logs")
	_ = os.MkdirAll(logs, 0o755)
	_ = os.WriteFile(filepath.Join(logs, "Docs.htm"), []byte("Backup started on Mon\nBackup completed in 3 minutes\n"), 0o644)
	_ = os.WriteFile(filepath.Join(logs, "Photos.htm"), []byte("Backup started on Mon\nBackup completed in 1 minute\n"), 0o644)
	// Photos was run by us just now: its client log must not be double-counted.
	addRun(t, e, "Photos", "success", 0)

	e.scanLogs(context.Background(), e.Config())
	e.scanLogs(context.Background(), e.Config()) // second pass: same content => no duplicates
	observed := map[string]int{}
	for _, r := range e.History() {
		if r.Trigger == "observed" {
			observed[r.Profile]++
		}
	}
	if observed["Docs"] != 1 || observed["Photos"] != 0 {
		t.Errorf("observed = %v (want Docs:1, Photos:0)", observed)
	}
	if lo := prof(e.Status(), "Docs").LastSuccess; lo == nil {
		t.Error("an observed success must count toward freshness")
	}
}

func TestMonitorModeIgnoresSyncAmpScheduleOverrides(t *testing.T) {
	// A leftover SyncAmp "interval" override describes nothing in monitor mode (the client
	// runs its own schedule), so the expectation must come from the profile: daily => 36h.
	e, _ := newTestEngine(t, `,"profiles":{"Docs":{"interval":"6h"}}`, map[string]string{"Docs": "127"})
	if got := prof(e.Status(), "Docs").StaleAfter; got != "36h" {
		t.Errorf("monitor mode: got %q want 36h", got)
	}
	e2, _ := newTestEngine(t, `,"mode":"scheduler","profiles":{"Docs":{"interval":"6h"}}`, map[string]string{"Docs": "127"})
	if got := prof(e2.Status(), "Docs").StaleAfter; got != "13h" {
		t.Errorf("scheduler mode: got %q want 13h (2x interval + 1h)", got)
	}
}

func TestServerDownIsDebounced(t *testing.T) {
	up, down := monitor.Health{Up: true}, monitor.Health{Up: false}
	s, rep := debounce(0, down)
	if rep || s != 1 {
		t.Errorf("one failure must not be reported: streak=%d report=%v", s, rep)
	}
	if s, rep = debounce(s, down); !rep || s != 2 {
		t.Errorf("second consecutive failure must be reported: streak=%d report=%v", s, rep)
	}
	if s, rep = debounce(s, up); !rep || s != 0 {
		t.Errorf("recovery is reported at once and resets: streak=%d report=%v", s, rep)
	}
	if _, rep = debounce(0, up); !rep {
		t.Error("success is always reported")
	}
}
