package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestJetBrainsCachesUseTheSafeEngine(t *testing.T) {
	local := t.TempDir()
	t.Setenv("LOCALAPPDATA", local)
	product := filepath.Join(local, "JetBrains", "GoLand2026.1")
	cacheFile := filepath.Join(product, "caches", "a", "cache.bin")
	if err := os.MkdirAll(filepath.Dir(cacheFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cacheFile, []byte("cache"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Settings live next to the caches and must survive.
	options := filepath.Join(product, "options", "ide.xml")
	if err := os.MkdirAll(filepath.Dir(options), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(options, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	// An index folder linked elsewhere (caches moved to another drive) must be
	// skipped, never followed.
	elsewhere := t.TempDir()
	linkedFile := filepath.Join(elsewhere, "index.bin")
	if err := os.WriteFile(linkedFile, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	linked := true
	if err := os.Symlink(elsewhere, filepath.Join(product, "index")); err != nil {
		linked = false
		t.Logf("symlinks unsupported, skipping the link check: %v", err)
	}

	if _, n, err := scanJetBrainsCaches(true, false); err != nil || n != 1 {
		t.Fatalf("scan found %d files (err %v), want only the cache file", n, err)
	}
	if _, n, err := scanJetBrainsCaches(false, false); err != nil || n != 1 {
		t.Fatalf("clean removed %d files (err %v), want 1", n, err)
	}
	if _, err := os.Stat(cacheFile); !os.IsNotExist(err) {
		t.Error("cache file survived the clean")
	}
	if _, err := os.Stat(options); err != nil {
		t.Errorf("settings next to the caches were touched: %v", err)
	}
	if linked {
		if _, err := os.Stat(linkedFile); err != nil {
			t.Errorf("clean followed the linked index folder: %v", err)
		}
	}
}
