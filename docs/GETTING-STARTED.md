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

**1. Download** `syncamp-windows-amd64.exe` and `syncamp-tray-windows-amd64.exe` from the
[Releases page](https://github.com/brielshallouf/SyncAmp/releases) (see the *nightly* entry for the newest build).

**2. Put them somewhere permanent** — the service remembers the file's location, so don't leave them in Downloads.
In an **Administrator PowerShell**:

```powershell
New-Item -ItemType Directory "C:\Program Files\SyncAmp" -Force
Move-Item "$env:USERPROFILE\Downloads\syncamp-windows-amd64.exe"      "C:\Program Files\SyncAmp\syncamp.exe"
Move-Item "$env:USERPROFILE\Downloads\syncamp-tray-windows-amd64.exe" "C:\Program Files\SyncAmp\syncamp-tray.exe"
cd "C:\Program Files\SyncAmp"
```

If Windows blocks the files, see [first-run warnings](#first-run-warnings). To check a download, compare
`Get-FileHash .\syncamp.exe -Algorithm SHA256` with the matching line in `SHA256SUMS` (use the *original* file name).

**3. Install and start the service** (still in the Administrator window):

```powershell
.\syncamp.exe install      # registers the "SyncAmp" Windows service, creates the locked-down data folder
.\syncamp.exe doctor       # shows anything that needs fixing: read it
.\syncamp.exe start
.\syncamp.exe status
```

**4. Tell SyncAmp where your Syncrify profiles are.** This is the step people miss.
The service runs as `SYSTEM` by default, which has its *own* home folder, so it will not see profiles that live in
*your* user folder (`C:\Users\<you>\.syncrify`). `status` will say **"No Syncrify profiles found"**. Pick one:

- **Option A — run the service as you (recommended).** Press `Win+R`, run `services.msc`, open **SyncAmp →
  Properties → Log On → This account**, enter your Windows account (it must be an administrator) and password,
  then restart the service. Backups now run with your rights and your profile folder is found automatically.
- **Option B — point SyncAmp at the folder.** Edit `C:\ProgramData\SyncAmp\config.json` in an Administrator
  Notepad (double each backslash):
  ```json
  { "profileDirs": ["C:\\Users\\alice\\.syncrify"] }
  ```
  then `.\syncamp.exe restart`. Only do this on a machine where nobody else can write to that folder: a Syncrify
  profile can run scripts, so a folder other people can write to would let them run code as SYSTEM
  (see [SECURITY.md](../SECURITY.md)).

Not sure where your profiles are? `dir $env:USERPROFILE\.syncrify` or
`Get-ChildItem C:\Users -Recurse -Filter *.syncrify -ErrorAction SilentlyContinue`.

If `doctor` says the client was not found, set it (use the real path to your Syncrify client program):

```powershell
.\syncamp.exe config set clientPath "C:\Program Files\Syncrify Client\SyncrifyClient.exe"
```

**5. Add the tray icon** (in a normal, non-admin window is fine):

```powershell
& "C:\Program Files\SyncAmp\syncamp-tray.exe" autostart on     # start it at every login
& "C:\Program Files\SyncAmp\syncamp-tray.exe"                  # start it now
```

The icon turns **green** (healthy), **blue** (running), **amber** (warning) or **red** (needs attention).
Click it and choose **Open dashboard** to get the popover. The first time, it asks for a token: in your
Administrator window run `.\syncamp.exe token`, then paste the result.

**Uninstall:** `.\syncamp.exe stop`, `.\syncamp.exe uninstall`, `.\syncamp-tray.exe autostart off`, then delete
`C:\Program Files\SyncAmp` and `C:\ProgramData\SyncAmp`.

---

## macOS

On a Mac SyncAmp runs as **you** (a login item), not as root, so there is no `sudo` and your `~/.syncrify`
profiles are found automatically.

**1. Download** the command-line program for your Mac (`syncamp-darwin-arm64` for Apple Silicon, `-amd64` for
Intel) and `SyncAmp-macos.zip` from the [Releases page](https://github.com/brielshallouf/SyncAmp/releases).

**2. Install and start the service** in Terminal:

```bash
chmod +x ~/Downloads/syncamp-darwin-arm64
xattr -d com.apple.quarantine ~/Downloads/syncamp-darwin-arm64 2>/dev/null   # allow the unsigned download
sudo mkdir -p /usr/local/bin && sudo mv ~/Downloads/syncamp-darwin-arm64 /usr/local/bin/syncamp
syncamp install && syncamp doctor && syncamp start && syncamp status
```

**3. Add the menu-bar app:** unzip `SyncAmp-macos.zip`, drag **SyncAmp** into *Applications*, then open it.
macOS will warn that it can't verify the developer: right-click the app → **Open** → **Open**, or run
`xattr -dr com.apple.quarantine /Applications/SyncAmp.app`. Use the **⋯** menu → *Launch at login*.
On macOS 26 it uses Apple's Liquid Glass look; older versions use the system blur.

If you back up protected folders, give the `syncamp` program **Full Disk Access** in
*System Settings → Privacy & Security*.

> The app is built and unit-tested on macOS in CI, but has not been launched on a real desktop by its author yet.
> Please report anything odd.

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
- **Dashboard over SSH:** `ssh -L 5801:127.0.0.1:5801 server`, open <http://127.0.0.1:5801/>, and paste the output of
  `sudo syncamp token`. The service only listens on the loopback address.
- **Monitoring:** `syncamp status` exits **0** (ok), **1** (warning) or **2** (error/unreachable), so cron, Nagios,
  Zabbix or a shell script can use it directly. `syncamp status -json` is machine-readable and
  `syncamp backup -wait NAME` blocks and fails the script if the backup fails.
- **Logs:** `journalctl -u SyncAmp` and `/var/lib/syncamp/syncamp.log`.
- **Desktop tray (optional):** `syncamp-tray-linux-amd64` needs a desktop that supports status-notifier icons
  (GNOME needs the AppIndicator extension).

---

## Using it day to day

| I want to… | Do this |
|---|---|
| See if everything is fine | Look at the tray icon, or `syncamp status` |
| Back up something right now | Press ▶ next to it in the popover, or `syncamp backup NAME` (`-wait` to wait) |
| See what happened | **Activity** in the popover, `syncamp history`, or `syncamp logs NAME` for the last run's output |
| Be warned if a backup silently stops | Expand a backup in the popover → **Alert if overdue**, or `syncamp profile stale NAME 26h` |
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
| `status` says **No Syncrify profiles found** | SyncAmp can't see your profile folder. On Windows this is the SYSTEM-vs-your-account issue in [step 4](#windows). `syncamp doctor` may name a likely folder. |
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
