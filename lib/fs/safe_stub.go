//go:build !windows

package fs

import "errors"

var errNotWindows = errors.New("only supported on Windows")

// GetSecureSystemDirectory is Windows-only; callers fall back to env-derived paths.
func GetSecureSystemDirectory() (string, error) {
	return "", errNotWindows
}

// GetSecureWindowsDirectory is Windows-only; callers fall back to env-derived paths.
func GetSecureWindowsDirectory() (string, error) {
	return "", errNotWindows
}

// IsOfflineFile reports whether a file is a cloud-storage placeholder; never true off Windows.
func IsOfflineFile(string) bool {
	return false
}

// getLongPathName is Windows-only (8.3 short-name expansion); a no-op elsewhere.
func getLongPathName(string) string {
	return ""
}
