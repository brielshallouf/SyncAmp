# What the Syncrify Server installer tells us

Source: static inspection (no execution) of a Syncrify **Server** `setup.exe` (Inno Setup 5.5 package,
unpacked with `innoextract`). Only configuration files, the server's request table, UI resource strings and
class-file string constants were read; vendor code was not decompiled or copied. Facts are marked
**verified** (read directly from the installer) or **unknown** (could not be determined).

## Architecture — verified
- Java application with its **own bundled JRE** (nothing to install separately), an embedded Tomcat-style web
  server (`htdocs/conf/server.xml`, JSP admin console, GWT file explorer), launched via a Windows service
  named `Syncrify` (`net start Syncrify`). One 32 MB `SyncrifyServer.jar` contains server **and** client
  (`com.synametrics.syncrify.client`, Swing + JavaFX UIs) plus BouncyCastle, Derby (optional RDBMS, off by
  default), Google auth/cloud libraries, jsoup, acme4j (Let's Encrypt).
- Shipped `AppConfig.xml`: HTTP on **port 5800** (`httpPort2`), `httpPortSSL=-1` (HTTPS **not configured out of
  the box**), SMTP for reports. `StaticConfig.xml`: `sslEnabled=true` flag, Derby disabled.
  → A fresh server speaks plain HTTP on 5800 until an admin sets up TLS, so **use an `https://` server URL
  and verify it** (`syncamp doctor` warns on `http://`).
- The server supports cloud targets (AWS/Azure/GCS/CIFS), Exchange/SQL Server/MySQL/bare-metal plugins,
  ransomware handling, a watchdog, user groups, public links, 2FA.

## HTTP operation table (`config/WorkerMappings.xml`) — verified
99 operations, each `?operation=<name>` → handler + access level (0 = no web session needed, 1 user, 2 admin…).
Client-facing ones: `version` (protocol version), `plist` (list profiles), `fpx` (fetch profile XML),
`uppr` (upload profile), `delProfile`, `fetchfl`/`fetchpl` (file/project lists), `up`/`dnwa` (upload/download),
`fsplit` (file splitter, the rsync block logic), `over` (restore versions), `nri` (**next-run info**: the server
records each profile's last run end time and active-backup start time), `cp` (client pinger → "Active clients"
in the admin console), `rw` (ransomware), `wd` (watchdog), `dfoc` (default folders on client).
Request parameters and auth details are **unknown** (the relevant classes are obfuscated).

## Profile & schedule — partly verified
- The server's "configure remote profile" page exposes these profile settings: schedule type (**daily** /
  **interval in minutes** / **disabled**), weekdays + **hour and minute**, direction (client→server, two-way,
  server→client), backup type (**Delta**, Simple Copy, Smart Backup), compress, versioning, "backup locked
  files" (Windows VSS), folder cache, delete-after-backup, skip newer, email report (and "only on error").
  Profiles can also be **fetched from / stored on the server**, so a machine's profile is not necessarily a
  local file.
- The client has a **pre/post-backup script** dialog, so a profile can cause the client to run commands.
- **Unknown:** exact XML element names inside a local `.syncrify` file (other than `schedulerDays` from the
  vendor's docs) and the exact console flags. Please provide a (redacted) sample profile and `--help`/run output.

## The client schedules itself — verified (important)
The client contains `WindowsTaskScheduler`, `LinuxScheduler`, `MacOSXScheduler`, a `BackupSchedulerFactory`
and `migrateSchedulesToService…` methods: current clients run schedules inside their own background service
(or the OS scheduler). If SyncAmp also schedules the same profile, runs collide. The client guards profiles with
a lock ("Unable to lock profile…" / "A previous backup run has locked the current profile"), so a second run is
safe but noisy. SyncAmp therefore (a) classifies the lock messages as **warnings**, not failures, (b) has
`"scheduling": "off"` to only monitor/run on demand, and (c) notes this in `syncamp doctor`.

## Real client messages — verified (used for `failurePatterns` / `warnPatterns`)
Outcomes: "Backup completed in {0}", "Backup started on {0} **and completed with one or more error**"
(Success-with-errors), "Backup started on {0} **but encountered one or more errors**" (failure), "Backup failed
to run for profile: {0}", "Backup aborted upon user's request", "Total errors: {0}".
Errors: "Unable to connect to the server…", "Authentication failed…", "Unable to backup {0}. Error: {1}",
"Unable to read backup profile", "Syncrify server ran out of client licenses", "Internal server error…",
"Unable to lock profile…". Whether the console client prints these to stdout or only to its log file is
**unknown**; reading the client's log folder may be a better verification source.

## Observations about the vendor software (worth checking on your server)
- Auto-update URLs in `StaticConfig.xml` are **plain `http://`** and point to Apache **Ant build files**
  (`patchOne.xml`/`patchTwo.xml`). Ant files can run commands, so an update fetched without TLS or signature
  verification would be a code-execution risk for anyone able to tamper with that traffic. We did not test
  whether the application verifies them.
- The client's file-encryption password is limited to **16 characters** ("cannot be longer than 16
  characters"), which hints at a short fixed-size key; prefer long keys elsewhere (e.g., full-disk encryption)
  if the data is sensitive.
- Default HTTP-only listener (above).

## Ideas this unlocks for SyncAmp (not built yet)
1. **End-to-end verification** via the server's `nri` last-run record, not just local exit codes.
2. Read the client's own log directory as an additional signal.
3. Show Syncrify's **next scheduled run** and per-profile last result in the popover.
4. Honour the profile's own **time of day / interval** instead of SyncAmp's `defaultTimes` (needs the XML names).
