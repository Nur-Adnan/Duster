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
