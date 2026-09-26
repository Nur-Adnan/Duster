## 1. Engine host (Go, milestone 1)

- [x] 1.1 Add hidden `du engine` command in cmd/engine.go: NDJSON loop, 1 MiB line cap, stdout reserved (D4), one encoder mutex; verify with cmd/engine_test.go driving it through io.Pipe (bad JSON, unknown method, id correlation)
- [x] 1.2 Implement `hello` (protocol 1 + Version) and `cancel`; verify handshake and cancel-unknown-id tests pass
- [x] 1.3 Implement per-request contexts, stdin-EOF shutdown, and the `busy` TryLock for state-changing methods; verify tests for concurrent read-only requests, a second destructive request getting `busy`, and EOF exiting cleanly
- [x] 1.4 Implement `status.get` (optional top processes) and `doctor.run`; verify results decode into the same structs `du status --json` and `du doctor --json` emit
- [x] 1.5 Implement `clean.scan` (categories in `cleanGroups` order, group, bytes, files, admin-blocked) and `clean.run` (engine-issued IDs only, progress events per category, cancel between categories, `admin_required` per-category error); verify with a test category rooted in t.TempDir() covering selected subset, unknown ID refused, cancel mid-run, and a symlink inside the root not traversed
- [x] 1.6 Run all gates (gofmt, vet, staticcheck, both GOOS) and `DU_NO_OPLOG=1 go test ./...`; update CLAUDE.md (engine row + layout)

## 2. GUI shell (milestone 2)

- [ ] 2.1 Scaffold gui/ with `winapp new --template winui-mvvm` on the PC (or equivalent csproj), add Duster.Core + Duster.Core.Tests, gui/Duster.slnx, x64 + ARM64 only, nullable + warnings as errors; verify `dotnet build -c Release -p:Platform=x64` succeeds on windows-latest
- [ ] 2.2 Duster.Core `EngineClient`: start du.exe beside the app (link refused), request/reply correlation, events as IAsyncEnumerable, cancel, crash detection; verify MSTest suite against an in-memory stream pair runs on macOS and Linux
- [ ] 2.3 Shell: NavigationView with all pages as placeholders, Mica backdrop, theme setting, AutomationIds; verify on the PC with `.\BuildAndRun.ps1` in light, dark, and high contrast
- [ ] 2.4 Home page: live status poll every 2 s (no top processes), doctor summary, engine-missing and protocol-mismatch screens; verify on the PC and by renaming du.exe
- [ ] 2.5 CI job `gui` (windows-latest x64, windows-11-arm ARM64) + Duster.Core tests on ubuntu; verify the jobs pass on the PR

## 3. Clean page (milestone 3)

- [ ] 3.1 Clean page: grouped categories with sizes from `clean.scan`, select/deselect, admin shield, confirm dialog, progress, Cancel, results; verify on the PC against categories that are safe to empty, plus a ViewModel unit test with a fake EngineClient
- [ ] 3.2 "Restart as administrator" relaunch; verify on the PC that UAC appears and prefetch becomes available

## 4. Quarantine-based pages (milestone 4)

- [ ] 4.1 Engine: progress + context hooks in purge loops, `purge.scan`/`purge.run`, `installer.scan`/`installer.run`, `restore.list`/`restore.run`/`restore.empty` with engine-issued IDs; verify Go tests with `tempQuarantine` for locked file, access denied, and reparse point scenarios
- [ ] 4.2 Purge, Installers, and Restore pages; verify a GUI purge shows up in `du restore` and restores (windows-smoke or a test folder on the PC)

## 5. Analyze (milestone 5)

- [ ] 5.1 Engine: `analyze.scan` with throttled progress and history changes (tests set LOCALAPPDATA to t.TempDir()); verify Go tests
- [ ] 5.2 Analyze page: virtualized folder tree, largest files, changes since last scan, send to Recycle Bin via engine; verify on the PC with a large folder that the UI stays responsive

## 6. Uninstall (milestone 6)

- [ ] 6.1 Extract the uninstall run + leftover sweep from `uninstallModel.Update` into a function the TUI and the engine share; verify existing uninstall tests pass unchanged and a new test covers the extracted function
- [ ] 6.2 Engine `uninstall.list`/`uninstall.run`/`uninstall.leftovers`; Uninstall page with search, leftovers starting unselected; verify with the e2e Inno test app in windows-smoke

## 7. Remaining pages (milestone 7)

- [ ] 7.1 Engine + pages for Optimize (with reclaim report), Virtual disks, Schedule; verify Go tests and on the PC (vdisk compaction only in windows-smoke)
- [ ] 7.2 Settings: theme, update check, about; `du update` installs Duster.exe too (extend `releaseBinaries`); verify update tests cover the new binary

## 8. Packaging (milestone 8)

- [ ] 8.1 Publish Duster.exe self-contained for win-x64 and win-arm64 in release.yml, Makefile, and build-release.sh; add to the Inno installer (Start menu shortcut) and zips; verify the release dry run produces both and checksums cover them
- [ ] 8.2 `du remove` removes Duster.exe; verify remove tests and a windows-smoke install/uninstall

## 9. Hardening (milestone 9)

- [ ] 9.1 FlaUI smoke test in windows-smoke (launch, visit every page, clean dry run); if hosted runners can't automate UI, add the flow to docs/release-checklist.md instead; verify one of the two lands
- [ ] 9.2 Accessibility Insights pass and keyboard-only walkthrough; verify zero automated failures
- [ ] 9.3 Measure cold start and memory; try Native AOT only if startup exceeds 1.5 s; verify numbers recorded in the PR
- [ ] 9.4 Run `winui-code-review` and the security review over the whole GUI; verify findings fixed or recorded
