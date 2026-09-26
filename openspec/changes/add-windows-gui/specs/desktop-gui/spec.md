## Purpose

Defines what the Windows GUI (Duster.exe) shows and does for people who do not use a terminal, with the same safety guarantees as the CLI.

## ADDED Requirements

### Requirement: Engine discovery and trust
The GUI SHALL start the engine only from `du.exe` in its own installation folder, by absolute path, without a shell, and SHALL refuse to start it if that file is a symbolic link or reparse point. If the handshake reports a protocol version the GUI does not support, the GUI SHALL show an error and offer no actions.

#### Scenario: du.exe missing
- **WHEN** `du.exe` is not beside `Duster.exe`
- **THEN** the GUI shows a clear "engine not found, reinstall Duster" message and offers no clean actions

#### Scenario: Protocol mismatch
- **WHEN** the engine reports a protocol version the GUI does not support
- **THEN** the GUI shows a version-mismatch error instead of the pages

### Requirement: Preview before every destructive action
Every page that deletes, moves, uninstalls, or compacts SHALL first show what will be affected with sizes, and SHALL act only after an explicit confirmation that states whether items are kept restorable for 7 days, sent to the Recycle Bin, or deleted permanently. Reclaimed space that is only moved to quarantine SHALL NOT be shown as freed.

#### Scenario: Clean confirmation
- **WHEN** the user selects categories and presses Clean
- **THEN** a confirmation dialog lists the categories and total size, and nothing changes until the user confirms

#### Scenario: Heuristic matches start unselected
- **WHEN** the uninstall leftover or purge list shows heuristic matches
- **THEN** they start unselected, exactly as in the TUI

### Requirement: Responsive and cancellable
Long operations SHALL show progress and a Cancel button, and the window SHALL stay responsive (resizable, navigable) while they run. Cancel SHALL stop at the next safe item boundary and show what was already done.

#### Scenario: Cancel a clean
- **WHEN** the user presses Cancel during a clean
- **THEN** the page shows the categories already cleaned and their sizes, and the rest are untouched

#### Scenario: Engine crash
- **WHEN** the engine process exits unexpectedly
- **THEN** the GUI reports the failure, marks the running operation as failed, and offers to restart the engine

### Requirement: Elevation on demand
The GUI SHALL run without administrator rights. Actions that need them SHALL be marked with a shield and SHALL offer "Restart as administrator"; the GUI SHALL never elevate silently.

#### Scenario: Admin-only action while unelevated
- **WHEN** an unelevated user opens an admin-only action
- **THEN** the action is disabled with a shield and a "Restart as administrator" button, and pressing it shows the Windows UAC prompt

### Requirement: Accessibility and theming
Every interactive control SHALL have an automation name and ID, be reachable and usable by keyboard alone, and render correctly in Light, Dark, and High Contrast themes.

#### Scenario: Keyboard-only clean
- **WHEN** a user navigates with Tab, arrows, Space, and Enter only
- **THEN** they can reach the Clean page, scan, select categories, clean, and confirm

#### Scenario: High contrast
- **WHEN** Windows high contrast is on
- **THEN** all text and controls use system colors and stay legible

### Requirement: Coexistence with the CLI
The GUI SHALL share the CLI's data directory, operations log, quarantine, and scheduled task, so work done in one is visible in the other.

#### Scenario: Restore across interfaces
- **WHEN** items are purged in the GUI
- **THEN** `du restore` lists that session and can restore it
