//go:build windows

package winapi

import (
	"syscall"
)

var (
	Shell32  = syscall.NewLazyDLL("shell32.dll")
	Kernel32 = syscall.NewLazyDLL("kernel32.dll")

	// shell32 operations
	ProcSHEmptyRecycleBin = Shell32.NewProc("SHEmptyRecycleBinW")
	ProcSHQueryRecycleBin = Shell32.NewProc("SHQueryRecycleBinW")
	ProcSHFileOperation   = Shell32.NewProc("SHFileOperationW")
	ProcShellExecute      = Shell32.NewProc("ShellExecuteW")

	// kernel32 operations
	ProcGlobalMemoryStatusEx = Kernel32.NewProc("GlobalMemoryStatusEx")
	ProcGetSystemPowerStatus = Kernel32.NewProc("GetSystemPowerStatus")
	ProcGetTickCount64       = Kernel32.NewProc("GetTickCount64")
	ProcGetDiskFreeSpaceEx   = Kernel32.NewProc("GetDiskFreeSpaceExW")
	ProcGetLogicalDrives     = Kernel32.NewProc("GetLogicalDrives")
)
