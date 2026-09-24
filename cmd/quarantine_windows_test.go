//go:build windows

package cmd

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Nur-Adnan/duster/internal/logging"
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
	// Exactly two entries, the user and SYSTEM: nothing inherited, nobody else.
	sddl := sd.String()
	i := strings.Index(sddl, "(")
	if i < 0 || !strings.HasPrefix(sddl, "D:") || !strings.Contains(sddl[:i], "P") {
		t.Fatalf("DACL %q is not protected", sddl)
	}
	if want := "(A;OICI;FA;;;" + sid + ")(A;OICI;FA;;;SY)"; sddl[i:] != want {
		t.Fatalf("DACL entries %q, want %q", sddl[i:], want)
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
	if err := os.MkdirAll(logging.Dir(), 0o700); err != nil {
		t.Fatal(err)
	}
	tmpVol, err1 := volumeRoot(os.TempDir())
	localVol, err2 := volumeRoot(logging.Dir())
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

func TestPinnedQuarantineCannotBeRenamed(t *testing.T) {
	sid, err := currentUserSID()
	if err != nil {
		t.Fatal(err)
	}
	parent := filepath.Join(t.TempDir(), "base")
	if err := os.Mkdir(parent, 0o700); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(parent, "priv")
	if err := createPrivateDir(dir, sid); err != nil {
		t.Fatal(err)
	}
	vr, err := volumeRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := pinQuarantineDir(dir, vr, sid); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { unpinQuarantineDir(dir) }) // runs before TempDir's cleanup
	if !quarantinePinned(dir) {
		t.Fatal("dir is not pinned")
	}
	if err := pinQuarantineDir(dir, vr, sid); err != nil {
		t.Fatalf("second pin: %v", err)
	}
	if err := os.Rename(dir, dir+"-moved"); err == nil {
		t.Fatal("a pinned quarantine folder was renamed")
	}
	if err := os.Rename(parent, parent+"-moved"); err == nil {
		t.Fatal("the parent of a pinned quarantine folder was renamed")
	}
	if !realDir(dir) {
		t.Fatal("pinned folder is gone")
	}
	// Once unpinned, the same rename works: the pin was what refused it.
	unpinQuarantineDir(dir)
	if err := os.Rename(parent, parent+"-moved"); err != nil {
		t.Fatalf("rename after unpin: %v", err)
	}
}

func TestPinRefusesForeignOwnerAndJunctionPath(t *testing.T) {
	sid, err := currentUserSID()
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "priv")
	if err := createPrivateDir(dir, sid); err != nil {
		t.Fatal(err)
	}
	vr, err := volumeRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := pinQuarantineDir(dir, vr, "S-1-5-18"); err == nil {
		unpinQuarantineDir(dir)
		t.Fatal("pinned a folder owned by someone else")
	}
	if quarantinePinned(dir) {
		t.Fatal("a refused folder stayed pinned")
	}
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(dir, link); err != nil {
		t.Skipf("symlinks unsupported: %v", err)
	}
	if err := pinQuarantineDir(link, vr, sid); err == nil {
		unpinQuarantineDir(link)
		t.Fatal("pinned a link")
	}
}

func TestMoveNoReplaceLongPath(t *testing.T) {
	d := t.TempDir()
	long := d
	for len(long) < 300 {
		long = filepath.Join(long, strings.Repeat("x", 40))
	}
	if err := os.MkdirAll(long, 0o755); err != nil {
		t.Fatal(err)
	}
	a, b := filepath.Join(long, "a.txt"), filepath.Join(long, "b.txt")
	if len(a) <= 300 {
		t.Fatalf("path is only %d characters", len(a))
	}
	if err := os.WriteFile(a, []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := moveNoReplace(a, b); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(b); err != nil || string(got) != "a" {
		t.Fatalf("moved file holds %q (%v)", got, err)
	}
	if err := moveNoReplace(b, a); err != nil {
		t.Fatalf("move back: %v", err)
	}
	if _, err := os.Lstat(a); err != nil {
		t.Fatal(err)
	}
	lv, err := volumeRoot(long)
	if err != nil {
		t.Fatal(err)
	}
	if tv, _ := volumeRoot(d); !strings.EqualFold(lv, tv) {
		t.Fatalf("volumeRoot of a long path = %q, want %q", lv, tv)
	}
}

func TestExtendedPath(t *testing.T) {
	long := `C:\` + strings.Repeat(`dir\`, 60) + "f"
	unc := `\\srv\share\` + strings.Repeat(`dir\`, 60) + "f"
	for in, want := range map[string]string{
		`C:\short\path`:            `C:\short\path`,
		long:                       `\\?\` + long,
		unc:                        `\\?\UNC\srv\share\` + strings.Repeat(`dir\`, 60) + "f",
		`\\?\` + long:              `\\?\` + long,
		strings.Repeat(`rel\`, 60): strings.Repeat(`rel\`, 60),
	} {
		if got := extendedPath(in); got != want {
			t.Errorf("extendedPath(%.30q...) = %.40q..., want %.40q...", in, got, want)
		}
		if plainPath(extendedPath(in)) != in && !strings.HasPrefix(in, `\\?\`) {
			t.Errorf("plainPath does not undo extendedPath for %.30q...", in)
		}
	}
}
