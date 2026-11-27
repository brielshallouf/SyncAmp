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

const tailLines = 100

// Options controls one execution.
type Options struct {
	ClientPath string
	Args       []string // "{profile}" is substituted
	Timeout    time.Duration
	FailRegex  []*regexp.Regexp
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
	prepare(cmd)
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
		tail = append(tail, line)
		if len(tail) > tailLines {
			tail = tail[1:]
		}
		if matched == "" {
			for _, re := range o.FailRegex {
				if re.MatchString(line) {
					matched = line
					break
				}
			}
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
