//go:build windows

package elevation

import (
	"os"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	shell32           = syscall.NewLazyDLL("shell32.dll")
	procShellExecuteW = shell32.NewProc("ShellExecuteW")
)

// IsAdmin checks if the current process is running with administrative privileges (UAC elevated).
func IsAdmin() bool {
	var sid *windows.SID
	err := windows.AllocateAndInitializeSid(
		&windows.SECURITY_NT_AUTHORITY,
		2,
		windows.SECURITY_BUILTIN_DOMAIN_RID,
		windows.DOMAIN_ALIAS_RID_ADMINS,
		0, 0, 0, 0, 0, 0,
		&sid,
	)
	if err != nil {
		return false
	}
	defer windows.FreeSid(sid)

	token := windows.Token(0)
	member, err := token.IsMember(sid)
	if err != nil {
		return false
	}
	return member
}

// RequestElevation relaunch the self binary with elevated administrator privileges.
func RequestElevation() error {
	exePath, err := os.Executable()
	if err != nil {
		return err
	}

	verbPtr, _ := windows.UTF16PtrFromString("runas")
	filePtr, _ := windows.UTF16PtrFromString(exePath)

	// Re-run with the same arguments
	args := os.Args[1:]
	var argsStr string
	if len(args) > 0 {
		var escapedArgs []string
		for _, arg := range args {
			if strings.ContainsAny(arg, " \t\"") {
				escaped := strings.ReplaceAll(arg, "\"", "\\\"")
				escapedArgs = append(escapedArgs, "\""+escaped+"\"")
			} else {
				escapedArgs = append(escapedArgs, arg)
			}
		}
		argsStr = strings.Join(escapedArgs, " ")
	}
	argsPtr, _ := windows.UTF16PtrFromString(argsStr)

	// ShellExecuteW parameters: hwnd, lpOperation, lpFile, lpParameters, lpDirectory, nShowCmd
	ret, _, err := procShellExecuteW.Call(
		0,
		uintptr(unsafe.Pointer(verbPtr)),
		uintptr(unsafe.Pointer(filePtr)),
		uintptr(unsafe.Pointer(argsPtr)),
		0,
		5, // SW_SHOW
	)

	// ShellExecuteW returns > 32 on success
	if ret <= 32 {
		return err
	}
	return nil
}
