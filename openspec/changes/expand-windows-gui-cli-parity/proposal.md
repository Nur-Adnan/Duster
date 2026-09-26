## Why

The V1 GUI (change `add-windows-gui`, commit 9bda094) covers Home, Clean, Restore and Analyze. Everything else a Duster user can do (purge, installers, uninstall, optimize, virtual disks, schedule, doctor, verify, benchmark, update, remove, the landing menu's Drivers, Startup, Network and Security views, several analyze and restore options) still needs a terminal. Goal of this change: **full user-facing CLI capability parity in the Windows GUI**, proven command by command against the CLI source.

## What Changes

- A complete CLI inventory (`cli-inventory.json`) and a parity matrix (`docs/gui-cli-parity.md`) that classify every command, flag, TUI key and environment variable as FULL, GUI-NATIVE-EQUIVALENT, CLI-ONLY, WINDOWS-ONLY or NOT-APPLICABLE, each with its reason.
- New engine methods over the existing CLI functions: `verify.run`, `benchmark.run`, `security.run`, `drivers.list`, `startup.list/toggle/remove`, `purge.scan/run`, `installer.scan/run`, `uninstall.list/run/sweep`, `optimize.list/run`, `vdisk.scan/run`, `schedule.get/set/off`, `update.check/install`, `remove.plan/run`, `oplog.list`, `analyze.reveal`.
- Extended V1 methods: `analyze.scan` takes `since` and `no_history` and returns IDs for changed folders; folder listings carry their breadcrumb; `restore.list` applies the same 7-day expiry as `du restore`; `restore.run` takes `dry_run`.
- New GUI pages in a grouped navigation (Overview, Cleanup, Storage, Recovery, System, Automation, Settings): Developer artifacts, Old installers, Apps, Virtual disks, Optimize, System (Startup apps, Security, Drivers), Diagnostics (Health check, Self-test, Benchmark), Schedule, Settings (About, Updates, Remove Duster). Home becomes the full status dashboard; Analyze and Restore gain the missing options; Restore gains the Activity log.
- Small Go extractions so the engine and the TUIs share one implementation: the purge scanner with cancellation, the installer and leftover sweeps, the native uninstall step, `isProtectedApp`, and schedule on/off as functions that return results instead of printing.
- **Destructive or privileged capabilities newly reachable from the GUI** (all through the same Go code, confirmation first): purge (quarantine, Recycle Bin, or permanent delete), installer and leftover sweeps (quarantine), running an app's own uninstaller, optimize (DNS flush, Delivery Optimization cache delete, TRIM and DISM as administrator), VHDX compaction with diskpart (administrator), Startup Run-key/StartupApproved writes and Startup-folder file deletion, Task Scheduler task create/delete, self-update (binary replacement), self-removal. Registry writes stay limited to the Startup entries the CLI already edits. Elevation only through the existing "Restart as administrator" (UAC).
- CLI behavior is unchanged except the refactors above, which keep output and semantics identical.

## Capabilities

### New Capabilities
- `engine-protocol-parity`: the engine methods and parameters this change adds, what each accepts and refuses.
- `desktop-gui-parity`: the GUI flows for every user-facing CLI capability, navigation, reports, confirmation and elevation rules for them.

### Modified Capabilities
None in `openspec/specs/` (the V1 capabilities are still in the unarchived change `add-windows-gui`; this change only adds requirements).

## Impact

- Subcommands touched: purge, installer, uninstall, schedule (refactors only), analyze and restore (engine parameters), engine. Every other command is reused unchanged. lib/ packages reused unchanged: `fs`, `elevation`, `sysinfo`, `uninstall`.
- Files: no new delete paths. Every delete, move, registry write, task registration and binary swap reachable from the GUI is one the CLI already performs through the same function.
- GUI: nine new pages, new ViewModels in `Duster.Core`, new DTOs and client methods, `IAppHost` gains copy-to-clipboard, open Windows Settings, exit and restart.
- Packaging unchanged (one single-file `Duster.exe` per architecture, `duster-gui-windows-<arch>.exe` artifact name).
- Tests: Go engine tests per method, ViewModel tests with a fake engine, real-engine round trips where the method works off Windows.

## Non-goals

- A command box that runs CLI text.
- Repair actions that the CLI does not have (for example doctor fixes).
- A `schedule run now` action (the CLI has none; manual cleaning is the Clean page).
- A settings store: Duster has no configuration file, and the GUI follows the Windows theme.
- Exposing `--json`, `--debug`, exit codes or piped output literally.
- MSIX, tray icon, notifications, localization.
