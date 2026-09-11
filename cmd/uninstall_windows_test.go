package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Batch uninstallers must launch under the raw command line, from a path with
// spaces, and receive the registered arguments exactly as written.
func TestNewUninstallCmdRunsBatchUninstallerVerbatim(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "Old App")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	bat := filepath.Join(dir, "uninstall.bat")
	out := filepath.Join(dir, "args.txt")
	// %* is the argument tail exactly as cmd.exe received it.
	script := "@echo off\r\necho %*> \"" + out + "\"\r\n"
	if err := os.WriteFile(bat, []byte(script), 0o644); err != nil {
		t.Fatal(err)
	}

	c, err := newUninstallCmd(`"` + bat + `" "/dir=C:\My Projects" -silent`)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Run(); err != nil {
		t.Fatalf("batch uninstaller did not run: %v", err)
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("batch uninstaller wrote nothing: %v", err)
	}
	if want := `"/dir=C:\My Projects" -silent`; strings.TrimSpace(string(got)) != want {
		t.Fatalf("batch uninstaller got args %q, want %q", strings.TrimSpace(string(got)), want)
	}
}

// Inno Setup and NSIS uninstallers start a copy of themselves and exit at
// once. runTree must wait for the copy, not just the first process.
func TestRunTreeWaitsForHandedOffUninstaller(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "second-phase-done")
	phase2 := filepath.Join(dir, "phase2.bat")
	unins := filepath.Join(dir, "unins.bat")
	if err := os.WriteFile(phase2, []byte("@ping -n 3 127.0.0.1 >nul\r\n@echo done> \""+marker+"\"\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// start /b returns at once, like the first phase handing off to its temp copy.
	if err := os.WriteFile(unins, []byte("@start \"\" /b \""+phase2+"\"\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	c, err := newUninstallCmd(`"` + unins + `"`)
	if err != nil {
		t.Fatal(err)
	}
	if err := runTree(c, func() bool { return false }); err != nil {
		t.Fatalf("runTree: %v", err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatal("runTree returned before the handed-off process finished")
	}
}

func TestNewUninstallCmdResolvesBareNamesAndKeepsArgs(t *testing.T) {
	msiexec := systemExecutable("MsiExec.exe")
	rundll := systemExecutable("RunDll32.exe")
	tests := []struct{ in, path, line string }{
		{
			in:   `MsiExec.exe /X{12345678-1234-1234-1234-123456789012}`,
			path: msiexec,
			line: `"` + msiexec + `" /X{12345678-1234-1234-1234-123456789012}`,
		},
		{
			in:   `RunDll32 C:\x\Ctor.dll,LaunchSetup "C:\Program Files (x86)\y\setup.exe" -l0x9  -removeonly`,
			path: rundll,
			line: `"` + rundll + `" C:\x\Ctor.dll,LaunchSetup "C:\Program Files (x86)\y\setup.exe" -l0x9  -removeonly`,
		},
		{
			in:   `"C:\Program Files\App\uninstall.exe" "/dir=C:\My Projects" --quiet`,
			path: `C:\Program Files\App\uninstall.exe`,
			line: `"C:\Program Files\App\uninstall.exe" "/dir=C:\My Projects" --quiet`,
		},
	}
	for _, tt := range tests {
		c, err := newUninstallCmd(tt.in)
		if err != nil {
			t.Fatalf("newUninstallCmd(%q): %v", tt.in, err)
		}
		if !strings.EqualFold(c.Path, tt.path) {
			t.Errorf("newUninstallCmd(%q).Path = %q, want %q", tt.in, c.Path, tt.path)
		}
		if c.SysProcAttr == nil || c.SysProcAttr.CmdLine != tt.line {
			t.Errorf("newUninstallCmd(%q) command line = %+v, want %q", tt.in, c.SysProcAttr, tt.line)
		}
	}
}
