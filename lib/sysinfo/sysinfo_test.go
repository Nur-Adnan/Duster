//go:build windows

package sysinfo

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestGetSystemStatsSkipsProcessWalk(t *testing.T) {
	stats, err := GetSystemStats()
	if err != nil {
		t.Fatal(err)
	}
	if stats.TopProcesses != nil {
		t.Fatal("GetSystemStats runs every UI tick and must not walk processes; that is TopProcesses' job")
	}
}

func TestTopProcessesFindsBusyProcess(t *testing.T) {
	// Keep one core busy in this process during the window, so something is.
	var stop atomic.Bool
	go func() {
		for !stop.Load() {
		}
	}()
	defer stop.Store(true)

	if top := TopProcesses(500 * time.Millisecond); len(top) == 0 {
		t.Fatal("a one-shot TopProcesses call must report busy processes")
	}
}

// Per-tick cost of the status/landing poll, and of the process walk that used
// to run inside it (old per-tick cost = both).
func BenchmarkGetSystemStats(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, _ = GetSystemStats()
	}
}

func BenchmarkProcessWalk(b *testing.B) {
	for i := 0; i < b.N; i++ {
		getTopProcesses()
	}
}

func TestGetSystemStats(t *testing.T) {
	stats, err := GetSystemStats()
	if err != nil {
		t.Fatalf("Failed to fetch system stats: %v", err)
	}

	if stats.HealthScore < 0 || stats.HealthScore > 100 {
		t.Errorf("Invalid HealthScore %d, expected range [0, 100]", stats.HealthScore)
	}

	if stats.RAMTotal == 0 {
		t.Error("Expected RAMTotal to be greater than 0")
	}

	if len(stats.Disks) == 0 {
		t.Log("Warning: No active disks found (could occur in isolated containers)")
	}
}
