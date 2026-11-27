// Package svcclient is how the desktop app talks to the background service.
//
// The app's window runs web content, so this package is the security boundary:
// the page can only ask for GET/POST requests to /api/... paths, the service
// address must be loopback, and the access key is attached HERE (read from a file
// only the machine's administrators and explicitly granted users can read). The
// page never sees the key, and no extra listening port is opened.
package svcclient

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/brielshallouf/syncamp/internal/auth"
)

// AddrFile is written by the service at start with its (loopback) listen address.
const AddrFile = "api.addr"

const (
	defaultAddr = "127.0.0.1:5801"
	maxBody     = 4096
	maxResp     = 4 << 20
)

// Client calls the local service.
type Client struct {
	Dir  string // the service's data directory
	HTTP *http.Client
}

// New builds a client for the service data directory dir.
func New(dir string) *Client {
	return &Client{Dir: dir, HTTP: &http.Client{Timeout: 10 * time.Second}}
}

// Result is a status code and body; Status 0 means the service was unreachable.
type Result struct {
	Status int    `json:"status"`
	Body   string `json:"body"`
}

func loopback(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// Addr returns the service address: the service-written file if present and
// loopback, else the default. A non-loopback value is ignored, never trusted.
func (c *Client) Addr() string {
	if b, err := os.ReadFile(filepath.Join(c.Dir, AddrFile)); err == nil {
		if a := strings.TrimSpace(string(b)); loopback(a) {
			return a
		}
	}
	return defaultAddr
}

// Token returns the access key if this account may read it, else "".
func (c *Client) Token() string {
	t, _ := auth.Read(c.Dir)
	return t
}

func okPath(p string) bool {
	if len(p) < 6 || len(p) > 256 || !strings.HasPrefix(p, "/api/") {
		return false
	}
	if strings.Contains(p, "..") || strings.Contains(p, "//") || strings.Contains(p, `\`) || strings.Contains(p, "?") || strings.Contains(p, "#") {
		return false
	}
	for _, r := range p {
		if r < 0x21 || r == 0x7f { // controls and spaces
			return false
		}
	}
	return true
}

// IsHex reports whether s is non-empty lowercase/uppercase hex.
func IsHex(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f' || r >= 'A' && r <= 'F') {
			return false
		}
	}
	return true
}

// Proof is what the real service answers to a nonce: HMAC-SHA256 keyed by the access key.
func Proof(token, nonce string) string {
	m := hmac.New(sha256.New, []byte(token))
	m.Write([]byte("syncamp-proof:" + nonce))
	return hex.EncodeToString(m.Sum(nil))
}

// Verify checks that whatever answers at base really knows the access key, using a fresh random
// nonce. Only a process that has the key can compute the answer, so call this before sending the
// key anywhere: it defeats a local user who started listening on the port before the service.
func Verify(h *http.Client, base, token string) bool {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return false
	}
	nonce := hex.EncodeToString(b)
	resp, err := h.Get(base + "/api/proof?nonce=" + nonce)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	var got struct{ Proof string }
	if resp.StatusCode != 200 || json.NewDecoder(io.LimitReader(resp.Body, 1024)).Decode(&got) != nil {
		return false
	}
	return hmac.Equal([]byte(got.Proof), []byte(Proof(token, nonce)))
}

// Do performs one request. Anything outside the allow-list is refused without
// touching the network.
func (c *Client) Do(method, path, body string) Result {
	if (method != http.MethodGet && method != http.MethodPost) || !okPath(path) || len(body) > maxBody {
		return Result{Status: 400, Body: `{"error":"request not allowed"}`}
	}
	req, err := http.NewRequest(method, "http://"+c.Addr()+path, strings.NewReader(body))
	if err != nil {
		return Result{Status: 400, Body: `{"error":"bad request"}`}
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if t := c.Token(); t != "" && path != "/api/summary" {
		if !Verify(c.HTTP, "http://"+c.Addr(), t) {
			return Result{Status: 0, Body: `{"error":"the listener on the service port did not prove it is the SyncAmp service"}`}
		}
		req.Header.Set("Authorization", "Bearer "+t)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return Result{Status: 0, Body: `{"error":"the SyncAmp service is not running"}`}
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, maxResp))
	return Result{Status: resp.StatusCode, Body: string(b)}
}
