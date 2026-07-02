package fs

import (
	"strings"
	"testing"
)

func TestResolveEnvPath(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
		setupEnv func(t *testing.T)
	}{
		{
			name:     "Static Windows path",
			input:    `C:\Windows\System32`,
			expected: `C:\Windows\System32`,
			setupEnv: func(t *testing.T) {},
		},
		{
			name:     "TEMP expansion",
			input:    `%TEMP%\duster-test`,
			expected: `C:\Windows\Temp\duster-test`, // Fallback value when env not set
			setupEnv: func(t *testing.T) {
				t.Setenv("TEMP", "") // empty reads as unset; auto-restored
			},
		},
		{
			name:     "Custom ENV expansion",
			input:    `%CUSTOM_PATH%\subdir`,
			expected: `D:\Projects\subdir`,
			setupEnv: func(t *testing.T) {
				t.Setenv("CUSTOM_PATH", `D:\Projects`)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupEnv(t)
			actual := ResolveEnvPath(tt.input)

			// Normalize slashes for comparison
			actualNorm := strings.ReplaceAll(actual, "/", "\\")
			expectedNorm := strings.ReplaceAll(tt.expected, "/", "\\")

			if actualNorm != expectedNorm {
				t.Errorf("ResolveEnvPath(%q) = %q, want %q", tt.input, actualNorm, expectedNorm)
			}
		})
	}
}

func TestIsSystemProtectedPath(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected bool
	}{
		{"System32 raw", `C:\Windows\System32`, true},
		{"System32 subfolder", `C:\Windows\System32\drivers`, true},
		{"Program Files raw", `C:\Program Files`, true},
		{"Program Files subfolder", `C:\Program Files\Internet Explorer`, true},
		{"Program Files x86 raw", `C:\Program Files (x86)`, true},
		{"Root drive C", `C:\`, true},
		{"Root drive D", `D:\`, true},
		{"Windows directory itself", `C:\Windows`, true},
		{"Allowable temp subfolder", `C:\Windows\Temp\test`, false},
		{"Allowable prefetch subfolder", `C:\Windows\Prefetch\cache`, false},
		{"Allowable software distribution", `C:\Windows\SoftwareDistribution\Download\package`, false},
		{"Allowable delivery optimization", `C:\Windows\SoftwareDistribution\DeliveryOptimization\Download\package`, false},
		{"Allowable delivery optimization root", `C:\Windows\SoftwareDistribution\DeliveryOptimization`, false},
		{"Allowable user temp", `C:\Users\Default\AppData\Local\Temp\cleanup`, false},
		{"Allowable Minidump directory", `C:\Windows\Minidump`, false}, // cleanable by the memdumps category
		{"Allowable CBS logs", `C:\Windows\Logs\CBS`, false},           // cleanable by the logfiles category
		{"Allowable DISM logs", `C:\Windows\Logs\DISM`, false},         // cleanable by the logfiles category
		{"Windows Logs root stays protected", `C:\Windows\Logs`, true},
		{"Windows Installer stays protected", `C:\Windows\Installer`, true},
		{"Boot directory", `C:\Boot`, true},
		{"Boot subdirectory", `C:\Boot\BCD`, true},
		{"Recovery directory", `C:\Recovery`, true},
		{"EFI directory", `C:\EFI`, true},
		{"System Volume Information", `C:\System Volume Information`, true},
		{"WinRE agent directory", `C:\$WinREAgent`, true},
		{"Non-windows user path", `C:\Users\TestUser\Desktop\junk`, false},
		{"Recovery-prefixed user folder is fine", `C:\RecoveryPhotos`, false},
		// Drive-relative and root forms: filepath.Clean("C:") is "C:." on
		// Windows and "C:" on POSIX; both must resolve as protected so a
		// drive-relative target can never reach removeAllSafe.
		{"Bare drive letter", `C:`, true},
		{"Drive-relative path", `C:foo`, true},
		{"Lowercase drive root", `d:\`, true},
		// Extended-length and device prefixes must not bypass the stem checks.
		{"Extended-length System32", `\\?\C:\Windows\System32`, true},
		{"Extended-length Windows", `\\?\C:\Windows`, true},
		{"Extended-length safe temp", `\\?\C:\Windows\Temp\x`, false},
		// UNC share roots are protected like drive roots.
		{"UNC share root", `\\server\share`, true},
		{"UNC server only", `\\server`, true},
		{"UNC deep path is fine", `\\server\share\sub\file`, false},
	}

	// Force env fallbacks (empty reads as unset); t.Setenv auto-restores.
	t.Setenv("WINDIR", "")
	t.Setenv("SYSTEMDRIVE", "")

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := IsSystemProtectedPath(tt.path)
			if actual != tt.expected {
				t.Errorf("IsSystemProtectedPath(%q) = %v, want %v", tt.path, actual, tt.expected)
			}
		})
	}
}

func TestIsValidPath(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected bool
	}{
		{"Empty path", "", false},
		{"Relative path", `.\relative\path`, false},
		{"Absolute safe path", `C:\Users\Default\AppData\Local\Temp\duster`, true},
		{"Absolute unsafe path", `C:\Windows\System32`, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := IsValidPath(tt.path)
			if actual != tt.expected {
				t.Errorf("IsValidPath(%q) = %v, want %v", tt.path, actual, tt.expected)
			}
		})
	}
}
