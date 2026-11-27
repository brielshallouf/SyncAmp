// Package api serves SyncAmp's local control API and web dashboard.
//
// Security model (the API can start backups and reveals backup state):
//   - listener is loopback-only (enforced in config validation);
//   - Host header must be a loopback name (defeats DNS rebinding);
//   - every /api/* route except /api/summary needs "Authorization: Bearer
//     <token>", where the token file is readable only by root/Administrators.
//     The custom header also forces a CORS preflight that is never granted,
//     so web pages cannot drive the API (no CSRF);
//   - /api/summary exposes only {overall, summary} so the per-user tray can
//     show colour without holding the admin token;
//   - strict CSP, no inline script, framing denied, no caching of API data.
package api

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strings"

	"github.com/brielshallouf/syncamp/internal/auth"
	"github.com/brielshallouf/syncamp/internal/core"
	"github.com/brielshallouf/syncamp/internal/profiles"
	"github.com/brielshallouf/syncamp/internal/store"
	"github.com/brielshallouf/syncamp/internal/svcclient"
	"github.com/brielshallouf/syncamp/internal/webui"
)

var static = map[string]string{
	"/":          "index.html",
	"/app.js":    "app.js",
	"/style.css": "style.css",
}

var ctype = map[string]string{
	"/": "text/html; charset=utf-8", "/app.js": "text/javascript; charset=utf-8", "/style.css": "text/css; charset=utf-8",
}

// Handler builds the HTTP handler for engine e protected by token.
func Handler(e *core.Engine, token string) http.Handler {
	mux := http.NewServeMux()
	writeJSON := func(w http.ResponseWriter, code int, v any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		_ = json.NewEncoder(w).Encode(v)
	}
	protected := func(h http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if !auth.Equal(auth.FromHeader(r.Header.Get("Authorization")), token) {
				w.Header().Set("WWW-Authenticate", `Bearer realm="syncamp"`)
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing or invalid token"})
				return
			}
			h(w, r)
		}
	}

	// Proof of identity. A client that holds the access key sends a random nonce and checks the
	// HMAC that comes back BEFORE it sends the key itself, so a local user who grabbed the port
	// first (while the service was stopped) learns nothing. The key never crosses the wire here.
	mux.HandleFunc("GET /api/proof", func(w http.ResponseWriter, r *http.Request) {
		n := r.URL.Query().Get("nonce")
		if len(n) < 16 || len(n) > 128 || !svcclient.IsHex(n) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "nonce must be 16-128 hex characters"})
			return
		}
		writeJSON(w, 200, map[string]string{"proof": svcclient.Proof(token, n)})
	})
	mux.HandleFunc("GET /api/summary", func(w http.ResponseWriter, r *http.Request) {
		st := e.Status()
		writeJSON(w, 200, map[string]string{"overall": st.Overall, "summary": st.Summary})
	})
	mux.HandleFunc("GET /api/status", protected(func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, e.Status()) }))
	mux.HandleFunc("GET /api/profiles", protected(func(w http.ResponseWriter, r *http.Request) {
		ps := e.Profiles()
		if ps == nil {
			ps = []profiles.Profile{}
		}
		writeJSON(w, 200, ps)
	}))
	mux.HandleFunc("GET /api/history", protected(func(w http.ResponseWriter, r *http.Request) {
		h := e.History()
		if h == nil {
			h = []store.Run{}
		}
		for i, j := 0, len(h)-1; i < j; i, j = i+1, j-1 { // newest first
			h[i], h[j] = h[j], h[i]
		}
		if len(h) > 100 {
			h = h[:100]
		}
		writeJSON(w, 200, h)
	}))
	mux.HandleFunc("POST /api/run/{name}", protected(func(w http.ResponseWriter, r *http.Request) {
		if err := e.RunProfile(r.PathValue("name"), "manual"); err != nil {
			writeJSON(w, 409, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, 202, map[string]string{"status": "started"})
	}))
	mux.HandleFunc("POST /api/profiles/{name}/schedule", protected(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Enabled    *bool   `json:"enabled"`
			Preset     string  `json:"preset"`
			StaleAfter *string `json:"staleAfter"`
		}
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
			return
		}
		switch err := e.UpdateSchedule(r.PathValue("name"), core.ScheduleUpdate{Enabled: body.Enabled, Preset: body.Preset, StaleAfter: body.StaleAfter}); {
		case errors.Is(err, core.ErrUnknownProfile):
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		case errors.Is(err, core.ErrBadPreset):
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		case err != nil && strings.Contains(err.Error(), "staleAfter"):
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "staleAfter must be \"auto\" or a duration of at least 1h"})
		case err != nil:
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not save schedule"})
		default:
			writeJSON(w, http.StatusOK, map[string]string{"status": "saved"})
		}
	}))
	mux.HandleFunc("POST /api/run", protected(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 202, map[string]int{"started": e.RunAll("manual")})
	}))
	mux.HandleFunc("POST /api/reload", protected(func(w http.ResponseWriter, r *http.Request) {
		e.Reload()
		writeJSON(w, 200, map[string]string{"status": "reloaded"})
	}))
	for path, file := range static {
		path, file := path, file
		pattern := "GET " + path
		if path == "/" {
			pattern = "GET /{$}"
		}
		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			b, _ := webui.File(file)
			w.Header().Set("Content-Type", ctype[path])
			_, _ = w.Write(b)
		})
	}
	return guard(mux)
}

func guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.Host)
		if err != nil {
			host = strings.Trim(r.Host, "[]")
		}
		if host != "localhost" && host != "127.0.0.1" && host != "::1" {
			http.Error(w, "forbidden host", http.StatusForbidden)
			return
		}
		h := w.Header()
		h.Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self'; connect-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Cross-Origin-Resource-Policy", "same-origin")
		h.Set("Cache-Control", "no-store")
		r.Body = http.MaxBytesReader(w, r.Body, 4096)
		next.ServeHTTP(w, r)
	})
}
