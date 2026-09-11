package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

func TestStatusQuitsOnQ(t *testing.T) {
	tm := start(t, "status")
	// The dashboard is up once the screen holds more than a header.
	for end := time.Now().Add(30 * time.Second); len(squash(tm.screen())) < 300; time.Sleep(200 * time.Millisecond) {
		if time.Now().After(end) {
			t.Fatal("the status dashboard did not render")
		}
	}
	tm.send("q")
	if code := tm.waitExit(10 * time.Second); code != 0 {
		t.Fatalf("status exited with %d", code)
	}
}

// Enter a folder and back out, then d on a file sends it to the Recycle Bin.
func TestAnalyzeNavigatesAndRecycles(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "ALPHA-DIR", "zulu-inner.bin"), 2<<20)
	scratch := filepath.Join(root, "mike-scratch.txt")
	mustWrite(t, scratch, 4<<10)
	before := recycled(t)

	tm := start(t, "analyze", root)
	tm.waitFor("mike-scratch.txt", time.Minute)
	tm.send(keyEnter) // entries are listed largest first, so the folder is selected
	tm.waitFor("zulu-inner.bin", 5*time.Second)
	tm.send(keyBackspace, keyDown, "d")
	tm.waitFor("Send to Recycle Bin?", 5*time.Second)
	if s := squash(tm.screen()); !strings.Contains(s, "mike-scratch.txt") || strings.Contains(s, "zulu-inner.bin") {
		t.Fatal("after Backspace, d did not target the file in the parent folder")
	}
	tm.send("y")
	waitUntil(t, 15*time.Second, func() bool { return !exists(scratch) }, "y did not remove the file")
	if recycled(t) <= before {
		t.Fatal("the file was deleted instead of sent to the Recycle Bin")
	}
	tm.send("q")
	tm.waitExit(10 * time.Second)
}

// With the Recycle Bin smaller than the file, Windows must ask before deleting
// it permanently, and No must keep it.
func TestRecycleBinTooSmallAsksFirst(t *testing.T) {
	duBin(t)
	root := t.TempDir()
	big := filepath.Join(root, "oscar-big.bin")
	mustWrite(t, big, 8<<20)
	limitRecycleBin(t, filepath.VolumeName(root), 1)

	tm := start(t, "analyze", root)
	tm.waitFor("oscar-big.bin", time.Minute)
	tm.send("d")
	tm.waitFor("Send to Recycle Bin?", 5*time.Second)
	tm.send("y")
	t.Logf("Windows asked first (dialog %q); answered No", answer(t, tm.pid, "", idNo, 30*time.Second))
	tm.waitFor("Error recycling", 10*time.Second)
	if !exists(big) {
		t.Fatal("answering No still deleted the file")
	}
	tm.send("q")
	tm.waitExit(10 * time.Second)
}

// `du clean --dry-run` promises to delete nothing: c (force a real clean) must
// do nothing, while d still runs the dry run and says nothing was deleted.
func TestCleanDryRunIgnoresC(t *testing.T) {
	duBin(t)
	bait := filepath.Join(os.TempDir(), "duster-e2e-bait.txt") // Windows Temp Files would take it
	mustWrite(t, bait, 1<<10)
	t.Cleanup(func() { os.Remove(bait) })

	tm := start(t, "clean", "--dry-run")
	tm.waitFor("Ready to clean", 5*time.Minute)
	tm.send("c")
	time.Sleep(5 * time.Second)
	if !tm.onScreen("Ready to clean") || !exists(bait) {
		t.Fatal("c started a clean in a --dry-run session")
	}
	tm.send("d")
	tm.waitFor("nothing was deleted", 3*time.Minute)
	if !exists(bait) {
		t.Fatal("the dry run deleted a temp file")
	}
	tm.send("q")
	tm.waitExit(10 * time.Second)
}

// In the landing screen's Startup view, d asks first, any other key cancels,
// and d twice removes the disabled entries.
func TestLandingStartupRemoveAsksFirst(t *testing.T) {
	duBin(t)
	const name = "DusterE2EDisabled"
	run := openKey(t, `Software\Microsoft\Windows\CurrentVersion\Run`)
	approved := openKey(t, `Software\Microsoft\Windows\CurrentVersion\Explorer\StartupApproved\Run`)
	if err := run.SetStringValue(name, `"C:\Windows\System32\notepad.exe"`); err != nil {
		t.Fatal(err)
	}
	// A first byte of 3 is how Task Manager marks an entry disabled.
	if err := approved.SetBinaryValue(name, append([]byte{3}, make([]byte, 11)...)); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = run.DeleteValue(name)
		_ = approved.DeleteValue(name)
	})
	present := func() bool { _, _, err := run.GetStringValue(name); return err == nil }

	tm := start(t)
	tm.waitFor("Manage startup applications", 30*time.Second)
	tm.send("7") // Startup
	tm.waitFor(name, 30*time.Second)
	tm.send("d")
	tm.waitFor("Press d again to confirm", 5*time.Second)
	tm.send("x")
	tm.waitFor("Removal cancelled.", 5*time.Second)
	if !present() {
		t.Fatal("cancelling still removed the entry")
	}
	tm.send("d", "d")
	waitUntil(t, 15*time.Second, func() bool { return !present() }, "d twice did not remove the disabled entry")
	tm.send("q", "q")
	tm.waitExit(10 * time.Second)
}

// A real Inno Setup app. Its uninstaller hands off to a copy of itself in
// %TEMP% and exits; Duster must wait for that copy, see the entry gone, and
// list leftovers with nothing selected.
func TestUninstallRealApp(t *testing.T) {
	dir := install(t, false)
	t.Cleanup(func() { uninstallQuietly(dir) })
	leftover := filepath.Join(os.Getenv("APPDATA"), "Duster")
	mustWrite(t, filepath.Join(leftover, "e2e-settings.json"), 64)
	t.Cleanup(func() { os.RemoveAll(leftover) })

	tm := openUninstall(t, "HKLM")
	t.Logf("uninstaller asked %q: Yes", answer(t, 0, "Uninstall", idYes, time.Minute))
	t.Logf("uninstaller reported %q: OK", answer(t, 0, "Uninstall", idOK, 2*time.Minute))
	tm.waitFor(`Roaming\Duster`, 2*time.Minute)
	s := tm.screen()
	if strings.Contains(s, "[x]") {
		t.Fatal("a leftover folder was pre-selected")
	}
	if strings.Contains(squash(s), "sweepskipped") {
		t.Fatal("Duster reported the finished uninstall as not confirmed")
	}
	if exists(filepath.Join(dir, "du.exe")) {
		t.Fatal("the uninstaller did not remove the app")
	}
	tm.send("q")
	tm.waitExit(10 * time.Second)
	if !exists(leftover) {
		t.Fatal("quitting with nothing selected still deleted a leftover folder")
	}
}

// Cancelling the vendor's wizard keeps the app and skips the leftover sweep.
func TestUninstallCancelledWizard(t *testing.T) {
	dir := install(t, false)
	t.Cleanup(func() { uninstallQuietly(dir) })

	tm := openUninstall(t, "HKLM")
	t.Logf("uninstaller asked %q: No", answer(t, 0, "Uninstall", idNo, time.Minute))
	tm.waitFor("Leftover sweep skipped", time.Minute)
	tm.send(keyEnter)
	tm.waitFor("UNINSTALL NOT CONFIRMED", 5*time.Second)
	tm.waitFor("LEFTOVER SWEEP SKIPPED", 5*time.Second)
	if !exists(filepath.Join(dir, "du.exe")) {
		t.Fatal("the app is gone although its wizard was cancelled")
	}
	tm.send("q")
	tm.waitExit(10 * time.Second)
}

// A per-user app's uninstaller sits in a folder the user can write, so an
// elevated Duster refuses to run it.
func TestUninstallPerUserRefusedWhenElevated(t *testing.T) {
	if !windows.GetCurrentProcessToken().IsElevated() {
		t.Skip("needs an elevated session")
	}
	dir := install(t, true)
	t.Cleanup(func() { uninstallQuietly(dir) })

	tm := openUninstall(t, "HKCU")
	tm.waitFor("run Duster without administrator rights", time.Minute)
	if !exists(filepath.Join(dir, "du.exe")) {
		t.Fatal("the per-user app was uninstalled by an elevated Duster")
	}
	tm.send("q")
	tm.waitExit(10 * time.Second)
}

// openUninstall starts `du uninstall`, picks the Duster entry in hive, and
// confirms the launch.
func openUninstall(t *testing.T, hive string) *term {
	t.Helper()
	tm := start(t, "uninstall")
	tm.waitFor("Filter:", time.Minute)
	tm.typeText("Duster")
	tm.waitFor("Name : Duster", 10*time.Second)
	tm.waitFor("Hive : "+hive, 5*time.Second)
	tm.send(keyEnter)
	tm.waitFor("LAUNCH SYSTEM APP UNINSTALLER", 5*time.Second)
	tm.send("y")
	return tm
}

// install runs the Duster setup built by windows-smoke.yml (DU_E2E_SETUP)
// silently, for all users or for the current one, and returns its folder.
func install(t *testing.T, perUser bool) string {
	t.Helper()
	duBin(t)
	setup := os.Getenv("DU_E2E_SETUP")
	if setup == "" {
		t.Skip("DU_E2E_SETUP is not set")
	}
	dir, scope := filepath.Join(os.Getenv("ProgramFiles"), "DusterE2E"), "/ALLUSERS"
	if perUser {
		dir, scope = filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs", "DusterE2E"), "/CURRENTUSER"
	}
	c := exec.Command(setup)
	// Inno Setup wants /DIR="x", which Go's argument quoting can't produce.
	c.SysProcAttr = &syscall.SysProcAttr{CmdLine: `"` + setup + `" /VERYSILENT /SUPPRESSMSGBOXES /NORESTART ` + scope + ` /DIR="` + dir + `"`}
	if out, err := c.CombinedOutput(); err != nil {
		t.Fatalf("setup %s: %v %s", scope, err, out)
	}
	if !exists(filepath.Join(dir, "du.exe")) {
		t.Fatalf("setup did not install into %s", dir)
	}
	return dir
}

// uninstallQuietly removes an install a failed test left behind.
func uninstallQuietly(dir string) {
	unins := filepath.Join(dir, "unins000.exe")
	if !exists(unins) {
		return
	}
	_ = exec.Command(unins, "/VERYSILENT", "/SUPPRESSMSGBOXES", "/NORESTART").Run()
	// It hands off to a copy of itself and returns at once.
	for i := 0; i < 60 && exists(filepath.Join(dir, "du.exe")); i++ {
		time.Sleep(time.Second)
	}
}

// limitRecycleBin sets the Recycle Bin on vol ("C:") to mb megabytes, as its
// Properties dialog does, until the test ends.
func limitRecycleBin(t *testing.T, vol string, mb uint32) {
	t.Helper()
	root, err := windows.UTF16PtrFromString(vol + `\`)
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]uint16, 64)
	if err := windows.GetVolumeNameForVolumeMountPoint(root, &buf[0], uint32(len(buf))); err != nil {
		t.Fatal(err)
	}
	name := windows.UTF16ToString(buf) // \\?\Volume{GUID}\
	i, j := strings.Index(name, "{"), strings.Index(name, "}")
	if i < 0 || j < i {
		t.Fatalf("unexpected volume name %q", name)
	}
	k := openKey(t, `Software\Microsoft\Windows\CurrentVersion\Explorer\BitBucket\Volume\`+name[i:j+1])
	old, _, oldErr := k.GetIntegerValue("MaxCapacity")
	if err := k.SetDWordValue("MaxCapacity", mb); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if oldErr == nil {
			_ = k.SetDWordValue("MaxCapacity", uint32(old))
		} else {
			_ = k.DeleteValue("MaxCapacity")
		}
	})
}

// recycled counts the items in this user's Recycle Bin on the system drive.
func recycled(t *testing.T) int {
	t.Helper()
	u, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	m, _ := filepath.Glob(filepath.Join(os.Getenv("SystemDrive")+`\`, "$Recycle.Bin", u.User.Sid.String(), "$R*"))
	return len(m)
}

func openKey(t *testing.T, path string) registry.Key {
	t.Helper()
	k, _, err := registry.CreateKey(registry.CURRENT_USER, path, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { k.Close() })
	return k
}

func mustWrite(t *testing.T, path string, size int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, make([]byte, size), 0o644); err != nil {
		t.Fatal(err)
	}
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func waitUntil(t *testing.T, timeout time.Duration, ok func() bool, failure string) {
	t.Helper()
	for end := time.Now().Add(timeout); !ok(); time.Sleep(250 * time.Millisecond) {
		if time.Now().After(end) {
			t.Fatal(failure)
		}
	}
}
