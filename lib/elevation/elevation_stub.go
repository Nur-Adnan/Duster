//go:build !windows

package elevation

import "fmt"

// IsAdmin fallback for non-Windows systems. Returns true to allow testing dry-runs smoothly.
func IsAdmin() bool {
	return true
}

// RequestElevation fallback for non-Windows systems.
func RequestElevation() error {
	return fmt.Errorf("elevation is only supported on Windows")
}
