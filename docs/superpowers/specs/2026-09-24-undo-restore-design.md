# Undo window (`du restore`): design

Status: approved in conversation 2026-09-24, pending review of this document.
Branch: `feat/undo-restore`, stacked on `feat/scheduled-clean` (the retention
sweep also runs from `du schedule run`).

## 1. Intent

A tool that deletes user data earns trust by letting a mistake be taken back.
Today, `purge`, the uninstall leftover sweep and the `installer` sweep delete
**permanently**. `analyze d` uses the Recycle Bin, but anything too big for the
bin currently ends in a "delete permanently?" prompt.

Success means that someone who removed the wrong folder can get it back days
later with one command. Restoring must never overwrite newer data, keeping an
item must cost no copy (it is a same-drive rename), and a nearly full disk must
still get its space back.

### Decisions made with the user

| Question | Decision |
|---|---|
| Scope | User-facing deletes only. Self-rebuilding caches (`clean`, scheduled cleans) keep deleting directly |
| Retention | 7 days; sooner, oldest first, when the drive is below 10% free |
| Approach | Hybrid: a Duster quarantine where Duster deletes permanently today, and as the fallback when an item is too big for the Recycle Bin. `analyze d` and `purge --safe` keep the Recycle Bin |

Rejected alternatives:
- Quarantine everywhere: it breaks the Explorer restore habit for `analyze d`.
- Recycle Bin plus a session log: items over the bin's size cap (`node_modules`, ISOs) still cannot be undone, and restoring would depend on the undocumented `$I`/`$R` format.

Prior art: FreeDesktop Trash (per-volume trash, one metadata file per item),
Windows `$Recycle.Bin\<SID>`, and gomi. None of them overwrites on restore.

### Non-goals
- A restore TUI.
- Restoring to another folder (`--to`).
- A per-drive quarantine cap.
- Quarantining registry values or startup entries.
- Undoing `clean`.

## 2. Storage

- **Location.** Always on the same volume as the item, so keeping an item is a rename:
  - On the volume holding `%LOCALAPPDATA%`: `logging.Dir()\quarantine\`.
  - On any other fixed drive `X:`: `X:\.duster-quarantine\<user SID>\`. The `.duster-quarantine` folder is hidden. The SID folder gets a protected DACL: full control for the user and SYSTEM only.
  - Both roots must pass `realDir`/`ensureRealDir` (a link or junction there is refused).

  Verified on a runner (spike run 36029125858): a same-volume move keeps the item's **own** ACL. The protected SID folder stops other users from listing what is kept (a second standard user, and even an administrator, got access denied), but a user who knows an item's full path can still open it if its own ACL allowed that before (bypass traverse checking). Duster does **not** rewrite item ACLs, because restore must bring back exactly the original permissions. The guarantee is: other users cannot list what you have kept, and nothing becomes readable that was not readable before.
- **Unsupported locations.** Network drives, and volumes where the root cannot be created, get no quarantine. The item is left in place and reported, unless the command's existing permanent path is explicitly requested (see §4). Nothing is ever copied across volumes.
- **Session.** One command run is one session, with id `<unixnanos>-<command>`. On each volume it touches, the session is a folder of numbered slots (`1\<original name>`, `2\...`) plus `session.json`:

  ```json
  {"id":"...","command":"purge","created":"<RFC3339>",
   "items":[{"slot":1,"path":"D:\\proj\\node_modules","size":123,
             "dir":true,"state":"kept","at":"<RFC3339>"}]}
  ```

  `state` is one of `pending`, `kept`, `restored` or `expired`. The manifest records each item's size, not its file count: a caller that already knows the size (every call site does) passes it, and nothing walks the tree again just to count files.
- **Crash safety.** An item is written `pending` before its move and `kept` after. On load:
  - a `pending` item whose slot exists is treated as `kept`;
  - a `pending` item whose slot is missing is treated as never moved.

  `session.json` is written atomically (temp file plus rename, as analyze history does).
- **Safety.** Before a move, the source passes every existing delete check: `fs.IsValidPath`, `Lstat` with no link following (a link is moved as the link itself), and OneDrive placeholders skipped via `fs.IsOfflineInfo`. A path inside any quarantine root is never quarantined again, and clean categories never walk into one. Every move is logged via `logging.LogDestructiveOperation(<command>, "quarantine", ...)`, once per action, at the call site that already logs today, not inside `quarantinePath` itself: that keeps one log line per user-visible action instead of one per helper call, matching every other destructive path in `CLAUDE.md`. Restore, expire and empty are logged the same way, from inside `quarantine.go`, with command `restore`.

## 3. Commands

`du restore`:

| Form | Effect |
|---|---|
| `du restore` | Lists sessions from the last 7 days, newest first: number, time, command, item count, size and expiry. The footer shows the total held per drive |
| `du restore <n>` | Restores every kept item in session `n` |
| `du restore <n> --item <k>` | Restores one item |
| `--dry-run` | Shows what would come back and what would be skipped |
| `--json` | Works with every form |
| `du restore --empty [<n>]` | Deletes kept items now. Asks first, naming each session it would delete (command, time, item count, size); needs `--yes` when not interactive |

Wherever `<n>` is taken it can also be the session id that `--json` prints. A number can shift when a new session is kept between listing and acting; an id always names the same session.

A session on a drive that isn't currently attached (an unplugged USB drive) is not listed as "not connected": finding it without reading every removable drive's quarantine on every `du restore` would need a central index that has to be kept in sync with drives that come and go, which is more machinery than the case is worth. The listing shows only sessions on drives Duster can currently reach; plugging the drive back in makes them reappear (§7 covers this in the release checklist).

Restore rules:
- The item's original path must pass `fs.IsValidPath`. Missing parent folders are created.
- The move back uses a rename that refuses an existing target at the OS level: `MoveFileEx` without `MOVEFILE_REPLACE_EXISTING`, not `os.Rename`.
- A target that exists is skipped and reported ("a newer one is there"). The item stays kept.
- Each item's state is updated in `session.json`. Every restore is logged as `restore`.

## 4. Call sites

| Command | Today | After |
|---|---|---|
| `purge` (TUI and headless) | `purgePermanentPath` (permanent) | quarantine. New `--permanent` flag keeps the old behavior |
| `purge --safe` | `purgeRecyclePath` (Recycle Bin) | Recycle Bin; if the bin does not take the item, quarantine |
| uninstall leftover sweep (`runSweepCmd`) | `removeAllSafe` (permanent) | quarantine |
| `installer` sweep (`runSetupSweepCmd`) | `removeFileSafe` (permanent) | quarantine |
| `analyze d` (`recyclePath`) | Recycle Bin; too big → Windows asks to delete permanently, No → error | Recycle Bin; if the bin does not take the item, quarantine |
| `clean`, scheduled cleans, `vdisk`, `remove` | unchanged | unchanged |

Other changes:
- "The bin does not take the item" means `recyclePathNative` returned an error. That happens when the item is too big and the user answers No in Windows' own "permanently delete?" dialog (Duster passes `FOF_WANTNUKEWARNING`, so Windows asks first; Duster cannot know in advance whether an item fits), or when the path is longer than the Recycle Bin API accepts (MAX_PATH). Answering No to *permanent* deletion therefore means "remove it, but keep it restorable". Answering Yes is Windows deleting permanently at the user's explicit request, as today.
- All call sites use one helper, `quarantinePath(s *quarantineSession, path string, size int64) error` (size is the byte count the caller already measured, recorded in the manifest), with the session opened once per command run.
- Finish screens and JSON gain the session number and the line "Kept for 7 days. Undo with `du restore <n>`".
- `du remove` empties every quarantine: `logging.Dir()` already covers the local one, and each fixed drive's SID folder is emptied too. Its confirmation states how much is held.

## 5. Retention sweep

`sweepQuarantine(now, mode)` runs:
- at the start of every command that opens a session (full sweep);
- on every `du schedule run` (full sweep);
- on every `du restore` except a dry run, with expiry only (step 1). `du restore` never runs step 2: the user came to get something back, and on a nearly full drive step 2 could otherwise delete the very session they are about to restore (for example, `du purge` then `du restore` on a full disk).

What it removes:
1. Items kept more than 7 days, logged as `expire`.
2. Full sweep only: on any volume below 10% free, the oldest sessions on that volume, one at a time, until the volume is at or above 10% or its quarantine is empty. Space freed by the expired sessions of step 1 counts first, so a fresh session is not deleted when expiry already made room. A volume at 0 bytes free is still swept; only a volume whose size cannot be read is left to step 1.

Which sessions to delete is decided by a pure function over (sessions, free space per volume, now, whether step 2 applies), so it can be unit-tested.

The sweep returns what it removed, split by reason (expired, or low space on which drives), with the sessions and bytes. `du restore` prints the expired count and size ("Removed 2 expired sessions (3.1 GB)."). The purge, uninstall and installer finish screens, `du purge --yes` and `du schedule run` report low-space removals in one line, and `du purge --json --yes` adds a `swept_low_space` object (sessions, bytes, volumes, note).

The sweep only deletes inside verified roots, with `removeAllSafe`. A session with a missing or damaged `session.json`, or one without a creation time, is aged by its folder's modification time.

## 6. Units

| File | Responsibility |
|---|---|
| `cmd/quarantine.go` | Session and manifest types, open/add/load, the sweep picker (pure), restore planning (pure), `quarantinePath`, `sweepQuarantine`, `restoreItems` |
| `cmd/quarantine_windows.go` | Quarantine root per volume (drive type, volume path, private folder with a protected DACL, hidden attribute), no-overwrite move |
| `cmd/stubs.go` | Non-Windows versions so tests run anywhere: every path's root is `logging.Dir()\quarantine`, and the move is `Lstat` (target must not exist) then `os.Rename` |
| `cmd/restore.go` | `du restore` command, text and JSON output |
| Call sites | `purge.go`, `uninstall.go`, `installer.go`, `analyze.go`, `remove.go`, `schedule.go` (sweep) |

## 7. Testing

- **Unit tests (temp folders):**
  - manifest crash states;
  - sweep picker (expiry, low space, oldest first, damaged manifest);
  - restore conflicts and the no-overwrite move;
  - link refusal at roots and slots;
  - session numbering;
  - each call site routes to `quarantinePath` (dry-run paths still touch nothing).
- **Windows smoke:**
  - `purge` of a real `node_modules` on C: and on D:, then `du restore`, then `du restore --empty`;
  - a standard user's D: SID folder is unreadable to a second standard user;
  - a restore onto an existing target is skipped;
  - the `analyze` too-big fallback: `TestRecycleBinTooSmallAsksFirst` still answers No in Windows' dialog, and now expects the file gone from its folder, kept in the quarantine and back after `du restore`.
- **Manual (release checklist):** a USB drive unplugged then replugged; a low-space sweep on a nearly full drive.

## 8. Documentation

- README: a row for `du restore` and a short section.
- CHANGELOG.
- `docs/security.md`: a new section on quarantine privacy and the no-overwrite restore.
- The release checklist.
- `CLAUDE.md`: the command table, "user-facing deletes go through `quarantinePath`" as a safety invariant, and the known gaps.
