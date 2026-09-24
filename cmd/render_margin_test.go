package cmd

import (
	"regexp"
	"strings"
	"testing"
)

var sgr = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// lipgloss pads every line of a multi-line string to the widest one, so
// style.Render("---\n\n") ends in a line of spaces and the next text starts
// about 70 columns in. verify's "Integrity Status: SECURED & CERTIFIED" showed
// as "SECURE", cut off at 120 columns. Screens must start text at the margin.
func TestScreensStartTextAtTheLeftMargin(t *testing.T) {
	healthy := VerifyReport{Healthy: true, Total: 1, Passed: 1, Cases: []VerifyTestCase{{Name: "Case", Passed: true, Details: "ok"}}}
	views := map[string]string{
		"verify":    verifyModel{report: healthy}.View(),
		"doctor":    doctorModel{}.View(),
		"benchmark": benchmarkModel{}.View(),
		"optimize":  initialOptimizeModel().View(),
		"remove":    initialRemoveModel(`C:\Tools\du.exe`).View(),
		"installer": initialInstallerModel().View(),
		"purge":     initialPurgeModel(t.TempDir()).View(),
		"vdisk":     initialVdiskModel().View(),
	}
	for name, v := range views {
		for i, line := range strings.Split(sgr.ReplaceAllString(v, ""), "\n") {
			text := strings.TrimLeft(line, " ")
			if indent := len(line) - len(text); text != "" && indent > 20 {
				t.Errorf("%s: line %d starts at column %d: %q", name, i, indent, text)
			}
		}
	}
}
