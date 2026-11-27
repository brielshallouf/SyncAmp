//go:build windows

package platform

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// These run only on a real Windows machine (CI). They verify that the icacls
// lockdown really leaves just SYSTEM and Administrators, which Unix-style mode
// bits cannot express.
func acl(t *testing.T, path string) string {
	t.Helper()
	out, err := exec.Command("icacls", path).CombinedOutput()
	if err != nil {
		t.Fatalf("icacls %s: %v\n%s", path, err, out)
	}
	t.Logf("icacls %s:\n%s", path, out)
	return string(out)
}

func assertLockedDown(t *testing.T, path string) {
	t.Helper()
	got := acl(t, path)
	for _, want := range []string{`NT AUTHORITY\SYSTEM`, `BUILTIN\Administrators`} {
		if !strings.Contains(got, want) {
			t.Errorf("%s: missing %q in ACL", path, want)
		}
	}
	for _, bad := range []string{`BUILTIN\Users`, `Everyone`, `Authenticated Users`} {
		if strings.Contains(got, bad) {
			t.Errorf("%s: %q must not have access:\n%s", path, bad, got)
		}
	}
}

func TestSecureDirAndWriteSecretLockDownACLs(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "SyncAmp")
	if err := SecureDir(dir); err != nil {
		t.Fatalf("SecureDir: %v", err)
	}
	assertLockedDown(t, dir)

	f := filepath.Join(dir, "api.token")
	if err := WriteSecret(f, []byte("secret")); err != nil {
		t.Fatalf("WriteSecret: %v", err)
	}
	assertLockedDown(t, f)
	if b, err := os.ReadFile(f); err != nil || string(b) != "secret" {
		t.Errorf("an administrator must still be able to read the file: %v %q", err, b)
	}
}

func TestScreenAndWorkAreaReadable(t *testing.T) {
	screen, work, ok := ScreenAndWorkArea()
	if !ok {
		t.Skip("no interactive desktop on this runner")
	}
	if screen.W() <= 0 || screen.H() <= 0 || work.W() <= 0 || work.H() <= 0 || work.W() > screen.W() || work.H() > screen.H() {
		t.Errorf("implausible geometry: screen=%+v work=%+v", screen, work)
	}
	if _, _, h, _ := PlacePanel(screen, work, 400, 680, 12); h <= 0 {
		t.Errorf("PlacePanel height %d", h)
	}
}
