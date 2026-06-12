package security

import (
	"path/filepath"
	"strings"

	"github.com/Nur-Adnan/duster/lib/fs"
)

// IsPathSafe checks if the destination path is resolved and fully authorized for cleanups.
// Explicitly protects: C:\Windows, System32, Program Files, Boot, Recovery, EFI, System Volume Info, and root drives.
func IsPathSafe(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}

	// Canonicalize and resolve path variables
	resolved := strings.ToLower(filepath.Clean(fs.ResolveEnvPath(path)))
	resolved = strings.ReplaceAll(resolved, "/", "\\")

	// Verify standard security checks
	if fs.IsSystemProtectedPath(resolved) {
		return false
	}

	// Additional structural protections
	forbiddenPrefixes := []string{
		`c:\boot`,
		`c:\recovery`,
		`c:\efi`,
		`c:\system volume information`,
		`c:\$winreagent`,
		`c:\windows\installer`,
	}

	for _, prefix := range forbiddenPrefixes {
		if resolved == prefix || strings.HasPrefix(resolved, prefix+`\`) {
			return false
		}
	}

	return true
}

// Self-update integrity model: release archives are downloaded over HTTPS and
// verified against the SHA-256 digests published in the release's
// checksums-sha256.txt asset (see cmd/update.go). Authenticode code signing of
// the binary itself is handled at release time by scripts/sign.ps1.
