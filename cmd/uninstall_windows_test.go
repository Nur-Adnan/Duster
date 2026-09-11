package cmd

import (
	"strings"
	"testing"
)

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
