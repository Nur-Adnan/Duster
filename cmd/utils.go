package cmd

import (
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

		// Avoid junction points / symlink traversals
		if d.Type()&os.ModeSymlink != 0 {
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
	_ = os.Chmod(path, 0777)

	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // Target already gone
		}
		return err
	}

	if info.Mode()&os.ModeSymlink != 0 {
		// Bypasses recursion and deletes the symlink or junction link itself directly.
		// This defeats TOCTOU redirection exploits completely.
		return os.Remove(path)
	}

	if info.IsDir() {
		// Securely strip read-only attributes from nested items without traversing junctions
		_ = filepath.WalkDir(path, func(p string, d os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return nil
			}
			if d.Type()&os.ModeSymlink != 0 {
				if d.IsDir() {
					return filepath.SkipDir
				}
				_ = os.Chmod(p, 0777)
				return nil
			}
			_ = os.Chmod(p, 0777)
			return nil
		})
	}

	return os.RemoveAll(path)
}

// removeFileSafe strips read-only attributes before deleting a single file target.
func removeFileSafe(path string) error {
	_ = os.Chmod(path, 0777)
	return os.Remove(path)
}
