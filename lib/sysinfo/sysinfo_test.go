//go:build windows

package sysinfo

import (
	"testing"
)

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
