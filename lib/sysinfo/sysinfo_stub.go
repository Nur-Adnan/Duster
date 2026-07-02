//go:build !windows

package sysinfo

import "errors"

// GetSystemStats gathers Windows system metrics; unsupported elsewhere.
func GetSystemStats() (SystemStats, error) {
	return SystemStats{}, errors.New("system stats are only supported on Windows")
}
