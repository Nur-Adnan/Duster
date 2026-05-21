package cmd

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestBenchmarkModelInitialization(t *testing.T) {
	m := initialBenchmarkModel()
	if !m.running {
		t.Error("Expected benchmark model to start in a running state")
	}
}

func TestBenchmarkModelUpdate(t *testing.T) {
	m := initialBenchmarkModel()

	// 1. Simulate benchmark complete event
	mockMetrics := BenchmarkMetrics{
		ScanFilesPerSec: 15000.5,
		ScanFilesCount:  5000,
		WriteOpsPerSec:  850.4,
		DeleteOpsPerSec: 1200.2,
		HeapAllocBytes:  12 * 1024 * 1024,
		GoroutineCount:  8,
		JsonSpeedPerSec: 6500.0,
	}

	completeMsg := benchmarkCompleteMsg(mockMetrics)
	updated, _ := m.Update(completeMsg)
	benchM := updated.(benchmarkModel)

	if benchM.running {
		t.Error("Expected running state to be false after completion message")
	}

	if benchM.metrics.ScanFilesCount != 5000 {
		t.Errorf("Expected ScanFilesCount of 5000, got %d", benchM.metrics.ScanFilesCount)
	}

	// 2. Test view output works without panicking
	view := benchM.View()
	if len(view) == 0 {
		t.Error("Expected View to return styled string, got empty string")
	}

	// 3. Test quit keyboard message
	quitMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")}
	_, quitCmd := benchM.Update(quitMsg)
	if quitCmd == nil {
		t.Error("Expected quit Cmd to be returned for q keypress")
	}
}
