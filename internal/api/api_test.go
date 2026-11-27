package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/brielshallouf/syncamp/internal/core"
)

func newEngine(t *testing.T) *core.Engine {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config.json")
	_ = os.WriteFile(cfg, []byte(`{"clientPath":"/bin/echo","clientArgs":["{profile}"],"profileDirs":["`+dir+`"],"listenAddr":"127.0.0.1:0"}`), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "Docs.syncrify"), []byte(`<p><schedulerDays>0</schedulerDays></p>`), 0o644)
	e, err := core.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func do(h http.Handler, method, path, host string, hdr map[string]string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, nil)
	r.Host = host
	for k, v := range hdr {
		r.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestGuard(t *testing.T) {
	h := Handler(newEngine(t))
	if c := do(h, "GET", "/api/status", "127.0.0.1:5801", nil).Code; c != 200 {
		t.Errorf("status: %d", c)
	}
	if c := do(h, "GET", "/api/status", "evil.example.com", nil).Code; c != 403 {
		t.Errorf("DNS-rebinding host must be rejected, got %d", c)
	}
	if c := do(h, "POST", "/api/run/Docs", "localhost:5801", nil).Code; c != 403 {
		t.Errorf("POST without header must be rejected, got %d", c)
	}
	if c := do(h, "POST", "/api/run/Docs", "localhost:5801", map[string]string{"X-SyncAmp": "1"}).Code; c != 202 {
		t.Errorf("valid run: %d", c)
	}
	if c := do(h, "POST", "/api/run/Nope", "localhost:5801", map[string]string{"X-SyncAmp": "1"}).Code; c != 409 {
		t.Errorf("unknown profile: %d", c)
	}
	if c := do(h, "GET", "/", "localhost:5801", nil).Code; c != 200 {
		t.Errorf("dashboard: %d", c)
	}
}
