// Package profiles discovers and parses Syncrify *.syncrify profile files.
//
// The profile XML schema is not publicly specified beyond a few fields, so the
// parser is deliberately lenient: it flattens every leaf element into a
// name->value map and only interprets the fields we know (schedulerDays).
// Profiles are never modified - Syncrify warns against hand-editing them.
package profiles

import (
	"encoding/xml"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Profile is one discovered Syncrify backup profile.
type Profile struct {
	Name   string            `json:"name"`
	Path   string            `json:"path"`
	Days   []time.Weekday    `json:"-"` // from schedulerDays; empty = no schedule in the profile
	Fields map[string]string `json:"fields,omitempty"`
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

// Parse reads one profile file.
func Parse(path string) (Profile, error) {
	f, err := os.Open(path)
	if err != nil {
		return Profile{}, err
	}
	defer f.Close()
	p := Profile{
		Name:   strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)),
		Path:   path,
		Fields: map[string]string{},
	}
	dec := xml.NewDecoder(f)
	var stack []string
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
			stack = append(stack, t.Name.Local)
			text.Reset()
		case xml.CharData:
			text.Write(t)
		case xml.EndElement:
			if v := strings.TrimSpace(text.String()); v != "" {
				p.Fields[t.Name.Local] = v
			}
			text.Reset()
			stack = stack[:len(stack)-1]
		}
	}
	for k, v := range p.Fields {
		if strings.EqualFold(k, "schedulerDays") {
			if n, err := strconv.Atoi(v); err == nil {
				p.Days = DaysFromBitmap(n)
			}
		}
		if strings.EqualFold(k, "encryptionKey") || strings.Contains(strings.ToLower(k), "password") {
			p.Fields[k] = "***" // never surface secrets via API/UI
		}
	}
	return p, nil
}

// Discover scans dirs for *.syncrify files. Unreadable dirs are skipped.
// Duplicate names keep the first occurrence.
func Discover(dirs []string) []Profile {
	seen := map[string]bool{}
	var out []Profile
	for _, d := range dirs {
		files, _ := filepath.Glob(filepath.Join(d, "*.syncrify"))
		for _, f := range files {
			p, err := Parse(f)
			if err != nil && len(p.Fields) == 0 {
				continue
			}
			if seen[p.Name] {
				continue
			}
			seen[p.Name] = true
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
