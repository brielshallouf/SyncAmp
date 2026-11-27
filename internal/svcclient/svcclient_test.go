package svcclient

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const tok = "0123456789abcdef0123456789abcdef0123456789abcdef"

func setup(t *testing.T, h http.HandlerFunc, addrOverride string) *Client {
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	dir := t.TempDir()
	addr := strings.TrimPrefix(srv.URL, "http://")
	if addrOverride != "" {
		addr = addrOverride
	}
	_ = os.WriteFile(filepath.Join(dir, AddrFile), []byte(addr+"\n"), 0o600)
	_ = os.WriteFile(filepath.Join(dir, "api.token"), []byte(tok+"\n"), 0o600)
	return New(dir)
}

func TestAttachesTokenAndPassesBody(t *testing.T) {
	var gotAuth, gotBody, gotPath, gotMethod string
	c := setup(t, func(w http.ResponseWriter, r *http.Request) {
		gotAuth, gotPath, gotMethod = r.Header.Get("Authorization"), r.URL.Path, r.Method
		b := make([]byte, 100)
		n, _ := r.Body.Read(b)
		gotBody = string(b[:n])
		_, _ = w.Write([]byte(`{"ok":true}`))
	}, "")
	r := c.Do("POST", "/api/profiles/Docs/schedule", `{"preset":"hourly"}`)
	if r.Status != 200 || r.Body != `{"ok":true}` {
		t.Fatalf("%+v", r)
	}
	if gotAuth != "Bearer "+tok || gotMethod != "POST" || gotPath != "/api/profiles/Docs/schedule" || gotBody != `{"preset":"hourly"}` {
		t.Errorf("auth=%q method=%s path=%s body=%q", gotAuth, gotMethod, gotPath, gotBody)
	}
}

func TestSummaryIsSentWithoutToken(t *testing.T) {
	var gotAuth string
	c := setup(t, func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte("{}"))
	}, "")
	c.Do("GET", "/api/summary", "")
	if gotAuth != "" {
		t.Errorf("the public summary must not carry the key: %q", gotAuth)
	}
}

func TestOnlyAllowListedRequestsReachTheNetwork(t *testing.T) {
	hit := false
	c := setup(t, func(w http.ResponseWriter, r *http.Request) { hit = true }, "")
	bad := []struct{ m, p, b string }{
		{"DELETE", "/api/status", ""}, {"PUT", "/api/status", ""}, {"GET", "/", ""}, {"GET", "/app.js", ""},
		{"GET", "http://evil.example/api/x", ""}, {"GET", "/api/../etc/passwd", ""}, {"GET", "//evil/api/x", ""},
		{"GET", "/api/status?x=1", ""}, {"GET", "/api/a b", ""}, {"GET", "/api/\x00", ""}, {"GET", `/api\x`, ""},
		{"POST", "/api/run", strings.Repeat("a", 5000)}, {"GET", "/api/", ""},
	}
	for _, b := range bad {
		if r := c.Do(b.m, b.p, b.b); r.Status != 400 {
			t.Errorf("%s %q should be refused, got %d", b.m, b.p, r.Status)
		}
	}
	if hit {
		t.Error("a refused request reached the service")
	}
}

func TestNonLoopbackAddressIsIgnored(t *testing.T) {
	c := setup(t, func(w http.ResponseWriter, r *http.Request) {}, "203.0.113.9:5801")
	if got := c.Addr(); got != defaultAddr {
		t.Errorf("a tampered non-loopback address must fall back to the default, got %q", got)
	}
	// Without the file, the default applies; localhost and IPv6 loopback are accepted.
	for _, ok := range []string{"localhost:5801", "[::1]:5801", "127.0.0.1:9"} {
		_ = os.WriteFile(filepath.Join(c.Dir, AddrFile), []byte(ok), 0o600)
		if c.Addr() != ok {
			t.Errorf("%s should be accepted", ok)
		}
	}
}

func TestUnreachableServiceReportsStatusZero(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, AddrFile), []byte("127.0.0.1:1"), 0o600)
	if r := New(dir).Do("GET", "/api/summary", ""); r.Status != 0 || !strings.Contains(r.Body, "not running") {
		t.Errorf("%+v", r)
	}
}

func TestUnreadableTokenMeansNoAuthHeader(t *testing.T) {
	var gotAuth string
	c := setup(t, func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(401)
	}, "")
	_ = os.Remove(filepath.Join(c.Dir, "api.token"))
	if r := c.Do("GET", "/api/status", ""); r.Status != 401 || gotAuth != "" {
		t.Errorf("status=%d auth=%q", r.Status, gotAuth)
	}
}
