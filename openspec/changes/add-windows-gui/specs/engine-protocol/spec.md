## Purpose

Defines the private stdio contract through which the Duster GUI drives the Go engine, so the GUI never re-implements scanning, deleting, or path-safety rules.

## ADDED Requirements

### Requirement: Stdio-only transport
`du engine` SHALL read one JSON request per line from stdin and write one JSON message per line to stdout, and SHALL NOT open any network socket, named pipe, or file-based channel. Nothing other than protocol messages SHALL be written to stdout; diagnostics go to stderr.

#### Scenario: Engine function prints text
- **WHEN** an engine function called by a request writes to standard output
- **THEN** that text appears on stderr and stdout still carries only valid protocol lines

#### Scenario: Parent closes stdin
- **WHEN** stdin reaches end of file
- **THEN** the engine cancels every in-flight request, waits for each to reach a safe stopping point, and exits with code 0

### Requirement: Message framing and correlation
Each request SHALL be `{"id": <positive integer>, "method": <string>, "params": <object, optional>}`. Each request SHALL receive exactly one final reply with the same id: `{"id", "result"}` on success or `{"id", "error": {"code", "message"}}` on failure. Zero or more `{"id", "event", "data"}` messages MAY precede the final reply. Lines longer than 1 MiB SHALL be rejected.

#### Scenario: Unknown method
- **WHEN** a request names a method the engine does not implement
- **THEN** the engine replies with error code `unknown_method` and keeps serving

#### Scenario: Malformed line
- **WHEN** a line is not valid JSON or has no positive integer id
- **THEN** the engine writes an error with id 0 and code `bad_request` and keeps serving

### Requirement: Version handshake
The `hello` method SHALL return the protocol version (integer, starting at 1) and the Duster version. The protocol version SHALL change whenever a method is removed or a field changes meaning.

#### Scenario: Handshake
- **WHEN** the client sends `hello`
- **THEN** the result contains `protocol: 1` and the build's version string

### Requirement: Cancellation
A `cancel` request with `params.id` SHALL cancel that in-flight request. The cancelled request SHALL stop at the next item boundary, never partway through moving or deleting a single item, and SHALL reply with error code `canceled` and a partial result in `error.data` of what was already done.

#### Scenario: Cancel during a clean
- **WHEN** the client cancels a running `clean.run` after some categories finished
- **THEN** the reply has code `canceled`, `error.data` lists the finished categories with their freed bytes, and no category is left half-processed beyond what a single item deletion allows

#### Scenario: Cancel an unknown id
- **WHEN** `cancel` names an id that is not running
- **THEN** the cancel request itself succeeds and nothing else changes

### Requirement: One destructive operation at a time
The engine SHALL run at most one state-changing request at a time. A second state-changing request while one is running SHALL fail at once with code `busy`. Read-only requests (status, doctor, scans) SHALL run concurrently.

#### Scenario: Two cleans
- **WHEN** a `clean.run` is running and another `clean.run` arrives
- **THEN** the second one replies `busy` without touching any file

### Requirement: Act only on engine-produced targets
State-changing methods SHALL accept only targets the engine itself returned from a scan in the same process (category IDs from `clean.scan`, item IDs from other scans), never raw paths from the client. Every path SHALL still pass the same safety checks the CLI applies before any change.

#### Scenario: Client sends an unknown target
- **WHEN** `clean.run` names a category ID that the latest `clean.scan` did not return
- **THEN** the engine replies `bad_request` and changes nothing

#### Scenario: Reparse point inside a clean target
- **WHEN** a clean category contains a symlink or junction
- **THEN** the link itself is removed or skipped exactly as the CLI does, and its target is never traversed

#### Scenario: Permission denied or file in use
- **WHEN** some files in a selected category are locked or access is denied
- **THEN** those files are skipped, the category result reports the failure count, and the remaining files are still processed

### Requirement: Clean methods
`clean.scan` SHALL return every clean category with its ID, name, group, estimated bytes and file count, and whether it is blocked for lack of admin rights. `clean.run` SHALL clean exactly the requested categories, stream a progress event as each category starts and finishes, and return per-category freed bytes, file counts, and errors. Admin-blocked categories SHALL be refused with a per-category error, not silently skipped. The operations log SHALL receive the same entries the CLI writes for the same categories.

#### Scenario: Selected subset
- **WHEN** the client runs `clean.run` with two of the scanned category IDs
- **THEN** only those two categories are cleaned and the result lists exactly those two

#### Scenario: Admin-only category while unelevated
- **WHEN** `prefetch` is requested and the engine is not elevated
- **THEN** its result entry carries an `admin_required` error and no file under it is touched

### Requirement: Status and doctor methods
`status.get` SHALL return the same system statistics as `du status --json`, sampling top processes only when `params.top_processes` is true (that sample takes about a second, so a polling GUI leaves it off). `doctor.run` SHALL return the same checks as `du doctor --json`.

#### Scenario: Status parity
- **WHEN** the client calls `status.get` with `top_processes: true`
- **THEN** the result has the same fields and meanings as `du status --json` output

#### Scenario: Cheap poll
- **WHEN** the client calls `status.get` without `top_processes`
- **THEN** the reply omits the process list and returns without the one-second sample
