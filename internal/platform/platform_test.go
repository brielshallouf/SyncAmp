//go:build !windows

package platform

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUntrustedWritable(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("check only applies to a root service")
	}
	dir := t.TempDir()
	_ = os.Chmod(dir, 0o755)
	exe := filepath.Join(dir, "client.sh")
	_ = os.WriteFile(exe, []byte("#!/bin/sh\n"), 0o755)
	if bad, why := UntrustedWritable(exe); bad && os.Geteuid() != 0 {
		t.Errorf("safe file flagged: %s", why)
	}
	_ = os.Chmod(exe, 0o777)
	if bad, _ := UntrustedWritable(exe); !bad {
		t.Error("world-writable executable must be flagged")
	}
	_ = os.Chmod(exe, 0o755)
	_ = os.Chmod(dir, 0o777)
	if bad, _ := UntrustedWritable(exe); !bad {
		t.Error("executable in world-writable dir must be flagged")
	}
}

func TestSecureDirIsPrivate(t *testing.T) {
	d := filepath.Join(t.TempDir(), "a", "b")
	if err := SecureDir(d); err != nil {
		t.Fatal(err)
	}
	if st, _ := os.Stat(d); st.Mode().Perm() != 0o700 {
		t.Errorf("mode %v", st.Mode().Perm())
	}
}
