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

// IsSystemProtectedPath checks if a path falls under protected Windows directories.
// Non-negotiable safety rules: C:\Windows\System32, C:\Program Files, and roots of drives are protected.
func IsSystemProtectedPath(path string) bool {
	resolved := strings.ToLower(filepath.Clean(ResolveEnvPath(path)))

	// Normalize slashes to backslashes for Windows standard auditing
	resolved = strings.ReplaceAll(resolved, "/", "\\")

	// Get system directory locations (normalized to lowercase)
	winDir := strings.ToLower(strings.ReplaceAll(filepath.Clean(ResolveEnvPath("%WINDIR%")), "/", "\\"))
	sysDrive := strings.ToLower(strings.ReplaceAll(filepath.Clean(ResolveEnvPath("%SYSTEMDRIVE%")), "/", "\\"))

	// Native Windows API overrides to completely defeat environment spoofing
	if secureWin, err := GetSecureWindowsDirectory(); err == nil && secureWin != "" {
		winDir = strings.ToLower(strings.ReplaceAll(filepath.Clean(secureWin), "/", "\\"))
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

	// 5. Never delete root drives (e.g. C:\ or D:\)
	// Volume paths are typically 3 chars long (e.g., C:\)
	if len(resolved) <= 3 && strings.HasSuffix(resolved, "\\") {
		return true
	}
	// Check for raw drive letters
	if len(resolved) == 2 && strings.HasSuffix(resolved, ":") {
		return true
	}

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
