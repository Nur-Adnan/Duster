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
