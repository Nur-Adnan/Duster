package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReplaceFile(t *testing.T) {
	t.Run("replaces and keeps the original aside", func(t *testing.T) {
		target := filepath.Join(t.TempDir(), "duw.exe")
		if err := os.WriteFile(target, []byte("old"), 0o755); err != nil {
			t.Fatal(err)
		}
		old, err := replaceFile(target, []byte("new"))
		if err != nil {
			t.Fatal(err)
		}
		if b, _ := os.ReadFile(target); string(b) != "new" {
			t.Errorf("target holds %q", b)
		}
		if b, _ := os.ReadFile(old); old != target+".old" || string(b) != "old" {
			t.Errorf("original at %q holds %q", old, b)
		}
		if _, err := os.Stat(target + ".new"); err == nil {
			t.Error("staged file left behind")
		}
	})
	t.Run("installs where there was none", func(t *testing.T) {
		target := filepath.Join(t.TempDir(), "duw.exe")
		old, err := replaceFile(target, []byte("new"))
		if err != nil || old != "" {
			t.Fatalf("old %q, err %v", old, err)
		}
		if b, _ := os.ReadFile(target); string(b) != "new" {
			t.Errorf("target holds %q", b)
		}
	})
	t.Run("a failed stage changes nothing", func(t *testing.T) {
		dir := t.TempDir()
		target := filepath.Join(dir, "duw.exe")
		if err := os.WriteFile(target, []byte("old"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(target+".new", 0o755); err != nil { // makes the staging write fail
			t.Fatal(err)
		}
		if _, err := replaceFile(target, []byte("new")); err == nil {
			t.Fatal("no error")
		}
		if b, _ := os.ReadFile(target); string(b) != "old" {
			t.Errorf("target changed to %q", b)
		}
	})
}
