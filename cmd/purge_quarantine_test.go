package cmd

import (
	"os"
	"path/filepath"
	"runtime"
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
	if runtime.GOOS == "windows" {
		t.Skip("the Recycle Bin takes it on Windows")
	}
	work := tempQuarantine(t)
	nm := filepath.Join(work, "proj", "node_modules")
	writeTree(t, nm)
	q, err := purgeOne(newQuarantineSession("purge"), nm, 5, true, false)
	if err != nil || !q || exists(nm) {
		t.Fatalf("--safe fallback: quarantined=%v err=%v", q, err)
	}
	_ = os.Remove
}
