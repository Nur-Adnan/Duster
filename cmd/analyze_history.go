package cmd

import (
	"compress/gzip"
	"container/heap"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Nur-Adnan/duster/internal/logging"
	"github.com/Nur-Adnan/duster/lib/elevation"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// "What grew since last time": every analyze saves a compact snapshot of the
// scanned folder's biggest items, and the next analyze of the same folder
// explains what changed. The question a disk analyzer is usually opened for is
// not "what is big" but "where did my space go", and only a comparison answers
// it. Paid tools (TreeSize Professional, SpaceObServer, FolderSizes) offer this;
// the free ones either do not, or need a manual export and a separate diff.

const (
	historyVersion = 1

	// Snapshot size is bounded no matter how big the tree is: the largest
	// entries are kept, and everything left out is known to be no bigger than
	// the snapshot's Floor. That bound is what makes a comparison trustworthy.
	historyMaxEntries   = 10000
	historyMinEntrySize = 64 << 10

	historyPerRoot       = 10
	historyMaxRoots      = 24
	historyReplaceWithin = time.Hour

	// A snapshot is Duster's own file, but it is still read back from disk:
	// cap what one can expand to.
	historyMaxFileBytes = 64 << 20
)

// sizeSnapshot is one saved scan of one folder. Keys are paths relative to
// Root ("" is Root itself), in the case the scan saw them.
type sizeSnapshot struct {
	Version  int              `json:"version"`
	Root     string           `json:"root"`
	Taken    time.Time        `json:"taken"`
	Elevated bool             `json:"elevated"`
	Volume   uint32           `json:"volume,omitempty"`
	Total    int64            `json:"total"`
	Floor    int64            `json:"floor"`
	Folders  map[string]int64 `json:"folders"`
	Files    map[string]int64 `json:"files"`

	// Built on load: Windows paths are case-insensitive, so lookups go
	// through lowercased keys.
	index    map[string]snapItem
	children map[string][]string
}

type snapItem struct {
	rel  string
	size int64
	file bool
}

// ─────────────────────────────────────────────
// Building a snapshot
// ─────────────────────────────────────────────

// snapHeap is a min-heap by importance: smaller first, and on equal size the
// deeper path first. That tie-break keeps the kept set closed under ancestors,
// since a parent is never smaller than its child and always has a shorter path.
type snapHeap []snapItem

func (h snapHeap) Len() int { return len(h) }
func (h snapHeap) Less(i, j int) bool {
	if h[i].size != h[j].size {
		return h[i].size < h[j].size
	}
	return len(h[i].rel) > len(h[j].rel)
}
func (h snapHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }
func (h *snapHeap) Push(x any)   { *h = append(*h, x.(snapItem)) }
func (h *snapHeap) Pop() any {
	old := *h
	item := old[len(old)-1]
	*h = old[:len(old)-1]
	return item
}

// buildSizeSnapshot keeps the largest historyMaxEntries folders and files of
// the tree. It only reads Path, Size, SubFolders and Files, never Entries,
// which the UI fills in lazily on another goroutine.
func buildSizeSnapshot(root *FolderNode, taken time.Time, elevated bool, volume uint32) *sizeSnapshot {
	h := &snapHeap{}
	floor := int64(historyMinEntrySize)

	consider := func(item snapItem) {
		if item.size < historyMinEntrySize {
			return
		}
		heap.Push(h, item)
		if h.Len() > historyMaxEntries {
			if dropped := heap.Pop(h).(snapItem); dropped.size > floor {
				floor = dropped.size
			}
		}
	}

	var walk func(n *FolderNode, rel string)
	walk = func(n *FolderNode, rel string) {
		for _, f := range n.Files {
			consider(snapItem{rel: filepath.Join(rel, filepath.Base(f.Path)), size: f.Size, file: true})
		}
		for _, sub := range n.SubFolders {
			subRel := filepath.Join(rel, sub.Name)
			consider(snapItem{rel: subRel, size: sub.Size})
			walk(sub, subRel)
		}
	}
	walk(root, "")

	s := &sizeSnapshot{
		Version:  historyVersion,
		Root:     root.Path,
		Taken:    taken.UTC(),
		Elevated: elevated,
		Volume:   volume,
		Total:    root.Size,
		Floor:    floor,
		Folders:  map[string]int64{"": root.Size},
		Files:    map[string]int64{},
	}
	for _, item := range *h {
		if item.file {
			s.Files[item.rel] = item.size
		} else {
			s.Folders[item.rel] = item.size
		}
	}
	s.buildIndex()
	return s
}

func (s *sizeSnapshot) buildIndex() {
	s.index = make(map[string]snapItem, len(s.Folders)+len(s.Files))
	s.children = make(map[string][]string)
	add := func(rel string, size int64, file bool) {
		key := strings.ToLower(rel)
		s.index[key] = snapItem{rel: rel, size: size, file: file}
		if key != "" {
			parent := parentKey(key)
			s.children[parent] = append(s.children[parent], key)
		}
	}
	for rel, size := range s.Folders {
		add(rel, size, false)
	}
	for rel, size := range s.Files {
		add(rel, size, true)
	}
}

// lookup returns an item's size in this snapshot. ok is false when it was not
// recorded, meaning it was no bigger than Floor, or did not exist at all.
func (s *sizeSnapshot) lookup(key string) (snapItem, bool) {
	item, ok := s.index[key]
	return item, ok
}

func parentKey(key string) string {
	parent := filepath.Dir(key)
	if parent == "." {
		return ""
	}
	return parent
}

// relKey is path relative to root as a lookup key, or ok=false when path is
// not inside root.
func relKey(root, path string) (string, bool) {
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	if rel == "." {
		rel = ""
	}
	return strings.ToLower(rel), true
}

// ─────────────────────────────────────────────
// Storage
// ─────────────────────────────────────────────

// historyDir is where snapshots live. It sits inside Duster's data directory,
// so `du remove` takes it away with everything else.
func historyDir() string {
	dir := logging.Dir()
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, "history")
}

// normalizeRoot is the identity of a scanned folder: Windows paths are
// case-insensitive, and a trailing separator names the same folder.
func normalizeRoot(root string) string {
	return strings.ToLower(filepath.Clean(root))
}

func rootDirName(root string) string {
	sum := sha256.Sum256([]byte(normalizeRoot(root)))
	return hex.EncodeToString(sum[:8])
}

type snapFile struct {
	path  string
	taken time.Time
}

// listSnapshots returns the saved snapshots of one folder, oldest first. The
// time comes from the file name, so choosing one never opens the others.
func listSnapshots(rootDir string) []snapFile {
	entries, err := os.ReadDir(rootDir)
	if err != nil {
		return nil
	}
	var files []snapFile
	for _, e := range entries {
		if !e.Type().IsRegular() {
			continue
		}
		stem, ok := strings.CutSuffix(e.Name(), ".json.gz")
		if !ok {
			continue
		}
		nanos, err := strconv.ParseInt(stem, 10, 64)
		if err != nil {
			continue
		}
		files = append(files, snapFile{path: filepath.Join(rootDir, e.Name()), taken: time.Unix(0, nanos)})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].taken.Before(files[j].taken) })
	return files
}

// realDir reports whether path is a directory and not a link to one. Duster
// may run elevated while writing into the user's profile, so a junction
// planted in the history path must never redirect its writes or deletes.
func realDir(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.IsDir() && info.Mode()&(os.ModeSymlink|os.ModeIrregular) == 0
}

// ensureRealDir creates path if needed and refuses it if it is a link.
func ensureRealDir(path string) error {
	if err := os.Mkdir(path, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	if !realDir(path) {
		return fmt.Errorf("%s is not a plain folder", path)
	}
	return nil
}

func saveSizeSnapshot(dir string, s *sizeSnapshot) error {
	for _, d := range []string{filepath.Dir(dir), dir} {
		if err := ensureRealDir(d); err != nil {
			return err
		}
	}
	rootDir := filepath.Join(dir, rootDirName(s.Root))
	if err := ensureRealDir(rootDir); err != nil {
		return err
	}

	existing := listSnapshots(rootDir)

	tmp, err := os.CreateTemp(rootDir, ".tmp-*")
	if err != nil {
		return err
	}
	gz := gzip.NewWriter(tmp)
	encErr := json.NewEncoder(gz).Encode(s)
	gzErr := gz.Close()
	closeErr := tmp.Close()
	if err := errors.Join(encErr, gzErr, closeErr); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	final := filepath.Join(rootDir, strconv.FormatInt(s.Taken.UnixNano(), 10)+".json.gz")
	if err := os.Rename(tmp.Name(), final); err != nil {
		os.Remove(tmp.Name())
		return err
	}

	// Scans minutes apart would crowd the long-range history out, so a new
	// one replaces a previous one less than an hour old.
	if n := len(existing); n > 0 {
		newest := existing[n-1]
		if age := s.Taken.Sub(newest.taken); age >= 0 && age < historyReplaceWithin {
			os.Remove(newest.path)
			existing = existing[:n-1]
		}
	}
	all := append(existing, snapFile{path: final, taken: s.Taken})
	for _, f := range thinSnapshots(all, historyPerRoot, s.Taken) {
		os.Remove(f.path)
	}
	pruneRootDirs(dir, rootDir, historyMaxRoots)
	return nil
}

// thinSnapshots picks which snapshots to drop to get down to keep. The newest
// and the oldest always stay: the oldest is the longest-range comparison there
// is. Among the rest, the one whose removal leaves the smallest gap relative
// to its age goes first, which keeps recent history dense and old history
// sparse, roughly like backup rotation.
func thinSnapshots(files []snapFile, keep int, now time.Time) []snapFile {
	files = append([]snapFile(nil), files...)
	var drop []snapFile
	for len(files) > keep && len(files) > 2 {
		best, bestScore := -1, 0.0
		for i := 1; i < len(files)-1; i++ {
			gap := files[i+1].taken.Sub(files[i-1].taken).Seconds()
			age := now.Sub(files[i].taken).Seconds() + 1
			if score := gap / age; best < 0 || score < bestScore {
				best, bestScore = i, score
			}
		}
		drop = append(drop, files[best])
		files = append(files[:best], files[best+1:]...)
	}
	return drop
}

// pruneRootDirs keeps history for at most maxRoots folders, forgetting the
// ones analyzed least recently. It deletes only snapshot files by name, then
// the folder if that left it empty: never a recursive delete.
func pruneRootDirs(dir, current string, maxRoots int) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	type rootInfo struct {
		path string
		mod  time.Time
	}
	var roots []rootInfo
	for _, e := range entries {
		path := filepath.Join(dir, e.Name())
		if !e.IsDir() || path == current || !realDir(path) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		roots = append(roots, rootInfo{path: path, mod: info.ModTime()})
	}
	excess := len(roots) + 1 - maxRoots
	if excess <= 0 {
		return
	}
	sort.Slice(roots, func(i, j int) bool { return roots[i].mod.Before(roots[j].mod) })
	for _, r := range roots[:excess] {
		for _, f := range listSnapshots(r.path) {
			os.Remove(f.path)
		}
		os.Remove(r.path) // fails, harmlessly, if anything else is in there
	}
}

func readSizeSnapshot(path string) (*sizeSnapshot, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return nil, err
	}
	defer gz.Close()

	var s sizeSnapshot
	if err := json.NewDecoder(io.LimitReader(gz, historyMaxFileBytes)).Decode(&s); err != nil {
		return nil, err
	}
	if s.Version != historyVersion || s.Folders == nil {
		return nil, fmt.Errorf("unsupported snapshot format %d", s.Version)
	}
	if s.Files == nil {
		s.Files = map[string]int64{}
	}
	s.buildIndex()
	return &s, nil
}

// loadBaseline picks the snapshot to compare with: the newest one, or with
// since set, the newest one at least that old. When none is old enough it
// falls back to the oldest there is and says so, rather than silently
// comparing across a different span than the one asked for.
func loadBaseline(dir, root string, volume uint32, since time.Duration, now time.Time) (*sizeSnapshot, []string) {
	files := listSnapshots(filepath.Join(dir, rootDirName(root)))
	if len(files) == 0 {
		return nil, nil
	}

	var notes []string
	order := make([]snapFile, 0, len(files))
	if since <= 0 {
		for i := len(files) - 1; i >= 0; i-- {
			order = append(order, files[i])
		}
	} else {
		cutoff := now.Add(-since)
		for i := len(files) - 1; i >= 0; i-- {
			if !files[i].taken.After(cutoff) {
				order = append(order, files[i])
			}
		}
		if len(order) == 0 {
			order = append(order, files...)
			notes = append(notes, fmt.Sprintf("No scan of this folder is %s old yet; comparing with the oldest one, from %s.",
				formatAge(since), formatSnapDate(files[0].taken)))
		}
	}

	// A damaged or foreign file is skipped for the next best, never fatal.
	// So is one whose name disagrees with its content: the name is what a
	// snapshot was chosen by, and the report must name the scan it compared.
	for _, f := range order {
		s, err := readSizeSnapshot(f.path)
		if err != nil || normalizeRoot(s.Root) != normalizeRoot(root) || !s.Taken.Equal(f.taken) {
			continue
		}
		if s.Volume != 0 && volume != 0 && s.Volume != volume {
			return nil, append(notes, "This path is on a different disk than the last time it was scanned, so there is nothing to compare with.")
		}
		return s, notes
	}
	return nil, notes
}

// recordScanHistory loads the comparison for this scan and then saves the
// scan itself for next time, in that order so a scan never compares with
// itself. Nothing here can fail an analysis: problems come back as notes.
func recordScanHistory(root *FolderNode, since time.Duration, now time.Time) (*sizeSnapshot, []string) {
	dir := historyDir()
	if dir == "" || root == nil {
		return nil, nil
	}
	if info, err := os.Stat(root.Path); err != nil || !info.IsDir() {
		return nil, nil
	}

	elevated := elevation.IsAdmin()
	volume := volumeSerial(root.Path)

	baseline, notes := loadBaseline(dir, root.Path, volume, since, now)
	if baseline != nil && baseline.Elevated != elevated {
		// Folders only an administrator can read are skipped silently by the
		// scan, so a change of rights looks exactly like a change of size.
		if baseline.Elevated {
			notes = append(notes, "The previous scan ran as administrator and this one did not: folders you cannot read now count as shrinkage.")
		} else {
			notes = append(notes, "This scan runs as administrator and the previous one did not: folders it could not read count as growth.")
		}
	}

	if err := saveSizeSnapshot(dir, buildSizeSnapshot(root, now, elevated, volume)); err != nil {
		notes = append(notes, "This scan could not be saved for next time: "+err.Error())
	}
	return baseline, notes
}

// ─────────────────────────────────────────────
// Explaining the change
// ─────────────────────────────────────────────

// changeEntry is one line of the explanation. Previous is only meaningful
// when PreviouslyTracked is set; otherwise the item did not exist or was no
// bigger than the report's Floor, and Status is "new" in either case.
type changeEntry struct {
	Path              string `json:"path"`
	Kind              string `json:"kind"`
	Status            string `json:"status"`
	Spread            bool   `json:"spread,omitempty"`
	Previous          int64  `json:"previous_size"`
	PreviouslyTracked bool   `json:"previously_tracked"`
	Size              int64  `json:"size"`
	Delta             int64  `json:"delta"`
}

const (
	changeGrew   = "grew"
	changeShrank = "shrank"
	changeNew    = "new"
	changeGone   = "gone"
)

// changeReport explains how one folder changed since the baseline.
type changeReport struct {
	Path              string        `json:"path"`
	Since             time.Time     `json:"since"`
	PreviousTotal     int64         `json:"previous_total"`
	PreviouslyTracked bool          `json:"previously_tracked"`
	Total             int64         `json:"total"`
	Delta             int64         `json:"delta"`
	Threshold         int64         `json:"threshold"`
	Floor             int64         `json:"floor"`
	Entries           []changeEntry `json:"entries"`
}

// changeThreshold is the smallest change worth a line of its own. It scales
// with the folder (a tenth of a percent), stays between 1 MB and 100 MB, and
// is never below twice the baseline's floor: an item missing from the
// baseline could have been up to Floor bytes, so a smaller change could be
// nothing but that uncertainty.
func changeThreshold(size, previous, floor int64) int64 {
	// Spelled out: package cmd has its own int-only min (clean_tui.go),
	// which shadows the builtin.
	t := size
	if previous > t {
		t = previous
	}
	t /= 1000
	if t < 1<<20 {
		t = 1 << 20
	}
	if t > 100<<20 {
		t = 100 << 20
	}
	if t < 2*floor {
		t = 2 * floor
	}
	return t
}

// explainChanges reports what changed under node since the baseline.
//
// A plain diff would list every ancestor of a change (C:\Users, then
// C:\Users\me, then its Downloads). Instead the explanation descends only
// into children whose own change is significant and stops where no single
// child explains it, so each line is as specific as the data allows and the
// lines add up to the folder's total change. Whatever no significant child
// explains is reported once, as spread across smaller items. An item that is
// new, or gone, is reported whole rather than split into its parts.
func explainChanges(base *sizeSnapshot, node *FolderNode) changeReport {
	report := changeReport{Path: node.Path, Since: base.Taken, Total: node.Size, Floor: base.Floor, Entries: []changeEntry{}}
	key, ok := relKey(base.Root, node.Path)
	if !ok {
		return report
	}
	prev, tracked := base.lookup(key)
	report.PreviousTotal, report.PreviouslyTracked = prev.size, tracked
	report.Delta = node.Size - prev.size
	report.Threshold = changeThreshold(node.Size, prev.size, base.Floor)
	if !tracked {
		// Everything below a new folder is new too: there is nothing to
		// explain beyond "this appeared".
		return report
	}

	// out starts empty rather than nil: "entries": [] and not null, so a
	// script can loop over it without checking.
	e := explainer{base: base, threshold: report.Threshold, out: []changeEntry{}}
	e.visit(node, key, report.Delta)
	sortChanges(e.out)
	report.Entries = e.out
	return report
}

type explainer struct {
	base      *sizeSnapshot
	threshold int64
	out       []changeEntry
}

type changeChild struct {
	key     string
	path    string
	file    bool
	node    *FolderNode
	now     int64
	prev    int64
	tracked bool
	gone    bool
}

func (e *explainer) visit(n *FolderNode, key string, delta int64) {
	var significant []changeChild
	var explained int64
	for _, c := range e.children(n, key) {
		if d := c.now - c.prev; abs64(d) >= e.threshold {
			significant = append(significant, c)
			explained += d
		}
	}

	for _, c := range significant {
		d := c.now - c.prev
		switch {
		case c.gone:
			e.emit(c, changeGone, false)
		case !c.tracked:
			e.emit(c, changeNew, false)
		case c.file:
			e.emit(c, growthStatus(d), false)
		default:
			e.visit(c.node, c.key, d)
		}
	}

	if rest := delta - explained; abs64(rest) >= e.threshold {
		prev, _ := e.base.lookup(key)
		e.out = append(e.out, changeEntry{
			Path: n.Path, Kind: "folder", Status: growthStatus(rest), Spread: true,
			Previous: prev.size, PreviouslyTracked: true, Size: n.Size, Delta: rest,
		})
	}
}

// children lists n's subfolders and files, plus the ones the baseline
// recorded that no longer exist.
func (e *explainer) children(n *FolderNode, key string) []changeChild {
	kids := make([]changeChild, 0, len(n.SubFolders)+len(n.Files))
	seen := make(map[string]bool, cap(kids))
	add := func(c changeChild) {
		item, ok := e.base.lookup(c.key)
		c.prev, c.tracked = item.size, ok
		seen[c.key] = true
		kids = append(kids, c)
	}
	for _, sub := range n.SubFolders {
		add(changeChild{key: childKey(key, sub.Name), path: sub.Path, node: sub, now: sub.Size})
	}
	for _, f := range n.Files {
		add(changeChild{key: childKey(key, filepath.Base(f.Path)), path: f.Path, file: true, now: f.Size})
	}
	for _, k := range e.base.children[key] {
		if seen[k] {
			continue
		}
		item, _ := e.base.lookup(k)
		kids = append(kids, changeChild{
			key: k, path: filepath.Join(n.Path, filepath.Base(item.rel)), file: item.file,
			prev: item.size, tracked: true, gone: true,
		})
	}
	return kids
}

func (e *explainer) emit(c changeChild, status string, spread bool) {
	kind := "folder"
	if c.file {
		kind = "file"
	}
	e.out = append(e.out, changeEntry{
		Path: c.path, Kind: kind, Status: status, Spread: spread,
		Previous: c.prev, PreviouslyTracked: c.tracked, Size: c.now, Delta: c.now - c.prev,
	})
}

func childKey(parent, name string) string {
	return strings.ToLower(filepath.Join(parent, name))
}

func growthStatus(delta int64) string {
	if delta < 0 {
		return changeShrank
	}
	return changeGrew
}

// sortChanges puts growth first, biggest first, then shrinkage, biggest
// first: growth is what the user came to find.
func sortChanges(entries []changeEntry) {
	sort.SliceStable(entries, func(i, j int) bool {
		a, b := entries[i].Delta, entries[j].Delta
		if (a >= 0) != (b >= 0) {
			return a >= 0
		}
		return abs64(a) > abs64(b)
	})
}

func abs64(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}

// ─────────────────────────────────────────────
// Wording
// ─────────────────────────────────────────────

// parseSinceDuration accepts Go durations plus days and weeks ("7d", "2w"),
// which are what people actually mean by "since".
func parseSinceDuration(s string) (time.Duration, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	var d time.Duration
	var err error
	switch {
	case strings.HasSuffix(s, "d") || strings.HasSuffix(s, "w"):
		unit := 24 * time.Hour
		if strings.HasSuffix(s, "w") {
			unit *= 7
		}
		var n float64
		n, err = strconv.ParseFloat(s[:len(s)-1], 64)
		d = time.Duration(n * float64(unit))
	default:
		d, err = time.ParseDuration(s)
	}
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("invalid --since %q: use a positive duration such as 12h, 7d or 2w", s)
	}
	return d, nil
}

func formatSnapDate(t time.Time) string {
	return t.Local().Format("Jan 2, 2006 15:04")
}

// formatAge renders a duration the way a person would say it.
func formatAge(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "under a minute"
	case d < time.Hour:
		return plural(int(d/time.Minute), "minute")
	case d < 48*time.Hour:
		return plural(int(d/time.Hour), "hour")
	default:
		return plural(int(d/(24*time.Hour)), "day")
	}
}

func plural(n int, unit string) string {
	if n == 1 {
		return "1 " + unit
	}
	return strconv.Itoa(n) + " " + unit + "s"
}

// formatDelta renders a signed size change.
func formatDelta(d int64) string {
	if d < 0 {
		return "-" + formatSize(-d)
	}
	return "+" + formatSize(d)
}

// ─────────────────────────────────────────────
// Analyze screen
// ─────────────────────────────────────────────

// changesVisible is how many change rows fit where the Largest Files panel
// normally sits, so opening the panel never makes the screen taller.
const changesVisible = 5

// listLen is the length of whichever list the cursor is in.
func (m analyzeModel) listLen() int {
	switch {
	case m.showChanges:
		return len(m.changes.Entries)
	case m.showLargeFiles:
		// The Largest Files panel renders at most 5 rows; navigating past
		// them would act on items the user cannot see.
		if len(m.largeFiles) > 5 {
			return 5
		}
		return len(m.largeFiles)
	case m.tree != nil:
		return len(m.tree.Entries)
	}
	return 0
}

// refreshChanges explains the folder on screen. It runs on every folder
// switch; the explanation only descends where something changed, so it
// stays cheap even on a whole drive.
func (m *analyzeModel) refreshChanges() {
	if m.baseline == nil || m.tree == nil {
		m.changes = changeReport{}
		return
	}
	m.changes = explainChanges(m.baseline, m.tree)
}

// jumpToChange shows the selected change in the explorer: it opens the
// folder that holds the item and puts the cursor on it, so d, o and Enter
// act on exactly what grew.
func (m analyzeModel) jumpToChange() (tea.Model, tea.Cmd) {
	if m.tree == nil || m.selectedIdx >= len(m.changes.Entries) {
		return m, nil
	}
	entry := m.changes.Entries[m.selectedIdx]
	m.showChanges = false
	m.selectedIdx = 0

	// A change spread across the folder on screen is this folder: there is
	// nowhere else to go.
	if normalizeRoot(entry.Path) == normalizeRoot(m.tree.Path) {
		return m, nil
	}
	parent := filepath.Dir(entry.Path)

	// After a recycle the in-memory tree no longer matches the disk, so
	// rescan the parent the way a drill-down does.
	if m.root != nil && !m.stale {
		if chain := folderChain(m.root, parent); len(chain) > 0 {
			m.historyStack = append([]*FolderNode(nil), chain[:len(chain)-1]...)
			m.showNode(chain[len(chain)-1])
			m.selectedIdx = entryIndex(m.tree.Entries, entry.Path)
			return m, nil
		}
	}
	m.historyStack = append(m.historyStack, m.tree)
	m.scanning = true
	m.scanChan = make(chan scanProgressInfo, 100)
	return m, tea.Batch(
		analyzeRunScanCmd(nearestExisting(parent, m.targetPath), m.scanChan),
		analyzeListenToScanProgress(m.scanChan),
	)
}

// folderChain walks from root towards target and returns every folder on the
// way, ending at the deepest one that exists in the tree: a gone item's
// parent may be gone as well.
func folderChain(root *FolderNode, target string) []*FolderNode {
	key, ok := relKey(root.Path, target)
	if !ok {
		return nil
	}
	chain := []*FolderNode{root}
	if key == "" {
		return chain
	}
	n := root
	for _, part := range strings.Split(key, string(filepath.Separator)) {
		var next *FolderNode
		for _, sub := range n.SubFolders {
			if strings.EqualFold(sub.Name, part) {
				next = sub
				break
			}
		}
		if next == nil {
			break
		}
		chain = append(chain, next)
		n = next
	}
	return chain
}

func entryIndex(entries []EntryInfo, path string) int {
	for i, e := range entries {
		if strings.EqualFold(e.Path, path) {
			return i
		}
	}
	return 0
}

// nearestExisting climbs from path to the closest folder that still exists,
// never above limit: Explorer cannot open a folder that is gone.
func nearestExisting(path, limit string) string {
	for p := path; ; p = filepath.Dir(p) {
		if _, err := os.Stat(p); err == nil {
			return p
		}
		if _, ok := relKey(limit, p); !ok || filepath.Dir(p) == p {
			return limit
		}
	}
}

func (m analyzeModel) changeSince() string {
	return formatSnapDate(m.baseline.Taken) + " (" + formatAge(time.Since(m.baseline.Taken)) + " ago)"
}

// renderChangeSummary is the "Change:" line under the totals, plus any
// caveat about the comparison. Line breaks stay outside Render: lipgloss pads
// every line of a multi-line string to the widest one.
func (m analyzeModel) renderChangeSummary() string {
	if analyzeNoHistory || !m.historyReady || m.tree == nil {
		return ""
	}
	label := lipgloss.NewStyle().Foreground(colorSilver).Render("Change:        ")
	var sb strings.Builder
	switch {
	case m.baseline == nil && len(m.historyNotes) > 0:
		sb.WriteString(label + styleSub.Render(m.historyNotes[0]) + "\n")
		return sb.String()
	case m.baseline == nil:
		sb.WriteString(label + styleSub.Render("first scan of this folder; the next one will show what changed") + "\n")
		return sb.String()
	case !m.changes.PreviouslyTracked:
		sb.WriteString(label + deltaStyle(1).Render(untrackedLabel(m.baseline.Floor)) + styleSub.Render(" on "+m.changeSince()) + "\n")
	default:
		sb.WriteString(label + deltaStyle(m.changes.Delta).Render(formatDelta(m.changes.Delta)) +
			styleSub.Render(" since "+m.changeSince()) + "\n")
	}
	for _, note := range m.historyNotes {
		sb.WriteString(styleSub.Render("               "+clampHead(note, 90)) + "\n")
	}
	return sb.String()
}

// deltaStyle colors growth like a warning and shrinkage like a success, the
// convention TreeSize uses: red grew, green shrank.
func deltaStyle(d int64) lipgloss.Style {
	if d < 0 {
		return lipgloss.NewStyle().Foreground(colorMint).Bold(true)
	}
	return lipgloss.NewStyle().Foreground(colorCoral).Bold(true)
}

func (m analyzeModel) renderChangesPanel() string {
	var sb strings.Builder
	title := lipgloss.NewStyle().Foreground(colorSkyBlue).Bold(true)

	switch {
	case analyzeNoHistory:
		sb.WriteString(title.Render("What changed:") + "\n")
		sb.WriteString("  " + styleSub.Render("History is off for this run (--no-history).") + "\n")
		return sb.String()
	case !m.historyReady:
		sb.WriteString(title.Render("What changed:") + "\n")
		sb.WriteString("  " + styleSub.Render("Loading the previous scan...") + "\n")
		return sb.String()
	case m.baseline == nil:
		sb.WriteString(title.Render("What changed:") + "\n")
		msg := "This is the first scan of this folder. Run du analyze again later to see what grew."
		if len(m.historyNotes) > 0 {
			msg = m.historyNotes[0]
		}
		sb.WriteString("  " + styleSub.Render(clampHead(msg, 90)) + "\n")
		return sb.String()
	}

	sb.WriteString(title.Render("What changed since "+m.changeSince()+":") + "\n")
	entries := m.changes.Entries
	if !m.changes.PreviouslyTracked {
		sb.WriteString("  " + styleSub.Render("This folder "+untrackedLabel(m.baseline.Floor)+" then, so there is nothing to break down.") + "\n")
	} else if len(entries) == 0 {
		sb.WriteString("  " + styleSub.Render("No single change of "+formatSize(m.changes.Threshold)+" or more in this folder.") + "\n")
	}

	start, end := visibleWindow(m.selectedIdx, len(entries), changesVisible)
	for i := start; i < end; i++ {
		sb.WriteString(renderChangeRow(entries[i], m.tree.Path, m.baseline.Floor, i == m.selectedIdx) + "\n")
	}
	if len(entries) > changesVisible {
		sb.WriteString("  " + styleSub.Render(fmt.Sprintf("%d of %d", m.selectedIdx+1, len(entries))) + "\n")
	}
	if len(entries) > 0 {
		sb.WriteString("  " + styleSub.Render("[Enter] go to it   [O] open in Explorer   [C] close") + "\n")
	}
	return sb.String()
}

// untrackedLabel says what is known about an item the baseline did not
// record: only that it was smaller than the floor. It may have existed, empty.
func untrackedLabel(floor int64) string {
	return "was under " + formatSize(floor)
}

// newTag labels an item the baseline did not record. Everything reported
// changed by at least twice the floor, so with the 64 KB minimum floor nearly
// all of it is new and "new" is fair; on a whole drive the floor can be tens
// of megabytes, and the tag says so.
func newTag(floor int64) string {
	if floor <= historyMinEntrySize {
		return "new"
	}
	return "new or <" + formatSize(floor)
}

func renderChangeRow(e changeEntry, base string, floor int64, selected bool) string {
	marker := "▲"
	if e.Delta < 0 {
		marker = "▼"
	}
	var tag string
	switch {
	case e.Status == changeNew:
		tag = newTag(floor)
	case e.Status == changeGone:
		tag = "gone"
	case e.Spread:
		tag = "across smaller items"
	}
	delta := fmt.Sprintf("%s %10s", marker, formatDelta(e.Delta))
	path := padRight(clampHead(formatRelativePath(base, e.Path), 58), 60)

	if selected {
		sel := lipgloss.NewStyle().Foreground(colorSkyBlue).Bold(true)
		return sel.Render("▸ " + delta + "  " + path + tag)
	}
	return "  " + deltaStyle(e.Delta).Render(delta) + "  " + styleLabel.Render(path) + styleSub.Render(tag)
}

// visibleWindow is the slice of a list of total rows to draw, keeping the
// cursor in view.
func visibleWindow(cursor, total, rows int) (int, int) {
	if total <= rows {
		return 0, total
	}
	start := cursor - rows/2
	if start < 0 {
		start = 0
	}
	if start+rows > total {
		start = total - rows
	}
	return start, start + rows
}
