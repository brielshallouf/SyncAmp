// Package api serves SyncAmp's local control API and web dashboard.
//
// Security model: the listener is loopback-only, every request's Host header
// must be a loopback name (defeats DNS rebinding), and state-changing requests
// must be POST with an X-SyncAmp header (a custom header forces a CORS
// preflight, which we never grant, so web pages in the user's browser cannot
// trigger backups).
package api

import (
	"embed"
	"encoding/json"
	"net"
	"net/http"
	"strings"

	"github.com/brielshallouf/syncamp/internal/core"
)

//go:embed web/index.html
var web embed.FS

// Handler builds the HTTP handler for engine e.
func Handler(e *core.Engine) http.Handler {
	mux := http.NewServeMux()
	writeJSON := func(w http.ResponseWriter, code int, v any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		_ = json.NewEncoder(w).Encode(v)
	}
	mux.HandleFunc("GET /api/status", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, e.Status()) })
	mux.HandleFunc("GET /api/profiles", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, e.Profiles()) })
	mux.HandleFunc("GET /api/history", func(w http.ResponseWriter, r *http.Request) {
		h := e.History()
		for i, j := 0, len(h)-1; i < j; i, j = i+1, j-1 { // newest first
			h[i], h[j] = h[j], h[i]
		}
		if len(h) > 100 {
			h = h[:100]
		}
		writeJSON(w, 200, h)
	})
	mux.HandleFunc("POST /api/run/{name}", func(w http.ResponseWriter, r *http.Request) {
		if err := e.RunProfile(r.PathValue("name"), "manual"); err != nil {
			writeJSON(w, 409, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, 202, map[string]string{"status": "started"})
	})
	mux.HandleFunc("POST /api/run", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 202, map[string]int{"started": e.RunAll("manual")})
	})
	mux.HandleFunc("POST /api/reload", func(w http.ResponseWriter, r *http.Request) {
		e.Reload()
		writeJSON(w, 200, map[string]string{"status": "reloaded"})
	})
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		b, _ := web.ReadFile("web/index.html")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(b)
	})
	return guard(mux)
}

func guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.Host)
		if err != nil {
			host = r.Host
		}
		if host != "localhost" && host != "127.0.0.1" && host != "[::1]" && host != "::1" {
			http.Error(w, "forbidden host", http.StatusForbidden)
			return
		}
		if r.Method == http.MethodPost && r.Header.Get("X-SyncAmp") == "" {
			http.Error(w, "missing X-SyncAmp header", http.StatusForbidden)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Cache-Control", "no-store")
		}
		next.ServeHTTP(w, r)
	})
}
