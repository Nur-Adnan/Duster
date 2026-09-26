## Why

Duster is CLI/TUI only, which keeps out users who never open a terminal. A native Windows GUI widens the audience, but Duster deletes user data and every safety rule lives in the Go engine with years of tests behind it, so the GUI must reuse that engine instead of re-implementing it.

## What Changes

- New hidden subcommand `du engine`: a long-lived process that speaks versioned NDJSON over stdin/stdout, calls the existing engine functions, streams progress, and supports cancellation. No network or named-pipe endpoint.
- New `gui/` tree: `Duster.exe`, a WinUI 3 (Windows App SDK 2.x, C#, .NET 10) app that starts `du.exe engine` from its own folder and drives it. Pages mirror the CLI: Home (status + doctor), Clean, Analyze, Purge, Uninstall, Installers, Optimize, Virtual disks, Restore, Schedule, Settings.
- Small engine refactors so the GUI and the TUI share one code path: clean include-list selection, the uninstall run/leftover sweep out of the Bubble Tea model, and progress + cancel hooks in clean and purge loops.
- Packaging: `Duster.exe` (self-contained, unpackaged, x64 + ARM64) ships beside `du.exe` and `duw.exe` in the Inno installer and the portable zips; `du update` and `du remove` learn about it.
- CLI behavior and output are unchanged.

## Capabilities

### New Capabilities
- `engine-protocol`: the `du engine` stdio contract (framing, versioning, methods, progress, cancellation, errors, and which inputs it refuses).
- `desktop-gui`: what the Windows GUI shows and does, including previews, confirmations, elevation, and accessibility.

### Modified Capabilities
None (no existing specs).

## Impact

- Subcommands touched: clean, analyze, purge, installer, uninstall, restore, optimize, vdisk, schedule, status, doctor (engine entry points only), update and remove (install and remove `Duster.exe`). lib/ packages reused unchanged: `fs`, `elevation`, `sysinfo`, `uninstall`.
- Files: the GUI deletes and moves exactly what the CLI already does, through the same engine functions (quarantine, Recycle Bin, category cleans, uninstallers, diskpart compaction). No new delete paths. Registry: read-only except the startup Run-key edit the landing view already has.
- Elevation: the GUI runs unelevated; admin-only actions (prefetch clean, defrag, DISM, vdisk compaction) offer "Restart as administrator", which relaunches `Duster.exe` elevated with UAC. No service, no elevated helper.
- New dependencies (all free/MIT): .NET 10 SDK, Windows App SDK, CommunityToolkit.Mvvm, Microsoft.Extensions.DependencyInjection, MSTest, FlaUI (UI smoke). Go side adds none.
- CI: a `gui` job on `windows-latest` (x64) and `windows-11-arm`; `Duster.Core` tests also run on Linux.

## Non-goals

- MSIX packaging or Microsoft Store listing.
- Tray icon, background service, auto-start, notifications.
- Telemetry or any network use beyond the existing update check.
- Localization beyond English.
- Re-implementing any delete, scan, or path-safety logic in C#.
- Replacing or changing the CLI/TUI.
