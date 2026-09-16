//go:build windows

package cmd

import (
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows/registry"
)

var (
	_procGetCompressedFileSize = _kernel32.NewProc("GetCompressedFileSizeW")
	_procGetFileAttributes     = _kernel32.NewProc("GetFileAttributesW")
	_procGetShortPathName      = _kernel32.NewProc("GetShortPathNameW")
)

const (
	_fileAttributeCompressed = 0x00000800
	_fileAttributeEncrypted  = 0x00004000
	_fileAttributeSparseFile = 0x00000200
	_invalidFileAttributes   = 0xFFFFFFFF
	_invalidFileSize         = 0xFFFFFFFF
)

// wslLxssKey holds one subkey per registered distribution. It is the only
// documented way to find a distribution's disk: Microsoft's own guidance for
// locating ext4.vhdx reads BasePath from here.
const wslLxssKey = `Software\Microsoft\Windows\CurrentVersion\Lxss`

// wslDistroEntries lists the registered WSL distributions. A distribution with
// no readable name or base path is dropped rather than guessed at: the path is
// about to be handed to diskpart.
func wslDistroEntries() []wslDistro {
	root, err := registry.OpenKey(registry.CURRENT_USER, wslLxssKey, registry.ENUMERATE_SUB_KEYS)
	if err != nil {
		return nil
	}
	defer root.Close()

	names, err := root.ReadSubKeyNames(-1)
	if err != nil {
		return nil
	}

	var distros []wslDistro
	for _, name := range names {
		k, err := registry.OpenKey(root, name, registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		distro, _, nameErr := k.GetStringValue("DistributionName")
		base, _, baseErr := k.GetStringValue("BasePath")
		version, _, verErr := k.GetIntegerValue("Version")
		k.Close()

		if nameErr != nil || baseErr != nil || distro == "" || base == "" {
			continue
		}
		if verErr != nil {
			// Absent on very old entries; WSL 1 distributions have no VHD,
			// and stripVDiskPrefix leaves them to the .vhdx glob to reject.
			version = 0
		}
		distros = append(distros, wslDistro{
			Name:     distro,
			BasePath: stripVDiskPrefix(base),
			Version:  int(version),
		})
	}
	return distros
}

// stripVDiskPrefix removes the extended-length prefix WSL stores in BasePath.
// diskpart's "select vdisk file=" does not accept a \\?\ path.
func stripVDiskPrefix(path string) string {
	return strings.TrimPrefix(path, `\\?\`)
}

// fileDiskUsage reports how much room a file actually takes on the volume plus
// the attributes that stop diskpart from compacting it. A sparse, compressed or
// encrypted VHDX is rejected outright by "compact vdisk", so the scan reports
// that up front instead of after a multi-minute wait.
func fileDiskUsage(path string) (allocated int64, sparse, compressed, encrypted bool) {
	ptr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return 0, false, false, false
	}

	var high uint32
	low, _, _ := _procGetCompressedFileSize.Call(
		uintptr(unsafe.Pointer(ptr)),
		uintptr(unsafe.Pointer(&high)),
	)
	if uint32(low) != _invalidFileSize || high != 0 {
		allocated = int64(uint64(high)<<32 | uint64(uint32(low)))
	}

	attrs, _, _ := _procGetFileAttributes.Call(uintptr(unsafe.Pointer(ptr)))
	if uint32(attrs) != _invalidFileAttributes {
		sparse = attrs&_fileAttributeSparseFile != 0
		compressed = attrs&_fileAttributeCompressed != 0
		encrypted = attrs&_fileAttributeEncrypted != 0
	}
	return allocated, sparse, compressed, encrypted
}

// shortPathName returns the 8.3 form of path, or "" when the volume has short
// names disabled. It is the escape hatch for a disk under a profile whose name
// is not ASCII: a diskpart script file is read in the system ANSI code page, so
// a UTF-8 path with non-ASCII bytes would not name the same file.
func shortPathName(path string) string {
	ptr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return ""
	}
	buf := make([]uint16, syscall.MAX_PATH)
	n, _, _ := _procGetShortPathName.Call(
		uintptr(unsafe.Pointer(ptr)),
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(len(buf)),
	)
	if n == 0 || int(n) >= len(buf) {
		return ""
	}
	return syscall.UTF16ToString(buf[:n])
}
