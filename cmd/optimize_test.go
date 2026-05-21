package cmd

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestOptimizeModelInitialization(t *testing.T) {
	m := initialOptimizeModel()
	if len(m.tasks) != 3 {
		t.Errorf("Expected 3 default optimization tasks, got %d", len(m.tasks))
	}

	if m.tasks[0].ID != "dns" {
		t.Errorf("Expected first task to be dns, got %s", m.tasks[0].ID)
	}

	if m.tasks[1].ID != "delivery_opt" {
		t.Errorf("Expected second task to be delivery_opt, got %s", m.tasks[1].ID)
	}

	if m.tasks[2].ID != "ssd_trim" {
		t.Errorf("Expected third task to be ssd_trim, got %s", m.tasks[2].ID)
	}

	if m.currentIdx != 0 {
		t.Errorf("Expected starting currentIdx to be 0, got %d", m.currentIdx)
	}

	if m.running {
		t.Error("Expected starting running state to be false")
	}
}

func TestOptimizeModelUpdate(t *testing.T) {
	m := initialOptimizeModel()

	// 1. Send enter key to start running tasks
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("enter")}
	updatedModel, cmd := m.Update(msg)
	optM := updatedModel.(optimizeModel)

	if !optM.running {
		t.Error("Expected model running state to become true after Enter key")
	}

	if optM.tasks[0].Status != statusRunning {
		t.Errorf("Expected first task status to become running, got %v", optM.tasks[0].Status)
	}

	if cmd == nil {
		t.Error("Expected running task command to be returned, got nil")
	}

	// 2. Simulate task progress completion message for first task
	progressMsg := optTaskProgressMsg{
		idx:       0,
		status:    statusCompleted,
		reclaimed: 1024 * 1024, // 1 MB
		err:       nil,
	}

	updatedModel2, cmd2 := optM.Update(progressMsg)
	optM2 := updatedModel2.(optimizeModel)

	if optM2.tasks[0].Status != statusCompleted {
		t.Errorf("Expected first task to be completed, got %v", optM2.tasks[0].Status)
	}

	if optM2.tasks[0].Reclaimed != 1024*1024 {
		t.Errorf("Expected first task reclaimed size to be 1MB, got %d", optM2.tasks[0].Reclaimed)
	}

	if optM2.currentIdx != 1 {
		t.Errorf("Expected currentIdx to increment to 1, got %d", optM2.currentIdx)
	}

	if optM2.tasks[1].Status != statusRunning {
		t.Errorf("Expected second task status to become running, got %v", optM2.tasks[1].Status)
	}

	if cmd2 == nil {
		t.Error("Expected second task command to be returned, got nil")
	}

	// 3. Test view output works without panicking
	view := optM2.View()
	if len(view) == 0 {
		t.Error("Expected View to return styled string, got empty string")
	}
}
