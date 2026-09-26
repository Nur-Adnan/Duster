## Purpose

Defines the engine methods and parameters that give the Windows GUI every user-facing CLI capability, each running the CLI's own code, accepting only targets the engine listed itself.

## ADDED Requirements

### Requirement: Listed targets only
Every method that changes state SHALL accept only 1-based IDs from the latest listing of its kind in the same engine process (`purge.scan`, `installer.scan`, `uninstall.list`, the leftovers returned by `uninstall.run`, `vdisk.scan`, `startup.list`), never a path, registry key, command line or executable from the client. Before acting it SHALL re-check the live state and refuse a target that changed or vanished.

#### Scenario: ID not from the latest listing
- **WHEN** `purge.run` names an ID outside the latest `purge.scan`
- **THEN** the engine replies `bad_request` and changes nothing

#### Scenario: Startup entry changed since the list
- **WHEN** `startup.remove` names an entry whose name, location or command no longer matches the registry or Startup folder
- **THEN** the engine refuses it and removes nothing

#### Scenario: Target became a reparse point
- **WHEN** a listed purge or installer target has been replaced by a symbolic link or junction since the scan
- **THEN** the link itself is moved or refused exactly as the CLI does, and its target is never traversed

### Requirement: Diagnostic methods
`doctor.run`, `verify.run`, `benchmark.run`, `security.run` and `drivers.list` SHALL return the same results the CLI shows for `du doctor`, `du verify`, `du benchmark` and the landing menu's Security and Drivers views, and SHALL change nothing outside their own temporary sandboxes.

#### Scenario: Doctor parity
- **WHEN** the client calls `doctor.run`
- **THEN** the result has the same checks, statuses and messages as `du doctor --json`

#### Scenario: Not on Windows
- **WHEN** a Windows-only view (security, drivers, startup) is called where Windows is not available
- **THEN** the engine replies `failed` with the reason and keeps serving

### Requirement: Purge methods
`purge.scan` SHALL take an absolute folder, find the same artifacts as `du purge` (project marker required, quarantines skipped), stream progress and stop when canceled. `purge.run` SHALL take listed IDs and a mode `keep` (quarantine, default), `recycle` (Recycle Bin, quarantine fallback) or `permanent`, apply the same retention sweep first, stop between items when canceled, and report freed, recycled and kept bytes separately with every failure.

#### Scenario: Kept bytes are not freed
- **WHEN** `purge.run` keeps items in the quarantine
- **THEN** the result reports them as kept, never as freed

#### Scenario: Locked or denied folder
- **WHEN** one selected artifact is in use or access is denied
- **THEN** that item is reported failed with the reason and the rest are still processed

#### Scenario: Cancel mid-run
- **WHEN** the client cancels `purge.run` after some items
- **THEN** the reply is `canceled` with the partial tally, and no item is left half-moved

### Requirement: Installer and uninstall methods
`installer.scan` SHALL list installers in Downloads at least 7 days old and at least `min_size_mb` (default 50); `installer.run` SHALL keep the listed IDs in the quarantine. `uninstall.list` SHALL list installed apps with a `protected` flag; `uninstall.run` SHALL refuse protected apps and per-user apps while elevated, run the app's own uninstaller through the CLI's process-tree wait, and return the leftover folders (none preselected) only when the app is gone; `uninstall.sweep` SHALL keep the chosen leftovers in the quarantine.

#### Scenario: App still installed after its uninstaller
- **WHEN** the uninstaller exits but the app's registry entry remains
- **THEN** the result says so and offers no leftovers

#### Scenario: Protected app
- **WHEN** `uninstall.run` names an app matching the protected list
- **THEN** the engine replies `bad_request` and runs nothing

#### Scenario: Leftover in use
- **WHEN** a chosen leftover folder cannot be moved because a file is locked or access is denied
- **THEN** it is reported failed and stays where it is

### Requirement: Optimize and virtual disk methods
`optimize.list` SHALL return the CLI's tasks (including the component store task), which need administrator rights, and the reclaim report. `optimize.run` SHALL run the named tasks in order with the CLI's rules (TRIM and component store skipped when not elevated; `dry_run` runs only the component store analysis) and report per-task status, reclaimed bytes and notes. `vdisk.scan` SHALL return the CLI's disks, block reasons and advice; `vdisk.run` SHALL refuse when not elevated, shut down WSL, compact each listed disk with the CLI's diskpart sequence, and leave a disk detached if canceled.

#### Scenario: Unelevated TRIM
- **WHEN** `optimize.run` includes `ssd_trim` while the engine is not elevated
- **THEN** that task is reported skipped, not failed

#### Scenario: Blocked disk
- **WHEN** a listed disk is sparse, compressed or encrypted
- **THEN** `vdisk.run` reports it skipped with the reason and never runs diskpart on it

### Requirement: Startup methods
`startup.list` SHALL return the Run-key and Startup-folder entries with enabled state and whether they need administrator rights. `startup.toggle` SHALL flip one entry's StartupApproved state (reversible). `startup.remove` SHALL delete only entries that are disabled, as the landing TUI does.

#### Scenario: Remove an enabled entry
- **WHEN** `startup.remove` names an entry that is enabled
- **THEN** the engine refuses it

#### Scenario: HKLM entry without rights
- **WHEN** a machine-wide entry is toggled while not elevated and Windows denies the write
- **THEN** the entry is reported failed with the reason and nothing else changes

### Requirement: Schedule methods
`schedule.get` SHALL return the same status as `du schedule status --json` plus the safe and opt-in category choices. `schedule.set` SHALL take `every`, `at`, `low_space` and `add`, validated by the CLI's parsers, and register the same task as `du schedule on` (or with `dry_run`, register nothing and report what a run would clean now). `schedule.off` SHALL delete this account's task and keep the run record.

#### Scenario: Invalid time
- **WHEN** `schedule.set` gets `at: "25:00"`
- **THEN** the engine replies with the parser's error and registers nothing

### Requirement: Update and remove methods
`update.check` SHALL report the current version, the latest release and whether it is newer, never offering a pre-release to a stable version. `update.install` SHALL re-fetch the release itself (never trusting client data), require the published SHA-256, and install du.exe, duw.exe and Duster.exe as `du update` does; `force` reinstalls the current version. `remove.plan` SHALL describe what `du remove` deletes; `remove.run` SHALL perform it and refuse when Duster sits in a system-protected path.

#### Scenario: Checksum mismatch
- **WHEN** the downloaded archive does not match the release's checksum
- **THEN** nothing is replaced and the reply names the mismatch

### Requirement: Analyze and restore additions
`analyze.scan` SHALL accept `since` (parsed like `--since`) and `no_history` (neither compare nor save), and SHALL give each changed folder still in the tree an ID; every folder listing SHALL include its breadcrumb (IDs from the root). `analyze.reveal` SHALL open an issued item (or a change's nearest existing folder) in Explorer. `restore.list` SHALL first delete sessions past the 7-day window exactly as `du restore` does, and `restore.run` SHALL accept `dry_run`. `oplog.list` SHALL return the newest operations log entries.

#### Scenario: Restore preview
- **WHEN** `restore.run` is called with `dry_run: true`
- **THEN** each item is reported as it would be (restored or skipped) and nothing moves

#### Scenario: Invalid since
- **WHEN** `analyze.scan` gets `since: "soon"`
- **THEN** the engine replies `bad_request` with the parser's message
