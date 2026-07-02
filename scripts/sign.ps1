# Duster Authenticode Code Signing Script
# Usage: .\sign.ps1 -CertPath "C:\path\to\cert.pfx" -CertPassword "PasswordHere" -BinaryPath "dist/duster-windows-amd64.exe"

param (
    [string]$CertPath = $env:DU_SIGNING_CERT_PATH,
    [string]$CertPassword = $env:DU_SIGNING_CERT_PASSWORD,
    [string]$BinaryPath = "dist/duster-windows-amd64.exe"
)

$ErrorActionPreference = "Stop"

Write-Host "====== Duster Authenticode Code Signing Pipeline ======" -ForegroundColor Cyan

# 1. Validate parameters
if (-not $CertPath) {
    Write-Error "Certificate path must be provided via -CertPath or DU_SIGNING_CERT_PATH environment variable."
}
if (-not $CertPassword) {
    Write-Error "Certificate password must be provided via -CertPassword or DU_SIGNING_CERT_PASSWORD environment variable."
}
if (-not (Test-Path $CertPath)) {
    Write-Error "Certificate not found at: $CertPath"
}
if (-not (Test-Path $BinaryPath)) {
    Write-Error "Binary not found at: $BinaryPath"
}

# 2. Locate signtool.exe
$sdkPaths = @(
    "C:\Program Files (x86)\Windows Kits\10\bin\x64\signtool.exe",
    "C:\Program Files (x86)\Windows Kits\10\bin\10.0.*\x64\signtool.exe",
    "C:\Program Files (x86)\Windows Kits\8.1\bin\x64\signtool.exe"
)

$signtool = $null
foreach ($path in $sdkPaths) {
    $resolved = Resolve-Path $path -ErrorAction SilentlyContinue
    if ($resolved) {
        $signtool = $resolved.Path
        break
    }
}

if (-not $signtool) {
    # Fallback to PATH search
    $signtool = Get-Command signtool.exe -ErrorAction SilentlyContinue | Select-Object -ExpandProperty Source
}

if (-not $signtool) {
    Write-Error "signtool.exe could not be located in Windows SDK paths or system PATH. Please install the Windows SDK."
}

Write-Host "Using SignTool located at: $signtool" -ForegroundColor Green
Write-Host "Signing target: $BinaryPath" -ForegroundColor Yellow

# 3. Perform modern SHA-256 signing with RFC 3161 timestamping
# We use DigiCert timestamp server as primary and Sectigo as backup
$timestampServers = @(
    "http://timestamp.digicert.com",
    "http://timestamp.sectigo.com",
    "http://timestamp.globalsign.com/tsa/r6advanced1"
)

$signed = $false
foreach ($server in $timestampServers) {
    Write-Host "Attempting signature with timestamp server: $server..." -ForegroundColor Gray
    
    # NB: not named $args — that is a PowerShell automatic variable and
    # assigning it at script scope is fragile across PS versions.
    $signtoolArgs = @(
        "sign",
        "/f", $CertPath,
        "/p", $CertPassword,
        "/fd", "sha256",
        "/tr", $server,
        "/td", "sha256",
        $BinaryPath
    )

    & $signtool $signtoolArgs 2>&1
    
    if ($LASTEXITCODE -eq 0) {
        Write-Host "Successfully signed binary with timestamp server: $server" -ForegroundColor Green
        $signed = $true
        break
    } else {
        Write-Host "Failed signing with timestamp server: $server. Retrying with backup server..." -ForegroundColor Red
    }
}

if (-not $signed) {
    Write-Error "Failed to sign binary using any of the configured timestamp servers."
}

# 4. Verify the signature
Write-Host "Verifying signature of: $BinaryPath..." -ForegroundColor Cyan
& $signtool verify /pa /v $BinaryPath

if ($LASTEXITCODE -eq 0) {
    Write-Host "Verification SUCCESSFUL. Binary is ready for public release!" -ForegroundColor Green
} else {
    Write-Error "Verification FAILED. Code signing is corrupted or invalid."
}
