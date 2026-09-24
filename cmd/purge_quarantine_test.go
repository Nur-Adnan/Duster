package cmd

import (
	"errors"
	"path/filepath"
	"runtime"
	"strings"
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
}

func TestPurgeOneNoQuarantineSuggestsPermanent(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the stub quarantineRoot has no quarantine without a profile folder; Windows resolves a real one")
	}
	work := t.TempDir()
	t.Setenv("LOCALAPPDATA", "")
	t.Setenv("USERPROFILE", "")
	t.Setenv("DU_NO_OPLOG", "1")
	nm := filepath.Join(work, "proj", "node_modules")
	writeTree(t, nm)
	q, err := purgeOne(newQuarantineSession("purge"), nm, 5, false, false)
	if q || !errors.Is(err, errNoQuarantine) || !strings.HasSuffix(err.Error(), "; use --permanent to delete it for good") {
		t.Fatalf("no quarantine: quarantined=%v err=%v", q, err)
	}
	if !exists(nm) {
		t.Fatal("an item that could not be kept must stay in place")
	}
}

func TestPurgeTallyKeptIsNotFreed(t *testing.T) {
	var tl purgeTally
	tl.add("a", 1<<20, true, false, nil)  // kept
	tl.add("b", 2<<20, false, true, nil)  // --permanent
	tl.add("c", 3<<20, false, false, nil) // --safe, into the Recycle Bin
	tl.add("d", 4<<20, false, false, errors.New("boom"))
	if tl.kept != 1<<20 || tl.keptCount != 1 || tl.freed != 2<<20 || tl.recycled != 3<<20 || tl.failed != 1 {
		t.Fatalf("tally: %+v", tl)
	}
	if len(tl.errs) != 1 || !strings.Contains(tl.errs[0], "d: boom") {
		t.Fatalf("errors: %v", tl.errs)
	}
	got := strings.Join(tl.lines(), "\n")
	for _, want := range []string{"Freed 2.00 MB", "Recycle Bin", "Kept 1.00 MB for 7 days", "--permanent frees it now", "du restore 1"} {
		if !strings.Contains(got, want) {
			t.Errorf("summary lacks %q:\n%s", want, got)
		}
	}

	var keptOnly purgeTally
	keptOnly.add("a", 5, true, false, nil)
	got = strings.ToLower(strings.Join(keptOnly.lines(), "\n"))
	if strings.Contains(got, "reclaimed") || strings.HasPrefix(got, "freed") {
		t.Errorf("kept bytes reported as freed:\n%s", got)
	}
}
