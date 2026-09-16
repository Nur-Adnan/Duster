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

// normalizeWinPath lowercases p and resolves it the way the Win32 path parser
// will, on every host OS, so the protected-path checks compare the path
// Windows actually opens rather than its spelling:
//   - "/" becomes "\"; \\?\, \\.\ and \??\ prefixes are removed (\\?\UNC\ -> \\)
//   - admin shares (\\host\c$\x) become their drive form (c:\x)
//   - trailing dots and spaces are trimmed from every component ("windows." -> "windows")
//   - "." and ".." are resolved lexically
//
// ok is false for forms that must never be a deletion target: device/NT object
// paths (GLOBALROOT, Volume{GUID}, PhysicalDrive0), alternate data streams or
// any other ':' past the drive, and components that vanish when trimmed.
func normalizeWinPath(p string) (norm string, ok bool) {
	p = strings.ToLower(strings.ReplaceAll(p, "/", `\`))
	device := false
	for _, pfx := range []string{`\\?\`, `\\.\`, `\??\`} {
		if strings.HasPrefix(p, pfx) {
			p, device = p[len(pfx):], true
			if strings.HasPrefix(p, `unc\`) {
				p = `\\` + p[len(`unc\`):]
			}
			break
		}
	}

	var prefix, rest string
	switch {
	case strings.HasPrefix(p, `\\`):
		parts := strings.SplitN(p[2:], `\`, 3)
		if len(parts) < 2 || parts[1] == "" {
			return p, true // \\server or \\server\ : a share root, protected by isUNCShareRoot
		}
		host, share := parts[0], parts[1]
		if len(share) == 2 && share[1] == '$' && isDriveLetter(share[0]) {
			prefix = share[:1] + ":" // \\host\c$ is drive c:
		} else if strings.HasSuffix(share, "$") {
			return "", false // admin$ (Windows dir), print$ (spool drivers), other hidden admin shares
		} else {
			prefix = `\\` + host + `\` + share
		}
		if len(parts) == 3 {
			rest = parts[2]
		}
	case len(p) >= 2 && p[1] == ':' && isDriveLetter(p[0]):
		prefix, rest = p[:2], p[2:]
		if rest != "" && rest[0] != '\\' {
			return p, true // drive-relative (c:foo): protected by isDriveRootOrRelative
		}
	default:
		if device {
			return "", false // \\.\PhysicalDrive0, \\?\GLOBALROOT\..., \\?\Volume{...}\...
		}
		rest = p // rooted (\x), relative, or a POSIX path on a test host
	}

	rooted := prefix != "" || strings.HasPrefix(rest, `\`)
	var out []string
	for _, s := range strings.Split(rest, `\`) {
		switch s {
		case "", ".":
			continue
		case "..":
			if len(out) > 0 {
				out = out[:len(out)-1]
			}
			continue
		}
		t := strings.TrimRight(s, ". ")
		if t == "" || strings.Contains(t, ":") {
			return "", false
		}
		out = append(out, t)
	}
	norm = prefix + strings.Join(out, `\`)
	if rooted {
		norm = prefix + `\` + strings.Join(out, `\`)
	}
	return norm, true
}

// normalizedOrRaw is normalizeWinPath for trusted system locations, where an
// unparseable value should still be compared rather than discarded.
func normalizedOrRaw(p string) string {
	if n, ok := normalizeWinPath(p); ok {
		return n
	}
	return strings.ToLower(p)
}

// IsSystemProtectedPath checks if a path falls under protected Windows directories.
// Non-negotiable safety rules: C:\Windows\System32, C:\Program Files, and roots of drives are protected.
// Any path it cannot confidently normalize is treated as protected (fail closed).
func IsSystemProtectedPath(path string) bool {
	resolved, ok := normalizeWinPath(ResolveEnvPath(path))
	if !ok {
		return true
	}

	// Canonicalize 8.3 short names (e.g. PROGRA~1 -> program files) so they
	// cannot alias past the long-form protected stems. No-op off Windows and
	// for paths that don't exist on disk.
	if long := getLongPathName(resolved); long != "" {
		if resolved, ok = normalizeWinPath(long); !ok {
			return true
		}
	}

	// Bare drive roots, dotted/relative drives, and UNC share roots are
	// always protected, regardless of the specific stem rules below.
	if isDriveRootOrRelative(resolved) || isUNCShareRoot(resolved) {
		return true
	}

	// Get system directory locations (normalized to lowercase)
	winDir := normalizedOrRaw(ResolveEnvPath("%WINDIR%"))
	// Just the drive ("c:"): %SYSTEMDRIVE% normalizes to "c:\" on POSIX but
	// "c:." on Windows (filepath.Clean("C:")), so take the first two bytes.
	sysDrive := "c:"
	if d := normalizedOrRaw(ResolveEnvPath("%SYSTEMDRIVE%")); len(d) >= 2 && d[1] == ':' {
		sysDrive = d[:2]
	}

	// Native Windows API overrides to completely defeat environment spoofing
	if secureWin, err := GetSecureWindowsDirectory(); err == nil && secureWin != "" {
		winDir = normalizedOrRaw(secureWin)
		// Derive the system drive from the kernel-provided Windows dir so a
		// spoofed %SYSTEMDRIVE% cannot unprotect Program Files / Boot / EFI.
		if len(winDir) >= 2 && winDir[1] == ':' {
			sysDrive = winDir[:2]
		}
	}

	system32 := winDir + "\\system32"
	if secureSys, err := GetSecureSystemDirectory(); err == nil && secureSys != "" {
		system32 = normalizedOrRaw(secureSys)
	}

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

	// 3. Never delete Program Files or Program Files (x86), on any drive
	if len(resolved) >= 3 && resolved[1] == ':' {
		for _, pf := range []string{`\program files`, `\program files (x86)`} {
			if stem := resolved[2:]; stem == pf || strings.HasPrefix(stem, pf+`\`) {
				return true
			}
		}
	}

	// 4. Never delete a previous Windows installation, on any drive. It holds
	// the 10-day rollback, and only Windows' own cleanup may remove it.
	if len(resolved) >= 3 && resolved[1] == ':' {
		if stem := resolved[2:]; stem == `\windows.old` || strings.HasPrefix(stem, `\windows.old\`) {
			return true
		}
	}

	// 5. Never delete boot, recovery, or volume-metadata structures
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
// LongPath expands 8.3 short components (C:\Users\RUNNER~1) to their long
// form, or returns p unchanged. Expand a walk's root once: paths built from a
// long root carry no "~", so every per-entry IsValidPath takes its fast path
// instead of a GetLongPathNameW disk lookup per file.
func LongPath(p string) string {
	if long := getLongPathName(p); long != "" {
		return long
	}
	return p
}

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
