# Duster — Security Architecture and Safety Boundaries

This document details the threat model, safety boundaries, security architecture, and defensive programming mitigations integrated into **Duster** (`du`) to guarantee maximum safety for millions of production Windows users.

---

## 1. Safety Core Philosophy

Duster is a system utility designed for deep-cleaning operations. Because file deletion is a destructive operation, Duster adheres to a **Strict Safety-First Policy**:
1. **Never Broaden Scope**: When in doubt or encountering an unexpected filesystem structure, Duster will skip, refuse, or raise a warning rather than broadening the sweep range.
2. **Never Touch System Criticals**: Hardcoded and dynamic overrides prevent the deletion of core Windows libraries and boot sectors.
3. **No Untrusted Shell Interpolation**: No user-controlled data is ever interpolated into PowerShell or CMD command strings. The one PowerShell invocation (delayed self-delete) passes its target path through an environment variable read with `-LiteralPath`, and resolves `powershell.exe` by absolute System32 path to defeat PATH planting.

---

## 2. Hardened Security Mitigations (v1.0 Releases)

### A. TOCTOU (Time-of-Check to Time-of-Use) Redirection Defense
* **Vulnerability Threat**: Standard recursive directory deletion walk routines can be hijacked if a concurrent unprivileged process swaps a cleanable subfolder with an NTFS Junction pointing to a protected folder (e.g. `C:\Windows\System32`) between Duster's path checks and file removals.
* **Mitigation**: Inside `removeAllSafe` (`utils.go`), Duster executes an `os.Lstat()` check prior to any action. If the mode mask matches `os.ModeSymlink` **or** `os.ModeIrregular` (Go 1.23+ reports NTFS junction points as irregular, not as symlinks), **Duster immediately halts traversal and deletes the link itself directly** via `os.Remove()`, rather than recursing.

### B. Environment Variable Spoofing Mitigation
* **Vulnerability Threat**: Command-line path boundaries (e.g., preventing deletions under `%WINDIR%`) can be subverted if a parent process launches Duster with custom-spoofed environment variables (e.g., setting `WINDIR=C:\Users\Public\Dummy`).
* **Mitigation**: Inside `safe.go`, Duster queries kernel directory locations directly from the immutable Windows API using `kernel32.dll` (`GetSystemDirectoryW` and `GetWindowsDirectoryW`). These native overrides completely bypass environment strings, guaranteeing protection boundaries.

### C. OneDrive Cloud Storage Placeholder Safebound
* **Stability Threat**: Walking cloud directories (OneDrive) that contain offline files can trigger automatic hydration (forcibly downloading files from the cloud), leading to extreme network usage and severe disk thrashing.
* **Mitigation**: The scanner checks the Win32 placeholder attributes — `FILE_ATTRIBUTE_OFFLINE` (0x1000) plus the modern cloud-sync markers `FILE_ATTRIBUTE_RECALL_ON_OPEN` (0x40000) and `FILE_ATTRIBUTE_RECALL_ON_DATA_ACCESS` (0x400000) — using native API calls. If any is flagged, Duster skips the placeholder file entirely without invoking read commands.

### D. Subprocess Context Leaks Prevention
* **Resource Threat**: If a user cancels optimization tasks (like SSD TRIM) or exits the TUI mid-sweep, subprocesses (`defrag.exe`) can continue thrumming in the background as orphans.
* **Mitigation**: Long-running optimization subprocesses are spawned with `exec.CommandContext` tied to a cancellation context, so quitting the optimizer explicitly kills them. `CREATE_NEW_PROCESS_GROUP` additionally detaches children from the console's Ctrl+C group so cancellation stays under Duster's control. Note: if the Duster process itself is killed abruptly (e.g. `taskkill`), in-flight children are not auto-terminated — that would require Windows Job objects, which are not currently used.

---

## 3. Cryptographic Self-Updater Security

The Duster self-update engine verifies release integrity with **SHA-256 checksum verification**:
1. Release metadata and all assets are downloaded exclusively over HTTPS; non-HTTPS URLs are refused.
2. Each release publishes a `checksums-sha256.txt` asset. The updater downloads it and looks up the expected digest for the platform archive; a release without checksums is treated as not installable.
3. The downloaded archive's SHA-256 digest must match the published entry exactly, or the update aborts before anything is written.
4. The new binary is swapped in atomically on the same volume, with the previous binary preserved for rollback if any step fails.

> **Trust model:** integrity is rooted in GitHub's TLS and the release checksums file. This protects against corrupted or man-in-the-middle-tampered downloads, but not against a compromised release-publishing account (which could publish a matching checksum). Authenticode signing of the binaries is not yet wired into the release pipeline; treat SmartScreen prompts as expected for unsigned builds.

---

## 4. Protected Paths Whitelist

Duster hard-blocks deletions on the following directory stems (case-insensitive), enforced by `fs.IsSystemProtectedPath`:
* `C:\Windows` and all subdirs, excluding exactly the cache/log subtrees the clean categories target: `Temp`, `Prefetch`, `SoftwareDistribution\Download`, `SoftwareDistribution\DeliveryOptimization`, `Minidump`, `Logs\CBS`, `Logs\DISM`
* `C:\Windows\System32` (strictly absolute protection, resolved via `GetSystemDirectoryW`)
* `C:\Program Files` & `C:\Program Files (x86)`
* `C:\Boot`, `C:\Recovery`, `C:\EFI`, `C:\$WinREAgent`
* `C:\System Volume Information`
* Root paths (e.g. `C:\`, `D:\`)
