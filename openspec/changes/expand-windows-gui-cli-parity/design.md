## Context

V1 (`add-windows-gui`) established the architecture this change keeps: WinUI 3 `Duster.App` (views, navigation, `WindowsAppHost`), `Duster.Core` (DTOs, `IEngineClient`, ViewModels, no WinUI types), `Duster.Infrastructure` (engine process, NDJSON channel, source-generated JSON), and the Go `du engine` host that owns every scan, delete and Windows call. The engine accepts only IDs it issued, holds `busy` for state-changing methods and cancels through a per-request context. The full CLI inventory is `cli-inventory.json`; the matrix is `docs/gui-cli-parity.md`.

## Goals / Non-Goals

**Goals:**
- Every user-facing CLI capability reachable in the GUI through the same Go function the CLI calls.
- One coherent product: grouped navigation, pages that combine related commands, shared patterns for progress, confirmation, elevation, empty and error states.
- Everything testable off Windows is tested on macOS: engine methods (with temp roots and stubs), ViewModels (fake engine), client serialization.

**Non-Goals:**
- Moving engine code from `cmd/` to `lib/` (V1 decision D2 stands).
- A generic "run CLI command" method or any method taking a raw path to delete, a registry path, or an executable.

## Decisions

### P1. Information architecture
Navigation groups (NavigationViewItemHeader) and pages:

| Group | Page | CLI it covers |
|---|---|---|
| Overview | Home | status, landing Network |
| Cleanup | Clean | clean |
| | Developer artifacts | purge |
| | Old installers | installer |
| | Apps | uninstall |
| Storage | Analyze | analyze |
| | Virtual disks | vdisk |
| Recovery | Restore (Kept items, Activity log) | restore, clean TUI `v` |
| System | Optimize | optimize (+ reclaim report) |
| | System (Startup apps, Security, Drivers) | landing Startup, Security, Drivers |
| | Diagnostics (Health check, Self-test, Benchmark) | doctor, verify, benchmark |
| Automation | Schedule | schedule |
| Footer | Settings (About, Updates, Remove Duster) | --version, update, remove |

Related read-only reports share a page with a SelectorBar (System, Diagnostics, Restore) instead of one tiny page each. Alternative (one page per command, 19 pages) rejected: the landing views and diagnostics are one screen of data each.

### P2. Engine-issued IDs for every new list
A generic helper keeps the latest listing per kind (`purge`, `installer`, `apps`, `leftovers`, `vdisk`, `startup`); action methods take 1-based IDs into that listing and refuse anything else. Before acting the engine re-checks the live state: purge/installer/leftover targets go through `purgeOne`/`quarantinePath` (which re-run `fs.IsValidPath` and fail on a vanished path), startup entries are re-read and matched on name, location and command, apps are re-read and matched on name and uninstall string, disks are re-checked by `compactVirtualDisk`. Alternative (GUI sends paths) rejected: the GUI is untrusted input (V1 D5).

### P3. Reuse, with small extractions
| Method | Reuses |
|---|---|
| `verify.run`, `benchmark.run`, `security.run`, `drivers.list` | `runIntegrityVerification`, `runSystemBenchmark`, `runSecurityAudit`, `scanInstalledDrivers` |
| `startup.*` | `getStartupEntries`, `toggleStartupApproval`, `removeStartupEntry`; remove only disabled entries (TUI rule) |
| `purge.*` | `scanArtifactsCtx` (new: `scanArtifacts` with a context check, which the TUI keeps calling), `purgeOne`, `purgeTally`, `sweepQuarantine(sweepFull)` |
| `installer.*` | `scanInstallerItems`, `sweepInstallerItems` (extracted from `runSetupSweepCmd`) |
| `uninstall.*` | `uninstall.GetInstalledApps`, `isProtectedApp` (extracted), `uninstallApp` (extracted from `runNativeUninstallCmd`: HKCU-while-elevated refusal, 3010/1641 success, still-installed check), `scanAppLeftovers`, `sweepLeftoverItems` (extracted from `runSweepCmd`) |
| `optimize.*` | `optimizeTasks(true)`, `execOptimizeTask` with `optCtx` set to the request context under `busy`, `buildReclaimItems` |
| `vdisk.*` | `scanVirtualDisks`, `vdiskAdvice`, `shutdownWSL`, `compactVirtualDisk(ctx)` (detaches on cancel) |
| `schedule.*` | `readScheduleStatus`, `applyScheduleOn` and `turnScheduleOff` (extracted from `executeScheduleOn/Off`), `scheduleSafe`/`scheduleOptIn` for the choices |
| `update.*` | `fetchLatestRelease` (through a package var so tests never reach the network), `isNewerVersion`, `downloadVerifiedBinary`, `swapBinary` |
| `remove.*` | `quarantineHeld`, `emptyAllQuarantines`, `cleanDusterDir`, `removeScheduleAndLauncher`, `removeGUI`, `scheduleDelayedDelete` |
| `oplog.list` | `readOperationsLog` (newest first, capped) |
| `analyze.reveal` | `openInExplorer` (Explorer from the secure Windows dir, path from an issued ID; `nearestExisting` for changes) |

New methods live in `cmd/engine_parity.go` so `cmd/engine.go` stays the transport.

### P4. What holds `busy`
State-changing: `purge.run`, `installer.run`, `uninstall.run`, `uninstall.sweep`, `optimize.run` (also its preview, because DISM analysis runs minutes and shares `optCtx`), `vdisk.run`, `startup.toggle`, `startup.remove`, `schedule.set`, `schedule.off`, `update.install`, `remove.run`, `restore.list` (it now deletes expired sessions, as `du restore` does). Everything else is read-only and concurrent.

### P5. Cancellation
Between items for purge, installers, leftovers, optimize tasks, vdisks (plus `compactVirtualDisk`'s own detach-on-cancel), clean (V1). Inside the purge scan (walk callback). Not cancellable, reported as such in the UI: verify, benchmark, security, drivers (30 s timeout in the CLI code), a running app uninstaller (it is the app's own wizard), update download (bounded by the CLI's HTTP timeouts).

### P6. Status dashboard refresh
Home refreshes once on open and on Refresh. A Live toggle polls `status.get` every 2 s only while Home is visible (stopped on navigation away and on window close). Top processes need a 1 s sample, so they load only when asked. No background polling anywhere else.

### P7. Reports instead of `--json`
Diagnostics and Home offer Copy report: a plain-text summary built in the ViewModel and put on the clipboard through `IAppHost.CopyText`. The data commands' `--json` stays CLI-only: the page already shows the data.

### P8. Elevation and Windows integration
The GUI never elevates silently. Admin-only actions (prefetch, TRIM, component store, vdisk compaction, HKLM startup entries) are marked and offer the existing Restart as administrator. Windows integration stays in the engine (registry, Task Scheduler, diskpart, DISM, Explorer, Recycle Bin) or in `WindowsAppHost` for UI-only services: clipboard (`DataPackage`), opening `ms-settings:appsfeatures` (`Launcher.LaunchUriAsync`), exit and restart of `Duster.exe` itself. No `cmd.exe`, no PowerShell, no arbitrary executable from C#.

### P9. Update and remove from a running GUI
`update.install` swaps du.exe, duw.exe and Duster.exe (rename-and-replace, old copies deleted after exit); the GUI then offers Restart Duster, which starts the new `Duster.exe` from its own path and exits. `remove.run` performs `du remove`'s steps and the GUI exits at once; Duster.exe and du.exe are deleted after exit. When `unins*.exe` from the setup sits beside Duster.exe, the page recommends Windows Settings > Apps and links to it.

### P10. Milestones
M8 inventory, matrix, OpenSpec. M9 engine parity methods with Go tests. M10 C# DTOs and client. M11 dashboard, diagnostics, system views. M12 cleanup pages (purge, installers, apps). M13 storage and system tools (virtual disks, optimize). M14 schedule and settings (update, remove). M15 Analyze and Restore completion. M16 review (security, performance), packaging check, final parity audit. M17 final Windows verification (docs/gui-windows-verification.md, extended).

## Risks / Trade-offs

- [Nine new XAML pages never compiled on this Mac] → reuse V1 page patterns verbatim, verify every WinUI type against the SDK metadata, keep logic in tested ViewModels; the first Windows build is the gate.
- [Uninstall runs third-party uninstallers from a GUI] → same `newUninstallCmd`/`runTree` path as the TUI, protected-app list enforced in the engine, HKCU-while-elevated refused, leftovers only offered after the app is gone and never preselected.
- [Self-update and self-removal while the GUI runs] → the CLI's rename-and-replace and delayed delete already handle a running image; unit tests never call `remove.run` or `swapBinary`.
- [`optCtx` is a package global] → set and cleared only under `busy`, so one optimize runs at a time. ponytail: pass a context through `execOptimizeTask` if a second caller appears.
- [restore.list now deletes expired sessions] → identical to `du restore`, which users already run; only past-7-day sessions, never the low-space pass.
- [Driver scan runs PowerShell] → it is the CLI's existing engine-side query with a fixed script and 30 s timeout; the GUI still starts only `du.exe`.

## Migration Plan

Additive; the V1 pages and protocol stay compatible (protocol stays 1: methods are added, none removed or redefined). Rollback: drop the new pages from navigation.
