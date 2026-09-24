package cmd

import (
	"os"
	"path/filepath"
	"runtime"
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
	mk := func(root, name string, age time.Duration, size int64) keptSession {
		return keptSession{Root: root, Dir: root + "/" + name, Created: now.Add(-age),
			Manifest: quarantineManifest{Items: []quarantineItem{{Size: size, State: "kept"}}}}
	}
	day := 24 * time.Hour
	tests := []struct {
		name string
		ks   []keptSession
		vols map[string]volSpace
		want string
	}{
		{"expired, then oldest on a low volume",
			[]keptSession{mk("C", "old", 8*day, 1), mk("C", "new", time.Hour, 1), mk("D", "a", 3*day, 30), mk("D", "b", 2*day, 30)},
			map[string]volSpace{"C": {Free: 50, Total: 100}, "D": {Free: 5, Total: 100}}, // D at 5% free
			"C/old,D/a"},
		{"expired bytes count before the low-space pass (same volume)",
			[]keptSession{mk("D", "expired", 8*day, 30), mk("D", "fresh", time.Hour, 30)},
			map[string]volSpace{"D": {Free: 5, Total: 100}},
			"D/expired"},
		{"a full volume is still swept",
			[]keptSession{mk("D", "a", 2*day, 5), mk("D", "b", day, 10), mk("D", "c", time.Hour, 10)},
			map[string]volSpace{"D": {Free: 0, Total: 100}},
			"D/a,D/b"},
		{"an unreadable volume is left to the age rule",
			[]keptSession{mk("E", "a", 2*day, 5)},
			map[string]volSpace{},
			""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := map[string]volSpace{}
			for k, v := range tt.vols {
				before[k] = v
			}
			if got := strings.Join(pickSweep(tt.ks, tt.vols, now, true), ","); got != tt.want {
				t.Errorf("pickSweep = %q, want %q", got, tt.want)
			}
			for k, v := range before {
				if tt.vols[k] != v {
					t.Errorf("pickSweep changed the caller's map: %s %+v -> %+v", k, v, tt.vols[k])
				}
			}
		})
	}
}

func TestQuarantineFirstFailureLeavesNoFolder(t *testing.T) {
	work := tempQuarantine(t)
	target := filepath.Join(work, "a.txt")
	os.WriteFile(target, []byte("a"), 0o644)
	root, err := quarantineRoot(work)
	if err != nil {
		t.Fatal(err)
	}
	s := newQuarantineSession("installer")
	dir := filepath.Join(root, s.id)
	os.MkdirAll(dir, 0o700)
	os.WriteFile(filepath.Join(dir, "1"), nil, 0o600) // slot 1 cannot be created
	if err := quarantinePath(s, target, 1); err == nil {
		t.Fatal("expected the first keep to fail")
	}
	if exists(dir) {
		t.Error("a session folder that kept nothing was left behind")
	}
	if len(s.dirs) != 0 || s.Kept() != 0 {
		t.Errorf("the failed folder is still tracked: dirs=%d kept=%d", len(s.dirs), s.Kept())
	}
	if b, _ := os.ReadFile(target); string(b) != "a" {
		t.Fatal("the item was touched by a failed keep")
	}
	if err := quarantinePath(s, target, 1); err != nil {
		t.Fatalf("a later keep on the same volume: %v", err)
	}
}

func TestRestoreTwiceReportsOnlyWhatIsLeft(t *testing.T) {
	work := tempQuarantine(t)
	a, b := filepath.Join(work, "a.txt"), filepath.Join(work, "b.txt")
	os.WriteFile(a, []byte("a"), 0o644)
	os.WriteFile(b, []byte("b"), 0o644)
	s := newQuarantineSession("installer")
	quarantinePath(s, a, 1)
	quarantinePath(s, b, 1)

	rs := groupSessions(loadKeptSessions(quarantineRoots()))[0]
	if res := restoreItems(rs, 1, false); len(res) != 1 || res[0].Path != a || res[0].Status != "restored" {
		t.Fatalf("first restore: %+v", res)
	}
	if n := len(rs.Items()); n != 1 {
		t.Fatalf("Items() after one restore = %d, want 1", n)
	}
	res := restoreItems(rs, 0, false)
	if len(res) != 1 || res[0].Path != b || res[0].Status != "restored" {
		t.Fatalf("second restore must list only the item still kept: %+v", res)
	}
}

func TestRestoreReportsBookkeepingFailures(t *testing.T) {
	if runtime.GOOS == "windows" || os.Getuid() == 0 {
		t.Skip("needs POSIX folder permissions as a regular user")
	}
	tests := []struct {
		name   string
		item   int // 1 restores one of two items (manifest rewrite), 0 restores all (folder removal)
		locked func(dir string) string
		reason string
	}{
		{"manifest rewrite", 1, func(dir string) string { return dir }, "session record"},
		{"folder removal", 0, filepath.Dir, "could not be removed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			work := tempQuarantine(t)
			a, b := filepath.Join(work, "a.txt"), filepath.Join(work, "b.txt")
			os.WriteFile(a, []byte("a"), 0o644)
			os.WriteFile(b, []byte("b"), 0o644)
			s := newQuarantineSession("installer")
			quarantinePath(s, a, 1)
			quarantinePath(s, b, 1)
			rs := groupSessions(loadKeptSessions(quarantineRoots()))[0]

			locked := tt.locked(rs.Parts[0].Dir)
			if err := os.Chmod(locked, 0o500); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { os.Chmod(locked, 0o700) })

			var failed []restoreResult
			for _, r := range restoreItems(rs, tt.item, false) {
				if r.Status == "failed" {
					failed = append(failed, r)
				}
			}
			if len(failed) != 1 || failed[0].Path != rs.Parts[0].Dir || !strings.Contains(failed[0].Reason, tt.reason) {
				t.Fatalf("the bookkeeping failure was not reported: %+v", failed)
			}
		})
	}
}

func TestEmptySessionsRefusesInvalidPath(t *testing.T) {
	tempQuarantine(t)
	rs := []restoreSession{{ID: "x", Parts: []keptSession{{Dir: "relative-dir"}}}}
	if err := emptySessions(rs); err == nil {
		t.Error("emptied a folder that fails the path check")
	}
}

func TestKeptSessionSizeIsPerPart(t *testing.T) {
	part := func(sizes ...int64) keptSession {
		var items []quarantineItem
		for _, n := range sizes {
			items = append(items, quarantineItem{Size: n, State: "kept"})
		}
		return keptSession{Manifest: quarantineManifest{Items: items}}
	}
	r := restoreSession{Parts: []keptSession{part(1, 2), part(10)}}
	r.Parts[0].Manifest.Items[1].State = "restored"
	if r.Parts[0].size() != 1 || r.Parts[1].size() != 10 || r.Size() != 11 {
		t.Errorf("sizes: %d %d %d", r.Parts[0].size(), r.Parts[1].size(), r.Size())
	}
}

func TestGroupSessionsPrefersIntactPart(t *testing.T) {
	now := time.Now()
	got := groupSessions([]keptSession{
		{Root: "C", Dir: "C/1-purge", Damaged: true, Created: now, Manifest: quarantineManifest{ID: "1-purge"}},
		{Root: "D", Dir: "D/1-purge", Created: now.Add(-time.Hour), Manifest: quarantineManifest{ID: "1-purge", Command: "purge",
			Created: now.Add(-time.Hour), Items: []quarantineItem{{Slot: 1, Size: 1, State: "kept"}}}},
	})
	if len(got) != 1 || got[0].Command != "purge" || !got[0].Created.Equal(now.Add(-time.Hour)) {
		t.Fatalf("command/time must come from the intact part: %+v", got)
	}
}

func TestLoadZeroCreatedIsDamaged(t *testing.T) {
	work := tempQuarantine(t)
	root, err := quarantineRoot(work)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "1-purge")
	os.MkdirAll(filepath.Join(dir, "1"), 0o700)
	os.WriteFile(filepath.Join(dir, "1", "x"), []byte("x"), 0o644)
	m := quarantineManifest{ID: "1-purge", Command: "purge", Items: []quarantineItem{{Slot: 1, Path: filepath.Join(work, "x"), State: "kept"}}}
	if err := writeManifest(dir, &m); err != nil {
		t.Fatal(err)
	}
	ks := loadKeptSessions([]string{root})
	if len(ks) != 1 || !ks[0].Damaged || ks[0].Created.IsZero() {
		t.Fatalf("a manifest without a creation time must be damaged and aged by its folder: %+v", ks)
	}
}
