# Getting started with SyncAmp

SyncAmp runs next to the **Syncrify client** on the machine being backed up. It watches your backups, alerts you
when one fails or goes overdue, and lets you run them on demand. It does not replace the Syncrify client and does
not manage the Syncrify server.

**Before you start** you need: the Syncrify client installed and at least one backup profile created in it.

- [Windows](#windows) · [macOS](#macos) · [Linux and headless servers](#linux-and-headless-servers)
- [Using it day to day](#using-it-day-to-day) · [Alerts](#alerts-to-your-phone-or-chat) · [Troubleshooting](#troubleshooting)

> Commands below use `syncamp`. On Windows that is the file you downloaded, renamed to `syncamp.exe`.

---

## Windows

**1. Run the installer.** Download **`SyncAmp-Setup.exe`** from the [Releases page](https://github.com/brielshallouf/SyncAmp/releases) (open the *Latest development build* entry) and run it. Windows will show a SmartScreen warning because the file is not code-signed: click **More info → Run anyway** ([details](#first-run-warnings)). Say **Yes** to the administrator prompt: it is needed to register the background service.

**2. Make one choice.** The installer has a single checkbox, *"Let SyncAmp see my Syncrify profiles in `C:\Users\you\.syncrify`"*. It is **off by default** on purpose:

- The background service runs as `SYSTEM`, and a Syncrify profile can run scripts. If SyncAmp watches a folder that *you* can write to, anything running as you could plant a profile and have `SYSTEM` run it.
- **Tick it** on a personal PC where only you use the machine and the Syncrify client keeps its profiles in your user folder (the usual case), and you accept that trade.
- **Leave it off** if you are unsure; the app tells you when it can't see any profiles, and you can turn it on later (below).

**3. Click the SyncAmp icon next to the clock** (it may be under the **^** overflow arrow; drag it out to keep it visible). The app opens its own window beside the taskbar. That's it: no sign-in, no second download. The installer also starts SyncAmp at every login.

> **How does it open without a password?** The installer gives the account that ran it read access to just two small files the service publishes (its access key and address). Any other account on the PC needs an administrator to run `syncamp grant THEIR_ACCOUNT` first.

**Later: tell SyncAmp about profiles or the client.** In an **Administrator** PowerShell (the command-line program lives in `C:\Program Files\SyncAmp`):

```powershell
cd "C:\Program Files\SyncAmp"
.\syncamp.exe watch "C:\Users\you\.syncrify"                       # same as the checkbox
.\syncamp.exe config set clientPath "C:\Program Files\Syncrify Client\SyncrifyClient.exe"
.\syncamp.exe doctor                                                # tells you anything else to fix
.\syncamp.exe status
```

Not sure where your profiles are? `Get-ChildItem C:\Users -Recurse -Filter *.syncrify -ErrorAction SilentlyContinue`.
Prefer the service to run as you instead (so no folder needs trusting)? Press `Win+R`, run `services.msc`, open **SyncAmp → Properties → Log On → This account**, enter an *administrator* account and its password, and restart the service.

**For IT / scripted installs:** `SyncAmp-Setup.exe /VERYSILENT /SUPPRESSMSGBOXES /NORESTART` (add `/TASKS="watchprofiles"` for the profile checkbox).

**Uninstall:** *Settings → Apps → SyncAmp → Uninstall*. It stops and removes the service and the login entry, and asks whether to delete your settings and history.

---

## macOS

On a Mac, SyncAmp runs as **you** (a login item), not as root, so there is no `sudo` and your `~/.syncrify` profiles are found automatically.

**1.** Download **`SyncAmp-macos.zip`** from the [Releases page](https://github.com/brielshallouf/SyncAmp/releases), unzip it and drag **SyncAmp** into *Applications*.

**2.** Open it. macOS will warn that it can't verify the developer: right-click the app → **Open** → **Open** (or run `xattr -dr com.apple.quarantine /Applications/SyncAmp.app`). The first time, the app sets up its own background service (a few seconds: you will see "Setting up the background service…"). That's the whole install: there is no separate download.

**3.** Click the menu-bar icon. Use the **⋯** menu → *Launch at login*. On macOS 26 it uses Apple's Liquid Glass look; older versions use the system blur.

Terminal users: the command-line program is inside the app, e.g. `/Applications/SyncAmp.app/Contents/Resources/syncamp status`. If you back up protected folders, give that `syncamp` program **Full Disk Access** in *System Settings → Privacy & Security*.

> The app is built and unit-tested on macOS in CI, but nobody has opened it on a real desktop yet. Please report anything odd.

---

## Linux and headless servers

You do not need the tray or any desktop. Everything works from the command line.

```bash
sudo install -m 0755 syncamp-linux-amd64 /usr/local/bin/syncamp     # use -arm64 on ARM
sudo syncamp install        # creates /var/lib/syncamp (root-only) and a hardened systemd unit
sudo syncamp doctor
sudo syncamp start
sudo syncamp status
```

- **Profiles in a user's home.** As a root service, SyncAmp only scans `/root/.syncrify` plus folders you list.
  Add the user's folder in `/var/lib/syncamp/config.json`:
  ```json
  { "profileDirs": ["/home/alice/.syncrify"] }
  ```
  then `sudo syncamp restart`. Each backup then runs **as that profile's owner**, not as root.
- **Optional browser dashboard (for a headless server you reach over SSH):** `ssh -L 5801:127.0.0.1:5801 server`, open <http://127.0.0.1:5801/>, and paste the output of
  `sudo syncamp token`. The service only listens on the loopback address. The desktop apps never need this.
- **Monitoring:** `syncamp status` exits **0** (ok), **1** (warning) or **2** (error/unreachable), so cron, Nagios,
  Zabbix or a shell script can use it directly. `syncamp status -json` is machine-readable and
  `syncamp backup -wait NAME` blocks and fails the script if the backup fails.
- **Logs:** `journalctl -u SyncAmp` and `/var/lib/syncamp/syncamp.log`.
- **Desktop icon (optional):** run `syncamp-app-linux-amd64` as your normal user (and `syncamp-app autostart on` to start it at login). It shows a status icon with a native menu (status, *Back up all now*, *Quit*) and needs a desktop that supports status-notifier icons (GNOME needs the AppIndicator extension). `sudo syncamp install` grants the account that ran it access, using POSIX ACLs (the `acl` package must be installed); otherwise run `sudo syncamp grant YOUR_USER`.

---

## Using it day to day

| I want to… | Do this |
|---|---|
| See if everything is fine | Look at the SyncAmp icon, or `syncamp status` |
| Back up something right now | Press ▶ next to it in the window, or `syncamp backup NAME` (`-wait` to wait) |
| See what happened | **Activity** in the window, `syncamp history`, or `syncamp logs NAME` for the last run's output |
| Be warned if a backup silently stops | Expand a backup in the window → **Alert if overdue**, or `syncamp profile stale NAME 26h` |
| Pause a profile | `syncamp profile disable NAME` (`enable` to resume) |
| Check security/config | `syncamp doctor` |

**Monitor vs scheduler mode.** By default SyncAmp is in **monitor** mode: the Syncrify client keeps running its
own schedule and SyncAmp watches. Switch to `scheduler` mode (`syncamp config set mode scheduler`) *only* if you
have turned the client's own schedule off, otherwise backups run twice. Details in the [README](../README.md#two-modes).

**"Overdue" needs a signal.** SyncAmp knows about backups it ran itself. To also learn about backups the Syncrify
client ran on its own, turn on the experimental log reader: `syncamp config set logWatch on`.
Until it has seen a successful backup it shows "not seen yet" and does not raise an overdue alarm.

## Alerts to your phone or chat

```bash
syncamp config set webhook https://hooks.slack.com/services/XXX/YYY/ZZZ
```

SyncAmp sends a small JSON message `{"text": "[SyncAmp] Backup \"Photos\" failed: …"}` for failures, overdue backups,
recoveries and "can't reach the server". That format is what Slack incoming webhooks (and Mattermost) accept;
other services may need a tiny adapter. The URL must be `https://`, and it is shown redacted in `syncamp config show`.

## First-run warnings

The programs are not code-signed (a certificate costs money), so operating systems warn once.

- **Windows SmartScreen** ("Windows protected your PC"): click **More info → Run anyway**. Or right-click the file →
  *Properties* → tick **Unblock**. Some antivirus tools quarantine unsigned service programs: add an exception for
  the install folder if that happens.
- **macOS Gatekeeper:** see the macOS steps above (right-click → Open, or remove the quarantine attribute).
- Always compare the download with `SHA256SUMS` from the same release if you are unsure where a file came from.

## Troubleshooting

| You see | Why / what to do |
|---|---|
| **No Syncrify profiles found** | SyncAmp can't see your profile folder. On Windows that is the installer checkbox in [step 2](#windows) (or `syncamp watch "C:\Users\you\.syncrify"`). `syncamp doctor` may name a likely folder. |
| The window says **Not connected** | This account may not read the service's access key. An administrator runs `syncamp grant YOUR_ACCOUNT`. |
| Clicking the icon does nothing, or a message about **WebView2** | Install the [Microsoft WebView2 Runtime](https://go.microsoft.com/fwlink/p/?LinkId=2124703) (built into Windows 11). Details are in `%LOCALAPPDATA%\SyncAmp\app.log`. |
| `doctor`: **Syncrify client not found** | `syncamp config set clientPath "<full path to the client program>"` |
| **Can't reach the Syncrify server** | The `server.url` you set isn't answering. Check the address and port (a fresh Syncrify server listens on plain HTTP port 5800) and try it in a browser. Set it with `syncamp config set server.url https://host:5800`. |
| "Reachable, but doesn't look like Syncrify" | The URL answered but not to Syncrify's build-number request: wrong port, or a proxy path prefix. |
| **Overdue** but you know it ran | SyncAmp didn't see it. Run backups from SyncAmp, or enable the log reader (see above), or raise the window: `syncamp profile stale NAME 72h`. |
| Backups run **twice** | You are in `scheduler` mode while the Syncrify client also schedules. Set `mode monitor`. |
| **Finished with warnings** | Syncrify's own "completed with one or more error" result, or the profile was already locked by another run. Not counted as a failure. |
| `syncamp token` / `status`: *cannot read API token* | Run it as Administrator/root: the token file is deliberately readable by administrators only. |
| Service won't start | Windows: `Get-Content C:\ProgramData\SyncAmp\syncamp.log -Tail 30` · Linux: `journalctl -u SyncAmp -n 50`. A common cause is a mistake in `config.json`: `syncamp config validate` points at it. |
| Config edits ignored | On Windows remember to double the backslashes in JSON, then `syncamp restart`. |

Still stuck? Run `syncamp doctor` and `syncamp status -json` and open an issue with the output (secrets are masked).
