# Changelog

## [Unreleased]

### Changed
- The clean screen (`du clean` in a terminal, or Clean in the `du` menu) now lists all 34 categories that `du clean --yes` cleans, in the same order and all ticked, instead of 9. **Check the list before pressing Enter:** besides the old 9, it now also clears developer caches (npm, NuGet, Gradle and others), app caches (Teams, Spotify, Discord and others), Recent shortcuts and memory dumps. Untick anything you want to keep. The list scrolls when it is taller than the window, and a new line shows how many categories are selected.
- The clean screen uses the command's category names. "Logs (System & Apps)" is now two rows, "Windows Error Reports" and "System Log Files", so `--whitelist wer` there protects only error reports, as it does in the command. `--whitelist logs` still protects both.
- Releases are signed through SignPath Foundation once the project's application is approved ([docs/code-signing.md](docs/code-signing.md)). The release fails if an exe or the setup exe comes out without a valid, timestamped signature. The old signing step that expected a certificate file is gone: certificate authorities no longer issue code signing keys as files, so it could never have been used.

## [1.1.0] - 2026-09-14

### Added
- `du status` shows the CPU temperature in the CPU panel, marked Normal, Warm (70°C and up) or Hot (85°C and up), and `du status --json` adds `CPUTempC`. It reads the hottest ACPI thermal zone through Windows performance counters, so it needs no admin rights and works on non-English Windows. Most desktops and virtual machines expose no thermal zone: they show N/A and the JSON field is left out. The firmware's thermal zone can lag behind or read lower than the CPU's own sensor.

## [1.0.6] - 2026-09-11

### Fixed
- `du uninstall` now waits for the whole uninstaller. Inno Setup and NSIS uninstallers, which many apps use, start a copy of themselves and exit at once. Duster checked too early, so it showed "UNINSTALL NOT CONFIRMED" and skipped the leftover scan while the app's own "Are you sure?" dialog was still open. It now waits until every process the uninstaller started has exited, or until the app is gone.
- A finished dry run in the clean screen said "System cache cleaned successfully!" and counted "Total files removed", although nothing was deleted. It now says "Dry run complete: nothing was deleted." and labels the totals as space and files to remove.
- Screens no longer push their content about 70 columns to the right. A styled divider with a line break inside ended in a line of padding spaces, and the next line of text continued after it: the boxes on the remove, installer, purge, optimize, update and uninstall screens started at column 73 (mostly cut off in a 120-column terminal), and verify showed "Integrity Status: SECURE" instead of "SECURED & CERTIFIED". Doctor and benchmark had the same fault.
- `du analyze` and `du purge` now fail with exit code 1 on a path that doesn't exist (and `purge` on a path that isn't a folder). A mistyped path used to print an empty result and exit 0, as if the folder had nothing in it.
- `du optimize --json --yes` now exits 1 when a task fails. It reported the failure in its JSON but exited 0, so scripts couldn't tell a failed DNS flush or TRIM from success.
- `install.ps1` and `uninstall.ps1` no longer rewrite your user PATH. They read it expanded and saved it back as a plain string (REG_SZ), which froze every `%VAR%` entry, including Windows' own `%USERPROFILE%\AppData\Local\Microsoft\WindowsApps`, into a fixed path. They now keep the value exactly as stored, as `REG_EXPAND_SZ`, like the setup exe does.
- Dry runs of the uninstall leftover sweep and the installer sweep no longer write "success" entries to `operations.log` for deletions that never happened.
- `du purge --dry-run --yes` no longer prints "SUCCESS" and "Purged N / N directories"; it says what would be purged and that nothing was deleted.
- The Chocolatey package (`scripts/manifests/duster.nuspec`) pointed at a `tools/` folder that didn't exist, so it couldn't be built. It now has an install script that downloads the release and checks its SHA-256, and CI builds, installs and uninstalls it.

### Added
- Every release file gets a signed build-provenance attestation (keyless, via GitHub and Sigstore). `gh attestation verify <file> --repo Nur-Adnan/Duster` proves a download was built by the release workflow from the tagged commit; a file swapped by hand on the release page fails it.
- The Windows Smoke Test now does the checks that needed a person at a desktop. It types keys into Duster in a real Windows terminal (a pseudo console), answers Windows' own dialogs, uninstalls a real Inno Setup app, runs install.ps1's elevation path, and upgrades a v1.0.2 install by reinstalling. It also checks that wrong input fails cleanly, that nothing is deleted without `--yes` or with `--dry-run`, and that repeated and concurrent runs work; CI checks that installing and uninstalling keep the user PATH's `%VAR%` entries and registry type.

## [1.0.5] - 2026-09-11

### Fixed
- `install.cmd` now works when the install folder contains a space or an apostrophe; it used to split `C:\Users\John Smith\...` into two arguments. It also no longer passes `-InstallDir` when you didn't give `--dir`, which had disabled install.ps1's Program Files fallback on PCs with WDAC or AppLocker policies.
- Uninstalling the setup exe no longer deletes all of `%LOCALAPPDATA%\Duster`. That folder is also where the PowerShell installer puts `du.exe`, so uninstalling one install method wiped the other. It now removes only Duster's operation logs.
- JetBrains cache cleaning now goes through the same safety checks as every other category (protected paths, linked roots, per-file checks), and it reports files it couldn't delete.
- `du clean --whitelist` now accepts the same names in the CLI as in the interactive screen (`chrome`, `edge`, `brave`, `firefox`, `logs`), and warns about names it doesn't recognize. It used to protect nothing, silently.
- The reclaimable total no longer counts the Yarn cache twice, or counts the Office clipboard temp folder on top of `%TEMP%`. The misnamed "Installer Patch Cache" category is gone.
- The "Recent Items" category no longer claims to clean jump lists. It never matched them, and they hold your pinned items.
- Categories under the Windows folder now follow Windows to whatever drive it's installed on, instead of assuming `C:`.
- OneDrive placeholder detection now also catches "recall on open" files, which Windows reports only in directory listings.

### Performance
- Clean scans read each file's attributes from the directory listing instead of asking Windows again for every file.
- Walks expand an 8.3 short root (such as `C:\Users\JOHNSM~1\...`) once, so the safety check on each file no longer does a disk lookup.
- The interactive clean now starts deleting each item right away, instead of first playing a progress animation that took about 3.5 s per run.

### Build and CI
- Release builds use the Inno Setup preinstalled on GitHub's Windows images instead of an unpinned Chocolatey package. CI now also builds the setup exe and installs and uninstalls it, and runs `install.cmd` into a path with a space and an apostrophe.

## [1.0.4] - 2026-09-11

### Fixed
- `uninstall` can now run batch-file uninstallers (`.bat`, `.cmd`) whose registered arguments contain quotes. Windows' implicit `cmd /c` stripped the outer quotes, so the uninstaller never started. This affected 1.0.2 and 1.0.3.
- `du status --json` now lists the busiest processes. It always returned an empty list, because it measured CPU use against an earlier call that a one-shot run never makes.

### Performance
- `du status` and the landing screen no longer walk every running process on each refresh (every second, and every half second on the landing screen) to build a list neither of them shows.

## [1.0.3] - 2026-09-11

### Upgrading from 1.0.2
`du update` in 1.0.2 cannot install new releases. Reinstall once with the install command in the README; after that, `du update` works.

### Security
- Protected-path checks now resolve paths the way Windows does. `\??\` and `\\?\` prefixes, trailing dots and spaces, `..`, and admin shares (`\\host\c$`) no longer get past them. Device and volume paths and alternate data streams are always refused. Program Files is protected on every drive.
- `clean` skips a category folder that is itself a symlink or junction.
- `uninstall` looks up bare uninstaller names (MsiExec, RunDll32) in System32 instead of PATH. It refuses to run a per-user (HKCU) uninstaller while Duster is elevated.
- `update` installs in headless mode only with `--yes`. It never offers a pre-release to someone on a stable version, and never treats an unparseable tag as newer.
- install.ps1: the admin re-launch for Program Files installs no longer runs a script from a temp file, which a non-admin process could swap before it runs. It also no longer re-runs, as admin, a script that pipes the installer into `iex`.
- Releases are built with Go 1.26, because the Go 1.25 toolchain that built 1.0.2 had known standard-library vulnerabilities. CI runs govulncheck. The release job uses least-privilege tokens and validates tags, and pre-releases are never marked latest.

### Fixed
- `uninstall` sweeps leftovers only after the uninstaller succeeds and the app is really gone. MSI reboot-required exit codes count as success. The sweep matches exact app folder names and starts with nothing selected.
- `uninstall` passes the registered uninstall arguments through unchanged, so InstallShield and rundll32 uninstallers work again.
- `clean` reports items it could not delete instead of claiming success, and counts space that was partly freed.
- `clean` opened with `--dry-run` can no longer start a real clean from the TUI.
- `purge` only flags `node_modules`, `.gradle` and `.m2` folders that sit next to their project file.
- `installer` scans only the top level of Downloads, skips OneDrive placeholders, and clamps `--min-size`.
- `remove` never falls back to the current directory, keeps the running binary while it clears the Duster folder, and exits 1 if the uninstall is incomplete. It also no longer writes its log entry back into the folder it has just deleted.
- Deleting to the Recycle Bin now warns before permanently deleting an item that is too large for the bin.
- `optimize` reports only the space it actually freed, and runs system tools by absolute path.
- install.ps1: the admin install works when the script is piped (`irm | iex`) and with paths that contain spaces. Adding and removing the PATH entry now matches whole entries only.
- The startup view on the landing screen asks before removing disabled entries, and reports any it could not remove.
- `doctor --json` and `verify --json` exit with code 1 when a check fails.

### Performance
- `analyze` is about 19% faster and uses about 41% less memory on 250k files, and moving between folders no longer rescans the disk. Measured on macOS.
- Recursive deletes are about 30% faster, with about 65% fewer allocations.
