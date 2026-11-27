// Package monitor is the client's connection check: can this machine reach the
// Syncrify server's web endpoint? (It never administers the server.)
package monitor

import (
	"context"
	"net/http"
	"time"
)

// Health is the result of one check.
type Health struct {
	URL       string    `json:"url"`
	Up        bool      `json:"up"`
	Status    int       `json:"status,omitempty"`
	LatencyMS int64     `json:"latencyMs,omitempty"`
	Error     string    `json:"error,omitempty"`
	Checked   time.Time `json:"checked"`
}

// Check issues one GET. Any HTTP response below 500 counts as "up": Syncrify
// redirects to a login page, and we only care that the server answers.
func Check(ctx context.Context, url string) Health {
	h := Health{URL: url, Checked: time.Now()}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		h.Error = err.Error()
		return h
	}
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		h.Error = err.Error()
		return h
	}
	resp.Body.Close()
	h.Status, h.LatencyMS, h.Up = resp.StatusCode, time.Since(start).Milliseconds(), resp.StatusCode < 500
	return h
}
