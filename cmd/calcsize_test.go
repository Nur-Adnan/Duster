package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

// calculateDirSize must sum only real files under the root and never follow a
// symlink/junction out of it (the documented boundary-leak safety guard).
func TestCalculateDirSizeSkipsSymlinks(t *testing.T) {
	root := t.TempDir()

	// Two real files inside the root: 100 + 200 = 300 bytes.
	if err := os.WriteFile(filepath.Join(root, "a.bin"), make([]byte, 100), 0o644); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(root, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "b.bin"), make([]byte, 200), 0o644); err != nil {
		t.Fatal(err)
	}

	// A large file in an external directory the symlink will point at.
	external := t.TempDir()
	if err := os.WriteFile(filepath.Join(external, "huge.bin"), make([]byte, 1<<20), 0o644); err != nil {
		t.Fatal(err)
	}

	// If the link can't be created (e.g. unprivileged Windows), the no-follow
	// guarantee still holds; just assert the base total.
	link := filepath.Join(root, "loop")
	if err := os.Symlink(external, link); err != nil {
		t.Skipf("symlink unsupported in this environment: %v", err)
	}

	if got := calculateDirSize(root); got != 300 {
		t.Errorf("calculateDirSize followed the symlink or miscounted: got %d, want 300", got)
	}
}
