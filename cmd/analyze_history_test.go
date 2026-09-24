package cmd

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

const mb = 1 << 20

// mkfile creates a file with the given logical size. Truncate makes it
// sparse where the file system allows, so a test can have gigabyte files
// without writing them; the scan counts logical size either way.
func mkfile(t *testing.T, path string, size int64) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(size); err != nil {
		t.Fatal(err)
	}
	f.Close()
}

func scanTree(t *testing.T, root string) *FolderNode {
	t.Helper()
	node, _, err := scanDirectory(root, nil)
	if err != nil {
		t.Fatalf("scan %s: %v", root, err)
	}
	return node
}

// snapshotOf scans root now and returns it as a baseline, as if it had been
// saved and read back.
func snapshotOf(t *testing.T, root string) *sizeSnapshot {
	t.Helper()
	return buildSizeSnapshot(scanTree(t, root), time.Now().Add(-24*time.Hour), false, 0)
}

func findChange(r changeReport, suffix string) (changeEntry, bool) {
	for _, e := range r.Entries {
		if strings.HasSuffix(e.Path, suffix) {
			return e, true
		}
	}
	return changeEntry{}, false
}

func TestExplainNamesTheNewFileNotItsAncestors(t *testing.T) {
	root := t.TempDir()
	mkfile(t, filepath.Join(root, "Users", "me", "Documents", "thesis.docx"), 40*mb)
	mkfile(t, filepath.Join(root, "Users", "me", "Downloads", "setup.exe"), 20*mb)
	base := snapshotOf(t, root)

	iso := filepath.Join(root, "Users", "me", "Downloads", "ubuntu.iso")
	mkfile(t, iso, 30*mb)

	r := explainChanges(base, scanTree(t, root))
	if r.Delta != 30*mb {
		t.Fatalf("total delta = %d, want %d", r.Delta, 30*mb)
	}
	if len(r.Entries) != 1 {
		t.Fatalf("entries = %+v, want exactly the new file", r.Entries)
	}
	got := r.Entries[0]
	if got.Path != iso || got.Status != changeNew || got.Kind != "file" || got.Delta != 30*mb {
		t.Errorf("entry = %+v, want the new ubuntu.iso", got)
	}
}

// Growth spread over many small files has no single culprit, so the folder
// holding them is the answer, marked as spread.
func TestExplainReportsSpreadGrowthOnTheFolder(t *testing.T) {
	root := t.TempDir()
	cache := filepath.Join(root, "AppData", "Cache")
	mkfile(t, filepath.Join(cache, "seed.bin"), 2*mb)
	mkfile(t, filepath.Join(root, "other.bin"), 50*mb)
	base := snapshotOf(t, root)

	for i := range 40 {
		mkfile(t, filepath.Join(cache, fmt.Sprintf("chunk-%02d.tmp", i)), 512<<10)
	}

	r := explainChanges(base, scanTree(t, root))
	got, ok := findChange(r, filepath.Join("AppData", "Cache"))
	if !ok {
		t.Fatalf("entries = %+v, want the Cache folder", r.Entries)
	}
	if !got.Spread || got.Status != changeGrew {
		t.Errorf("entry = %+v, want grew and spread", got)
	}
}

func TestExplainReportsNewFolderWholeAndGoneFolder(t *testing.T) {
	root := t.TempDir()
	old := filepath.Join(root, "Videos", "old-project")
	mkfile(t, filepath.Join(old, "render.mp4"), 200*mb)
	mkfile(t, filepath.Join(root, "Games", "keep.txt"), 1*mb)
	base := snapshotOf(t, root)

	if err := os.RemoveAll(old); err != nil {
		t.Fatal(err)
	}
	game := filepath.Join(root, "Games", "NewGame")
	mkfile(t, filepath.Join(game, "data", "pak0.bin"), 150*mb)
	mkfile(t, filepath.Join(game, "bin", "game.exe"), 90*mb)

	r := explainChanges(base, scanTree(t, root))

	gone, ok := findChange(r, filepath.Join("Videos", "old-project"))
	if !ok || gone.Status != changeGone || gone.Delta != -200*mb {
		t.Errorf("gone entry = %+v (found %v), want old-project gone, -200 MB", gone, ok)
	}
	added, ok := findChange(r, filepath.Join("Games", "NewGame"))
	if !ok || added.Status != changeNew || added.Delta != 240*mb {
		t.Errorf("new entry = %+v (found %v), want NewGame new, +240 MB", added, ok)
	}
	// A new folder is one fact, not a list of its parts.
	for _, e := range r.Entries {
		if strings.Contains(e.Path, filepath.Join("NewGame", "")) && e.Path != game {
			t.Errorf("new folder was split into its parts: %+v", e)
		}
	}
	// Growth is listed before shrinkage.
	if len(r.Entries) < 2 || r.Entries[0].Delta < 0 {
		t.Errorf("entries = %+v, want growth first", r.Entries)
	}
}

// Moving a folder is a real loss in one place and a gain in another, and a
// root whose total barely moved must still show both.
func TestExplainShowsOffsettingChanges(t *testing.T) {
	root := t.TempDir()
	mkfile(t, filepath.Join(root, "Desktop", "vm.vhdx"), 50*mb)
	mkfile(t, filepath.Join(root, "Archive", "readme.txt"), 1*mb)
	base := snapshotOf(t, root)

	if err := os.Rename(filepath.Join(root, "Desktop", "vm.vhdx"), filepath.Join(root, "Archive", "vm.vhdx")); err != nil {
		t.Fatal(err)
	}

	r := explainChanges(base, scanTree(t, root))
	if r.Delta != 0 {
		t.Fatalf("total delta = %d, want 0 for a move", r.Delta)
	}
	if _, ok := findChange(r, filepath.Join("Archive", "vm.vhdx")); !ok {
		t.Errorf("entries = %+v, want the file at its new place", r.Entries)
	}
	if _, ok := findChange(r, filepath.Join("Desktop", "vm.vhdx")); !ok {
		t.Errorf("entries = %+v, want the file gone from its old place", r.Entries)
	}
}

// The explanation must add up: its lines account for the folder's whole
// change, so nothing is double counted and nothing significant is missed.
func TestExplainEntriesAddUpToTheTotal(t *testing.T) {
	root := t.TempDir()
	mkfile(t, filepath.Join(root, "a", "x.bin"), 100*mb)
	mkfile(t, filepath.Join(root, "b", "c", "y.bin"), 80*mb)
	mkfile(t, filepath.Join(root, "d", "z.bin"), 60*mb)
	base := snapshotOf(t, root)

	mkfile(t, filepath.Join(root, "a", "x.bin"), 160*mb)       // grew
	mkfile(t, filepath.Join(root, "b", "c", "new.bin"), 30*mb) // new, nested
	os.Remove(filepath.Join(root, "d", "z.bin"))               // gone
	mkfile(t, filepath.Join(root, "e", "w.bin"), 45*mb)        // new folder

	r := explainChanges(base, scanTree(t, root))
	var sum int64
	for _, e := range r.Entries {
		sum += e.Delta
	}
	if sum != r.Delta {
		t.Errorf("entries sum to %d, total delta is %d: %+v", sum, r.Delta, r.Entries)
	}
}

func TestExplainIgnoresChangesBelowThreshold(t *testing.T) {
	root := t.TempDir()
	mkfile(t, filepath.Join(root, "docs", "a.pdf"), 30*mb)
	base := snapshotOf(t, root)

	mkfile(t, filepath.Join(root, "docs", "b.pdf"), 200<<10) // 200 KB, under 1 MB

	r := explainChanges(base, scanTree(t, root))
	if len(r.Entries) != 0 {
		t.Errorf("entries = %+v, want none for a change below the threshold", r.Entries)
	}
	// Scripts loop over entries: no changes is [], never null.
	if data, _ := json.Marshal(r); !strings.Contains(string(data), `"entries":[]`) {
		t.Errorf("JSON = %s, want an empty entries array", data)
	}
}

// Windows paths are case-insensitive: renaming Downloads to downloads is not
// a folder vanishing and another appearing.
func TestExplainMatchesPathsCaseInsensitively(t *testing.T) {
	root := t.TempDir()
	mkfile(t, filepath.Join(root, "Downloads", "big.bin"), 100*mb)
	base := snapshotOf(t, root)

	tree := scanTree(t, root)
	for _, sub := range tree.SubFolders {
		sub.Name = strings.ToLower(sub.Name)
		sub.Path = filepath.Join(root, sub.Name)
	}
	if r := explainChanges(base, tree); len(r.Entries) != 0 {
		t.Errorf("entries = %+v, want none for a case-only rename", r.Entries)
	}
}

func TestSnapshotIsBoundedAndClosedUnderAncestors(t *testing.T) {
	root := t.TempDir()
	for i := range 30 {
		mkfile(t, filepath.Join(root, "d"+string(rune('a'+i%26))+strings.Repeat("_", i/26), "f.bin"), int64(i+1)*mb)
	}
	mkfile(t, filepath.Join(root, "tiny.txt"), 1024) // under the 64 KB entry minimum

	s := buildSizeSnapshot(scanTree(t, root), time.Now(), false, 0)
	if _, ok := s.Files["tiny.txt"]; ok {
		t.Error("files under the minimum entry size must not be recorded")
	}
	if s.Floor < historyMinEntrySize {
		t.Errorf("floor = %d, want at least %d", s.Floor, historyMinEntrySize)
	}
	for key := range s.index {
		if key == "" {
			continue
		}
		if _, ok := s.index[parentKey(key)]; !ok {
			t.Errorf("%q is recorded but its parent is not", key)
		}
	}
}

func TestSaveLoadRoundTripAndReplacement(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(t.TempDir(), "Duster", "history")
	mkfile(t, filepath.Join(root, "a.bin"), 5*mb)
	tree := scanTree(t, root)
	start := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)

	if err := saveSizeSnapshot(dir, buildSizeSnapshot(tree, start, false, 7)); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, notes := loadBaseline(dir, root, 7, 0, start.Add(time.Minute))
	if got == nil || len(notes) != 0 {
		t.Fatalf("load = %v, %v; want the saved snapshot", got, notes)
	}
	if got.Total != tree.Size || got.Folders[""] != tree.Size {
		t.Errorf("round trip lost the sizes: %+v", got)
	}

	// Scans within an hour replace each other instead of piling up.
	if err := saveSizeSnapshot(dir, buildSizeSnapshot(tree, start.Add(20*time.Minute), false, 7)); err != nil {
		t.Fatal(err)
	}
	if n := len(listSnapshots(filepath.Join(dir, rootDirName(root)))); n != 1 {
		t.Errorf("%d snapshots after two scans 20 minutes apart, want 1", n)
	}
	if err := saveSizeSnapshot(dir, buildSizeSnapshot(tree, start.Add(3*time.Hour), false, 7)); err != nil {
		t.Fatal(err)
	}
	if n := len(listSnapshots(filepath.Join(dir, rootDirName(root)))); n != 2 {
		t.Errorf("%d snapshots after a scan 3 hours later, want 2", n)
	}
}

func TestLoadBaselineSinceAndFallback(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(t.TempDir(), "Duster", "history")
	mkfile(t, filepath.Join(root, "a.bin"), 5*mb)
	tree := scanTree(t, root)
	now := time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)

	for _, age := range []time.Duration{10 * 24 * time.Hour, 3 * 24 * time.Hour, 2 * time.Hour} {
		if err := saveSizeSnapshot(dir, buildSizeSnapshot(tree, now.Add(-age), false, 0)); err != nil {
			t.Fatal(err)
		}
	}

	latest, _ := loadBaseline(dir, root, 0, 0, now)
	if latest == nil || !latest.Taken.Equal(now.Add(-2*time.Hour)) {
		t.Errorf("default baseline = %v, want the newest", latest)
	}
	week, notes := loadBaseline(dir, root, 0, 7*24*time.Hour, now)
	if week == nil || !week.Taken.Equal(now.Add(-10*24*time.Hour)) || len(notes) != 0 {
		t.Errorf("--since 7d = %v %v, want the 10-day-old scan", week, notes)
	}
	month, notes := loadBaseline(dir, root, 0, 30*24*time.Hour, now)
	if month == nil || !month.Taken.Equal(now.Add(-10*24*time.Hour)) {
		t.Errorf("--since 30d = %v, want the oldest as a fallback", month)
	}
	if len(notes) != 1 || !strings.Contains(notes[0], "oldest") {
		t.Errorf("notes = %v, want the fallback said out loud", notes)
	}
}

// A different USB stick under the same drive letter is not the same disk.
func TestLoadBaselineRefusesADifferentVolume(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(t.TempDir(), "Duster", "history")
	mkfile(t, filepath.Join(root, "a.bin"), 5*mb)
	if err := saveSizeSnapshot(dir, buildSizeSnapshot(scanTree(t, root), time.Now().Add(-time.Hour*5), false, 1111)); err != nil {
		t.Fatal(err)
	}
	got, notes := loadBaseline(dir, root, 2222, 0, time.Now())
	if got != nil {
		t.Error("a snapshot from another volume must not be compared")
	}
	if len(notes) == 0 || !strings.Contains(notes[0], "different disk") {
		t.Errorf("notes = %v, want the reason", notes)
	}
}

// A damaged file is skipped for the next best one, never fatal.
func TestLoadBaselineSkipsDamagedSnapshots(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(t.TempDir(), "Duster", "history")
	mkfile(t, filepath.Join(root, "a.bin"), 5*mb)
	older := time.Now().Add(-48 * time.Hour)
	if err := saveSizeSnapshot(dir, buildSizeSnapshot(scanTree(t, root), older, false, 0)); err != nil {
		t.Fatal(err)
	}
	rootDir := filepath.Join(dir, rootDirName(root))
	newer := filepath.Join(rootDir, "9999999999999999999.json.gz")
	if err := os.WriteFile(newer, []byte("not gzip"), 0o600); err != nil {
		t.Fatal(err)
	}

	got, _ := loadBaseline(dir, root, 0, 0, time.Now())
	if got == nil || !got.Taken.Equal(older.UTC()) {
		t.Errorf("baseline = %v, want the intact older snapshot", got)
	}
}

// Another folder's snapshot filed under this folder's name (a hash
// collision, or a copied file) must never be used.
func TestLoadBaselineChecksTheRoot(t *testing.T) {
	root, other := t.TempDir(), t.TempDir()
	dir := filepath.Join(t.TempDir(), "Duster", "history")
	mkfile(t, filepath.Join(other, "a.bin"), 5*mb)
	s := buildSizeSnapshot(scanTree(t, other), time.Now().Add(-5*time.Hour), false, 0)
	if err := saveSizeSnapshot(dir, s); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(dir, rootDirName(other)), filepath.Join(dir, rootDirName(root))); err != nil {
		t.Fatal(err)
	}
	if got, _ := loadBaseline(dir, root, 0, 0, time.Now()); got != nil {
		t.Error("a snapshot of another folder must not be compared")
	}
}

func TestThinSnapshotsKeepsEndsAndThinsOldHistory(t *testing.T) {
	now := time.Date(2026, 9, 24, 0, 0, 0, 0, time.UTC)
	var files []snapFile
	for day := 30; day >= 0; day-- {
		files = append(files, snapFile{path: time.Duration(day).String(), taken: now.Add(-time.Duration(day) * 24 * time.Hour)})
	}

	drop := thinSnapshots(files, 10, now)
	if len(files)-len(drop) != 10 {
		t.Fatalf("kept %d, want 10", len(files)-len(drop))
	}
	dropped := map[string]bool{}
	for _, d := range drop {
		dropped[d.path] = true
	}
	if dropped[files[0].path] || dropped[files[len(files)-1].path] {
		t.Error("the oldest and newest snapshots must always be kept")
	}
	// Recent history stays denser than old history.
	recent, old := 0, 0
	for _, f := range files {
		if dropped[f.path] {
			continue
		}
		if now.Sub(f.taken) <= 7*24*time.Hour {
			recent++
		} else {
			old++
		}
	}
	if recent < old/2 {
		t.Errorf("kept %d from the last week and %d older, want recent history favored", recent, old)
	}
}

func TestPruneRootDirsForgetsTheLeastRecent(t *testing.T) {
	dir := t.TempDir()
	current := filepath.Join(dir, "current")
	for i, name := range []string{"oldest", "middle", "newest"} {
		sub := filepath.Join(dir, name)
		mkfile(t, filepath.Join(sub, "1.json.gz"), 10)
		stamp := time.Now().Add(time.Duration(i-10) * time.Hour)
		os.Chtimes(sub, stamp, stamp)
	}
	mkfile(t, filepath.Join(current, "1.json.gz"), 10)

	pruneRootDirs(dir, current, 3)

	if _, err := os.Stat(filepath.Join(dir, "oldest")); !os.IsNotExist(err) {
		t.Error("the least recently analyzed folder should be forgotten")
	}
	for _, keep := range []string{"middle", "newest", "current"} {
		if _, err := os.Stat(filepath.Join(dir, keep)); err != nil {
			t.Errorf("%s was removed: %v", keep, err)
		}
	}
}

// Duster may run elevated while writing into the user's profile: a link in
// the history path must never redirect its writes.
func TestSaveRefusesALinkedHistoryFolder(t *testing.T) {
	base := t.TempDir()
	target := t.TempDir()
	if err := os.Mkdir(filepath.Join(base, "Duster"), 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(base, "Duster", "history")
	if err := os.Symlink(target, link); err != nil {
		t.Skip("symlinks unsupported here")
	}
	root := t.TempDir()
	mkfile(t, filepath.Join(root, "a.bin"), 5*mb)

	if err := saveSizeSnapshot(link, buildSizeSnapshot(scanTree(t, root), time.Now(), false, 0)); err == nil {
		t.Error("saving through a linked folder must fail")
	}
	if entries, _ := os.ReadDir(target); len(entries) != 0 {
		t.Errorf("the link target was written to: %v", entries)
	}
}

func TestRecordScanHistoryEndToEnd(t *testing.T) {
	t.Setenv("LOCALAPPDATA", t.TempDir())
	root := t.TempDir()
	mkfile(t, filepath.Join(root, "Documents", "a.pdf"), 20*mb)
	mkfile(t, filepath.Join(root, "Downloads", "notes.txt"), 1*mb)
	now := time.Now()

	first, notes := recordScanHistory(scanTree(t, root), 0, now.Add(-2*time.Hour))
	if first != nil || len(notes) != 0 {
		t.Fatalf("first scan = %v %v, want no baseline and no notes", first, notes)
	}

	mkfile(t, filepath.Join(root, "Downloads", "big.iso"), 40*mb)
	tree := scanTree(t, root)
	base, notes := recordScanHistory(tree, 0, now)
	if base == nil || len(notes) != 0 {
		t.Fatalf("second scan = %v %v, want the first scan as baseline", base, notes)
	}
	r := explainChanges(base, tree)
	if e, ok := findChange(r, "big.iso"); !ok || e.Status != changeNew {
		t.Errorf("entries = %+v, want big.iso reported as new", r.Entries)
	}
}

// Without a profile directory there is nowhere to keep history: analyze
// still works, and nothing is written to the working directory.
func TestRecordScanHistoryWithoutProfileDir(t *testing.T) {
	t.Setenv("LOCALAPPDATA", "")
	t.Setenv("USERPROFILE", "")
	root := t.TempDir()
	mkfile(t, filepath.Join(root, "a.bin"), 5*mb)
	if base, notes := recordScanHistory(scanTree(t, root), 0, time.Now()); base != nil || notes != nil {
		t.Errorf("got %v %v, want nothing", base, notes)
	}
}

func TestParseSinceDuration(t *testing.T) {
	tests := []struct {
		in   string
		want time.Duration
		ok   bool
	}{
		{"7d", 7 * 24 * time.Hour, true},
		{"2w", 14 * 24 * time.Hour, true},
		{"36h", 36 * time.Hour, true},
		{"90m", 90 * time.Minute, true},
		{"1.5d", 36 * time.Hour, true},
		{" 7D ", 7 * 24 * time.Hour, true},
		{"0", 0, false},
		{"-3d", 0, false},
		{"week", 0, false},
		{"", 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := parseSinceDuration(tt.in)
			if (err == nil) != tt.ok || got != tt.want {
				t.Errorf("parseSinceDuration(%q) = %v, %v; want %v, ok=%v", tt.in, got, err, tt.want, tt.ok)
			}
		})
	}
}

func TestFormatAge(t *testing.T) {
	tests := map[time.Duration]string{
		30 * time.Second:    "under a minute",
		time.Minute:         "1 minute",
		5 * time.Hour:       "5 hours",
		47 * time.Hour:      "47 hours",
		17 * 24 * time.Hour: "17 days",
	}
	for d, want := range tests {
		if got := formatAge(d); got != want {
			t.Errorf("formatAge(%v) = %q, want %q", d, got, want)
		}
	}
}

// A snapshot that decompresses to more than the cap is refused rather than
// read into memory.
func TestReadSizeSnapshotCapsItsSize(t *testing.T) {
	path := filepath.Join(t.TempDir(), "1.json.gz")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(f)
	gz.Write([]byte(`{"version":1,"root":"x","folders":{"`))
	chunk := []byte(strings.Repeat("a", 1<<20))
	for range historyMaxFileBytes/len(chunk) + 1 {
		gz.Write(chunk)
	}
	gz.Close()
	f.Close()

	if _, err := readSizeSnapshot(path); err == nil {
		t.Error("an oversized snapshot must be refused")
	}
}

// modelWithHistory drives the analyze screen the way a user's session does:
// an earlier scan is saved, the folder changes, then the screen scans it and
// loads the comparison.
func modelWithHistory(t *testing.T) (analyzeModel, string) {
	t.Helper()
	t.Setenv("LOCALAPPDATA", t.TempDir())
	root := t.TempDir()
	mkfile(t, filepath.Join(root, "Documents", "report.pdf"), 30*mb)
	mkfile(t, filepath.Join(root, "Downloads", "notes.txt"), 2*mb)
	if _, notes := recordScanHistory(scanTree(t, root), 0, time.Now().Add(-72*time.Hour)); len(notes) != 0 {
		t.Fatalf("saving the earlier scan: %v", notes)
	}
	mkfile(t, filepath.Join(root, "Downloads", "game-setup.exe"), 25*mb)

	m := initialAnalyzeModel(root)
	tree, large, err := scanDirectory(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	next, cmd := m.Update(analyzeScanCompleteMsg{Root: tree, LargeFiles: large})
	m = next.(analyzeModel)
	if cmd == nil {
		t.Fatal("the first scan of the folder should load and save history")
	}
	next, _ = m.Update(cmd())
	return next.(analyzeModel), root
}

func press(m analyzeModel, key string) (analyzeModel, tea.Cmd) {
	var msg tea.KeyMsg
	switch key {
	case "enter":
		msg = tea.KeyMsg{Type: tea.KeyEnter}
	case "down":
		msg = tea.KeyMsg{Type: tea.KeyDown}
	default:
		msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
	}
	next, cmd := m.Update(msg)
	return next.(analyzeModel), cmd
}

func TestAnalyzeScreenExplainsWhatGrew(t *testing.T) {
	m, _ := modelWithHistory(t)
	if !m.historyReady || m.baseline == nil {
		t.Fatalf("history not loaded: ready=%v baseline=%v notes=%v", m.historyReady, m.baseline, m.historyNotes)
	}
	if m.changes.Delta != 25*mb {
		t.Errorf("change = %d, want +25 MB", m.changes.Delta)
	}

	m, _ = press(m, "c")
	if !m.showChanges {
		t.Fatal("c should open the changes panel")
	}
	view := sgr.ReplaceAllString(m.View(), "")
	for _, want := range []string{"Change:", "+25 MB", "What changed since", "game-setup.exe", "new"} {
		if !strings.Contains(view, want) {
			t.Errorf("view is missing %q", want)
		}
	}
}

// Enter on a change opens the folder that holds it with the cursor on the
// item, so d, o and Enter then act on exactly what grew.
func TestAnalyzeJumpToChangeSelectsTheItem(t *testing.T) {
	m, root := modelWithHistory(t)
	m, _ = press(m, "c")
	m, cmd := press(m, "enter")
	if cmd != nil {
		t.Fatal("an in-memory jump should not rescan")
	}
	if m.showChanges {
		t.Error("the panel should close after a jump")
	}
	if want := filepath.Join(root, "Downloads"); m.tree.Path != want {
		t.Fatalf("tree = %s, want %s", m.tree.Path, want)
	}
	if got := m.tree.Entries[m.selectedIdx].Path; filepath.Base(got) != "game-setup.exe" {
		t.Errorf("cursor on %s, want game-setup.exe", got)
	}
	if len(m.historyStack) != 1 || m.historyStack[0].Path != root {
		t.Errorf("back stack = %v, want just the root", m.historyStack)
	}

	// b walks back up to where the jump started.
	m, _ = press(m, "b")
	if m.tree.Path != root {
		t.Errorf("back went to %s, want %s", m.tree.Path, root)
	}
}

// The changes list can name items that are gone, so d never deletes from it.
func TestAnalyzeChangesPanelNeverDeletes(t *testing.T) {
	m, _ := modelWithHistory(t)
	m, _ = press(m, "c")
	m, _ = press(m, "d")
	if m.confirmRecycle {
		t.Error("d in the changes panel must not offer to recycle")
	}
}

func TestAnalyzePanelsAreExclusive(t *testing.T) {
	m, _ := modelWithHistory(t)
	m, _ = press(m, "c")
	m, _ = press(m, "L")
	if m.showChanges || !m.showLargeFiles {
		t.Error("L should switch from the changes panel to largest files")
	}
	m, _ = press(m, "c")
	if !m.showChanges || m.showLargeFiles {
		t.Error("c should switch from largest files to the changes panel")
	}
	m, _ = press(m, "b")
	if m.showChanges {
		t.Error("b should close the changes panel")
	}
}

// A rescan after a recycle is not a new scan of the folder: it must not save
// another snapshot or reload the comparison.
func TestAnalyzeSavesOneSnapshotPerSession(t *testing.T) {
	m, root := modelWithHistory(t)
	tree, large, err := scanDirectory(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, cmd := m.Update(analyzeScanCompleteMsg{Root: tree, LargeFiles: large}); cmd != nil {
		t.Error("a rescan in the same session must not touch history again")
	}
}

func TestAnalyzeNoHistoryTouchesNothing(t *testing.T) {
	analyzeNoHistory = true
	t.Cleanup(func() { analyzeNoHistory = false })
	t.Setenv("LOCALAPPDATA", t.TempDir())
	root := t.TempDir()
	mkfile(t, filepath.Join(root, "a.bin"), 5*mb)

	tree, large, _ := scanDirectory(root, nil)
	next, cmd := initialAnalyzeModel(root).Update(analyzeScanCompleteMsg{Root: tree, LargeFiles: large})
	if cmd != nil {
		t.Error("--no-history must not load or save anything")
	}
	if !next.(analyzeModel).historyReady {
		t.Error("the screen should not wait for history that will never load")
	}
	if _, err := os.Stat(historyDir()); !os.IsNotExist(err) {
		t.Error("--no-history must not create the history folder")
	}
}

func TestChangesPanelStartsAtTheLeftMargin(t *testing.T) {
	m, _ := modelWithHistory(t)
	m, _ = press(m, "c")
	first := initialAnalyzeModel(t.TempDir())
	first.tree = m.tree
	first.historyReady = true
	first.showChanges = true

	for name, v := range map[string]string{"with history": m.View(), "first scan": first.View()} {
		for i, line := range strings.Split(sgr.ReplaceAllString(v, ""), "\n") {
			text := strings.TrimLeft(line, " ")
			if indent := len(line) - len(text); text != "" && indent > 20 {
				t.Errorf("%s: line %d starts at column %d: %q", name, i, indent, text)
			}
		}
	}
}

// The --json output carries the same explanation, with changes null on the
// first scan so scripts can tell "nothing to compare" from "nothing changed".
func TestHeadlessAnalyzeReportsChanges(t *testing.T) {
	t.Setenv("LOCALAPPDATA", t.TempDir())
	root := t.TempDir()
	mkfile(t, filepath.Join(root, "Videos", "clip.mp4"), 40*mb)

	run := func() AnalyzeJSONOutput {
		t.Helper()
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		stdout := os.Stdout
		os.Stdout = w
		runHeadlessAnalyze(root)
		os.Stdout = stdout
		w.Close()
		var out AnalyzeJSONOutput
		if err := json.NewDecoder(r).Decode(&out); err != nil {
			t.Fatalf("decode: %v", err)
		}
		return out
	}

	first := run()
	if first.Changes != nil {
		t.Errorf("first scan changes = %+v, want null", first.Changes)
	}
	mkfile(t, filepath.Join(root, "Videos", "movie.mkv"), 70*mb)

	second := run()
	if second.Changes == nil {
		t.Fatal("second scan reported no changes")
	}
	if second.Changes.Delta != 70*mb {
		t.Errorf("delta = %d, want +70 MB", second.Changes.Delta)
	}
	if e, ok := findChange(*second.Changes, "movie.mkv"); !ok || e.Status != changeNew {
		t.Errorf("entries = %+v, want movie.mkv as new", second.Changes.Entries)
	}
}

// A snapshot is chosen by the time in its file name, and the report names
// the time inside it. A renamed file would make those disagree, so it is
// skipped rather than reported under a date it does not describe.
func TestLoadBaselineSkipsRenamedSnapshots(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(t.TempDir(), "Duster", "history")
	mkfile(t, filepath.Join(root, "a.bin"), 5*mb)
	if err := saveSizeSnapshot(dir, buildSizeSnapshot(scanTree(t, root), time.Now(), false, 0)); err != nil {
		t.Fatal(err)
	}
	snaps := listSnapshots(filepath.Join(dir, rootDirName(root)))
	renamed := filepath.Join(filepath.Dir(snaps[0].path), fmt.Sprintf("%d.json.gz", time.Now().Add(-240*time.Hour).UnixNano()))
	if err := os.Rename(snaps[0].path, renamed); err != nil {
		t.Fatal(err)
	}
	if got, _ := loadBaseline(dir, root, 0, 0, time.Now()); got != nil {
		t.Errorf("a renamed snapshot was used, dated %v", got.Taken)
	}
}

// "New" is only claimed when the baseline recorded everything down to the
// entry minimum; with a coarser floor all that is known is "smaller then".
func TestUntrackedLabelsAreHonestAboutTheFloor(t *testing.T) {
	if got := newTag(historyMinEntrySize); got != "new" {
		t.Errorf("fine floor tag = %q, want new", got)
	}
	if got := newTag(30 * mb); got != "new or <30 MB" {
		t.Errorf("coarse floor tag = %q, want new or <30 MB", got)
	}
	// The sentence form never claims an item did not exist: it may have
	// been there, empty.
	for _, floor := range []int64{historyMinEntrySize, 30 * mb} {
		if got := untrackedLabel(floor); !strings.HasPrefix(got, "was under ") {
			t.Errorf("label for floor %d = %q, want was under ...", floor, got)
		}
	}
}

// The snapshot is built on a background goroutine while the screen keeps
// navigating the same tree, which fills in Entries lazily. The builder must
// only read fields the UI never writes. Run with -race.
func TestHistoryBuildRacesSafelyWithNavigation(t *testing.T) {
	t.Setenv("LOCALAPPDATA", t.TempDir())
	root := t.TempDir()
	for i := range 20 {
		mkfile(t, filepath.Join(root, fmt.Sprintf("d%02d", i), "sub", "f.bin"), int64(i+1)*mb)
	}
	tree := scanTree(t, root)

	done := make(chan struct{})
	go func() {
		defer close(done)
		analyzeHistoryCmd(tree)()
	}()
	var m analyzeModel
	for _, sub := range tree.SubFolders {
		m.showNode(sub)
		for _, subsub := range sub.SubFolders {
			m.showNode(subsub)
		}
	}
	<-done
}
