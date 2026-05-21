//go:build !windows

package cmd

import (
	"os"
)

// recyclePathNative securely deletes a file or directory to the Windows Recycle Bin natively.
func recyclePathNative(path string) error {
	// Fallback for non-Windows platforms: delete immediately
	return os.RemoveAll(path)
}

// queryRecycleBinNative returns total size and number of items in Recycle Bin.
func queryRecycleBinNative() (int64, int64, error) {
	// Stub return zero
	return 0, 0, nil
}

// emptyRecycleBinNative empties the Recycle Bin.
func emptyRecycleBinNative() error {
	// Stub
	return nil
}
