package cmd

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestVerifyModelInitialization(t *testing.T) {
	m := initialVerifyModel()
	if !m.running {
		t.Error("Expected verify model to start in a running state")
	}
}

func TestVerifyModelUpdate(t *testing.T) {
	m := initialVerifyModel()

	// 1. Simulate verification complete event
	mockReport := VerifyReport{
		Healthy: true,
		Total:   7,
		Passed:  7,
		Failed:  0,
		Cases: []VerifyTestCase{
			{
				ID:     "path_protection",
				Name:   "Path Guard",
				Passed: true,
			},
		},
	}

	completeMsg := verifyCompleteMsg(mockReport)
	updated, _ := m.Update(completeMsg)
	verM := updated.(verifyModel)

	if verM.running {
		t.Error("Expected running state to be false after completion message")
	}

	if verM.report.Passed != 7 {
		t.Errorf("Expected Passed count of 7, got %d", verM.report.Passed)
	}

	// 2. Test view output works without panicking
	view := verM.View()
	if len(view) == 0 {
		t.Error("Expected View to return styled string, got empty string")
	}

	// 3. Test quit keyboard message
	quitMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")}
	_, quitCmd := verM.Update(quitMsg)
	if quitCmd == nil {
		t.Error("Expected quit Cmd to be returned for q keypress")
	}
}
