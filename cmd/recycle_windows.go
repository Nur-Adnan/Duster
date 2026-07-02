//go:build windows

package cmd

import (
	"fmt"
	"path/filepath"
	"syscall"
	"unsafe"
)

// WinAPI declarations for secure native Recycle Bin deletion (SHFileOperationW)
const (
	foDelete          = 3
	fofAllowUndo      = 0x0040
	fofNoConfirmation = 0x0010
	fofSilent         = 0x0004
	fofNoErrorUI      = 0x0400
)

type shFileOpStructW struct {
	hwnd                  syscall.Handle
	wFunc                 uint32
	pFrom                 *uint16 // Double null-terminated path
	pTo                   *uint16
	fFlags                uint16
	fAnyOperationsAborted int32
	hNameMappings         uintptr
	lpszProgressTitle     *uint16
}

// Dynamic WinAPI loaders for Recycle Bin operation
var (
	shell32               = syscall.NewLazyDLL("shell32.dll")
	procSHFileOperation   = shell32.NewProc("SHFileOperationW")
	procSHEmptyRecycleBin = shell32.NewProc("SHEmptyRecycleBinW")
	procSHQueryRecycleBin = shell32.NewProc("SHQueryRecycleBinW")
)

// SHQUERYRBINFO represents Windows native Recycle Bin metrics structure.
type SHQUERYRBINFO struct {
	CbSize      uint32
	_           uint32 // Padding for 8-byte alignment under 64-bit platforms
	I64Size     int64
	I64NumItems int64
}

const (
	SHERB_NOCONFIRMATION = 0x00000001
	SHERB_NOPROGRESSUI   = 0x00000002
	SHERB_NOSOUND        = 0x00000004
)

// MAX_PATH in UTF-16 code units including the terminating NUL.
// SHFileOperationW is a legacy shell API: it rejects longer paths even in
// longPathAware processes (returning DE_PATHTOODEEP / DE_ERROR_MAX).
const shellMaxPath = 260

// shFileOpErrors maps SHFileOperationW's documented pre-Win32 result codes
// (the delete-relevant subset) to readable causes. These are NOT Win32
// error codes and must not be passed to FormatMessage/GetLastError.
var shFileOpErrors = map[uintptr]string{
	0x74:    "the source is a root directory",
	0x75:    "the operation was canceled",
	0x78:    "security settings denied access to the source",
	0x79:    "the path exceeds MAX_PATH",
	0x7C:    "the path was invalid",
	0x81:    "the file name exceeds MAX_PATH",
	0xB7:    "MAX_PATH was exceeded during the operation",
	0x402:   "an unknown error occurred (typically an invalid path)",
	0x10000: "an unspecified destination error occurred",
}

// recyclePathNative securely deletes a file or directory to the Windows Recycle Bin natively.
func recyclePathNative(path string) error {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return err
	}

	// SHFileOperationW expects a double-null-terminated string
	utf16Path, err := syscall.UTF16FromString(absPath)
	if err != nil {
		return err
	}
	utf16Path = append(utf16Path, 0) // Append second null-terminator

	// UTF16FromString's result already includes one NUL, so its length is
	// the code-unit count MAX_PATH constrains. Fail with a clear message
	// instead of the shell's opaque numeric code.
	if len(utf16Path)-1 > shellMaxPath {
		return fmt.Errorf("path is %d UTF-16 units; the Recycle Bin API (SHFileOperationW) supports at most %d (MAX_PATH) regardless of long-path awareness; delete permanently instead", len(utf16Path)-2, shellMaxPath-1)
	}

	if procSHFileOperation.Find() != nil {
		return fmt.Errorf("SHFileOperationW native procedure not found")
	}

	var op shFileOpStructW
	op.wFunc = foDelete
	op.pFrom = &utf16Path[0]
	op.fFlags = fofAllowUndo | fofNoConfirmation | fofSilent | fofNoErrorUI

	ret, _, _ := procSHFileOperation.Call(uintptr(unsafe.Pointer(&op)))
	if ret != 0 {
		if msg, ok := shFileOpErrors[ret]; ok {
			return fmt.Errorf("recycle bin deletion failed: %s (SHFileOperationW code %#x)", msg, ret)
		}
		return fmt.Errorf("SHFileOperationW native call failed with code %#x", ret)
	}
	if op.fAnyOperationsAborted != 0 {
		return fmt.Errorf("recycle bin deletion transaction was aborted")
	}

	return nil
}

// queryRecycleBinNative returns total size and number of items in Recycle Bin.
func queryRecycleBinNative() (int64, int64, error) {
	if procSHQueryRecycleBin.Find() != nil {
		return 0, 0, fmt.Errorf("SHQueryRecycleBinW native procedure not found")
	}

	var queryInfo SHQUERYRBINFO
	queryInfo.CbSize = uint32(unsafe.Sizeof(queryInfo))

	// NULL root path queries across all system volumes
	ret, _, _ := procSHQueryRecycleBin.Call(
		0,
		uintptr(unsafe.Pointer(&queryInfo)),
	)
	if ret != 0 {
		return 0, 0, fmt.Errorf("SHQueryRecycleBinW native call failed with error code %d", ret)
	}

	return queryInfo.I64Size, queryInfo.I64NumItems, nil
}

// emptyRecycleBinNative empties the Recycle Bin.
func emptyRecycleBinNative() error {
	if procSHEmptyRecycleBin.Find() != nil {
		return fmt.Errorf("SHEmptyRecycleBinW native procedure not found")
	}

	ret, _, _ := procSHEmptyRecycleBin.Call(
		0,
		0,
		SHERB_NOCONFIRMATION|SHERB_NOPROGRESSUI|SHERB_NOSOUND,
	)
	// S_OK is 0. E_UNEXPECTED (0x8000FFFF) is SHEmptyRecycleBinW's documented
	// quirk for an already-empty bin — success, not an error.
	const eUnexpected = 0x8000FFFF
	if ret != 0 && ret != eUnexpected {
		return fmt.Errorf("SHEmptyRecycleBinW native call failed with error code %d", ret)
	}
	return nil
}
