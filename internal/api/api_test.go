package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/brielshallouf/syncamp/internal/core"
)

const tok = "0123456789abcdef0123456789abcdef0123456789abcdef"

func newEngine(t *testing.T) *core.Engine {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config.json")
	_ = os.WriteFile(cfg, []byte(`{"clientPath":"/bin/echo","clientArgs":["{profile}"],"profileDirs":["`+dir+`"],"listenAddr":"127.0.0.1:0"}`), 0o600)
	_ = os.WriteFile(filepath.Join(dir, "Docs.syncrify"), []byte(`<p><schedulerDays>0</schedulerDays></p>`), 0o600)
	e, err := core.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
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
	for _, bad := range []string{"Docs", "clientPath", "/bin/echo", "profiles"} {
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
	_ = os.WriteFile(cfg, []byte(`{"clientPath":"/bin/echo","profileDirs":["`+dir+`/none"],"listenAddr":"127.0.0.1:0"}`), 0o600)
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
