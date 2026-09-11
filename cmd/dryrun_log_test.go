package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

// A dry run deletes nothing, so it must not write deletions to the operations
// log. The uninstall and installer sweeps used to log each one as a success.
func TestDryRunSweepsLogNothing(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DU_NO_OPLOG", "")
	t.Setenv("LOCALAPPDATA", dir)
	target := filepath.Join(dir, "leftover")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}

	runSweepCmd([]leftoverItem{{Path: target, Size: 1, Selected: true}}, true)()
	runSetupSweepCmd([]installerItem{{Path: target, Size: 1, Selected: true}}, true)()

	if _, err := os.Stat(filepath.Join(dir, "Duster", "operations.log")); err == nil {
		t.Error("a dry run wrote deletions to the operations log")
	}
	if _, err := os.Stat(target); err != nil {
		t.Error("a dry run deleted its target")
	}
}
