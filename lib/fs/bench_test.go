package fs

import (
	"path/filepath"
	"testing"
)

// BenchmarkIsValidPath is the check purge and clean run on every directory
// (purge) or file (clean) they touch. "short-root" is a path under an 8.3
// root such as the runner's C:\Users\RUNNER~1\...\Temp; "long-root" is the
// same path after LongPath expanded the root once, as the walks now do.
func BenchmarkIsValidPath(b *testing.B) {
	dir := b.TempDir()
	for _, bc := range []struct{ name, path string }{
		{"short-root", filepath.Join(dir, "cache", "entry.bin")},
		{"long-root", filepath.Join(LongPath(dir), "cache", "entry.bin")},
	} {
		b.Run(bc.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				if !IsValidPath(bc.path) {
					b.Fatalf("%s reported invalid", bc.path)
				}
			}
		})
	}
}
