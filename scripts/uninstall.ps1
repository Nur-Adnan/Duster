<#
.SYNOPSIS
    Duster - Official Uninstaller
    https://github.com/Nur-Adnan/Duster

.DESCRIPTION
    Removes Duster from the system:
    - Deletes the du.exe binary
    - Removes the install directory (if empty after removal)
    - Removes the install directory from user PATH
    - Cleans up registry keys written during installation
    - Removes application data (optional)

.PARAMETER InstallDir
    Directory where Duster was installed.
    Default: $env:LOCALAPPDATA\Duster

.PARAMETER RemoveAppData
    Also remove %APPDATA%\Duster (operation logs, config).
    Default: $false (prompted interactively unless -Silent)

.PARAMETER Silent
    Suppress prompts and output. Implies -RemoveAppData=$false.

.EXAMPLE
    # Interactive uninstall:
    .\uninstall.ps1

.EXAMPLE
    # Silent full removal including logs:
    .\uninstall.ps1 -Silent -RemoveAppData
#>

[CmdletBinding()]
param(
    [string] $InstallDir     = "$env:LOCALAPPDATA\Duster",
    [switch] $RemoveAppData,
    [switch] $Silent
)

$ErrorActionPreference = "Stop"

function Write-Step { param([string]$M) if (-not $Silent) { Write-Host "  $M" -ForegroundColor Cyan  } }
function Write-OK   { param([string]$M) if (-not $Silent) { Write-Host "  $([char]0x2713) $M" -ForegroundColor Green } }
function Write-Info { param([string]$M) if (-not $Silent) { Write-Host "  -> $M" -ForegroundColor Gray  } }

if (-not $Silent) {
    Write-Host ""
    Write-Host "  =================================================" -ForegroundColor DarkRed
    Write-Host "             Duster - Uninstaller" -ForegroundColor DarkRed
    Write-Host "  =================================================" -ForegroundColor DarkRed
    Write-Host ""
}

# == 1. Confirm ========================================================
if (-not $Silent) {
    $Confirm = Read-Host "  Remove Duster from $InstallDir? [y/N]"
    if ($Confirm -notmatch "^[yY]") {
        Write-Host "  Uninstall cancelled." -ForegroundColor Yellow
        return
    }
}

# == 2. Remove Binary ==================================================
Write-Step "Removing binary..."

$ExePath = Join-Path $InstallDir "du.exe"
if (Test-Path $ExePath) {
    # Delete the scheduled clean before the binary is gone: du.exe is what
    # knows how to find and remove its own Task Scheduler task. Best-effort -
    # a task left behind only fails to start, so errors here are ignored.
    $PrevErrorActionPreference = $ErrorActionPreference
    $ErrorActionPreference = "Continue"
    try {
        & $ExePath schedule off 2>&1 | Out-Null
    } catch {
    } finally {
        $ErrorActionPreference = $PrevErrorActionPreference
    }

    Remove-Item $ExePath -Force
    Write-OK "Removed: $ExePath"
} else {
    Write-Info "Binary not found at $ExePath - already removed or different install dir."
}

$LauncherPath = Join-Path $InstallDir "duw.exe"
if (Test-Path -LiteralPath $LauncherPath) {
    Remove-Item -LiteralPath $LauncherPath -Force -ErrorAction SilentlyContinue
    Write-OK "Removed: $LauncherPath"
}

# Remove install directory if now empty
if ((Test-Path $InstallDir) -and ((Get-ChildItem $InstallDir | Measure-Object).Count -eq 0)) {
    Remove-Item $InstallDir -Force
    Write-OK "Removed empty directory: $InstallDir"
} elseif (Test-Path $InstallDir) {
    Write-Info "Install directory not empty, keeping: $InstallDir"
}

# == 3. Remove from PATH ===============================================
Write-Step "Cleaning PATH..."

# Exact entry match (case-insensitive, trailing backslash ignored): a substring
# test would also strip or misreport entries like C:\Tools\Duster-old.
# PATH is read and written as stored (unexpanded, REG_EXPAND_SZ):
# [Environment]::Get/SetEnvironmentVariable would expand every %VAR% entry and
# rewrite the value as REG_SZ, freezing the user's other entries.
$Target   = $InstallDir.Trim().TrimEnd('\')
$EnvKey   = [Microsoft.Win32.Registry]::CurrentUser.CreateSubKey('Environment')
$UserPath = [string]$EnvKey.GetValue('Path', '', [Microsoft.Win32.RegistryValueOptions]::DoNotExpandEnvironmentNames)
$Parts    = @($UserPath -split ";" | Where-Object { $_ -ne "" })
$Keep     = @($Parts | Where-Object { [Environment]::ExpandEnvironmentVariables($_).Trim().TrimEnd('\') -ne $Target })
if ($Keep.Count -lt $Parts.Count) {
    $EnvKey.SetValue('Path', ($Keep -join ";"), [Microsoft.Win32.RegistryValueKind]::ExpandString)
    # A registry write alone doesn't reach Explorer. Setting and clearing a
    # throwaway variable broadcasts WM_SETTINGCHANGE, so new terminals see PATH.
    [Environment]::SetEnvironmentVariable('DusterPathRefresh', '1', 'User')
    [Environment]::SetEnvironmentVariable('DusterPathRefresh', $null, 'User')
    $env:Path = ($env:Path -split ";" | Where-Object { $_.Trim().TrimEnd('\') -ne $Target }) -join ";"
    Write-OK "Removed $InstallDir from user PATH"
} else {
    Write-Info "PATH entry not found - nothing to remove."
}
$EnvKey.Close()

# == 4. Clean Registry Keys ============================================
Write-Step "Cleaning registry entries..."

$RegPaths = @(
    "HKCU:\Software\Microsoft\Windows\CurrentVersion\App Paths\du.exe",
    "HKCU:\Software\Nur Adnan\Duster"
)

foreach ($RegPath in $RegPaths) {
    if (Test-Path $RegPath) {
        Remove-Item $RegPath -Recurse -Force
        Write-OK "Removed registry key: $RegPath"
    }
}

# == 5. Remove App Data (Optional) =====================================
# Duster writes its operation logs to %LOCALAPPDATA%\Duster; %APPDATA%\Duster
# is checked too in case older builds left anything there.
$AppDataDirs = @(
    (Join-Path $env:LOCALAPPDATA "Duster"),
    (Join-Path $env:APPDATA "Duster")
) | Where-Object { $_ -and (Test-Path $_) }

if (-not $Silent -and -not $RemoveAppData -and $AppDataDirs.Count -gt 0) {
    $RemoveData = Read-Host "  Also remove app data and logs from $($AppDataDirs -join ', ')? [y/N]"
    if ($RemoveData -match "^[yY]") { $RemoveAppData = $true }
}

foreach ($AppDataDir in $AppDataDirs) {
    if ($RemoveAppData) {
        Remove-Item $AppDataDir -Recurse -Force
        Write-OK "Removed app data: $AppDataDir"
    } else {
        Write-Info "App data kept at: $AppDataDir (contains operation logs)"
    }
}

# == Done ==============================================================
if (-not $Silent) {
    Write-Host ""
    Write-Host "  $([char]0x2713) Duster has been completely removed." -ForegroundColor Green
    Write-Host ""
}
