package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// A category outside every group would be missing from the CLI scan listing
// and the TUI, yet the CLI delete phase would still clean it.
func TestCleanGroupsCoverEveryCategory(t *testing.T) {
	ids := map[string]bool{}
	for _, c := range getCategories() {
		ids[c.ID] = true
		n := 0
		for _, g := range cleanGroups {
			if g.catID[c.ID] {
				n++
			}
		}
		if n != 1 {
			t.Errorf("category %q is in %d groups, want 1", c.ID, n)
		}
	}
	for _, g := range cleanGroups {
		for id := range g.catID {
			if !ids[id] {
				t.Errorf("group %q lists unknown category %q", g.name, id)
			}
		}
	}
}

// The TUI must offer exactly what `du clean --yes` cleans, in the CLI's order.
func TestCleanTuiOffersEveryCategory(t *testing.T) {
	cats := getCategories()
	items := cleanTuiItems(cats)
	if len(items) != len(cats) {
		t.Fatalf("TUI lists %d items, the CLI cleans %d categories", len(items), len(cats))
	}
	names := map[string]string{}
	for _, c := range cats {
		names[c.ID] = c.Name
	}
	group := func(id string) int {
		for i, g := range cleanGroups {
			if g.catID[id] {
				return i
			}
		}
		return -1
	}
	prev := 0
	for _, it := range items {
		if it.Name != names[it.ID] || it.cat.ID != it.ID {
			t.Errorf("item %q (%q) does not match its category", it.ID, it.Name)
		}
		if !it.Checked {
			t.Errorf("%q starts unticked, but the CLI cleans it", it.ID)
		}
		if g := group(it.ID); g < prev {
			t.Errorf("%q is out of the CLI's group order", it.ID)
		} else {
			prev = g
		}
	}
}

// `--whitelist logs` protected the TUI's old combined item, so it must still
// skip both categories that item covered.
func TestCleanTuiWhitelistLogsSkipsBothParts(t *testing.T) {
	saved := whitelist
	t.Cleanup(func() { whitelist = saved })
	whitelist = []string{"logs"}

	m := cleanModel{items: cleanTuiItems(getCategories())}
	cmds := m.scanCmds()
	seen := 0
	for i, it := range m.items {
		if it.ID != "wer" && it.ID != "logfiles" {
			continue
		}
		seen++
		if msg, ok := cmds[i]().(cleanScanProgressMsg); !ok || msg.Status != "skipped" {
			t.Errorf("%s: scan returned %+v, want skipped", it.ID, msg)
		}
	}
	if seen != 2 {
		t.Fatalf("found %d of the wer and logfiles items, want 2", seen)
	}
}

func TestRunCategoryScansOrCleans(t *testing.T) {
	var scanOnly []bool
	custom := CleanCategory{ID: "custom", CustomScan: func(s, _ bool) (int64, int, error) {
		scanOnly = append(scanOnly, s)
		return 0, 0, nil
	}}
	_, _, _ = runCategory(custom, true)
	_, _, _ = runCategory(custom, false)
	if len(scanOnly) != 2 || !scanOnly[0] || scanOnly[1] {
		t.Errorf("custom handler got scanOnly %v, want [true false]", scanOnly)
	}

	dir := t.TempDir()
	f := filepath.Join(dir, "cache.bin")
	if err := os.WriteFile(f, make([]byte, 10), 0o644); err != nil {
		t.Fatal(err)
	}
	cat := CleanCategory{ID: "dir", Name: "Dir", Paths: []string{dir}}
	if size, files, err := runCategory(cat, true); err != nil || size != 10 || files != 1 {
		t.Fatalf("scan = %d bytes, %d files, %v; want 10, 1, nil", size, files, err)
	}
	if _, err := os.Stat(f); err != nil {
		t.Fatal("a scan deleted the file")
	}
	if _, _, err := runCategory(cat, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(f); !os.IsNotExist(err) {
		t.Error("clean left the file behind")
	}
}

// scannedCleanModel is a TUI that finished scanning every category, in a
// 120-column terminal of the given height.
func scannedCleanModel(height int) cleanModel {
	items := cleanTuiItems(getCategories())
	for _, it := range items {
		it.Status, it.Scanning, it.Progress = "ok", false, 100
	}
	return cleanModel{state: cleanStateReady, dryRun: true, launchDryRun: true, width: 120, height: height, items: items}
}

func screenLines(v string) int { return len(strings.Split(v, "\n")) }

// Bubble Tea drops lines above the top of the terminal: an unscrolled list of
// every category would push the header and first rows off screen.
func TestCleanListScrollsToFitTheTerminal(t *testing.T) {
	for _, height := range []int{40, 15} {
		m := scannedCleanModel(height)
		first, last := m.items[0].Name, m.items[len(m.items)-1].Name

		v := m.View()
		if strings.Contains(v, last) {
			t.Errorf("height %d: drew every row, which cannot fit", height)
		}
		if height == 40 {
			if n := screenLines(v); n > height {
				t.Errorf("height 40: view is %d lines", n)
			}
			for _, want := range []string{"du clean", "Ready to clean", first, "more"} {
				if !strings.Contains(v, want) {
					t.Errorf("height 40: %q is not on screen", want)
				}
			}
		}

		for range m.items {
			updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
			m = updated.(cleanModel)
		}
		v = m.View()
		if !strings.Contains(v, last) || strings.Contains(v, first) {
			t.Errorf("height %d: with the cursor on the last row, the list did not scroll to it", height)
		}
		if height == 40 && screenLines(v) > height {
			t.Errorf("height 40: scrolled view is %d lines", screenLines(v))
		}
	}
}

// A finished run keeps the window where cleaning left it, and up/down scroll
// back to the rows above.
func TestCleanDoneKeepsAndScrollsTheWindow(t *testing.T) {
	m := scannedCleanModel(40)
	first, last := m.items[0].Name, m.items[len(m.items)-1].Name

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(cleanModel)
	var next tea.Msg
	for _, c := range cmd().(tea.BatchMsg) {
		if msg, ok := c().(cleanDeletionProgressMsg); ok {
			next = msg
		}
	}
	for m.state != cleanStateDone {
		if next == nil {
			t.Fatal("the dry run stopped before Done")
		}
		updated, cmd = m.Update(next)
		m = updated.(cleanModel)
		next = nil
		if cmd != nil {
			next = cmd()
		}
	}

	if v := m.View(); !strings.Contains(v, last) || strings.Contains(v, first) {
		t.Error("Done moved the window away from the last cleaned row")
	}
	for range m.items {
		updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
		m = updated.(cleanModel)
	}
	if v := m.View(); !strings.Contains(v, first) {
		t.Error("up in Done did not scroll back to the first row")
	}
}
