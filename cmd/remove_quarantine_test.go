package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Nur-Adnan/duster/internal/logging"
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

// tempQuarantine must hide every other drive's quarantine for the test (on
// Windows those hold a developer's real kept items) and restore the seam after.
func TestTempQuarantineSeesOnlyItsOwnRoot(t *testing.T) {
	t.Run("inside", func(t *testing.T) {
		work := tempQuarantine(t)
		if otherDriveQuarantines {
			t.Fatal("tempQuarantine left the other drives' quarantines on")
		}
		f := filepath.Join(work, "a.txt")
		os.WriteFile(f, []byte("a"), 0o644)
		if err := quarantinePath(newQuarantineSession("purge"), f, 1); err != nil {
			t.Fatal(err)
		}
		local := strings.ToLower(filepath.Join(logging.Dir(), "quarantine"))
		roots := quarantineRoots()
		if len(roots) != 1 || strings.ToLower(roots[0]) != local {
			t.Fatalf("quarantineRoots() = %v, want only %s", roots, local)
		}
	})
	if !otherDriveQuarantines {
		t.Fatal("tempQuarantine did not restore otherDriveQuarantines")
	}
}
