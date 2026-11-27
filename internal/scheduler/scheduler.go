// Package scheduler decides when profiles are due. It replaces the Windows
// Task Scheduler / cron entries the stock Syncrify client relies on, so the
// same schedule logic runs identically on every OS.
package scheduler

import (
	"fmt"
	"strings"
	"time"
)

// Spec is a calendar schedule (Days x Times) or a fixed Interval.
type Spec struct {
	Days     []time.Weekday
	Times    []string // "HH:MM" local
	Interval time.Duration
}

// ParseDays converts "mon","tue",... to weekdays.
func ParseDays(in []string) ([]time.Weekday, error) {
	m := map[string]time.Weekday{"sun": 0, "mon": 1, "tue": 2, "wed": 3, "thu": 4, "fri": 5, "sat": 6}
	var out []time.Weekday
	for _, s := range in {
		d, ok := m[strings.ToLower(strings.TrimSpace(s))[:min(3, len(strings.TrimSpace(s)))]]
		if !ok {
			return nil, fmt.Errorf("bad weekday %q", s)
		}
		out = append(out, d)
	}
	return out, nil
}

func parseHM(s string) (h, m int, err error) {
	_, err = fmt.Sscanf(s, "%d:%d", &h, &m)
	if err == nil && (h < 0 || h > 23 || m < 0 || m > 59) {
		err = fmt.Errorf("bad time %q", s)
	}
	return
}

// Due reports whether a scheduled instant falls in (prev, now]. Using a window
// rather than "is it exactly HH:MM" means a late tick, or a short sleep/resume,
// never silently skips a backup. lastRun is only used for Interval specs.
func (s Spec) Due(prev, now, lastRun time.Time) bool {
	if s.Interval > 0 {
		return now.Sub(lastRun) >= s.Interval
	}
	if len(s.Days) == 0 || len(s.Times) == 0 || !now.After(prev) {
		return false
	}
	// Check each calendar day touched by the window.
	for day := startOfDay(prev); !day.After(now); day = day.AddDate(0, 0, 1) {
		ok := false
		for _, d := range s.Days {
			if day.Weekday() == d {
				ok = true
			}
		}
		if !ok {
			continue
		}
		for _, t := range s.Times {
			h, m, err := parseHM(t)
			if err != nil {
				continue
			}
			at := time.Date(day.Year(), day.Month(), day.Day(), h, m, 0, 0, now.Location())
			if at.After(prev) && !at.After(now) {
				return true
			}
		}
	}
	return false
}

func startOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

// ShortDur renders 6h0m0s as "6h" and 90m as "90m".
func ShortDur(d time.Duration) string {
	if d%time.Hour == 0 {
		return fmt.Sprintf("%dh", int(d/time.Hour))
	}
	return fmt.Sprintf("%dm", int(d/time.Minute))
}

// Describe is a short human summary: "Every 6h", "Daily · 02:00", "Mon, Wed · 02:00".
func (s Spec) Describe() string {
	if s.Interval > 0 {
		return "Every " + ShortDur(s.Interval)
	}
	if len(s.Days) == 0 || len(s.Times) == 0 {
		return "Manual"
	}
	days := "Daily"
	if len(s.Days) < 7 {
		names := make([]string, len(s.Days))
		for i, d := range s.Days {
			names[i] = d.String()[:3]
		}
		days = strings.Join(names, ", ")
	}
	return days + " · " + strings.Join(s.Times, ", ")
}

// MaxGap is the longest stretch between two consecutive scheduled days
// (cyclic over the week), i.e. how long a healthy profile can legitimately go
// without a backup. 0 means "no schedule known". Daily => 24h, Mon+Thu => 4d.
func MaxGap(days []time.Weekday) time.Duration {
	var have [7]bool
	n := 0
	for _, d := range days {
		if d >= 0 && d < 7 && !have[d] {
			have[d] = true
			n++
		}
	}
	if n == 0 {
		return 0
	}
	if n == 7 {
		return 24 * time.Hour
	}
	maxGap, last, first := 0, -1, -1
	for d := 0; d < 7; d++ {
		if !have[d] {
			continue
		}
		if first < 0 {
			first = d
		}
		if last >= 0 && d-last > maxGap {
			maxGap = d - last
		}
		last = d
	}
	if wrap := first + 7 - last; wrap > maxGap { // Sat -> next Sun etc.
		maxGap = wrap
	}
	return time.Duration(maxGap) * 24 * time.Hour
}
