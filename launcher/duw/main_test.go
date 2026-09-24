package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAllowedArgs(t *testing.T) {
	for _, args := range [][]string{
		{"schedule", "run"},
		{"schedule", "run", "--every", "weekly", "--low-space", "10"},
	} {
		if !allowedArgs(args) {
			t.Errorf("refused %v", args)
		}
	}
	for _, args := range [][]string{
		nil, {"schedule"}, {"clean", "--yes"}, {"schedule", "on"}, {"remove", "--force"}, {"run", "schedule"},
	} {
		if allowedArgs(args) {
			t.Errorf("accepted %v", args)
		}
	}
}

func TestRunRefusesOtherCommandsWithoutStartingDu(t *testing.T) {
	dir := t.TempDir()
	if code := run([]string{"clean", "--yes"}, filepath.Join(dir, "duw.exe"), dir); code != 2 {
		t.Errorf("exit %d, want 2", code)
	}
	if _, err := os.Stat(filepath.Join(dir, "schedule.log")); err == nil {
		t.Error("a refused call wrote the log")
	}
}

func TestRunReportsMissingDu(t *testing.T) {
	dir := t.TempDir()
	code := run([]string{"schedule", "run"}, filepath.Join(dir, "duw.exe"), dir)
	if code == 0 {
		t.Error("exit 0 without a du.exe beside duw.exe")
	}
	b, _ := os.ReadFile(filepath.Join(dir, "schedule.log"))
	if !strings.Contains(string(b), "du.exe") {
		t.Errorf("log does not say what failed: %q", b)
	}
}

func TestOpenScheduleLogRotates(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "schedule.log")
	if err := os.WriteFile(path, make([]byte, maxLogBytes+1), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := openScheduleLog(dir)
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	if info, err := os.Stat(path + ".old"); err != nil || info.Size() != maxLogBytes+1 {
		t.Errorf("old log not kept: %v", err)
	}
	if info, _ := os.Stat(path); info.Size() != 0 {
		t.Errorf("new log has %d bytes", info.Size())
	}
}

func TestOpenScheduleLogRefusesLinks(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(t.TempDir(), "elsewhere.txt")
	if err := os.WriteFile(target, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(dir, "schedule.log")); err != nil {
		t.Skipf("symlinks unsupported: %v", err)
	}
	if f, err := openScheduleLog(dir); err == nil {
		f.Close()
		t.Error("opened a log through a link")
	}

	linkedDir := filepath.Join(t.TempDir(), "Duster")
	if err := os.Symlink(t.TempDir(), linkedDir); err != nil {
		t.Skipf("symlinks unsupported: %v", err)
	}
	if f, err := openScheduleLog(linkedDir); err == nil {
		f.Close()
		t.Error("opened a log in a linked folder")
	}
}
