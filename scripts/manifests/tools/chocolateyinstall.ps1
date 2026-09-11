$ErrorActionPreference = 'Stop'

# Downloads the release named by the package version and checks its SHA-256.
# After each release, set $checksum64 to duster-windows-amd64.exe's line in
# checksums-sha256.txt (docs/release-checklist.md, step 5).
$checksum64 = 'd94bbbebbf3f6c807979d03f49b09a932b5bee75d41c91941641e1ce4c00d6f0'
$toolsDir   = Split-Path -Parent $MyInvocation.MyCommand.Definition
$url64      = "https://github.com/Nur-Adnan/Duster/releases/download/v$($env:ChocolateyPackageVersion)/duster-windows-amd64.exe"

# Chocolatey puts a shim for each .exe in the package folder on PATH, so this
# becomes `du`. Uninstalling the package removes both.
Get-ChocolateyWebFile -PackageName $env:ChocolateyPackageName `
    -FileFullPath (Join-Path $toolsDir 'du.exe') `
    -Url64bit $url64 -Checksum64 $checksum64 -ChecksumType64 'sha256'
