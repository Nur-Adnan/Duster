<#
.SYNOPSIS
    Duster - Official PowerShell Installer
    https://github.com/Nur-Adnan/Duster

.DESCRIPTION
    Downloads, verifies, and installs the latest Duster release.
    Works without administrator rights (per-user install by default).

.PARAMETER InstallDir
    Directory to install du.exe into.
    Default: $env:LOCALAPPDATA\Duster

.PARAMETER Version
    Specific version to install (e.g. "1.0.1").
    Default: latest release from GitHub.

.PARAMETER AddToPath
    Whether to add the install directory to the user PATH.
    Default: $true

.PARAMETER Silent
    Suppress all progress output.

.PARAMETER Force
    Overwrite existing installation without prompting.

.EXAMPLE
    # One-liner install (run in PowerShell):
    irm https://raw.githubusercontent.com/Nur-Adnan/Duster/main/scripts/install.ps1 | iex

.EXAMPLE
    # Install to custom directory:
    .\install.ps1 -InstallDir "C:\Tools\Duster"

.EXAMPLE
    # Install specific version silently:
    .\install.ps1 -Version "1.0.1" -Silent
#>

[CmdletBinding()]
param(
    [string]  $InstallDir = "$env:LOCALAPPDATA\Duster",
    [string]  $Version    = "",
    [bool]    $AddToPath  = $true,
    [switch]  $Silent,
    [switch]  $Force
)

$ErrorActionPreference = "Stop"
$ProgressPreference    = "SilentlyContinue"   # Speeds up Invoke-WebRequest dramatically

# Guarantee TLS 1.2 is enabled for all secure REST and download queries (critical for GitHub API/CDN on older OS versions)
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12

# == Helpers ==========================================================
# NOTE: No extra braces inside function bodies - that creates a ScriptBlock
# literal which outputs raw code text instead of executing it.
function Write-Step {
    param([string]$Msg)
    if (-not $Silent) { Write-Host "  $Msg" -ForegroundColor Cyan }
}
function Write-OK {
    param([string]$Msg)
    if (-not $Silent) { Write-Host "  $([char]0x2713) $Msg" -ForegroundColor Green }
}
function Write-Warn {
    param([string]$Msg)
    Write-Host "  $([char]0x26A0) $Msg" -ForegroundColor Yellow
}
function Write-Fail {
    param([string]$Msg)
    Write-Host "  $([char]0x2717) $Msg" -ForegroundColor Red
    throw "Duster installation aborted: $Msg"
}

# == Banner ============================================================
if (-not $Silent) {
    Write-Host ""
    Write-Host "  =================================================" -ForegroundColor DarkCyan
    Write-Host "    Duster  -  Windows System Cleaner" -ForegroundColor DarkCyan
    Write-Host "    https://github.com/Nur-Adnan/Duster" -ForegroundColor DarkCyan
    Write-Host "  =================================================" -ForegroundColor DarkCyan
    Write-Host ""
}

# == 1. Detect Architecture ============================================
Write-Step "Detecting system architecture..."

$Arch = $env:PROCESSOR_ARCHITECTURE
if ($Arch -eq "ARM64") {
    $ArchSuffix = "windows-arm64"
    Write-OK "ARM64 detected"
} elseif ($Arch -eq "AMD64" -or $Arch -eq "x86") {
    $ArchSuffix = "windows-amd64"
    Write-OK "x64 detected"
} else {
    Write-Fail "Unsupported architecture: $Arch. Only x64 and ARM64 are supported."
}

# == 2. Resolve Target Version =========================================
Write-Step "Resolving release version..."

$ApiBase = "https://api.github.com/repos/Nur-Adnan/Duster"
$FallbackVersion = "1.0.2"

if ($Version -eq "") {
    try {
        $Release = Invoke-RestMethod "$ApiBase/releases/latest" -Headers @{ "User-Agent" = "DusterInstaller" }
        $Version = $Release.tag_name -replace "^v", ""
        $TagName = $Release.tag_name
    } catch {
        Write-Warn "Could not fetch latest release from GitHub API (might be rate-limited)."
        Write-Step "Falling back to default production version: $FallbackVersion"
        $Version = $FallbackVersion
        $TagName = "v$Version"
    }
} else {
    $TagName = "v$Version"
}

Write-OK "Target version: $Version"

# == 3. Check Existing Installation ===================================
$ExePath = Join-Path $InstallDir "du.exe"

if (Test-Path $ExePath) {
    try {
        $ExistingOutput = & $ExePath --version 2>&1
        $ExistingMatch = [regex]::Match("$ExistingOutput", "\d+\.\d+\.\d+")
        if ($ExistingMatch.Success) {
            $Existing = $ExistingMatch.Value
        } else {
            $Existing = "unknown"
        }
    } catch {
        $Existing = "unknown"
    }

    if ($Existing -eq $Version -and -not $Force) {
        Write-OK "Duster $Version is already installed at $ExePath"
        Write-Host ""
        Write-Host "  Run 'du --help' to get started." -ForegroundColor White
        Write-Host ""
        return
    }
    if ($Existing -ne "unknown" -and -not $Force) {
        Write-Warn "Duster $Existing is already installed. Upgrading to $Version."
    }
}

# == 4. Download Release Archive =======================================
$ArchiveName = "duster-$Version-$ArchSuffix.zip"
Write-Step "Downloading Duster $Version ($ArchiveName)..."

$DownloadBase = "https://github.com/Nur-Adnan/Duster/releases/download/$TagName"
$ArchiveUrl   = "$DownloadBase/$ArchiveName"
$ChecksumUrl  = "$DownloadBase/checksums-sha256.txt"
$TempDir      = Join-Path $env:TEMP "duster-install-$Version"
$TempZip      = Join-Path $TempDir $ArchiveName
$TempChecksum = Join-Path $TempDir "checksums-sha256.txt"

# Create temp directory
$null = New-Item -ItemType Directory -Path $TempDir -Force

try {
    Invoke-WebRequest -Uri $ArchiveUrl -OutFile $TempZip -UseBasicParsing
} catch {
    Write-Fail "Failed to download release archive. Check if version '$Version' exists at:`n  $ArchiveUrl"
}

$ArchiveSize = [math]::Round((Get-Item $TempZip).Length / 1MB, 1)
Write-OK "Downloaded $ArchiveName ($ArchiveSize MB)"

# == 5. Verify SHA-256 Checksum ========================================
Write-Step "Verifying SHA-256 checksum..."

$ChecksumVerified = $false
try {
    Invoke-WebRequest -Uri $ChecksumUrl -OutFile $TempChecksum -UseBasicParsing

    # Parse the expected hash for our archive
    $ChecksumLines = Get-Content $TempChecksum
    $ExpectedLine  = $ChecksumLines | Where-Object { $_ -like "*$ArchiveName*" }

    if ($ExpectedLine) {
        $ExpectedHash = ($ExpectedLine -split '\s+')[0].ToUpper()
        
        # Defensive check for Get-FileHash availability
        if (Get-Command Get-FileHash -ErrorAction SilentlyContinue) {
            $ActualHash = (Get-FileHash -Path $TempZip -Algorithm SHA256).Hash.ToUpper()
        } else {
            # Fallback to .NET SHA256 class (extremely compatible across all PowerShell versions!)
            $Sha256 = [System.Security.Cryptography.SHA256]::Create()
            $Stream = [System.IO.File]::OpenRead($TempZip)
            $Bytes = $Sha256.ComputeHash($Stream)
            $Stream.Close()
            $ActualHash = (($Bytes | ForEach-Object { "{0:X2}" -f $_ }) -join "").ToUpper()
        }

        if ($ActualHash -ne $ExpectedHash) {
            Remove-Item $TempDir -Recurse -Force -ErrorAction SilentlyContinue
            Write-Fail "Checksum mismatch! Archive may be corrupted.`n  Expected: $ExpectedHash`n  Got:      $ActualHash"
        }

        $ChecksumVerified = $true
        Write-OK "Checksum verified: $($ActualHash.Substring(0,16))..."
    } else {
        Write-Warn "Checksum entry not found for $ArchiveName. Skipping verification."
    }
} catch {
    Write-Warn "Could not fetch checksums file. Skipping verification."
}

# == 6. Extract & Install ==============================================
Write-Step "Extracting release files..."

try {
    Expand-Archive -Path $TempZip -DestinationPath $TempDir -Force
} catch {
    Write-Fail "Failed to extract release archive. Ensure Expand-Archive is available."
}

$TempExe = Join-Path $TempDir "du.exe"
if (-not (Test-Path $TempExe)) {
    Write-Fail "Failed to find 'du.exe' inside extracted archive."
}

Write-Step "Installing to $InstallDir..."
$null = New-Item -ItemType Directory -Path $InstallDir -Force

try {
    Copy-Item -Path $TempExe -Destination $ExePath -Force
} catch {
    Write-Fail "Could not write to $InstallDir. Try a different -InstallDir or run as Admin."
}

Write-OK "Installed: $ExePath"

# == 7. Add to PATH ====================================================
if ($AddToPath) {
    Write-Step "Updating PATH..."

    $UserPath = [Environment]::GetEnvironmentVariable("Path", "User")

    if ($UserPath -notlike "*$InstallDir*") {
        if ([string]::IsNullOrEmpty($UserPath)) {
            $NewPath = $InstallDir
        } elseif ($UserPath.EndsWith(";")) {
            $NewPath = "$UserPath$InstallDir"
        } else {
            $NewPath = "$UserPath;$InstallDir"
        }
        [Environment]::SetEnvironmentVariable("Path", $NewPath, "User")

        # Also update the current PowerShell session so du works immediately in PS
        $env:Path = "$env:Path;$InstallDir"

        Write-OK "Added $InstallDir to user PATH"
    } else {
        Write-OK "PATH already contains install directory"
    }
}

# == 8. Cleanup Temp Files =============================================
Remove-Item $TempDir -Recurse -Force -ErrorAction SilentlyContinue

# == 9. Verify Installation ============================================
Write-Step "Verifying installation..."

$VerifyOk = $false
if (Test-Path $ExePath) {
    $FileSize = [math]::Round((Get-Item $ExePath).Length / 1MB, 1)
    if ($FileSize -gt 1) {
        Write-OK "Binary present: $ExePath ($FileSize MB)"
        $VerifyOk = $true
    } else {
        Write-Warn "Binary is unusually small ($FileSize MB). Download may have failed."
    }
} else {
    Write-Warn "Binary not found at $ExePath after install."
}

# Try running the binary (may fail if cross-architecture, which is OK)
if ($VerifyOk) {
    try {
        $VersionOutput = & $ExePath --version 2>&1
        Write-OK "Verified: $VersionOutput"
    } catch {
        Write-OK "Binary installed (could not execute for verification - this is normal during cross-install)"
    }
}

# == Done ==============================================================
if (-not $Silent) {
    Write-Host ""
    Write-Host "  =================================================" -ForegroundColor Green
    Write-Host "    Duster $Version installed successfully!" -ForegroundColor Green
    Write-Host "  =================================================" -ForegroundColor Green
    Write-Host ""
    Write-Host "  Quick Start:" -ForegroundColor White
    Write-Host "    du --help          Show all commands" -ForegroundColor Gray
    Write-Host "    du status          Live system dashboard" -ForegroundColor Gray
    Write-Host "    du clean --dry-run Preview cleanable data" -ForegroundColor Gray
    Write-Host "    du clean           Run deep clean" -ForegroundColor Gray
    Write-Host "    du doctor          System diagnostics" -ForegroundColor Gray
    Write-Host ""
    Write-Host "  IMPORTANT: You must open a NEW terminal window for" -ForegroundColor Yellow
    Write-Host "  the 'du' command to be available in your PATH." -ForegroundColor Yellow
    Write-Host ""
    Write-Host "  Or refresh PATH in this window:" -ForegroundColor DarkGray
    Write-Host "    PowerShell:  `$env:Path = [Environment]::GetEnvironmentVariable('Path','User') + ';' + [Environment]::GetEnvironmentVariable('Path','Machine')" -ForegroundColor DarkGray
    Write-Host "    CMD:         set `"PATH=%PATH%;$InstallDir`"" -ForegroundColor DarkGray
    Write-Host ""
}
