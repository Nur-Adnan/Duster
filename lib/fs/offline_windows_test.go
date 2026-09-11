package fs

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

// Placeholder detection must work from the attributes a directory listing
// carries, since that is all the clean and installer walks hand it.
func TestIsOfflineInfoReadsListingAttributes(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"online.txt", "offline.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	p, err := syscall.UTF16PtrFromString(filepath.Join(dir, "offline.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if err := syscall.SetFileAttributes(p, FILE_ATTRIBUTE_OFFLINE); err != nil {
		t.Fatalf("SetFileAttributes: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			t.Fatal(err)
		}
		if got, want := IsOfflineInfo(info), e.Name() == "offline.txt"; got != want {
			t.Errorf("IsOfflineInfo(%s) = %v, want %v", e.Name(), got, want)
		}
	}
}

func TestLongPathExpandsShortNames(t *testing.T) {
	long := filepath.Join(t.TempDir(), "A Long Folder Name")
	if err := os.Mkdir(long, 0o755); err != nil {
		t.Fatal(err)
	}
	lp, err := syscall.UTF16PtrFromString(long)
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]uint16, 520)
	n, err := syscall.GetShortPathName(lp, &buf[0], uint32(len(buf)))
	if err != nil || n == 0 {
		t.Skipf("no short name available: %v", err)
	}
	short := syscall.UTF16ToString(buf[:n])
	if !strings.Contains(short, "~") {
		t.Skip("8.3 names are disabled on this volume")
	}

	got := LongPath(short)
	if strings.Contains(got, "~") {
		t.Fatalf("LongPath(%s) = %s, still has a short component", short, got)
	}
	if !strings.EqualFold(filepath.Base(got), "A Long Folder Name") {
		t.Fatalf("LongPath(%s) = %s, want it to end in the long folder name", short, got)
	}
}
