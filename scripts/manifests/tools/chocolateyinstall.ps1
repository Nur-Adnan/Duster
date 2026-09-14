$ErrorActionPreference = 'Stop'

# Downloads the release named by the package version and checks its SHA-256.
# After each release, set $checksum64 to duster-windows-amd64.exe's line in
# checksums-sha256.txt (docs/release-checklist.md, step 5).
$checksum64 = 'b202523ac0d201a45ebe12dfa03ab2e22d437d59575a201f61b4de66639f8bcc'
$toolsDir   = Split-Path -Parent $MyInvocation.MyCommand.Definition
$url64      = "https://github.com/Nur-Adnan/Duster/releases/download/v$($env:ChocolateyPackageVersion)/duster-windows-amd64.exe"

# Chocolatey puts a shim for each .exe in the package folder on PATH, so this
# becomes `du`. Uninstalling the package removes both.
Get-ChocolateyWebFile -PackageName $env:ChocolateyPackageName `
    -FileFullPath (Join-Path $toolsDir 'du.exe') `
    -Url64bit $url64 -Checksum64 $checksum64 -ChecksumType64 'sha256'
