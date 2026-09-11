package cmd

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestRemoveModelInitialization(t *testing.T) {
	local := t.TempDir()
	t.Setenv("LOCALAPPDATA", local)
	exePath := `C:\Users\Default\AppData\Local\Temp\duster.exe`
	m := initialRemoveModel(exePath)

	if m.state != stateRmIdle {
		t.Errorf("Expected initial state to be stateRmIdle, got %v", m.state)
	}

	if m.currentExe != exePath {
		t.Errorf("Expected currentExe path to be %s, got %s", exePath, m.currentExe)
	}

	// The data dir must come from the profile, never a working-directory
	// fallback: `du remove` deletes whatever this resolves to.
	if want := filepath.Join(local, "Duster"); m.logDir != want {
		t.Errorf("logDir = %q, want %q", m.logDir, want)
	}
}

func TestRemoveModelTransitions(t *testing.T) {
	exePath := `C:\Users\Default\AppData\Local\Temp\duster.exe`
	m := initialRemoveModel(exePath)

	// 1. Press Y to confirm uninstall
	yMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")}
	updatedModel, cmd := m.Update(yMsg)
	rmM := updatedModel.(removeModel)

	if rmM.state != stateRmUninstalling {
		t.Errorf("Expected state to become stateRmUninstalling after confirmation, got %v", rmM.state)
	}

	if cmd == nil {
		t.Error("Expected uninstallation command to be returned, got nil")
	}

	// 2. Simulate uninstall complete (dry run context / simulated)
	rmDryRun = true
	defer func() { rmDryRun = false }()

	completeMsg := rmUninstallCompleteMsg{err: nil}
	updatedModel2, _ := rmM.Update(completeMsg)
	rmM2 := updatedModel2.(removeModel)

	if rmM2.state != stateRmFinished {
		t.Errorf("Expected state to become stateRmFinished, got %v", rmM2.state)
	}

	if !strings.Contains(rmM2.statusMsg, "completed successfully") {
		t.Errorf("Expected successful status message, got: %q", rmM2.statusMsg)
	}

	// Test View rendering in finished state
	view := rmM2.View()
	if len(view) == 0 {
		t.Error("Expected View to render styled uninstaller, got empty string")
	}
}

func TestRemoveModelCancel(t *testing.T) {
	exePath := `C:\Users\Default\AppData\Local\Temp\duster.exe`
	m := initialRemoveModel(exePath)

	// Press N to cancel uninstall
	nMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")}
	_, cmd := m.Update(nMsg)

	if cmd == nil {
		t.Fatal("Expected command to be returned")
	}

	// Verify the command is tea.Quit
	msg := cmd()
	if _, ok := msg.(tea.QuitMsg); !ok {
		t.Errorf("Expected command to trigger tea.Quit, got: %T", msg)
	}
}

func TestRemoveModelErrorHandling(t *testing.T) {
	exePath := `C:\Users\Default\AppData\Local\Temp\duster.exe`
	m := initialRemoveModel(exePath)
	m.state = stateRmUninstalling

	completeMsg := rmUninstallCompleteMsg{err: errors.New("permission denied")}
	updatedModel, _ := m.Update(completeMsg)
	rmM := updatedModel.(removeModel)

	if rmM.state != stateRmFinished {
		t.Errorf("Expected error to transition to stateRmFinished, got %v", rmM.state)
	}

	if rmM.err == nil {
		t.Error("Expected error field to be populated")
	}

	if !strings.Contains(rmM.statusMsg, "permission denied") {
		t.Errorf("Expected error message in statusMsg, got: %q", rmM.statusMsg)
	}
}

func TestCleanDusterDirSafety(t *testing.T) {
	// Verify that unsafe paths are blocked
	unsafePaths := []string{
		"",
		".",
		"/",
		`C:\`,
		`D:\`,
		`C:`,
	}

	for _, path := range unsafePaths {
		err := cleanDusterDir(path, "", false)
		if err == nil {
			t.Errorf("Expected error for unsafe path %q, but deletion was allowed", path)
		}
	}

	// Verify that a safe path is allowed
	err := cleanDusterDir(`C:\Users\Default\AppData\Local\Duster`, "", true)
	if err != nil {
		t.Errorf("Expected safe path to be allowed in dry-run, got error: %v", err)
	}
}

// The default install puts du.exe inside the data dir; removal must delete
// everything else and leave the running exe for the delayed self-delete.
func TestCleanDusterDirKeepsRunningExe(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "Duster")
	if err := os.MkdirAll(filepath.Join(dir, "cache"), 0o755); err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(dir, "du.exe")
	for _, f := range []string{exe, filepath.Join(dir, "operations.log"), filepath.Join(dir, "cache", "x")} {
		if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// Spell the exe path differently (macOS /var -> /private/var): identity,
	// not string equality, must decide what is kept.
	exeAlias, err := filepath.EvalSymlinks(exe)
	if err != nil {
		t.Fatal(err)
	}
	if err := cleanDusterDir(dir, exeAlias, false); err != nil {
		t.Fatalf("cleanDusterDir: %v", err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 || entries[0].Name() != "du.exe" {
		t.Fatalf("want only du.exe left, got %v", entries)
	}

	// Exe installed elsewhere: the whole data dir goes, including the dir.
	other := filepath.Join(t.TempDir(), "Duster")
	if err := os.MkdirAll(filepath.Join(other, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(other, "sub", "f"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := cleanDusterDir(other, exe, false); err != nil {
		t.Fatalf("cleanDusterDir(other): %v", err)
	}
	if _, err := os.Stat(other); !os.IsNotExist(err) {
		t.Fatalf("data dir should be gone when the exe lives elsewhere, err=%v", err)
	}
}
