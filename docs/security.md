# Duster — Security Architecture and Safety Boundaries

This document details the threat model, safety boundaries, security architecture, and defensive programming mitigations integrated into **Duster** (`du`) to guarantee maximum safety for millions of production Windows users.

---

## 1. Safety Core Philosophy

Duster is a system utility designed for deep-cleaning operations. Because file deletion is a destructive operation, Duster adheres to a **Strict Safety-First Policy**:
1. **Never Broaden Scope**: When in doubt or encountering an unexpected filesystem structure, Duster will skip, refuse, or raise a warning rather than broadening the sweep range.
2. **Never Touch System Criticals**: Hardcoded and dynamic overrides prevent the deletion of core Windows libraries and boot sectors.
3. **No Untrusted Shell Interpolation**: No user-controlled data is ever interpolated into PowerShell or CMD command strings. PowerShell only ever runs fixed scripts (the drivers and security views' queries, and the delayed self-delete), always from its absolute System32 path to defeat PATH planting; the self-delete passes its target path through an environment variable read with `-LiteralPath`.

---

## 2. Hardened Security Mitigations (v1.0 Releases)

### A. TOCTOU (Time-of-Check to Time-of-Use) Redirection Defense
* **Vulnerability Threat**: Standard recursive directory deletion walk routines can be hijacked if a concurrent unprivileged process swaps a cleanable subfolder with an NTFS Junction pointing to a protected folder (e.g. `C:\Windows\System32`) between Duster's path checks and file removals.
* **Mitigation**: Inside `removeAllSafe` (`utils.go`), Duster executes an `os.Lstat()` check prior to any action. If the mode mask matches `os.ModeSymlink` **or** `os.ModeIrregular` (Go 1.23+ reports NTFS junction points as irregular, not as symlinks), **Duster immediately halts traversal and deletes the link itself directly** via `os.Remove()`, rather than recursing.
* **Deletion roots**: a clean category's root folder is `Lstat`ed first and skipped if it is itself a symlink or junction (`skipCategoryRoot`), so a junction planted at a cache path cannot redirect a clean into another folder.

### B. Environment Variable Spoofing Mitigation
* **Vulnerability Threat**: Command-line path boundaries (e.g., preventing deletions under `%WINDIR%`) can be subverted if a parent process launches Duster with custom-spoofed environment variables (e.g., setting `WINDIR=C:\Users\Public\Dummy`).
* **Mitigation**: Inside `safe.go`, Duster queries kernel directory locations directly from the immutable Windows API using `kernel32.dll` (`GetSystemDirectoryW` and `GetWindowsDirectoryW`). These native overrides completely bypass environment strings, guaranteeing protection boundaries.

### C. OneDrive Cloud Storage Placeholder Safebound
* **Stability Threat**: Walking cloud directories (OneDrive) that contain offline files can trigger automatic hydration (forcibly downloading files from the cloud), leading to extreme network usage and severe disk thrashing.
* **Mitigation**: The scanner checks the Win32 placeholder attributes — `FILE_ATTRIBUTE_OFFLINE` (0x1000) plus the modern cloud-sync markers `FILE_ATTRIBUTE_RECALL_ON_OPEN` (0x40000) and `FILE_ATTRIBUTE_RECALL_ON_DATA_ACCESS` (0x400000) — read from the directory listing itself, which is also the only place Windows reports `RECALL_ON_OPEN` (`GetFileAttributes` never does). If any is flagged, Duster skips the placeholder file entirely without invoking read commands.

### D. Subprocess Context Leaks Prevention
* **Resource Threat**: If a user cancels optimization tasks (like SSD TRIM) or exits the TUI mid-sweep, subprocesses (`defrag.exe`) can continue thrumming in the background as orphans.
* **Mitigation**: Long-running optimization subprocesses are spawned with `exec.CommandContext` tied to a cancellation context, so quitting the optimizer explicitly kills them. `CREATE_NEW_PROCESS_GROUP` additionally detaches children from the console's Ctrl+C group so cancellation stays under Duster's control. Note: if the Duster process itself is killed abruptly (e.g. `taskkill`), in-flight children are not auto-terminated; that would need a kill-on-close job object. (Uninstallers do run in a job object, but only so Duster knows when they finish, see E.)

### E. Third-Party Uninstallers
* **Threat**: `du uninstall` runs the command an app registered under the registry's Uninstall keys. A bare `MsiExec.exe` looked up through `PATH` could run a planted copy with Duster's token, and a per-user (HKCU) entry points into a folder any process of that user can write.
* **Mitigation**: Bare executable names resolve only inside the System32 directory (`GetSystemDirectoryW`), never through `PATH`. The executable is fixed separately and the registered arguments reach `CreateProcess` verbatim, so nothing is re-tokenized. Batch-file uninstallers run through System32's `cmd.exe /d /s /c`: `/s` keeps their quoting intact, and `/d` stops registry AutoRun commands from running with Duster's token. HKCU entries are refused while Duster is elevated. Duster waits for the uninstaller and every process it starts: the uninstaller starts suspended inside a job object (never kill-on-close, so closing Duster leaves it running). That covers Inno Setup and NSIS uninstallers, which hand off to a copy in `%TEMP%` and exit at once. The leftover sweep runs only after the uninstaller succeeded and its registry entry is gone, matches exact folder names, and starts with nothing selected.

### F. Installer Script Elevation
* **Threat**: When an application-control policy forces a Program Files install, `install.ps1` re-launches itself elevated. Re-running a script saved under `%TEMP%` would let a non-admin process swap it before it runs as admin.
* **Mitigation**: The elevated process receives its command through `-EncodedCommand`, with every forwarded value as a single-quoted literal. A piped (`irm | iex`) install fetches the script straight into memory over HTTPS, never through a file.

### G. Virtual Disk Compaction
* **Threat**: `du vdisk` drives `diskpart` with administrator rights. diskpart can erase physical disks, so any path or command it is given is security-relevant, and a VHDX left attached would break the distribution that owns it. A diskpart script file is also read in the system ANSI code page, so a path outside ASCII would not name the file it appears to name.
* **Mitigation**: diskpart is resolved inside the System32 directory (`GetSystemDirectoryW`), never through `PATH`, and only ever receives `select vdisk` / `attach vdisk readonly` / `compact vdisk` / `detach vdisk`. No `select disk`, `select volume` or `clean` is ever written, so no command in the script can reach a physical disk. Every target is checked first: it has to be an existing regular `.vhdx` file, found under a registered WSL distribution's own `BasePath` or a Docker Desktop disk folder, and `Lstat` refuses a symlink or junction at the target rather than following it. A path that cannot be written to an ANSI script is refused outright, with the manual commands, rather than compacted at a mangled path: the 8.3 short name is the only fallback. The disk is attached read-only, so the guest file system cannot be modified, and the detach runs on its own context after any failure, timeout or cancellation, so an interrupted run never leaves a disk attached. Sparse, NTFS-compressed and EFS-encrypted disks are reported and skipped before diskpart is started at all.

### H. Scan History
* **Threat**: `du analyze` saves size snapshots under `%LOCALAPPDATA%\Duster\history`, and Duster may run elevated while writing and pruning files inside the user's profile. A junction planted in that path would redirect an elevated write or delete elsewhere. The snapshots are read back from disk, so a damaged or crafted file must not break or mislead the analysis. They also record the names of large folders and files.
* **Mitigation**: `Duster`, `history` and each per-folder directory are checked with `Lstat` and refused if they are a link or reparse point, so writes and deletes never follow one. Pruning removes only files named `<digits>.json.gz` and then the folder if it is empty: never a recursive delete. A snapshot is written to a temporary file and renamed into place, so a crash never leaves half a file. On load, decompression is capped at 64 MB; a snapshot that fails to parse, names another folder, or whose embedded time disagrees with its file name is skipped for the next one; and a snapshot from a different volume (serial number) is never compared. Snapshots hold names and sizes only, never file contents, stay on the machine, and are deleted by `du remove`. `--no-history` neither reads nor writes one.

### I. Scheduled Cleaning
* **Threat**: `du schedule on` registers a Task Scheduler task that runs unattended, days later, with nobody watching it start. Its arguments live inside the task itself, which anyone with access to Task Scheduler can edit. `duw.exe` is a second, windowless executable whose whole purpose is to start another program with no console, which is also what a hidden-runner primitive looks like. The task's state has to be read back from `schtasks.exe`, whose human-readable output is localized. The record and log it writes live in the user's profile, same as scan history above.
* **Mitigation**: The task's principal is the current user's own SID with `LogonType InteractiveToken` and `RunLevel LeastPrivilege`; it is never registered `HighestAvailable`, and a test asserts this. The category policy (the safe set, the opt-in allow-list and the never-list, each ID in exactly one) is compiled into `du.exe` and re-applied on **every** run, not only at setup: if the task's arguments are edited in Task Scheduler afterwards, `schedule run` re-validates them against that same policy and a refused argument cleans nothing, rather than trusting whatever the task now says. `duw.exe` accepts only `schedule run ...` as its arguments and starts `du.exe` from its own folder, never through `PATH`; anything else exits without starting `du.exe`, so it cannot be handed a different command to run hidden, and it is not a general-purpose windowless launcher. Task state is read only from `schtasks /Query /TN <name> /XML`; the localized table and CSV formats are never parsed for settings, only the CSV name column, and only to find tasks to delete on `off --uninstall`. `schedule.json` and `schedule.log` follow the same rule as scan history (§H): the profile folder and the file itself are `Lstat`ed and refused if either is a link, and the record is written to a temporary file and renamed into place. A Duster uninstall deletes this account's task and `duw.exe` before removing itself; a task an incomplete removal leaves behind points at a missing `duw.exe`, so Task Scheduler logs a failed start and nothing more, never a destructive one.

### J. The Undo Window (Quarantine)
* **Threat**: `du purge`, the uninstall leftover sweep and the `installer` sweep now keep what they remove instead of deleting it, so `du restore` can bring it back. A quarantine that other users could browse would turn every kept item into a way to see another account's files; a restore that could overwrite would turn "undo" into a second way to lose data; and a quarantine that copied across drives would cost time and space on a delete that has to feel instant.
* **Mitigation**: An item's quarantine is always on its own volume: the volume holding `%LOCALAPPDATA%` uses `%LOCALAPPDATA%\Duster\quarantine`; every other fixed or removable drive gets a hidden `.duster-quarantine` folder with a per-user subfolder carved out with its own DACL, full control for that user's SID and `SYSTEM` only, refusing a subfolder that already exists under a different owner (so another account cannot pre-create a readable folder for you to write into). A one-off spike on a GitHub runner (run 36029125858, before this code existed) observed that a same-volume move keeps the item's own ACL, that a folder with this DACL refuses a listing by a second standard user, and that a user who knows an item's full path can still open it if the item's own ACL allowed that before (bypass traverse checking). The `Undo window` step in windows-smoke.yml repeats the second-standard-user check against the real build. This is not a barrier against administrators: an administrator can still take ownership of any file, as with any folder on Windows. Duster never rewrites an item's own permissions, before quarantining it or after restoring it, because a restore has to bring back exactly the access it had before. The guarantee is: **other users cannot list what you have kept, and nothing becomes readable that was not readable before.** Restoring never overwrites: the move back uses `MoveFileEx` without `MOVEFILE_REPLACE_EXISTING`, so an existing target is refused at the OS level rather than raced with a check-then-write; a conflicting restore is reported and the kept item stays kept. Every quarantine root and slot is `Lstat`ed and refused if it is a link, the same rule as scan history and scheduled cleaning (§H, §I); a network drive, or a volume where the root cannot be created, gets no quarantine, and the item is left in place rather than copied elsewhere or deleted for good. Kept items expire after 7 days, or sooner and oldest first on a volume that drops below 10% free, so an undo window can never silently become the reason a drive stays full.

---

## 3. Cryptographic Self-Updater Security

The Duster self-update engine verifies release integrity with **SHA-256 checksum verification**:
1. Release metadata and all assets are downloaded exclusively over HTTPS; non-HTTPS URLs are refused.
2. Each release publishes a `checksums-sha256.txt` asset. The updater downloads it and looks up the expected digest for the platform archive; a release without checksums is treated as not installable.
3. The downloaded archive's SHA-256 digest must match the published entry exactly, or the update aborts before anything is written.
4. The new binary is renamed into place on the same volume, with the previous binary kept as `du.exe.old` so a failed step rolls back.
5. In headless / `--json` mode an update is installed only with `--yes`. Pre-release tags are never offered to users on a stable version, and a tag that doesn't parse as a version never counts as newer (no downgrade through a malformed tag).

> **Trust model:** integrity is rooted in GitHub's TLS and the release checksums file. This protects against corrupted or man-in-the-middle-tampered downloads, but not against a compromised release-publishing account (which could publish a matching checksum). Releases after 1.0.5 also carry a signed build-provenance attestation for every file (GitHub artifact attestations: Sigstore, keyless, recorded in a public transparency log). `gh attestation verify <file> --repo Nur-Adnan/Duster` proves the file was built by this repository's release workflow from the tagged commit, so a file swapped by hand on the release page fails it. It can't stop someone who can change the workflows themselves. Once the SignPath repository variables are configured, the release pipeline Authenticode-signs both exes and the setup exe through SignPath Foundation and fails the release if a signature is missing, invalid or untimestamped ([code-signing.md](code-signing.md)). Until then releases ship unsigned and SmartScreen prompts are expected. The updater and install.ps1 rely on the checksum, not on the Authenticode signature.

---

## 4. Protected Paths Whitelist

Duster hard-blocks deletions on the following directory stems (case-insensitive), enforced by `fs.IsSystemProtectedPath`:
* `C:\Windows` and all subdirs, excluding exactly the cache/log subtrees the clean categories target: `Temp`, `Prefetch`, `SoftwareDistribution\Download`, `SoftwareDistribution\DeliveryOptimization`, `Minidump`, `Logs\CBS`, `Logs\DISM`
* `C:\Windows\System32` (strictly absolute protection, resolved via `GetSystemDirectoryW`)
* `Program Files` & `Program Files (x86)` on any drive
* `Windows.old` on any drive (the previous Windows installation and its 10-day rollback; only Windows' own cleanup may remove it)
* `C:\Boot`, `C:\Recovery`, `C:\EFI`, `C:\$WinREAgent`
* `C:\System Volume Information`
* Root paths (e.g. `C:\`, `D:\`, `\\server\share`, `\\host\c$`)

Before comparison, paths are normalized the way Windows resolves them: `\\?\`, `\\.\` and `\??\` prefixes are removed, admin shares (`\\host\c$\...`) map to their drive while other hidden `$` shares (`admin$`, `print$`) are refused, trailing dots and spaces are trimmed from each component (`C:\Windows.\System32` is `C:\Windows\System32`), and `..` is resolved. Device and volume paths (`\\?\GLOBALROOT\...`, `\\?\Volume{...}`), alternate data streams, and anything else that can't be normalized are treated as protected.
