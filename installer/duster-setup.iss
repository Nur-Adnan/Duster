; ╔══════════════════════════════════════════════════════════════════════╗
; ║               Duster — Inno Setup Installer Script                 ║
; ║               Enterprise-Grade Windows Installer                   ║
; ╚══════════════════════════════════════════════════════════════════════╝
;
; Build this installer:
;   1. Install Inno Setup 6.x from https://jrsoftware.org/isinfo.php
;   2. Open this .iss file in Inno Setup Compiler
;   3. Press Ctrl+F9 to compile
;
; Or from command line:
;   iscc.exe installer\duster-setup.iss
;
; Prerequisites:
;   - dist\duster-windows-amd64.exe (or duster-windows-arm64.exe)
;   - assets\duster.ico (application icon)

#define MyAppName "Duster"
#ifndef MyAppVersion
  #define MyAppVersion "1.0.2"
#endif
#define MyAppPublisher "Nur Adnan"
#define MyAppURL "https://github.com/Nur-Adnan/Duster"
#define MyAppExeName "du.exe"
#define MyAppDescription "Windows Deep Cleaner & System Optimizer"
#define MyAppCopyright "Copyright (c) 2026 Nur Adnan"

[Setup]
; Application identity
AppId={{B3E4F1A2-8C7D-4E2F-9A1B-6D3C5E7F8A9B}
AppName={#MyAppName}
AppVersion={#MyAppVersion}
AppVerName={#MyAppName} {#MyAppVersion}
AppPublisher={#MyAppPublisher}
AppPublisherURL={#MyAppURL}
AppSupportURL={#MyAppURL}/issues
AppUpdatesURL={#MyAppURL}/releases
AppCopyright={#MyAppCopyright}

; Version metadata for Windows properties dialog
VersionInfoVersion={#MyAppVersion}.0
VersionInfoCompany={#MyAppPublisher}
VersionInfoDescription={#MyAppDescription}
VersionInfoCopyright={#MyAppCopyright}
VersionInfoProductName={#MyAppName}
VersionInfoProductVersion={#MyAppVersion}.0

; Installation directories
DefaultDirName={autopf}\{#MyAppName}
DefaultGroupName={#MyAppName}

; Security: Default to admin install (Program Files) for WDAC/AppLocker compatibility.
; Executables in user-writable directories (AppData) are blocked by enterprise security policies.
; Users on unrestricted PCs can still choose per-user install via the dialog.
PrivilegesRequired=admin
PrivilegesRequiredOverridesAllowed=dialog

; Output settings
OutputDir=..\dist\installer
OutputBaseFilename=Duster-Setup-{#MyAppVersion}-x64
SetupIconFile=..\assets\duster.ico
UninstallDisplayIcon={app}\{#MyAppExeName}
UninstallDisplayName={#MyAppName} {#MyAppVersion}

; Compression (maximum LZMA2 for smallest installer)
Compression=lzma2/ultra64
SolidCompression=yes
LZMAUseSeparateProcess=yes
LZMANumBlockThreads=4

; UI Configuration
WizardStyle=modern
WizardSizePercent=110,110
DisableProgramGroupPage=yes
DisableWelcomePage=no

; Behavior
AllowNoIcons=yes
ChangesEnvironment=yes
CloseApplications=yes
RestartApplications=no
SetupLogging=yes

; Minimum OS: Windows 10 build 17763 (1809)
MinVersion=10.0.17763

; Architecture
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible

[Languages]
Name: "english"; MessagesFile: "compiler:Default.isl"

[Tasks]
Name: "desktopicon"; Description: "{cm:CreateDesktopIcon}"; GroupDescription: "{cm:AdditionalIcons}"; Flags: unchecked
Name: "addtopath"; Description: "Add Duster to system PATH (recommended)"; GroupDescription: "System Integration:"; Flags: checkedonce

[Files]
; Main executable
Source: "..\dist\duster-windows-amd64.exe"; DestDir: "{app}"; DestName: "{#MyAppExeName}"; Flags: ignoreversion

; Documentation
Source: "..\README.md"; DestDir: "{app}"; Flags: ignoreversion
Source: "..\LICENSE"; DestDir: "{app}"; Flags: ignoreversion

[Icons]
; Start Menu shortcuts
Name: "{group}\{#MyAppName}"; Filename: "{cmd}"; Parameters: "/K ""{app}\{#MyAppExeName}"" --help"; IconFilename: "{app}\{#MyAppExeName}"; Comment: "Launch Duster CLI"
Name: "{group}\{#MyAppName} Status Dashboard"; Filename: "{cmd}"; Parameters: "/K ""{app}\{#MyAppExeName}"" status"; IconFilename: "{app}\{#MyAppExeName}"; Comment: "Open Duster System Status Dashboard"
Name: "{group}\{#MyAppName} Deep Clean"; Filename: "{cmd}"; Parameters: "/K ""{app}\{#MyAppExeName}"" clean --dry-run"; IconFilename: "{app}\{#MyAppExeName}"; Comment: "Preview Duster Deep Clean"
Name: "{group}\Uninstall {#MyAppName}"; Filename: "{uninstallexe}"

; Desktop shortcut (optional task)
Name: "{autodesktop}\{#MyAppName}"; Filename: "{cmd}"; Parameters: "/K ""{app}\{#MyAppExeName}"" --help"; IconFilename: "{app}\{#MyAppExeName}"; Tasks: desktopicon; Comment: "Duster CLI"

[Registry]
; HKA resolves to HKLM for an elevated (admin-mode) install and HKCU for a
; per-user install, so an elevated install never writes per-user state into
; the elevating admin's profile.
; Add application to App Paths for Win+R launching: "duster" or "du"
Root: HKA; Subkey: "Software\Microsoft\Windows\CurrentVersion\App Paths\du.exe"; ValueType: string; ValueName: ""; ValueData: "{app}\{#MyAppExeName}"; Flags: uninsdeletekey
Root: HKA; Subkey: "Software\Microsoft\Windows\CurrentVersion\App Paths\du.exe"; ValueType: string; ValueName: "Path"; ValueData: "{app}"; Flags: uninsdeletekey

; Store version info for auto-update queries
Root: HKA; Subkey: "Software\{#MyAppPublisher}\{#MyAppName}"; ValueType: string; ValueName: "Version"; ValueData: "{#MyAppVersion}"; Flags: uninsdeletekey
Root: HKA; Subkey: "Software\{#MyAppPublisher}\{#MyAppName}"; ValueType: string; ValueName: "InstallPath"; ValueData: "{app}"; Flags: uninsdeletekey

[Run]
; Post-install: show help output to verify installation
Filename: "{cmd}"; Parameters: "/K echo. & echo   Duster v{#MyAppVersion} installed successfully! & echo. & echo   Type 'du --help' to get started. & echo. & ""{app}\{#MyAppExeName}"" --version & echo. & pause"; Description: "Launch Duster CLI"; Flags: nowait postinstall skipifsilent shellexec

[UninstallDelete]
; Clean up application data on uninstall
Type: filesandordirs; Name: "{localappdata}\Duster"

[Code]
// ─────────────────────────────────────────────
// Pascal Script: PATH modification on install/uninstall
// ─────────────────────────────────────────────

const
  UserEnvKey   = 'Environment';
  SystemEnvKey = 'SYSTEM\CurrentControlSet\Control\Session Manager\Environment';

// An elevated install must edit the machine PATH: writing HKCU\Environment
// while elevated would modify the elevating ADMIN's profile, not the
// logged-on user's, leaving the actual user without `du` on PATH.
function EnvRootKey: Integer;
begin
  if IsAdminInstallMode then
    Result := HKEY_LOCAL_MACHINE
  else
    Result := HKEY_CURRENT_USER;
end;

function EnvSubKey: string;
begin
  if IsAdminInstallMode then
    Result := SystemEnvKey
  else
    Result := UserEnvKey;
end;

// Normalize a PATH entry for comparison: uppercase, no trailing backslash.
function NormalizePathEntry(const Entry: string): string;
begin
  Result := Uppercase(Trim(Entry));
  if (Result <> '') and (Result[Length(Result)] = '\') then
    Result := Copy(Result, 1, Length(Result) - 1);
end;

// True if Dir exists in Path as a COMPLETE semicolon-delimited entry.
// A plain substring test would match inside unrelated entries
// (e.g. "...\DusterX") and corrupt them on removal.
function PathEntryExists(const Path, Dir: string): Boolean;
var
  Rest, Entry, NormDir: string;
  P: Integer;
begin
  Result := False;
  NormDir := NormalizePathEntry(Dir);
  Rest := Path;
  while Rest <> '' do
  begin
    P := Pos(';', Rest);
    if P > 0 then
    begin
      Entry := Copy(Rest, 1, P - 1);
      Rest := Copy(Rest, P + 1, Length(Rest));
    end
    else
    begin
      Entry := Rest;
      Rest := '';
    end;
    if NormalizePathEntry(Entry) = NormDir then
    begin
      Result := True;
      Exit;
    end;
  end;
end;

procedure AddToPath(const Dir: string);
var
  Path: string;
begin
  if not RegQueryStringValue(EnvRootKey, EnvSubKey, 'Path', Path) then
    Path := '';

  if PathEntryExists(Path, Dir) then
    Exit;

  // Append with semicolon separator
  if (Path <> '') and (Path[Length(Path)] <> ';') then
    Path := Path + ';';
  Path := Path + Dir;

  // Write REG_EXPAND_SZ: the Path value is conventionally expandable, and
  // RegWriteStringValue would silently convert it to REG_SZ, breaking any
  // pre-existing %VAR% entries the user (or system) relies on.
  RegWriteExpandStringValue(EnvRootKey, EnvSubKey, 'Path', Path);

  // Notify Windows of the environment change
  // SendMessage(HWND_BROADCAST, WM_SETTINGCHANGE ...) is handled by Inno automatically
end;

procedure RemoveFromPath(const Dir: string);
var
  Path, Rest, Entry, NewPath, NormDir: string;
  P: Integer;
begin
  if not RegQueryStringValue(EnvRootKey, EnvSubKey, 'Path', Path) then
    Exit;

  // Rebuild the PATH keeping every entry except exact matches for Dir.
  NormDir := NormalizePathEntry(Dir);
  NewPath := '';
  Rest := Path;
  while Rest <> '' do
  begin
    P := Pos(';', Rest);
    if P > 0 then
    begin
      Entry := Copy(Rest, 1, P - 1);
      Rest := Copy(Rest, P + 1, Length(Rest));
    end
    else
    begin
      Entry := Rest;
      Rest := '';
    end;

    if (Entry <> '') and (NormalizePathEntry(Entry) <> NormDir) then
    begin
      if NewPath <> '' then
        NewPath := NewPath + ';';
      NewPath := NewPath + Entry;
    end;
  end;

  if NewPath <> Path then
    RegWriteExpandStringValue(EnvRootKey, EnvSubKey, 'Path', NewPath);
end;

procedure CurStepChanged(CurStep: TSetupStep);
begin
  if CurStep = ssPostInstall then
  begin
    if IsTaskSelected('addtopath') then
      AddToPath(ExpandConstant('{app}'));
  end;
end;

procedure CurUninstallStepChanged(CurUninstallStep: TUninstallStep);
begin
  if CurUninstallStep = usPostUninstall then
    RemoveFromPath(ExpandConstant('{app}'));
end;

// Display estimated disk space requirement
function InitializeSetup: Boolean;
begin
  Result := True;
end;
