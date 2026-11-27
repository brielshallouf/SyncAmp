package monitor

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestProbeURL(t *testing.T) {
	for in, want := range map[string]string{
		"http://backup.lan:5800":        "http://backup.lan:5800/app?operation=gbn",
		"https://backup.lan:5800/":      "https://backup.lan:5800/app?operation=gbn",
		"https://x.example/syncrify":    "https://x.example/syncrify/app?operation=gbn",
		"http://h:1/app?operation=junk": "http://h:1/app/app?operation=gbn", // user query is replaced, never forwarded
	} {
		got, err := probeURL(in)
		if err != nil || got != want {
			t.Errorf("%s: got %s (%v) want %s", in, got, err, want)
		}
	}
}

func TestIdentifiesSyncrifyAndBuild(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/app" && r.URL.Query().Get("operation") == "gbn" {
			_, _ = w.Write([]byte("5871\n"))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()
	h := Check(context.Background(), srv.URL)
	if !h.Up || !h.Identified || h.Build != "5871" {
		t.Errorf("%+v", h)
	}
}

func TestReachableButNotSyncrify(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("<html>welcome to some other web server, this body is not a build number</html>"))
	}))
	defer srv.Close()
	h := Check(context.Background(), srv.URL)
	if !h.Up || h.Identified || h.Build != "" {
		t.Errorf("a random web server must be Up but not Identified: %+v", h)
	}
}

func TestDownAndServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "boom", 503) }))
	h := Check(context.Background(), srv.URL)
	srv.Close()
	if h.Up || h.Identified {
		t.Errorf("5xx must not be up: %+v", h)
	}
	h = Check(context.Background(), srv.URL) // now closed: connection refused
	if h.Up || h.Error == "" {
		t.Errorf("closed server must be down with an error: %+v", h)
	}
}

func TestHugeBodyIsBounded(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("9", 5<<20)))
	}))
	defer srv.Close()
	if h := Check(context.Background(), srv.URL); h.Identified {
		t.Errorf("an enormous body is not a build number: %+v", h)
	}
}
