// Package runner executes the official Syncrify client for one profile.
package runner

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/brielshallouf/syncamp/internal/store"
)

const (
	tailLines   = 50  // lines of client output kept in history
	maxLineLen  = 300 // longer lines are truncated (bounds history size)
	redactedStr = "[REDACTED]"
)

// Cred identifies the unix account a backup should run as.
type Cred struct{ UID, GID uint32 }

// Options controls one execution.
type Options struct {
	ClientPath string
	Args       []string // "{profile}" is substituted
	Timeout    time.Duration
	FailRegex  []*regexp.Regexp
	// RunAs, when set (unix, service is root), drops the child to this account
	// so a backup can only read what the profile's owner could read.
	RunAs *Cred
}

// Compile turns pattern strings into regexps, skipping invalid ones.
func Compile(patterns []string) []*regexp.Regexp {
	var out []*regexp.Regexp
	for _, p := range patterns {
		if re, err := regexp.Compile(p); err == nil {
			out = append(out, re)
		}
	}
	return out
}

// secretRe masks values that look like credentials before output is stored or
// sent to a webhook: client logs sometimes echo connection parameters.
var secretRe = regexp.MustCompile(`(?i)\b(pass(?:word|wd)?|secret|token|api[-_ ]?key|encryption[-_ ]?key|authorization)\b(\s*[=:]\s*|\s+)("[^"]*"|'[^']*'|\S+)`)

// schemeRe masks the credential after an auth scheme ("Bearer xyz"), which the
// key=value rule above would otherwise treat as "Bearer" being the value.
var schemeRe = regexp.MustCompile(`(?i)\b(bearer|basic)\s+\S+`)

// Redact masks credential-looking values in a line of output.
func Redact(s string) string {
	s = schemeRe.ReplaceAllString(s, "${1} "+redactedStr)
	return secretRe.ReplaceAllString(s, "${1}${2}"+redactedStr)
}

func clip(s string) string {
	if len(s) > maxLineLen {
		return s[:maxLineLen] + "…"
	}
	return s
}

// Run executes the client for profilePath and returns a finished record.
// Exit code 0 is necessary but not sufficient: the Syncrify console client
// can print a failure and still exit 0, so output is also checked.
func Run(ctx context.Context, name, profilePath, trigger string, o Options) store.Run {
	r := store.Run{Profile: name, Start: time.Now(), Trigger: trigger}
	finish := func(status, errMsg string) store.Run {
		r.End, r.Status, r.Error = time.Now(), status, errMsg
		return r
	}
	if o.ClientPath == "" {
		return finish("failed", "Syncrify client not found; set clientPath in config")
	}
	args := make([]string, len(o.Args))
	for i, a := range o.Args {
		args[i] = strings.ReplaceAll(a, "{profile}", profilePath)
	}
	if o.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, o.Timeout)
		defer cancel()
	}
	cmd := exec.CommandContext(ctx, o.ClientPath, args...)
	if err := prepare(cmd, o); err != nil {
		return finish("failed", err.Error())
	}
	pr, err := cmd.StdoutPipe()
	if err != nil {
		return finish("failed", err.Error())
	}
	cmd.Stderr = cmd.Stdout
	if err := cmd.Start(); err != nil {
		return finish("failed", err.Error())
	}
	var tail []string
	matched := ""
	sc := bufio.NewScanner(pr)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if matched == "" {
			for _, re := range o.FailRegex {
				if re.MatchString(line) {
					matched = clip(Redact(line))
					break
				}
			}
		}
		tail = append(tail, clip(Redact(line)))
		if len(tail) > tailLines {
			tail = tail[1:]
		}
	}
	werr := cmd.Wait()
	r.Tail = tail
	var ee *exec.ExitError
	switch {
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		return finish("timeout", fmt.Sprintf("exceeded %s", o.Timeout))
	case errors.As(werr, &ee):
		r.ExitCode = ee.ExitCode()
		return finish("failed", fmt.Sprintf("client exited with code %d", r.ExitCode))
	case werr != nil:
		return finish("failed", werr.Error())
	case matched != "":
		return finish("failed", "output indicates failure: "+matched)
	}
	return finish("success", "")
}
