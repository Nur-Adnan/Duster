package cmd

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestDoctorModelInitialization(t *testing.T) {
	m := initialDoctorModel()
	if !m.running {
		t.Error("Expected doctor model to start in a running state")
	}

	if len(m.logEntries) != 1 {
		t.Errorf("Expected 1 initial log entry, got %d", len(m.logEntries))
	}
}

func TestDoctorModelUpdate(t *testing.T) {
	m := initialDoctorModel()

	// 1. Simulate doctor complete event
	mockSnapshot := DoctorSnapshot{
		Healthy:  true,
		Passed:   8,
		Warnings: 1,
		Failed:   0,
		Results: []DoctorResult{
			{
				ID:      "privilege",
				Name:    "UAC Admin",
				Status:  "PASS",
				Message: "Elevated successfully",
			},
		},
	}

	completeMsg := doctorCompleteMsg(mockSnapshot)
	updated, _ := m.Update(completeMsg)
	docM := updated.(doctorModel)

	if docM.running {
		t.Error("Expected running state to be false after completion message")
	}

	if docM.snapshot.Passed != 8 {
		t.Errorf("Expected Passed count of 8, got %d", docM.snapshot.Passed)
	}

	// 2. Test view output works without panicking
	view := docM.View()
	if len(view) == 0 {
		t.Error("Expected View to return styled string, got empty string")
	}

	// 3. Test quit keyboard message
	quitMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")}
	_, quitCmd := docM.Update(quitMsg)
	if quitCmd == nil {
		t.Error("Expected quit Cmd to be returned for q keypress")
	}
}
