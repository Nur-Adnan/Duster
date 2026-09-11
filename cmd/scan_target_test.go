package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

// analyze and purge must reject a mistyped path instead of reporting an empty
// scan with exit 0 (found by the Windows smoke test's invalid-input checks).
func TestScanTargetError(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name    string
		path    string
		needDir bool
		wantErr bool
	}{
		{"folder", dir, true, false},
		{"missing path", filepath.Join(dir, "missing"), false, true},
		{"file where purge needs a folder", file, true, true},
		{"file is fine for analyze", file, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := scanTargetError(tt.path, tt.needDir); (err != nil) != tt.wantErr {
				t.Errorf("scanTargetError(%q, %v) = %v, want error: %v", tt.path, tt.needDir, err, tt.wantErr)
			}
		})
	}
}
