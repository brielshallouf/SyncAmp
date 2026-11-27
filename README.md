# SyncAmp

A **client-side companion for [Syncrify](https://web.synametrics.com/) backups** on Windows, macOS and Linux: a small background service plus a tray popover (and a headless CLI) that tells you whether this machine's backups are actually healthy, lets you run them on demand, and alerts you when one fails or goes **overdue**.

**Scope.** SyncAmp runs on each machine being backed up and watches that machine's Syncrify client. It does not administer a Syncrify *server* (users, quotas, storage, settings): use Syncrify's own web console for that. Its only server feature is a connection check: "can this machine reach the server, and does it answer like Syncrify?".

SyncAmp is independent of Synametrics. It does **not** reimplement the Syncrify protocol; it drives and observes the official client, so you keep Syncrify's delta engine, encryption and server compatibility. What we know about Syncrify (and what we don't) is in [docs/SYNCRIFY-NOTES.md](docs/SYNCRIFY-NOTES.md).

## Two modes

The Syncrify client already schedules its own backups (its background service, or Task Scheduler / cron / launchd). So SyncAmp defaults to **monitor** mode and never starts a scheduled backup behind the client's back.

| | `monitor` (default) | `scheduler` |
|---|---|---|
| Who starts scheduled backups | the Syncrify client | SyncAmp |
| SyncAmp's job | watch, verify, run on demand, alert | all of that, plus replace Task Scheduler / cron |
| Use when | you use the client normally | you have **disabled** the client's own schedule (otherwise runs collide; the client locks the profile, so the second run is reported as a warning) |

Switch with `syncamp config set mode scheduler`.

## What SyncAmp adds

| Stock Syncrify | SyncAmp |
|---|---|
| No at-a-glance status | Tray popover (state ring, per-backup rows) + local dashboard + `syncamp status` with monitoring-friendly exit codes |
| A failed backup is easy to miss | Real outcome detection: exit code **and** the client's own messages ("…encountered one or more errors", "Unable to connect…"). "Success with errors" is a distinct **warning** |
| A backup that silently *stopped happening* is invisible | **Overdue alerts**: no successful backup within the window derived from the profile's weekday schedule (longest gap + 12 h), or one you set (`syncamp profile stale NAME 26h`) |
| Console client may print a failure and exit 0 | Output is checked too; timeouts kill the whole process tree |
| No alerting | Webhook (Slack/Teams/ntfy-style) on failures, overdue and recovery, and server unreachable/restored (debounced: two failed checks) |
| Hard to tell if the server is the right one | Connection check calls the server's anonymous build-number operation: shows "Server build N" and warns if the URL answers but isn't Syncrify |
| Profiles can run scripts as the service account | Hardened discovery: only your own `~/.syncrify` plus folders you list; symlinks refused; Linux runs each backup as the profile's owner (see [SECURITY.md](SECURITY.md)) |

**Where the signals come from.** SyncAmp always knows about backups it started (run on demand, or scheduled in `scheduler` mode). For backups the client ran by itself it can optionally read the client's own HTML logs (`"logWatch": true`, **experimental**: log layout is unverified, so it is off by default and only attributes a log to a profile when the profile's name is in the file name). With no signal, SyncAmp shows "not seen yet" and never raises an overdue alarm it can't justify.

## How Syncrify backup works (what SyncAmp builds on)

From Synametrics' documentation:

- **Transport:** the client talks to the server over HTTP(S) (the server build we inspected ships with HTTP on port **5800** and HTTPS not configured: see [docs/SYNCRIFY-NOTES.md](docs/SYNCRIFY-NOTES.md)). All server administration is through its web UI.
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
| No hint when the server can't be reached | Optional connection check from this machine to your Syncrify server, with alerts |
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

### Windows / Linux tray popover
`syncamp-tray` opens the dashboard as a compact chromeless window (Edge on Windows, Chromium-family on Linux). On Windows it is placed in the corner next to the taskbar, **whichever edge the taskbar is docked to** (top, bottom, left or right); there is deliberately no pointer arrow. Expand a backup to pause/resume it or pick **Hourly / Every 6 h / Daily** (also available as `syncamp profile schedule` and `POST /api/profiles/NAME/schedule`). The same page works in any browser and over SSH tunnels (`http://127.0.0.1:5801/`).

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
  "mode": "monitor",
  "staleAfter": "auto",
  "logWatch": false,
  "profiles": {
    "Documents": { "staleAfter": "26h" },
    "Photos":    { "staleAfter": "auto" },
    "Scratch":   { "enabled": false }
  }
}
```

`mode`, `staleAfter`, per-profile `staleAfter`, `logWatch` and `logDirs` are described above; schedule overrides (`days`, `times`, `interval`) only apply in `scheduler` mode. `clientPath` is auto-detected in standard install locations (never via `PATH`). `clientArgs` defaults to `["-console","{profile}"]`; if your launcher takes just the profile, use `["{profile}"]`. By default only the service account's own `~/.syncrify` is scanned — see **Windows profile folders** in [SECURITY.md](SECURITY.md) before adding other users' folders.

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
- **Never run against a live Syncrify client/server.** `failurePatterns`/`warnPatterns` now use the product's own message strings (from the installer you provided), but whether the console client prints them to stdout is unverified, and the profile XML element names beyond `schedulerDays` are still unknown, so SyncAmp still uses `defaultTimes` for time of day. See [docs/SYNCRIFY-NOTES.md](docs/SYNCRIFY-NOTES.md).
- **Overdue/observed signals are new and lightly validated:** the freshness logic is unit-tested, and log-watching is tested against the product's own message strings with synthetic logs, but neither has seen a real client log. If your Syncrify client already runs its own schedule, leave SyncAmp in `monitor` mode.
- Slots missed while the service was *stopped* are not replayed (only sleep/late ticks are).
- Not implemented: MSI installer, code signing, restore verification. Out of scope by design: Syncrify server administration.
