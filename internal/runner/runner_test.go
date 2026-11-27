//go:build !windows

package runner

import (
	"context"
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
