package platform

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
)

// Why grants exist: the service's data folder (access key, config) is readable only
// by administrators. The desktop app runs as the ordinary logged-in user, and an
// administrator's *non-elevated* token cannot use the Administrators group either,
// so without help the app could not talk to the service and would have to ask the
// user to paste a key. `install` therefore grants the installing account read access
// to just two small files (the key and the service address), and the service
// re-applies the grants every start (rewriting a file resets its permissions).

const grantsFile = "grants"

// validAccount allows normal account names ("DOMAIN\\alice", "alice", "alice@example.com")
// and nothing that could be mistaken for an option or a path.
func validAccount(u string) bool {
	if u == "" || len(u) > 128 || strings.HasPrefix(u, "-") {
		return false
	}
	for _, r := range u {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case strings.ContainsRune(` .\_-@$`, r):
		default:
			return false
		}
	}
	return true
}

// Grants lists the accounts granted read access.
func Grants(dir string) []string {
	b, err := os.ReadFile(filepath.Join(dir, grantsFile))
	if err != nil {
		return nil
	}
	var out []string
	for _, l := range strings.Split(string(b), "\n") {
		if l = strings.TrimSpace(l); validAccount(l) {
			out = append(out, l)
		}
	}
	return out
}

// AddGrant remembers account (once) and applies the grant to files.
func AddGrant(dir, account string, files ...string) error {
	if !validAccount(account) {
		return fmt.Errorf("%q is not a valid account name", account)
	}
	have := Grants(dir)
	found := false
	for _, g := range have {
		found = found || strings.EqualFold(g, account)
	}
	if !found {
		have = append(have, account)
		if err := WriteSecret(filepath.Join(dir, grantsFile), []byte(strings.Join(have, "\n")+"\n")); err != nil {
			return err
		}
	}
	return grantRead(dir, account, files)
}

// ApplyGrants re-applies every remembered grant. Errors are returned, not fatal:
// a missing account must not stop the service.
func ApplyGrants(dir string, files ...string) []error {
	var errs []error
	for _, g := range Grants(dir) {
		if err := grantRead(dir, g, files); err != nil {
			errs = append(errs, fmt.Errorf("grant %s: %w", g, err))
		}
	}
	return errs
}

// InvokingAccount is the human who ran `install`, to be granted access: the current
// account on Windows (an elevated process keeps the same account), or $SUDO_USER on
// Linux. Empty when it would be SYSTEM/root (nobody to grant).
func InvokingAccount() string {
	if runtime.GOOS == "windows" {
		if u, err := user.Current(); err == nil && !strings.Contains(strings.ToUpper(u.Username), "SYSTEM") && validAccount(u.Username) {
			return u.Username
		}
		return ""
	}
	if su := os.Getenv("SUDO_USER"); su != "" && su != "root" && validAccount(su) {
		return su
	}
	return ""
}
