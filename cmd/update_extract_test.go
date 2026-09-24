package cmd

import (
	"archive/zip"
	"bytes"
	"errors"
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
		got, err := extractFileFromZip(makeZip(t, map[string][]byte{"du.exe": mz}), "du.exe")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !bytes.Equal(got, mz) {
			t.Errorf("payload mismatch: got %q", got)
		}
	})

	t.Run("valid du.exe nested in a folder", func(t *testing.T) {
		got, err := extractFileFromZip(makeZip(t, map[string][]byte{
			"Duster-1.0.2-Portable-x64/du.exe":    mz,
			"Duster-1.0.2-Portable-x64/README.md": []byte("docs"),
		}), "du.exe")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !bytes.Equal(got, mz) {
			t.Errorf("payload mismatch: got %q", got)
		}
	})

	t.Run("rejects non-MZ payload", func(t *testing.T) {
		_, err := extractFileFromZip(makeZip(t, map[string][]byte{"du.exe": []byte("#!/bin/sh")}), "du.exe")
		if err == nil {
			t.Error("expected error for a non-Windows-executable payload, got nil")
		}
	})

	t.Run("rejects archive without du.exe", func(t *testing.T) {
		_, err := extractFileFromZip(makeZip(t, map[string][]byte{"notes.txt": mz}), "du.exe")
		if err == nil {
			t.Error("expected error when du.exe is absent, got nil")
		}
	})

	t.Run("rejects a non-zip blob", func(t *testing.T) {
		if _, err := extractFileFromZip([]byte("not a zip file at all"), "du.exe"); err == nil {
			t.Error("expected error for invalid zip, got nil")
		}
	})
}

func TestExtractLauncherFromZip(t *testing.T) {
	mz := []byte("MZ launcher")
	got, err := extractFileFromZip(makeZip(t, map[string][]byte{
		"Duster-1.3.0-Portable-x64/du.exe":  []byte("MZ du"),
		"Duster-1.3.0-Portable-x64/duw.exe": mz,
	}), "duw.exe")
	if err != nil || !bytes.Equal(got, mz) {
		t.Fatalf("duw.exe: %q, %v", got, err)
	}
	// Releases before scheduled cleaning have no duw.exe: not an error for update.
	_, err = extractFileFromZip(makeZip(t, map[string][]byte{"du.exe": []byte("MZ du")}), "duw.exe")
	if !errors.Is(err, errNotInArchive) {
		t.Errorf("missing duw.exe: %v, want errNotInArchive", err)
	}
}
