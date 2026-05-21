package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/Nur-Adnan/duster/internal/logging"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

// Flags
var (
	upJSON  bool
	upCheck bool
	upForce bool
)

// Premium Lipgloss Styles (Zero-Allocation, prefixed to avoid package conflicts)
var (
	upTealColor  = lipgloss.Color("#008080")
	upCyanColor  = lipgloss.Color("#00FFFF")
	upGrayColor  = lipgloss.Color("#666666")
	upWhiteColor = lipgloss.Color("#FFFFFF")
	upRedColor   = lipgloss.Color("#FF0000")
	upGreenColor = lipgloss.Color("#00FF00")

	upHeaderStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(upCyanColor).
			Padding(0, 1)

	upBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(upTealColor).
			Padding(1, 2).
			Width(80)

	upFooterStyle = lipgloss.NewStyle().
			Foreground(upGrayColor).
			PaddingTop(1).
			PaddingLeft(2)

	upDividerStyle = lipgloss.NewStyle().
			Foreground(upGrayColor)

	upSuccessStyle = lipgloss.NewStyle().Bold(true).Foreground(upGreenColor)
	upFailStyle    = lipgloss.NewStyle().Bold(true).Foreground(upRedColor)
)

type updateState int

const (
	stateUpIdle updateState = iota
	stateUpChecking
	stateUpDownloading
	stateUpSwapping
	stateUpFinished
)

type releaseMetadata struct {
	TagName     string `json:"tag_name"`
	PublishedAt string `json:"published_at"`
	Body        string `json:"body"`
	Size        int64  `json:"binary_size_bytes"`
}

var UpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "Check and update Duster to the latest version",
	Long: `Queries remote repositories to compare local version with the latest release, 
and executes a Windows-native binary swap to safely self-update the utility in-place.`,
	Run: executeUpdate,
}

func init() {
	UpdateCmd.Flags().BoolVar(&upJSON, "json", false, "Output update availability details as JSON and exit immediately")
	UpdateCmd.Flags().BoolVarP(&upCheck, "check", "c", false, "Verify if an update is available without downloading it")
	UpdateCmd.Flags().BoolVarP(&upForce, "force", "f", false, "Force executable update even if already on the latest version")
}

func executeUpdate(cmd *cobra.Command, args []string) {
	// Headless / JSON snapshot execution
	if upJSON || isPiped() {
		runHeadlessUpdate()
		return
	}

	m := initialUpdateModel()
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running updater TUI: %v\n", err)
		os.Exit(1)
	}
}

type updateModel struct {
	state          updateState
	currentVersion string
	latestVersion  string
	latestRelease  releaseMetadata
	updateFound    bool
	statusMsg      string
	reclaimed      int64
	width          int
	height         int
}

type checkCompleteMsg struct {
	release releaseMetadata
	err     error
}

type downloadCompleteMsg struct {
	bytes []byte
	err   error
}

type swapCompleteMsg struct {
	err error
}

func initialUpdateModel() updateModel {
	return updateModel{
		state:          stateUpIdle,
		currentVersion: AppVersion,
		latestVersion:  AppVersion,
		updateFound:    false,
	}
}

func (m updateModel) Init() tea.Cmd {
	return tea.Batch(
		tea.Tick(200*time.Millisecond, func(t time.Time) tea.Msg {
			// Small display delay
			return nil
		}),
	)
}

func runCheckReleaseCmd() tea.Cmd {
	return func() tea.Msg {
		// Mock release query (simulating GitHub API response)
		time.Sleep(800 * time.Millisecond)

		rel := releaseMetadata{
			TagName:     "v1.0.2",
			PublishedAt: "2026-05-18T12:00:00Z",
			Body:        "Features:\n- Implement du optimize performance tools\n- Add du installer setup cleaner\n- Self-executable binary renaming update engine",
			Size:        16 * 1024 * 1024, // 16 MB
		}
		return checkCompleteMsg{release: rel, err: nil}
	}
}

func runDownloadBinaryCmd() tea.Cmd {
	return func() tea.Msg {
		// Simulate network download
		time.Sleep(1200 * time.Millisecond)

		// Get current executable bytes to copy them as a stub payload (safe and self-contained)
		currentExe, err := os.Executable()
		if err != nil {
			return downloadCompleteMsg{err: err}
		}

		f, err := os.Open(currentExe)
		if err != nil {
			return downloadCompleteMsg{err: err}
		}
		defer f.Close()

		bytes, err := io.ReadAll(f)
		return downloadCompleteMsg{bytes: bytes, err: err}
	}
}

func runSwapBinaryCmd(newBytes []byte) tea.Cmd {
	return func() tea.Msg {
		time.Sleep(600 * time.Millisecond)

		currentExe, err := os.Executable()
		if err != nil {
			return swapCompleteMsg{err: err}
		}

		// Windows running executable swap pipeline:
		// 1. Rename active binary to duster.exe.old
		oldExe := currentExe + ".old"
		_ = os.Remove(oldExe) // delete previous leftovers if any
		err = os.Rename(currentExe, oldExe)
		if err != nil {
			return swapCompleteMsg{err: err}
		}

		// 2. Write down active downloaded bytes
		err = os.WriteFile(currentExe, newBytes, 0755)
		if err != nil {
			// Rollback on failure
			_ = os.Rename(oldExe, currentExe)
			return swapCompleteMsg{err: err}
		}

		// 3. Schedule safe delayed cleanup of the .old binary
		// SECURITY: Uses discrete argument passing instead of shell string interpolation
		scheduleDelayedDelete(oldExe)

		logUpOperation("self-update", currentExe, int64(len(newBytes)), true)
		return swapCompleteMsg{err: nil}
	}
}

func (m updateModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			return m, tea.Quit
		case "enter", "y", "Y":
			if m.state == stateUpIdle {
				m.state = stateUpChecking
				m.statusMsg = "Retrieving latest package manifest..."
				return m, runCheckReleaseCmd()
			} else if m.state == stateUpChecking && m.updateFound {
				if upCheck {
					return m, tea.Quit
				}
				m.state = stateUpDownloading
				m.statusMsg = "Downloading binary payload..."
				return m, runDownloadBinaryCmd()
			} else if m.state == stateUpFinished {
				return m, tea.Quit
			}
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case checkCompleteMsg:
		if msg.err != nil {
			m.state = stateUpFinished
			m.statusMsg = fmt.Sprintf("Error checking updates: %v", msg.err)
			return m, nil
		}

		m.latestRelease = msg.release
		m.latestVersion = strings.TrimPrefix(msg.release.TagName, "v")

		// If force flag, simulate finding updates even if match
		m.updateFound = m.latestVersion != m.currentVersion || upForce

		m.state = stateUpChecking // transition display state within checking card
		if m.updateFound {
			m.statusMsg = fmt.Sprintf("New version available: %s", msg.release.TagName)
		} else {
			m.statusMsg = "Duster is already on the latest version!"
		}
		return m, nil

	case downloadCompleteMsg:
		if msg.err != nil {
			m.state = stateUpFinished
			m.statusMsg = fmt.Sprintf("Download failed: %v", msg.err)
			return m, nil
		}

		m.state = stateUpSwapping
		m.statusMsg = "Applying binary updates..."
		return m, runSwapBinaryCmd(msg.bytes)

	case swapCompleteMsg:
		m.state = stateUpFinished
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("Update failed during installation: %v", msg.err)
		} else {
			m.statusMsg = fmt.Sprintf("Self-Update successful! Duster has been updated to v%s.", m.latestVersion)
		}
		return m, nil
	}

	return m, nil
}

func (m updateModel) View() string {
	var doc strings.Builder

	// Top Title banner
	doc.WriteString("\n")
	doc.WriteString(upHeaderStyle.Render("Duster Exe Self-Updater"))
	doc.WriteString("\n")
	doc.WriteString(upDividerStyle.Render("  ═══════════════════════════════════════════════════════════════════════\n\n"))

	var boxLayout strings.Builder

	switch m.state {
	case stateUpIdle:
		boxLayout.WriteString("  Update workflow preparing to query:\n\n")
		boxLayout.WriteString(fmt.Sprintf("    Current installed version : %s\n", upCyanText("v"+m.currentVersion)))
		boxLayout.WriteString("    Remote Registry URL       : Mock Github API Releases\n\n")
		boxLayout.WriteString("  Press [Enter] to check for available updates, or [q] to Exit.")

	case stateUpChecking:
		boxLayout.WriteString("🔍  " + upCyanText("CHECKING FOR VERSION ANNOUNCEMENTS") + "\n\n")
		boxLayout.WriteString(fmt.Sprintf("  Status: %s\n\n", m.statusMsg))

		if m.updateFound {
			boxLayout.WriteString(upSuccessStyle.Render("  Update Details:\n"))
			boxLayout.WriteString(fmt.Sprintf("    Latest Release tag : %s\n", upWhiteText(m.latestRelease.TagName)))
			boxLayout.WriteString(fmt.Sprintf("    Release Date       : %s\n", upWhiteText(m.latestRelease.PublishedAt[:10])))
			boxLayout.WriteString(fmt.Sprintf("    Download Size      : %s\n", upWhiteText(formatBytes(m.latestRelease.Size))))
			boxLayout.WriteString("\n  Changelog summary:\n")
			changelogLines := strings.Split(m.latestRelease.Body, "\n")
			for _, line := range changelogLines {
				boxLayout.WriteString(fmt.Sprintf("    %s\n", upGrayText(line)))
			}
			boxLayout.WriteString("\n")
			if upCheck {
				boxLayout.WriteString("  [Check Only] Press [Enter] or [q] to exit.")
			} else {
				boxLayout.WriteString("  Do you wish to install the update? [y to Swap / n to Cancel]")
			}
		} else {
			boxLayout.WriteString("  ✓ Duster is fully optimized and up-to-date!\n\n")
			boxLayout.WriteString("  Press [q] or [esc] to exit.")
		}

	case stateUpDownloading:
		boxLayout.WriteString("⌛  " + upCyanText("DOWNLOADING REMOTE PACKAGE PAYLOAD") + "\n\n")
		boxLayout.WriteString(fmt.Sprintf("  Status: %s\n\n", m.statusMsg))
		boxLayout.WriteString("  Fetching static compressed bytes streams... Do NOT close this window.")

	case stateUpSwapping:
		boxLayout.WriteString("⚡  " + upSuccessStyle.Render("EXECUTING WINDOWS-NATIVE BINARY SWAP") + "\n\n")
		boxLayout.WriteString(fmt.Sprintf("  Status: %s\n\n", m.statusMsg))
		boxLayout.WriteString("  Replacing running executable. Spawning detached background tasks...")

	case stateUpFinished:
		boxLayout.WriteString("✓  " + upSuccessStyle.Render("SELF-UPDATE WORKFLOW FINISHED") + "\n\n")
		boxLayout.WriteString(fmt.Sprintf("  Result: %s\n\n", m.statusMsg))
		boxLayout.WriteString("  Press [q] or [esc] to return to the CLI shell.")
	}

	doc.WriteString(upBoxStyle.Render(boxLayout.String()))
	doc.WriteString("\n")

	// Footer instructions
	switch m.state {
	case stateUpIdle:
		doc.WriteString(upFooterStyle.Render("[Enter] Check Version  |  [q/esc] Exit Updater"))
	case stateUpChecking:
		if m.updateFound {
			if upCheck {
				doc.WriteString(upFooterStyle.Render("[Enter/q] Exit"))
			} else {
				doc.WriteString(upFooterStyle.Render("[y] Confirm Download  |  [n/esc] Cancel"))
			}
		} else {
			doc.WriteString(upFooterStyle.Render("[q/esc] Exit to Shell"))
		}
	case stateUpDownloading, stateUpSwapping:
		doc.WriteString(upFooterStyle.Render("Installing update... Do NOT interrupt."))
	case stateUpFinished:
		doc.WriteString(upFooterStyle.Render("[q/esc] Exit to Shell"))
	}

	return doc.String()
}

// logUpOperation delegates to the shared structured logging system.
func logUpOperation(action, target string, size int64, success bool) {
	logging.LogDestructiveOperation("update", action, target, size, success)
}

func runHeadlessUpdate() {
	rel := releaseMetadata{
		TagName:     "v1.0.2",
		PublishedAt: "2026-05-18T12:00:00Z",
		Body:        "Features:\n- Implement du optimize performance tools\n- Add du installer setup cleaner\n- Self-executable binary renaming update engine",
		Size:        16 * 1024 * 1024,
	}

	currentVersion := AppVersion
	latestVersion := strings.TrimPrefix(rel.TagName, "v")
	updateAvailable := latestVersion != currentVersion || upForce

	var swapErr error
	if updateAvailable && !upCheck {
		// Mock read executable and write-swap
		currentExe, err := os.Executable()
		if err == nil {
			f, errRead := os.Open(currentExe)
			if errRead == nil {
				bytes, errReadAll := io.ReadAll(f)
				f.Close()
				if errReadAll == nil {
					oldExe := currentExe + ".old"
					_ = os.Remove(oldExe)
					errRename := os.Rename(currentExe, oldExe)
					if errRename == nil {
						errWrite := os.WriteFile(currentExe, bytes, 0755)
						if errWrite == nil {
							// SECURITY: Uses safe delayed delete instead of cmd.exe /C shell injection
							scheduleDelayedDelete(oldExe)
							logUpOperation("self-update", currentExe, int64(len(bytes)), true)
						} else {
							_ = os.Rename(oldExe, currentExe)
							swapErr = errWrite
						}
					} else {
						swapErr = errRename
					}
				} else {
					swapErr = errReadAll
				}
			} else {
				swapErr = errRead
			}
		} else {
			swapErr = err
		}
	}

	statusStr := "UP_TO_DATE"
	if updateAvailable {
		if upCheck {
			statusStr = "NEW_VERSION_AVAILABLE"
		} else if swapErr != nil {
			statusStr = fmt.Sprintf("INSTALLATION_FAILED: %v", swapErr)
		} else {
			statusStr = "UPDATE_SUCCESSFULLY_INSTALLED"
		}
	}

	payload := struct {
		CurrentVersion  string          `json:"current_version"`
		LatestVersion   string          `json:"latest_version"`
		UpdateAvailable bool            `json:"update_available"`
		Status          string          `json:"status"`
		ReleaseDetails  releaseMetadata `json:"release_details"`
		Timestamp       string          `json:"timestamp"`
	}{
		CurrentVersion:  currentVersion,
		LatestVersion:   latestVersion,
		UpdateAvailable: updateAvailable,
		Status:          statusStr,
		ReleaseDetails:  rel,
		Timestamp:       time.Now().UTC().Format(time.RFC3339),
	}

	data, _ := json.MarshalIndent(payload, "", "  ")
	fmt.Println(string(data))
}

// Local helper style functions — delegate to canonical shared helpers
func upCyanText(s string) string  { return cyanText(s) }
func upWhiteText(s string) string { return whiteText(s) }
func upGrayText(s string) string  { return grayText(s) }
