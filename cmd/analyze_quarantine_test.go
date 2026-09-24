package cmd

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// analyze's own delete (d, then y) falls back to Duster's quarantine when the
// Recycle Bin will not take the item. Off Windows, recyclePathNative always
// fails (stubs.go), so this exercises the fallback on every dev machine.
func TestAnalyzeRecycleFallsBackToQuarantine(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows actually recycles here; the fallback needs recyclePathNative to fail")
	}
	work := tempQuarantine(t)
	target := filepath.Join(work, "big.bin")
	if err := os.WriteFile(target, make([]byte, 10), 0o644); err != nil {
		t.Fatal(err)
	}

	tree, _, err := scanDirectory(work, nil)
	if err != nil {
		t.Fatalf("scanDirectory: %v", err)
	}

	m := analyzeModel{targetPath: work, tree: tree}
	m, _ = press(m, "d")
	if !m.confirmRecycle {
		t.Fatal("d did not arm the recycle confirmation")
	}
	m, _ = press(m, "y")

	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatal("the file is still at its original path after being kept")
	}
	if m.undo == nil || m.undo.Kept() != 1 {
		t.Fatalf("want the session to have kept one item, got %v", m.undo)
	}
	if !strings.Contains(m.notice, "Kept in Duster's quarantine") {
		t.Errorf("notice = %q, want it to mention Duster's quarantine", m.notice)
	}

	sessions := groupSessions(loadKeptSessions(quarantineRoots()))
	if len(sessions) != 1 || len(sessions[0].Items()) != 1 {
		t.Fatalf("want one session with one item listed, got %+v", sessions)
	}
}

// The keep triggers a rescan of the current folder to refresh sizes
// (analyze.go's "y" handler); the resulting analyzeScanCompleteMsg must not
// wipe the quarantine notice the way it wipes errorMsg, or the notice
// vanishes before anyone can read it (smoke run 36046777355).
func TestAnalyzeQuarantineNoticeSurvivesRescan(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows actually recycles here; the fallback needs recyclePathNative to fail")
	}
	work := tempQuarantine(t)
	target := filepath.Join(work, "big.bin")
	if err := os.WriteFile(target, make([]byte, 10), 0o644); err != nil {
		t.Fatal(err)
	}

	tree, _, err := scanDirectory(work, nil)
	if err != nil {
		t.Fatalf("scanDirectory: %v", err)
	}

	m := analyzeModel{targetPath: work, tree: tree}
	m, _ = press(m, "d")
	m, cmd := press(m, "y")
	if !strings.Contains(m.notice, "Kept in Duster's quarantine") {
		t.Fatalf("notice = %q, want it to mention Duster's quarantine before the rescan lands", m.notice)
	}
	if cmd == nil || !m.scanning {
		t.Fatal("keeping an item should trigger a rescan")
	}

	// Simulate that rescan landing, the way the real runtime feeds the
	// analyzeScanCompleteMsg its Init-triggered command eventually produces.
	rescanned, _, err := scanDirectory(work, nil)
	if err != nil {
		t.Fatalf("scanDirectory: %v", err)
	}
	next, _ := m.Update(analyzeScanCompleteMsg{Root: rescanned})
	m = next.(analyzeModel)
	if !strings.Contains(m.View(), "Kept in Duster's quarantine") {
		t.Errorf("View() after the rescan completed lost the quarantine notice: %q", m.notice)
	}
}
