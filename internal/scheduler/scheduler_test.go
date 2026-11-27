package scheduler

import (
	"testing"
	"time"
)

func at(s string) time.Time {
	t, err := time.ParseInLocation("2006-01-02 15:04", s, time.Local)
	if err != nil {
		panic(err)
	}
	return t
}

func TestDue(t *testing.T) {
	// 2026-11-29 is a Sunday.
	s := Spec{Days: []time.Weekday{time.Sunday, time.Tuesday}, Times: []string{"02:00"}}
	cases := []struct {
		name      string
		prev, now string
		want      bool
	}{
		{"tick spans the time", "2026-11-29 01:59", "2026-11-29 02:00", true},
		{"before time", "2026-11-29 01:00", "2026-11-29 01:30", false},
		{"already past (prev==at is exclusive)", "2026-11-29 02:00", "2026-11-29 02:05", false},
		{"wrong day (Monday)", "2026-11-30 01:59", "2026-11-30 02:01", false},
		{"resume from sleep across slot", "2026-11-29 01:00", "2026-11-29 09:00", true},
		{"multi-day gap hits Tuesday", "2026-11-30 03:00", "2026-12-02 03:00", true},
	}
	for _, c := range cases {
		if got := s.Due(at(c.prev), at(c.now), time.Time{}); got != c.want {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}

func TestInterval(t *testing.T) {
	s := Spec{Interval: time.Hour}
	if s.Due(time.Time{}, at("2026-11-29 12:00"), at("2026-11-29 11:30")) {
		t.Error("30m since last run should not be due")
	}
	if !s.Due(time.Time{}, at("2026-11-29 12:00"), at("2026-11-29 10:30")) {
		t.Error("90m since last run should be due")
	}
}

func TestParseDays(t *testing.T) {
	d, err := ParseDays([]string{"Mon", "friday"})
	if err != nil || len(d) != 2 || d[0] != time.Monday || d[1] != time.Friday {
		t.Fatalf("got %v %v", d, err)
	}
	if _, err := ParseDays([]string{"xyz"}); err == nil {
		t.Error("expected error")
	}
}

func TestDescribe(t *testing.T) {
	all := []time.Weekday{0, 1, 2, 3, 4, 5, 6}
	cases := map[string]Spec{
		"Every 6h":                {Interval: 6 * time.Hour},
		"Every 90m":               {Interval: 90 * time.Minute},
		"Daily · 02:00":           {Days: all, Times: []string{"02:00"}},
		"Mon, Wed · 02:00, 14:30": {Days: []time.Weekday{1, 3}, Times: []string{"02:00", "14:30"}},
		"Manual":                  {},
	}
	for want, s := range cases {
		if got := s.Describe(); got != want {
			t.Errorf("got %q want %q", got, want)
		}
	}
}
