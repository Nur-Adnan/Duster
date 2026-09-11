//go:build windows

package fs

import (
	"os"
	"strings"
	"sync"
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
	// A fixed buffer silently skipped expansion for long paths; when it is too
	// small the API returns the required size (incl. NUL), so retry once.
	n := 320
	for attempt := 0; attempt < 2; attempt++ {
		buf := make([]uint16, n)
		ret, _, _ := procGetLongPathName.Call(
			uintptr(unsafe.Pointer(p)),
			uintptr(unsafe.Pointer(&buf[0])),
			uintptr(len(buf)),
		)
		if ret == 0 {
			return ""
		}
		if int(ret) < len(buf) {
			return syscall.UTF16ToString(buf[:ret])
		}
		n = int(ret)
	}
	return ""
}

const (
	FILE_ATTRIBUTE_OFFLINE               = 0x1000
	FILE_ATTRIBUTE_RECALL_ON_OPEN        = 0x40000
	FILE_ATTRIBUTE_RECALL_ON_DATA_ACCESS = 0x400000

	// Modern cloud sync engines mark placeholders with the RECALL_* attributes,
	// not the deprecated OFFLINE bit — all three must be checked or the guard is dead.
	placeholderMask = FILE_ATTRIBUTE_OFFLINE | FILE_ATTRIBUTE_RECALL_ON_OPEN | FILE_ATTRIBUTE_RECALL_ON_DATA_ACCESS
)

// The system directories can't change while Duster runs, and IsValidPath asks
// for them on every folder purge and clean visit, so the kernel is asked once.
var (
	secureSystemDir  = sync.OnceValues(querySystemDirectory)
	secureWindowsDir = sync.OnceValues(queryWindowsDirectory)
)

// GetSecureSystemDirectory returns the true, immutable system32 path from the
// kernel, circumventing %WINDIR% / %SYSTEMROOT% manipulation.
func GetSecureSystemDirectory() (string, error) { return secureSystemDir() }

// GetSecureWindowsDirectory returns the true, immutable Windows path from the kernel.
func GetSecureWindowsDirectory() (string, error) { return secureWindowsDir() }

func querySystemDirectory() (string, error) {
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

func queryWindowsDirectory() (string, error) {
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

// IsOfflineInfo reports whether a file from a directory listing is a cloud
// placeholder (OneDrive etc.), which reading would force to download. It uses
// the attributes the listing already carries, so it costs no system call and
// never opens the file. It is also the complete check: Windows reports
// FILE_ATTRIBUTE_RECALL_ON_OPEN only in directory enumeration, never from
// GetFileAttributes.
func IsOfflineInfo(info os.FileInfo) bool {
	d, ok := info.Sys().(*syscall.Win32FileAttributeData)
	return ok && d.FileAttributes&placeholderMask != 0
}
