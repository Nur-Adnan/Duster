//go:build windows

package fs

import (
	"syscall"
	"unicode/utf16"
	"unsafe"
)

var (
	kernel32                = syscall.NewLazyDLL("kernel32.dll")
	procGetSystemDirectory  = kernel32.NewProc("GetSystemDirectoryW")
	procGetWindowsDirectory = kernel32.NewProc("GetWindowsDirectoryW")
)

const FILE_ATTRIBUTE_OFFLINE = 0x1000

// GetSecureSystemDirectory queries the kernel32 DLL directly to get the true, immutable system32 path.
// This completely circumvents %WINDIR% or %SYSTEMROOT% environment manipulation attempts.
func GetSecureSystemDirectory() (string, error) {
	buf := make([]uint16, 260)
	ret, _, err := procGetSystemDirectory.Call(
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(len(buf)),
	)
	if ret == 0 {
		return "", err
	}
	return string(utf16.Decode(buf[:ret])), nil
}

// GetSecureWindowsDirectory queries the kernel32 DLL directly to get the true, immutable Windows path.
// This prevents spoofing of system directories.
func GetSecureWindowsDirectory() (string, error) {
	buf := make([]uint16, 260)
	ret, _, err := procGetWindowsDirectory.Call(
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(len(buf)),
	)
	if ret == 0 {
		return "", err
	}
	return string(utf16.Decode(buf[:ret])), nil
}

// IsOfflineFile checks if a file is stored offline (OneDrive cloud storage placeholder)
// to prevent WalkDir from triggering automatic high-latency hydration/download.
func IsOfflineFile(path string) bool {
	pointer, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return false
	}
	attrs, err := syscall.GetFileAttributes(pointer)
	if err != nil {
		return false
	}
	return attrs&FILE_ATTRIBUTE_OFFLINE != 0
}
