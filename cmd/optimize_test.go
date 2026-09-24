package cmd

import (
	"strings"
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

// Headless optimize exits 1 when any task failed; skipped tasks don't count.
func TestAnyTaskFailed(t *testing.T) {
	tests := []struct {
		name     string
		statuses []taskStatus
		want     bool
	}{
		{"all completed", []taskStatus{statusCompleted, statusCompleted}, false},
		{"preview skips everything", []taskStatus{statusSkipped, statusSkipped, statusSkipped}, false},
		{"one failed", []taskStatus{statusCompleted, statusFailed, statusSkipped}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var tasks []optimizeTask
			for _, s := range tt.statuses {
				tasks = append(tasks, optimizeTask{Status: s})
			}
			if got := anyTaskFailed(tasks); got != tt.want {
				t.Errorf("anyTaskFailed = %v, want %v", got, tt.want)
			}
		})
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

// The default run stays fast: the component store cleanup is opt-in, because
// it needs admin rights and can run for tens of minutes.
func TestOptimizeTasksDeepIsOptIn(t *testing.T) {
	defaults := optimizeTasks(false)
	wantIDs := []string{"dns", "delivery_opt", "ssd_trim"}
	if len(defaults) != len(wantIDs) {
		t.Fatalf("default run has %d tasks, want %d", len(defaults), len(wantIDs))
	}
	for i, id := range wantIDs {
		if defaults[i].ID != id {
			t.Errorf("task %d is %q, want %q", i, defaults[i].ID, id)
		}
	}

	deep := optimizeTasks(true)
	if len(deep) != len(wantIDs)+1 {
		t.Fatalf("--deep run has %d tasks, want %d", len(deep), len(wantIDs)+1)
	}
	last := deep[len(deep)-1]
	if last.ID != "component_store" {
		t.Errorf("--deep added %q, want component_store", last.ID)
	}
	if last.Status != statusPending {
		t.Errorf("component_store starts as %v, want pending", last.Status)
	}
}

// Without admin rights the task is skipped and says why; it must never reach
// DISM, which would fail with "elevated permissions are required".
func TestComponentStoreTaskNeedsAdmin(t *testing.T) {
	res := runComponentStoreTask(false, false)
	if res.status != statusSkipped {
		t.Errorf("status = %v, want skipped", res.status)
	}
	if res.note == "" || res.err != nil {
		t.Errorf("note = %q, err = %v; want a reason and no error", res.note, res.err)
	}
	if res.reclaimed != 0 {
		t.Errorf("a skipped task reported %d bytes reclaimed", res.reclaimed)
	}
}

// runningComponentStoreModel is a TUI in the middle of the DISM servicing task.
func runningComponentStoreModel() optimizeModel {
	tasks := optimizeTasks(true)
	idx := len(tasks) - 1
	tasks[idx].Status = statusRunning
	return optimizeModel{tasks: tasks, currentIdx: idx, running: true, isAdmin: true}
}

func isQuitCmd(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}

// Stopping DISM in the middle of servicing Windows takes a second keypress.
func TestComponentStoreQuitNeedsConfirmation(t *testing.T) {
	m := runningComponentStoreModel()

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	m = updated.(optimizeModel)
	if isQuitCmd(cmd) {
		t.Fatal("the first q stopped a running component store cleanup")
	}
	if !m.quitConfirm {
		t.Fatal("the first q did not ask for confirmation")
	}
	if view := m.View(); !strings.Contains(view, "Press q again") {
		t.Errorf("the screen does not ask for confirmation:\n%s", view)
	}

	updated, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	m = updated.(optimizeModel)
	if !isQuitCmd(cmd) {
		t.Error("the second q did not quit")
	}
	if !m.abortedCleanup {
		t.Error("quitting mid-cleanup was not recorded, so the user is never told")
	}
}

// Any other key means "keep going".
func TestComponentStoreQuitConfirmationCanBeCancelled(t *testing.T) {
	m := runningComponentStoreModel()
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	m = updated.(optimizeModel)

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	m = updated.(optimizeModel)
	if m.quitConfirm {
		t.Error("another key did not cancel the quit prompt")
	}

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	m = updated.(optimizeModel)
	if isQuitCmd(cmd) || !m.quitConfirm {
		t.Error("after cancelling, q quit without asking again")
	}
}

// The short tasks keep quitting on the first keypress.
func TestQuitIsImmediateForShortTasks(t *testing.T) {
	tasks := optimizeTasks(false)
	tasks[0].Status = statusRunning
	m := optimizeModel{tasks: tasks, currentIdx: 0, running: true, isAdmin: true}

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	m = updated.(optimizeModel)
	if !isQuitCmd(cmd) || m.quitConfirm || m.abortedCleanup {
		t.Error("quitting during a short task asked for confirmation")
	}
}
