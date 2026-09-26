## Why

Duster is CLI/TUI only, which keeps out users who never open a terminal. A native Windows GUI widens the audience, but Duster deletes user data and every safety rule lives in the Go engine with years of tests behind it, so the GUI must reuse that engine instead of re-implementing it.

## What Changes

- New hidden subcommand `du engine`: a long-lived process that speaks versioned NDJSON over stdin/stdout, calls the existing engine functions, streams progress, and supports cancellation. No network or named-pipe endpoint.
- New `gui/` tree: `Duster.exe`, a WinUI 3 (Windows App SDK 2.x, C#, .NET 10) app that starts `du.exe engine` from its own folder and drives it. **V1 pages: Home, Clean, Restore, Analyze.** Doctor, Purge, Uninstall, Installers, Optimize, Virtual disks, Schedule and the other deferred features stay CLI-only for now (see design.md, "Scope: V1, deferred, and CLI-only", which also lists the features that stay CLI-only for good); the shell adds a page with one navigation entry, so they need no redesign later.
- Engine methods for the V1 pages only: status, doctor, clean (done), restore, analyze. `doctor.run` is in the engine and client but no V1 page shows it (deferred). No refactor of CLI code paths is needed for V1.
- Packaging: `Duster.exe` (self-contained, unpackaged, x64 + ARM64) ships beside `du.exe` and `duw.exe` in the Inno installer and the portable zips; `du update` and `du remove` learn about it.
- CLI behavior and output are unchanged.

## Capabilities

### New Capabilities
- `engine-protocol`: the `du engine` stdio contract (framing, versioning, methods, progress, cancellation, errors, and which inputs it refuses).
- `desktop-gui`: what the Windows GUI shows and does, including previews, confirmations, elevation, and accessibility.

### Modified Capabilities
None (no existing specs).

## Impact

- Subcommands touched in V1: clean, restore, analyze, status, doctor (engine entry points only), update and remove (install and remove `Duster.exe`). Purge, installer, uninstall, optimize, vdisk, and schedule are untouched. lib/ packages reused unchanged: `fs`, `elevation`, `sysinfo`.
- Files: the GUI deletes and moves exactly what the CLI already does, through the same engine functions (V1: category cleans, quarantine restore and empty, Analyze's send-to-Recycle-Bin). No new delete paths. Registry: read-only.
- Elevation: the GUI runs unelevated; admin-only actions (in V1 only the prefetch clean category) offer "Restart as administrator", which relaunches `Duster.exe` elevated with UAC. No service, no elevated helper.
- New dependencies (all free/MIT): .NET 10 SDK, Windows App SDK, CommunityToolkit.Mvvm, Microsoft.Extensions.DependencyInjection, MSTest, FlaUI (UI smoke). Go side adds none.
- CI: a `gui` job on `windows-latest` (x64) and `windows-11-arm`; `Duster.Core`/`Duster.Infrastructure` tests also run on Linux.

## Non-goals

- MSIX packaging or Microsoft Store listing.
- Tray icon, background service, auto-start, notifications.
- Telemetry or any network use beyond the existing update check.
- Localization beyond English.
- Re-implementing any delete, scan, or path-safety logic in C#.
- Replacing or changing the CLI/TUI.
