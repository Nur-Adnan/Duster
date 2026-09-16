package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// eventually reports whether want reaches the screen within timeout.
func (tm *term) eventually(want string, timeout time.Duration) bool {
	for end := time.Now().Add(timeout); time.Now().Before(end); time.Sleep(100 * time.Millisecond) {
		if tm.onScreen(want) {
			return true
		}
	}
	return false
}

// The report screens run to the end and exit cleanly on q.
func TestReportScreensFinishAndQuit(t *testing.T) {
	for _, name := range []string{"doctor", "verify", "benchmark"} {
		t.Run(name, func(t *testing.T) {
			tm := start(t, name)
			tm.waitFor("Press [q/esc] to exit.", 3*time.Minute)
			if name == "verify" && !tm.onScreen("SECURED & CERTIFIED") {
				// Same checks, same folder, no terminal: tells a Duster fault from
				// a screen-replay one.
				out, err := exec.Command(duBin(t), "verify", "--json").CombinedOutput()
				t.Errorf("verify did not certify the build; screen:\n%s\nverify --json (err %v):\n%s", tm.screen(), err, out)
			}
			tm.send("q")
			if code := tm.waitExit(10 * time.Second); code != 0 {
				t.Fatalf("%s exited with %d", name, code)
			}
		})
	}
}

// optimize --dry-run runs every task as a simulation.
func TestOptimizeDryRunSimulatesOnly(t *testing.T) {
	tm := start(t, "optimize", "--dry-run")
	tm.waitFor("DRY RUN MODE (SIMULATION)", 30*time.Second)
	tm.waitFor("Press [Enter] to run the optimization workflow", 30*time.Second)
	// The reported-only section is measured in the background, so the header
	// is there from the first frame and the sizes arrive shortly after.
	tm.waitFor("Reclaimable space", 30*time.Second)
	tm.send(keyEnter)
	tm.waitFor("Press [q] or [esc] to exit to CLI shell.", 2*time.Minute)
	if !tm.onScreen("Skipped (Simulation)") {
		t.Error("the dry run did not mark its tasks as simulated")
	}
	tm.send("q")
	tm.waitExit(10 * time.Second)
}

// The landing screen's Drivers, Network and Security views open without an
// error, and q returns to the menu.
func TestLandingViewsOpenAndReturn(t *testing.T) {
	tm := start(t)
	tm.waitFor("Manage startup applications", 30*time.Second)
	for _, v := range []struct{ key, title string }{
		{"6", "Installed Drivers Scanner"},
		{"8", "Real-time Network Traffic"},
		{"9", "Windows Security & Privacy Shield"},
	} {
		tm.send(v.key)
		tm.waitFor(v.title, time.Minute)
		time.Sleep(3 * time.Second) // let its scan finish or fail
		if tm.onScreen("Error:") {
			t.Errorf("%s shows an error", v.title)
		}
		tm.send("q")
		tm.waitFor("Manage startup applications", 10*time.Second)
	}
	tm.send("q")
	if code := tm.waitExit(10 * time.Second); code != 0 {
		t.Fatalf("exited with %d", code)
	}
}

// The installer screen lists only week-old setups, asks before deleting, n
// goes back, and y deletes what it listed.
func TestInstallerAsksThenDeletesOldSetups(t *testing.T) {
	duBin(t)
	dl := filepath.Join(os.Getenv("USERPROFILE"), "Downloads")
	old := filepath.Join(dl, "e2e-old-setup.exe")
	recent := filepath.Join(dl, "e2e-recent-setup.exe")
	for _, f := range []string{old, recent} {
		mustWrite(t, f, 60<<20)
		t.Cleanup(func() { os.Remove(f) })
	}
	tenDaysAgo := time.Now().AddDate(0, 0, -10)
	if err := os.Chtimes(old, tenDaysAgo, tenDaysAgo); err != nil {
		t.Fatal(err)
	}

	tm := start(t, "installer")
	tm.waitFor("e2e-old-setup.exe", time.Minute)
	if tm.onScreen("e2e-recent-setup.exe") {
		t.Fatal("a setup file newer than 7 days was listed")
	}
	tm.send(keyEnter)
	tm.waitFor("CONFIRM SETUPS PURGE WORKFLOW", 5*time.Second)
	tm.send("n")
	waitUntil(t, 5*time.Second, func() bool { return !tm.onScreen("CONFIRM SETUPS PURGE WORKFLOW") }, "n did not leave the confirmation")
	if !exists(old) {
		t.Fatal("going back deleted the file")
	}
	tm.send(keyEnter)
	tm.waitFor("CONFIRM SETUPS PURGE WORKFLOW", 5*time.Second)
	tm.send("y")
	tm.waitFor("INSTALLER SWEEP TRANSACTION COMPLETED", time.Minute)
	if exists(old) {
		t.Fatal("the confirmed sweep left the old setup")
	}
	if !exists(recent) {
		t.Fatal("the sweep deleted a recent setup")
	}
	tm.send("q")
	tm.waitExit(10 * time.Second)
}

// The purge screen asks before deleting, n goes back, and y removes only build
// folders that sit next to a project marker.
func TestPurgeAsksThenDeletesMarkedArtifacts(t *testing.T) {
	ws := t.TempDir()
	mustWrite(t, filepath.Join(ws, "app", "package.json"), 2)
	mustWrite(t, filepath.Join(ws, "app", "node_modules", "x", "f.js"), 1<<10)
	mustWrite(t, filepath.Join(ws, "bare", "node_modules", "x", "f.js"), 1<<10)
	marked, unmarked := filepath.Join(ws, "app", "node_modules"), filepath.Join(ws, "bare", "node_modules")

	tm := start(t, "purge", "--path", ws)
	tm.waitFor("PERMANENT CLEAN MODE", 30*time.Second)
	confirm := func() {
		t.Helper()
		for end := time.Now().Add(time.Minute); time.Now().Before(end); {
			tm.send(keyEnter) // ignored until the scan has finished
			if tm.eventually("CONFIRM BULK PURGE TRANSACTION", time.Second) {
				return
			}
		}
		t.Fatal("Enter never asked for confirmation")
	}
	confirm()
	tm.send("n")
	waitUntil(t, 5*time.Second, func() bool { return !tm.onScreen("CONFIRM BULK PURGE TRANSACTION") }, "n did not leave the confirmation")
	if !exists(marked) {
		t.Fatal("going back deleted the folder")
	}
	confirm()
	tm.send("y")
	tm.waitFor("DEVELOPER WORKSPACE PURGE COMPLETED", time.Minute)
	if tm.onScreen("COMPLETED WITH ERRORS") {
		t.Fatal("the purge reported errors")
	}
	if exists(marked) {
		t.Fatal("the confirmed purge left node_modules")
	}
	if !exists(unmarked) {
		t.Fatal("a node_modules with no project marker was purged")
	}
	tm.send("q")
	tm.waitExit(10 * time.Second)
}

// du remove asks first; n leaves the binary in place.
func TestRemoveCancelKeepsBinary(t *testing.T) {
	b, err := os.ReadFile(duBin(t))
	if err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(t.TempDir(), "du.exe")
	if err := os.WriteFile(exe, b, 0o755); err != nil {
		t.Fatal(err)
	}

	tm := startBin(t, exe, "remove")
	tm.waitFor("YOU ARE ABOUT TO COMPLETELY REMOVE DUSTER", 30*time.Second)
	tm.send("n")
	tm.waitExit(10 * time.Second)
	time.Sleep(3 * time.Second) // a scheduled self-delete would have run by now
	if !exists(exe) {
		t.Fatal("cancelling still removed the binary")
	}
}
