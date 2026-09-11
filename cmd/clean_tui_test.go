package cmd

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// dryRunCleanModel is a clean TUI ready to run in simulation, so the
// deletion commands return at once and touch nothing on disk.
func dryRunCleanModel() cleanModel {
	return cleanModel{
		state:        cleanStateReady,
		dryRun:       true,
		launchDryRun: true,
		items: []*cleanTuiItem{
			{ID: "temp", Name: "Temp", Checked: true, Status: "ok", Size: 10, FileCount: 1},
			{ID: "dns", Name: "DNS", Checked: true, Status: "ok", FileCount: 1},
		},
	}
}

// Each item must start deleting when it becomes active, not after a progress
// animation, and finishing one must start the next directly.
func TestCleanStartsDeletingImmediately(t *testing.T) {
	m := dryRunCleanModel()
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(cleanModel)
	if m.state != cleanStateCleaning || m.items[0].Status != "deleting" {
		t.Fatalf("after Enter: state %v, first item %q; want cleaning and deleting", m.state, m.items[0].Status)
	}
	batch, ok := cmd().(tea.BatchMsg)
	if !ok {
		t.Fatal("Enter must start the animation and the deletion together")
	}
	var first *cleanDeletionProgressMsg
	for _, c := range batch {
		if msg, ok := c().(cleanDeletionProgressMsg); ok {
			first = &msg
		}
	}
	if first == nil || first.ItemIdx != 0 {
		t.Fatal("Enter did not start deleting the first item immediately")
	}

	updated, cmd = m.Update(*first)
	m = updated.(cleanModel)
	if m.items[0].Status != "done" || m.items[1].Status != "deleting" {
		t.Fatalf("after item 0 reported: statuses %q, %q; want done, deleting", m.items[0].Status, m.items[1].Status)
	}
	// Only the deletion: the tick chain is still running, and a second one
	// would double the animation speed.
	second, ok := cmd().(cleanDeletionProgressMsg)
	if !ok || second.ItemIdx != 1 {
		t.Fatal("finishing item 0 must start deleting item 1 directly")
	}

	updated, cmd = m.Update(second)
	m = updated.(cleanModel)
	if m.state != cleanStateDone || cmd != nil {
		t.Fatalf("after the last item: state %v, cmd %v; want done and no further work", m.state, cmd)
	}
}

// A finished dry run must not tell the user that files were removed.
func TestCleanDryRunDoneSaysNothingDeleted(t *testing.T) {
	m := dryRunCleanModel()
	m.state = cleanStateDone
	v := m.View()
	if !strings.Contains(v, "nothing was deleted") {
		t.Error("a finished dry run does not say that nothing was deleted")
	}
	for _, claim := range []string{"cleaned successfully", "files removed", "space recovered"} {
		if strings.Contains(v, claim) {
			t.Errorf("a finished dry run claims %q", claim)
		}
	}
}

func TestCleanTickOnlyAnimatesTheActiveDeletion(t *testing.T) {
	m := dryRunCleanModel()
	m.state = cleanStateCleaning
	m.activeItemIdx = 0
	m.items[0].Status = "deleting"
	m.items[0].Progress = 90

	updated, cmd := m.Update(animateTickMsg(time.Now()))
	m = updated.(cleanModel)
	if m.items[0].Progress != 95 || m.items[0].Status != "deleting" {
		t.Fatalf("tick moved item 0 to %.0f%% (%s); want capped at 95%% and still deleting", m.items[0].Progress, m.items[0].Status)
	}
	if cmd == nil {
		t.Fatal("the tick chain must stay alive while cleaning")
	}
}
