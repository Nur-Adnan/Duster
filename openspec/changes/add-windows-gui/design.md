## Context

See proposal.md for why. The engine functions the GUI needs already take plain inputs and return structs: `runCategory`/`getCategories`/`cleanGroups` (cmd/clean.go), `scanDirectory` with a progress channel and `recordScanHistory` (analyze), `scanArtifacts` with a callback and `purgeOne` (purge.go), `quarantinePath`/`recycleOrQuarantine` and restore sessions (quarantine.go, restore.go), `optimizeTasks`/`execOptimizeTask`/`buildReclaimItems`, `compactVirtualDisk(ctx, ...)`, `runDoctorDiagnostics`, `sysinfo.GetSystemStats`, `uninstall.GetInstalledApps`, `scanAppLeftovers`, `runTree`, `parseRunArgs`. Two gaps: the uninstall run and leftover sweep live inside `uninstallModel.Update` (uninstall.go), and only optimize and vdisk take a `context.Context`.

WinUI 3 does not build on macOS or Linux (XamlCompiler is a .NET Framework binary), so the GUI builds on Windows only; the protocol client must be testable anywhere.

## Goals / Non-Goals

**Goals:**
- One engine, one set of safety checks, two front ends.
- A GUI that never blocks its UI thread and survives an engine crash.
- The protocol client can be unit-tested on any OS.

**Non-Goals:**
- A public or stable API for third parties. The protocol is private between two binaries shipped together.
- Moving engine code from `cmd/` to `lib/` (a large refactor with no behavior gain; see Decisions).

## Decisions

### D1. WinUI 3 + Windows App SDK 2.x, C# on .NET 10 LTS
Microsoft's current native UI stack, MIT, builds with `dotnet` + `winapp` (no Visual Studio). .NET 10 is supported to 2028-11-14. Alternatives: Wails (Go + WebView2) keeps one language but renders web UI; Fyne needs CGO (Duster is CGO_ENABLED=0); Gio is not native-looking and weak on accessibility; Avalonia/Uno solve cross-platform, which Duster does not need; WPF is not the forward path for new Windows apps.

### D2. The engine stays in Go; the GUI spawns `du.exe engine` (justified process spawn)
The project prefers Win32 over spawning processes. This spawn is the exception because the alternative is a second implementation of every delete path in C# or a CGO c-shared DLL (which needs CGO). The child is our own binary, found by absolute path next to `Duster.exe`, started without a shell, with `CREATE_NO_WINDOW`, and refused if it is a link, the same pattern `launcher/duw` uses.

### D3. NDJSON over the child's stdin/stdout
Anonymous pipes created by the parent: no endpoint another process can open, no pipe name to squat, no DACL to get wrong, no localhost port for a browser to reach. Named pipes would need `go-winio` or hand-written `CreateNamedPipe` with an SDDL; localhost HTTP would need a token and origin checks. Framing: one JSON object per line, requests `{id, method, params}`, final reply `{id, result}` or `{id, error:{code, message, data}}`, events `{id, event, data}`. Max line 1 MiB. Protocol version 1, returned by `hello`.

### D4. Stdout is reserved for the protocol
At start the engine keeps the original stdout for protocol writes and points `os.Stdout` at stderr, so a stray `fmt.Print` inside an engine function cannot corrupt the stream. All writes go through one mutex-guarded encoder.

### D5. Concurrency and cancellation
Each request runs in its own goroutine with a context derived from the process context. State-changing methods take a single `sync.Mutex` with `TryLock`; failure replies `busy`. `cancel` cancels the target's context; loops check `ctx.Err()` between items (between categories in clean, between artifacts in purge), never inside a single move or delete. Stdin EOF cancels the process context, waits for in-flight requests, and exits 0.

### D6. Targets are engine-issued
`clean.scan` stores the category IDs it returned; `clean.run` accepts only those. Later scans (purge, installers, leftovers) store their items under opaque IDs for the process lifetime; destructive calls take IDs, never paths. All existing path checks still run. The GUI is treated as untrusted input.

### D7. Where the host lives
`cmd/engine.go` (package cmd), because the engine functions are unexported in package cmd. A hidden cobra command (`Hidden: true`) keeps it out of `du --help`. No Windows-only code is added to the host itself; methods call functions that already have `stubs.go` coverage, so `go test ./cmd` runs the protocol tests on macOS/Linux.

### D8. Unpackaged, self-contained GUI in the existing installer
Packaged (MSIX) apps and the processes they spawn get AppData write virtualization, which would split the GUI's quarantine and logs from the CLI's `%LOCALAPPDATA%\Duster`, and sideloaded MSIX needs a certificate the user's machine trusts. So `Duster.exe` is published unpackaged and self-contained (`WindowsPackageType=None`, `WindowsAppSDKSelfContained=true`, `SelfContained=true`) for `win-x64` and `win-arm64`, and ships in the Inno installer and zips beside `du.exe`. This is a distribution choice made up front, not a workaround for a launch failure.

### D9. GUI layering
- `Duster.App` (WinUI): Views, ViewModels (CommunityToolkit.Mvvm partial-property `[ObservableProperty]`, `[RelayCommand]`), navigation, startup, window, theme. ViewModels see only `IEngineClient`; they never touch `Process`.
- `Duster.Core` (net10.0, no WinUI, no I/O): protocol DTOs, `IEngineClient`, `EngineState`, `EngineException` with a typed error kind.
- `Duster.Infrastructure` (net10.0): engine path resolution, process lifecycle, NDJSON framing and correlation, cancellation, `System.Text.Json` source-generated context.
- `Duster.Tests` (MSTest): protocol, lifecycle, and path tests; runs on any OS, and against the real `du engine` when one is built.
Microsoft.Extensions.DependencyInjection wires the few services. `IEngineClient` exists for one reason: ViewModel tests substitute a fake. No domain layer: the domain is the Go engine.

### D10. Elevation by relaunch
Unelevated by default (`asInvoker`). Admin actions offer "Restart as administrator": `ShellExecute` with `runas` on `Duster.exe`, then exit; the new GUI starts an elevated engine. No elevated helper, no service.

## Risks / Trade-offs

- [Engine and GUI versions drift in a partial update] → handshake refuses a protocol mismatch; `du update` installs both binaries from one release.
- [A long category clean can't be cancelled mid-category] → cancel checks between categories; per-item checks are future work, needed before Purge ships.
- [Hosted Windows runners may not allow UI automation] → FlaUI smoke is attempted in windows-smoke; fallback is a scripted manual checklist in docs/release-checklist.md.
- [Self-contained GUI adds roughly 60-100 MB to the installer (unmeasured)] → measure in the packaging milestone; trimming or Native AOT (supported since Windows App SDK 1.6) only if the size or startup numbers justify it.
- [Inno Setup is free only under $5,000/yr revenue including donations] → recorded; revisit if donations approach it.
- [WinUI builds only on Windows] → `Duster.Core` holds all non-UI logic and its tests run on any OS; the UI project builds in CI on every PR.

## Scope: V1, deferred, and CLI-only

The GUI is not meant to mirror every CLI command. V1 is frozen at four pages; anything below marked Deferred may get a page later, CLI-only means no GUI is planned.

**1. V1 GUI:** Home, Clean, Restore, Analyze.

| CLI feature | GUI equivalent | Status | Difference |
|---|---|---|---|
| `clean` (scan, categories, admin-only `prefetch`) | Clean page | Covered | Cancel stops between categories, not inside one (same as the engine's `clean.run`) |
| `clean --dry-run`, `--yes` | Scan is the preview; the confirm dialog is the `--yes` | Covered | |
| `clean --whitelist` | Uncheck categories | Covered | Per run in both |
| `restore` list, `[n\|id]`, `--item`, `--empty` | Restore page | Covered | |
| `restore` 7-day cleanup | None | Partial (intentional) | `du restore` deletes expired sessions before listing; the GUI's `restore.list` stays read-only, so it can show a session past 7 days until a CLI run, purge, installer sweep or scheduled clean applies retention |
| `restore --dry-run` | None | Deferred | A GUI restore never overwrites and reports skips |
| `analyze` scan, drill-down, largest files, changes, `d` | Analyze page (Move to Recycle Bin) | Covered | |
| `analyze --since` | Changes compares with the previous scan only | Deferred | |
| `analyze --no-history` | None: the GUI always records scan history | Deferred | |
| `analyze` Enter on a change jumps to its folder | Changes list is not clickable | Deferred | |
| `status` | Home snapshot: host, Windows version, CPU % and model, memory, drives, Refresh | Partial | No live refresh, temperature, disk I/O, network, battery, uptime, health score or top processes |
| `doctor` | None | Deferred | `doctor.run` exists in the engine and `RunDoctorAsync` in the client; no screen shows it |

**2. Deferred GUI features** (each is an engine method set plus one page or control; D3-D7 already cover them): Doctor; full live Status dashboard; `restore --dry-run`; `analyze --since` and `--no-history`; opening a folder from an Analyze change; Purge; Installers; Uninstall (needs the run and leftover sweep extracted from `uninstallModel.Update` first); Optimize with the reclaim report; Virtual disks; Schedule; a Settings page; an update flow (`du update` already installs `Duster.exe`); a remove flow; cancelling partway through a single category (needed before Purge ships).

**3. CLI-only features** (no GUI planned): the landing menu's Drivers, Network, Security and Startup views; `benchmark`; `verify`; the hidden `engine` command itself. Installed copies are removed by the setup uninstaller, portable ones by `du remove`.

**4. CLI-only flags and modes:** `--json` and piped headless output, `--yes` for unattended runs, `--debug`, `du schedule run` (the scheduled task's entry point), and `update --check`/`--force`.

## Migration Plan

Additive. The CLI, TUI, scheduled task, and JSON outputs are unchanged. Rollback: remove `Duster.exe` from packaging; `du engine` is hidden and unused by the CLI.
