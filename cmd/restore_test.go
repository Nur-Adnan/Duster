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

func TestRestoreRequestError(t *testing.T) {
	two := restoreSession{Parts: []keptSession{{Manifest: quarantineManifest{Items: []quarantineItem{
		{Slot: 1, Path: "a", State: "kept"}, {Slot: 2, Path: "b", State: "kept"},
	}}}}}
	for _, tc := range []struct {
		item      int
		itemGiven bool
		ok        bool
	}{
		{0, false, true}, {1, true, true}, {2, true, true},
		{0, true, false}, {-1, true, false}, {3, true, false},
	} {
		err := restoreRequestError(two, 1, tc.item, tc.itemGiven)
		if (err == nil) != tc.ok {
			t.Errorf("--item %d (given=%v): err=%v", tc.item, tc.itemGiven, err)
		}
	}

	damaged := restoreSession{Parts: []keptSession{{Dir: "d", Damaged: true, Manifest: quarantineManifest{ID: "x"}}}}
	err := restoreRequestError(damaged, 3, 0, false)
	if err == nil || err.Error() != "session 3 is damaged: its items cannot be listed or restored; empty it with du restore --empty 3" {
		t.Fatalf("damaged session: %v", err)
	}
	// A partly damaged session still restores its readable items, and
	// reports the damaged folder as failed when restoring everything.
	partly := restoreSession{Parts: append(append([]keptSession{}, two.Parts...), damaged.Parts...)}
	if err := restoreRequestError(partly, 1, 0, false); err != nil {
		t.Fatalf("partly damaged: %v", err)
	}
	if res := damagedPartResults(partly, 1); len(res) != 1 || res[0].Status != "failed" || res[0].Path != "d" {
		t.Fatalf("damaged part results: %+v", res)
	}
}

func TestRestoreListShowsDamaged(t *testing.T) {
	now := time.Now()
	rs := []restoreSession{{ID: "x", Command: "purge", Created: now,
		Parts: []keptSession{{Dir: "d", Damaged: true, Created: now, Manifest: quarantineManifest{ID: "x"}}}}}
	var b bytes.Buffer
	renderRestoreList(&b, rs, now)
	if !strings.Contains(b.String(), "damaged") || strings.Contains(b.String(), "0 items") {
		t.Errorf("damaged session listing:\n%s", b.String())
	}
}
