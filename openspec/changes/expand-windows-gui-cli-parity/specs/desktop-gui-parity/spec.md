## Purpose

Defines how the Windows GUI presents every user-facing CLI capability as native pages, with the CLI's safety rules: preview, explicit confirmation, visible elevation, and nothing done by the GUI itself.

## ADDED Requirements

### Requirement: Grouped navigation covering the CLI
The GUI SHALL offer, in grouped navigation, Home, Clean, Developer artifacts, Old installers, Apps, Analyze, Virtual disks, Restore, Optimize, System, Diagnostics, Schedule and Settings, so that every capability the parity matrix marks FULL or GUI-NATIVE-EQUIVALENT is reachable without a terminal.

#### Scenario: Every CLI command has a place
- **WHEN** a user looks for any user-facing CLI command from `docs/gui-cli-parity.md`
- **THEN** the matrix names the page that provides it and that page exists in the navigation

### Requirement: Preview and confirm before change
Every page that deletes, moves, uninstalls, compacts, registers or removes SHALL show what it will act on first and SHALL act only after a confirmation dialog whose default button is Cancel and which states where the items go (kept 7 days, Recycle Bin, deleted permanently, uninstalled by the app's own uninstaller, compacted in place).

#### Scenario: Permanent purge
- **WHEN** the user chooses Delete permanently on Developer artifacts
- **THEN** the dialog says the folders are deleted for good and cannot be restored

#### Scenario: Declined confirmation
- **WHEN** the user cancels any confirmation
- **THEN** no engine request that changes state is sent

### Requirement: Results never overstate
Pages SHALL report outcomes as the engine reports them: freed, moved to the Recycle Bin, kept for 7 days, skipped or failed, per item where the engine gives items. Kept or recycled bytes SHALL NOT be shown as freed.

#### Scenario: Purge kept items
- **WHEN** a purge keeps items in the quarantine
- **THEN** the page says they are kept for 7 days and restorable, with their size

### Requirement: Elevation shown per action
Actions that need administrator rights SHALL be marked with a shield and, while unelevated, SHALL offer Restart as administrator instead of failing silently.

#### Scenario: Virtual disks unelevated
- **WHEN** the Virtual disks page is open without administrator rights
- **THEN** Compact is unavailable and Restart as administrator is offered

### Requirement: Live data only on request
The status dashboard SHALL refresh on open and on Refresh; continuous refresh SHALL run only while the user has turned Live on and the page is visible. Expensive samples (top processes, benchmark, driver scan) SHALL run only when the user asks.

#### Scenario: Leaving Home
- **WHEN** Live is on and the user navigates to another page
- **THEN** polling stops

### Requirement: Reports
Home and Diagnostics SHALL offer Copy report, putting a plain-text summary of the shown results on the clipboard.

#### Scenario: Copy doctor report
- **WHEN** the user presses Copy report after a health check
- **THEN** the clipboard holds each check's name, status and message

### Requirement: Self-update and removal
Settings SHALL show the Duster and engine versions, check for updates, install one after confirmation and offer to restart Duster, and offer Remove Duster with the same plan as `du remove`; after removal the GUI SHALL exit. A setup install SHALL be pointed to Windows Settings > Apps.

#### Scenario: Update installed
- **WHEN** an update finishes installing
- **THEN** the page offers Restart Duster, which starts the new Duster.exe and closes this one
