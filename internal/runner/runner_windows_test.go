//go:build windows

package runner

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

// Exercises the Windows-only paths for real: console hiding, output capture,
// credential redaction, and killing the whole process tree on timeout via taskkill.
func winRun(t *testing.T, timeout time.Duration, args ...string) (statusErr string, tail string, took time.Duration) {
	t.Helper()
	start := time.Now()
	r := Run(context.Background(), "p", "x", "manual", Options{
		ClientPath: os.Getenv("ComSpec"), Args: args, Timeout: timeout,
		FailRegex: Compile([]string{`(?i)unable to connect`}), WarnRegex: Compile([]string{`(?i)completed with one or more error`}),
	})
	return r.Status + "|" + r.Error, strings.Join(r.Tail, "\n"), time.Since(start)
}

func TestWindowsCapturesOutputAndClassifies(t *testing.T) {
	if got, _, _ := winRun(t, 30*time.Second, "/c", "echo", "Backup", "completed", "in", "1", "minute"); got != "success|" {
		t.Errorf("success: %s", got)
	}
	if got, _, _ := winRun(t, 30*time.Second, "/c", "echo", "Unable", "to", "connect", "to", "the", "server"); !strings.HasPrefix(got, "failed|") {
		t.Errorf("failure pattern: %s", got)
	}
	if got, _, _ := winRun(t, 30*time.Second, "/c", "echo", "completed", "with", "one", "or", "more", "error."); !strings.HasPrefix(got, "warning|") {
		t.Errorf("warning pattern: %s", got)
	}
	if got, _, _ := winRun(t, 30*time.Second, "/c", "exit", "3"); got != "failed|client exited with code 3" {
		t.Errorf("exit code: %s", got)
	}
}

func TestWindowsRedactsSecrets(t *testing.T) {
	_, tail, _ := winRun(t, 30*time.Second, "/c", "echo", "connecting", "password=hunter2", "ok")
	if strings.Contains(tail, "hunter2") || !strings.Contains(tail, "connecting") {
		t.Errorf("tail: %q", tail)
	}
}

// If the process TREE is not killed, the pipe stays open and Run only returns after
// WaitDelay (5s). Returning in well under that proves taskkill /T worked.
func TestWindowsTimeoutKillsProcessTree(t *testing.T) {
	got, _, took := winRun(t, 1*time.Second, "/c", "ping", "-n", "40", "127.0.0.1")
	if !strings.HasPrefix(got, "timeout|") {
		t.Fatalf("want timeout, got %s", got)
	}
	if took > 4500*time.Millisecond {
		t.Errorf("took %v: the child process tree was not killed promptly", took)
	}
}
