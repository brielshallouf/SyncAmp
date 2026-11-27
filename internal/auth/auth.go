// Package auth manages the local API bearer token.
//
// The token (256 random bits) lives in the data directory, which is readable
// only by root / Administrators. Possession of the token is therefore the
// proof that a caller is an administrator of this machine - the loopback API
// otherwise reachable by every local user is closed to everyone else.
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/brielshallouf/syncamp/internal/platform"
)

// FileName is the token file inside the data directory.
const FileName = "api.token"

// LoadOrCreate returns the token in dir, generating it on first use.
func LoadOrCreate(dir string) (string, error) {
	if t, err := Read(dir); err == nil {
		return t, nil
	}
	if err := platform.SecureDir(dir); err != nil {
		return "", err
	}
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	t := hex.EncodeToString(b)
	return t, platform.WriteSecret(filepath.Join(dir, FileName), []byte(t+"\n"))
}

// Read returns the existing token or an error (typically permission denied
// for non-admin users, which is the point).
func Read(dir string) (string, error) {
	b, err := os.ReadFile(filepath.Join(dir, FileName))
	if err != nil {
		return "", err
	}
	t := strings.TrimSpace(string(b))
	if len(t) < 32 {
		return "", errors.New("token file is corrupt")
	}
	return t, nil
}

// Equal compares tokens in constant time (hashing first hides length).
func Equal(a, b string) bool {
	ha, hb := sha256.Sum256([]byte(a)), sha256.Sum256([]byte(b))
	return subtle.ConstantTimeCompare(ha[:], hb[:]) == 1
}

// FromHeader extracts a bearer token from an Authorization header value.
func FromHeader(h string) string {
	const p = "Bearer "
	if len(h) > len(p) && strings.EqualFold(h[:len(p)], p) {
		return strings.TrimSpace(h[len(p):])
	}
	return ""
}
