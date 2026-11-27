; SyncAmp for Windows: ONE installer for the whole app.
;   - installs the background service (syncamp.exe) and the desktop app (syncamp-app.exe)
;   - registers + starts the Windows service, starts the app at every login, adds a Start-menu entry
;   - the account that runs the installer is granted access, so the app opens with no sign-in
; Build:  iscc /DAppVersion=1.2.3 installer\windows\SyncAmp.iss   (expects ..\..\dist\ from `make dist`)

#ifndef AppVersion
  #define AppVersion "0.0.0-dev"
#endif
#define ServiceExe "syncamp.exe"
#define AppExe "syncamp-app.exe"
#define WebView2Guid "{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}"

[Setup]
AppId={{B7F2C3A1-5D2E-4C1B-9A71-3B6C0F5A9E21}
AppName=SyncAmp
AppVersion={#AppVersion}
AppPublisher=SyncAmp contributors
AppPublisherURL=https://github.com/brielshallouf/SyncAmp
AppSupportURL=https://github.com/brielshallouf/SyncAmp/issues
DefaultDirName={autopf}\SyncAmp
DisableProgramGroupPage=yes
DisableDirPage=auto
PrivilegesRequired=admin
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
OutputDir=..\..\dist
OutputBaseFilename=SyncAmp-Setup
Compression=lzma2
SolidCompression=yes
WizardStyle=modern
UninstallDisplayName=SyncAmp
UninstallDisplayIcon={app}\{#AppExe}

[Files]
Source: "..\..\dist\syncamp-windows-amd64.exe";     DestDir: "{app}"; DestName: "{#ServiceExe}"; Flags: ignoreversion
Source: "..\..\dist\syncamp-app-windows-amd64.exe"; DestDir: "{app}"; DestName: "{#AppExe}";     Flags: ignoreversion
Source: "..\..\LICENSE";     DestDir: "{app}"; Flags: ignoreversion
Source: "..\..\README.md";   DestDir: "{app}"; Flags: ignoreversion
Source: "..\..\SECURITY.md"; DestDir: "{app}"; Flags: ignoreversion

[Icons]
Name: "{autoprograms}\SyncAmp"; Filename: "{app}\{#AppExe}"

[Run]
; The service steps run from [Code] below (so a failure is reported); these two run as the person
; who is installing (not the elevated installer), because the login entry and the window belong to them.
Filename: "{app}\{#AppExe}"; Parameters: "autostart on"; Flags: runhidden waituntilterminated runasoriginaluser
Filename: "{app}\{#AppExe}"; Description: "Open SyncAmp now"; Flags: nowait postinstall skipifsilent runasoriginaluser

[Code]
function RunTool(const Exe, Params: String): Integer;
var rc: Integer;
begin
  if not Exec(Exe, Params, '', SW_HIDE, ewWaitUntilTerminated, rc) then rc := -1;
  Log(Exe + ' ' + Params + ' -> ' + IntToStr(rc));
  Result := rc;
end;

function Svc(const Params: String): Integer;
begin
  Result := RunTool(ExpandConstant('{app}\{#ServiceExe}'), Params);
end;

function WebView2Installed(): Boolean;
var v: String;
begin
  Result :=
    (RegQueryStringValue(HKLM32, 'SOFTWARE\Microsoft\EdgeUpdate\Clients\{#WebView2Guid}', 'pv', v) and (v <> '') and (v <> '0.0.0.0')) or
    (RegQueryStringValue(HKCU,   'Software\Microsoft\EdgeUpdate\Clients\{#WebView2Guid}', 'pv', v) and (v <> '') and (v <> '0.0.0.0'));
end;

function InitializeSetup(): Boolean;
begin
  Result := True;
  if not WebView2Installed() then
    Result := MsgBox('SyncAmp''s window uses the Microsoft WebView2 Runtime, which is built into Windows 11 and ' +
      'recent Windows 10 but was not found on this PC.' + #13#10#13#10 +
      'You can continue: the background service, the tray icon and the command line work without it. ' +
      'To get the window, install the runtime from https://go.microsoft.com/fwlink/p/?LinkId=2124703' + #13#10#13#10 +
      'Continue installing SyncAmp?', mbConfirmation, MB_YESNO) = IDYES;
end;

function PrepareToInstall(var NeedsRestart: Boolean): String;
begin
  Result := '';
  { Upgrade: release the files that are about to be replaced. }
  if FileExists(ExpandConstant('{app}\{#ServiceExe}')) then Svc('stop');
  RunTool(ExpandConstant('{sys}\taskkill.exe'), '/F /IM {#AppExe}');
end;

procedure CurStepChanged(CurStep: TSetupStep);
begin
  if CurStep = ssPostInstall then
  begin
    if Svc('install') <> 0 then
      MsgBox('SyncAmp could not set up its background service. Open an Administrator terminal and run:' + #13#10 +
        '"' + ExpandConstant('{app}\{#ServiceExe}') + '" doctor', mbError, MB_OK)
    else if Svc('start') <> 0 then
      MsgBox('SyncAmp installed but its background service did not start. Run:' + #13#10 +
        '"' + ExpandConstant('{app}\{#ServiceExe}') + '" doctor', mbError, MB_OK);
  end;
end;

procedure CurUninstallStepChanged(CurUninstallStep: TUninstallStep);
begin
  if CurUninstallStep = usUninstall then
  begin
    RunTool(ExpandConstant('{sys}\taskkill.exe'), '/F /IM {#AppExe}');
    Svc('stop');
    Svc('uninstall');
    RegDeleteValue(HKCU, 'Software\Microsoft\Windows\CurrentVersion\Run', 'SyncAmp');
  end
  else if CurUninstallStep = usPostUninstall then
  begin
    { Settings, the access key and backup history live in ProgramData. Keep them unless asked
      (a silent uninstall keeps them: the default answer is No). }
    if MsgBox('Also delete SyncAmp''s settings and history?', mbConfirmation, MB_YESNO or MB_DEFBUTTON2) = IDYES then
      DelTree(ExpandConstant('{commonappdata}\SyncAmp'), True, True, True);
  end;
end;
