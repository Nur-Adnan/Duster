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

// Every Windows machine has the Processor counter set, so this proves the PDH
// reader (English path, wildcard instances, item layout) even on runners that
// expose no thermal zone.
func TestSampleCounterArrayReadsProcessorInstances(t *testing.T) {
	query, counter, ok := openEnglishCounter(`\Processor(*)\% Processor Time`)
	if !ok {
		t.Fatal("could not open the Processor counter")
	}
	defer procPdhCloseQuery.Call(query)

	sampleCounterArray(query, counter) // a rate counter needs a baseline sample
	time.Sleep(200 * time.Millisecond)
	values := sampleCounterArray(query, counter)
	if len(values) < 2 {
		t.Fatalf("got %d Processor instances, want every CPU plus _Total", len(values))
	}
	for _, v := range values {
		if v < 0 || v > 100 {
			t.Errorf("Processor instance reads %v%%, want 0-100 (misread counter item?)", v)
		}
	}
}

func TestCPUTemperatureIsPlausibleOrUnavailable(t *testing.T) {
	c := cpuTemperature()
	t.Logf("cpuTemperature() = %.1f°C (0 = no thermal zone exposed)", c)
	if c < 0 || c >= 150 {
		t.Fatalf("cpuTemperature() = %v, want 0 or a plausible reading", c)
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
