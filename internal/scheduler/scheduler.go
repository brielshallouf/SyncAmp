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
