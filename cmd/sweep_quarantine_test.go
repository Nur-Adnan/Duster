package cmd

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
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

// du restore sweeps with expiry only: a fresh session on a nearly full drive
// must never be removed before the user can restore it.
func TestPickSweepExpiredOnlyNeverTakesFreshSessionsForSpace(t *testing.T) {
	now := time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)
	mk := func(name string, age time.Duration) keptSession {
		return keptSession{Root: "D", Dir: "D/" + name, Created: now.Add(-age),
			Manifest: quarantineManifest{ID: name, Items: []quarantineItem{{Size: 1, State: "kept"}}}}
	}
	ks := []keptSession{mk("old", 8*24*time.Hour), mk("fresh", time.Hour)}
	vols := map[string]volSpace{"D": {Free: 0, Total: 100}}
	if got := strings.Join(pickSweep(ks, vols, now, false), ","); got != "D/old" {
		t.Errorf("expiry only: %q, want only the expired session", got)
	}
	if got := strings.Join(pickSweep(ks, vols, now, true), ","); got != "D/old,D/fresh" {
		t.Errorf("full sweep on a full drive: %q", got)
	}
}

// ageSession rewrites every manifest under the local root so its sessions
// look created at t.
func ageSession(t *testing.T, at time.Time) {
	t.Helper()
	for _, k := range loadKeptSessions(quarantineRoots()) {
		m := k.Manifest
		m.Created = at
		if err := writeManifest(k.Dir, &m); err != nil {
			t.Fatal(err)
		}
	}
}

func TestSweepQuarantineReportsWhatItRemoved(t *testing.T) {
	work := tempQuarantine(t)
	for _, name := range []string{"a.txt", "b.txt"} {
		f := filepath.Join(work, name)
		os.WriteFile(f, []byte("abc"), 0o644)
		if err := quarantinePath(newQuarantineSession("purge-"+name), f, 3); err != nil {
			t.Fatal(err)
		}
	}
	ageSession(t, time.Now().Add(-8*24*time.Hour))
	f := filepath.Join(work, "fresh.txt")
	os.WriteFile(f, []byte("x"), 0o644)
	if err := quarantinePath(newQuarantineSession("purge"), f, 1); err != nil {
		t.Fatal(err)
	}

	rep := sweepQuarantine(time.Now(), sweepExpiredOnly)
	if rep.Expired != 2 || rep.ExpiredBytes != 6 || rep.LowSpace != 0 || len(rep.Errs) != 0 {
		t.Fatalf("report: %+v", rep)
	}
	if l := rep.expiredLine(); !strings.HasPrefix(l, "Removed 2 expired sessions (") {
		t.Errorf("expiredLine: %q", l)
	}
	if left := groupSessions(loadKeptSessions(quarantineRoots())); len(left) != 1 || left[0].Size() != 1 {
		t.Errorf("the fresh session must stay kept: %+v", left)
	}
	if rep := sweepQuarantine(time.Now(), sweepExpiredOnly); rep.expiredLine() != "" || sweepNotice(rep) != "" {
		t.Errorf("a sweep that removed nothing must say nothing: %+v", rep)
	}
}

func TestSweepNoticeNamesLowSpaceRemovals(t *testing.T) {
	rep := sweepReport{Expired: 3, ExpiredBytes: 10, LowSpace: 2, LowSpaceBytes: 2048, LowSpaceVols: []string{"D:"}}
	n := sweepNotice(rep)
	if strings.Contains(n, "\n") || !strings.Contains(n, "2 kept sessions") || !strings.Contains(n, "2.00 KB") || !strings.Contains(n, "low space on D:") {
		t.Errorf("sweepNotice: %q", n)
	}
	if strings.Contains(n, "expired") {
		t.Errorf("finish views do not report routine expiry: %q", n)
	}
	rep.Errs = []error{errors.New("locked")}
	if n := sweepNotice(rep); strings.Contains(n, "\n") || !strings.Contains(n, "low space on D:") || !strings.Contains(n, "Warning:") {
		t.Errorf("sweepNotice with errors: %q", n)
	}
	rep.Errs = nil
	pm := purgeModel{state: stateFinished, purgeTally: purgeTally{sweep: rep}}
	if v := pm.View(); !strings.Contains(v, "low space on D:") {
		t.Errorf("purge finish view lacks the low-space line:\n%s", v)
	}
}
