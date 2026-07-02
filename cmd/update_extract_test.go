package cmd

import (
	"archive/zip"
	"bytes"
	"testing"
)

// makeZip builds an in-memory zip from name->content entries.
func makeZip(t *testing.T, entries map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, data := range entries {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("zip create %q: %v", name, err)
		}
		if _, err := w.Write(data); err != nil {
			t.Fatalf("zip write %q: %v", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zip close: %v", err)
	}
	return buf.Bytes()
}

func TestExtractBinaryFromZip(t *testing.T) {
	mz := []byte("MZ\x90\x00\x03 fake pe payload")

	t.Run("valid du.exe at root", func(t *testing.T) {
		got, err := extractBinaryFromZip(makeZip(t, map[string][]byte{"du.exe": mz}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !bytes.Equal(got, mz) {
			t.Errorf("payload mismatch: got %q", got)
		}
	})

	t.Run("valid du.exe nested in a folder", func(t *testing.T) {
		got, err := extractBinaryFromZip(makeZip(t, map[string][]byte{
			"Duster-1.0.2-Portable-x64/du.exe":    mz,
			"Duster-1.0.2-Portable-x64/README.md": []byte("docs"),
		}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !bytes.Equal(got, mz) {
			t.Errorf("payload mismatch: got %q", got)
		}
	})

	t.Run("rejects non-MZ payload", func(t *testing.T) {
		_, err := extractBinaryFromZip(makeZip(t, map[string][]byte{"du.exe": []byte("#!/bin/sh")}))
		if err == nil {
			t.Error("expected error for a non-Windows-executable payload, got nil")
		}
	})

	t.Run("rejects archive without du.exe", func(t *testing.T) {
		_, err := extractBinaryFromZip(makeZip(t, map[string][]byte{"notes.txt": mz}))
		if err == nil {
			t.Error("expected error when du.exe is absent, got nil")
		}
	})

	t.Run("rejects a non-zip blob", func(t *testing.T) {
		if _, err := extractBinaryFromZip([]byte("not a zip file at all")); err == nil {
			t.Error("expected error for invalid zip, got nil")
		}
	})
}
