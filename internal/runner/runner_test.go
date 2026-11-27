//go:build !windows

package runner

import (
	"context"
	"github.com/brielshallouf/syncamp/internal/config"
	"os"
	"strings"
	"testing"
	"time"
)

func run(t *testing.T, script string, timeout time.Duration) string {
	t.Helper()
	r := Run(context.Background(), "p", "/x.syncrify", "manual", Options{
		ClientPath: "/bin/sh", Args: []string{"-c", script, "sh", "{profile}"},
		Timeout: timeout, FailRegex: Compile([]string{`(?i)backup failed`}),
	})
	return r.Status + "|" + r.Error
}

func TestRunnerOutcomes(t *testing.T) {
	if got := run(t, "echo done", 5*time.Second); got != "success|" {
		t.Errorf("success: %s", got)
	}
	if got := run(t, "exit 3", 5*time.Second); got != "failed|client exited with code 3" {
		t.Errorf("exit code: %s", got)
	}
	// Syncrify console can print a failure yet exit 0; we must still flag it.
	if got := run(t, "echo 'Backup FAILED: no route'; exit 0", 5*time.Second); got == "success|" {
		t.Errorf("pattern failure not detected: %s", got)
	}
	if got := run(t, "sleep 5", 200*time.Millisecond); got[:7] != "timeout" {
		t.Errorf("timeout: %s", got)
	}
}

func TestMissingClient(t *testing.T) {
	r := Run(context.Background(), "p", "x", "manual", Options{})
	if r.Status != "failed" {
		t.Error("expected failure with no client")
	}
}

func TestRedactsSecretsInStoredOutput(t *testing.T) {
	r := Run(context.Background(), "p", "/x", "manual", Options{
		ClientPath: "/bin/sh",
		Args:       []string{"-c", `echo "connecting password=hunter2 ok"; echo 'token: abc123'; echo "encryptionKey \"my key\""; echo "Authorization: Bearer xyz"`},
		Timeout:    5 * time.Second,
	})
	out := strings.Join(r.Tail, "\n")
	for _, leak := range []string{"hunter2", "abc123", "my key", "xyz"} {
		if strings.Contains(out, leak) {
			t.Errorf("secret %q leaked into history: %s", leak, out)
		}
	}
	if !strings.Contains(out, "connecting") {
		t.Errorf("non-secret text lost: %s", out)
	}
}

func TestOutputIsBounded(t *testing.T) {
	r := Run(context.Background(), "p", "/x", "manual", Options{
		ClientPath: "/bin/sh",
		Args:       []string{"-c", `i=0; while [ $i -lt 500 ]; do printf 'line%d ' $i; head -c 2000 /dev/zero | tr '\0' 'a'; echo; i=$((i+1)); done`},
		Timeout:    10 * time.Second,
	})
	if len(r.Tail) > 50 {
		t.Errorf("tail not capped: %d lines", len(r.Tail))
	}
	for _, l := range r.Tail {
		if len(l) > 400 {
			t.Fatalf("line not clipped: %d bytes", len(l))
		}
	}
}

// Needs root (the CI/sandbox user): proves the backup really runs as the
// profile owner and not as root.
func TestPrivilegeDropToProfileOwner(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("needs root to drop privileges")
	}
	dir := t.TempDir()
	_ = os.Chmod(dir, 0o755)
	r := Run(context.Background(), "p", "/x", "manual", Options{
		ClientPath: "/bin/sh",
		Args:       []string{"-c", "id -u; echo HOME=$HOME"},
		Timeout:    5 * time.Second,
		RunAs:      &Cred{UID: 65534, GID: 65534},
	})
	out := strings.Join(r.Tail, "\n")
	if r.Status != "success" || !strings.HasPrefix(out, "65534") {
		t.Errorf("child must run as uid 65534; status=%s err=%s out=%q", r.Status, r.Error, out)
	}
}

// Lines below are the product's own wording, taken from the message strings in
// Syncrify's resource bundle (LOG_*/ERROR_*/STS_* keys).
func TestDefaultPatternsAgainstRealSyncrifyMessages(t *testing.T) {
	d := config.Default()
	opts := func(line string) Options {
		return Options{ClientPath: "/bin/sh", Args: []string{"-c", "echo \"$0\"", line}, Timeout: 5 * time.Second,
			FailRegex: Compile(d.FailurePatterns), WarnRegex: Compile(d.WarnPatterns)}
	}
	cases := map[string]string{
		"Backup completed in 2 minutes": "success",
		"Total errors: 0":               "success",
		"Backup started on Nov 27 and completed with one or more error.":            "warning",
		"Unable to lock profile. Make sure another backup session is NOT running":   "warning",
		"A previous backup run has locked the current profile, which was last":      "warning",
		"Backup started on Nov 27 but encountered one or more errors.":              "failed",
		"Backup failed to run for profile: Documents":                               "failed",
		"Unable to connect to the server backup.lan. Please make sure":              "failed",
		"Authentication failed. Ensure email and password are correctly specified.": "failed",
		"Unable to backup C:\\x.doc. Error: access denied":                          "failed",
		"Syncrify server ran out of client licenses. Please purchase":               "failed",
		"Internal server error has occurred. Refer to the logs for details.":        "failed",
	}
	for line, want := range cases {
		if got := Run(context.Background(), "p", "/x", "manual", opts(line)).Status; got != want {
			t.Errorf("%q: got %s want %s", line, got, want)
		}
	}
}

func TestFailureWinsOverWarning(t *testing.T) {
	d := config.Default()
	r := Run(context.Background(), "p", "/x", "manual", Options{ClientPath: "/bin/sh",
		Args:      []string{"-c", "echo 'completed with one or more error.'; echo 'Unable to connect to the server'"},
		Timeout:   5 * time.Second,
		FailRegex: Compile(d.FailurePatterns), WarnRegex: Compile(d.WarnPatterns)})
	if r.Status != "failed" {
		t.Errorf("got %s", r.Status)
	}
}
