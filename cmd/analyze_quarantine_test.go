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
	if !strings.Contains(m.errorMsg, "Kept in Duster's quarantine") {
		t.Errorf("status = %q, want it to mention Duster's quarantine", m.errorMsg)
	}

	sessions := groupSessions(loadKeptSessions(quarantineRoots()))
	if len(sessions) != 1 || len(sessions[0].Items()) != 1 {
		t.Fatalf("want one session with one item listed, got %+v", sessions)
	}
}
