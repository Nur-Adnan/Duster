# Security Policy

Duster is a local system maintenance and optimization utility for Windows. Because it performs high-risk operations (such as deep caching cleanups, software remnant uninstallation, and system optimization), we treat safety boundaries, path validation, reparse point traversal protections, and release integrity as security-sensitive areas.

## Reporting a Vulnerability

Please report suspected security vulnerabilities privately.

- Channel: Please utilize **GitHub Security Advisories private vulnerability reporting** on the official repository: [github.com/Nur-Adnan/duster](https://github.com/Nur-Adnan/duster).
- Alternate Contact: Please open a draft security advisory or contact `@Nur-Adnan` directly via GitHub.

Do not open a public GitHub issue or pull request for an unpatched vulnerability.

Include as much of the following as possible:

- Duster version and installation method (Scoop, WinGet, Chocolatey, or Portable EXE)
- Windows version and build number (e.g., Windows 11 Build 22631)
- Exact command or diagnostic workflow involved
- Reproduction steps or a safe proof of concept (PoC)
- Details about which safety boundaries (e.g., OneDrive placeholder hydration bypass, junction protections, admin privilege limits) are affected

## Supported Versions

Security updates are actively provided for:

- The latest stable release tag
- The current `main` branch

We highly recommend that users stay up to date with the latest stable releases to ensure safety mechanisms remain active.

## Security Controls in Duster

Duster incorporates key design-level security safeguards:

1. **Junction & Symlink Traversal Protections**: The recursive filesystem sweep walks dynamically verify `os.ModeSymlink` and standard NTFS reparse points to prevent directory loop attacks or accidental deletions of source folders via traversal.
2. **OneDrive Bypass**: Detects and skips `FILE_ATTRIBUTE_OFFLINE` files to prevent thrashing offline storage caches or triggering mass cloud downloads.
3. **Spoof-Proof Path Protections**: Resolves system folders natively using direct DLL queries to `kernel32.dll` (`GetSystemDirectoryW`, `GetWindowsDirectoryW`) rather than relying on mutable environment variables (e.g., `%SYSTEMROOT%`), mitigating path-spoofing escalation.
4. **Subprocess Lifetime Controls**: Restricts spawned utility subprocesses (like native defrag operations) to matching process groups, ensuring no orphaned background processes remain active upon CLI exit.
