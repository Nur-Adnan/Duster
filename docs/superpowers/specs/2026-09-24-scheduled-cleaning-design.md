# Scheduled cleaning (`du schedule`) — design

Status: approved; implemented on feat/scheduled-clean.
Branch: `feat/scheduled-clean` (stacked on `feat/analyze-changes`, whose
link-refusing directory helpers it reuses).

## 1. Intent

Keep a PC's caches tidy **without the user having to remember Duster exists**.
Most people never run a cleaner by hand, so without scheduling Duster only
helps on the day it is opened.

Success means:

- It runs **silently**: no console or Windows Terminal window, ever.
- It **never runs elevated** and never touches anything the user would miss.
- It is **easy to turn off** and always able to say **what the last run did**.
- It complements, not duplicates, Windows Storage Sense, which by default runs
  only on low disk and cleans temp files and the Recycle Bin, never browser,
  app or developer caches.

### Decisions made with the user

| Question | Decision |
|---|---|
| Triggers | A chosen schedule (daily, weekly, monthly) **plus** an early clean when the system drive runs low on space |
| Categories | A safe set by default; developer and app caches opt-in from an allow-list; a never-list refused with a stated reason |
| Silent execution | `schtasks.exe` + task XML + a windowless `duw.exe` launcher (approach A) |

Rejected, with reasons:

- `conhost.exe --headless`: undocumented, and flagged by Splunk's detection
  "Windows ConHost with Headless Argument" (T1564.003 Hidden Window).
- S4U "run whether logged on or not": needs "Log on as a batch job", which
  standard users lack by default, and has no access to EFS files.
- `consoleAllocationPolicy=detached`: Windows 11 24H2+ only, and it would stop
  a Start-menu launch of `du` from opening a window.
- Task Scheduler COM via go-ole: same behaviour as `schtasks` for ~300 more
  lines and a new direct dependency.
- Plain `du.exe` as the action: flashes a console, or opens a Windows Terminal
  window on Windows 11.
- `MaintenanceSettings` (Automatic Maintenance): whether standard users may
  register maintenance tasks is undocumented.

### Non-goals

- No config file: every setting lives in the task's own arguments.
- No TUI: set-and-forget; plain CLI output plus `--json`.
- No toast notifications: `du schedule` reports the last run.
- No low-disk event trigger: client Windows has none, so a daily check
  measures free space itself.
- Fixing the `spotify` category's deletion of offline downloads (see §10):
  a separate change.

## 2. Architecture

| Unit | Responsibility | Depends on |
|---|---|---|
| `cmd/schedule.go` | `du schedule` command: `status` (default), `on`, `off`, hidden `run` | the units below |
| `cmd/schedule_policy.go` | Pure logic, no I/O: category policy, due decision, argument encoding and parsing | nothing |
| `cmd/schedule_task.go` | Build and parse task XML, and read task names out of `schtasks` CSV output; untagged, so it is tested on every OS | `encoding/xml` |
| `cmd/schedule_task_windows.go` | Register, delete and query the task through System32 `schtasks.exe`; only this file is Windows-only | `systemExecutable`, `cmd/schedule_task.go` |
| `cmd/stubs.go` | Non-Windows stubs for every symbol in the Windows file | — |
| `launcher/duw/main.go` | Windowless launcher: runs `du.exe` from its own folder with no console, appends output to the schedule log | stdlib, `internal/logging` |

### Data flow

Setup:

```
du schedule on --every weekly --at 19:00 --low-space 10% --add npm,gradle
  → validate flags and categories (unknown or never-listed = refuse with reason)
  → locate duw.exe beside the running du.exe (missing = refuse)
  → build task XML (encoding/xml)
  → schtasks /Create /XML <private temp file> /TN "Duster Scheduled Clean (<user>)" /F
  → print the summary and the first check time
```

Each day at the chosen time, while the user is logged on:

```
Task Scheduler → duw.exe schedule run --every weekly --low-space 10 --add npm,gradle
  → du.exe schedule run …        (CREATE_NO_WINDOW; stdout/stderr → schedule.log)
  → re-validate arguments against the compiled-in policy
  → due? no → record the check, exit 0
  → clean each allowed category through runCategory
  → record the result in schedule.json, exit 0 or 1
```

## 3. Command-line surface

```
du schedule                        show status (same as `du schedule status`)
du schedule on [flags]             create or replace the scheduled clean
    --every daily|weekly|monthly   how often to clean              (default weekly)
    --at HH:MM                     daily check time, 24-hour clock (default 19:00)
    --low-space N% | off           clean early below this free %   (default 10%)
    --add id,id,...                opt-in categories from the allow-list
    --dry-run                      print what would be registered and what a run
                                   would clean right now; change nothing
du schedule off                    delete this user's task; keep the history record
    --uninstall                    delete every "Duster Scheduled Clean (*)" task
                                   this account can see (used by the uninstaller)
--json                             on status, on and off
```

`run` is hidden and takes the same `--every`, `--low-space` and `--add` flags.

"System drive" always means the drive holding the Windows directory, from
`systemDriveRoot()` (kernel-resolved, never `%SystemDrive%`). `off` with no
task registered prints "Scheduled clean is already off" and exits 0.
`--uninstall` has no wildcard `schtasks` query to call, so it finds its
targets by reading the name column out of `schtasks /Query /FO CSV /NH` and
deleting every name that starts with `Duster Scheduled Clean (`.
`status --json` carries the same facts as the text: `enabled`, `task_name`,
`every`, `at`, `low_space_percent` (null when off), `categories`,
`next_check`, `last_check`, `last_clean`, `task_command`, `warnings`.

Validation:

- `--at` must be `HH:MM`, 00:00–23:59.
- `--low-space` accepts `1%`–`50%` or `off`.
- `--add` IDs must exist, and must be opt-in (§4). Safe-default IDs are
  accepted and ignored with a note. Never-listed IDs are refused with their
  reason, e.g. `Error: Recycle Bin can't be scheduled: it's how you undo a delete.`

Status when on:

```
Scheduled clean: ON
  Cleans weekly, or early when C: has under 10% free. Checks daily at 19:00.
  Cleans: Temporary Files, Browser Caches, ... + npm Cache, Gradle Build Cache
  Next check:  Thu Sep 25, 19:00
  Last clean:  Mon Sep 22, 19:02 (weekly): freed 1.84 GB, no errors
  Last check:  Wed Sep 24, 19:00: not due (C: 38% free)
  Log:         %LOCALAPPDATA%\Duster\schedule.log
Turn off with: du schedule off
```

Status warns when the task points at a `duw.exe` that no longer exists, when
the task is disabled in Task Scheduler, and (on `on`) when run elevated:
"registered for <user>; runs without administrator rights".

## 4. Category policy

Every ID returned by `getCategories()` must appear in exactly one row. A test
fails when a category exists without a decision.

| Class | IDs | Reason |
|---|---|---|
| **Safe default** (7) | `temp`, `browsers`, `opera`, `thumbs`, `wer`, `crash_dumps`, `gpu_shader` | Rebuild themselves; hold nothing the user made. Browser categories delete cache only, never cookies, history or sessions |
| **Opt-in** (17) | `npm`, `pnpm`, `yarn`, `bun`, `pip`, `cargo`, `gradle`, `nuget`, `docker`, `vscode`, `jetbrains`, `discord`, `slack`, `teams`, `steam`, `epic`, `adobe` | Safe, but the next build or launch downloads it again: costly offline or on a metered connection |
| **Never** (10) | `recycle` | It's how you undo a delete |
| | `recent` | It's your recent-files list |
| | `spotify` | It holds Spotify's offline downloads |
| | `dns` | Flushing the DNS cache frees no space |
| | `update`, `prefetch`, `delivery_opt`, `memdumps`, `logfiles` | Machine-wide: needs administrator rights, and scheduled cleans never run as administrator |
| | `fontcache` | Held by a Windows service during a session; clean it by hand with `du clean` |

The policy is compiled into `du.exe` and applied on **every** run, not only at
setup: the task's arguments are user-editable, and a future Duster may narrow
the allow-list.

## 5. What a scheduled run does

1. **Parse and re-validate** the arguments against the policy. Any refusal is
   recorded and the run exits 1 without cleaning anything.
2. **Decide whether a clean is due** (pure function of now, last successful
   clean, cadence, free %, threshold):

   | Situation | Result |
   |---|---|
   | No clean has fully succeeded yet (`last_success` unset) | Due: "no successful clean yet" |
   | Last success at least cadence − 4 h ago (daily 20 h, weekly 164 h, monthly 716 h) | Due: the cadence name |
   | Low space enabled and system drive free % below it | Due: "low space (N% free)" |
   | Otherwise | Not due |

   The decision keys on `last_success`, the last clean where at least one
   category did not fail, not merely the last attempt: a run where every
   category failed leaves `last_success` untouched, so the next day's check
   is still "no successful clean yet" and retries rather than waiting out
   the cadence. The 4-hour slack absorbs start-time jitter, so a clean at
   19:02 does not make the next day's 19:00 check "not yet". Low space can
   trigger at most once per day because the task fires daily.
3. **Clean** each allowed category in `cleanGroups` order through
   `runCategory(cat, false)`, exactly as `du clean` does, inheriting every
   safety invariant. `adminOnlyBlocked` categories never appear (policy).
4. **Classify each category's result**:
   - no error: `cleaned`;
   - error whose cause is Windows error 5 (access denied), 32 (sharing
     violation) or 33 (lock violation): `partial`, "some items in use or
     protected were skipped". This is the normal case for `%TEMP%` and for
     WER's machine-wide half, and is **not** a failure;
   - any other error: `failed`.
5. **Record** in `%LOCALAPPDATA%\Duster\schedule.json` (atomic temp + rename;
   the directory must not be a link, via `ensureRealDir`/`realDir`):
   last check (time, result, free %), last clean (time, reason, total
   freed, per-category `{id, freed, files, status, error}`, free % before
   and after), and `last_success` (the time of the last clean where at
   least one category did not fail; what step 2's due decision reads back).
   A damaged file reads as "no history".
6. **Exit** 0 for cleaned, partial or not due; 1 for refused arguments or any
   `failed` category. Task Scheduler's "Last Run Result" therefore matches.

## 6. The task

Registered with `schtasks /Create /XML <file> /TN <name> /F`, the XML built
with `encoding/xml` and written UTF-16LE with a BOM to a private temporary
file, deleted afterwards.

- **Name**: `Duster Scheduled Clean (<account>)`, with the account name
  reduced to `[A-Za-z0-9._-]`. Task names are machine-wide; one per user
  prevents two users' tasks from colliding. When sanitising drops a
  character, the name gets a `-<6 hex of sha256(account)>` suffix, so two
  different account names that sanitise to the same string (`José` and
  `Jos`) never collide on one task.
- **Principal**: the current user's SID, `LogonType InteractiveToken`,
  `RunLevel LeastPrivilege`. Never `HighestAvailable`.
- **Trigger**: one `CalendarTrigger`, daily at `--at`.
- **Action**: `Command` = the quoted absolute path of `duw.exe`
  (`"C:\Program Files\Duster\duw.exe"`, as Task Scheduler's own exports do,
  so a path with a space is still one program); `Arguments` =
  `schedule run --every <e> --low-space <n|off> [--add a,b]` (validated tokens
  only, so Windows argv quoting cannot be abused).
- **Settings**: `DisallowStartIfOnBatteries` true, `StopIfGoingOnBatteries`
  true, `StartWhenAvailable` true, `ExecutionTimeLimit` PT1H,
  `MultipleInstancesPolicy` IgnoreNew, `Priority` 7, `Enabled` true.
- **Reading state**: `schtasks /Query /TN <name> /XML` only. The table and
  CSV formats are localized and are never parsed. The next check time is
  computed by Duster from the trigger time.

## 7. `duw.exe`

`launcher/duw/main.go`, built with `-ldflags "-H=windowsgui"` so Windows never
gives it a console.

- Resolves `du.exe` as `filepath.Join(filepath.Dir(os.Executable()), "du.exe")`;
  never `%PATH%`.
- Accepts only `schedule run ...` as its arguments; anything else exits 2
  without starting `du.exe`. It is not a general hidden runner.
- Starts `du.exe` with `CREATE_NO_WINDOW`, arguments verbatim, no shell.
- Appends stdout and stderr to `logging.Dir()\schedule.log`, rotated at 1 MB
  with one `.old` kept; refuses to write through a link.
- Exits with `du.exe`'s exit code.

## 8. Shipping `duw.exe`

| Touch point | Change |
|---|---|
| Makefile, `scripts/build-release.sh`, `.github/workflows/release.yml` | Build `duw` for amd64 and arm64 beside `du`; include it in SignPath signing, attestations and `checksums-sha256.txt` |
| Portable zips (all three paths) | Ship `duw.exe` beside `du.exe` |
| `installer/duster-setup.iss` | Install `duw.exe`; on uninstall run `{app}\du.exe schedule off --uninstall` (hidden, errors ignored) |
| `scripts/install.ps1` | Copy `duw.exe` from the archive |
| `du update` (`cmd/update.go`) | Also extract `duw.exe` from the already SHA-256-verified zip and replace it with the rename-then-write method of `swapBinary`, with rollback. This is how existing installs, whose zips never had `duw.exe`, receive it |
| `du remove` | Delete this user's task first; remove `duw.exe` with `du.exe` |
| `du schedule on` without `duw.exe` | Refuse: "`duw.exe` is missing from `<dir>`: run `du update` or reinstall". Never fall back to `du.exe` |

A task left behind after an uninstall points at a missing file; Task
Scheduler logs a failed start. Untidy, never destructive, so cleanup is
best-effort.

## 9. Safety and error handling

- Never elevated (principal and run level above; asserted by a test).
- XML built with `encoding/xml`; a test uses an install path containing
  `& < > " '`.
- `duw.exe` runs `du.exe` from its own folder as the same user: replacing it
  requires already being that user, so there is no privilege to gain.
- No localized output is parsed (§6).
- `schtasks` failures are reported with its message and exit 1.
- `du schedule` stays useful when the task is missing but a record exists,
  when the task is disabled, and when it points at a missing `duw.exe`.
- Every deletion goes through `runCategory` and is logged to
  `operations.log`, like `du clean`.

## 10. Known issue found during design (separate change)

The existing `spotify` clean category deletes `%LOCALAPPDATA%\Spotify\Storage`,
which is Spotify's default **offline storage location** on the desktop app.
`du clean` therefore removes users' downloaded playlists. It is on this
design's never-list; fixing `du clean` itself is a separate change.

## 11. Testing

1. **Spike (first implementation task, gates the rest).** On the Windows
   runner, create a local standard user. As that user, register the generated
   XML with `schtasks`, start it with `schtasks /Run`, confirm `duw.exe` ran
   `du.exe schedule run` (via `schedule.log` and `schedule.json`), then delete
   the task. If a standard user cannot register it, stop and revisit the design
   with the user before building further.
2. **Unit tests**: policy completeness over `getCategories()`; due decision
   table (first run, slack boundary, low space, monthly, low space off);
   error classification (5, 32, 33 vs other); XML golden file, escaping,
   least-privilege assertion; task-name sanitising; flags → task arguments →
   `run` parsing round trip; `duw` argument restriction; `schedule.json`
   atomic write, damaged file, link refusal; log rotation; `update`
   extracting `duw.exe`.
3. **Windows smoke** (permanent steps): `on --dry-run`, `on`, `status --json`
   reports ON, `schtasks /Run`, status shows the recorded check or clean,
   `off`, the task is gone; repeated as a standard user.
4. **Manual checklist** (`docs/release-checklist.md`): unplugged laptop
   (skips), asleep at the check time (catches up), Windows Terminal as the
   default console (no window), two users on one PC (separate tasks),
   uninstall removes the task.

## 12. Documentation

README command row and a short section; CHANGELOG; `docs/security.md`
(new section on scheduled runs and `duw.exe`); `docs/code-signing.md`
(the second binary); release checklist; CLAUDE.md (command table, safety
invariant for the policy, known gaps).
