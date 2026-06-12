package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseUninstallString(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		expectedCmd  string
		expectedArgs []string
	}{
		{
			name:         "Empty command",
			input:        "",
			expectedCmd:  "",
			expectedArgs: nil,
		},
		{
			name:         "Simple command without quotes or args",
			input:        `C:\Windows\System32\uninstall.exe`,
			expectedCmd:  `C:\Windows\System32\uninstall.exe`,
			expectedArgs: nil,
		},
		{
			name:         "Command with unquoted args",
			input:        `C:\Windows\System32\uninstall.exe /S /clean`,
			expectedCmd:  `C:\Windows\System32\uninstall.exe`,
			expectedArgs: []string{"/S", "/clean"},
		},
		{
			name:         "Quoted command executable with unquoted args",
			input:        `"C:\Program Files\App\uninstall.exe" --silent --force`,
			expectedCmd:  `C:\Program Files\App\uninstall.exe`,
			expectedArgs: []string{"--silent", "--force"},
		},
		{
			name:         "Quoted command executable and quoted args",
			input:        `"C:\Program Files\App\uninstall.exe" "/dir=C:\My Projects" --quiet`,
			expectedCmd:  `C:\Program Files\App\uninstall.exe`,
			expectedArgs: []string{`/dir=C:\My Projects`, "--quiet"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmdVal, args := parseUninstallString(tt.input)
			if cmdVal != tt.expectedCmd {
				t.Errorf("parseUninstallString(%q) got cmd=%q, want %q", tt.input, cmdVal, tt.expectedCmd)
			}
			if len(args) != len(tt.expectedArgs) {
				t.Errorf("parseUninstallString(%q) got args len=%d, want %d (got: %v, want: %v)",
					tt.input, len(args), len(tt.expectedArgs), args, tt.expectedArgs)
			} else {
				for i := range args {
					if args[i] != tt.expectedArgs[i] {
						t.Errorf("parseUninstallString(%q) arg[%d] got %q, want %q", tt.input, i, args[i], tt.expectedArgs[i])
					}
				}
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

	// 2. Search for Google Chrome
	chromeLeftovers := scanAppLeftovers("Google Chrome", "Google LLC")
	if len(chromeLeftovers) != 1 {
		t.Errorf("Expected 1 leftover folder for Google Chrome, got %d: %v", len(chromeLeftovers), chromeLeftovers)
	} else {
		baseName := filepath.Base(chromeLeftovers[0])
		if baseName != "Google" {
			t.Errorf("Expected leftover folder name to be 'Google', got %q", baseName)
		}
	}

	// 3. Search for a non-existent app
	none := scanAppLeftovers("NonExistentApp", "SomePublisher")
	if len(none) != 0 {
		t.Errorf("Expected 0 leftovers for non-existent app, got %d: %v", len(none), none)
	}
}
