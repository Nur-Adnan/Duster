package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// Regression: a category root that is a symlink/junction must be skipped, not
// followed (os.Stat/ReadDir follow links, so its target used to be emptied).
func TestCleanDirCategorySkipsSymlinkedRoot(t *testing.T) {
	tmp := t.TempDir()
	victim := filepath.Join(tmp, "victim")
	if err := os.MkdirAll(victim, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(victim, "important.txt"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(tmp, "cache")
	if err := os.Symlink(victim, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	cat := CleanCategory{ID: "linkroot", Name: "Test Link Root", Paths: []string{link}}
	if size, files, _ := scanDirCategory(cat); size != 0 || files != 0 {
		t.Errorf("scan must skip a linked root, got %d bytes / %d files", size, files)
	}
	if _, _, err := cleanDirCategory(cat); err != nil {
		t.Fatalf("cleanDirCategory: %v", err)
	}
	if _, err := os.Stat(filepath.Join(victim, "important.txt")); err != nil {
		t.Fatalf("symlink target was emptied: %v", err)
	}
}

// A root the user can't list (C:\Windows\Temp for a standard user) is skipped
// like the scan skips it, not reported as a failed delete.
func TestCleanDirCategorySkipsUnlistableRoot(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("needs POSIX directory permissions")
	}
	root := t.TempDir()
	if err := os.Chmod(root, 0o300); err != nil { // write+search, no read: can't list
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(root, 0o755) })
	cat := CleanCategory{ID: "unlistable", Name: "Test Unlistable", Paths: []string{root}}
	if _, _, err := cleanDirCategory(cat); err != nil {
		t.Fatalf("unlistable root must be skipped, not failed: %v", err)
	}
}

// Delete failures must be returned (not only logged) while bytes that were
// freed still count.
func TestCleanDirCategoryReportsFailures(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("a read-only directory does not block deleting its children on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	okRoot, lockedRoot := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(okRoot, "a.tmp"), []byte("12345"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(lockedRoot, "b.tmp"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(lockedRoot, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(lockedRoot, 0o755) })

	cat := CleanCategory{ID: "partial", Name: "Test Partial", Paths: []string{okRoot, lockedRoot}}
	freed, _, err := cleanDirCategory(cat)
	if err == nil {
		t.Fatal("expected an error for the undeletable entry")
	}
	if freed != 5 {
		t.Errorf("freed = %d, want 5 (the deletable file still counts)", freed)
	}
}

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		name     string
		input    int64
		expected string
	}{
		{"Zero bytes", 0, "0 B"},
		{"One byte", 1, "1 B"},
		{"512 bytes", 512, "512 B"},
		{"1 KB", 1024, "1.00 KB"},
		{"1.5 KB", 1536, "1.50 KB"},
		{"1 MB", 1024 * 1024, "1.00 MB"},
		{"1 GB", 1024 * 1024 * 1024, "1.00 GB"},
		{"10 GB", 10 * 1024 * 1024 * 1024, "10.00 GB"},
		{"1 TB", int64(1024) * 1024 * 1024 * 1024, "1.00 TB"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatBytes(tt.input)
			if result != tt.expected {
				t.Errorf("formatBytes(%d) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestFormatInt(t *testing.T) {
	tests := []struct {
		input    int
		expected string
	}{
		{0, "0"},
		{5, "5"},
		{999, "999"},
		{1000, "1,000"},
		{12345, "12,345"},
		{1000000, "1,000,000"},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("formatInt(%d)", tt.input), func(t *testing.T) {
			result := formatInt(tt.input)
			if result != tt.expected {
				t.Errorf("formatInt(%d) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestGetCategories(t *testing.T) {
	cats := getCategories()

	// We now have 35 categories — verify minimum expected count
	if len(cats) < 30 {
		t.Errorf("Expected at least 30 cleanup categories, got %d", len(cats))
	}

	// Verify no duplicate IDs
	seen := make(map[string]bool)
	for _, cat := range cats {
		if seen[cat.ID] {
			t.Errorf("Duplicate category ID found: %q", cat.ID)
		}
		seen[cat.ID] = true
	}

	// Verify critical categories exist
	critical := []string{"temp", "browsers", "recycle", "dns", "npm", "discord", "spotify", "jetbrains"}
	for _, id := range critical {
		if !seen[id] {
			t.Errorf("Missing critical cleanup category: %q", id)
		}
	}
}

func TestScanDirCategoryWithTempDir(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test files
	for i := 0; i < 5; i++ {
		fpath := filepath.Join(tmpDir, fmt.Sprintf("testfile_%d.tmp", i))
		_ = os.WriteFile(fpath, []byte("test data payload"), 0644)
	}

	cat := CleanCategory{
		ID:    "test_scan",
		Name:  "Test Scan",
		Paths: []string{tmpDir},
	}

	size, files, err := scanDirCategory(cat)
	if err != nil {
		t.Fatalf("scanDirCategory failed: %v", err)
	}
	if files != 5 {
		t.Errorf("Expected 5 files, got %d", files)
	}
	if size <= 0 {
		t.Errorf("Expected positive size, got %d", size)
	}
}

func TestScanDirCategoryWithPattern(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test files with different extensions
	_ = os.WriteFile(filepath.Join(tmpDir, "data.log"), []byte("log content"), 0644)
	_ = os.WriteFile(filepath.Join(tmpDir, "data.txt"), []byte("txt content"), 0644)
	_ = os.WriteFile(filepath.Join(tmpDir, "cache.log"), []byte("cache log"), 0644)

	cat := CleanCategory{
		ID:      "test_pattern",
		Name:    "Test Pattern",
		Paths:   []string{tmpDir},
		Pattern: "*.log",
	}

	size, files, err := scanDirCategory(cat)
	if err != nil {
		t.Fatalf("scanDirCategory with pattern failed: %v", err)
	}
	if files != 2 {
		t.Errorf("Expected 2 .log files, got %d", files)
	}
	if size <= 0 {
		t.Errorf("Expected positive size, got %d", size)
	}
}

func TestCleanDirCategoryActualDelete(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test files
	for i := 0; i < 3; i++ {
		fpath := filepath.Join(tmpDir, fmt.Sprintf("deleteme_%d.tmp", i))
		_ = os.WriteFile(fpath, []byte("delete this data"), 0644)
	}

	cat := CleanCategory{
		ID:    "test_delete",
		Name:  "Test Delete",
		Paths: []string{tmpDir},
	}

	sizeFreed, filesFreed, err := cleanDirCategory(cat)
	if err != nil {
		t.Fatalf("cleanDirCategory failed: %v", err)
	}
	if filesFreed != 3 {
		t.Errorf("Expected 3 files freed, got %d", filesFreed)
	}
	if sizeFreed <= 0 {
		t.Errorf("Expected positive size freed, got %d", sizeFreed)
	}

	// Verify files are actually gone
	entries, _ := os.ReadDir(tmpDir)
	if len(entries) != 0 {
		t.Errorf("Expected empty directory after cleanup, got %d entries", len(entries))
	}
}

func TestWhitelistFiltering(t *testing.T) {
	cats := getCategories()

	whitelist := map[string]bool{
		"temp":     true,
		"browsers": true,
	}

	var scannedIDs []string
	for _, cat := range cats {
		if !whitelist[cat.ID] {
			scannedIDs = append(scannedIDs, cat.ID)
		}
	}

	// Verify whitelisted items are excluded
	for _, id := range scannedIDs {
		if id == "temp" || id == "browsers" {
			t.Errorf("Whitelisted category %q should have been excluded", id)
		}
	}

	// Verify non-whitelisted items remain
	if len(scannedIDs) < len(cats)-2 {
		t.Errorf("Too many categories excluded; expected %d, got %d", len(cats)-2, len(scannedIDs))
	}
}

func TestPadRight(t *testing.T) {
	tests := []struct {
		input    string
		width    int
		expected string
	}{
		{"hello", 10, "hello     "},
		{"exactly10!", 10, "exactly10!"},
		{"long string here", 5, "long string here"},
		{"", 3, "   "},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("padRight(%q,%d)", tt.input, tt.width), func(t *testing.T) {
			result := padRight(tt.input, tt.width)
			if result != tt.expected {
				t.Errorf("padRight(%q, %d) = %q, want %q", tt.input, tt.width, result, tt.expected)
			}
		})
	}
}

func TestTruncateString(t *testing.T) {
	tests := []struct {
		input    string
		maxLen   int
		expected string
	}{
		{"hello", 10, "hello"},
		{"hello world", 5, "hell…"},
		{"abc", 3, "abc"},
		{"ab", 3, "ab"},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("truncateString(%q,%d)", tt.input, tt.maxLen), func(t *testing.T) {
			result := truncateString(tt.input, tt.maxLen)
			if result != tt.expected {
				t.Errorf("truncateString(%q, %d) = %q, want %q", tt.input, tt.maxLen, result, tt.expected)
			}
		})
	}
}
