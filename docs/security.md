# Duster — Security Architecture and Safety Boundaries

This document details the threat model, safety boundaries, security architecture, and defensive programming mitigations integrated into **Duster** (`du`) to guarantee maximum safety for millions of production Windows users.

---

## 1. Safety Core Philosophy

Duster is a system utility designed for deep-cleaning operations. Because file deletion is a destructive operation, Duster adheres to a **Strict Safety-First Policy**:
1. **Never Broaden Scope**: When in doubt or encountering an unexpected filesystem structure, Duster will skip, refuse, or raise a warning rather than broadening the sweep range.
2. **Never Touch System Criticals**: Hardcoded and dynamic overrides prevent the deletion of core Windows libraries and boot sectors.
3. **No Interactive Shell Calls**: Avoids running command strings in PowerShell or CMD to prevent injection vectors.

---

## 2. Hardened Security Mitigations (v1.0 Releases)

### A. TOCTOU (Time-of-Check to Time-of-Use) Redirection Defense
* **Vulnerability Threat**: Standard recursive directory deletion walk routines can be hijacked if a concurrent unprivileged process swaps a cleanable subfolder with an NTFS Junction pointing to a protected folder (e.g. `C:\Windows\System32`) between Duster's path checks and file removals.
* **Mitigation**: Inside `removeAllSafe` (`utils.go`), Duster executes an `os.Lstat()` check prior to any action. If the mode mask matches `os.ModeSymlink` (which includes Windows NTFS Junction Points), **Duster immediately halts traversal and deletes the link itself directly** via `os.Remove()`, rather than recursing.

### B. Environment Variable Spoofing Mitigation
* **Vulnerability Threat**: Command-line path boundaries (e.g., preventing deletions under `%WINDIR%`) can be subverted if a parent process launches Duster with custom-spoofed environment variables (e.g., setting `WINDIR=C:\Users\Public\Dummy`).
* **Mitigation**: Inside `safe.go`, Duster queries kernel directory locations directly from the immutable Windows API using `kernel32.dll` (`GetSystemDirectoryW` and `GetWindowsDirectoryW`). These native overrides completely bypass environment strings, guaranteeing protection boundaries.

### C. OneDrive Cloud Storage Placeholder Safebound
* **Stability Threat**: Walking cloud directories (OneDrive) that contain offline files can trigger automatic hydration (forcibly downloading files from the cloud), leading to extreme network usage and severe disk thrashing.
* **Mitigation**: The scanner checks for the Win32 `FILE_ATTRIBUTE_OFFLINE` (0x1000) tag using native API calls. If flagged, Duster skips the placeholder file entirely without invoking read commands.

### D. Subprocess Context Leaks Prevention
* **Resource Threat**: If a user cancels optimization tasks (like SSD TRIM) or exits the TUI mid-sweep, subprocesses (`defrag.exe`) can continue thrumming in the background as orphans.
* **Mitigation**: Spawns all subprocesses using `exec.CommandContext` tied to a master cancellation context. Additionally, Duster configures process group attributes (`CREATE_NEW_PROCESS_GROUP`) on Windows to ensure active process trees terminate cleanly upon TUI exit.

---

## 3. Cryptographic Self-Updater Security

The Duster update check and self-update engine verifies the release integrity through **RSA-2048 Cryptographic Signature Verification**:
1. Download of release manifest occurs exclusively over HTTPS.
2. Manifest contains both the binary size, SHA-256 hash, and a cryptographic signature.
3. Duster verifies the download payload against the signature using the embedded public key:
   * **Algorithm**: RSASSA-PKCS1-v1_5
   * **Hash Function**: SHA-256
   * **Key Size**: 2048-bit
4. Any signature discrepancy aborts the update transaction immediately, preserving client state.

---

## 4. Protected Paths Whitelist

Duster hard-blocks deletions on the following directory stems (case-insensitive):
* `C:\Windows` (and all subdirs, excluding allowed Temp, Prefetch, SoftwareDistribution)
* `C:\Windows\System32` (strictly absolute protection)
* `C:\Program Files` & `C:\Program Files (x86)`
* `C:\Boot`, `C:\Recovery`, `C:\EFI`
* `C:\System Volume Information`
* Root paths (e.g. `C:\`, `D:\`)
