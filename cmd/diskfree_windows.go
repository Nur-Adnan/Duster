//go:build windows

package cmd

import (
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
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

// diskFreePercent returns the free share (0-100) of the volume holding path,
// as this user sees it, or -1 when it cannot be read.
func diskFreePercent(path string) float64 {
	pathPtr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return -1
	}
	var freeAvail, totalBytes, totalFree uint64
	ret, _, _ := _procGetDiskFreeSpace.Call(
		uintptr(unsafe.Pointer(pathPtr)),
		uintptr(unsafe.Pointer(&freeAvail)),
		uintptr(unsafe.Pointer(&totalBytes)),
		uintptr(unsafe.Pointer(&totalFree)),
	)
	if ret == 0 || totalBytes == 0 {
		return -1
	}
	return float64(freeAvail) * 100 / float64(totalBytes)
}

// volumeSerial returns the serial number of the volume holding path, or 0 when
// it cannot be read. It tells a USB stick from the one that had the same drive
// letter last week.
func volumeSerial(path string) uint32 {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0
	}
	root := make([]uint16, windows.MAX_PATH+1)
	if err := windows.GetVolumePathName(p, &root[0], uint32(len(root))); err != nil {
		return 0
	}
	var serial uint32
	if err := windows.GetVolumeInformation(&root[0], nil, 0, &serial, nil, nil, nil, 0); err != nil {
		return 0
	}
	return serial
}
