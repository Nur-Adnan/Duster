package cmd

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func TestInstallerModelInitialization(t *testing.T) {
	m := initialInstallerModel()

	if m.state != instStateScanning {
		t.Errorf("Expected initial state to be instStateScanning, got %v", m.state)
	}

	if m.cursor != 0 {
		t.Errorf("Expected starting cursor to be 0, got %d", m.cursor)
	}

	if m.scrollOffset != 0 {
		t.Errorf("Expected starting scrollOffset to be 0, got %d", m.scrollOffset)
	}

	if m.sweepSize != 0 {
		t.Errorf("Expected starting sweepSize to be 0, got %d", m.sweepSize)
	}
}

func TestInstallerModelTransitions(t *testing.T) {
	m := initialInstallerModel()

	// 1. Receive scan complete message with mock data
	mockItems := []installerItem{
		{
			Path:     `C:\Users\Default\Downloads\vscode_setup.exe`,
			Name:     "vscode_setup.exe",
			Size:     85 * 1024 * 1024,
			AgeDays:  14,
			ModTime:  time.Now().Add(-14 * 24 * time.Hour),
			Selected: true,
		},
		{
			Path:     `C:\Users\Default\Downloads\git_setup.exe`,
			Name:     "git_setup.exe",
			Size:     110 * 1024 * 1024,
			AgeDays:  9,
			ModTime:  time.Now().Add(-9 * 24 * time.Hour),
			Selected: true,
		},
	}

	msg := installerScanCompleteMsg{items: mockItems}
	updated, _ := m.Update(msg)
	instM := updated.(installerModel)

	if instM.state != instStateSelecting {
		t.Errorf("Expected state to become instStateSelecting, got %v", instM.state)
	}

	if len(instM.items) != 2 {
		t.Errorf("Expected 2 items, got %d", len(instM.items))
	}

	if instM.sweepSize != 195*1024*1024 {
		t.Errorf("Expected sweepSize to be 195MB, got %d", instM.sweepSize)
	}

	// 2. Press Space to uncheck first item
	spaceMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")}
	updated2, _ := instM.Update(spaceMsg)
	instM2 := updated2.(installerModel)

	if instM2.items[0].Selected {
		t.Error("Expected first item to be unselected after Space press")
	}

	if instM2.sweepSize != 110*1024*1024 {
		t.Errorf("Expected sweepSize to decrease to 110MB, got %d", instM2.sweepSize)
	}

	// 3. Press 'a' to toggle all back
	toggleMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")}
	updated3, _ := instM2.Update(toggleMsg)
	instM3 := updated3.(installerModel)

	if !instM3.items[0].Selected || !instM3.items[1].Selected {
		t.Error("Expected all items to become selected after 'a' toggle")
	}

	// 4. Press Enter to proceed to confirmation card
	enterMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("enter")}
	updated4, _ := instM3.Update(enterMsg)
	instM4 := updated4.(installerModel)

	if instM4.state != instStateConfirming {
		t.Errorf("Expected state to become instStateConfirming, got %v", instM4.state)
	}

	// 5. Test view works without panicking
	view := instM4.View()
	if len(view) == 0 {
		t.Error("Expected View to render styled TUI, got empty string")
	}
}

func TestScanInstallersCrawler(t *testing.T) {
	// Create a temp directory to simulate Downloads folder
	tempDir, err := os.MkdirTemp("", "duster-installer-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create Downloads structure
	var downloadsDir string
	if runtime.GOOS == "windows" {
		downloadsDir = filepath.Join(tempDir, "Downloads")
	} else {
		// Mock home Downloads path
		downloadsDir = filepath.Join(tempDir, "Downloads")
	}
	if err := os.MkdirAll(downloadsDir, 0755); err != nil {
		t.Fatalf("failed to create downloads directory: %v", err)
	}

	// Override environment variables so crawler targets our temp folder
	origHome := os.Getenv("HOME")
	origUserProfile := os.Getenv("USERPROFILE")
	defer func() {
		os.Setenv("HOME", origHome)
		os.Setenv("USERPROFILE", origUserProfile)
	}()

	if runtime.GOOS == "windows" {
		os.Setenv("USERPROFILE", tempDir)
	} else {
		os.Setenv("HOME", tempDir)
	}

	// Create mock installer files:
	// 1. Bulky outdated installer: SHOULD BE DISCOVERED
	oldBulkyPath := filepath.Join(downloadsDir, "outdated_large_setup.exe")
	oldBulkyData := make([]byte, 15*1024*1024) // 15MB
	_ = os.WriteFile(oldBulkyPath, oldBulkyData, 0644)
	oldTime := time.Now().Add(-10 * 24 * time.Hour)
	_ = os.Chtimes(oldBulkyPath, oldTime, oldTime)

	// 2. Fresh bulky installer: SHOULD BE FILTERED (too fresh, less than 7 days)
	freshPath := filepath.Join(downloadsDir, "fresh_large_setup.msi")
	_ = os.WriteFile(freshPath, oldBulkyData, 0644)

	// 3. Outdated small installer: SHOULD BE FILTERED (too small, less than minSize)
	smallPath := filepath.Join(downloadsDir, "outdated_small_setup.exe")
	smallData := make([]byte, 1024) // 1KB
	_ = os.WriteFile(smallPath, smallData, 0644)
	_ = os.Chtimes(smallPath, oldTime, oldTime)

	// Run scan command (setting minSize to 10MB to match our 15MB test files)
	cmdFn := scanInstallersCmd(10, make(chan []installerItem))
	msg := cmdFn()
	completeMsg := msg.(installerScanCompleteMsg)

	// Assert discovered items
	if len(completeMsg.items) != 1 {
		t.Fatalf("Expected exactly 1 installer to be scanned, got %d: %v", len(completeMsg.items), completeMsg.items)
	}

	discItem := completeMsg.items[0]
	if discItem.Name != "outdated_large_setup.exe" {
		t.Errorf("Expected discovered installer to be 'outdated_large_setup.exe', got %q", discItem.Name)
	}
}
