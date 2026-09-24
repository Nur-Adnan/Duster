package cmd

import (
	"strings"
	"testing"
	"time"
	"unicode/utf16"
)

func utf16Text(t *testing.T, b []byte) string {
	t.Helper()
	if len(b) < 2 || b[0] != 0xFF || b[1] != 0xFE {
		t.Fatalf("task XML must start with a UTF-16LE BOM, got % x", b[:min(len(b), 4)])
	}
	units := make([]uint16, 0, len(b)/2)
	for i := 2; i+1 < len(b); i += 2 {
		units = append(units, uint16(b[i])|uint16(b[i+1])<<8)
	}
	return string(utf16.Decode(units))
}

func TestBuildTaskXML(t *testing.T) {
	cfg := scheduleConfig{Every: "weekly", At: "19:00", LowSpace: 10, Add: []string{"gradle", "npm"}}
	duw := `C:\Program Files\A & B <x> "q" 'y'\duw.exe`
	now := time.Date(2026, 9, 24, 8, 0, 0, 0, time.Local)
	b, err := buildTaskXML(cfg, duw, "S-1-5-21-1-2-3-1001", now)
	if err != nil {
		t.Fatal(err)
	}
	text := utf16Text(t, b)
	for _, want := range []string{
		`<?xml version="1.0" encoding="UTF-16"?>`,
		`xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task"`,
		`<StartBoundary>2026-09-24T19:00:00</StartBoundary>`,
		`<DaysInterval>1</DaysInterval>`,
		`<UserId>S-1-5-21-1-2-3-1001</UserId>`,
		`<LogonType>InteractiveToken</LogonType>`,
		`<RunLevel>LeastPrivilege</RunLevel>`,
		`<DisallowStartIfOnBatteries>true</DisallowStartIfOnBatteries>`,
		`<StopIfGoingOnBatteries>true</StopIfGoingOnBatteries>`,
		`<StartWhenAvailable>true</StartWhenAvailable>`,
		`<ExecutionTimeLimit>PT1H</ExecutionTimeLimit>`,
		`<MultipleInstancesPolicy>IgnoreNew</MultipleInstancesPolicy>`,
		`<Priority>7</Priority>`,
		`<Arguments>schedule run --every weekly --low-space 10 --add gradle,npm</Arguments>`,
		`A &amp; B &lt;x&gt;`,
	} {
		if !strings.Contains(text, want) {
			t.Errorf("task XML lacks %s\n%s", want, text)
		}
	}
	if strings.Contains(text, "HighestAvailable") {
		t.Error("the task must never ask for the highest run level")
	}

	doc, err := parseTaskXML(b)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Trim(doc.Actions.Command, `"`); got != duw {
		t.Errorf("command round trip: %q", got)
	}
	if doc.Start != "2026-09-24T19:00:00" || doc.Principal.RunLevel != "LeastPrivilege" {
		t.Errorf("round trip: %+v", doc)
	}
	if doc.Settings.Enabled == nil || !*doc.Settings.Enabled {
		t.Error("Settings.Enabled must round-trip as true")
	}
}

func TestParseTaskXMLAcceptsWindowsEncodings(t *testing.T) {
	plain := `<?xml version="1.0" encoding="UTF-16"?>
<Task version="1.2" xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task">
  <Triggers><CalendarTrigger><StartBoundary>2026-09-24T03:00:00</StartBoundary></CalendarTrigger></Triggers>
  <Settings><Enabled>false</Enabled></Settings>
  <Actions Context="Author"><Exec><Command>C:\x\duw.exe</Command><Arguments>schedule run --every daily --low-space off</Arguments></Exec></Actions>
</Task>`
	le := func(s string, bom bool) []byte {
		var out []byte
		if bom {
			out = []byte{0xFF, 0xFE}
		}
		for _, u := range utf16.Encode([]rune(s)) {
			out = append(out, byte(u), byte(u>>8))
		}
		return out
	}
	for name, b := range map[string][]byte{
		"single-byte":       []byte(plain),
		"UTF-16LE":          le(plain, false),
		"UTF-16LE with BOM": le(plain, true),
		"UTF-8 with BOM":    append([]byte{0xEF, 0xBB, 0xBF}, plain...),
	} {
		t.Run(name, func(t *testing.T) {
			doc, err := parseTaskXML(b)
			if err != nil {
				t.Fatal(err)
			}
			if doc.Actions.Command != `C:\x\duw.exe` || doc.Start != "2026-09-24T03:00:00" {
				t.Errorf("parsed %+v", doc)
			}
			if doc.Settings.Enabled == nil || *doc.Settings.Enabled {
				t.Error("a disabled task must read as disabled")
			}
		})
	}
	// A task exported without <Enabled> is enabled (the schema default).
	doc, err := parseTaskXML([]byte(strings.Replace(plain, "<Enabled>false</Enabled>", "", 1)))
	if err != nil || doc.Settings.Enabled != nil {
		t.Errorf("missing Enabled: %v, %v", doc.Settings.Enabled, err)
	}
}

func TestParseTaskNames(t *testing.T) {
	out := []byte("\"\\Duster Scheduled Clean (alice)\",\"9/25/2026 7:00:00 PM\",\"Ready\"\r\n" +
		"\"\\Microsoft\\Windows\\Defrag\\ScheduledDefrag\",\"N/A\",\"Ready\"\r\n" +
		"\"\\Duster Scheduled Clean (bob)\",\"N/A\",\"Disabled\"\r\n" +
		"\"\\Duster Scheduled Clean (alice)\",\"9/26/2026 7:00:00 PM\",\"Ready\"\r\n")
	got := parseTaskNames(out)
	if strings.Join(got, "|") != "Duster Scheduled Clean (alice)|Duster Scheduled Clean (bob)" {
		t.Errorf("parseTaskNames = %q", got)
	}
}
