# Duster GUI ↔ CLI parity

Source of truth: the CLI in `cmd/` and `main.go`. Machine-readable inventory: `openspec/changes/expand-windows-gui-cli-parity/cli-inventory.json`. OpenSpec change: `expand-windows-gui-cli-parity`.

**State:** implemented and macOS-tested. The GUI pages build and run only on Windows, so every row is **Windows verification pending** (docs/gui-windows-verification.md).

Statuses: **FULL** (same capability in the GUI), **GUI-NATIVE-EQUIVALENT** (same outcome through a GUI control instead of the CLI mechanism), **CLI-ONLY** (scripting or terminal mode with no user value in a window), **WINDOWS-ONLY** (Windows' own UI does it), **NOT-APPLICABLE** (internal; nothing to expose), **PARTIAL** (some of it; none left).

Every GUI action runs in the Go engine (`du engine`) through the same functions the CLI uses. The GUI never deletes, moves, or runs anything itself.

## Commands

| CLI | GUI location | GUI equivalent | Status | Admin | Engine method | Difference / reason |
|---|---|---|---|---|---|---|
| `du` (landing menu) | Navigation pane | Groups: Overview, Cleanup, Storage, Recovery, System, Automation, Settings | GUI-NATIVE-EQUIVALENT | – | – | A window's navigation replaces a numbered menu |
| `du --version` | Settings > About | Duster and engine versions | FULL | – | `hello` | |
| `du --help` | – | Labels on every page | CLI-ONLY | – | – | Terminal help text |
| `status` | Home | Dashboard: host, Windows, CPU % and per core, temperature, memory, drives, disk I/O, network, battery, uptime, health; Live toggle; Top processes on request | FULL | – | `status.get` | Live refresh every 2 s only while Home is open and Live is on; top processes take a 1 s sample, so they load on request |
| landing Network | Home > Network | Live download/upload rate | GUI-NATIVE-EQUIVALENT | – | `status.get` | Same numbers as the landing view, shown on the dashboard |
| `doctor` | Diagnostics > Health check | Checks with status, explanation and message; Restart as administrator for the privilege check | FULL | – | `doctor.run` | Doctor has no repair actions; the GUI adds none |
| `verify` | Diagnostics > Self-test | 8 self-tests, pass/fail and details | FULL | – | `verify.run` | |
| `benchmark` | Diagnostics > Benchmark | Scan, write, delete, JSON rates, memory, CPU | FULL | – | `benchmark.run` | Runs only when asked |
| `clean` | Cleanup > Clean | Scan, grouped categories, confirm, progress, Stop, per-category result | FULL | prefetch | `clean.scan`, `clean.run` | Stop takes effect between categories, as in the engine |
| clean TUI `v` | Recovery > Restore > Activity log | Recent operations log entries | FULL | – | `oplog.list` | |
| `purge` | Cleanup > Developer artifacts | Folder, scan, select, mode (keep 7 days / Recycle Bin / permanent), confirm, progress, Stop | FULL | – | `purge.scan`, `purge.run` | Stop takes effect between items |
| `installer` | Cleanup > Old installers | Minimum size, scan, select, confirm (kept 7 days) | FULL | – | `installer.scan`, `installer.run` | |
| `uninstall` | Cleanup > Apps | Search, run the app's own uninstaller, then pick leftovers (none preselected) to keep 7 days | FULL | HKLM apps prompt through their own uninstaller; HKCU apps refused while elevated | `uninstall.list`, `uninstall.run`, `uninstall.sweep` | Protected system apps are listed but cannot be uninstalled, as in the TUI |
| `analyze` | Storage > Analyze | Scan, drill-down, breadcrumb, Largest files, Changes (click to open), Show in Explorer, Move to Recycle Bin | FULL | – | `analyze.scan`, `analyze.children`, `analyze.recycle`, `analyze.reveal` | |
| `vdisk` | Storage > Virtual disks | Disks with kind, size, on-disk, estimate, block reason; compact selected with confirm, progress, Stop | FULL | compaction | `vdisk.scan`, `vdisk.run` | Stop detaches the disk, as the TUI's quit does |
| `restore` | Recovery > Restore | Sessions, items, Restore all / item, Preview, Delete for good, Empty quarantine | FULL | – | `restore.list`, `restore.run`, `restore.empty` | Listing applies the same 7-day expiry as `du restore` |
| `optimize` | System > Optimize | Tasks with admin shields, Preview, Run with confirm, progress, Stop; reclaim report (Windows.old, hibernation file, with Windows' own advice) | FULL | TRIM, component store | `optimize.list`, `optimize.run` | Windows.old and hiberfil.sys are reported, never removed (as in the CLI) |
| landing Startup | System > Startup apps | Enable/disable (reversible), remove disabled entries with confirm | FULL | HKLM entries | `startup.list`, `startup.toggle`, `startup.remove` | Only disabled entries can be removed, as in the TUI |
| landing Security | System > Security | Five checks and a score | FULL | – | `security.run` | |
| landing Drivers | System > Drivers | Driver list with signing state | FULL | – | `drivers.list` | The engine runs the CLI's PowerShell query, never the GUI |
| `schedule` / `schedule status` | Automation > Schedule | On/off, cadence, time, next check, last check, last clean, warnings | FULL | – | `schedule.get` | One task per account, as in the CLI |
| `schedule on` | Automation > Schedule | Turn on / Save changes | FULL | – | `schedule.set` | |
| `schedule off` | Automation > Schedule | Turn off (confirm) | FULL | – | `schedule.off` | |
| `schedule run` (hidden) | – | – | NOT-APPLICABLE | – | – | The task's own entry point; cleaning by hand is the Clean page |
| `update` | Settings > Updates | Check, install with confirm, restart Duster | FULL | – | `update.check`, `update.install` | Same release source and mandatory SHA-256 |
| `remove` | Settings > Remove Duster | Plan, confirm, remove, exit | FULL | – | `remove.plan`, `remove.run` | A setup install is better removed from Windows Settings > Apps, which also clears its entry there; the page says so and links to it |
| setup uninstall | Windows Settings > Apps | Inno uninstaller | WINDOWS-ONLY | – | – | Windows' own app management |
| `engine` (hidden) | – | – | NOT-APPLICABLE | – | – | The GUI's transport |

## Flags

| Flag | Class | GUI | Status |
|---|---|---|---|
| `--json` on analyze, installer, optimize, purge, remove, restore, schedule, uninstall, update, vdisk | scripting | the page shows the same data | CLI-ONLY |
| `--json` on status, doctor, verify, benchmark | GUI-native | Copy report (text) | GUI-NATIVE-EQUIVALENT |
| `--yes`/`-y`, `remove --force`, `restore --yes` | GUI-native | the confirm dialog (default button: Cancel) | GUI-NATIVE-EQUIVALENT |
| `clean --dry-run`, `installer --dry-run`, `purge --dry-run`, `vdisk --dry-run`, `remove --dry-run`, `uninstall --dry-run` | GUI-native | the scan/plan shown before the confirm is the preview; nothing changes without it | GUI-NATIVE-EQUIVALENT |
| `optimize --dry-run`, `restore --dry-run`, `schedule on --dry-run` | user-facing | Preview button | FULL |
| `clean --whitelist` | GUI-native | uncheck categories (aliases map to the same categories) | GUI-NATIVE-EQUIVALENT |
| `clean --debug` | scripting | none: a verbose terminal log | CLI-ONLY |
| `analyze --no-history` | user-facing | "Don't save this scan" | FULL |
| `analyze --since` | user-facing | "Compare with" (editable; the engine parses it like the CLI) | FULL |
| `installer --min-size` | user-facing | Minimum size (MB) | FULL |
| `optimize --deep` | GUI-native | component store task, unchecked by default, admin shield | GUI-NATIVE-EQUIVALENT |
| `purge --path`, `--safe`, `--permanent` | user-facing | folder box, Recycle Bin option, Delete permanently option | FULL |
| `restore --item`, `--empty` | user-facing | Restore on an item, Delete for good | FULL |
| `schedule on --every`, `--at`, `--low-space`, `--add` | user-facing | How often, Check time, Clean early below, Also clean | FULL |
| `schedule off --uninstall` | internal | none: the setup uninstaller's sweep | CLI-ONLY |
| `update --check` | GUI-native | Check for updates | GUI-NATIVE-EQUIVALENT |
| `update --force` | user-facing | Reinstall this version | FULL |
| exit codes, piped headless output | scripting | each page shows failures | CLI-ONLY |
| `DU_NO_OPLOG` | test-only | none | CLI-ONLY |

## Result

- **Fully covered:** status, doctor, verify, benchmark, clean, purge, installer, uninstall, analyze (with `--since`, `--no-history`, jump, Explorer), vdisk, restore (with `--dry-run` and expiry), optimize (with preview and reclaim report), schedule status/on/off (with preview), update (with reinstall), remove, Drivers, Startup, Security, the clean operations log, `--version`.
- **GUI-native equivalents:** the landing menu, Network, `--yes`/`--force`, the scan-as-preview dry runs, `--whitelist`, `--deep`, `update --check`, Copy report for the diagnostic `--json` outputs.
- **Windows' own UI:** removing a setup install (Settings > Apps).
- **CLI-only by design:** `--json` on the data commands, `--debug`, `--help`, exit codes, piped output, `schedule off --uninstall`, `DU_NO_OPLOG`.
- **Not applicable:** `engine`, `schedule run`, environment variables.
- **Unavailable:** none.
