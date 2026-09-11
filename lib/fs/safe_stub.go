//go:build !windows

package fs

import (
	"errors"
	"os"
)

// IsOfflineInfo reports whether a directory-listing FileInfo is a cloud placeholder; never true off Windows.
func IsOfflineInfo(os.FileInfo) bool { return false }

var errNotWindows = errors.New("only supported on Windows")

// GetSecureSystemDirectory is Windows-only; callers fall back to env-derived paths.
func GetSecureSystemDirectory() (string, error) {
	return "", errNotWindows
}

// GetSecureWindowsDirectory is Windows-only; callers fall back to env-derived paths.
func GetSecureWindowsDirectory() (string, error) {
	return "", errNotWindows
}

// getLongPathName is Windows-only (8.3 short-name expansion); a no-op elsewhere.
func getLongPathName(string) string {
	return ""
}
