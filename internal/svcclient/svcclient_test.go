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

// realService wraps h so it also answers the identity proof the way the real service does.
func realService(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/proof" {
			_, _ = w.Write([]byte(`{"proof":"` + Proof(tok, r.URL.Query().Get("nonce")) + `"}`))
			return
		}
		h(w, r)
	}
}

func setup(t *testing.T, h http.HandlerFunc, addrOverride string) *Client {
	srv := httptest.NewServer(realService(h))
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

// The point of the proof: a local user who grabbed the port first must never receive the key.
func TestKeyIsNeverSentToAnImpostor(t *testing.T) {
	var sawKey bool
	impostor := func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			sawKey = true
		}
		if r.URL.Path == "/api/proof" {
			_, _ = w.Write([]byte(`{"proof":"` + strings.Repeat("0", 64) + `"}`)) // cannot know the key
			return
		}
		w.WriteHeader(200)
	}
	srv := httptest.NewServer(http.HandlerFunc(impostor))
	defer srv.Close()
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, AddrFile), []byte(strings.TrimPrefix(srv.URL, "http://")), 0o600)
	_ = os.WriteFile(filepath.Join(dir, "api.token"), []byte(tok+"\n"), 0o600)
	r := New(dir).Do("GET", "/api/status", "")
	if sawKey || r.Status != 0 || !strings.Contains(r.Body, "did not prove") {
		t.Errorf("impostor got the key=%v, result=%+v", sawKey, r)
	}
	// An impostor that answers nothing sensible, or errors, is refused too.
	for _, h := range []http.HandlerFunc{
		func(w http.ResponseWriter, r *http.Request) { http.Error(w, "no", 500) },
		func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("not json")) },
	} {
		s2 := httptest.NewServer(h)
		_ = os.WriteFile(filepath.Join(dir, AddrFile), []byte(strings.TrimPrefix(s2.URL, "http://")), 0o600)
		if r := New(dir).Do("POST", "/api/run", ""); r.Status != 0 {
			t.Errorf("must refuse: %+v", r)
		}
		s2.Close()
	}
}

func TestProofIsKeyedAndNonceDependent(t *testing.T) {
	a, b := Proof(tok, "aaaaaaaaaaaaaaaa"), Proof(tok, "bbbbbbbbbbbbbbbb")
	if a == b || a == Proof("another key another key another key!!", "aaaaaaaaaaaaaaaa") || len(a) != 64 {
		t.Error("proof must depend on both the key and the nonce")
	}
	if !IsHex("deadBEEF09") || IsHex("") || IsHex("xyz") || IsHex("ab cd") {
		t.Error("IsHex misbehaves")
	}
}
