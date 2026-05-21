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
    Remove-Item $ExePath -Force
    Write-OK "Removed: $ExePath"
} else {
    Write-Info "Binary not found at $ExePath - already removed or different install dir."
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

$UserPath = [Environment]::GetEnvironmentVariable("Path", "User")
if ($UserPath -like "*$InstallDir*") {
    # Remove all occurrences, handle trailing/leading semicolons
    $Parts   = $UserPath -split ";" | Where-Object { $_ -ne $InstallDir -and $_ -ne "" }
    $NewPath = $Parts -join ";"
    [Environment]::SetEnvironmentVariable("Path", $NewPath, "User")
    $env:Path = ($env:Path -split ";" | Where-Object { $_ -ne $InstallDir }) -join ";"
    Write-OK "Removed $InstallDir from user PATH"
} else {
    Write-Info "PATH entry not found - nothing to remove."
}

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
$AppDataDir = Join-Path $env:APPDATA "Duster"

if (-not $Silent -and -not $RemoveAppData -and (Test-Path $AppDataDir)) {
    $RemoveData = Read-Host "  Also remove app data and logs from $AppDataDir? [y/N]"
    if ($RemoveData -match "^[yY]") { $RemoveAppData = $true }
}

if ($RemoveAppData -and (Test-Path $AppDataDir)) {
    Remove-Item $AppDataDir -Recurse -Force
    Write-OK "Removed app data: $AppDataDir"
} elseif (Test-Path $AppDataDir) {
    Write-Info "App data kept at: $AppDataDir (contains operation logs)"
}

# == Done ==============================================================
if (-not $Silent) {
    Write-Host ""
    Write-Host "  $([char]0x2713) Duster has been completely removed." -ForegroundColor Green
    Write-Host ""
}
