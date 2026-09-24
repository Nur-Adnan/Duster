# Undo Window (`du restore`) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** User-facing deletes (`purge`, uninstall leftovers, `installer` sweep, and the Recycle Bin fallback of `analyze d` / `purge --safe`) move items into a per-volume Duster quarantine that `du restore` can put back for 7 days.

**Architecture:** `cmd/quarantine.go` holds all logic: sessions, the manifest, keeping an item, loading, the pure sweep picker, restore and empty. It is OS-neutral apart from three volume-level primitives:
- `quarantineRoot`, `quarantineRoots` and `moveNoReplace`, implemented in `cmd/quarantine_windows.go`;
- test-friendly versions of the same three in `cmd/stubs.go`.

`cmd/restore.go` is the command. Five call sites switch to `quarantinePath` / `recycleOrQuarantine`.

**Tech Stack:** Go 1.25 stdlib and `golang.org/x/sys/windows` v0.47.0, already a dependency (verified: `CreateDirectory`, `SecurityDescriptorFromString`, `GetVolumePathName`, `GetDriveType`, `GetVolumeInformation`, `GetNamedSecurityInfo`, `MoveFileEx`, `SetFileAttributes`, `GetCurrentProcessToken().GetTokenUser()`). No new dependencies.

**Spec:** `docs/superpowers/specs/2026-09-24-undo-restore-design.md`

## Execution waves (for parallel agents)

| Wave | Tasks (run in parallel) | Files owned |
|---|---|---|
| 1 | T1 core; T2 Windows layer | T1: `cmd/quarantine.go`, `cmd/quarantine_test.go`, `cmd/stubs.go`. T2: `cmd/quarantine_windows.go`, `cmd/quarantine_windows_test.go` |
| 2 | T3 restore cmd; T4 purge; T5 uninstall+installer; T6 analyze; T7 remove+schedule | T3: `cmd/restore.go`, `cmd/restore_test.go`, `main.go`. T4: `cmd/purge.go`, `cmd/purge_quarantine_test.go`. T5: `cmd/uninstall.go`, `cmd/installer.go`, `cmd/sweep_quarantine_test.go`. T6: `cmd/analyze.go`, `e2e/tui_windows_test.go`. T7: `cmd/remove.go`, `cmd/schedule.go`, `cmd/remove_quarantine_test.go` |
| 3 | T8 smoke + docs | `.github/workflows/windows-smoke.yml`, `README.md`, `CHANGELOG.md`, `docs/security.md`, `docs/release-checklist.md`, the spec |

Every agent commits only its own files with `git commit -m "..." -- <paths>` and never uses `git add -A`, stash, reset or switch.

## Global Constraints

- **Toolchain:** Go 1.25, `CGO_ENABLED=0`, only GOOS=windows ships. Every Windows-only symbol gets a stub in `cmd/stubs.go` (`//go:build !windows`).
- **Gates:**
  - `gofmt -s -l .` prints nothing;
  - `go vet ./...` and `GOOS=windows go vet ./...`;
  - `~/go/bin/staticcheck ./...` and `GOOS=windows ~/go/bin/staticcheck ./...`;
  - `DU_NO_OPLOG=1 go test ./...`.
- **No destructive runs here:** never run destructive `du` commands on this machine. Tests use `t.TempDir()`, and anything that touches `logging.Dir()` must `t.Setenv("LOCALAPPDATA", t.TempDir())` first.
- **Tests never change the machine:** unit tests must not create folders outside temp dirs. No test may create `X:\.duster-quarantine` on a real drive.
- **`min` is shadowed:** package cmd defines `func min(a, b int) int`, which shadows the builtin.
- **Same-line nosemgrep:** every `exec.Command` with a non-constant argument needs `// nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command` on the same line.
- **Retention:** 7 days (`quarantineKeep = 7 * 24 * time.Hour`). Low-space line: 10% free (`quarantineLowSpace = 10.0`).
- **Where the quarantine lives:**
  - on the volume of `%LOCALAPPDATA%`: `logging.Dir()\quarantine`;
  - any other fixed or removable drive: `X:\.duster-quarantine\<user SID>`. `.duster-quarantine` is hidden, and the SID folder has a protected DACL: `D:P(A;OICI;FA;;;<SID>)(A;OICI;FA;;;SY)`;
  - network drives: none.
- **Session layout:** a session folder `<unixnanos>-<command>`, slots `<n>\<original base name>`, and `session.json` written atomically. Item states: `pending`, `kept`, `restored`.
- **Restore never overwrites:** it uses `moveNoReplace`, which is `MoveFileEx` without `MOVEFILE_REPLACE_EXISTING`, never `os.Rename`.
- **Fail-safe deletes:** a failed quarantine move leaves the item in place and reports it. Nothing silently becomes permanent.
- **Logging:** each destructive action is logged exactly once, by the call site that already logs today (action `quarantine` instead of `delete`/`sweep`). Restore, expire and empty log in `quarantine.go` with command `restore`.
- **Copy:** user-visible copy for undo reads: `Kept for 7 days: du restore lists it, du restore 1 puts it back.` No em-dashes.

## Review Focus

1. **Unsupported volume.** An item on a network drive or unsupported volume stays exactly where it was, with a clear error, and is never deleted permanently. Test in T1.
2. **Deleted parent.** Restoring an item whose parent folder was deleted meanwhile recreates the parent and restores. Test in T1.
3. **Crash between write and move.** A crash between writing `pending` and the move: the item is found either in place (the entry is dropped on load) or in its slot (treated as kept). Test in T1.
4. **Session on two volumes.** A session that touched two volumes is one entry in `du restore` and restores both parts. Test in T1 (pure grouping).
5. **Link items.** A symlink or junction item is moved as the link itself; its target is untouched. Test in T1 (skip where symlinks are unsupported).

## Deviations from the spec (decided while planning)

- **No "not connected" listing.** A session on a drive that is not attached cannot be discovered without a central index, so the listing shows only connected drives. The release checklist covers replugging.
- **Logging stays at the call sites** (once per action, CLAUDE.md), not inside `quarantinePath`. The manifest records size but not a file count.
- **Expiry is per session** (all its items are kept within the same run).
- **`quarantinePath` takes a size:** `quarantinePath(s *quarantineSession, path string, size int64) error`. The size comes from the caller, which already has it.

---

### Task 1: Quarantine core (`cmd/quarantine.go`)

**Files:** Create `cmd/quarantine.go`, `cmd/quarantine_test.go`. Modify `cmd/stubs.go` (append the three non-Windows primitives).

**Interfaces:**
- Consumes:
  - `fs.IsValidPath`, `fs.IsOfflineInfo` (lib/fs);
  - `ensureRealDir`, `realDir` (cmd/analyze_history.go);
  - `removeAllSafe` (cmd/utils.go);
  - `recyclePathNative` (cmd/recycle_windows.go, stub in stubs.go);
  - `getDiskFreeBytes`, `diskFreePercent` (cmd/styles.go, cmd/diskfree_windows.go);
  - `logging.Dir`, `logging.LogDestructiveOperation`.
- Produces (used by T2-T7):
  - `const quarantineKeep`, `const quarantineLowSpace`;
  - `var errNoQuarantine error`;
  - `type quarantineItem struct{ Slot int; Path string; Size int64; Dir bool; State string; At time.Time }`;
  - `type quarantineManifest struct{ ID, Command string; Created time.Time; Items []quarantineItem }`;
  - `type quarantineSession` with `newQuarantineSession(command string) *quarantineSession` and `(*quarantineSession).Kept() int`;
  - `quarantinePath(s *quarantineSession, path string, size int64) error`;
  - `recycleOrQuarantine(s *quarantineSession, path string, size int64) (quarantined bool, err error)`;
  - `type keptSession struct{ Root, Dir string; Manifest quarantineManifest; Damaged bool; Created time.Time }`;
  - `loadKeptSessions(roots []string) []keptSession`;
  - `type restoreSession struct{ ID, Command string; Created time.Time; Parts []keptSession }` and `groupSessions([]keptSession) []restoreSession` (newest first);
  - `(restoreSession).Items() []keptItemRef` and `(restoreSession).Size() int64`;
  - `type keptItemRef struct{ Part int; Index int; Item quarantineItem }`;
  - `type volSpace struct{ Free, Total int64 }` and `pickSweep(ks []keptSession, vols map[string]volSpace, now time.Time) []string` (pure; returns session dirs);
  - `sweepQuarantine(now time.Time) []error`;
  - `type restoreResult struct{ Path string; Size int64; Status, Reason string }`;
  - `restoreItems(rs restoreSession, item int, dryRun bool) []restoreResult` (item 0 = all, else a 1-based number in `rs.Items()` order);
  - `emptySessions(rs []restoreSession) error`;
  - `quarantineHeld() int64`.
  - Primitives (T2 on Windows, this task's stubs elsewhere): `quarantineRoot(path string) (string, error)`, `quarantineRoots() []string`, `moveNoReplace(from, to string) error`.

- [ ] **Step 1: Write the failing tests** (`cmd/quarantine_test.go`)

```go
package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func tempQuarantine(t *testing.T) string {
	t.Helper()
	t.Setenv("LOCALAPPDATA", t.TempDir())
	t.Setenv("DU_NO_OPLOG", "1")
	return t.TempDir()
}

func writeTree(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sub", "a.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestQuarantineAndRestoreRoundTrip(t *testing.T) {
	work := tempQuarantine(t)
	target := filepath.Join(work, "proj", "node_modules")
	writeTree(t, target)

	s := newQuarantineSession("purge")
	if err := quarantinePath(s, target, 5); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatal("the item is still in place after being kept")
	}
	if s.Kept() != 1 {
		t.Errorf("Kept() = %d", s.Kept())
	}

	sessions := groupSessions(loadKeptSessions(quarantineRoots()))
	if len(sessions) != 1 || sessions[0].Command != "purge" || len(sessions[0].Items()) != 1 {
		t.Fatalf("sessions: %+v", sessions)
	}
	if got := sessions[0].Size(); got != 5 {
		t.Errorf("Size() = %d", got)
	}

	res := restoreItems(sessions[0], 0, false)
	if len(res) != 1 || res[0].Status != "restored" {
		t.Fatalf("restore: %+v", res)
	}
	if b, err := os.ReadFile(filepath.Join(target, "sub", "a.txt")); err != nil || string(b) != "hello" {
		t.Fatalf("restored content: %q, %v", b, err)
	}
	if left := groupSessions(loadKeptSessions(quarantineRoots())); len(left) != 0 {
		t.Errorf("a fully restored session is still listed: %+v", left)
	}
}

func TestRestoreNeverOverwrites(t *testing.T) {
	work := tempQuarantine(t)
	target := filepath.Join(work, "installer.exe")
	os.WriteFile(target, []byte("old"), 0o644)
	s := newQuarantineSession("installer")
	if err := quarantinePath(s, target, 3); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(target, []byte("newer"), 0o644) // recreated meanwhile

	rs := groupSessions(loadKeptSessions(quarantineRoots()))[0]
	res := restoreItems(rs, 0, false)
	if res[0].Status != "skipped" || !strings.Contains(res[0].Reason, "newer") {
		t.Fatalf("restore over an existing item: %+v", res)
	}
	if b, _ := os.ReadFile(target); string(b) != "newer" {
		t.Fatalf("the newer file was overwritten: %q", b)
	}
	if again := groupSessions(loadKeptSessions(quarantineRoots())); len(again) != 1 {
		t.Error("the skipped item must stay kept")
	}
}

func TestRestoreRecreatesParent(t *testing.T) {
	work := tempQuarantine(t)
	target := filepath.Join(work, "gone", "deep", "file.txt")
	os.MkdirAll(filepath.Dir(target), 0o755)
	os.WriteFile(target, []byte("x"), 0o644)
	s := newQuarantineSession("uninstall")
	if err := quarantinePath(s, target, 1); err != nil {
		t.Fatal(err)
	}
	os.RemoveAll(filepath.Join(work, "gone"))

	rs := groupSessions(loadKeptSessions(quarantineRoots()))[0]
	if res := restoreItems(rs, 0, false); res[0].Status != "restored" {
		t.Fatalf("%+v", res)
	}
	if _, err := os.Stat(target); err != nil {
		t.Fatal(err)
	}
}

func TestRestoreDryRunAndSingleItem(t *testing.T) {
	work := tempQuarantine(t)
	a, b := filepath.Join(work, "a.txt"), filepath.Join(work, "b.txt")
	os.WriteFile(a, []byte("a"), 0o644)
	os.WriteFile(b, []byte("b"), 0o644)
	s := newQuarantineSession("installer")
	quarantinePath(s, a, 1)
	quarantinePath(s, b, 1)

	rs := groupSessions(loadKeptSessions(quarantineRoots()))[0]
	if res := restoreItems(rs, 0, true); len(res) != 2 || res[0].Status != "would restore" {
		t.Fatalf("dry run: %+v", res)
	}
	if _, err := os.Stat(a); !os.IsNotExist(err) {
		t.Fatal("dry run restored something")
	}
	if res := restoreItems(rs, 2, false); len(res) != 1 || res[0].Path != b {
		t.Fatalf("--item 2: %+v", res)
	}
	if _, err := os.Stat(a); !os.IsNotExist(err) {
		t.Fatal("--item 2 also restored item 1")
	}
}

func TestQuarantineRefusals(t *testing.T) {
	work := tempQuarantine(t)
	s := newQuarantineSession("purge")
	if err := quarantinePath(s, filepath.Join(work, "missing"), 0); err == nil {
		t.Error("kept a path that does not exist")
	}
	root, err := quarantineRoot(work)
	if err != nil {
		t.Fatal(err)
	}
	inside := filepath.Join(root, "x.txt")
	os.WriteFile(inside, []byte("x"), 0o644)
	if err := quarantinePath(s, inside, 1); err == nil {
		t.Error("kept an item that is already in the quarantine")
	}
}

func TestQuarantineMovesLinkNotTarget(t *testing.T) {
	work := tempQuarantine(t)
	target := filepath.Join(work, "real")
	writeTree(t, target)
	link := filepath.Join(work, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unsupported: %v", err)
	}
	s := newQuarantineSession("purge")
	if err := quarantinePath(s, link, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(target, "sub", "a.txt")); err != nil {
		t.Fatal("keeping a link touched its target")
	}
}

func TestLoadRecoversCrashStates(t *testing.T) {
	work := tempQuarantine(t)
	root, err := quarantineRoot(work)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "1-purge")
	os.MkdirAll(filepath.Join(dir, "1"), 0o700)
	os.WriteFile(filepath.Join(dir, "1", "moved"), []byte("x"), 0o644) // pending, but the move happened
	m := quarantineManifest{ID: "1-purge", Command: "purge", Created: time.Now(), Items: []quarantineItem{
		{Slot: 1, Path: filepath.Join(work, "moved"), State: "pending"},
		{Slot: 2, Path: filepath.Join(work, "never"), State: "pending"}, // crashed before its move
	}}
	if err := writeManifest(dir, &m); err != nil {
		t.Fatal(err)
	}
	ks := loadKeptSessions([]string{root})
	if len(ks) != 1 || len(ks[0].Manifest.Items) != 1 || ks[0].Manifest.Items[0].State != "kept" {
		t.Fatalf("crash recovery: %+v", ks)
	}

	os.MkdirAll(filepath.Join(root, "2-installer"), 0o700)
	os.WriteFile(filepath.Join(root, "2-installer", quarantineManifestName), []byte("{broken"), 0o600)
	ks = loadKeptSessions([]string{root})
	var damaged bool
	for _, k := range ks {
		damaged = damaged || k.Damaged
	}
	if !damaged {
		t.Error("a damaged manifest must still be listed (so it can be emptied)")
	}
}

func TestGroupSessionsAcrossVolumes(t *testing.T) {
	now := time.Now()
	mk := func(root, id string, at time.Time) keptSession {
		return keptSession{Root: root, Dir: filepath.Join(root, id), Created: at,
			Manifest: quarantineManifest{ID: id, Command: "purge", Created: at,
				Items: []quarantineItem{{Slot: 1, Path: root + "x", Size: 10, State: "kept"}}}}
	}
	got := groupSessions([]keptSession{
		mk("C", "1-purge", now.Add(-2*time.Hour)),
		mk("D", "2-purge", now.Add(-time.Hour)),
		mk("C", "2-purge", now.Add(-time.Hour)),
	})
	if len(got) != 2 || got[0].ID != "2-purge" || len(got[0].Parts) != 2 || got[0].Size() != 20 {
		t.Fatalf("grouping: %+v", got)
	}
}

func TestPickSweep(t *testing.T) {
	now := time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)
	ks := []keptSession{
		{Root: "C", Dir: "C/old", Created: now.Add(-8 * 24 * time.Hour), Manifest: quarantineManifest{Items: []quarantineItem{{Size: 1, State: "kept"}}}},
		{Root: "C", Dir: "C/new", Created: now.Add(-time.Hour), Manifest: quarantineManifest{Items: []quarantineItem{{Size: 1, State: "kept"}}}},
		{Root: "D", Dir: "D/a", Created: now.Add(-3 * 24 * time.Hour), Manifest: quarantineManifest{Items: []quarantineItem{{Size: 30, State: "kept"}}}},
		{Root: "D", Dir: "D/b", Created: now.Add(-2 * 24 * time.Hour), Manifest: quarantineManifest{Items: []quarantineItem{{Size: 30, State: "kept"}}}},
	}
	vols := map[string]volSpace{"C": {Free: 50, Total: 100}, "D": {Free: 5, Total: 100}} // D at 5% free
	got := strings.Join(pickSweep(ks, vols, now), ",")
	if got != "C/old,D/a" {
		t.Errorf("pickSweep = %q, want expired C/old then oldest D/a (5%%+30%% >= 10%%)", got)
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `DU_NO_OPLOG=1 go test ./cmd -run 'Quarantine|Restore|LoadRecovers|GroupSessions|PickSweep' 2>&1 | tail -5`
Expected: build failure (undefined `newQuarantineSession` ...).

- [ ] **Step 3: Append the non-Windows primitives to `cmd/stubs.go`** (add imports as needed: `errors`, `fmt`, `os`, `path/filepath`, `github.com/Nur-Adnan/duster/internal/logging`)

```go
// Non-Windows quarantine primitives: one local root, so tests run anywhere.
func quarantineRoot(string) (string, error) {
	d := logging.Dir()
	if d == "" {
		return "", errNoQuarantine
	}
	for _, p := range []string{d, filepath.Join(d, "quarantine")} {
		if err := ensureRealDir(p); err != nil {
			return "", err
		}
	}
	return filepath.Join(d, "quarantine"), nil
}

func quarantineRoots() []string {
	if d := logging.Dir(); d != "" && realDir(filepath.Join(d, "quarantine")) {
		return []string{filepath.Join(d, "quarantine")}
	}
	return nil
}

func moveNoReplace(from, to string) error {
	if _, err := os.Lstat(to); err == nil {
		return fmt.Errorf("%s: %w", to, os.ErrExist)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return os.Rename(from, to)
}
```

- [ ] **Step 4: Implement `cmd/quarantine.go`**

```go
package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Nur-Adnan/duster/internal/logging"
	"github.com/Nur-Adnan/duster/lib/fs"
)

// The undo window: user-facing deletes move items into a quarantine on the
// item's own volume (a rename: no copy, permissions kept) that `du restore`
// can put back for quarantineKeep. See docs/superpowers/specs/2026-09-24-undo-restore-design.md.

const (
	quarantineKeep         = 7 * 24 * time.Hour
	quarantineLowSpace     = 10.0 // % free: below it the oldest sessions on that volume go first
	quarantineManifestName = "session.json"
	quarantineDirName      = ".duster-quarantine"
)

var errNoQuarantine = errors.New("this drive has no Duster quarantine (network drive?), so the item was left in place")

type quarantineItem struct {
	Slot  int       `json:"slot"`
	Path  string    `json:"path"`
	Size  int64     `json:"size"`
	Dir   bool      `json:"dir"`
	State string    `json:"state"` // pending, kept, restored
	At    time.Time `json:"at"`
}

type quarantineManifest struct {
	ID      string           `json:"id"`
	Command string           `json:"command"`
	Created time.Time        `json:"created"`
	Items   []quarantineItem `json:"items"`
}

// quarantineSession is one command run. Its folder on a volume is created the
// first time an item from that volume is kept.
type quarantineSession struct {
	mu      sync.Mutex
	id      string
	command string
	created time.Time
	dirs    map[string]*quarantineManifest // session folder -> its manifest
	kept    int
}

func newQuarantineSession(command string) *quarantineSession {
	now := time.Now()
	return &quarantineSession{
		id: fmt.Sprintf("%d-%s", now.UnixNano(), command), command: command,
		created: now, dirs: map[string]*quarantineManifest{},
	}
}

// Kept is how many items this session has kept so far.
func (s *quarantineSession) Kept() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.kept
}

// insideQuarantine reports whether path is in a quarantine root, so a kept
// item is never kept again.
func insideQuarantine(path string) bool {
	p := strings.ToLower(filepath.Clean(path))
	sep := string(filepath.Separator)
	if strings.Contains(p+sep, sep+quarantineDirName+sep) {
		return true
	}
	if d := logging.Dir(); d != "" {
		q := strings.ToLower(filepath.Join(d, "quarantine"))
		return p == q || strings.HasPrefix(p, q+sep)
	}
	return false
}

// quarantinePath moves path into the session's folder on the same volume.
// The item is recorded as pending before the move and kept after, so a crash
// leaves it either in place or in its slot, never lost. The caller logs.
func quarantinePath(s *quarantineSession, path string, size int64) error {
	if !fs.IsValidPath(path) {
		return fmt.Errorf("deleting system protected paths is blocked for safety")
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if fs.IsOfflineInfo(info) {
		return fmt.Errorf("%s is a OneDrive online-only file, so it was left in place", path)
	}
	if insideQuarantine(path) {
		return fmt.Errorf("%s is already in Duster's quarantine", path)
	}
	root, err := quarantineRoot(path)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	dir := filepath.Join(root, s.id)
	m := s.dirs[dir]
	if m == nil {
		if err := ensureRealDir(dir); err != nil {
			return err
		}
		m = &quarantineManifest{ID: s.id, Command: s.command, Created: s.created}
		s.dirs[dir] = m
	}
	slot := len(m.Items) + 1
	isDir := info.IsDir() && info.Mode()&(os.ModeSymlink|os.ModeIrregular) == 0
	m.Items = append(m.Items, quarantineItem{Slot: slot, Path: path, Size: size, Dir: isDir, State: "pending", At: time.Now()})
	undo := func() { m.Items = m.Items[:len(m.Items)-1]; _ = writeManifest(dir, m) }
	if err := writeManifest(dir, m); err != nil {
		m.Items = m.Items[:len(m.Items)-1]
		return err
	}
	slotDir := filepath.Join(dir, strconv.Itoa(slot))
	if err := os.Mkdir(slotDir, 0o700); err != nil {
		undo()
		return err
	}
	if err := moveNoReplace(path, filepath.Join(slotDir, filepath.Base(path))); err != nil {
		os.Remove(slotDir)
		undo()
		return fmt.Errorf("could not move %s into Duster's quarantine, so it was left in place: %w", path, err)
	}
	m.Items[slot-1].State = "kept"
	_ = writeManifest(dir, m) // on failure the item stays pending with its slot filled: load treats that as kept
	s.kept++
	return nil
}

// recycleOrQuarantine sends path to the Recycle Bin and, when the bin does not
// take it (too big and the user said No to Windows' permanent-delete prompt,
// or a path too long for the bin's API), keeps it in the quarantine instead.
func recycleOrQuarantine(s *quarantineSession, path string, size int64) (bool, error) {
	rerr := recyclePathNative(path)
	if rerr == nil {
		return false, nil
	}
	if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) {
		return false, nil // Windows deleted it (the user answered Yes to its prompt)
	}
	if qerr := quarantinePath(s, path, size); qerr != nil {
		return false, fmt.Errorf("%v; keeping it in Duster's quarantine also failed: %v", rerr, qerr)
	}
	return true, nil
}

func writeManifest(dir string, m *quarantineManifest) error {
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".session-*")
	if err != nil {
		return err
	}
	_, werr := tmp.Write(b)
	if err := errors.Join(werr, tmp.Close()); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	if err := os.Rename(tmp.Name(), filepath.Join(dir, quarantineManifestName)); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return nil
}

// keptSession is one session folder on one volume.
type keptSession struct {
	Root     string
	Dir      string
	Manifest quarantineManifest
	Damaged  bool      // session.json missing or unreadable: aged by folder time, emptiable only
	Created  time.Time // manifest time, or the folder's when damaged
}

// slotPath is where item it of session k is stored.
func (k keptSession) slotPath(it quarantineItem) string {
	return filepath.Join(k.Dir, strconv.Itoa(it.Slot), filepath.Base(it.Path))
}

// loadKeptSessions reads every session folder under roots. Pending items whose
// slot is filled are treated as kept; pending items with an empty slot were
// never moved and are dropped. Restored items are dropped from the listing.
func loadKeptSessions(roots []string) []keptSession {
	var out []keptSession
	for _, root := range roots {
		if !realDir(root) {
			continue
		}
		entries, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, e := range entries {
			dir := filepath.Join(root, e.Name())
			if !realDir(dir) {
				continue
			}
			k := keptSession{Root: root, Dir: dir}
			b, err := readSmallFile(filepath.Join(dir, quarantineManifestName), 4<<20)
			if err != nil || json.Unmarshal(b, &k.Manifest) != nil || k.Manifest.ID == "" {
				k.Damaged = true
				if info, err := os.Lstat(dir); err == nil {
					k.Created = info.ModTime()
				}
				k.Manifest = quarantineManifest{ID: e.Name()}
				out = append(out, k)
				continue
			}
			k.Created = k.Manifest.Created
			var items []quarantineItem
			for _, it := range k.Manifest.Items {
				if it.State == "pending" {
					if _, err := os.Lstat(k.slotPath(it)); err != nil {
						continue
					}
					it.State = "kept"
				}
				if it.State == "kept" {
					items = append(items, it)
				}
			}
			if len(items) == 0 {
				continue
			}
			k.Manifest.Items = items
			out = append(out, k)
		}
	}
	return out
}

func readSmallFile(path string, limit int64) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > limit {
		return nil, fmt.Errorf("%s is not a small regular file", path)
	}
	return os.ReadFile(path)
}

// restoreSession is one command run's kept items across every volume.
type restoreSession struct {
	ID      string
	Command string
	Created time.Time
	Parts   []keptSession
}

type keptItemRef struct {
	Part  int
	Index int
	Item  quarantineItem
}

// Items lists the session's kept items in a stable order (numbered from 1 by `du restore`).
func (r restoreSession) Items() []keptItemRef {
	var out []keptItemRef
	for p, k := range r.Parts {
		for i, it := range k.Manifest.Items {
			out = append(out, keptItemRef{Part: p, Index: i, Item: it})
		}
	}
	return out
}

func (r restoreSession) Size() int64 {
	var n int64
	for _, ref := range r.Items() {
		n += ref.Item.Size
	}
	return n
}

// groupSessions merges one run's folders from every volume, newest first.
func groupSessions(ks []keptSession) []restoreSession {
	byID := map[string]*restoreSession{}
	var order []string
	for _, k := range ks {
		r := byID[k.Manifest.ID]
		if r == nil {
			r = &restoreSession{ID: k.Manifest.ID, Command: k.Manifest.Command, Created: k.Created}
			byID[k.Manifest.ID] = r
			order = append(order, k.Manifest.ID)
		}
		r.Parts = append(r.Parts, k)
	}
	out := make([]restoreSession, 0, len(order))
	for _, id := range order {
		r := byID[id]
		sort.Slice(r.Parts, func(i, j int) bool { return r.Parts[i].Root < r.Parts[j].Root })
		out = append(out, *r)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Created.After(out[j].Created) })
	return out
}

type volSpace struct{ Free, Total int64 }

// pickSweep decides which session folders to delete: every one older than
// quarantineKeep, then on each volume below quarantineLowSpace the oldest
// remaining ones until the volume would be back at or above it. Pure.
func pickSweep(ks []keptSession, vols map[string]volSpace, now time.Time) []string {
	sorted := append([]keptSession(nil), ks...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Created.Before(sorted[j].Created) })
	var out []string
	gone := map[string]bool{}
	for _, k := range sorted {
		if now.Sub(k.Created) > quarantineKeep {
			out = append(out, k.Dir)
			gone[k.Dir] = true
		}
	}
	for _, k := range sorted {
		if gone[k.Dir] {
			continue
		}
		v, ok := vols[k.Root]
		if !ok || v.Total <= 0 || float64(v.Free)*100/float64(v.Total) >= quarantineLowSpace {
			continue
		}
		out = append(out, k.Dir)
		gone[k.Dir] = true
		var size int64
		for _, it := range k.Manifest.Items {
			size += it.Size
		}
		v.Free += size
		vols[k.Root] = v
	}
	return out
}

// sweepQuarantine applies retention to every quarantine on this machine.
func sweepQuarantine(now time.Time) []error {
	roots := quarantineRoots()
	ks := loadKeptSessions(roots)
	vols := map[string]volSpace{}
	for _, r := range roots {
		free, pct := getDiskFreeBytes(r), diskFreePercent(r)
		if pct > 0 {
			vols[r] = volSpace{Free: free, Total: int64(float64(free) * 100 / pct)}
		}
	}
	var errs []error
	for _, dir := range pickSweep(ks, vols, now) {
		err := removeAllSafe(dir)
		logging.LogDestructiveOperation("restore", "expire", dir, 0, err == nil)
		if err != nil {
			errs = append(errs, err)
		}
	}
	return errs
}

type restoreResult struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	Status string `json:"status"` // restored, would restore, skipped, failed
	Reason string `json:"reason,omitempty"`
}

// restoreItems puts a session's items back (item 0 = all, else the 1-based
// number from Items()). It never overwrites: an existing target is skipped
// and the item stays kept. Missing parent folders are created.
func restoreItems(rs restoreSession, item int, dryRun bool) []restoreResult {
	var out []restoreResult
	touched := map[int]bool{}
	for n, ref := range rs.Items() {
		if item != 0 && n+1 != item {
			continue
		}
		k := rs.Parts[ref.Part]
		it := ref.Item
		r := restoreResult{Path: it.Path, Size: it.Size}
		switch {
		case !fs.IsValidPath(it.Path):
			r.Status, r.Reason = "failed", "the original location is protected"
		case exists(it.Path):
			r.Status, r.Reason = "skipped", "a newer one is there"
		case dryRun:
			r.Status = "would restore"
		default:
			err := os.MkdirAll(filepath.Dir(it.Path), 0o755)
			if err == nil {
				err = moveNoReplace(k.slotPath(it), it.Path)
			}
			if err != nil {
				r.Status, r.Reason = "failed", err.Error()
			} else {
				r.Status = "restored"
				rs.Parts[ref.Part].Manifest.Items[ref.Index].State = "restored"
				os.Remove(filepath.Join(k.Dir, strconv.Itoa(it.Slot)))
				touched[ref.Part] = true
			}
			logging.LogDestructiveOperation("restore", "restore", it.Path, it.Size, err == nil)
		}
		out = append(out, r)
	}
	for p := range touched {
		k := rs.Parts[p]
		if allRestored(k.Manifest.Items) {
			_ = removeAllSafe(k.Dir) // nothing kept is left in it
		} else {
			_ = writeManifest(k.Dir, &rs.Parts[p].Manifest)
		}
	}
	return out
}

func allRestored(items []quarantineItem) bool {
	for _, it := range items {
		if it.State != "restored" {
			return false
		}
	}
	return true
}

func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

// emptySessions deletes the given sessions for good.
func emptySessions(rs []restoreSession) error {
	var errs []error
	for _, r := range rs {
		for _, k := range r.Parts {
			err := removeAllSafe(k.Dir)
			logging.LogDestructiveOperation("restore", "empty", k.Dir, r.Size(), err == nil)
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// quarantineHeld is the total size kept on this machine.
func quarantineHeld() int64 {
	var n int64
	for _, r := range groupSessions(loadKeptSessions(quarantineRoots())) {
		n += r.Size()
	}
	return n
}
```

If `exists` already exists in package cmd (grep `func exists(`), use it and drop this copy.

- [ ] **Step 5: Run tests and gates.** `DU_NO_OPLOG=1 go test ./cmd -run 'Quarantine|Restore|LoadRecovers|GroupSessions|PickSweep' -v 2>&1 | tail -30`, then all gates. On `GOOS=windows`, the three primitives come from T2, which is built in parallel. If T2 has not landed yet, `GOOS=windows go vet` fails on undefined `quarantineRoot`, `quarantineRoots` and `moveNoReplace`. In that case, commit after the non-Windows gates pass, and say so in the report. The controller runs the Windows gates once both tasks have landed.

- [ ] **Step 6: Commit** `git commit -m "feat(restore): quarantine core (keep, load, sweep, restore)" -- cmd/quarantine.go cmd/quarantine_test.go cmd/stubs.go`

---

### Task 2: Windows quarantine primitives (`cmd/quarantine_windows.go`)

**Files:** Create `cmd/quarantine_windows.go` (`//go:build windows`) and `cmd/quarantine_windows_test.go` (`//go:build windows`; it runs in CI's Windows Test Suite).

**Interfaces:**
- Consumes: `ensureRealDir`, `realDir`, `logging.Dir()`, `fs.LongPath`, `quarantineDirName` (T1; declare nothing that T1 declares).
- Produces:
  - `quarantineRoot(path string) (string, error)`;
  - `quarantineRoots() []string`;
  - `moveNoReplace(from, to string) error`;
  - helpers `volumeRoot(path string) (string, error)`, `createPrivateDir(path, sid string) error`, `dirOwner(path string) (string, error)`, `currentUserSID() (string, error)`.

Behavior:
- `volumeRoot`: `windows.GetVolumePathName` (see `volumeSerial` in cmd/diskfree_windows.go for the calling pattern), returning, for example, `D:\`.
- `quarantineRoot(path)`:
  1. `vr := volumeRoot(path)`. If `windows.GetDriveType(vr)` is neither `DRIVE_FIXED` nor `DRIVE_REMOVABLE`, return `errNoQuarantine`.
  2. If `vr` equals `volumeRoot(logging.Dir())` (case-insensitive): `ensureRealDir(logging.Dir())`, then `ensureRealDir(logging.Dir()\quarantine)`, and return the latter.
  3. Otherwise:
     - `base := vr + ".duster-quarantine"`, then `ensureRealDir(base)`, then `windows.SetFileAttributes(base, FILE_ATTRIBUTE_HIDDEN)` (best-effort).
     - `dir := base\<SID>`. If it does not exist, `createPrivateDir(dir, sid)`.
     - Then `realDir(dir)` must hold. When the volume keeps ACLs (`GetVolumeInformation` flags include `FILE_PERSISTENT_ACLS`), `dirOwner(dir)` must equal the current SID; otherwise return an error saying the folder belongs to someone else. This stops another user pre-creating a readable folder for you.
     - Return `dir`.
- `createPrivateDir(path, sid)`: `windows.SecurityDescriptorFromString("D:P(A;OICI;FA;;;" + sid + ")(A;OICI;FA;;;SY)")`, then `windows.CreateDirectory(utf16(path), &windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(...)), SecurityDescriptor: sd})`. When the volume has no persistent ACLs (FAT/exFAT), fall back to `os.Mkdir(path, 0o700)`.
- `dirOwner`: `windows.GetNamedSecurityInfo(path, SE_FILE_OBJECT, OWNER_SECURITY_INFORMATION)`, then `sd.Owner()`, then `.String()`.
- `quarantineRoots()`:
  - the local root, if `realDir`;
  - plus, for each letter from `windows.GetLogicalDrives()` whose type is fixed or removable and whose root is not the local volume, `X:\.duster-quarantine\<SID>` if `realDir`.
- `moveNoReplace(from, to)`: `windows.MoveFileEx(utf16(fs.LongPath(from)), utf16(fs.LongPath(to)), 0)`, with no `MOVEFILE_REPLACE_EXISTING` and no `MOVEFILE_COPY_ALLOWED`. The latter means a cross-volume move fails instead of copying.

- [ ] **Step 1: Failing tests** (`cmd/quarantine_windows_test.go`, temp dirs only)

```go
//go:build windows

package cmd

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

func TestMoveNoReplaceRefusesExistingTarget(t *testing.T) {
	d := t.TempDir()
	a, b := filepath.Join(d, "a"), filepath.Join(d, "b")
	os.WriteFile(a, []byte("a"), 0o644)
	os.WriteFile(b, []byte("b"), 0o644)
	if err := moveNoReplace(a, b); err == nil {
		t.Fatal("moveNoReplace overwrote an existing file")
	}
	if got, _ := os.ReadFile(b); string(got) != "b" {
		t.Fatalf("target changed to %q", got)
	}
	c := filepath.Join(d, "c")
	if err := moveNoReplace(a, c); err != nil {
		t.Fatal(err)
	}
}

func TestCreatePrivateDirIsOwnedAndProtected(t *testing.T) {
	sid, err := currentUserSID()
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "priv")
	if err := createPrivateDir(dir, sid); err != nil {
		t.Fatal(err)
	}
	owner, err := dirOwner(dir)
	if err != nil || owner != sid {
		t.Fatalf("owner %q (%v), want %q", owner, err, sid)
	}
	sd, err := windows.GetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	ctrl, _, err := sd.Control()
	if err != nil || ctrl&windows.SE_DACL_PROTECTED == 0 {
		t.Fatalf("DACL not protected (control %#x, %v)", ctrl, err)
	}
}

func TestQuarantineRootOnProfileVolume(t *testing.T) {
	t.Setenv("LOCALAPPDATA", t.TempDir())
	root, err := quarantineRoot(os.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if !realDir(root) || filepath.Base(root) != "quarantine" {
		t.Fatalf("root %q", root)
	}
	_ = errors.Is
}
```

The third test relies on `%TEMP%` and the test's `LOCALAPPDATA` both being on C:, which holds on runners. If they differ, it skips via `volumeRoot` comparison: `t.Skip` when `volumeRoot(os.TempDir()) != volumeRoot(os.Getenv("LOCALAPPDATA"))`.

- [ ] **Step 2: Implement** per the behavior above, with doc comments stating the privacy and no-overwrite guarantees. Use `unsafe.Sizeof(windows.SecurityAttributes{})` for `Length`.

- [ ] **Step 3: Gates.**
  - `GOOS=windows go vet ./...` and `GOOS=windows ~/go/bin/staticcheck ./...`: they need T1's symbols, so if T1 has not landed yet, wait for it or report.
  - `gofmt -s -l .`.
  - The Windows tests run in CI. Locally, `GOOS=windows go test -c -o /dev/null ./cmd` must compile.

- [ ] **Step 4: Commit** `git commit -m "feat(restore): Windows quarantine roots, private folders, no-overwrite move" -- cmd/quarantine_windows.go cmd/quarantine_windows_test.go`

---

### Task 3: `du restore` command (`cmd/restore.go`)

**Files:** Create `cmd/restore.go` and `cmd/restore_test.go`. Modify `main.go`: `rootCmd.AddCommand(cmd.RestoreCmd)` after `cmd.ScheduleCmd`.

**Interfaces:**
- Consumes (T1): `sweepQuarantine`, `loadKeptSessions`, `quarantineRoots`, `groupSessions`, `restoreSession.Items/Size`, `restoreItems`, `emptySessions`, `quarantineKeep`, `restoreResult`.
- Also uses `formatBytes`, `isPiped`.
- Produces: `var RestoreCmd *cobra.Command`, `renderRestoreList(w io.Writer, rs []restoreSession, now time.Time)`, `pickRestoreSession(rs []restoreSession, arg string) (restoreSession, error)`.

Behavior:
- `du restore [n]`, with flags `--item k`, `--dry-run`, `--json`, `--empty`, `--yes`. It always starts with `sweepQuarantine(time.Now())`; errors become warnings.
- **No `n`, no `--empty`:** list. Text:

  ```
  Kept by Duster (restorable for 7 days):
    1  Sep 24, 22:15  purge      3 items   1.2 GB  expires Oct 1
    2  Sep 23, 09:02  uninstall  1 item   80 MB   expires Sep 30
  Held: 1.3 GB. Restore with: du restore <n>   Empty now: du restore --empty
  ```

  When there are no sessions: "Nothing is kept. User-facing deletes (purge, uninstall leftovers, old installers) are kept here for 7 days."

  JSON: `{"sessions":[{"number","id","command","created","expires","items":[{"number","path","size","dir"}],"size"}]}`.
- **`n` given:**
  - `pickRestoreSession` takes a 1-based number from the list; anything else gives the error `no kept session <arg>: run du restore to list them`.
  - Then `restoreItems(rs, item, dryRun)`, printing one line per result (`restored` / `would restore` / `skipped: <reason>` / `failed: <reason>`) plus a summary.
  - Exit 1 if any result failed. A skipped result does not change the exit code.
- **`--empty [n]`:**
  - Which sessions: all, or the one given.
  - Interactive terminal without `--yes`: prompt `Delete <size> kept by Duster for good? [y/N]` and read stdin.
  - Piped or `--json` without `--yes`: refuse with `use --yes to empty the quarantine without a prompt`, exit 1.
  - Otherwise `emptySessions`.
  - `--dry-run` prints what would be emptied.
- `--item` without `n` is an error, as is `--item` with `--empty`.

- [ ] **Step 1: Failing tests** (`cmd/restore_test.go`)

```go
package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRestoreListAndPick(t *testing.T) {
	work := tempQuarantine(t)
	f := filepath.Join(work, "a.txt")
	os.WriteFile(f, []byte("a"), 0o644)
	s := newQuarantineSession("purge")
	if err := quarantinePath(s, f, 1); err != nil {
		t.Fatal(err)
	}
	rs := groupSessions(loadKeptSessions(quarantineRoots()))

	var b bytes.Buffer
	renderRestoreList(&b, rs, time.Now())
	for _, want := range []string{"Kept by Duster", "1 ", "purge", "1 item", "expires", "du restore <n>"} {
		if !strings.Contains(b.String(), want) {
			t.Errorf("list lacks %q:\n%s", want, b.String())
		}
	}
	if _, err := pickRestoreSession(rs, "1"); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"0", "2", "x", ""} {
		if _, err := pickRestoreSession(rs, bad); err == nil {
			t.Errorf("pickRestoreSession(%q) accepted", bad)
		}
	}
	b.Reset()
	renderRestoreList(&b, nil, time.Now())
	if !strings.Contains(b.String(), "Nothing is kept") {
		t.Errorf("empty list text: %s", b.String())
	}
}
```

- [ ] **Step 2: Implement** following `cmd/schedule.go`'s pattern: package-level flag vars, `init()` registering flags, `Run` handlers, and a `restoreFail(err)` helper that prints a JSON error or `Error: ...` and exits 1.

- [ ] **Step 3: Gates.** Also run `go run . restore --help`.

- [ ] **Step 4: Commit** `git commit -m "feat(restore): du restore lists, restores and empties kept sessions" -- cmd/restore.go cmd/restore_test.go main.go`

---

### Task 4: purge keeps instead of deleting (`cmd/purge.go`)

**Files:** Modify `cmd/purge.go`. Create `cmd/purge_quarantine_test.go`.

**Interfaces:**
- Consumes (T1): `newQuarantineSession`, `quarantinePath`, `recycleOrQuarantine`, `sweepQuarantine`, `(*quarantineSession).Kept`.
- Produces: `purgeOne(s *quarantineSession, path string, size int64, safe, permanent bool) (quarantined bool, err error)`.

Changes:
- **New flag** `--permanent`: "Delete permanently instead of keeping items restorable for 7 days". It cannot be combined with `--safe`: refuse with an error in `executePurge` before anything runs.
- **`purgeOne`:**
  - permanent: `purgePermanentPath` (unchanged; it logs `delete`);
  - safe: `recycleOrQuarantine(s, path, size)`, logging `logPurgeOperation("recycle" or "quarantine", ...)` once;
  - default: `quarantinePath(s, path, size)`, logging `logPurgeOperation("quarantine", ...)`.
  - In every case check `fs.IsValidPath` first, as today.
- **`runPurgeCmd` and `runHeadlessPurge`:** create one session per run (only when not dry-run), call `sweepQuarantine(time.Now())` before deleting, and use `purgeOne` for each selected artifact.
- **Text:**
  - The confirm warning (purge.go:573) becomes, unless `--permanent`: `This moves %d selected folders to Duster's quarantine (restorable for 7 days with du restore).` With `--permanent`, keep the old text.
  - Finish screen: when `s.Kept() > 0`, add `Kept for 7 days: du restore lists it, du restore 1 puts it back.`
  - Headless JSON: add `"kept": <count>` and `"undo": "du restore 1"` when anything was kept.
- **Flag help:** `--yes` becomes "Skip interactive prompts and remove all detected build artifacts (restorable for 7 days unless --permanent)".

- [ ] **Step 1: Failing test** (`cmd/purge_quarantine_test.go`)

```go
package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPurgeOneKeepsByDefault(t *testing.T) {
	work := tempQuarantine(t)
	nm := filepath.Join(work, "proj", "node_modules")
	writeTree(t, nm)
	s := newQuarantineSession("purge")
	q, err := purgeOne(s, nm, 5, false, false)
	if err != nil || !q {
		t.Fatalf("default purge: quarantined=%v err=%v", q, err)
	}
	if exists(nm) || len(groupSessions(loadKeptSessions(quarantineRoots()))) != 1 {
		t.Fatal("default purge must move the folder into the quarantine")
	}
}

func TestPurgeOnePermanentDeletes(t *testing.T) {
	work := tempQuarantine(t)
	nm := filepath.Join(work, "proj", "node_modules")
	writeTree(t, nm)
	q, err := purgeOne(newQuarantineSession("purge"), nm, 5, false, true)
	if err != nil || q || exists(nm) {
		t.Fatalf("--permanent: quarantined=%v err=%v exists=%v", q, err, exists(nm))
	}
	if len(groupSessions(loadKeptSessions(quarantineRoots()))) != 0 {
		t.Fatal("--permanent kept something")
	}
}

func TestPurgeOneSafeFallsBackToQuarantine(t *testing.T) {
	// Off Windows recyclePathNative always fails, which exercises the fallback.
	work := tempQuarantine(t)
	nm := filepath.Join(work, "proj", "node_modules")
	writeTree(t, nm)
	q, err := purgeOne(newQuarantineSession("purge"), nm, 5, true, false)
	if err != nil || !q || exists(nm) {
		t.Fatalf("--safe fallback: quarantined=%v err=%v", q, err)
	}
	_ = os.Remove
}
```

On Windows CI, `recyclePathNative` succeeds for a small temp folder. Guard the third test with `if runtime.GOOS == "windows" { t.Skip("the Recycle Bin takes it on Windows") }` so it never touches the real bin (add the `runtime` import).

- [ ] **Step 2: Implement.** **Step 3:** gates. **Step 4:** `git commit -m "feat(restore): purge keeps folders restorable, --permanent for space now" -- cmd/purge.go cmd/purge_quarantine_test.go`

---

### Task 5: uninstall leftovers and installer sweep keep instead of deleting

**Files:** Modify `cmd/uninstall.go` (`runSweepCmd`, around line 249, and its finish view) and `cmd/installer.go` (`runSetupSweepCmd`, around line 217, and its finish view). Create `cmd/sweep_quarantine_test.go`.

**Interfaces:**
- Consumes (T1): `newQuarantineSession`, `quarantinePath`, `sweepQuarantine`, `Kept`.

Changes:
- In both sweep commands, when not dry-run, the returned closure first calls `sweepQuarantine(time.Now())` and creates `s := newQuarantineSession("uninstall")` (or `"installer"`).
- Each selected item uses `quarantinePath(s, item.Path, item.Size)` instead of `removeAllSafe` / `removeFileSafe`.
- Keep the existing log call, changing its action from `"sweep"` to `"quarantine"`.
- Add `kept int` to `sweepCompleteMsg` and `setupSweepCompleteMsg`. Their finish views print `Kept for 7 days: du restore lists it, du restore 1 puts it back.` when `kept > 0`.
- Confirm or warning text that says "permanently" for these sweeps must say the items are kept restorable for 7 days instead (grep both files for "permanent").

- [ ] **Step 1: Failing test** (`cmd/sweep_quarantine_test.go`)

```go
package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUninstallSweepKeepsLeftovers(t *testing.T) {
	work := tempQuarantine(t)
	left := filepath.Join(work, "AppData", "Roaming", "SomeApp")
	writeTree(t, left)
	msg := runSweepCmd([]leftoverItem{{Path: left, Size: 5, Selected: true}}, false)()
	done, ok := msg.(sweepCompleteMsg)
	if !ok || done.kept != 1 || exists(left) {
		t.Fatalf("sweep: %#v, exists=%v", msg, exists(left))
	}
}

func TestInstallerSweepKeepsInstallers(t *testing.T) {
	work := tempQuarantine(t)
	f := filepath.Join(work, "Downloads", "old-setup.exe")
	os.MkdirAll(filepath.Dir(f), 0o755)
	os.WriteFile(f, []byte("MZ"), 0o644)
	msg := runSetupSweepCmd([]installerItem{{Path: f, Size: 2, Selected: true}}, false)()
	done, ok := msg.(setupSweepCompleteMsg)
	if !ok || done.kept != 1 || exists(f) {
		t.Fatalf("installer sweep: %#v", msg)
	}
}
```

Check the exact field names of `leftoverItem` and `installerItem` (grep their `type ... struct`) and adapt the literals.

- [ ] **Step 2: Implement.** **Step 3:** gates. **Step 4:** `git commit -m "feat(restore): uninstall leftovers and old installers are kept restorable" -- cmd/uninstall.go cmd/installer.go cmd/sweep_quarantine_test.go`

---

### Task 6: `analyze d` falls back to the quarantine

**Files:** Modify `cmd/analyze.go` (`recyclePath`, around line 894, its caller in the `"y"` confirm path around lines 315-333, and the status message) and `e2e/tui_windows_test.go` (`TestRecycleBinTooSmallAsksFirst`).

**Interfaces:**
- Consumes (T1): `newQuarantineSession`, `recycleOrQuarantine`.

Changes:
- `analyzeModel` gets `undo *quarantineSession`, created lazily on the first delete.
- `recyclePath(path, size)` becomes `recyclePath(s *quarantineSession, path string, size int64) (kept bool, err error)`:
  - the `IsValidPath` guard is unchanged;
  - the delete is `recycleOrQuarantine`;
  - it logs `logDestructiveOperation(<"quarantine" if kept, else "recycle">, ...)` once.
- On `kept`, the status line reads: `Kept in Duster's quarantine (the Recycle Bin did not take it): du restore puts it back`.
- **e2e:** keep the test's name and flow up to answering No. Then:
  - `tm.waitFor("Kept in Duster's quarantine", 10*time.Second)`;
  - assert `!exists(big)`;
  - quit;
  - run the real binary `exec.Command(duBinPath, "restore", "1")` (see how e2e resolves the binary: `duBin(t)`), with the same-line nosemgrep;
  - assert `exists(big)` again.

  e2e runs only in windows-smoke with `DU_E2E_BIN` set.

- [ ] **Step 1:** Add a unit test for the analyze model, off Windows. Recycling always fails there, so a `d` then `y` on a temp file keeps it: build the model the way the existing analyze tests do (grep `analyzeModel{` in `cmd/*_test.go`), send `d` and `y`, and assert the file is gone and one session is listed. Use `tempQuarantine(t)`. Guard it with `runtime.GOOS == "windows"` → `t.Skip`.
- [ ] **Step 2: Implement.** **Step 3:** gates, plus `GOOS=windows go vet ./e2e/...`. **Step 4:** `git commit -m "feat(restore): analyze keeps what the Recycle Bin will not take" -- cmd/analyze.go cmd/<test file> e2e/tui_windows_test.go`

---

### Task 7: `du remove` empties quarantines; scheduled runs sweep

**Files:** Modify `cmd/remove.go` and `cmd/schedule.go` (only the `scheduleRunCmd` Run func). Create `cmd/remove_quarantine_test.go`.

**Interfaces:**
- Consumes (T1): `loadKeptSessions`, `quarantineRoots`, `groupSessions`, `emptySessions`, `quarantineHeld`, `sweepQuarantine`.

Changes:
- **`remove.go`:**
  - Add `emptyAllQuarantines() error`: `emptySessions(groupSessions(loadKeptSessions(quarantineRoots())))`.
  - Call it in all three delete paths (`runUninstallCmd`, `runSilentRemove`, `runHeadlessRemove`), only when actually deleting, before `cleanDusterDir`.
  - The confirm screen and headless JSON show `quarantine_held` (bytes) when > 0. The TUI text reads: `Also deletes <size> kept by Duster for undo (du restore).`
- **`schedule.go`:** in `scheduleRunCmd.Run`, call `for _, err := range sweepQuarantine(time.Now()) { fmt.Println("quarantine sweep:", err) }` before `runScheduledClean`. Do not change `runScheduledClean`, which its tests call directly.

- [ ] **Step 1: Failing test** (`cmd/remove_quarantine_test.go`)

```go
package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEmptyAllQuarantines(t *testing.T) {
	work := tempQuarantine(t)
	f := filepath.Join(work, "a.txt")
	os.WriteFile(f, []byte("a"), 0o644)
	if err := quarantinePath(newQuarantineSession("purge"), f, 1); err != nil {
		t.Fatal(err)
	}
	if quarantineHeld() != 1 {
		t.Fatalf("held %d", quarantineHeld())
	}
	if err := emptyAllQuarantines(); err != nil {
		t.Fatal(err)
	}
	if quarantineHeld() != 0 {
		t.Fatal("quarantine not emptied")
	}
}
```

- [ ] **Step 2: Implement.** **Step 3:** gates. **Step 4:** `git commit -m "feat(restore): du remove empties quarantines, scheduled runs apply retention" -- cmd/remove.go cmd/schedule.go cmd/remove_quarantine_test.go`

---

### Task 8: Windows smoke and docs

**Files:** `.github/workflows/windows-smoke.yml` (a new step before "Remove (self-uninstall)"), `README.md`, `CHANGELOG.md`, `docs/security.md`, `docs/release-checklist.md`, and the spec, into which the plan's Deviations section is folded.

The smoke step runs under Windows PowerShell 5.1: put `$ErrorActionPreference = 'Continue'` first, check every native call's `$LASTEXITCODE`, and keep `net user` passwords at 14 characters or fewer. It runs:
1. Build a fake project `C:\...\RUNNER_TEMP\undo\p1` with `package.json` and a `node_modules\x\a.js` (purge requires a project marker; see `scanArtifacts`). Run `du purge <dir> --yes --json` and assert `kept >= 1` and that the folder is gone. `du restore --json` must list 1 session with that path. `du restore 1 --json` brings it back. Assert it exists.
2. The same on `D:\undo-smoke\p2`, which uses the `D:\.duster-quarantine\<SID>` root. Assert that root exists, is hidden, and that `icacls` shows only the user and SYSTEM. Purge again, then create a newer `node_modules` at the same path. `du restore 1` must report `skipped`, leave the newer one, and exit 0.
3. As a new standard user (the Start-Process -Credential pattern from the "Scheduled clean" step): the user purges a project on D:. A second standard user must get access denied listing the first user's SID folder.
4. `du restore --empty --yes --json` empties. `du restore --json` lists no sessions.

Always clean up the users and folders in `finally`.

Docs:
- README: a command-table row for `du restore`, plus a short paragraph (what is kept, 7 days, low space, `--permanent`, never overwrites).
- CHANGELOG: an Added bullet.
- security.md: a new section on quarantine privacy (the per-user protected folder, owner check), the no-overwrite restore, and that nothing is ever copied across drives.
- release checklist: manual items for a USB drive unplugged then replugged, and a low-space sweep on a nearly full drive.

No em-dashes.

- [ ] Commit: `git commit -m "docs(restore): smoke round trip, README, changelog, security notes" -- <those paths>`

---

## After Task 8 (controller)

1. Run the Windows gates on the combined tree. Then run a whole-branch review on the most capable model, one fix wave, and a scoped re-review.
2. Push `feat/undo-restore` (stacked on `feat/scheduled-clean`) and `smoke/undo-restore`. Dispatch CI on the branch.
3. Update `CLAUDE.md` in the main checkout:
   - a `restore` row, and 15 commands;
   - the invariant "user-facing deletes go through `quarantinePath`/`recycleOrQuarantine`";
   - the known gaps.
