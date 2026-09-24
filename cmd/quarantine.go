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

	// nextSlot is the live session's monotonic slot counter (in-memory only:
	// unexported, so encoding/json never sees it). It only ever grows, so a
	// slot number is never reused, even after quarantinePath gives up on a
	// move that later turns out to have landed anyway.
	nextSlot int
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
	m.nextSlot++
	slot := m.nextSlot
	isDir := info.IsDir() && info.Mode()&(os.ModeSymlink|os.ModeIrregular) == 0
	m.Items = append(m.Items, quarantineItem{Slot: slot, Path: path, Size: size, Dir: isDir, State: "pending", At: time.Now()})

	// cleanup drops the item just appended (it never became "kept"; its slot
	// number is retired, never reused, by design). When that was the only
	// item this session had kept on this volume, the folder is removed too
	// and forgotten, so a session that never actually kept anything never
	// lingers as an empty, unsweepable folder.
	cleanup := func() {
		m.Items = m.Items[:len(m.Items)-1]
		if len(m.Items) == 0 {
			_ = removeAllSafe(dir)
			delete(s.dirs, dir)
			return
		}
		_ = writeManifest(dir, m)
	}

	if err := writeManifest(dir, m); err != nil {
		cleanup()
		return err
	}
	slotDir := filepath.Join(dir, strconv.Itoa(slot))
	if err := os.Mkdir(slotDir, 0o700); err != nil {
		cleanup()
		return err
	}
	dest := filepath.Join(slotDir, filepath.Base(path))
	if err := moveNoReplace(path, dest); err != nil {
		if _, statErr := os.Lstat(dest); statErr == nil {
			// The move actually landed despite the reported error (seen on
			// network shares and behind AV filters that lag the metadata
			// update). Leave the item pending rather than undo it: load
			// treats a pending item whose slot is filled as kept.
			s.kept++
			return nil
		}
		os.Remove(slotDir)
		cleanup()
		return fmt.Errorf("could not move %s into Duster's quarantine, so it was left in place: %w", path, err)
	}
	m.Items[len(m.Items)-1].State = "kept"
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
		// Gone although the bin reported an error (a Yes to Windows' prompt
		// returns success, so it is not that): the operation completed behind a
		// lagging report, or something else removed it. Nothing is left to keep.
		return false, nil
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
	if werr == nil {
		werr = tmp.Sync()
	}
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
	Damaged  bool      // session.json missing, unreadable or without a creation time: aged by folder time, emptiable only
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
			// A manifest without a creation time cannot be aged, so it counts
			// as damaged too and is aged by its folder.
			if err != nil || json.Unmarshal(b, &k.Manifest) != nil || k.Manifest.ID == "" || k.Manifest.Created.IsZero() {
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

// Items lists the session's kept items in a stable order (numbered from 1 by
// `du restore`). Items already restored are left out, so restoring twice never
// reports them again.
func (r restoreSession) Items() []keptItemRef {
	var out []keptItemRef
	for p, k := range r.Parts {
		for i, it := range k.Manifest.Items {
			if it.State == "kept" {
				out = append(out, keptItemRef{Part: p, Index: i, Item: it})
			}
		}
	}
	return out
}

func (r restoreSession) Size() int64 {
	var n int64
	for _, k := range r.Parts {
		n += k.size()
	}
	return n
}

// size is what this session folder keeps (a damaged one reports 0: its
// contents are unknown).
func (k keptSession) size() int64 {
	var n int64
	for _, it := range k.Manifest.Items {
		if it.State == "kept" {
			n += it.Size
		}
	}
	return n
}

// groupSessions merges one run's folders from every volume, newest first. The
// command and time come from an intact part when there is one.
func groupSessions(ks []keptSession) []restoreSession {
	byID := map[string]*restoreSession{}
	fromDamaged := map[string]bool{}
	var order []string
	for _, k := range ks {
		id := k.Manifest.ID
		r := byID[id]
		if r == nil {
			r = &restoreSession{ID: id}
			byID[id] = r
			order = append(order, id)
		}
		if len(r.Parts) == 0 || (fromDamaged[id] && !k.Damaged) {
			r.Command, r.Created = k.Manifest.Command, k.Created
			fromDamaged[id] = k.Damaged
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
// remaining ones until the volume would be back at or above it. Expired
// sessions count as freed space first, so the low-space pass never deletes a
// fresh session that the expired ones already made room for. Pure: vols is
// not modified.
func pickSweep(ks []keptSession, vols map[string]volSpace, now time.Time) []string {
	space := make(map[string]volSpace, len(vols))
	for r, v := range vols {
		space[r] = v
	}
	sorted := append([]keptSession(nil), ks...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Created.Before(sorted[j].Created) })
	var out []string
	gone := map[string]bool{}
	take := func(k keptSession) {
		out = append(out, k.Dir)
		gone[k.Dir] = true
		if v, ok := space[k.Root]; ok {
			v.Free += k.size()
			space[k.Root] = v
		}
	}
	for _, k := range sorted {
		if now.Sub(k.Created) > quarantineKeep {
			take(k)
		}
	}
	for _, k := range sorted {
		if gone[k.Dir] {
			continue
		}
		v, ok := space[k.Root]
		if !ok || v.Total <= 0 || float64(v.Free)*100/float64(v.Total) >= quarantineLowSpace {
			continue
		}
		take(k)
	}
	return out
}

// sweepQuarantine applies retention to every quarantine on this machine.
func sweepQuarantine(now time.Time) []error {
	roots := quarantineRoots()
	ks := loadKeptSessions(roots)
	vols := map[string]volSpace{}
	for _, r := range roots {
		// A volume that cannot be read is left to the age rule; a full one
		// (free == 0) is exactly the one that needs the low-space pass.
		if free, total, ok := diskSpace(r); ok {
			vols[r] = volSpace{Free: free, Total: total}
		}
	}
	sizes := map[string]int64{}
	for _, k := range ks {
		sizes[k.Dir] = k.size()
	}
	var errs []error
	for _, dir := range pickSweep(ks, vols, now) {
		err := removeSessionDir(dir)
		logging.LogDestructiveOperation("restore", "expire", dir, sizes[dir], err == nil)
		if err != nil {
			errs = append(errs, err)
		}
	}
	return errs
}

// removeSessionDir deletes one session folder, after the same path check
// every other delete passes.
func removeSessionDir(dir string) error {
	if !fs.IsValidPath(dir) {
		return fmt.Errorf("%s: deleting system protected paths is blocked for safety", dir)
	}
	return removeAllSafe(dir)
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
	for p := range rs.Parts {
		if !touched[p] {
			continue
		}
		k := rs.Parts[p]
		if allRestored(k.Manifest.Items) {
			// Nothing kept is left in it. Every item is already back, so a
			// leftover folder is only reported.
			if err := removeSessionDir(k.Dir); err != nil {
				out = append(out, restoreResult{Path: k.Dir, Status: "failed",
					Reason: "the items are back, but the emptied quarantine folder could not be removed: " + err.Error()})
			}
		} else if err := writeManifest(k.Dir, &rs.Parts[p].Manifest); err != nil {
			out = append(out, restoreResult{Path: k.Dir, Status: "failed",
				Reason: "the items are back, but the session record could not be updated, so they may be listed again: " + err.Error()})
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
			err := removeSessionDir(k.Dir)
			logging.LogDestructiveOperation("restore", "empty", k.Dir, k.size(), err == nil)
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
