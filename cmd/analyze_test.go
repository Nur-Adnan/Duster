package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

// scanDirectory sizes the tree, skips node_modules/.git, and the in-memory
// navigation helpers (showNode, topFiles, countFilesAndFolders) describe a
// subfolder without rescanning it.
func TestAnalyzeScanAndInMemoryNavigation(t *testing.T) {
	root := t.TempDir()
	write := func(rel string, n int) {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, make([]byte, n), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("a/big.bin", 300)
	write("a/sub/x.bin", 50)
	write("b/small.bin", 10)
	write("c.txt", 5)
	write("node_modules/dep/i.js", 1000) // skipped by the scanner

	tree, large, err := scanDirectory(root, nil)
	if err != nil {
		t.Fatalf("scanDirectory: %v", err)
	}
	if tree.Size != 365 {
		t.Errorf("root size = %d, want 365 (node_modules excluded)", tree.Size)
	}
	if len(large) != 4 || filepath.Base(large[0].Path) != "big.bin" || filepath.Base(large[3].Path) != "c.txt" {
		t.Errorf("large files not ordered largest-first: %v", large)
	}
	if files, folders := countFilesAndFolders(tree); files != 4 || folders != 3 {
		t.Errorf("counts = %d files / %d folders, want 4 / 3", files, folders)
	}
	if len(tree.Entries) != 3 {
		t.Errorf("root entries = %d, want 3 (a, b, c.txt)", len(tree.Entries))
	}
	if top := topFiles(tree, 2); len(top) != 2 || top[0].Size != 300 || top[1].Size != 50 {
		t.Errorf("topFiles(2) = %v, want sizes 300, 50", top)
	}

	var a *FolderNode
	for _, sub := range tree.SubFolders {
		if sub.Name == "a" {
			a = sub
		}
	}
	if a == nil {
		t.Fatal("subfolder a not scanned")
	}
	var m analyzeModel
	m.selectedIdx = 2
	m.showNode(a)
	if m.tree != a || m.selectedIdx != 0 {
		t.Fatalf("showNode did not switch to the subfolder")
	}
	if len(m.tree.Entries) != 2 || m.fileCount != 2 || m.folderCount != 1 {
		t.Errorf("subfolder view: %d entries, %d files, %d folders; want 2, 2, 1",
			len(m.tree.Entries), m.fileCount, m.folderCount)
	}
	if len(m.largeFiles) != 2 || m.largeFiles[0].Size != 300 {
		t.Errorf("subfolder large files = %v", m.largeFiles)
	}
}
