Overall status: IMPLEMENTED — WINDOWS VERIFICATION PENDING. Implementation is complete and macOS-tested; nothing below is verified on Windows until group 8 passes (7.2 and 7.3 are Windows-only measurements).

## 1. Engine host (Go, milestone 1)

- [x] 1.1 Add hidden `du engine` command in cmd/engine.go: NDJSON loop, 1 MiB line cap, stdout reserved (D4), one encoder mutex; verify with cmd/engine_test.go driving it through io.Pipe (bad JSON, unknown method, id correlation)
- [x] 1.2 Implement `hello` (protocol 1 + Version) and `cancel`; verify handshake and cancel-unknown-id tests pass
- [x] 1.3 Implement per-request contexts, stdin-EOF shutdown, and the `busy` TryLock for state-changing methods; verify tests for concurrent read-only requests, a second destructive request getting `busy`, and EOF exiting cleanly
- [x] 1.4 Implement `status.get` (optional top processes) and `doctor.run`; verify results decode into the same structs `du status --json` and `du doctor --json` emit
- [x] 1.5 Implement `clean.scan` (categories in `cleanGroups` order, group, bytes, files, admin-blocked) and `clean.run` (engine-issued IDs only, progress events per category, cancel between categories, `admin_required` per-category error); verify with a test category rooted in t.TempDir() covering selected subset, unknown ID refused, cancel mid-run, and a symlink inside the root not traversed
- [x] 1.6 Run all gates (gofmt, vet, staticcheck, both GOOS) and `DU_NO_OPLOG=1 go test ./...`; update CLAUDE.md (engine row + layout)

## 2. GUI shell (milestone 2)

Status: IMPLEMENTED. Windows build, launch, and UI checks happen once, at the final gate (group 8).

- [x] 2.1 Create gui/Duster.slnx with Duster.App (WinUI 3 from the official winui-mvvm template, unpackaged + self-contained, x64 + ARM64 only), Duster.Core, Duster.Infrastructure, Duster.Tests; nullable + warnings as errors; verified on macOS: Core/Infrastructure/Tests build, App packages restore (XAML compile is Windows-only)
- [x] 2.2 Infrastructure `EngineClient`: path beside the app (link refused), redirected stdio, no window, handshake, correlation, progress, cancel, stderr drained, malformed lines skipped, exit detection, clean shutdown; verified on macOS against an in-memory engine and the real `du engine`
- [x] 2.3 Shell: NavigationView (Home, Clean, Restore, Analyze) routed by tag, Mica, system theme, engine status footer, error bar with Restart engine, AutomationIds (Windows-only code: verified at the final gate)
- [x] 2.4 Home page: engine state, `status.get` on connect and on Refresh (no timer), task links, engine-missing and protocol-mismatch states (Windows-only code: verified at the final gate)
- [x] 2.5 `gui/smoke.ps1`: builds du.exe and Duster.exe, runs the tests against the real engine, launches, checks the engine child, closes, checks for orphans (runs at the final gate)

## 3. Clean page (milestone 3)

Status: IMPLEMENTED, macOS-tested. Windows verification at the final gate.

- [x] 3.0 Move the ViewModels into Duster.Core (they use no WinUI types) and add `IAppHost` (confirm dialog, elevated restart, folder picker) so page logic is unit-tested on macOS; verified by ViewModelTests
- [x] 3.1 Clean page: categories grouped as the engine returns them, all selectable ones selected by default (as the TUI), admin-only ones shown with a shield and not selectable, confirm dialog stating deletion is permanent, progress, Stop, per-category outcomes, partial result on cancel; verified on macOS by ViewModelTests (confirm declined sends nothing, only selected selectable IDs sent, cancel reports what finished)
- [x] 3.2 "Restart as administrator": relaunch Duster.exe with the runas verb (the GUI's only ShellExecute; own path, no arguments) and exit; declined UAC reported; verified on macOS for the declined path (UAC itself is Windows-only)

## 4. Restore page (milestone 4)

Status: IMPLEMENTED, macOS-tested. Windows verification at the final gate.

- [x] 4.1 Engine: `restore.list`/`restore.run`/`restore.empty` over the existing quarantine code (`restoreListJSON` now shared with `du restore --json`), session IDs from the latest list only, gone sessions refused; verified by Go tests with `tempQuarantine` (unknown ID, conflict skip, item restore, gone session, empty)
- [x] 4.2 Restore page: sessions list, items of the selected one, Restore all / per item, Delete for good and Empty quarantine behind confirm dialogs; verified on macOS by ViewModelTests and the real-engine round trip (analyze recycle → quarantine → restore)

## 5. Analyze (milestone 5)

Status: IMPLEMENTED, macOS-tested. Windows verification at the final gate.

- [x] 5.1 Engine: `analyze.scan` (absolute path, progress every 100 ms, cancel mid-walk via `scanDirectoryCtx`, history changes), `analyze.children`, `analyze.recycle` (not the root; tree patched after); verified by Go tests (tests isolate LOCALAPPDATA)
- [x] 5.2 Analyze page: path + Browse (Windows App SDK FolderPicker) + Scan/Cancel, breadcrumb drill-down over virtualized lists, Largest files, Changes, Move to Recycle Bin with confirm, kept-in-quarantine never shown as freed; verified on macOS by ViewModelTests and the real-engine round trip

## 6. Packaging (milestone 6)

Status: IMPLEMENTED. The Windows CI jobs first run on the next push; installer, upgrade and uninstall are final-gate items.

- [x] 6.1 Single-file, self-contained publish profiles (one Duster.exe per arch); release.yml `build-gui` job (x64 + ARM64, fails if the publish leaves any file but Duster.exe) feeding the portable zips and the installer (Start menu "Duster" opens the GUI, the post-install launch runs unelevated); Makefile `gui`/`gui-test` and copy-if-present in Makefile + build-release.sh; ci.yml `gui` (WinUI build, x64 + ARM64) and `gui-tests` (Windows + Linux, real engine); verified on macOS: workflows parse, the Makefile copy loop runs both branches, `bash -n` passes. Duster.exe is not in the SignPath artifact configuration yet (signing is off)
- [x] 6.2 `du update` installs Duster.exe from the verified zip (optional for older releases; order duw.exe, Duster.exe, du.exe) and `du remove` removes it (delayed while the GUI runs); verified by Go tests (mixed-case zip entry, missing entry, removal)

## 7. Hardening (milestone 7)

- [x] 7.1 Final Windows verification checklist (docs/gui-windows-verification.md, 31 items, linked from docs/release-checklist.md) plus `gui/smoke.ps1` checking the single-file release layout and that the GUI starts nothing but du.exe; FlaUI automation deferred until the gate shows whether hosted runners allow UI automation
- [ ] 7.2 Accessibility Insights pass and keyboard-only walkthrough (Windows: checklist items 19-23)
- [ ] 7.3 Measure cold start and memory; Native AOT only if startup exceeds 1.5 s (Windows: checklist item 30)
- [x] 7.4 Code, security and performance review of the whole GUI and the engine additions (self-review plus an independent reviewer); fixed: analyze.recycle used the size from when the ID was issued, and left ancestor listings cached with pre-recycle sizes (both covered by TestEngineAnalyzeScanChildrenRecycle). Release-candidate audit fixed two more: `dotnet test` from the repo root (CI, Makefile, smoke.ps1) ignored gui/global.json and fell back to VSTest (moved to the repo root), and the GUI build output `Duster-windows-<arch>.exe` was the same file as the CLI's `duster-windows-<arch>.exe` on case-insensitive NTFS (renamed `duster-gui-windows-<arch>.exe`; the installer's GUI line is optional so windows-smoke, which builds no GUI, still compiles it)

## 8. Final Windows verification gate

- [ ] 8.1 Run docs/gui-windows-verification.md end to end on a Windows PC and record the results; tick Windows verification for milestones 2-7 only for items that passed
- [ ] 8.2 Fix what the gate finds, rerun `gui\smoke.ps1` and the affected manual checks
