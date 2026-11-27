// Package profiles discovers and parses Syncrify *.syncrify profile files.
//
// The profile XML schema is not publicly specified beyond a few fields, so the
// parser is lenient: it flattens leaf elements into a name->value map and only
// interprets schedulerDays. Profiles are never modified (Syncrify warns
// against hand-editing them) and their contents are never exposed over the
// API: profile directories may be user-writable, so everything in them is
// untrusted input.
package profiles

import (
	"encoding/xml"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// maxProfileBytes bounds what we will read from a (possibly hostile) file.
const maxProfileBytes = 1 << 20

// Profile is one discovered Syncrify backup profile.
type Profile struct {
	Name   string            `json:"name"`
	Path   string            `json:"path"`
	Days   []time.Weekday    `json:"-"` // from schedulerDays; empty = no schedule in the profile
	Fields map[string]string `json:"-"` // internal only; never serialised
}

// DaysFromBitmap decodes Syncrify's schedulerDays. Per Synametrics' docs a
// Sun+Tue+Thu schedule is binary 0010101 = 21, i.e. bit n set => weekday n
// (Sunday=0), which matches time.Weekday.
func DaysFromBitmap(v int) []time.Weekday {
	var out []time.Weekday
	for d := 0; d < 7; d++ {
		if v&(1<<d) != 0 {
			out = append(out, time.Weekday(d))
		}
	}
	return out
}

// Parse reads one profile file. Symlinks and non-regular files are refused:
// otherwise a link named x.syncrify could point the privileged service at an
// arbitrary file.
func Parse(path string) (Profile, error) {
	p := Profile{
		Name:   strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)),
		Path:   path,
		Fields: map[string]string{},
	}
	st, err := os.Lstat(path)
	if err != nil {
		return p, err
	}
	if !st.Mode().IsRegular() {
		return p, errors.New("not a regular file (symlinks are refused)")
	}
	f, err := os.Open(path)
	if err != nil {
		return p, err
	}
	defer f.Close()
	dec := xml.NewDecoder(io.LimitReader(f, maxProfileBytes))
	var text strings.Builder
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return p, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			text.Reset()
		case xml.CharData:
			if text.Len() < 4096 {
				text.Write(t)
			}
		case xml.EndElement:
			if v := strings.TrimSpace(text.String()); v != "" {
				p.Fields[t.Name.Local] = v
			}
			text.Reset()
		}
	}
	for k, v := range p.Fields {
		lk := strings.ToLower(k)
		if lk == "schedulerdays" {
			if n, err := strconv.Atoi(v); err == nil && n >= 0 && n < 128 {
				p.Days = DaysFromBitmap(n)
			}
		}
		if lk == "encryptionkey" || strings.Contains(lk, "password") || strings.Contains(lk, "secret") {
			p.Fields[k] = "***"
		}
	}
	return p, nil
}

// validName limits profile names to characters that are safe in URLs, logs and
// the dashboard, whatever the file is called.
func validName(n string) bool {
	if n == "" || len(n) > 100 {
		return false
	}
	for _, r := range n {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.', r == ' ':
		default:
			return false
		}
	}
	return !strings.HasPrefix(n, ".")
}

// Skipped describes a *.syncrify file that was found but not used.
type Skipped struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

// Discover scans dirs for *.syncrify files; see DiscoverReport.
func Discover(dirs []string) []Profile {
	p, _ := DiscoverReport(dirs)
	return p
}

// DiscoverReport scans dirs for *.syncrify files. Unreadable dirs are
// ignored; unusable files (bad name, symlink, malformed, duplicate) are
// reported in the second result so admins can see *why* a profile is absent.
func DiscoverReport(dirs []string) ([]Profile, []Skipped) {
	seen := map[string]bool{}
	var out []Profile
	var skipped []Skipped
	for _, d := range dirs {
		files, _ := filepath.Glob(filepath.Join(d, "*.syncrify"))
		for _, f := range files {
			p, err := Parse(f)
			switch {
			case err != nil:
				skipped = append(skipped, Skipped{f, err.Error()})
			case !validName(p.Name):
				skipped = append(skipped, Skipped{f, "profile name has unsupported characters (allowed: letters, digits, space . _ -)"})
			case seen[p.Name]:
				skipped = append(skipped, Skipped{f, "duplicate profile name; first one wins"})
			default:
				seen[p.Name] = true
				out = append(out, p)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, skipped
}
