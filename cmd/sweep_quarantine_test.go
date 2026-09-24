package cmd

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
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

// With no quarantine to keep them in, selected items stay in place and the
// finish views say so instead of counting them as swept.
func TestSweepsCountItemsThatCouldNotBeKept(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the stub quarantineRoot has no quarantine without a profile folder; Windows resolves a real one")
	}
	work := t.TempDir()
	t.Setenv("LOCALAPPDATA", "")
	t.Setenv("USERPROFILE", "")
	t.Setenv("DU_NO_OPLOG", "1")
	left := filepath.Join(work, "AppData", "Roaming", "SomeApp")
	writeTree(t, left)
	f := filepath.Join(work, "Downloads", "old-setup.exe")
	os.MkdirAll(filepath.Dir(f), 0o755)
	os.WriteFile(f, []byte("MZ"), 0o644)

	u, ok := runSweepCmd([]leftoverItem{{Path: left, Size: 5, Selected: true}}, false)().(sweepCompleteMsg)
	if !ok || u.failed != 1 || u.kept != 0 || u.size != 0 || !exists(left) {
		t.Fatalf("uninstall sweep without a quarantine: %#v, exists=%v", u, exists(left))
	}
	i, ok := runSetupSweepCmd([]installerItem{{Path: f, Size: 2, Selected: true}}, false)().(setupSweepCompleteMsg)
	if !ok || i.failed != 1 || i.kept != 0 || i.size != 0 || !exists(f) {
		t.Fatalf("installer sweep without a quarantine: %#v, exists=%v", i, exists(f))
	}

	m := uninstallModel{state: uninstStateFinished, kept: u.kept, keepFailed: u.failed, sweepWarn: "Warning: x"}
	if v := m.View(); !strings.Contains(v, "1 item could not be kept and was left in place") || !strings.Contains(v, "Warning: x") {
		t.Errorf("uninstall finish view:\n%s", v)
	}
	im := installerModel{state: instStateFinished, keepFailed: 2}
	if v := im.View(); !strings.Contains(v, "2 items could not be kept and were left in place") {
		t.Errorf("installer finish view:\n%s", v)
	}
}

func TestSweepFinishViewsSayKeptNotReclaimed(t *testing.T) {
	m := uninstallModel{state: uninstStateFinished, kept: 1, selectedSize: 2048}
	im := installerModel{state: instStateFinished, kept: 1, keptSize: 2048}
	for name, v := range map[string]string{"uninstall": m.View(), "installer": im.View()} {
		if !strings.Contains(v, "Kept") || !strings.Contains(v, "2.00 KB") || !strings.Contains(v, "for 7 days (du restore puts it back)") {
			t.Errorf("%s finish view lacks the kept line:\n%s", name, v)
		}
		if strings.Contains(strings.ToLower(v), "reclaimed") {
			t.Errorf("%s finish view calls kept bytes reclaimed:\n%s", name, v)
		}
	}
}

func TestSweepWarningIsOneLine(t *testing.T) {
	if sweepWarning(nil) != "" {
		t.Error("no errors must give no warning")
	}
	w := sweepWarning([]error{errors.New("a"), errors.New("b")})
	if strings.Contains(w, "\n") || !strings.HasPrefix(w, "Warning:") || !strings.Contains(w, "a; b") {
		t.Errorf("sweepWarning: %q", w)
	}
}
