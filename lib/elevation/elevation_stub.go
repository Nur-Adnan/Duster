//go:build !windows

package elevation

import "errors"

// IsAdmin reports Windows administrator rights; always false off Windows.
func IsAdmin() bool {
	return false
}

// RequestElevation relaunches the process elevated on Windows; fails gracefully elsewhere.
func RequestElevation() error {
	return errors.New("elevation is only supported on Windows")
}
