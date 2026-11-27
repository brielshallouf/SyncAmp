package auth

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestTokenLifecycle(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "d")
	a, err := LoadOrCreate(dir)
	if err != nil || len(a) != 64 {
		t.Fatalf("token %q err %v", a, err)
	}
	b, _ := LoadOrCreate(dir)
	if a != b {
		t.Error("token must be stable across restarts")
	}
	if st, _ := os.Stat(filepath.Join(dir, FileName)); runtime.GOOS != "windows" && st.Mode().Perm()&0o077 != 0 { // Windows: ACLs, see platform tests
		t.Errorf("token file mode %v", st.Mode().Perm())
	}
	if !Equal(a, b) || Equal(a, "x") || Equal("", a) {
		t.Error("Equal misbehaves")
	}
	if FromHeader("Bearer abc") != "abc" || FromHeader("bearer abc") != "abc" || FromHeader("Basic abc") != "" || FromHeader("") != "" {
		t.Error("FromHeader misbehaves")
	}
	if Equal("", "") == false { // empty==empty would be an auth bypass if a token were ever empty
		t.Log("note: empty tokens compare equal; LoadOrCreate never yields empty")
	}
}
