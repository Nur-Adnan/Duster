//go:build !windows

package fs

import (
	"errors"
)

// GetSecureSystemDirectory is a stub returning a default value for non-Windows platforms.
func GetSecureSystemDirectory() (string, error) {
	return "", errors.New("GetSecureSystemDirectory is only supported on Windows")
}

// GetSecureWindowsDirectory is a stub returning a default value for non-Windows platforms.
func GetSecureWindowsDirectory() (string, error) {
	return "", errors.New("GetSecureWindowsDirectory is only supported on Windows")
}

// IsOfflineFile is a stub returning false on non-Windows systems.
func IsOfflineFile(path string) bool {
	return false
}
