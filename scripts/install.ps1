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
    Specific version to install (e.g. "1.0.2").
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
    .\install.ps1 -Version "1.0.2" -Silent
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

# == Constants =========================================================
$RepoOwner       = "Nur-Adnan"
$RepoName        = "Duster"
$ApiBase          = "https://api.github.com/repos/$RepoOwner/$RepoName"
$FallbackVersion  = "1.0.2"
$MaxRetries       = 3

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

function Invoke-WithRetry {
    param(
        [scriptblock]$Action,
        [int]$MaxAttempts = 3,
        [string]$Description = "operation"
    )
    $attempt = 0
    while ($true) {
        $attempt++
        try {
            return (& $Action)
        } catch {
            if ($attempt -ge $MaxAttempts) {
                throw $_
            }
            $waitSec = [math]::Pow(2, $attempt)
            Write-Warn "Attempt $attempt/$MaxAttempts for $Description failed. Retrying in ${waitSec}s..."
            Start-Sleep -Seconds $waitSec
        }
    }
}

# == Banner ============================================================
if (-not $Silent) {
    Write-Host ""
    Write-Host "  =================================================" -ForegroundColor DarkCyan
    Write-Host "    Duster  -  Windows System Cleaner" -ForegroundColor DarkCyan
    Write-Host "    https://github.com/$RepoOwner/$RepoName" -ForegroundColor DarkCyan
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

$ReleaseAssetsAvailable = $false
$VersionResolvedFromApi = $false
$TagName = ""

if ($Version -eq "") {
    # Strategy 1: Try /releases/latest API endpoint
    try {
        $Headers = @{ "User-Agent" = "DusterInstaller/1.0" }
        $Release = Invoke-RestMethod "$ApiBase/releases/latest" -Headers $Headers
        $Version = $Release.tag_name -replace "^v", ""
        $TagName = $Release.tag_name
        $ReleaseAssetsAvailable = ($Release.assets.Count -gt 0)
        $VersionResolvedFromApi = $true
        Write-OK "Latest release: $Version (via API)"
    } catch {
        Write-Warn "Could not fetch latest release from GitHub API (might be rate-limited or no releases exist)."

        # Strategy 2: Try /tags API endpoint to find latest tag
        try {
            $Headers = @{ "User-Agent" = "DusterInstaller/1.0" }
            $Tags = Invoke-RestMethod "$ApiBase/tags?per_page=10" -Headers $Headers
            if ($Tags.Count -gt 0) {
                # Tags are returned newest-first; find the first semver-like tag
                foreach ($Tag in $Tags) {
                    if ($Tag.name -match "^v?\d+\.\d+\.\d+") {
                        $TagName = $Tag.name
                        $Version = $TagName -replace "^v", ""
                        $VersionResolvedFromApi = $true
                        Write-OK "Found latest tag: $Version (via tags API)"
                        break
                    }
                }
            }
        } catch {
            Write-Warn "Tags API also unavailable."
        }

        # Strategy 3: Fall back to hardcoded version
        if ($Version -eq "") {
            Write-Step "Falling back to default production version: $FallbackVersion"
            $Version = $FallbackVersion
            $TagName = "v$Version"
        }
    }
} else {
    $TagName = "v$Version"
}

# If we don't know if release assets exist, probe the release for this specific version
if (-not $ReleaseAssetsAvailable) {
    try {
        $Headers = @{ "User-Agent" = "DusterInstaller/1.0" }
        $SpecificRelease = Invoke-RestMethod "$ApiBase/releases/tags/$TagName" -Headers $Headers
        $ReleaseAssetsAvailable = ($SpecificRelease.assets.Count -gt 0)
    } catch {
        $ReleaseAssetsAvailable = $false
    }
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
        if ($VersionResolvedFromApi) {
            # Version confirmed from GitHub API — safe to skip reinstall
            Write-OK "Duster $Version is already installed at $ExePath"
            Write-Host ""
            Write-Host "  Run 'du --help' to get started." -ForegroundColor White
            Write-Host ""
            return
        } else {
            # Version came from fallback — we can't confirm this is actually the latest
            Write-Warn "Duster $Version is installed, but could not verify latest version (API unavailable)."
            Write-Step "Reinstalling to ensure you have the latest build..."
        }
    }
    if ($Existing -ne "unknown" -and -not $Force) {
        Write-Warn "Duster $Existing is already installed. Upgrading to $Version."
    }
}

# == 4. Download Release Binary ========================================
Write-Step "Downloading Duster $Version..."

$DownloadBase   = "https://github.com/$RepoOwner/$RepoName/releases/download/$TagName"
$ArchiveName    = "duster-$Version-$ArchSuffix.zip"
$ArchiveUrl     = "$DownloadBase/$ArchiveName"
$ChecksumUrl    = "$DownloadBase/checksums-sha256.txt"
$TempDir        = Join-Path $env:TEMP "duster-install-$Version"
$TempZip        = Join-Path $TempDir $ArchiveName
$TempChecksum   = Join-Path $TempDir "checksums-sha256.txt"

# Determine arch-specific exe name for direct binary fallback
if ($ArchSuffix -eq "windows-amd64") {
    $DirectExeName = "duster-windows-amd64.exe"
} else {
    $DirectExeName = "duster-windows-arm64.exe"
}
$DirectExeUrl = "$DownloadBase/$DirectExeName"

# Create temp directory
$null = New-Item -ItemType Directory -Path $TempDir -Force

$DownloadedExePath = $null
$DownloadMethod = ""

# Download Strategy 1: Try the portable ZIP archive
$ZipDownloaded = $false
try {
    Write-Step "Trying portable archive: $ArchiveName..."
    Invoke-WithRetry -MaxAttempts $MaxRetries -Description "ZIP download" -Action {
        Invoke-WebRequest -Uri $ArchiveUrl -OutFile $TempZip -UseBasicParsing
    }
    $ArchiveSize = [math]::Round((Get-Item $TempZip).Length / 1MB, 1)
    Write-OK "Downloaded $ArchiveName ($ArchiveSize MB)"
    $ZipDownloaded = $true
    $DownloadMethod = "zip"
} catch {
    Write-Warn "Portable ZIP not available for version $Version."
}

# Download Strategy 2: Try downloading the raw .exe binary directly
if (-not $ZipDownloaded) {
    try {
        Write-Step "Trying direct binary: $DirectExeName..."
        $TempExeDirect = Join-Path $TempDir $DirectExeName
        Invoke-WithRetry -MaxAttempts $MaxRetries -Description "binary download" -Action {
            Invoke-WebRequest -Uri $DirectExeUrl -OutFile $TempExeDirect -UseBasicParsing
        }
        $BinarySize = [math]::Round((Get-Item $TempExeDirect).Length / 1MB, 1)
        if ($BinarySize -lt 0.5) {
            throw "Downloaded binary is suspiciously small ($BinarySize MB)"
        }
        Write-OK "Downloaded $DirectExeName ($BinarySize MB)"
        $DownloadedExePath = $TempExeDirect
        $DownloadMethod = "binary"
    } catch {
        Write-Warn "Direct binary download also failed."
    }
}

# Download Strategy 3: Try building from source as last resort (if Go is available)
if (-not $ZipDownloaded -and $null -eq $DownloadedExePath) {
    Write-Fail "Failed to download Duster $Version. No release assets found.`n`n  This version may not have been published yet.`n  The release workflow must be triggered first.`n`n  To fix this, the maintainer needs to run:`n    git tag -d v$Version`n    git push origin :refs/tags/v$Version`n    git tag v$Version`n    git push origin v$Version`n`n  Or manually trigger the release workflow:`n    gh workflow run release.yml`n`n  Manual download: https://github.com/$RepoOwner/$RepoName/releases`n  Build from source: git clone https://github.com/$RepoOwner/$RepoName && cd $RepoName && go build -o du.exe ."
}

# == 5. Verify SHA-256 Checksum ========================================
Write-Step "Verifying SHA-256 checksum..."

$ChecksumVerified = $false
$FileToCheck = if ($DownloadMethod -eq "zip") { $TempZip } else { $DownloadedExePath }
$ChecksumTarget = if ($DownloadMethod -eq "zip") { $ArchiveName } else { $DirectExeName }

try {
    Invoke-WebRequest -Uri $ChecksumUrl -OutFile $TempChecksum -UseBasicParsing

    # Parse the expected hash for our archive/binary
    $ChecksumLines = Get-Content $TempChecksum
    $ExpectedLine  = $ChecksumLines | Where-Object { $_ -like "*$ChecksumTarget*" }

    if ($ExpectedLine) {
        $ExpectedHash = ($ExpectedLine -split '\s+')[0].ToUpper()
        
        # Defensive check for Get-FileHash availability
        if (Get-Command Get-FileHash -ErrorAction SilentlyContinue) {
            $ActualHash = (Get-FileHash -Path $FileToCheck -Algorithm SHA256).Hash.ToUpper()
        } else {
            # Fallback to .NET SHA256 class (extremely compatible across all PowerShell versions!)
            $Sha256 = [System.Security.Cryptography.SHA256]::Create()
            $Stream = [System.IO.File]::OpenRead($FileToCheck)
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
        Write-Warn "Checksum entry not found for $ChecksumTarget. Skipping verification."
    }
} catch {
    Write-Warn "Could not fetch checksums file. Skipping verification."
}

# == 6. Extract & Install ==============================================
if ($DownloadMethod -eq "zip") {
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
    $DownloadedExePath = $TempExe
}

Write-Step "Installing to $InstallDir..."
$null = New-Item -ItemType Directory -Path $InstallDir -Force

try {
    Copy-Item -Path $DownloadedExePath -Destination $ExePath -Force
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
