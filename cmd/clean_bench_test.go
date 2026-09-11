package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// BenchmarkScanDirCategory measures the clean scan over a 2,000-file tree.
// On large caches the per-file work (placeholder check, pattern match, stat)
// dominates the whole scan.
func BenchmarkScanDirCategory(b *testing.B) {
	root := b.TempDir()
	for d := 0; d < 20; d++ {
		dir := filepath.Join(root, fmt.Sprintf("d%02d", d))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			b.Fatal(err)
		}
		for f := 0; f < 100; f++ {
			if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("f%03d.tmp", f)), []byte("x"), 0o644); err != nil {
				b.Fatal(err)
			}
		}
	}
	cat := CleanCategory{Name: "bench", Paths: []string{root}, FilesOnly: true}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, n, err := scanDirCategory(cat); err != nil || n != 2000 {
			b.Fatalf("scan found %d files (err %v), want 2000", n, err)
		}
	}
}
