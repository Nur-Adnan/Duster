package cmd

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
)

// AppVersion is the current application version, set from main.go at startup.
// This allows subcommands like `update` to reference the compiled version
// instead of hardcoding version strings.
var AppVersion = "0.0.0"

// calculateDirSize recursively computes the physical size of a directory.
// Non-negotiable safety guard: explicitly skips junctions and symlinks to prevent boundary leaks and infinite traversal loops.
func calculateDirSize(dirPath string) int64 {
	var size int64
	_ = filepath.WalkDir(dirPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // Skip inaccessible directories and items gracefully
		}

		// Avoid junction points / symlink traversals. Since Go 1.23 Windows
		// junctions report as ModeIrregular rather than ModeSymlink, so both
		// bits must be checked.
		if d.Type()&(os.ModeSymlink|os.ModeIrregular) != 0 {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if !d.IsDir() {
			info, err := d.Info()
			if err == nil {
				size += info.Size()
			}
		}
		return nil
	})
	return size
}

// removeAllSafe handles the classic Windows read-only file lock issues.
// Iterates and strips the read-only file attribute recursively before wiping directories to prevent silent deletion failures.
func removeAllSafe(path string) error {
	// Lstat BEFORE any chmod: os.Chmod follows symlinks, so chmod'ing a reparse
	// point would clear the read-only attribute on its target — a file outside the
	// deletion root. Reparse points (symlinks, and junctions which report as
	// ModeIrregular on Go 1.23+) are deleted as the link itself, never followed.
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // Target already gone
		}
		return err
	}

	if info.Mode()&(os.ModeSymlink|os.ModeIrregular) != 0 {
		return os.Remove(path)
	}

	_ = os.Chmod(path, 0777)

	if info.IsDir() {
		// Securely strip read-only attributes from nested items without traversing
		// or chmod'ing through junctions/symlinks.
		_ = filepath.WalkDir(path, func(p string, d os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return nil
			}
			if d.Type()&(os.ModeSymlink|os.ModeIrregular) != 0 {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil // do not chmod through a link
			}
			_ = os.Chmod(p, 0777)
			return nil
		})
	}

	return os.RemoveAll(path)
}

// removeFileSafe strips read-only attributes before deleting a single file target.
func removeFileSafe(path string) error {
	// Skip chmod on links (it would follow to the target); Remove deletes the link.
	if info, err := os.Lstat(path); err == nil && info.Mode()&(os.ModeSymlink|os.ModeIrregular) != 0 {
		return os.Remove(path)
	}
	_ = os.Chmod(path, 0777)
	return os.Remove(path)
}

// sha256Hex returns the lowercase hex-encoded SHA-256 digest of data.
func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
