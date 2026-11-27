# Runs on a real Windows machine in CI: installs SyncAmp with the real installer, checks the
# service and the app's own window against a live profile, then uninstalls and checks cleanup.
$ErrorActionPreference = 'Stop'
$app = "$env:ProgramFiles\SyncAmp"
function Section($t) { Write-Host "`n=== $t" }

Section 'install (silent)'
$p = Start-Process dist\SyncAmp-Setup.exe -Wait -PassThru -ArgumentList '/VERYSILENT','/SUPPRESSMSGBOXES','/NORESTART','/LOG=install.log'
Get-Content install.log -Tail 25
if ($p.ExitCode -ne 0) { throw "installer exited $($p.ExitCode)" }
Get-ChildItem $app | Format-Table Name, Length

Section 'service'
$svc = Get-Service SyncAmp
"service: $($svc.Status)"
if ($svc.Status -ne 'Running') { Get-Content "$env:ProgramData\SyncAmp\syncamp.log" -ErrorAction SilentlyContinue; throw 'service is not running' }
icacls "$env:ProgramData\SyncAmp"
icacls "$env:ProgramData\SyncAmp\api.token"
Get-Content "$env:ProgramData\SyncAmp\grants"

Section 'point it at a real profile and a client, restart'
New-Item -ItemType Directory "$env:USERPROFILE\.syncrify" -Force | Out-Null
Set-Content "$env:USERPROFILE\.syncrify\Demo.syncrify" '<p><schedulerDays>127</schedulerDays></p>'
& "$app\syncamp.exe" watch "$env:USERPROFILE\.syncrify"
& "$app\syncamp.exe" config set clientPath $env:ComSpec
& "$app\syncamp.exe" restart
Start-Sleep -Seconds 4
& "$app\syncamp.exe" doctor
"doctor exit: $LASTEXITCODE (0/1 acceptable, 2 means a FAIL finding)"
& "$app\syncamp.exe" status
"status exit: $LASTEXITCODE"
& "$app\syncamp.exe" profiles

Section 'the app window: real WebView2 window, live data through the bridge, no sign-in'
$out = Join-Path $env:RUNNER_TEMP 'selftest.json'
$a = Start-Process "$app\syncamp-app.exe" -ArgumentList 'selftest', $out -Wait -PassThru
"selftest exit code: $($a.ExitCode)  (0 ok, 1 timeout, 2 not signed in, 3 window could not start)"
if (Test-Path $out) { Get-Content $out }
if ($a.ExitCode -ne 0) { throw "app self-test failed ($($a.ExitCode))" }
$r = Get-Content $out | ConvertFrom-Json
if ($r.report.profiles -lt 1) { throw "the window rendered but saw no profiles: $($r.report | ConvertTo-Json -Compress)" }
if (-not $r.tokenReadable) { throw 'the app account could not read the access key' }

Section 'uninstall (silent)'
& "$app\unins000.exe" /VERYSILENT /SUPPRESSMSGBOXES /NORESTART /LOG=uninstall.log
$deadline = (Get-Date).AddSeconds(90)
while ((Test-Path $app) -and (Get-Date) -lt $deadline) { Start-Sleep -Seconds 2 }
Get-Content uninstall.log -Tail 15 -ErrorAction SilentlyContinue
if (Get-Service SyncAmp -ErrorAction SilentlyContinue) { throw 'service still registered after uninstall' }
if (Test-Path "$app\syncamp.exe") { throw 'files left behind after uninstall' }
"uninstalled cleanly"
