//go:build windows

package cmd

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

func TestMoveNoReplaceRefusesExistingTarget(t *testing.T) {
	d := t.TempDir()
	a, b := filepath.Join(d, "a"), filepath.Join(d, "b")
	for p, s := range map[string]string{a: "a", b: "b"} {
		if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	err := moveNoReplace(a, b)
	if err == nil {
		t.Fatal("moveNoReplace overwrote an existing file")
	}
	if !errors.Is(err, os.ErrExist) {
		t.Errorf("error %v is not os.ErrExist", err)
	}
	if got, _ := os.ReadFile(b); string(got) != "b" {
		t.Fatalf("target changed to %q", got)
	}
	if got, _ := os.ReadFile(a); string(got) != "a" {
		t.Fatalf("source changed to %q", got)
	}
	c := filepath.Join(d, "c")
	if err := moveNoReplace(a, c); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(c); string(got) != "a" {
		t.Fatalf("moved file holds %q", got)
	}
}

func TestMoveNoReplaceRefusesExistingFolder(t *testing.T) {
	d := t.TempDir()
	src, dst := filepath.Join(d, "src"), filepath.Join(d, "dst")
	for _, p := range []string{src, dst} {
		if err := os.Mkdir(p, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := moveNoReplace(src, dst); err == nil {
		t.Fatal("moveNoReplace replaced an existing folder")
	}
	if !realDir(src) {
		t.Fatal("source folder is gone after a refused move")
	}
}

func TestCreatePrivateDirIsOwnedAndProtected(t *testing.T) {
	sid, err := currentUserSID()
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "priv")
	if err := createPrivateDir(dir, sid); err != nil {
		t.Fatal(err)
	}
	owner, err := dirOwner(dir)
	if err != nil || owner != sid {
		t.Fatalf("owner %q (%v), want %q", owner, err, sid)
	}
	sd, err := windows.GetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	ctrl, _, err := sd.Control()
	if err != nil || ctrl&windows.SE_DACL_PROTECTED == 0 {
		t.Fatalf("DACL not protected (control %#x, %v)", ctrl, err)
	}
	if err := checkOwnQuarantine(dir, filepath.VolumeName(dir)+`\`, sid); err != nil {
		t.Fatalf("own private folder refused: %v", err)
	}
	if err := checkOwnQuarantine(dir, filepath.VolumeName(dir)+`\`, "S-1-5-18"); err == nil {
		t.Fatal("a folder owned by someone else was accepted")
	}
	if err := createPrivateDir(dir, sid); !errors.Is(err, os.ErrExist) {
		t.Fatalf("second create: %v, want os.ErrExist", err)
	}
}

func TestQuarantineRootOnProfileVolume(t *testing.T) {
	t.Setenv("LOCALAPPDATA", t.TempDir())
	tmpVol, err1 := volumeRoot(os.TempDir())
	localVol, err2 := volumeRoot(os.Getenv("LOCALAPPDATA"))
	if err1 != nil || err2 != nil || !strings.EqualFold(tmpVol, localVol) {
		// Otherwise quarantineRoot would create X:\.duster-quarantine on a real drive.
		t.Skipf("TEMP (%s, %v) and LOCALAPPDATA (%s, %v) are on different volumes", tmpVol, err1, localVol, err2)
	}
	root, err := quarantineRoot(os.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if !realDir(root) || filepath.Base(root) != "quarantine" {
		t.Fatalf("root %q", root)
	}
	if !slices.Contains(quarantineRoots(), root) {
		t.Fatalf("quarantineRoots() = %v, missing %q", quarantineRoots(), root)
	}
}

func TestIsDriveLetterRoot(t *testing.T) {
	for root, want := range map[string]bool{
		`C:\`:               true,
		`d:\`:               true,
		`C:\mnt\data\`:      false,
		`\\server\share\`:   false,
		`\\?\Volume{1234}\`: false,
		`C:`:                false,
		`1:\`:               false,
		``:                  false,
	} {
		if got := isDriveLetterRoot(root); got != want {
			t.Errorf("isDriveLetterRoot(%q) = %v, want %v", root, got, want)
		}
	}
}
