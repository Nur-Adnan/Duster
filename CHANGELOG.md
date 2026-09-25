# Changelog

## [Unreleased]

## [1.3.0] - 2026-09-25

### Added
- `du restore` gives every user-facing delete an undo window. `du purge`, the uninstall leftover sweep and the old-installer sweep now keep what they remove in a Duster quarantine on the same drive instead of deleting it for good, so keeping something costs no copy. `du restore` lists what's kept (time, command, item count, size, expiry), `du restore <n>` brings a whole session back and `du restore <n> --item <k>` brings back one item (`<n>` can also be the session id `du restore --json` prints, which never shifts); `du restore --empty [<n>]` names each session it would delete for good (command, time, item count, size) before asking; `--dry-run` shows what would happen without changing anything, and `--json` works with every form. A restore never overwrites: if something new already exists at the original path, that item is skipped and reported, and stays kept. Kept items still take up their space until they are swept away after 7 days, or sooner and oldest first when their drive drops below 10% free, on another keeping command's next run or `du schedule run`; that early removal is reported in one line (and as `swept_low_space` in `du purge --json --yes`). `du restore` itself only removes sessions past their 7 days, and says how many, so it never removes the session you came to restore to make room. `du purge --permanent` deletes for good right away, for when the space is needed now. Summaries report kept space apart from freed space: purge says "Kept X for 7 days" rather than "reclaimed", `du purge --json --yes` reports `kept_bytes` next to `reclaimed` (bytes deleted for good), `failed` and `errors`, and the uninstall and installer screens count items that could not be kept and were left in place. `du purge --json --yes` now acts (keeps the items, or with `--permanent` or `--safe` deletes or recycles them) and exits 1 if any item failed, while `--json` alone still only previews. On the drive holding `%LOCALAPPDATA%`, kept items live under `%LOCALAPPDATA%\Duster\quarantine`; on any other drive, under a hidden, per-user folder that only that user (and SYSTEM) can list, so a second standard user cannot see what you have kept. An administrator can still take ownership of it, as with any folder on Windows. `du remove` empties every quarantine it can reach as part of removing Duster, and `analyze d` and `purge --safe` still try the Recycle Bin first, falling back to the quarantine only when the bin won't take an item.
- `du schedule` cleans caches automatically, so you don't have to remember Duster exists. `du schedule on` registers a daily Task Scheduler check (`--at`, default 19:00) that cleans on your chosen interval (`--every daily|weekly|monthly`, default weekly) or early when the system drive runs low on free space (`--low-space`, default 10%, or `off`). It always cleans the safe set that rebuilds itself and holds nothing you made: temp files, browser caches (never cookies, history or sessions), thumbnails, error reports, crash dumps and GPU shader caches. `--add npm,gradle,docker` (and pnpm, yarn, bun, pip, cargo, nuget, docker, vscode, jetbrains, discord, slack, teams, steam, epic, adobe) opts in, since the next build or app launch just downloads it again. The Recycle Bin (it's how you undo a delete), Recent files, Spotify's offline downloads, the DNS cache (flushing it frees no space) and anything needing administrator rights (Windows Update cache, prefetch, Delivery Optimization, memory dumps, log files) are refused, each with the reason; the font cache is held by a Windows service during a session, so clean it by hand with `du clean` instead. It never runs elevated, skips a run while the PC is on battery power, and catches up on a check it missed while asleep once it wakes. Every run happens silently through a separate windowless launcher, `duw.exe`, so no console or Windows Terminal window ever appears; its output goes to `%LOCALAPPDATA%\Duster\schedule.log`, and the last check and last clean are kept in `schedule.json`. `du schedule` (or `status`) shows whether it's on, what it cleans, the next check and what the last run did; `du schedule off` deletes the task and keeps that history; `on --dry-run` shows what would be registered and what a run would clean right now without changing anything; `status`, `on` and `off` all support `--json`. The setup exe's uninstaller and `du remove` keep `duw.exe` and this task in sync. An existing install's first update to this release still runs its old updater, which installs only `du.exe`; running `du update --force` once afterwards runs the new updater, which also installs `duw.exe`, so `du schedule on` picks it up.
- `du analyze` now tells you what changed since the last time you scanned the same folder, the question a disk analyzer is usually opened for. Under the totals, a new line reads for example `Change: +6.89 GB since Sep 7, 2026 14:42 (17 days ago)`. Press `c` to see what is behind it: the most specific folders and files that explain the change, such as `Downloads\ubuntu-24.04.iso +5.66 GB new`, `AppData\Local\Temp +52 MB across smaller items` or `Videos\old -900 MB gone`, biggest growth first. The lines add up to the total, and a new folder is one line rather than a list of everything in it. Enter jumps to the item with the cursor on it, so `d` then sends it to the Recycle Bin. `--since 7d` (or `24h`, `2w`) compares with an older scan instead of the previous one, and says so if none is that old yet. `du analyze --json` includes the same explanation as `changes`, which is `null` on a folder's first scan.
- Each scan keeps a small snapshot (a few hundred KB even for a whole drive) of the folder's largest folders and files in `%LOCALAPPDATA%\Duster\history`: names and sizes only, never contents. Up to 10 are kept per folder, thinned so that recent history stays detailed and the oldest scan is always kept for long comparisons. `--no-history` neither reads nor writes one, and `du remove` deletes them with the rest of Duster's data. Duster will not compare a different disk that now has the same drive letter, and it warns when one scan ran as administrator and the other did not, since folders only an administrator can read would otherwise look like growth or shrinkage.
- `du vdisk` shrinks the virtual disks that WSL 2 and Docker Desktop grow on developer machines. Both keep their Linux file system in a `.vhdx` that expands as you work and never shrinks when you delete, which commonly costs 20 to 100 GB without showing up anywhere. Duster finds every one of them (registered WSL distributions plus Docker Desktop's own disks, whichever layout your version uses), shows what each takes on disk against what the guest actually uses, and compacts them in place with `diskpart`. Nothing inside a disk is read, changed or deleted. Compacting needs administrator rights and runs `wsl --shutdown` first, so every running distribution, shell and container stops and unsaved work in them is lost: the screen says so and waits for you. Quit Docker Desktop too, or its disk stays locked and is skipped. A disk that Windows already shrinks by itself (a sparse one), or that is NTFS-compressed or EFS-encrypted, is reported as such and left alone rather than handed to a `diskpart` run that would take minutes and then refuse it. Stopping part-way takes two keypresses and detaches the disk before exiting, so WSL always starts again afterwards. `du vdisk --dry-run` and `du vdisk --json` (without `--yes`) report only.
- `du vdisk` also reports, without ever applying them, the two settings that stop the disks growing back: `docker system prune -a` to free space inside Docker's disk first, and `sparseVhd=true` under `[experimental]` in `%UserProfile%\.wslconfig`. WSL itself gates sparse mode behind `--allow-unsafe` and documents it as a potential data-corruption risk, so it stays your decision.
- `du optimize` now reports the two biggest things a cleaner cannot remove for you: a previous Windows installation (`Windows.old`, often 10 to 30 GB after a feature update) and the hibernation file. It shows their size and the exact steps to remove or shrink them. Duster never deletes either: removing `Windows.old` ends the 10-day option to go back to your previous Windows, and the hibernation file is a power setting. Sizes also appear in `du optimize --json`.
- `du optimize --deep` cleans the Windows component store (WinSxS) through DISM. It first runs the read-only analysis and skips the cleanup when Windows reports nothing to gain, so a pointless 20-minute run is avoided. It needs administrator rights, can run for tens of minutes, and shows the elapsed time while it works. It never passes `/ResetBase`, so updates you already installed can still be uninstalled. Quitting while it runs asks for confirmation first, because stopping DISM part-way through servicing Windows is not something to do by accident; if you do stop it, Duster says so and how to finish. `du optimize --deep --dry-run` (or `--deep --json` without `--yes`) shows the analysis and removes nothing. If DISM's report cannot be read in full, for example on a Windows build that ignores `/English`, the task fails and says so instead of reporting that there is nothing to reclaim.

### Fixed
- `Windows.old` is now a protected path on every drive, so no Duster command can delete a previous Windows installation.

## [1.2.0] - 2026-09-14

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
