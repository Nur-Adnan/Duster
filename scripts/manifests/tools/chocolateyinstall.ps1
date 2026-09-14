$ErrorActionPreference = 'Stop'

# Downloads the release named by the package version and checks its SHA-256.
# After each release, set $checksum64 to duster-windows-amd64.exe's line in
# checksums-sha256.txt (docs/release-checklist.md, step 5).
$checksum64 = '95b358cd0323fc6b1218caf7c122381cece4d7f5ee947d1bde3b88f84893971c'
$toolsDir   = Split-Path -Parent $MyInvocation.MyCommand.Definition
$url64      = "https://github.com/Nur-Adnan/Duster/releases/download/v$($env:ChocolateyPackageVersion)/duster-windows-amd64.exe"

# Chocolatey puts a shim for each .exe in the package folder on PATH, so this
# becomes `du`. Uninstalling the package removes both.
Get-ChocolateyWebFile -PackageName $env:ChocolateyPackageName `
    -FileFullPath (Join-Path $toolsDir 'du.exe') `
    -Url64bit $url64 -Checksum64 $checksum64 -ChecksumType64 'sha256'
