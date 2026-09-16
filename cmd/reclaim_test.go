package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The report Microsoft documents for AnalyzeComponentStore.
const dismAnalyzeSample = `Deployment Image Servicing and Management tool
Version: 10.0.22621.1

Image Version: 10.0.22621.1

[==========================100.0%==========================]

Component Store (WinSxS) information:

Windows Explorer Reported Size of Component Store : 4.98 GB

Actual Size of Component Store : 4.88 GB

    Shared with Windows : 4.38 GB
    Backups and Disabled Features : 506.90 MB
    Cache and Temporary Data : 279.52 KB

Date of Last Cleanup : 2021-06-24 23:32:22

Number of Reclaimable Packages : 0
Component Store Cleanup Recommended : No

The operation completed successfully.
`

// dismBytes mirrors the conversion parseDismSize does, so the expectations
// round the same way DISM's own two-decimal output does.
func dismBytes(amount float64, unit int64) int64 { return int64(amount * float64(unit)) }

func TestParseComponentStore(t *testing.T) {
	info := parseComponentStore(dismAnalyzeSample)

	if info.Unparsed {
		t.Fatal("the documented DISM report was not understood")
	}
	if want := dismBytes(4.88, 1<<30); info.ActualBytes != want {
		t.Errorf("actual size = %d, want %d", info.ActualBytes, want)
	}
	if want := dismBytes(506.90, 1<<20); info.BackupsBytes != want {
		t.Errorf("backups = %d, want %d", info.BackupsBytes, want)
	}
	if want := dismBytes(279.52, 1<<10); info.CacheBytes != want {
		t.Errorf("cache = %d, want %d", info.CacheBytes, want)
	}
	// Microsoft defines the overhead as backups plus cache; the rest is
	// shared with Windows and cannot be reclaimed.
	if want := info.BackupsBytes + info.CacheBytes; info.OverheadBytes != want {
		t.Errorf("overhead = %d, want %d", info.OverheadBytes, want)
	}
	if info.ReclaimablePkgs != 0 {
		t.Errorf("reclaimable packages = %d, want 0", info.ReclaimablePkgs)
	}
	if info.CleanupRecommended {
		t.Error("cleanup reported as recommended, but the report says No")
	}
	if info.LastCleanup != "2021-06-24 23:32:22" {
		t.Errorf("last cleanup = %q", info.LastCleanup)
	}
}

func TestParseComponentStoreCleanupRecommended(t *testing.T) {
	out := strings.Replace(dismAnalyzeSample, "Component Store Cleanup Recommended : No",
		"Component Store Cleanup Recommended : Yes", 1)
	out = strings.Replace(out, "Number of Reclaimable Packages : 0",
		"Number of Reclaimable Packages : 7", 1)

	info := parseComponentStore(out)
	if !info.CleanupRecommended || info.ReclaimablePkgs != 7 {
		t.Errorf("recommended = %v, packages = %d; want true, 7", info.CleanupRecommended, info.ReclaimablePkgs)
	}
}

// A localized or otherwise unreadable report must be reported as unknown, so
// the caller never tells the user there is nothing to reclaim.
func TestParseComponentStoreUnreadableOutput(t *testing.T) {
	for name, out := range map[string]string{
		"localized": "Informationen zum Komponentenspeicher:\n\nTatsächliche Größe des Komponentenspeichers: 4,88 GB\n",
		"empty":     "",
		"error":     "Error: 740\n\nElevated permissions are required to run DISM.\n",
	} {
		t.Run(name, func(t *testing.T) {
			info := parseComponentStore(out)
			if !info.Unparsed {
				t.Error("unreadable output was accepted as a valid report")
			}
			if info.ActualBytes != 0 || info.OverheadBytes != 0 {
				t.Errorf("unreadable output produced sizes: %+v", info)
			}
		})
	}
}

func TestParseDismSize(t *testing.T) {
	tests := []struct {
		in    string
		want  int64
		valid bool
	}{
		{"4.98 GB", dismBytes(4.98, 1<<30), true},
		{"506.90 MB", dismBytes(506.90, 1<<20), true},
		{"279.52 KB", dismBytes(279.52, 1<<10), true},
		{"512 B", 512, true},
		{"0 bytes", 0, true}, // DISM's spelling for a zero-size field
		{"1 byte", 1, true},
		{"1,024 MB", 1024 * (1 << 20), true},
		{"4,88 GB", 0, false}, // comma decimal separator from a localized build
		{"unknown", 0, false},
		{"12 QB", 0, false},
		{"", 0, false},
	}
	for _, tt := range tests {
		got, ok := parseDismSize(tt.in)
		if ok != tt.valid || got != tt.want {
			t.Errorf("parseDismSize(%q) = %d, %v; want %d, %v", tt.in, got, ok, tt.want, tt.valid)
		}
	}
}

func TestDismCodeOK(t *testing.T) {
	tests := []struct {
		code   int
		reboot bool
		ok     bool
	}{
		{0, false, true},
		{3010, true, true}, // success, restart to finish
		{740, false, false},
		{87, false, false},
		{-1, false, false},
	}
	for _, tt := range tests {
		reboot, ok := dismCodeOK(tt.code)
		if reboot != tt.reboot || ok != tt.ok {
			t.Errorf("dismCodeOK(%d) = %v, %v; want %v, %v", tt.code, reboot, ok, tt.reboot, tt.ok)
		}
	}
}

func TestBuildReclaimItemsReportsWhatExists(t *testing.T) {
	root := t.TempDir()
	oldDir := filepath.Join(root, "Windows.old")
	if err := os.MkdirAll(filepath.Join(oldDir, "Users"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(oldDir, "Users", "profile.dat"), make([]byte, 2048), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "hiberfil.sys"), make([]byte, 4096), 0o644); err != nil {
		t.Fatal(err)
	}

	items := buildReclaimItems(root)
	if len(items) != 2 {
		t.Fatalf("got %d items, want 2", len(items))
	}
	byID := map[string]reclaimItem{}
	for _, item := range items {
		byID[item.ID] = item
	}

	if got := byID["windows_old"]; !got.Present || got.Bytes != 2048 || got.Partial {
		t.Errorf("windows_old = %+v; want present, 2048 bytes, complete", got)
	}
	if got := byID["hibernation"]; !got.Present || got.Bytes != 4096 {
		t.Errorf("hibernation = %+v; want present, 4096 bytes", got)
	}
	for _, item := range items {
		if item.Advice == "" || len(item.Hints) == 0 {
			t.Errorf("%s has no advice for the user", item.ID)
		}
	}
}

func TestBuildReclaimItemsReportsNothingWhenAbsent(t *testing.T) {
	for _, item := range buildReclaimItems(t.TempDir()) {
		if item.Present || item.Bytes != 0 {
			t.Errorf("%s reported on an empty drive: %+v", item.ID, item)
		}
	}
}

// A preview must not hang on a huge tree: an expired budget reports a lower
// bound instead of a complete total.
func TestDirSizeWithinStopsAtBudget(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.bin"), make([]byte, 32), 0o644); err != nil {
		t.Fatal(err)
	}

	if size, partial := dirSizeWithin(dir, time.Minute); size != 32 || partial {
		t.Errorf("with budget: %d bytes, partial=%v; want 32, false", size, partial)
	}
	if _, partial := dirSizeWithin(dir, -time.Second); !partial {
		t.Error("an expired budget was not reported as a partial measurement")
	}
}

func TestRenderReclaimSection(t *testing.T) {
	measuring := renderReclaimSection(nil, false)
	if !strings.Contains(measuring, "Measuring") {
		t.Error("the section does not say it is still measuring")
	}

	empty := renderReclaimSection(buildReclaimItems(t.TempDir()), true)
	if !strings.Contains(empty, "Nothing to report") {
		t.Errorf("an empty report does not say so:\n%s", empty)
	}

	view := renderReclaimSection([]reclaimItem{
		{ID: "windows_old", Name: "Previous Windows installation", Present: true, Bytes: 12 << 30, Partial: true, Hints: windowsOldHints},
		{ID: "hibernation", Name: "Hibernation file", Present: true, Bytes: 16 << 30, Hints: hibernationHints},
	}, true)
	for _, want := range []string{"Previous Windows installation", "at least", "12.00 GB", "Hibernation file", "16.00 GB", "powercfg"} {
		if !strings.Contains(view, want) {
			t.Errorf("the report does not mention %q:\n%s", want, view)
		}
	}
	// Same rule as the other screens: styled text must not push lines right.
	for i, line := range strings.Split(view, "\n") {
		text := strings.TrimLeft(line, " ")
		if indent := len(line) - len(text); text != "" && indent > 20 {
			t.Errorf("line %d starts at column %d: %q", i, indent, text)
		}
	}
}

// A report where only some labels are understood must not look like a clean
// store: the decision fields drive "nothing to clean up", so a half-read
// report counts as unreadable.
// DISM writes zero-size fields as "0 bytes"; a report using it is complete.
func TestParseComponentStoreAcceptsByteSpelling(t *testing.T) {
	out := strings.Replace(dismAnalyzeSample, "    Cache and Temporary Data : 279.52 KB",
		"    Cache and Temporary Data : 0 bytes", 1)
	info := parseComponentStore(out)
	if info.Unparsed {
		t.Fatalf("a report with \"0 bytes\" was rejected, missing: %v", info.MissingFields)
	}
	if info.CacheBytes != 0 || info.OverheadBytes != info.BackupsBytes {
		t.Errorf("cache = %d, overhead = %d; want 0 and the backups size", info.CacheBytes, info.OverheadBytes)
	}
}

func TestParseComponentStorePartialReportIsUnreadable(t *testing.T) {
	drop := map[string]string{
		"no recommendation": "Component Store Cleanup Recommended : No",
		"no package count":  "Number of Reclaimable Packages : 0",
		"no backups size":   "    Backups and Disabled Features : 506.90 MB",
		"no cache size":     "    Cache and Temporary Data : 279.52 KB",
		"no actual size":    "Actual Size of Component Store : 4.88 GB",
	}
	for name, line := range drop {
		t.Run(name, func(t *testing.T) {
			out := strings.Replace(dismAnalyzeSample, line, "", 1)
			info := parseComponentStore(out)
			if !info.Unparsed {
				t.Fatalf("a report without %q was accepted as complete", line)
			}
			if info.ActualBytes != 0 || info.OverheadBytes != 0 || info.ReclaimablePkgs != 0 || info.CleanupRecommended {
				t.Errorf("a half-read report produced values: %+v", info)
			}
			if len(info.MissingFields) == 0 {
				t.Error("the report does not name the label it could not read")
			}
		})
	}
}

// A localized value keeps the label readable but the number unusable; that
// must be treated the same way.
func TestParseComponentStoreLocalizedNumberIsUnreadable(t *testing.T) {
	out := strings.Replace(dismAnalyzeSample, "Actual Size of Component Store : 4.88 GB",
		"Actual Size of Component Store : 4,88 GB", 1)
	if info := parseComponentStore(out); !info.Unparsed || info.ActualBytes != 0 {
		t.Errorf("localized size accepted: %+v", info)
	}
}
