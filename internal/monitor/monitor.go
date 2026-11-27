// Package monitor is the client's connection check: can this machine reach the
// Syncrify server, and does the endpoint really look like Syncrify? (It never
// administers the server.)
package monitor

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// Health is the result of one check.
type Health struct {
	URL        string    `json:"url"`
	Up         bool      `json:"up"`
	Identified bool      `json:"identified"`      // answered Syncrify's anonymous build-number operation
	Build      string    `json:"build,omitempty"` // as reported by the server, if parseable
	Status     int       `json:"status,omitempty"`
	LatencyMS  int64     `json:"latencyMs,omitempty"`
	Error      string    `json:"error,omitempty"`
	Checked    time.Time `json:"checked"`
}

var buildRe = regexp.MustCompile(`^[0-9A-Za-z][0-9A-Za-z._-]{0,31}$`)

// probeURL builds <base>/app?operation=gbn. Syncrify routes every request
// through its "app" controller; "gbn" (GetBuildNumber) is one of the operations
// that needs no login, so it is a safe, read-only way to identify the server.
func probeURL(base string) (string, error) {
	u, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/app"
	u.RawQuery = url.Values{"operation": {"gbn"}}.Encode()
	u.Fragment = ""
	return u.String(), nil
}

func get(ctx context.Context, u string) (*http.Response, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, nil, err
	}
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 512)) // never read an unbounded body from the network
	return resp, b, nil
}

// Check probes the server. TLS certificates are verified (default transport).
// Up means *something* answered with a status below 500; Identified means it
// also answered the Syncrify build-number operation with a short plain body.
func Check(ctx context.Context, base string) Health {
	h := Health{URL: base, Checked: time.Now()}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	start := time.Now()
	if pu, err := probeURL(base); err == nil {
		if resp, body, err := get(ctx, pu); err == nil {
			h.Status, h.LatencyMS = resp.StatusCode, time.Since(start).Milliseconds()
			h.Up = resp.StatusCode < 500
			if resp.StatusCode == http.StatusOK {
				if t := strings.TrimSpace(string(body)); buildRe.MatchString(t) {
					h.Identified, h.Build = true, t
				}
			}
			return h
		} else {
			h.Error = err.Error()
		}
	}
	// Fall back to the bare URL so a reverse proxy / unusual path still counts as "reachable".
	start = time.Now()
	resp, _, err := get(ctx, base)
	if err != nil {
		h.Error = err.Error()
		return h
	}
	h.Error = ""
	h.Status, h.LatencyMS, h.Up = resp.StatusCode, time.Since(start).Milliseconds(), resp.StatusCode < 500
	return h
}
