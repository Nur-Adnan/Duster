//go:build !windows

package cmd

import "syscall"

// getDiskFreeBytesOS returns the number of free bytes on the volume containing path.
func getDiskFreeBytesOS(path string) int64 {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0
	}
	return int64(stat.Bavail) * int64(stat.Bsize)
}
