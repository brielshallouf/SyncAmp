//go:build windows

package platform

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

// Well-known SIDs (locale independent): SYSTEM and BUILTIN\Administrators.
const aclGrant = "*S-1-5-18:(OI)(CI)F"
const aclGrantAdmins = "*S-1-5-32-544:(OI)(CI)F"

func icacls(args ...string) error {
	c := exec.Command("icacls", args...)
	c.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return c.Run()
}

// SecureDir creates dir and removes inherited access, leaving only SYSTEM and
// Administrators (ProgramData is otherwise readable by every local user).
func SecureDir(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	return icacls(dir, "/inheritance:r", "/grant:r", aclGrant, "/grant:r", aclGrantAdmins)
}

// SecureFile: files inherit the locked-down directory ACL; re-apply for safety.
func SecureFile(path string) error {
	return icacls(path, "/inheritance:r", "/grant:r", "*S-1-5-18:F", "/grant:r", "*S-1-5-32-544:F")
}

// WriteSecret atomically writes data into the (already secured) directory,
// then locks the file down explicitly.
func WriteSecret(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	return SecureFile(path)
}

func FileOwner(string) (uint32, uint32, bool) { return 0, 0, false }
func IsRoot() bool                            { return false }

// UntrustedWritable is not evaluated on Windows (ACL inspection needs the
// Win32 security API); `doctor` tells the admin to check Program Files ACLs.
func UntrustedWritable(string) (bool, string) { return false, "" }

// grantRead lets account list the folder (this folder only, not inherited) and read the given
// files. The folder's other files (config, history, log) stay administrator-only.
func grantRead(dir, account string, files []string) error {
	if err := icacls(dir, "/grant", account+":(RX)"); err != nil {
		return fmt.Errorf("icacls %s: %w", dir, err)
	}
	for _, f := range files {
		if _, err := os.Stat(f); err != nil {
			continue
		}
		if err := icacls(f, "/grant", account+":(R)"); err != nil {
			return fmt.Errorf("icacls %s: %w", f, err)
		}
	}
	return nil
}
