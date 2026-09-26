# Duster GUI: final Windows verification

The Windows GUI (`gui/`, OpenSpec change `add-windows-gui`) was written on macOS. Everything that can run there has passed (Go engine tests, .NET protocol, lifecycle and ViewModel tests against the real engine). Nothing below has been run yet: WinUI compiles and runs only on Windows. Run this once, top to bottom, on a Windows 10 1809+ or Windows 11 PC, and record each result.

Nothing here deletes your files except where a step says so, and those steps use folders you create for the test.

## 0. Setup

- [ ] **1. Toolchain.** In PowerShell: `[Environment]::OSVersion.Version`, `$env:PROCESSOR_ARCHITECTURE`, `$PSVersionTable.PSVersion`, `dotnet --list-sdks` (needs a 10.0 SDK), `go version` (1.25+), `git --version`, `winapp --version` (optional, 0.6+). Developer Mode is not required (the app is unpackaged). Missing .NET 10: `winget install Microsoft.DotNet.SDK.10`, or run `/winui:winui-setup` in Claude Code.
- [ ] **2. Repository.** `git checkout feat/windows-gui`, `git status` clean apart from your own edits.

## 1. Automated gate

- [ ] **3. Smoke test.** From the repo root: `powershell -ExecutionPolicy Bypass -File gui\smoke.ps1` (ARM64 PC: add `-Platform ARM64`). It covers items 4-8 and 24-25 and prints a PASS summary. Save the full output.
  - 4. Restore: `dotnet` restores every project.
  - 5. Build and XAML compile: `Duster.App` builds with warnings as errors.
  - 6. Unit and integration tests: all `Duster.Tests` pass against the real `du.exe` (0 skipped expected on Windows, except the symlink test when Developer Mode is off and the `/bin/cat` test).
  - 7. Engine startup: `du.exe` starts as a child of `Duster.exe`, from `Duster.exe`'s own folder, and nothing else is started.
  - 8. Handshake: the child is still alive after 6 s (a failed handshake kills it).
  - 24. Shutdown: closing the window exits `Duster.exe` within 15 s.
  - 25. Orphans: no `du.exe` is left afterwards. Both checks run for the Debug build and for the single-file release layout.
- [ ] If the build fails, keep the full output (first error first) and fix only what it names. Do not turn off warnings as errors.

Start `%TEMP%\duster-smoke\release-layout\Duster.exe` for the manual part.

## 2. Manual checks

- [ ] **9. Home.** Footer reads `Engine <version>`. Device, Processor, Memory and one row per drive show real values (compare with Task Manager). Refresh reloads them. With `du.exe` renamed, the app shows "Engine not found" and Restart engine keeps failing cleanly; rename it back and Restart engine connects.
- [ ] **10. Clean scan.** Clean > Scan: progress moves, categories appear grouped (System Core, Web Browsers, ...), sizes are plausible, nothing is deleted (sizes unchanged on a second scan).
- [ ] **11. Clean selection.** Everything selectable starts checked; Prefetch shows a shield and cannot be checked; Select none / Select all work; the "n selected · size" line follows.
- [ ] **12. Clean confirmation.** Clean selected shows "Delete permanently?" naming the categories and size; Enter/Escape cancel; Cancel changes nothing.
- [ ] **13. Clean execution.** Select only safe categories (for example Temporary files, npm cache); confirm Delete. Each row shows "Freed ..." or a reason; a second scan shows them smaller. `%LOCALAPPDATA%\Duster\operations.log` has one entry per cleaned category.
- [ ] **14. Progress.** The bar advances per category during scan and clean; the window stays movable and resizable throughout.
- [ ] **15. Cancel.** Stop during a multi-category clean: the status starts "Canceled.", finished categories show what they freed, the rest show "Not cleaned". Cancel scan during a scan says "Scan canceled."
- [ ] **16. Administrator flow.** "Some categories need administrator rights" appears; Restart as administrator shows UAC. Choosing No keeps the app open with "Administrator restart was canceled." Choosing Yes relaunches elevated (footer: `· administrator`), Prefetch becomes selectable, and the old window's engine exits (Task Manager: one `du.exe`).
- [ ] **17. Restore.** Create something to restore: `mkdir C:\duster-test\proj\node_modules\x`, `Set-Content C:\duster-test\proj\package.json '{}'` (purge needs a project marker), then `du purge -p C:\duster-test --yes`. Restore lists the purge session; Restore all brings `node_modules` back; running it again reports a skip; purge again, then Delete for good (confirm) removes it; Empty quarantine asks first. `du restore` in a terminal agrees with the GUI at each step.
- [ ] **18. Analyze.** Browse picks a folder (also works in the elevated window); Scan shows progress and Cancel stops it; entries are largest first with share bars; opening a folder and the breadcrumb both work; Largest files and Changes switch views; a second scan of the same folder fills Changes. Move to Recycle Bin (confirm) on a test file sends it to the Recycle Bin and the listing and totals update; a file too big for the bin is reported as kept for 7 days and appears in Restore.
- [ ] **19. Navigation.** Home, Clean, Restore, Analyze all open; each page keeps its results when you leave and come back; Home's task buttons open the right page; keyboard only (Tab, arrows, Space, Enter) reaches every control.
- [ ] **20. Light theme**, **21. Dark theme**, **22. High Contrast** (Settings > Accessibility > Contrast themes). In each: text readable, no invisible controls, selection and focus visible, nothing clipped at 100% and 150% scaling, Mica falls back cleanly on Windows 10.
- [ ] **23. Accessibility Insights** (free, Microsoft): FastPass on each page reports no failures; Narrator reads the engine status, progress and status lines.

## 3. Packaging and install

- [ ] **26. Installer.** Build the release pieces on Windows: `make gui` (or `dotnet publish gui/Duster.App -c Release -p:Platform=x64 -o dist/gui-x64` and copy `Duster.exe` to `dist/duster-gui-windows-amd64.exe`), the Go binaries (`make build-amd64` or the release workflow's names), then `ISCC.exe /DMyAppVersion=<v> installer\duster-setup.iss`. Install: Start menu "Duster" opens the GUI, "Duster Command Line" opens the CLI; the optional "Open Duster" at the end starts the GUI **not** elevated (footer has no `· administrator`).
- [ ] **27. Upgrade path.** With an older Duster installed (1.3.0), install the new setup over it: the GUI appears and the CLI keeps working. With a portable install, `du update` (after a release that contains the GUI) installs `Duster.exe` beside `du.exe`, even while the GUI is open.
- [ ] **28. Uninstall.** Uninstall from Settings > Apps: `Duster.exe`, `du.exe`, `duw.exe` are gone, and so are the shortcuts. For a portable install, `du remove` removes `Duster.exe` too (delayed if the GUI is open).

## 4. Security and performance

- [ ] **29. Security.** While using every page, Process Explorer or `Get-CimInstance Win32_Process -Filter "ParentProcessId=<Duster pid>"` shows only `du.exe` (from Duster's folder), never `cmd.exe` or `powershell.exe`; the only UAC prompt is the one you asked for; replacing `du.exe` with a symbolic link (`mklink`, admin) makes the GUI refuse to start it; Clean with nothing confirmed deletes nothing.
- [ ] **30. Performance.** Cold start to Home showing data (stopwatch, or `Measure-Command { Start-Process ... }` plus the footer turning to Engine): record it; above 1.5 s, try Native AOT (task 7.3). Working set of `Duster.exe` idle on Home and after an Analyze of a large folder (Task Manager, Details). The UI never freezes during a scan of `C:\`.

## 5. Final

- [ ] **31. Final smoke.** After any fixes, run `gui\smoke.ps1` again (both architectures if you ship ARM64) and all of section 2 that the fixes touched.

Record: Windows version, architecture, .NET SDK, the smoke summary, and PASS/FAIL per item. Only after every item passes can Milestone 2-7 Windows verification be ticked in `openspec/changes/add-windows-gui/tasks.md`.
