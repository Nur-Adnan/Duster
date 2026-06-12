package cmd

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestUpdateModelInitialization(t *testing.T) {
	m := initialUpdateModel()

	if m.state != stateUpIdle {
		t.Errorf("Expected initial state to be stateUpIdle, got %v", m.state)
	}

	if m.currentVersion != AppVersion {
		t.Errorf("Expected starting currentVersion to be %s, got %s", AppVersion, m.currentVersion)
	}

	if m.updateFound {
		t.Error("Expected starting updateFound to be false")
	}
}

func TestUpdateModelTransitions(t *testing.T) {
	m := initialUpdateModel()

	// 1. Press Enter to start checking
	enterMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("enter")}
	updatedModel, cmd := m.Update(enterMsg)
	upM := updatedModel.(updateModel)

	if upM.state != stateUpChecking {
		t.Errorf("Expected state to become stateUpChecking after Enter, got %v", upM.state)
	}

	if cmd == nil {
		t.Error("Expected check command to be returned, got nil")
	}

	// 2. Simulate checking complete with new version found
	rel := releaseMetadata{
		TagName:     "v1.0.2",
		PublishedAt: "2026-05-18T12:00:00Z",
		Body:        "New features and improvements",
		Size:        16 * 1024 * 1024,
	}
	checkMsg := checkCompleteMsg{release: rel, err: nil}
	updatedModel2, _ := upM.Update(checkMsg)
	upM2 := updatedModel2.(updateModel)

	if !upM2.updateFound {
		t.Error("Expected updateFound to be true when tag is newer")
	}

	if upM2.latestVersion != "1.0.2" {
		t.Errorf("Expected latestVersion to be 1.0.2, got %s", upM2.latestVersion)
	}

	// 3. Press y to start downloading
	yMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")}
	updatedModel3, cmd3 := upM2.Update(yMsg)
	upM3 := updatedModel3.(updateModel)

	if upM3.state != stateUpDownloading {
		t.Errorf("Expected state to become stateUpDownloading, got %v", upM3.state)
	}

	if cmd3 == nil {
		t.Error("Expected download command, got nil")
	}

	// 4. Simulate download complete
	downloadMsg := downloadCompleteMsg{bytes: []byte("mock binary content"), err: nil}
	updatedModel4, cmd4 := upM3.Update(downloadMsg)
	upM4 := updatedModel4.(updateModel)

	if upM4.state != stateUpSwapping {
		t.Errorf("Expected state to become stateUpSwapping, got %v", upM4.state)
	}

	if cmd4 == nil {
		t.Error("Expected swap command, got nil")
	}

	// 5. Simulate swap complete
	swapMsg := swapCompleteMsg{err: nil}
	updatedModel5, _ := upM4.Update(swapMsg)
	upM5 := updatedModel5.(updateModel)

	if upM5.state != stateUpFinished {
		t.Errorf("Expected state to become stateUpFinished, got %v", upM5.state)
	}

	if !strings.Contains(upM5.statusMsg, "successful") {
		t.Errorf("Expected successful status message, got: %q", upM5.statusMsg)
	}

	// Test View rendering in finished state
	view := upM5.View()
	if len(view) == 0 {
		t.Error("Expected View to render styled results, got empty string")
	}
}

func TestUpdateModelNoUpdateFound(t *testing.T) {
	m := initialUpdateModel()
	m.currentVersion = "1.0.2" // set current equal to check version

	rel := releaseMetadata{
		TagName:     "v1.0.2",
		PublishedAt: "2026-05-18T12:00:00Z",
		Body:        "Same version details",
		Size:        16 * 1024 * 1024,
	}
	checkMsg := checkCompleteMsg{release: rel, err: nil}
	updatedModel, _ := m.Update(checkMsg)
	upM := updatedModel.(updateModel)

	if upM.updateFound {
		t.Error("Expected updateFound to be false when versions are identical")
	}

	if !strings.Contains(upM.statusMsg, "already on the latest version") {
		t.Errorf("Expected already optimized status message, got: %q", upM.statusMsg)
	}
}

func TestIsNewerVersion(t *testing.T) {
	cases := []struct {
		latest, current string
		want            bool
	}{
		{"1.0.2", "1.0.1", true},
		{"v1.0.2", "1.0.1", true},
		{"1.0.1", "1.0.1", false},
		{"1.0.0", "1.0.1", false},
		{"2.0.0", "1.9.9", true},
		{"1.10.0", "1.9.0", true},
		{"1.0.2-rc1", "1.0.1", true},
		{"1.0.2", "0.0.0", true},
		{"dev", "1.0.1", true}, // unparseable: any difference counts as an update
	}
	for _, c := range cases {
		if got := isNewerVersion(c.latest, c.current); got != c.want {
			t.Errorf("isNewerVersion(%q, %q) = %v, want %v", c.latest, c.current, got, c.want)
		}
	}
}

func TestExpectedChecksumFor(t *testing.T) {
	checksums := []byte(
		"abc123  duster-1.0.2-windows-amd64.zip\n" +
			"def456  duster-1.0.2-windows-arm64.zip\n" +
			"malformed line\n")

	hash, err := expectedChecksumFor(checksums, "duster-1.0.2-windows-amd64.zip")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hash != "abc123" {
		t.Errorf("expected abc123, got %s", hash)
	}

	if _, err := expectedChecksumFor(checksums, "missing.zip"); err == nil {
		t.Error("expected error for missing asset entry, got nil")
	}
}

func TestSelectArchiveAssetRejectsMissingChecksums(t *testing.T) {
	rel := releaseMetadata{
		TagName: "v1.0.2",
		Assets: []releaseAsset{
			{Name: "duster-1.0.2-windows-amd64.zip", DownloadURL: "https://example.com/a.zip"},
		},
	}
	if _, err := selectChecksumsAsset(rel); err == nil {
		t.Error("expected error when release publishes no checksums file, got nil")
	}
}

func TestUpdateModelErrorHandling(t *testing.T) {
	m := initialUpdateModel()

	// Simulate error checking updates
	checkMsg := checkCompleteMsg{err: errors.New("network timeout")}
	updatedModel, _ := m.Update(checkMsg)
	upM := updatedModel.(updateModel)

	if upM.state != stateUpFinished {
		t.Errorf("Expected error to transition to stateUpFinished, got %v", upM.state)
	}

	if !strings.Contains(upM.statusMsg, "Error checking updates") {
		t.Errorf("Expected error status message, got: %q", upM.statusMsg)
	}
}
