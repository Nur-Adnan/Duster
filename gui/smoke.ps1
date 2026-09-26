# Milestone 2 smoke test for the Duster GUI. Windows only; run from anywhere:
#   powershell -ExecutionPolicy Bypass -File gui\smoke.ps1              # x64
#   powershell -ExecutionPolicy Bypass -File gui\smoke.ps1 -Platform ARM64
# Builds du.exe and Duster.exe, runs the .NET tests against the real engine,
# launches the app, checks the engine is alive (a failed handshake stops it),
# closes the window, and checks no du.exe is left behind. Read-only: nothing
# here cleans or deletes anything.
param([ValidateSet('x64', 'ARM64')][string]$Platform = 'x64')
$ErrorActionPreference = 'Stop'

$repo = Split-Path $PSScriptRoot -Parent
$goArch = if ($Platform -eq 'ARM64') { 'arm64' } else { 'amd64' }
$rid = if ($Platform -eq 'ARM64') { 'win-arm64' } else { 'win-x64' }
$results = [ordered]@{}

function Step($name, [scriptblock]$body) {
    Write-Host "`n== $name" -ForegroundColor Cyan
    & $body
    if ($LASTEXITCODE -and $LASTEXITCODE -ne 0) { throw "$name failed (exit $LASTEXITCODE)" }
    $results[$name] = 'PASS'
}

Write-Host "Windows $([Environment]::OSVersion.Version), $env:PROCESSOR_ARCHITECTURE"
Write-Host "dotnet $(dotnet --version); $(go version)"
if (Get-Command winapp -ErrorAction SilentlyContinue) { Write-Host "winapp $(winapp --version)" } else { Write-Host 'winapp: not installed (optional for this script)' }

$engine = Join-Path $env:TEMP "duster-smoke\du.exe"
New-Item -ItemType Directory -Force (Split-Path $engine) | Out-Null
Step 'build du.exe' {
    $env:GOOS = 'windows'; $env:GOARCH = $goArch; $env:CGO_ENABLED = '0'
    Push-Location $repo
    try { go build -o $engine . } finally { Pop-Location; Remove-Item Env:GOOS, Env:GOARCH, Env:CGO_ENABLED }
}

Step 'dotnet test (real engine)' {
    $env:DUSTER_ENGINE = $engine
    try { dotnet test --project (Join-Path $repo 'gui\Duster.Tests') } finally { Remove-Item Env:DUSTER_ENGINE }
}

Step 'dotnet build Duster.App' {
    dotnet build (Join-Path $repo 'gui\Duster.App') -c Debug -p:Platform=$Platform
}

$exe = Get-ChildItem (Join-Path $repo "gui\Duster.App\bin\$Platform\Debug") -Recurse -Filter Duster.exe |
    Where-Object { $_.DirectoryName -like "*$rid*" } | Select-Object -First 1
if (-not $exe) { throw "Duster.exe not found under gui\Duster.App\bin\$Platform\Debug" }
Copy-Item $engine (Join-Path $exe.DirectoryName 'du.exe') -Force
Write-Host "app: $($exe.FullName)"

Step 'launch + handshake' {
    $script:app = Start-Process $exe.FullName -PassThru
    Start-Sleep -Seconds 6
    if ($script:app.HasExited) { throw "Duster.exe exited during startup (code $($script:app.ExitCode))" }
    $script:child = Get-CimInstance Win32_Process -Filter "ParentProcessId=$($script:app.Id) AND Name='du.exe'"
    if (-not $script:child) { throw 'no du.exe child: the engine did not start or failed its handshake (see the error bar in the window)' }
    Write-Host "engine pid $($script:child.ProcessId) is running under Duster pid $($script:app.Id)"
}

Step 'close + no orphan' {
    [void]$script:app.CloseMainWindow()
    if (-not $script:app.WaitForExit(15000)) { throw 'Duster.exe did not exit within 15 s of closing its window' }
    $deadline = (Get-Date).AddSeconds(15)
    while ((Get-Process -Id $script:child.ProcessId -ErrorAction SilentlyContinue) -and (Get-Date) -lt $deadline) { Start-Sleep -Milliseconds 250 }
    if (Get-Process -Id $script:child.ProcessId -ErrorAction SilentlyContinue) { throw "orphan: du.exe pid $($script:child.ProcessId) still running" }
}

Write-Host "`n== summary" -ForegroundColor Cyan
$results.GetEnumerator() | ForEach-Object { Write-Host ("{0,-28} {1}" -f $_.Key, $_.Value) }
Write-Host "`nNow check by eye: run $($exe.FullName) and look at Home (device, drives), the pane footer ('Engine <version>'), Clean > Scan, and light/dark/high contrast."
