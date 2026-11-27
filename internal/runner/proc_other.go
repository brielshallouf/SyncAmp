//go:build !windows

package runner

import (
	"os"
	"os/exec"
	"os/user"
	"strconv"
	"syscall"
	"time"
)

// prepare puts the client in its own process group so cancellation kills the
// launcher script *and* the java process it spawned, and optionally drops
// privileges to the profile owner.
func prepare(cmd *exec.Cmd, o Options) error {
	attr := &syscall.SysProcAttr{Setpgid: true}
	if o.RunAs != nil {
		cred := &syscall.Credential{Uid: o.RunAs.UID, Gid: o.RunAs.GID}
		env := os.Environ()
		if u, err := user.LookupId(strconv.Itoa(int(o.RunAs.UID))); err == nil {
			if gids, err := u.GroupIds(); err == nil {
				for _, g := range gids {
					if n, err := strconv.ParseUint(g, 10, 32); err == nil {
						cred.Groups = append(cred.Groups, uint32(n))
					}
				}
			}
			env = withEnv(env, "HOME", u.HomeDir, "USER", u.Username, "LOGNAME", u.Username)
		} else {
			env = withEnv(env, "HOME", "/nonexistent")
		}
		attr.Credential = cred
		cmd.Env = env
	}
	cmd.SysProcAttr = attr
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = 5 * time.Second
	return nil
}

// withEnv overrides key/value pairs in env.
func withEnv(env []string, kv ...string) []string {
	drop := map[string]bool{}
	for i := 0; i < len(kv); i += 2 {
		drop[kv[i]+"="] = true
	}
	out := env[:0:0]
	for _, e := range env {
		skip := false
		for p := range drop {
			if len(e) >= len(p) && e[:len(p)] == p {
				skip = true
			}
		}
		if !skip {
			out = append(out, e)
		}
	}
	for i := 0; i < len(kv); i += 2 {
		out = append(out, kv[i]+"="+kv[i+1])
	}
	return out
}
