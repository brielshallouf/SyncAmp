# SyncAmp

A cross-platform (Windows, macOS, Linux) service with a glass-style dashboard and native tray / menu-bar apps that supervises the
[Syncrify](https://web.synametrics.com/) backup client: scheduling, monitoring,
failure detection and alerts, with one binary and one config on every OS.

SyncAmp is independent of Synametrics. It does **not** reimplement the Syncrify
protocol; it drives the official client, so you keep Syncrify's delta engine,
encryption and server compatibility.

## How Syncrify backup works (what SyncAmp builds on)

From Synametrics' documentation:

- **Transport:** the client talks to the server over HTTP(S) (default web port 80; 5800 is a common alternative). All server administration is through its web UI.
- **Efficiency:** the rsync algorithm sends only changed *deltas* within files.
- **Profiles:** a backup job is a *profile*, an XML file with extension `.syncrify` stored in the user's `~/.syncrify` folder. Profiles are created in the client GUI. The schedule lives inside as `schedulerDays`, a weekday bitmap (Sun+Tue+Thu = `0010101` = 21; bit *n* = weekday *n*, Sunday = 0). Secrets like `encryptionKey` must never be hand-edited.
- **Running:** since v3.3 the client can run as a Windows service; the older "integrated" client used Windows Task Scheduler (deprecated). Headless: `SyncrifyClient.exe <profile>` on Windows, `SyncrifyClient.sh <profile>` on Linux, or `java -jar SyncrifyClient.jar -console <profile>`.

## What SyncAmp adds

| Stock Syncrify | SyncAmp |
|---|---|
| Schedules via Windows Task Scheduler / cron (differs per OS) | One built-in scheduler, identical on both OSes; catches slots missed during sleep |
| Console client may print a failure and still exit 0 | Exit code **and** output patterns decide success; timeouts kill the whole process tree |
| No at-a-glance status | Tray icon (green/blue/amber/red) + local web dashboard |
| No alerting | Webhook (Slack/Teams/ntfy-style) on failed backups and server up/down changes |
| Server health is opaque | Optional periodic server reachability check |
| Profiles are per-user | Scans every user's `~/.syncrify`, so a SYSTEM service finds profiles created by the logged-in user |

## Architecture

```
 syncamp (service + CLI)        ── Windows service / systemd unit; no GUI code linked
   ├─ profiles   discover & parse *.syncrify (read-only; symlinks, bad names, >1 MiB refused)
   ├─ scheduler  days×times or interval, window-based "due" check
   ├─ runner     launches the Syncrify client, redacts output, kills the process tree on timeout,
   │             (Linux, root service) drops to the profile owner's uid/gid
   ├─ store / monitor / notify / audit
   └─ api        127.0.0.1:5801, bearer-token auth + web dashboard
 syncamp-tray                   ── optional, per-user binary; polls the public /api/summary
```

Only `internal/platform` and `internal/runner/proc_*.go` contain OS-specific code.

## Install (headless-friendly: no GUI needed anywhere)

From an elevated prompt / root, with the `syncamp` binary for your OS (`make dist` builds all):

```
syncamp install        # creates secured data dir + API token, registers service
syncamp doctor         # security/config audit; fix anything FAIL/WARN
syncamp start
syncamp status         # exit 0 ok / 1 warn / 2 error — usable from cron, Nagios, Zabbix
```

Everything is scriptable from the CLI:

| Command | Purpose |
|---|---|
| `status [-json]` / `doctor [-json]` | health and security audit |
| `profiles`, `history [-n N]`, `logs NAME` | inspect profiles, runs, last run's (redacted) output |
| `backup [-wait] [NAME]` | trigger; with `-wait` blocks and exits non-zero on failure (good for scripts/CI) |
| `profile enable\|disable\|reset NAME` | per-profile switches |
| `profile schedule NAME -days mon,wed -times 02:00` / `-interval 6h` | override schedules |
| `config show\|validate\|set KEY VALUE` | safe edits (webhook, server.url, clientPath, …); webhook is shown redacted |
| `token` | print the API token for the web dashboard (`http://127.0.0.1:5801/`, e.g. via `ssh -L 5801:127.0.0.1:5801`) |
| `init`, `start/stop/restart`, `uninstall`, `run`, `version` | lifecycle |

Desktop users can additionally run `syncamp-tray` (and `syncamp-tray autostart on`) for the status icon.

## macOS (menu-bar app, Liquid Glass)

On a Mac the service runs as **your own LaunchAgent** (no root): your `~/.syncrify` profiles are found by default, backups run with your rights, and the API token in `~/Library/Application Support/SyncAmp` is readable only by you, so the menu-bar app can show full status and start backups.

```
syncamp install && syncamp start          # per-user LaunchAgent
cd macos && ./build-app.sh                # needs Xcode 15+; Xcode 26 enables Liquid Glass
open dist/SyncAmp.app                     # menu-bar only (no Dock icon); ⋯ menu → Launch at login
```

The app is SwiftUI with **zero third-party dependencies**: `glassEffect` on macOS 26 (Tahoe), system materials on macOS 14–15. Click the menu-bar icon for the popover: a state ring, one row per backup with a play button, "Back up all", and a ⋯ menu (dashboard, launch at login, quit).

The same design language powers the web dashboard (`http://127.0.0.1:5801/`) used on Windows/Linux and over SSH tunnels.

## Configuration

`%ProgramData%\SyncAmp\config.json` (Windows) or `/var/lib/syncamp/config.json` (Linux); `syncamp init` writes defaults. Unknown keys are rejected (a typo in a security option must not be silently ignored).

```json
{
  "clientPath": "C:\\Program Files\\Syncrify Client\\SyncrifyClient.exe",
  "clientArgs": ["{profile}"],
  "profileDirs": ["C:\\Users\\alice\\.syncrify"],
  "defaultTimes": ["02:00"],
  "webhook": "https://hooks.example.com/...",
  "server": { "url": "https://backup.lan:5800", "checkInterval": "1m" },
  "profiles": {
    "Documents": { "days": ["mon","wed","fri"], "times": ["12:30","22:00"] },
    "Photos":    { "interval": "6h" },
    "Scratch":   { "enabled": false }
  }
}
```

`clientPath` is auto-detected in standard install locations (never via `PATH`). `clientArgs` defaults to `["-console","{profile}"]`; if your launcher takes just the profile, use `["{profile}"]`. By default only the service account's own `~/.syncrify` is scanned — see **Windows profile folders** in [SECURITY.md](SECURITY.md) before adding other users' folders.

## Security

Backups are sensitive, so the service is locked down by default: loopback-only API with token auth, owner-only config/token/history, refusal to run a client that unprivileged users can modify, least-privilege execution on Linux, credential redaction, and a hardened systemd unit. Read **[SECURITY.md](SECURITY.md)** for the threat model, what is and isn't covered, and Windows-specific guidance.

## Toolchain & dependencies (audited 2026-11-27)

| | Version | Note |
|---|---|---|
| Go | 1.27.4 (latest) | `go.mod` pins it; the previous 1.24 is out of support |
| `github.com/kardianos/service` | v1.3.0 | latest; service/systemd integration |
| `fyne.io/systray` | v1.12.2 | latest; **only** in `syncamp-tray` |
| `github.com/godbus/dbus/v5` | v5.2.4 | transitive (tray); was 5.1.0 from 2022 |
| `golang.org/x/sys` | v0.50.0 | transitive; was 11 months old |

All shipped binaries (Windows/Linux × amd64/arm64) scan clean with `govulncheck -mode=binary` (`make vuln`). CI re-scans weekly, and Dependabot proposes updates. `make dist` writes `dist/SHA256SUMS`; binaries are static (`CGO_ENABLED=0`) and built with `-trimpath`.

## Status / limitations

- **Verified (Linux):** unit tests incl. race detector; end-to-end with a stub client covering CLI, exit codes, token auth, symlink refusal, redaction, privilege drop to the profile owner, refusal of a world-writable client, fail-closed config, startup permission repair; vulnerability scans of all binaries.
- **macOS: the Swift app has NOT been compiled or run** (no Swift/macOS toolchain in the build sandbox). The Go service cross-compiles for macOS (arm64/amd64) and scans clean, but the LaunchAgent install and Syncrify's macOS install paths are unverified. Run `./build-app.sh` (it runs the Swift unit tests first) and report any compile errors.
- **Compiles but untested on a real Windows host:** the Windows service path, `icacls` ACL hardening (`internal/platform/perms_windows.go`), the tray (ICO icon, `reg add` autostart), and the console-hiding/`taskkill` process-tree code.
- **Untested here:** the hardened systemd unit (no systemd in the build sandbox) — run `systemctl status syncamp` after install and report any sandbox denial.
- **Never tested with a real Syncrify client/server.** The profile XML schema beyond `schedulerDays` is inferred; `failurePatterns` are heuristics to tune against real client output.
- Slots missed while the service was *stopped* are not replayed (only sleep/late ticks are).
- Not implemented: MSI installer, code signing, restore verification, server-side management.
