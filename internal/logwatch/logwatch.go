// Package logwatch is an EXPERIMENTAL, opt-in source of backup outcomes: it
// reads the Syncrify client's own logs (HTML files under "<data folder>/logs",
// per the product's messages) so SyncAmp learns about backups it did not start,
// e.g. those run by the client's built-in scheduler.
//
// What is verified: the client's outcome wording (from its resource bundle) and
// that logs live in "<data folder>/logs" and are HTML. What is NOT verified:
// log file names and exact layout, so a log is attributed to a profile only if
// the profile's name appears in the file name, and anything ambiguous is ignored
// rather than guessed. Log folders may be user-writable, so they are treated as
// hostile input: regular files only (no symlinks), size-capped, redacted.
package logwatch

import (
	"crypto/sha256"
	"encoding/hex"
	"html"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/brielshallouf/syncamp/internal/runner"
)

const (
	maxFileBytes = 16 << 20 // ignore absurdly large files
	tailBytes    = 64 << 10 // only the end of a log matters
	maxLineLen   = 300
)

// Patterns classify log lines. Failure beats warning beats success when they
// appear on the same line; across lines the LAST marker in the file wins (a
// log accumulates runs; the newest outcome is at the end).
type Patterns struct{ Success, Warn, Fail []*regexp.Regexp }

// Observation is one backup outcome read from a log.
type Observation struct {
	Profile string
	Status  string // success | warning | failed
	End     time.Time
	Detail  string
	File    string
	// Key identifies this specific outcome by content (profile + the lines around the
	// outcome line), NOT by file time: touching or rotating a log must never make an
	// old success look new, which would hide an overdue backup.
	Key string
}

var tagRe = regexp.MustCompile(`<[^>]*>`)

// Scan inspects "<dir>/logs" for every profile dir and each extra logDir.
func Scan(profileDirs, logDirs, profiles []string, p Patterns) []Observation {
	var dirs []string
	for _, d := range profileDirs {
		dirs = append(dirs, filepath.Join(d, "logs"))
	}
	dirs = append(dirs, logDirs...)
	seen := map[string]bool{}
	var out []Observation
	for _, d := range dirs {
		entries, err := os.ReadDir(d)
		if err != nil {
			continue
		}
		for _, e := range entries {
			path := filepath.Join(d, e.Name())
			if seen[path] || !logExt(e.Name()) {
				continue
			}
			seen[path] = true
			prof := attribute(e.Name(), profiles)
			if prof == "" {
				continue
			}
			if o, ok := readOne(path, prof, p); ok {
				out = append(out, o)
			}
		}
	}
	return out
}

func logExt(n string) bool {
	switch strings.ToLower(filepath.Ext(n)) {
	case ".htm", ".html", ".log", ".txt":
		return true
	}
	return false
}

// attribute returns the profile whose name occurs in the file name (the longest
// match wins so "Docs Archive" beats "Docs"), or "" when none.
func attribute(file string, profiles []string) string {
	lf, best := strings.ToLower(file), ""
	for _, p := range profiles {
		if p != "" && strings.Contains(lf, strings.ToLower(p)) && len(p) > len(best) {
			best = p
		}
	}
	return best
}

func readOne(path, profile string, p Patterns) (Observation, bool) {
	st, err := os.Lstat(path)
	if err != nil || !st.Mode().IsRegular() || st.Size() > maxFileBytes || st.Size() == 0 {
		return Observation{}, false
	}
	f, err := os.Open(path)
	if err != nil {
		return Observation{}, false
	}
	defer f.Close()
	off := st.Size() - tailBytes
	if off < 0 {
		off = 0
	}
	buf := make([]byte, st.Size()-off)
	if _, err := f.ReadAt(buf, off); err != nil && err != io.EOF {
		return Observation{}, false
	}
	text := html.UnescapeString(tagRe.ReplaceAllString(strings.NewReplacer("<br>", "\n", "<br/>", "\n", "<BR>", "\n", "</p>", "\n", "</tr>", "\n").Replace(string(buf)), ""))
	var lines []string
	for _, l := range strings.Split(text, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			lines = append(lines, l)
		}
	}
	status, detail, at := "", "", -1
	for i, line := range lines {
		switch {
		case matchAny(p.Fail, line):
			status, detail, at = "failed", line, i
		case matchAny(p.Warn, line):
			status, detail, at = "warning", line, i
		case matchAny(p.Success, line):
			status, detail, at = "success", line, i
		}
	}
	if status == "" {
		return Observation{}, false
	}
	if len(detail) > maxLineLen {
		detail = detail[:maxLineLen] + "…"
	}
	ctx := strings.Join(lines[max(0, at-8):at], "\n")
	if len(ctx) > 400 {
		ctx = ctx[len(ctx)-400:]
	}
	sum := sha256.Sum256([]byte(profile + "\x00" + ctx + "\x00" + detail))
	return Observation{Profile: profile, Status: status, End: st.ModTime(), Detail: runner.Redact(detail), File: path,
		Key: hex.EncodeToString(sum[:8])}, true
}

func matchAny(res []*regexp.Regexp, s string) bool {
	for _, re := range res {
		if re.MatchString(s) {
			return true
		}
	}
	return false
}
