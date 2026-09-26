Overall status: IMPLEMENTED — WINDOWS VERIFICATION PENDING. Groups 1-8 are implemented; the Go engine, the ViewModels and the client are tested on macOS; the WinUI pages have not been compiled yet (WinUI builds only on Windows). Group 9 is the Windows gate.

## 1. Inventory and plan (M8)

- [x] 1.1 CLI inventory from the source (every command, subcommand, flag, TUI key, environment variable) in cli-inventory.json; verified by cross-checking `grep Flags()` and the TUI key handlers against it
- [x] 1.2 Parity matrix docs/gui-cli-parity.md with a status and reason for every row; verified: no row without a status, no MISSING
- [x] 1.3 Proposal, design (information architecture, reuse table, milestones), specs, tasks; verified by `openspec validate expand-windows-gui-cli-parity --strict`

## 2. Engine parity methods (M9)

- [x] 2.1 Extractions shared with the TUIs: `scanArtifactsCtx`, `sweepInstallerItems`, `sweepLeftoverItems`, `uninstallApp`, `isProtectedApp`, `applyScheduleOn`, `turnScheduleOff`; verify the existing purge, installer, uninstall and schedule tests still pass unchanged
- [x] 2.2 Listing helper (latest listing per kind, 1-based IDs); verify unknown, repeated and stale IDs are refused in tests
- [x] 2.3 `verify.run`, `benchmark.run`, `security.run`, `drivers.list`, `oplog.list`; verify results decode to the CLI structs and non-Windows views fail cleanly
- [x] 2.4 `purge.scan`/`purge.run` with modes, progress, cancel; verify with a temp project and `tempQuarantine` (keep, permanent, cancel, unknown ID, relative path)
- [x] 2.5 `installer.scan`/`installer.run`; verify with a temp USERPROFILE\Downloads (age, size filter, keep)
- [x] 2.6 `uninstall.list`/`uninstall.run`/`uninstall.sweep`; verify list, protected refusal and unknown IDs (no uninstaller is run in unit tests)
- [x] 2.7 `optimize.list`/`optimize.run`, `vdisk.scan`/`vdisk.run`; verify list shapes and refusals (unit tests never run ipconfig, defrag, DISM or diskpart)
- [x] 2.8 `startup.*`, `schedule.*`; verify refusals and the schedule parser errors (unit tests never register a task)
- [x] 2.9 `update.check`/`update.install` through a replaceable release fetcher, `remove.plan`; verify with a fake release (newer, same, pre-release) and that no test calls `remove.run` or `swapBinary`
- [x] 2.10 Analyze `since`/`no_history`, change IDs, breadcrumb, `analyze.reveal`; restore expiry on list and `dry_run`; verify in the analyze and restore engine tests
- [x] 2.11 Gates: gofmt, vet and staticcheck on both GOOS, `DU_NO_OPLOG=1 go test -race ./...`

## 3. C# contracts (M10)

- [x] 3.1 DTOs and `IEngineClient` methods for every new method, registered in `EngineJson`; verify real-engine client tests for the methods that work off Windows
- [x] 3.2 `IAppHost`: CopyText, OpenAppsSettings, Exit, Restart; fake host in tests

## 4. Overview and diagnostics (M11)

- [x] 4.1 Home dashboard (all status fields, Live while visible, top processes on request, Copy report); verify HomeViewModel tests (live starts/stops, report text)
- [x] 4.2 Diagnostics page (Health check, Self-test, Benchmark, Copy report); verify DiagnosticsViewModel tests
- [x] 4.3 System page (Startup apps toggle/remove-disabled with confirm, Security, Drivers); verify SystemViewModel tests (declined confirm sends nothing, only disabled entries offered)

## 5. Cleanup pages (M12)

- [x] 5.1 Developer artifacts (folder, scan, modes, confirm wording per mode, progress, Stop, tally); verify PurgeViewModel tests
- [x] 5.2 Old installers (minimum size, scan, confirm, result); verify InstallersViewModel tests
- [x] 5.3 Apps (search, protected, uninstall with confirm, leftovers unselected, keep with confirm); verify AppsViewModel tests

## 6. Storage and system tools (M13)

- [x] 6.1 Virtual disks (list, block reasons, admin gate, confirm, progress, Stop); verify VirtualDisksViewModel tests
- [x] 6.2 Optimize (tasks with admin marks, component store unchecked by default, Preview, Run with confirm, reclaim report); verify OptimizeViewModel tests

## 7. Automation and settings (M14)

- [x] 7.1 Schedule (status, on/edit, preview, off with confirm); verify ScheduleViewModel tests
- [x] 7.2 Settings (About, Updates with restart, Remove Duster with setup-install note, exit after removal); verify SettingsViewModel tests

## 8. V1 page completion, review, packaging, audit (M15-M16)

- [x] 8.1 Analyze: Compare with, Don't save this scan, click a change to open it, Show in Explorer; verify AnalyzeViewModel tests
- [x] 8.2 Restore: Preview, Activity log; verify RestoreViewModel tests
- [x] 8.3 Navigation groups and page registration; every XAML element and attribute name checked against the WinUI 2.3.9 metadata that Windows App SDK 2.5.1 ships (first compile happens on Windows)
- [x] 8.4 Security and performance review of every new method and page; fix findings with tests
- [x] 8.5 Packaging check (no new files, single-file publish unchanged, artifact names distinct); extend docs/gui-windows-verification.md with the new pages
- [x] 8.6 Final parity audit against the CLI source; update docs/gui-cli-parity.md and cli-inventory.json; `openspec validate expand-windows-gui-cli-parity --strict`
- [x] 8.7 WinUI code review (winui-code-review checklist): bindings, templates, accessibility names, theming, no blocking waits; `gui\smoke.ps1` now opens every page through UI Automation, so a page that fails to load fails the smoke

## 9. Final Windows verification (M17)

- [ ] 9.1 Run docs/gui-windows-verification.md end to end on Windows (x64 and ARM64) and record the results
- [ ] 9.2 Fix what it finds; rerun `gui\smoke.ps1` and the affected checks

## Notes from implementation

- Found while testing: source-generated JSON sets every `init` DTO property on creation, so fields Go omits became null. In V1 this crashes the Clean page after a successful clean; fixed on main in PR #33 (DTOs use `set`) and here; `OmittedFieldsKeepTheirDefaults` guards it.
- `restore.list` now applies the 7-day expiry, like `du restore` (it holds `busy`).
- Cancel granularity as designed (P5): between items; not cancellable: verify, benchmark, security, drivers, a running app uninstaller, an update download.
