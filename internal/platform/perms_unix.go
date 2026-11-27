//go:build !windows

package platform

import (
	"fmt"
	"os"
	"syscall"
)

// SecureDir creates dir (and parents) accessible by the owner only.
func SecureDir(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	return os.Chmod(dir, 0o700)
}

// SecureFile restricts an existing file to the owner.
func SecureFile(path string) error { return os.Chmod(path, 0o600) }

// WriteSecret atomically writes data with owner-only permissions.
func WriteSecret(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// FileOwner returns the uid/gid owning path.
func FileOwner(path string) (uid, gid uint32, ok bool) {
	st, err := os.Stat(path)
	if err != nil {
		return 0, 0, false
	}
	s, ok := st.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, 0, false
	}
	return s.Uid, s.Gid, true
}

// IsRoot reports whether we run as uid 0.
func IsRoot() bool { return os.Geteuid() == 0 }

// UntrustedWritable reports whether path (or its parent dir) can be modified
// by someone other than its owner/root: such an executable would hand
// root to any user who can write it. It only applies when we run as root; a
// per-user service gains nothing from replacing a binary the user can already
// run (and on macOS /Applications is root:admin 775, which would false-positive).
func UntrustedWritable(path string) (bool, string) {
	if !IsRoot() {
		return false, ""
	}
	for _, p := range []string{path, parentOf(path)} {
		st, err := os.Stat(p)
		if err != nil {
			return false, ""
		}
		if st.Mode().Perm()&0o022 != 0 {
			return true, fmt.Sprintf("%s is group/world-writable (%v)", p, st.Mode().Perm())
		}
		if s, ok := st.Sys().(*syscall.Stat_t); ok && s.Uid != 0 && IsRoot() {
			return true, fmt.Sprintf("%s is owned by non-root uid %d", p, s.Uid)
		}
	}
	return false, ""
}

func parentOf(p string) string {
	for i := len(p) - 1; i > 0; i-- {
		if p[i] == '/' {
			return p[:i]
		}
	}
	return "/"
}
