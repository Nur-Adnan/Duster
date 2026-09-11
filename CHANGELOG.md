# Changelog

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
