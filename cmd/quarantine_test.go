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
