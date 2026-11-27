# SyncAmp

A cross-platform (Windows + Linux) service and system-tray app that supervises the
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
 syncamp (service)  ── runs as Windows service / systemd unit
   ├─ profiles   discover & parse *.syncrify (read-only, secrets masked)
   ├─ scheduler  days×times or interval, window-based "due" check
   ├─ runner     launches the Syncrify client, captures output, kills tree on timeout
   ├─ store      run history (JSON)      ├─ monitor  server health
   ├─ notify     webhook                 └─ api      127.0.0.1:5801 + dashboard
 syncamp tray   ── per-user process; polls the API (a service can't draw a tray icon: Windows session 0)
```

Only `internal/platform` and the tiny `runner/proc_*.go` files contain OS-specific code.

## Install

Download/build (`make dist` produces Windows and Linux binaries), then from an **elevated** prompt / root:

```
syncamp install      # Windows service / systemd unit
syncamp start
syncamp tray-autostart on     # run as the logged-in user; starts the tray at login
syncamp tray
```

Open the dashboard at <http://127.0.0.1:5801/>. Other commands: `status`, `profiles`, `backup [NAME]`, `init`, `restart`, `uninstall`.

## Configuration

`%ProgramData%\SyncAmp\config.json` (Windows) or `/var/lib/syncamp/config.json` (Linux root); `syncamp init` writes defaults.

```json
{
  "clientPath": "C:\\Program Files\\Syncrify Client\\SyncrifyClient.exe",
  "clientArgs": ["{profile}"],
  "defaultTimes": ["02:00"],
  "webhook": "https://hooks.example.com/...",
  "server": { "url": "http://backup.lan:5800", "checkInterval": "1m" },
  "profiles": {
    "Documents": { "days": ["mon","wed","fri"], "times": ["12:30","22:00"] },
    "Photos":    { "interval": "6h" },
    "Scratch":   { "enabled": false }
  }
}
```

`clientPath` is auto-detected in common install locations. `clientArgs` defaults to `["-console","{profile}"]`; if your launcher takes just the profile (as in Syncrify's Windows docs), set `["{profile}"]`. Time of day is not stored in a way SyncAmp relies on, so profile `schedulerDays` are combined with `defaultTimes` unless overridden.

## Security

The API binds to loopback only, rejects non-loopback `Host` headers (DNS rebinding) and requires an `X-SyncAmp` header on POSTs (CSRF). It exposes no secrets (profile passwords/keys are masked). Webhook URLs and history are stored unencrypted in the data directory.

## Status / limitations

- Verified: unit tests for scheduling, profile parsing, runner outcomes and the API guard; end-to-end run on Linux with a stub client; Windows cross-compiles.
- **Not yet tested on a real Windows host or with a real Syncrify client/server.** The profile XML schema beyond `schedulerDays` is inferred, and failure patterns (`config.failurePatterns`) are heuristics to tune against real client output.
- The tray was not exercised in a desktop session. It uses `fyne.io/systray`; Linux needs a StatusNotifier-capable desktop (GNOME needs an AppIndicator extension).
- Missed slots while the service was *stopped* are not replayed (only sleep/late ticks are).
- Not yet implemented: server-side management (users, quotas), restore verification, installer (MSI), code signing, per-profile tray submenus.
