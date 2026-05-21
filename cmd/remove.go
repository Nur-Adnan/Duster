package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/Nur-Adnan/duster/internal/logging"
	"github.com/Nur-Adnan/duster/lib/fs"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

// Flags for RemoveCmd
var (
	rmForce  bool
	rmDryRun bool
	rmJSON   bool
)

// Premium Lipgloss Styles (Zero-Allocation, prefixed for remove)
var (
	rmTealColor  = lipgloss.Color("#008080")
	rmCyanColor  = lipgloss.Color("#00FFFF")
	rmGrayColor  = lipgloss.Color("#666666")
	rmWhiteColor = lipgloss.Color("#FFFFFF")
	rmRedColor   = lipgloss.Color("#FF0000")
	rmGreenColor = lipgloss.Color("#00FF00")

	rmHeaderStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(rmRedColor).
			Padding(0, 1)

	rmBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(rmRedColor).
			Padding(1, 2).
			Width(80)

	rmFooterStyle = lipgloss.NewStyle().
			Foreground(rmGrayColor).
			PaddingTop(1).
			PaddingLeft(2)

	rmDividerStyle = lipgloss.NewStyle().
			Foreground(rmGrayColor)

	rmSuccessStyle = lipgloss.NewStyle().Bold(true).Foreground(rmGreenColor)
	rmFailStyle    = lipgloss.NewStyle().Bold(true).Foreground(rmRedColor)
)

type removeState int

const (
	stateRmIdle removeState = iota
	stateRmUninstalling
	stateRmFinished
)

var RemoveCmd = &cobra.Command{
	Use:   "remove",
	Short: "Uninstall Duster and delete all configuration files and logs",
	Long: `Cleans all system traces of Duster, deletes the application directory containing 
configuration files and operations logs, and schedules a self-deletion script for the Duster binary itself.`,
	Run: executeRemove,
}

func init() {
	RemoveCmd.Flags().BoolVar(&rmJSON, "json", false, "Output uninstallation plan as JSON and exit immediately")
	RemoveCmd.Flags().BoolVarP(&rmForce, "force", "f", false, "Uninstall silently without rendering confirmation UI")
	RemoveCmd.Flags().BoolVarP(&rmDryRun, "dry-run", "d", false, "Simulate uninstallation to inspect what files would be removed")
}

func executeRemove(cmd *cobra.Command, args []string) {
	currentExe, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Unable to locate active executable: %v\n", err)
		os.Exit(1)
	}

	// Safety check: protect system directories
	if fs.IsSystemProtectedPath(currentExe) {
		fmt.Fprintf(os.Stderr, "Error: Duster is installed in a system-protected path (%s). Self-uninstallation is blocked for safety.\n", currentExe)
		os.Exit(1)
	}

	// Headless JSON execution
	if rmJSON || isPiped() {
		runHeadlessRemove(currentExe)
		return
	}

	// Force/silent execution
	if rmForce {
		runSilentRemove(currentExe)
		return
	}

	m := initialRemoveModel(currentExe)
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running uninstaller TUI: %v\n", err)
		os.Exit(1)
	}
}

type removeModel struct {
	state      removeState
	currentExe string
	logDir     string
	statusMsg  string
	err        error
	width      int
	height     int
}

type rmUninstallCompleteMsg struct {
	err error
}

func initialRemoveModel(currentExe string) removeModel {
	return removeModel{
		state:      stateRmIdle,
		currentExe: currentExe,
		logDir:     getDusterDir(),
	}
}

func (m removeModel) Init() tea.Cmd {
	return nil
}

func runUninstallCmd(currentExe, logDir string, dryRun bool) tea.Cmd {
	return func() tea.Msg {
		// Log the uninstallation transaction first before removing the folder
		logRmOperation("self-uninstall", currentExe, 0, true)

		time.Sleep(1000 * time.Millisecond)

		// 1. Safe purge configuration directory
		if err := cleanDusterDir(logDir, dryRun); err != nil {
			return rmUninstallCompleteMsg{err: err}
		}

		// 2. Schedule safe delayed binary self-deletion
		// SECURITY: Uses discrete argument passing instead of cmd.exe /C shell injection
		if !dryRun {
			scheduleDelayedDelete(currentExe)
		}

		return rmUninstallCompleteMsg{err: nil}
	}
}

func (m removeModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c", "esc", "n", "N":
			if m.state == stateRmIdle {
				return m, tea.Quit
			}
		case "y", "Y", "enter":
			if m.state == stateRmIdle {
				m.state = stateRmUninstalling
				m.statusMsg = "Purging local assets and logs..."
				return m, runUninstallCmd(m.currentExe, m.logDir, rmDryRun)
			} else if m.state == stateRmFinished {
				return m, tea.Quit
			}
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case rmUninstallCompleteMsg:
		m.state = stateRmFinished
		if msg.err != nil {
			m.err = msg.err
			m.statusMsg = fmt.Sprintf("Uninstallation encountered errors: %v", msg.err)
		} else {
			if rmDryRun {
				m.statusMsg = "Dry-run uninstallation completed successfully! (No files deleted)"
			} else {
				m.statusMsg = "Duster has been completely uninstalled. This window will now exit."
				return m, tea.Sequence(
					tea.Tick(800*time.Millisecond, func(t time.Time) tea.Msg {
						os.Exit(0)
						return nil
					}),
					tea.Quit,
				)
			}
		}
		return m, nil
	}

	return m, nil
}

func (m removeModel) View() string {
	var doc strings.Builder

	doc.WriteString("\n")
	doc.WriteString(rmHeaderStyle.Render("Duster Uninstaller & Cleanup"))
	doc.WriteString("\n")
	doc.WriteString(rmDividerStyle.Render("  ═══════════════════════════════════════════════════════════════════════\n\n"))

	var boxLayout strings.Builder

	switch m.state {
	case stateRmIdle:
		boxLayout.WriteString("⚠️  " + rmFailStyle.Render("WARNING: YOU ARE ABOUT TO COMPLETELY REMOVE DUSTER") + "\n\n")
		boxLayout.WriteString("  This action will permanently delete:\n")
		boxLayout.WriteString(fmt.Sprintf("    • Running binary executable: %s\n", rmWhiteText(m.currentExe)))
		boxLayout.WriteString(fmt.Sprintf("    • Local configuration files: %s\n", rmWhiteText(m.logDir)))
		boxLayout.WriteString("    • Operational logs and transaction history\n\n")
		if rmDryRun {
			boxLayout.WriteString(rmSuccessStyle.Render("  [DRY-RUN SIMULATION ACTIVE] — No bytes will actually be deleted.\n\n"))
		}
		boxLayout.WriteString("  Are you sure you want to proceed? [y to Confirm / n to Cancel]")

	case stateRmUninstalling:
		boxLayout.WriteString("🧹  " + rmCyanText("CLEANING SYSTEM TRACES") + "\n\n")
		boxLayout.WriteString(fmt.Sprintf("  Status: %s\n\n", m.statusMsg))
		boxLayout.WriteString("  Deauthorizing background telemetry and purging cache folders...")

	case stateRmFinished:
		if m.err != nil {
			boxLayout.WriteString("❌  " + rmFailStyle.Render("UNINSTALLATION FAILED") + "\n\n")
			boxLayout.WriteString(fmt.Sprintf("  Error: %s\n\n", m.statusMsg))
		} else {
			boxLayout.WriteString("✓  " + rmSuccessStyle.Render("DUSTER SUCCESSFULLY UNINSTALLED") + "\n\n")
			boxLayout.WriteString(fmt.Sprintf("  Result: %s\n\n", m.statusMsg))
		}
		boxLayout.WriteString("  Press [q] or [Enter] to exit.")
	}

	doc.WriteString(rmBoxStyle.Render(boxLayout.String()))
	doc.WriteString("\n")

	// Footer instructions
	switch m.state {
	case stateRmIdle:
		doc.WriteString(rmFooterStyle.Render("[y] Confirm Uninstallation  |  [n/esc] Cancel & Return"))
	case stateRmUninstalling:
		doc.WriteString(rmFooterStyle.Render("Deauthorizing files... Please stand by."))
	case stateRmFinished:
		doc.WriteString(rmFooterStyle.Render("[q/esc] Exit to Console"))
	}

	return doc.String()
}

func getDusterDir() string {
	var logDir string
	if runtime.GOOS == "windows" {
		logDir = os.Getenv("LOCALAPPDATA")
		if logDir == "" {
			logDir = os.Getenv("USERPROFILE")
		}
		if logDir != "" {
			logDir = filepath.Join(logDir, "Duster")
		}
	} else {
		logDir = filepath.Clean("./")
	}
	if logDir == "" {
		logDir = filepath.Clean("./")
	}
	return logDir
}

func cleanDusterDir(logDir string, simulate bool) error {
	if logDir == "" || logDir == "." || logDir == "/" || len(logDir) <= 3 {
		return fmt.Errorf("unsafe configuration path blocked: %s", logDir)
	}
	if simulate {
		return nil
	}
	return removeAllSafe(logDir)
}

// logRmOperation delegates to the shared structured logging system.
func logRmOperation(action, target string, size int64, success bool) {
	logging.LogDestructiveOperation("remove", action, target, size, success)
}

func runSilentRemove(currentExe string) {
	logDir := getDusterDir()
	logRmOperation("silent-uninstall", currentExe, 0, true)

	_ = cleanDusterDir(logDir, rmDryRun)

	if !rmDryRun {
		// SECURITY: Uses safe delayed delete instead of cmd.exe /C shell injection
		scheduleDelayedDelete(currentExe)
		os.Exit(0)
	} else {
		fmt.Println("Dry-run uninstallation complete. (No files deleted)")
	}
}

func runHeadlessRemove(currentExe string) {
	logDir := getDusterDir()
	logRmOperation("headless-uninstall", currentExe, 0, true)

	var err error
	if !rmDryRun {
		err = cleanDusterDir(logDir, false)
		if err == nil {
			// SECURITY: Uses safe delayed delete instead of cmd.exe /C shell injection
			scheduleDelayedDelete(currentExe)
		}
	}

	statusStr := "SUCCESS"
	if err != nil {
		statusStr = fmt.Sprintf("FAILED: %v", err)
	} else if rmDryRun {
		statusStr = "DRY_RUN_SUCCESS"
	}

	payload := struct {
		ExecutablePath      string `json:"executable_path"`
		ConfigurationFolder string `json:"configuration_folder"`
		Status              string `json:"status"`
		DryRun              bool   `json:"dry_run"`
		Timestamp           string `json:"timestamp"`
	}{
		ExecutablePath:      currentExe,
		ConfigurationFolder: logDir,
		Status:              statusStr,
		DryRun:              rmDryRun,
		Timestamp:           time.Now().UTC().Format(time.RFC3339),
	}

	data, _ := json.MarshalIndent(payload, "", "  ")
	fmt.Println(string(data))

	if !rmDryRun && err == nil {
		os.Exit(0)
	}
}

func rmCyanText(s string) string {
	return lipgloss.NewStyle().Foreground(rmCyanColor).Render(s)
}

func rmWhiteText(s string) string {
	return lipgloss.NewStyle().Foreground(rmWhiteColor).Render(s)
}
