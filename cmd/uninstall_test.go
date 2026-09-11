package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Nur-Adnan/duster/lib/elevation"
	"github.com/Nur-Adnan/duster/lib/uninstall"
)

func TestUninstallRefusesPerUserAppWhenElevated(t *testing.T) {
	if !elevation.IsAdmin() {
		t.Skip("needs an elevated test process (the Windows CI runner is)")
	}
	app := uninstall.InstalledApp{Name: "x", RegistryHive: "HKCU", UninstallString: `C:\duster-test-missing\uninstall.exe`}
	msg, ok := runNativeUninstallCmd(app)().(nativeUninstallDoneMsg)
	if !ok || msg.err == nil || !strings.Contains(msg.err.Error(), "per-user") {
		t.Fatalf("HKCU uninstall while elevated = %+v, want the per-user refusal", msg)
	}
}

func TestSplitUninstallString(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		expectedCmd  string
		expectedTail string
	}{
		{
			name: "Empty command",
		},
		{
			name:        "Simple command without quotes or args",
			input:       `C:\Windows\System32\uninstall.exe`,
			expectedCmd: `C:\Windows\System32\uninstall.exe`,
		},
		{
			name:         "Command with unquoted args",
			input:        `C:\Windows\System32\uninstall.exe /S /clean`,
			expectedCmd:  `C:\Windows\System32\uninstall.exe`,
			expectedTail: "/S /clean",
		},
		{
			name:         "Quoted command executable with unquoted args",
			input:        `"C:\Program Files\App\uninstall.exe" --silent --force`,
			expectedCmd:  `C:\Program Files\App\uninstall.exe`,
			expectedTail: "--silent --force",
		},
		{
			// The tail is passed on verbatim: re-quoting it breaks uninstallers
			// that parse their own command line.
			name:         "Quoted command executable and quoted args",
			input:        `"C:\Program Files\App\uninstall.exe" "/dir=C:\My Projects" --quiet`,
			expectedCmd:  `C:\Program Files\App\uninstall.exe`,
			expectedTail: `"/dir=C:\My Projects" --quiet`,
		},
		{
			// Security: an UNQUOTED path with spaces must not be misparsed into
			// "C:\Program" (hijackable by a planted C:\Program.exe).
			name:         "Unquoted executable path with spaces",
			input:        `C:\Program Files\App\uninstall.exe /S`,
			expectedCmd:  `C:\Program Files\App\uninstall.exe`,
			expectedTail: "/S",
		},
		{
			name:         "Unquoted MSI-style command",
			input:        `MsiExec.exe /X{12345678-1234-1234-1234-123456789012}`,
			expectedCmd:  `MsiExec.exe`,
			expectedTail: "/X{12345678-1234-1234-1234-123456789012}",
		},
		{
			name:         "InstallShield rundll32 entry point",
			input:        `RunDll32 C:\PROGRA~2\COMMON~1\INSTAL~1\PROFES~1\RunTime\11\50\Intel32\Ctor.dll,LaunchSetup "C:\Program Files (x86)\InstallShield Installation Information\{8E0A1A2B-1111-2222-3333-444455556666}\setup.exe" -l0x9  -removeonly`,
			expectedCmd:  `RunDll32`,
			expectedTail: `C:\PROGRA~2\COMMON~1\INSTAL~1\PROFES~1\RunTime\11\50\Intel32\Ctor.dll,LaunchSetup "C:\Program Files (x86)\InstallShield Installation Information\{8E0A1A2B-1111-2222-3333-444455556666}\setup.exe" -l0x9  -removeonly`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exe, tail := splitUninstallString(tt.input)
			if exe != tt.expectedCmd || tail != tt.expectedTail {
				t.Errorf("splitUninstallString(%q) = (%q, %q), want (%q, %q)", tt.input, exe, tail, tt.expectedCmd, tt.expectedTail)
			}
		})
	}
}

func TestScanAppLeftovers(t *testing.T) {
	// Create a temp directory to simulate the base AppData / ProgramFiles folders
	tempDir, err := os.MkdirTemp("", "duster-uninst-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create subdirectories to simulate installed leftovers
	slackDir := filepath.Join(tempDir, "Slack")
	googleDir := filepath.Join(tempDir, "Google")
	ignoredDir := filepath.Join(tempDir, "Microsoft") // should be ignored

	if err := os.MkdirAll(slackDir, 0755); err != nil {
		t.Fatalf("failed to create slack dir: %v", err)
	}
	if err := os.MkdirAll(googleDir, 0755); err != nil {
		t.Fatalf("failed to create google dir: %v", err)
	}
	if err := os.MkdirAll(ignoredDir, 0755); err != nil {
		t.Fatalf("failed to create ignored dir: %v", err)
	}

	// Backup environment variables
	origHome := os.Getenv("HOME")
	origAppData := os.Getenv("APPDATA")
	origLocal := os.Getenv("LOCALAPPDATA")
	origProg := os.Getenv("ProgramFiles")
	origProgX86 := os.Getenv("ProgramFiles(x86)")

	defer func() {
		os.Setenv("HOME", origHome)
		os.Setenv("APPDATA", origAppData)
		os.Setenv("LOCALAPPDATA", origLocal)
		os.Setenv("ProgramFiles", origProg)
		os.Setenv("ProgramFiles(x86)", origProgX86)
	}()

	os.Setenv("APPDATA", tempDir)
	os.Setenv("LOCALAPPDATA", tempDir)
	os.Setenv("ProgramFiles", tempDir)
	os.Setenv("ProgramFiles(x86)", tempDir)

	// 1. Search for Slack
	slackLeftovers := scanAppLeftovers("Slack", "Slack Technologies LLC")
	if len(slackLeftovers) != 1 {
		t.Errorf("Expected 1 leftover folder for Slack, got %d: %v", len(slackLeftovers), slackLeftovers)
	} else {
		baseName := filepath.Base(slackLeftovers[0])
		if baseName != "Slack" {
			t.Errorf("Expected leftover folder name to be 'Slack', got %q", baseName)
		}
	}

	// 2. Google Chrome must NOT sweep the shared "Google" vendor folder, which
	// also holds other Google apps' data (Drive, Earth, ...).
	if chromeLeftovers := scanAppLeftovers("Google Chrome", "Google LLC"); len(chromeLeftovers) != 0 {
		t.Errorf("Google Chrome must not match the shared Google folder, got %v", chromeLeftovers)
	}

	// 3. Search for a non-existent app
	none := scanAppLeftovers("NonExistentApp", "SomePublisher")
	if len(none) != 0 {
		t.Errorf("Expected 0 leftovers for non-existent app, got %d: %v", len(none), none)
	}
}

// Regression: substring/publisher matching swept other apps' data (Firefox
// profiles for Thunderbird, Chrome data for Google Drive, global npm for pnpm).
func TestScanAppLeftoversExactMatchOnly(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"Mozilla", "Thunderbird", "Google", "npm", "Ethereum", "7-Zip", "Adobe", "GitHubDesktop"} {
		if err := os.MkdirAll(filepath.Join(dir, n), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("APPDATA", dir)
	t.Setenv("LOCALAPPDATA", dir)
	t.Setenv("ProgramFiles", "")
	t.Setenv("ProgramFiles(x86)", "")

	cases := []struct {
		name, publisher string
		want            []string
	}{
		{"Mozilla Thunderbird (x64 en-US)", "Mozilla", []string{"Thunderbird"}},
		{"Google Drive", "Google LLC", nil},
		{"pnpm", "pnpm", nil},
		{"Git", "The Git Development Community", nil},
		{"7-Zip 23.01 (x64)", "Igor Pavlov", []string{"7-Zip"}},
		{"Adobe Acrobat (64-bit)", "Adobe", nil},
	}
	for _, c := range cases {
		got := scanAppLeftovers(c.name, c.publisher)
		if len(got) != len(c.want) {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
			continue
		}
		for i := range got {
			if filepath.Base(got[i]) != c.want[i] {
				t.Errorf("%s: got %v, want %v", c.name, got, c.want)
			}
		}
	}
}
