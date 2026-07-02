package fs

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var envRegex = regexp.MustCompile(`%([^%]+)%`)

// ResolveEnvPath expands Windows-style environment variables like %TEMP% or %USERPROFILE%.
func ResolveEnvPath(path string) string {
	result := envRegex.ReplaceAllStringFunc(path, func(m string) string {
		varName := strings.Trim(m, "%")
		val := os.Getenv(varName)
		if val == "" {
			// Fallback standard paths if env vars aren't populated (e.g. during cross-platform testing)
			switch strings.ToUpper(varName) {
			case "SYSTEMDRIVE":
				return "C:"
			case "WINDIR", "SYSTEMROOT":
				return `C:\Windows`
			case "TEMP":
				return `C:\Windows\Temp`
			case "USERPROFILE":
				return `C:\Users\Default`
			case "LOCALAPPDATA":
				return `C:\Users\Default\AppData\Local`
			case "APPDATA":
				return `C:\Users\Default\AppData\Roaming`
			case "PROGRAMDATA":
				return `C:\ProgramData`
			}
			return m
		}
		return val
	})
	return filepath.Clean(result)
}

// isDriveLetter reports whether b is an ASCII drive letter.
func isDriveLetter(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

// isDriveRootOrRelative reports whether p is a bare drive root or a
// drive-relative reference, both of which are always protected.
//
// filepath.Clean("C:") returns "C:." on Windows and "C:" on POSIX; a bare or
// dotted drive ("C:", "C:."), a drive root ("C:\", "C:/"), or a drive-relative
// path ("C:foo", with no separator after the colon) all resolve against that
// drive's *current directory* — an unpredictable, dangerous deletion target.
func isDriveRootOrRelative(p string) bool {
	if len(p) < 2 || p[1] != ':' || !isDriveLetter(p[0]) {
		return false
	}
	rest := p[2:]
	switch rest {
	case "", ".", "\\", "/":
		return true // bare drive, dotted drive, or drive root
	}
	// A separator here means a real absolute path under the drive (e.g.
	// C:\Users\...), which is judged by the specific stem rules elsewhere.
	// No separator means a drive-relative path — always unsafe.
	return rest[0] != '\\' && rest[0] != '/'
}

// isUNCShareRoot reports whether p is a bare UNC share root (\\server\share
// with no deeper path), which must be protected like a drive root.
func isUNCShareRoot(p string) bool {
	if !strings.HasPrefix(p, "\\\\") {
		return false
	}
	segs := make([]string, 0, 4)
	for _, s := range strings.Split(p[2:], "\\") {
		if s != "" {
			segs = append(segs, s)
		}
	}
	return len(segs) <= 2 // \\server or \\server\share
}

// stripExtendedPrefix removes the \\?\ and \\.\ extended-length / device
// prefixes (including the \\?\UNC\ form) so they cannot be used to slip a
// protected path past the string comparisons in IsSystemProtectedPath.
func stripExtendedPrefix(p string) string {
	for _, pfx := range []string{"\\\\?\\", "\\\\.\\"} {
		if strings.HasPrefix(p, pfx) {
			p = p[len(pfx):]
			if strings.HasPrefix(p, "unc\\") { // \\?\UNC\server\share
				p = "\\\\" + p[len("unc\\"):]
			}
			break
		}
	}
	return p
}

// IsSystemProtectedPath checks if a path falls under protected Windows directories.
// Non-negotiable safety rules: C:\Windows\System32, C:\Program Files, and roots of drives are protected.
func IsSystemProtectedPath(path string) bool {
	resolved := strings.ToLower(filepath.Clean(ResolveEnvPath(path)))

	// Normalize slashes to backslashes for Windows standard auditing
	resolved = strings.ReplaceAll(resolved, "/", "\\")

	// Strip \\?\ / \\.\ device prefixes before any comparison; otherwise
	// "\\?\C:\Windows" would defeat every stem check below.
	resolved = stripExtendedPrefix(resolved)

	// Canonicalize 8.3 short names (e.g. PROGRA~1 -> program files) so they
	// cannot alias past the long-form protected stems. No-op off Windows and
	// for paths that don't exist on disk.
	if long := getLongPathName(resolved); long != "" {
		resolved = strings.ToLower(strings.ReplaceAll(filepath.Clean(long), "/", "\\"))
	}

	// Bare drive roots, dotted/relative drives, and UNC share roots are
	// always protected, regardless of the specific stem rules below.
	if isDriveRootOrRelative(resolved) || isUNCShareRoot(resolved) {
		return true
	}

	// Get system directory locations (normalized to lowercase)
	winDir := strings.ToLower(strings.ReplaceAll(filepath.Clean(ResolveEnvPath("%WINDIR%")), "/", "\\"))
	sysDrive := strings.ToLower(strings.ReplaceAll(filepath.Clean(ResolveEnvPath("%SYSTEMDRIVE%")), "/", "\\"))

	// Native Windows API overrides to completely defeat environment spoofing
	if secureWin, err := GetSecureWindowsDirectory(); err == nil && secureWin != "" {
		winDir = strings.ToLower(strings.ReplaceAll(filepath.Clean(secureWin), "/", "\\"))
		// Derive the system drive from the kernel-provided Windows dir so a
		// spoofed %SYSTEMDRIVE% cannot unprotect Program Files / Boot / EFI.
		if len(winDir) >= 2 && winDir[1] == ':' {
			sysDrive = winDir[:2]
		}
	}

	system32 := winDir + "\\system32"
	if secureSys, err := GetSecureSystemDirectory(); err == nil && secureSys != "" {
		system32 = strings.ToLower(strings.ReplaceAll(filepath.Clean(secureSys), "/", "\\"))
	}
	programFiles := sysDrive + "\\program files"
	programFilesX86 := sysDrive + "\\program files (x86)"

	// 1. Never delete C:\Windows\System32 or anything inside it
	if resolved == system32 || strings.HasPrefix(resolved, system32+"\\") {
		return true
	}

	// 2. Never delete Windows directory itself or its critical subdirs (excluding Temp/SoftwareDistribution/Prefetch)
	if resolved == winDir {
		return true
	}
	if strings.HasPrefix(resolved, winDir+"\\") {
		// Exceptions allowed: the cache/log subtrees Duster's clean categories
		// legitimately target. Everything else under the Windows dir is blocked.
		allowedSubtrees := []string{
			winDir + "\\temp",
			winDir + "\\softwaredistribution\\download",
			winDir + "\\softwaredistribution\\deliveryoptimization",
			winDir + "\\prefetch",
			winDir + "\\minidump",
			winDir + "\\logs\\cbs",
			winDir + "\\logs\\dism",
		}

		isAllowed := false
		for _, allowed := range allowedSubtrees {
			if resolved == allowed || strings.HasPrefix(resolved, allowed+"\\") {
				isAllowed = true
				break
			}
		}

		if !isAllowed {
			return true
		}
	}

	// 3. Never delete C:\Program Files or C:\Program Files (x86) directly
	if resolved == programFiles || strings.HasPrefix(resolved, programFiles+"\\") {
		return true
	}
	if resolved == programFilesX86 || strings.HasPrefix(resolved, programFilesX86+"\\") {
		return true
	}

	// 4. Never delete boot, recovery, or volume-metadata structures
	// (documented protections in docs/security.md §4)
	for _, stem := range []string{"\\boot", "\\recovery", "\\efi", "\\system volume information", "\\$winreagent"} {
		p := sysDrive + stem
		if resolved == p || strings.HasPrefix(resolved, p+"\\") {
			return true
		}
	}

	// Root drives and drive-relative paths were already handled up front by
	// isDriveRootOrRelative / isUNCShareRoot.
	return false
}

// IsValidPath checks if the target path is absolute, not empty, and not system protected.
func IsValidPath(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	resolved := ResolveEnvPath(path)
	if !filepath.IsAbs(resolved) && !strings.Contains(resolved, ":\\") {
		return false
	}
	return !IsSystemProtectedPath(resolved)
}
