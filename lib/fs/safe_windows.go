//go:build windows

package fs

import (
	"strings"
	"syscall"
	"unicode/utf16"
	"unsafe"
)

var (
	kernel32                = syscall.NewLazyDLL("kernel32.dll")
	procGetSystemDirectory  = kernel32.NewProc("GetSystemDirectoryW")
	procGetWindowsDirectory = kernel32.NewProc("GetWindowsDirectoryW")
	procGetLongPathName     = kernel32.NewProc("GetLongPathNameW")
)

// getLongPathName expands 8.3 short components (e.g. PROGRA~1 -> Program Files)
// to their long form so short-name aliases cannot slip past the protected-path
// string checks. Returns "" if there is nothing to expand, the path does not
// exist, or the call fails — callers then use the path as-is.
func getLongPathName(path string) string {
	// Fast path: 8.3 aliases always contain a tilde, so skip the syscall for
	// the overwhelmingly common long-form path.
	if path == "" || !strings.Contains(path, "~") {
		return ""
	}
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return ""
	}
	buf := make([]uint16, 320)
	ret, _, _ := procGetLongPathName.Call(
		uintptr(unsafe.Pointer(p)),
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(len(buf)),
	)
	if ret == 0 || int(ret) > len(buf) {
		return ""
	}
	return syscall.UTF16ToString(buf[:ret])
}

const (
	FILE_ATTRIBUTE_OFFLINE               = 0x1000
	FILE_ATTRIBUTE_RECALL_ON_OPEN        = 0x40000
	FILE_ATTRIBUTE_RECALL_ON_DATA_ACCESS = 0x400000
)

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
// Modern cloud sync engines mark placeholders with the RECALL_* attributes, not
// the deprecated OFFLINE bit — all three must be checked or the guard is dead.
func IsOfflineFile(path string) bool {
	pointer, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return false
	}
	attrs, err := syscall.GetFileAttributes(pointer)
	if err != nil {
		return false
	}
	const placeholderMask = FILE_ATTRIBUTE_OFFLINE | FILE_ATTRIBUTE_RECALL_ON_OPEN | FILE_ATTRIBUTE_RECALL_ON_DATA_ACCESS
	return attrs&placeholderMask != 0
}
