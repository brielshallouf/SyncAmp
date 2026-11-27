package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/brielshallouf/syncamp/internal/core"
)

// clientPath is an absolute path to *some* real executable on this OS (Config.Validate
// requires an absolute clientPath, and "/bin/echo" is not absolute on Windows).
func clientPath() string {
	if runtime.GOOS == "windows" {
		return os.Getenv("ComSpec")
	}
	return "/bin/echo"
}

// writeCfg writes config JSON through json.Marshal: pasting a Windows path into a
// string literal produces invalid escapes like \U.
func writeCfg(t *testing.T, path string, m map[string]any) {
	t.Helper()
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
}

const tok = "0123456789abcdef0123456789abcdef0123456789abcdef"

func newEngine(t *testing.T) *core.Engine {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config.json")
	writeCfg(t, cfg, map[string]any{"clientPath": clientPath(), "clientArgs": []string{"{profile}"}, "profileDirs": []string{dir}, "listenAddr": "127.0.0.1:0"})
	_ = os.WriteFile(filepath.Join(dir, "Docs.syncrify"), []byte(`<p><schedulerDays>0</schedulerDays></p>`), 0o600)
	e, err := core.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(e.Wait) // let any backup started by a test finish before the temp dir is removed (Windows cannot delete open files)
	return e
}

func do(h http.Handler, method, path, host, bearer string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, nil)
	r.Host = host
	if bearer != "" {
		r.Header.Set("Authorization", "Bearer "+bearer)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestAuthAndHostGuard(t *testing.T) {
	h := Handler(newEngine(t), tok)
	const lo = "127.0.0.1:5801"
	cases := []struct {
		name, method, path, host, bearer string
		want                             int
	}{
		{"summary is public", "GET", "/api/summary", lo, "", 200},
		{"status needs token", "GET", "/api/status", lo, "", 401},
		{"status wrong token", "GET", "/api/status", lo, "nope", 401},
		{"status ok", "GET", "/api/status", lo, tok, 200},
		{"history needs token", "GET", "/api/history", lo, "", 401},
		{"profiles needs token", "GET", "/api/profiles", lo, "", 401},
		{"run needs token", "POST", "/api/run/Docs", lo, "", 401},
		{"run-all needs token", "POST", "/api/run", lo, "", 401},
		{"reload needs token", "POST", "/api/reload", lo, "", 401},
		{"run ok", "POST", "/api/run/Docs", "localhost:5801", tok, 202},
		{"run unknown profile", "POST", "/api/run/Nope", lo, tok, 409},
		{"DNS rebinding blocked", "GET", "/api/summary", "evil.example.com", "", 403},
		{"DNS rebinding blocked even with token", "GET", "/api/status", "evil.example.com:5801", tok, 403},
		{"ipv6 loopback host ok", "GET", "/api/summary", "[::1]:5801", "", 200},
		{"dashboard", "GET", "/", lo, "", 200},
		{"js", "GET", "/app.js", lo, "", 200},
	}
	for _, c := range cases {
		if got := do(h, c.method, c.path, c.host, c.bearer).Code; got != c.want {
			t.Errorf("%s: got %d want %d", c.name, got, c.want)
		}
	}
}

func TestSummaryLeaksNothingSensitive(t *testing.T) {
	h := Handler(newEngine(t), tok)
	body := do(h, "GET", "/api/summary", "127.0.0.1:1", "").Body.String()
	for _, bad := range []string{"Docs", "clientPath", clientPath(), "profiles"} {
		if strings.Contains(body, bad) {
			t.Errorf("public summary leaked %q: %s", bad, body)
		}
	}
}

func TestSecurityHeadersAndNoInlineScript(t *testing.T) {
	h := Handler(newEngine(t), tok)
	w := do(h, "GET", "/", "127.0.0.1:1", "")
	csp := w.Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "script-src 'self'") || strings.Contains(csp, "unsafe-inline") || !strings.Contains(csp, "frame-ancestors 'none'") {
		t.Errorf("weak CSP: %q", csp)
	}
	if w.Header().Get("X-Content-Type-Options") != "nosniff" || w.Header().Get("Cache-Control") != "no-store" {
		t.Error("missing hardening headers")
	}
	html := w.Body.String()
	if strings.Contains(html, "onclick") || strings.Contains(strings.ToLower(html), "<script>") {
		t.Error("index.html must not contain inline script/handlers")
	}
	js := do(h, "GET", "/app.js", "127.0.0.1:1", "").Body.String()
	if regexp.MustCompile(`\.innerHTML|insertAdjacentHTML|document\.write|eval\(`).MatchString(js) {
		t.Error("app.js must not use innerHTML (profile names are untrusted)")
	}
}

func TestProfilesEndpointHidesFileContents(t *testing.T) {
	h := Handler(newEngine(t), tok)
	body := do(h, "GET", "/api/profiles", "127.0.0.1:1", tok).Body.String()
	if strings.Contains(body, "schedulerDays") || strings.Contains(body, "fields") {
		t.Errorf("profile file contents exposed: %s", body)
	}
}

// Clients index straight into these arrays; JSON null would crash them.
func TestEmptyCollectionsAreArraysNotNull(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config.json")
	writeCfg(t, cfg, map[string]any{"clientPath": clientPath(), "profileDirs": []string{filepath.Join(dir, "none")}, "listenAddr": "127.0.0.1:0"})
	e, err := core.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	h := Handler(e, tok)
	for path, want := range map[string]string{"/api/history": "[]", "/api/profiles": "[]"} {
		if got := strings.TrimSpace(do(h, "GET", path, "127.0.0.1:1", tok).Body.String()); got != want {
			t.Errorf("%s = %s, want %s", path, got, want)
		}
	}
	if b := do(h, "GET", "/api/status", "127.0.0.1:1", tok).Body.String(); !strings.Contains(b, `"profiles":[]`) {
		t.Errorf("status.profiles must be []: %s", b)
	}
}

func post(h http.Handler, path, body, bearer string) *httptest.ResponseRecorder {
	r := httptest.NewRequest("POST", path, strings.NewReader(body))
	r.Host = "127.0.0.1:5801"
	if bearer != "" {
		r.Header.Set("Authorization", "Bearer "+bearer)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func presetOf(t *testing.T, h http.Handler, name string) (preset string, paused bool) {
	var st struct {
		Profiles []struct {
			Name   string `json:"name"`
			Preset string `json:"preset"`
			Paused bool   `json:"paused"`
		}
	}
	body := do(h, "GET", "/api/status", "127.0.0.1:5801", tok).Body.Bytes()
	if err := json.Unmarshal(body, &st); err != nil {
		t.Fatal(err)
	}
	for _, p := range st.Profiles {
		if p.Name == name {
			return p.Preset, p.Paused
		}
	}
	t.Fatalf("profile %s missing", name)
	return
}

func TestScheduleEndpoint(t *testing.T) {
	e := newEngine(t)
	h := Handler(e, tok)
	const path = "/api/profiles/Docs/schedule"
	if c := post(h, path, `{"preset":"hourly"}`, "").Code; c != 401 {
		t.Errorf("needs token: %d", c)
	}
	if c := post(h, path, `{"preset":"hourly"}`, tok).Code; c != 200 {
		t.Fatalf("hourly: %d", c)
	}
	if p, _ := presetOf(t, h, "Docs"); p != "hourly" {
		t.Errorf("preset after hourly = %q", p)
	}
	post(h, path, `{"preset":"every6h","enabled":false}`, tok)
	if p, paused := presetOf(t, h, "Docs"); p != "every6h" || !paused {
		t.Errorf("want every6h+paused, got %q paused=%v", p, paused)
	}
	post(h, path, `{"enabled":true,"preset":"daily"}`, tok)
	if p, paused := presetOf(t, h, "Docs"); p != "daily" || paused {
		t.Errorf("want daily+active, got %q paused=%v", p, paused)
	}
	post(h, path, `{"preset":"profile"}`, tok)
	if p, _ := presetOf(t, h, "Docs"); p != "profile" {
		t.Errorf("revert: %q", p)
	}
	for body, want := range map[string]int{
		`{"preset":"every-second"}`: 400, `{"preset":1}`: 400, `{"bogus":true}`: 400, `not json`: 400,
	} {
		if c := post(h, path, body, tok).Code; c != want {
			t.Errorf("%s: got %d want %d", body, c, want)
		}
	}
	if c := post(h, "/api/profiles/Nope/schedule", `{"preset":"hourly"}`, tok).Code; c != 404 {
		t.Errorf("unknown profile: %d", c)
	}
	if st, err := os.Stat(e.CfgPath()); err != nil || st.Mode().Perm()&0o077 != 0 {
		t.Errorf("config must stay owner-only after API edits: %v %v", st, err)
	}
	raw, _ := os.ReadFile(e.CfgPath())
	if strings.Contains(string(raw), "Nope") {
		t.Error("unknown profile leaked into config")
	}
}

func TestStaleAfterEndpoint(t *testing.T) {
	e := newEngine(t)
	h := Handler(e, tok)
	const path = "/api/profiles/Docs/schedule"
	setting := func() string {
		var st struct {
			Profiles []struct{ Name, StaleSetting string }
		}
		_ = json.Unmarshal(do(h, "GET", "/api/status", "127.0.0.1:5801", tok).Body.Bytes(), &st)
		for _, p := range st.Profiles {
			if p.Name == "Docs" {
				return p.StaleSetting
			}
		}
		return "?"
	}
	if c := post(h, path, `{"staleAfter":"24h"}`, tok).Code; c != 200 || setting() != "24h" {
		t.Errorf("set 24h: %d %q", c, setting())
	}
	for _, bad := range []string{`{"staleAfter":"5m"}`, `{"staleAfter":"soon"}`, `{"staleAfter":"-3h"}`} {
		if c := post(h, path, bad, tok).Code; c != 400 {
			t.Errorf("%s: got %d want 400", bad, c)
		}
	}
	if setting() != "24h" {
		t.Error("a rejected update must not change the saved value")
	}
	if c := post(h, path, `{"staleAfter":"auto"}`, tok).Code; c != 200 || setting() != "auto" {
		t.Errorf("reset: %d %q", c, setting())
	}
	if c := post(h, path, `{"staleAfter":"24h"}`, "").Code; c != 401 {
		t.Errorf("needs token: %d", c)
	}
}
