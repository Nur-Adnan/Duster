package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// removeFileSafe deletes read-only files and removes a link without touching
// its target.
func TestRemoveFileSafe(t *testing.T) {
	tmp := t.TempDir()
	ro := filepath.Join(tmp, "ro.txt")
	if err := os.WriteFile(ro, []byte("x"), 0o444); err != nil {
		t.Fatal(err)
	}
	if err := removeFileSafe(ro); err != nil {
		t.Fatalf("removeFileSafe(read-only): %v", err)
	}
	if _, err := os.Stat(ro); !os.IsNotExist(err) {
		t.Errorf("read-only file should be gone, err=%v", err)
	}

	target := filepath.Join(tmp, "target.txt")
	if err := os.WriteFile(target, []byte("keep"), 0o444); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(tmp, "link.txt")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := removeFileSafe(link); err != nil {
		t.Fatalf("removeFileSafe(link): %v", err)
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatalf("link target must survive: %v", err)
	}
	if info.Mode().Perm()&0o200 != 0 {
		t.Errorf("link target permissions changed through the link: %v", info.Mode())
	}
}

// A tree the first RemoveAll can't delete (read-only directory) is retried
// after clearing permissions.
func TestRemoveAllSafeRetriesReadOnlyDir(t *testing.T) {
	root := filepath.Join(t.TempDir(), "ro")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "f"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(root, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(root, 0o755) })
	if err := removeAllSafe(root); err != nil {
		t.Fatalf("removeAllSafe: %v", err)
	}
	if _, err := os.Lstat(root); !os.IsNotExist(err) {
		t.Fatalf("read-only tree should be gone, err=%v", err)
	}
}

// BenchmarkRemoveAllSafe measures deleting a 2,000-file tree (setup excluded).
func BenchmarkRemoveAllSafe(b *testing.B) {
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		root := filepath.Join(b.TempDir(), "tree")
		for d := 0; d < 50; d++ {
			dir := filepath.Join(root, fmt.Sprintf("d%d", d))
			if err := os.MkdirAll(dir, 0o755); err != nil {
				b.Fatal(err)
			}
			for f := 0; f < 40; f++ {
				if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("f%d", f)), []byte("x"), 0o644); err != nil {
					b.Fatal(err)
				}
			}
		}
		b.StartTimer()
		if err := removeAllSafe(root); err != nil {
			b.Fatal(err)
		}
	}
}

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
