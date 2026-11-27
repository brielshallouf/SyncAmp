# SyncAmp security notes

SyncAmp supervises backups, so it runs with high privilege (SYSTEM / root) and touches your data and
credentials. This document states what it defends against, how, and what it does **not** cover.

## Threat model

| Adversary | Goal | Defence |
|---|---|---|
| Unprivileged local user | Make the privileged service back up files (or, since the Syncrify client supports **pre/post-backup scripts**, run commands) they can't read (via a planted `.syncrify` profile) | Only the service account's own `~/.syncrify` and admin-listed `profileDirs` are scanned. **Linux:** the client is run as the profile's *owner* (uid/gid + groups), so a profile can never read more than its owner could. Symlinked/oversized/oddly-named profiles are refused. |
| Unprivileged local user | Get code run as SYSTEM/root via the client binary or config | Config/token/history are owner-only (`0600`/`0700`; Windows ACL = SYSTEM + Administrators). A client executable (or its folder) that is group/world-writable or non-root-owned is **refused** on Unix; `doctor` flags it. `clientPath` must be absolute — no `PATH` lookup. Config is validated on load; unknown keys rejected; invalid config = service won't start (fail closed). |
| Unprivileged local user | Use or read the local API | Every `/api/*` route except `/api/summary` needs a 256-bit bearer token stored in the admin-only data dir (constant-time compare). `/api/summary` exposes only `{overall, summary}`. |
| Malicious web page in an admin's browser | Drive the API (CSRF), DNS rebinding | Loopback-only listener (enforced), `Host` header must be loopback, `Authorization` header forces a CORS preflight that is never granted. |
| Attacker controlling file *names/contents* shown in the dashboard | XSS to steal the token | Strict CSP (`script-src 'self'`, no inline), all dynamic text via `textContent`, profile names restricted to `[A-Za-z0-9 ._-]`, framing denied. |
| Anyone reading logs/history/webhooks | Harvest credentials echoed by the client | Output is redacted (`password`, `token`, `secret`, `api key`, `encryptionKey`, `Bearer/Basic …`), line- and size-capped before storage or sending. Profile contents are never exposed via the API; secrets in profiles are masked internally. Webhook URLs are shown redacted in `config show`. |
| Network attacker | Sniff backup traffic / webhook | Webhooks must be `https://` (explicit opt-out `allowInsecureWebhook`). `doctor` warns on a plain-`http` `server.url`. (Syncrify's own transport/encryption is the client's responsibility.) |
| Disk-fill / resource abuse | DoS | Size-rotated log, capped history (count and per-run size), HTTP server timeouts and body/header limits, per-run timeout that kills the whole process tree. |

## Monitor mode is also the safer default
Because SyncAmp no longer starts backups by itself unless you opt into `scheduler` mode, it cannot double-run a profile or be tricked into running one on a schedule it did not create. Log watching treats the (possibly user-writable) logs folder as hostile: regular files only, size-capped, outcomes keyed on content, credentials redacted, and it never executes anything it reads.

## Hardening in the box
- Service and CLI binary contain **no GUI code**; the desktop app is a separate unprivileged program. Smaller dependency surface (the web-runtime and tray libraries are only in the app).
- Linux unit: `NoNewPrivileges`, `ProtectSystem=full`, `PrivateTmp`, kernel/cgroup/clock protections, `RestrictSUIDSGID`, `RestrictNamespaces`, `RestrictAddressFamilies`, `UMask=0077`. Deliberately *not* set: `ProtectHome` and a capability bound (backups need home access; root needs `CAP_DAC_READ_SEARCH`/`CAP_SETUID`) and `MemoryDenyWriteExecute` (Java JIT).
- Startup repairs loose permissions on config, token, history and log.
- `syncamp doctor` audits all of the above and is safe to run from monitoring (exit 0/1/2).

## macOS
The service is a **per-user LaunchAgent**, not a root daemon: nothing runs with elevated rights, there is no privilege-escalation path through profiles, and the API token (`0600`, in your Application Support folder) is readable only by you. The menu-bar app reads that token directly. Grant Full Disk Access to the `syncamp` binary (System Settings → Privacy & Security) if you back up protected folders. The "client is user-writable" check is skipped for non-root services (it would false-positive on `/Applications`, which is `root:admin 775`, and a per-user service gains nothing from it).

## The desktop app (Windows window, Linux icon)
The app runs as the ordinary logged-in user and talks to the service only through an in-process bridge:
- **The page cannot reach the network.** Its UI is inlined and pinned by a hash-based Content-Security-Policy (no `unsafe-inline`, no `connect-src`, no navigation). Its only channel is the bridge, which allows `GET`/`POST` to `/api/…` paths only (no `..`, queries or absolute URLs), caps body and response sizes, and takes the service address only from a root-written file, ignored unless loopback. There is no extra listening port.
- **The access key never reaches the page.** The bridge attaches it in Go, and only **after the listener proves it holds the key**: the app sends a random nonce and checks an HMAC of it before sending the key itself (`/api/proof`). A local user who grabbed the service port while the service was stopped therefore learns nothing; the CLI does the same.
- **Why no sign-in:** `install` grants the installing account read access to exactly two small files (the key and the address) via an ACL (Windows `icacls`, Linux POSIX ACL; macOS already runs as the user). The folder's other files (config, history, log) stay administrator-only; the grants are re-applied on every service start; `syncamp grant ACCOUNT` adds another account. An administrator's *non-elevated* token cannot use the Administrators group, so the explicit grant is what lets the app work. Anyone granted can drive SyncAmp's API (start backups, change schedules) but cannot change what the service executes.
- **Native window ≠ browser:** WebView2 uses a private data folder under `%LOCALAPPDATA%\SyncAmp`, the window has no address bar, and the page loads no remote content.

## Windows profile folders (read this)
*The installer's "Let SyncAmp see my Syncrify profiles" checkbox and `syncamp watch FOLDER` are option 2 below, and are **off / not done by default** for this reason.*

Windows cannot drop privileges per profile the way the Linux build does. Syncrify stores profiles in the *interactive user's* `%USERPROFILE%\.syncrify`, while a LocalSystem service's own home is elsewhere. Pick one:

1. **Recommended — run the service as the user that owns the profiles** (Services → SyncAmp → Log On, or `sc config SyncAmp obj= ".\alice" password= ...`). Then the backup has exactly that user's rights and the default scan finds their profiles. No escalation path exists.
2. Keep LocalSystem and list **admin-controlled** folders in `profileDirs` (copy profiles there; ACL = Administrators/SYSTEM only). Do **not** list a folder standard users can write: they could plant a profile that SYSTEM runs.
3. `scanAllUserHomes: true` — only on single-user machines; `doctor` warns because it is an escalation path on shared machines.

## Known residual risks / not covered
- **Loopback port squatting** can no longer steal the key (the app and CLI verify the listener first, see above), but a local user could still occupy the port while the service is stopped, which only makes the app show "service not running".
- **Windows ACL hardening and the systemd sandbox are untested on real hosts** (see README).
- History and logs are not tamper-evident; webhook messages contain profile names and error text.
- The API is plain HTTP on loopback; do not forward it to a network without TLS (e.g. use an SSH tunnel).
- Binaries are not code-signed; verify `SHA256SUMS` and build from source if in doubt.
- Syncrify client/server vulnerabilities, server-side access control and backup encryption are outside SyncAmp (it is a client-side tool and does not manage the server).

## Reporting
Open a private security advisory on the repository.
