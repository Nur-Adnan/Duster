## 1. Engine host (Go, milestone 1)

- [x] 1.1 Add hidden `du engine` command in cmd/engine.go: NDJSON loop, 1 MiB line cap, stdout reserved (D4), one encoder mutex; verify with cmd/engine_test.go driving it through io.Pipe (bad JSON, unknown method, id correlation)
- [x] 1.2 Implement `hello` (protocol 1 + Version) and `cancel`; verify handshake and cancel-unknown-id tests pass
- [x] 1.3 Implement per-request contexts, stdin-EOF shutdown, and the `busy` TryLock for state-changing methods; verify tests for concurrent read-only requests, a second destructive request getting `busy`, and EOF exiting cleanly
- [x] 1.4 Implement `status.get` (optional top processes) and `doctor.run`; verify results decode into the same structs `du status --json` and `du doctor --json` emit
- [x] 1.5 Implement `clean.scan` (categories in `cleanGroups` order, group, bytes, files, admin-blocked) and `clean.run` (engine-issued IDs only, progress events per category, cancel between categories, `admin_required` per-category error); verify with a test category rooted in t.TempDir() covering selected subset, unknown ID refused, cancel mid-run, and a symlink inside the root not traversed
- [x] 1.6 Run all gates (gofmt, vet, staticcheck, both GOOS) and `DU_NO_OPLOG=1 go test ./...`; update CLAUDE.md (engine row + layout)

## 2. GUI shell (milestone 2)

- [ ] 2.1 Create gui/Duster.slnx with Duster.App (WinUI 3, from the official winui-mvvm template, x64 + ARM64 only), Duster.Core (net10.0: protocol DTOs, `IEngineClient`, errors), Duster.Infrastructure (net10.0: engine path resolution, process + NDJSON client), Duster.Tests (MSTest); nullable + warnings as errors; verify `dotnet build` of Core/Infrastructure/Tests on macOS and of the whole solution on the PC
- [ ] 2.2 Infrastructure `EngineClient`: resolve du.exe beside the app (link refused), start with redirected stdio and no window, handshake, request/reply correlation, progress events, cancel, stderr drained, malformed lines tolerated, exit detection, clean shutdown (stdin close, then kill after a grace period); verify tests against an in-memory stream pair and against the real `du engine` binary
- [ ] 2.3 Shell: NavigationView (Home, Clean, Restore, Analyze) with page routing by type so later pages are one entry each, Mica, system theme, engine status in the footer, app-level error bar, AutomationIds; verify on the PC with `.\BuildAndRun.ps1` in light, dark, and high contrast
- [ ] 2.4 Home page: engine connection state, version, one `status.get` on load plus a Refresh button (no timer), links to Clean/Restore/Analyze, engine-missing and protocol-mismatch states; Clean/Restore/Analyze pages as shells with ViewModels; verify on the PC, including with du.exe renamed
- [ ] 2.5 Verify on the PC: `dotnet test`, the app launches, handshake shows connected, and closing the window leaves no `du.exe` running (`Get-Process du`)

## 3. Clean page (milestone 3)

- [ ] 3.1 Clean page: grouped categories with sizes from `clean.scan`, select/deselect, admin shield, confirm dialog stating deletion is permanent, progress, Cancel, results; verify on the PC against categories that are safe to empty, plus a ViewModel unit test with a fake `IEngineClient`
- [ ] 3.2 "Restart as administrator" relaunch; verify on the PC that UAC appears and prefetch becomes available

## 4. Restore page (milestone 4)

- [ ] 4.1 Engine: `restore.list`/`restore.run`/`restore.empty` over the existing quarantine sessions with engine-issued IDs; verify Go tests with `tempQuarantine` for a conflict skip, access denied, and a reparse point
- [ ] 4.2 Restore page; verify a `du purge` session appears in the GUI and restores (windows-smoke or a test folder on the PC)

## 5. Analyze (milestone 5)

- [ ] 5.1 Engine: `analyze.scan` with throttled progress and history changes (tests set LOCALAPPDATA to t.TempDir()); verify Go tests
- [ ] 5.2 Analyze page: virtualized folder tree, largest files, changes since last scan, send to Recycle Bin via engine; verify on the PC with a large folder that the UI stays responsive

## 6. Packaging (milestone 6)

- [ ] 6.1 Publish Duster.exe self-contained for win-x64 and win-arm64 in release.yml, Makefile, and build-release.sh; add to the Inno installer (Start menu shortcut) and zips; CI job `gui` (windows-latest, windows-11-arm) plus Core/Infrastructure tests on ubuntu; verify the release dry run produces both and checksums cover them
- [ ] 6.2 `du update` installs Duster.exe too (extend `releaseBinaries`) and `du remove` removes it; verify update and remove tests and a windows-smoke install/uninstall

## 7. Hardening (milestone 7)

- [ ] 7.1 FlaUI smoke test in windows-smoke (launch, visit every page, clean scan only); if hosted runners can't automate UI, add the flow to docs/release-checklist.md instead; verify one of the two lands
- [ ] 7.2 Accessibility Insights pass and keyboard-only walkthrough; verify zero automated failures
- [ ] 7.3 Measure cold start and memory; try Native AOT only if startup exceeds 1.5 s; verify numbers recorded in the PR
- [ ] 7.4 Run `winui-code-review` and the security review over the whole GUI; verify findings fixed or recorded
