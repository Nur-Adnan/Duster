//go:build !windows

package sysinfo

import (
	"errors"
	"time"
)

// GetSystemStats gathers Windows system metrics; unsupported elsewhere.
func GetSystemStats() (SystemStats, error) {
	return SystemStats{}, errors.New("system stats are only supported on Windows")
}

// TopProcesses is Windows-only; elsewhere it reports nothing.
func TopProcesses(time.Duration) []ProcessInfo { return nil }
