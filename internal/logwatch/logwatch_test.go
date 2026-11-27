package logwatch

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/brielshallouf/syncamp/internal/config"
	"github.com/brielshallouf/syncamp/internal/runner"
)

func pats() Patterns {
	d := config.Default()
	return Patterns{Success: runner.Compile(d.SuccessPatterns), Warn: runner.Compile(d.WarnPatterns), Fail: runner.Compile(d.FailurePatterns)}
}

func setup(t *testing.T, files map[string]string) string {
	root := t.TempDir()
	logs := filepath.Join(root, "logs")
	_ = os.MkdirAll(logs, 0o755)
	for n, c := range files {
		_ = os.WriteFile(filepath.Join(logs, n), []byte(c), 0o644)
	}
	return root
}

func TestClassifiesRealMessagesInHTML(t *testing.T) {
	root := setup(t, map[string]string{
		"Documents.htm": "<html><body><p>Backup started on Nov 27 10:00</p><br>Uploading a.doc<br><b>Backup completed in 2 minutes</b></body></html>",
		"Photos.htm":    "<p>Backup started on Nov 27 &amp; going</p><p>Unable to connect to the server backup.lan:5800</p>",
		"Laptop.htm":    "Backup started on X and completed with one or more error.\n",
	})
	got := map[string]string{}
	for _, o := range Scan([]string{root}, nil, []string{"Documents", "Photos", "Laptop"}, pats()) {
		got[o.Profile] = o.Status
	}
	want := map[string]string{"Documents": "success", "Photos": "failed", "Laptop": "warning"}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s: got %q want %q (all: %v)", k, got[k], v, got)
		}
	}
}

func TestLastMarkerWinsAndFailureBeatsWarning(t *testing.T) {
	root := setup(t, map[string]string{
		"A.htm": "Unable to connect to the server\nBackup completed in 1 minute\n",     // failed then succeeded -> success
		"B.htm": "Backup completed in 1 minute\nUnable to connect to the server\n",     // succeeded then failed -> failed
		"C.htm": "Backup completed in 1 minute and completed with one or more error\n", // same line: warn beats success
	})
	got := map[string]string{}
	for _, o := range Scan([]string{root}, nil, []string{"A", "B", "C"}, pats()) {
		got[o.Profile] = o.Status
	}
	if got["A"] != "success" || got["B"] != "failed" || got["C"] != "warning" {
		t.Errorf("%v", got)
	}
}

func TestKeyIsContentBasedNotTimeBased(t *testing.T) {
	root := setup(t, map[string]string{"D.htm": "Backup started on Mon 02:00\nBackup completed in 3 minutes\n"})
	first := Scan([]string{root}, nil, []string{"D"}, pats())
	if len(first) != 1 {
		t.Fatal(first)
	}
	// Touch the file: the same outcome must keep the same key (so it is not re-counted as fresh).
	p := filepath.Join(root, "logs", "D.htm")
	_ = os.Chtimes(p, first[0].End.Add(48*3600e9), first[0].End.Add(48*3600e9))
	again := Scan([]string{root}, nil, []string{"D"}, pats())
	if again[0].Key != first[0].Key {
		t.Error("touching a log changed its key: an old success would look new and hide an overdue backup")
	}
	// A genuinely new run (different date line) must get a new key.
	_ = os.WriteFile(p, []byte("Backup started on Mon 02:00\nBackup completed in 3 minutes\nBackup started on Tue 02:00\nBackup completed in 3 minutes\n"), 0o644)
	if next := Scan([]string{root}, nil, []string{"D"}, pats()); next[0].Key == first[0].Key {
		t.Error("a new run must produce a new key")
	}
}

func TestHostileLogFoldersAreHandledSafely(t *testing.T) {
	root := setup(t, map[string]string{"Docs.htm": "Backup completed in 1 minute\n", "Docs.exe": "Backup completed in 1 minute", "notes.htm": "Backup completed in 1 minute"})
	secret := filepath.Join(t.TempDir(), "shadow")
	_ = os.WriteFile(secret, []byte("Backup completed in 1 minute"), 0o600)
	if err := os.Symlink(secret, filepath.Join(root, "logs", "Docs2.htm")); err != nil {
		t.Skip(err)
	}
	var names []string
	for _, o := range Scan([]string{root}, nil, []string{"Docs", "Docs2"}, pats()) {
		names = append(names, filepath.Base(o.File))
	}
	if strings.Join(names, ",") != "Docs.htm" {
		t.Errorf("only the plain, attributable log may be used (no symlinks, no odd extensions, no unattributed files): %v", names)
	}
	// Big files: only the tail is read, and a >16 MiB file is ignored outright.
	big := setup(t, map[string]string{"Big.htm": strings.Repeat("x", 17<<20)})
	if got := Scan([]string{big}, nil, []string{"Big"}, pats()); len(got) != 0 {
		t.Error("oversized log must be ignored")
	}
}

func TestLongestProfileNameWins(t *testing.T) {
	root := setup(t, map[string]string{"Docs Archive.htm": "Backup completed in 1 minute\n"})
	got := Scan([]string{root}, nil, []string{"Docs", "Docs Archive"}, pats())
	if len(got) != 1 || got[0].Profile != "Docs Archive" {
		t.Errorf("%+v", got)
	}
}

func TestDetailIsRedacted(t *testing.T) {
	root := setup(t, map[string]string{"X.htm": "Unable to connect to the server password=hunter2\n"})
	got := Scan([]string{root}, nil, []string{"X"}, pats())
	if len(got) != 1 || strings.Contains(got[0].Detail, "hunter2") {
		t.Errorf("%+v", got)
	}
}
