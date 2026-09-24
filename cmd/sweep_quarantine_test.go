package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUninstallSweepKeepsLeftovers(t *testing.T) {
	work := tempQuarantine(t)
	left := filepath.Join(work, "AppData", "Roaming", "SomeApp")
	writeTree(t, left)
	msg := runSweepCmd([]leftoverItem{{Path: left, Size: 5, Selected: true}}, false)()
	done, ok := msg.(sweepCompleteMsg)
	if !ok || done.kept != 1 || exists(left) {
		t.Fatalf("sweep: %#v, exists=%v", msg, exists(left))
	}
}

func TestInstallerSweepKeepsInstallers(t *testing.T) {
	work := tempQuarantine(t)
	f := filepath.Join(work, "Downloads", "old-setup.exe")
	os.MkdirAll(filepath.Dir(f), 0o755)
	os.WriteFile(f, []byte("MZ"), 0o644)
	msg := runSetupSweepCmd([]installerItem{{Path: f, Size: 2, Selected: true}}, false)()
	done, ok := msg.(setupSweepCompleteMsg)
	if !ok || done.kept != 1 || exists(f) {
		t.Fatalf("installer sweep: %#v", msg)
	}
}
