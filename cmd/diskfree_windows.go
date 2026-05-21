//go:build windows

package cmd

import (
	"syscall"
	"unsafe"
)

var (
	_kernel32             = syscall.NewLazyDLL("kernel32.dll")
	_procGetDiskFreeSpace = _kernel32.NewProc("GetDiskFreeSpaceExW")
)

// getDiskFreeBytesOS returns the number of free bytes on the volume containing path.
func getDiskFreeBytesOS(path string) int64 {
	pathPtr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return 0
	}
	var freeAvail, totalBytes, totalFree uint64
	ret, _, _ := _procGetDiskFreeSpace.Call(
		uintptr(unsafe.Pointer(pathPtr)),
		uintptr(unsafe.Pointer(&freeAvail)),
		uintptr(unsafe.Pointer(&totalBytes)),
		uintptr(unsafe.Pointer(&totalFree)),
	)
	if ret != 0 {
		return int64(freeAvail)
	}
	return 0
}
