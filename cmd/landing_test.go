package cmd

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestLandingModelInitialization(t *testing.T) {
	m := initialLandingModel()
	if m.cursor != 0 {
		t.Errorf("Expected initial cursor to be 0, got %d", m.cursor)
	}
	if len(m.items) != 10 {
		t.Errorf("Expected 10 menu items, got %d", len(m.items))
	}
	if m.subTui != tuiNone {
		t.Errorf("Expected initial subTui to be tuiNone, got %v", m.subTui)
	}
}

func TestLandingModelNavigation(t *testing.T) {
	m := initialLandingModel()

	// 1. Move down
	updatedModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = updatedModel.(landingModel)
	if m.cursor != 1 {
		t.Errorf("Expected cursor to be 1 after pressing j, got %d", m.cursor)
	}

	// 2. Move up
	updatedModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	m = updatedModel.(landingModel)
	if m.cursor != 0 {
		t.Errorf("Expected cursor to be 0 after pressing k, got %d", m.cursor)
	}

	// 3. Move up boundary check (should not go below 0)
	updatedModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	m = updatedModel.(landingModel)
	if m.cursor != 0 {
		t.Errorf("Expected cursor to remain 0 when moving up from index 0, got %d", m.cursor)
	}

	// 4. Numeric Jump
	updatedModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("5")})
	m = updatedModel.(landingModel)
	if m.cursor != 4 {
		t.Errorf("Expected cursor to jump to 4 after pressing key '5', got %d", m.cursor)
	}

	// Reset runningSub simulation to allow subsequent key testing
	m.runningSub = false

	// 5. Jump to Exit (10th item via '0')
	updatedModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("0")})
	m = updatedModel.(landingModel)
	if m.cursor != 9 {
		t.Errorf("Expected cursor to jump to 9 after pressing key '0', got %d", m.cursor)
	}
}

func TestSubTuiStateTransitions(t *testing.T) {
	m := initialLandingModel()

	// 1. Simulate selecting Drivers (index 5) and pressing enter
	m.cursor = 5
	updatedModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updatedModel.(landingModel)

	if m.subTui != tuiDrivers {
		t.Errorf("Expected subTui to transition to tuiDrivers, got %v", m.subTui)
	}
	if m.subTuiState == nil {
		t.Error("Expected subTuiState to be initialized")
	}

	// 2. Simulate pressing esc inside Drivers TUI to return
	updatedModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updatedModel.(landingModel)

	if m.subTui != tuiNone {
		t.Errorf("Expected subTui to return to tuiNone, got %v", m.subTui)
	}
	if m.subTuiState != nil {
		t.Error("Expected subTuiState to be cleared")
	}
}

func TestViewSmokeRenders(t *testing.T) {
	m := initialLandingModel()

	// Main view smoke test
	view := m.View()
	if len(view) == 0 {
		t.Error("Expected View to render non-empty string")
	}

	// Sub-TUI smoke tests
	m.subTui = tuiDrivers
	m.subTuiState = initialDriversState()
	view = m.View()
	if len(view) == 0 {
		t.Error("Expected Drivers sub-TUI View to render non-empty string")
	}

	m.subTui = tuiStartup
	m.subTuiState = initialStartupState()
	view = m.View()
	if len(view) == 0 {
		t.Error("Expected Startup sub-TUI View to render non-empty string")
	}

	m.subTui = tuiNetwork
	m.subTuiState = initialNetworkState()
	view = m.View()
	if len(view) == 0 {
		t.Error("Expected Network sub-TUI View to render non-empty string")
	}

	m.subTui = tuiSecurity
	m.subTuiState = initialSecurityState()
	view = m.View()
	if len(view) == 0 {
		t.Error("Expected Security sub-TUI View to render non-empty string")
	}
}
