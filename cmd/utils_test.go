package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

// removeAllSafe must delete a symlink/junction as the link itself and never
// follow it — neither to chmod nor to delete the target outside the root.
func TestRemoveAllSafeDoesNotFollowSymlink(t *testing.T) {
	tmp := t.TempDir()

	// A "target" tree that lives outside what we ask to delete.
	target := filepath.Join(tmp, "target")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatalf("mkdir target: %v", err)
	}
	keep := filepath.Join(target, "keep.txt")
	if err := os.WriteFile(keep, []byte("data"), 0o444); err != nil {
		t.Fatalf("write keep: %v", err)
	}

	link := filepath.Join(tmp, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable on this platform: %v", err)
	}

	if err := removeAllSafe(link); err != nil {
		t.Fatalf("removeAllSafe(link): %v", err)
	}
	if _, err := os.Lstat(link); !os.IsNotExist(err) {
		t.Errorf("expected link to be removed, got err=%v", err)
	}
	if _, err := os.Stat(keep); err != nil {
		t.Errorf("target file must survive, but got err=%v", err)
	}
}

// removeAllSafe deletes a normal directory tree, including read-only files.
func TestRemoveAllSafeDeletesReadOnlyTree(t *testing.T) {
	tmp := t.TempDir()
	dir := filepath.Join(tmp, "tree", "nested")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	f := filepath.Join(dir, "ro.txt")
	if err := os.WriteFile(f, []byte("x"), 0o444); err != nil {
		t.Fatalf("write: %v", err)
	}

	if err := removeAllSafe(filepath.Join(tmp, "tree")); err != nil {
		t.Fatalf("removeAllSafe: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tmp, "tree")); !os.IsNotExist(err) {
		t.Errorf("expected tree removed, got err=%v", err)
	}
}
