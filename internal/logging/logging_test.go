package logging

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLogDestructiveOperationWritesEntry(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LOCALAPPDATA", dir)
	t.Setenv("DU_NO_OPLOG", "") // ensure logging is enabled for this test

	LogDestructiveOperation("clean", "purge", "TestTarget", 123, true)

	data, err := os.ReadFile(filepath.Join(dir, "Duster", "operations.log"))
	if err != nil {
		t.Fatalf("expected an operations.log to be written: %v", err)
	}
	content := string(data)
	for _, want := range []string{
		"Command: clean", "Action: purge", "Target: TestTarget",
		"Size: 123 bytes", "Status: SUCCESS",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("log entry missing %q\ngot: %s", want, content)
		}
	}
}

func TestLogDestructiveOperationRecordsFailure(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LOCALAPPDATA", dir)
	t.Setenv("DU_NO_OPLOG", "")

	LogDestructiveOperation("purge", "delete", "X", 0, false)

	data, err := os.ReadFile(filepath.Join(dir, "Duster", "operations.log"))
	if err != nil {
		t.Fatalf("expected an operations.log: %v", err)
	}
	if !strings.Contains(string(data), "Status: FAILED") {
		t.Errorf("expected FAILED status, got: %s", data)
	}
}

func TestDuNoOplogSuppressesWrites(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LOCALAPPDATA", dir)
	t.Setenv("DU_NO_OPLOG", "1")

	LogDestructiveOperation("clean", "purge", "X", 1, true)

	if _, err := os.Stat(filepath.Join(dir, "Duster", "operations.log")); !os.IsNotExist(err) {
		t.Errorf("DU_NO_OPLOG=1 must suppress the log file, but Stat returned: %v", err)
	}
}

func TestRotateIfNeeded(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "operations.log")

	// Sparse file just over the rotation threshold (no 10 MB write needed).
	f, err := os.Create(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(int64(maxLogSize) + 1); err != nil {
		t.Fatal(err)
	}
	f.Close()

	rotateIfNeeded(logPath)

	if _, err := os.Stat(logPath + ".old"); err != nil {
		t.Errorf("expected the oversized log to rotate to .old: %v", err)
	}
	// A small log must be left untouched.
	small := filepath.Join(dir, "small.log")
	if err := os.WriteFile(small, []byte("tiny"), 0o644); err != nil {
		t.Fatal(err)
	}
	rotateIfNeeded(small)
	if _, err := os.Stat(small + ".old"); !os.IsNotExist(err) {
		t.Errorf("a sub-threshold log must not rotate")
	}
}
