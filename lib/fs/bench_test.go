package fs

import "testing"

// BenchmarkIsValidPath is the check purge and clean run on every directory
// they visit.
func BenchmarkIsValidPath(b *testing.B) {
	p := b.TempDir()
	for i := 0; i < b.N; i++ {
		if !IsValidPath(p) {
			b.Fatalf("temp dir %s reported invalid", p)
		}
	}
}
