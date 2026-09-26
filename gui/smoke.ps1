# Duster GUI smoke test. Windows only; run from anywhere:
#   powershell -ExecutionPolicy Bypass -File gui\smoke.ps1              # x64
#   powershell -ExecutionPolicy Bypass -File gui\smoke.ps1 -Platform ARM64
# Builds du.exe/duw.exe and Duster.exe, runs the .NET tests against the real
# engine, then for both the Debug build and the release layout (single-file
# Duster.exe beside du.exe and duw.exe, as the zip and installer ship it):
# launches, checks the engine child is alive (a failed handshake stops it),
# closes the window, and checks no du.exe is left behind. Read-only: nothing
# here cleans or deletes anything outside its own temp folders.
param([ValidateSet('x64', 'ARM64')][string]$Platform = 'x64')
$ErrorActionPreference = 'Stop'

$repo = Split-Path $PSScriptRoot -Parent
$goArch = if ($Platform -eq 'ARM64') { 'arm64' } else { 'amd64' }
$rid = if ($Platform -eq 'ARM64') { 'win-arm64' } else { 'win-x64' }
$work = Join-Path $env:TEMP 'duster-smoke'
$results = [ordered]@{}

function Step($name, [scriptblock]$body) {
    Write-Host "`n== $name" -ForegroundColor Cyan
    & $body
    if ($LASTEXITCODE -and $LASTEXITCODE -ne 0) { throw "$name failed (exit $LASTEXITCODE)" }
    $results[$name] = 'PASS'
}

# Launch, check the engine child, close, check for an orphan.
function Test-Lifecycle($exe) {
    $app = Start-Process $exe -PassThru
    Start-Sleep -Seconds 6
    if ($app.HasExited) { throw "Duster.exe exited during startup (code $($app.ExitCode))" }
    $child = Get-CimInstance Win32_Process -Filter "ParentProcessId=$($app.Id) AND Name='du.exe'"
    if (-not $child) { throw 'no du.exe child: the engine did not start or failed its handshake (see the error bar in the window)' }
    $expected = Join-Path (Split-Path $exe) 'du.exe'
    if ($child.ExecutablePath -ne $expected) { throw "engine started from $($child.ExecutablePath), expected $expected" }
    $others = Get-CimInstance Win32_Process -Filter "ParentProcessId=$($app.Id)" | Where-Object { $_.Name -ne 'du.exe' }
    if ($others) { throw "Duster.exe started unexpected processes: $($others.Name -join ', ')" }
    Write-Host "engine pid $($child.ProcessId) ($($child.ExecutablePath)) under Duster pid $($app.Id)"

    [void]$app.CloseMainWindow()
    if (-not $app.WaitForExit(15000)) { throw 'Duster.exe did not exit within 15 s of closing its window' }
    $deadline = (Get-Date).AddSeconds(15)
    while ((Get-Process -Id $child.ProcessId -ErrorAction SilentlyContinue) -and (Get-Date) -lt $deadline) { Start-Sleep -Milliseconds 250 }
    if (Get-Process -Id $child.ProcessId -ErrorAction SilentlyContinue) { throw "orphan: du.exe pid $($child.ProcessId) still running" }
}

Write-Host "Windows $([Environment]::OSVersion.Version), $env:PROCESSOR_ARCHITECTURE"
Write-Host "dotnet $(dotnet --version); $(go version)"
if (Get-Command winapp -ErrorAction SilentlyContinue) { Write-Host "winapp $(winapp --version)" } else { Write-Host 'winapp: not installed (optional for this script)' }

New-Item -ItemType Directory -Force $work | Out-Null
$engine = Join-Path $work 'du.exe'
$launcher = Join-Path $work 'duw.exe'
Step 'build du.exe + duw.exe' {
    $env:GOOS = 'windows'; $env:GOARCH = $goArch; $env:CGO_ENABLED = '0'
    Push-Location $repo
    try {
        go build -o $engine .
        if (-not $LASTEXITCODE) { go build -ldflags '-H=windowsgui' -o $launcher ./launcher/duw }
    } finally { Pop-Location; Remove-Item Env:GOOS, Env:GOARCH, Env:CGO_ENABLED }
}

Step 'dotnet test (real engine)' {
    $env:DUSTER_ENGINE = $engine
    try { dotnet test --project (Join-Path $repo 'gui\Duster.Tests') } finally { Remove-Item Env:DUSTER_ENGINE }
}

Step 'dotnet build Duster.App (Debug)' {
    dotnet build (Join-Path $repo 'gui\Duster.App') -c Debug -p:Platform=$Platform
}

Step 'Debug build: launch + handshake + close + no orphan' {
    $exe = Get-ChildItem (Join-Path $repo "gui\Duster.App\bin\$Platform\Debug") -Recurse -Filter Duster.exe |
        Where-Object { $_.DirectoryName -like "*$rid*" } | Select-Object -First 1
    if (-not $exe) { throw "Duster.exe not found under gui\Duster.App\bin\$Platform\Debug" }
    Copy-Item $engine (Join-Path $exe.DirectoryName 'du.exe') -Force
    Test-Lifecycle $exe.FullName
}

$layout = Join-Path $work 'release-layout'
Step 'dotnet publish (single-file, as released)' {
    $out = Join-Path $work 'publish'
    Remove-Item $out, $layout -Recurse -Force -ErrorAction SilentlyContinue
    dotnet publish (Join-Path $repo 'gui\Duster.App') -c Release -p:Platform=$Platform -o $out
    if ($LASTEXITCODE) { return }
    $extra = Get-ChildItem $out -File | Where-Object { $_.Name -ne 'Duster.exe' -and $_.Extension -ne '.pdb' }
    if ($extra) { throw "single-file publish left extra files (du update would not install them): $($extra.Name -join ', ')" }
    New-Item -ItemType Directory $layout | Out-Null
    Copy-Item (Join-Path $out 'Duster.exe'), $engine, $launcher $layout
    Write-Host ("Duster.exe {0:N1} MB" -f ((Get-Item (Join-Path $layout 'Duster.exe')).Length / 1MB))
}

Step 'release layout: launch + handshake + close + no orphan' {
    Test-Lifecycle (Join-Path $layout 'Duster.exe')
}

Write-Host "`n== summary" -ForegroundColor Cyan
$results.GetEnumerator() | ForEach-Object { Write-Host ("{0,-55} {1}" -f $_.Key, $_.Value) }
Write-Host "`nNext: the manual part of docs/gui-windows-verification.md, starting with $layout\Duster.exe"
