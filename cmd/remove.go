package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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

// rmExitMsg fires after the final success screen has been visible briefly.
type rmExitMsg struct{}

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
		case "ctrl+c":
			// Emergency exit is always available, even mid-uninstall.
			return m, tea.Quit
		case "q", "esc", "n", "N":
			// Quit from the confirm screen (cancel) or the finished screen
			// (both advertise [q]/[esc]); ignored only while work is in flight.
			if m.state == stateRmIdle || m.state == stateRmFinished {
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
				// Never os.Exit inside a running Bubble Tea program: it skips
				// terminal restore and leaves the console in alt-screen raw mode.
				return m, tea.Tick(800*time.Millisecond, func(time.Time) tea.Msg {
					return rmExitMsg{}
				})
			}
		}
		return m, nil

	case rmExitMsg:
		return m, tea.Quit
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
	logDir := os.Getenv("LOCALAPPDATA")
	if logDir == "" {
		logDir = os.Getenv("USERPROFILE")
	}
	if logDir != "" {
		logDir = filepath.Join(logDir, "Duster")
	}
	if logDir == "" {
		logDir = filepath.Clean("./")
	}
	return logDir
}

func cleanDusterDir(logDir string, simulate bool) error {
	if logDir == "" || logDir == "." || logDir == "/" {
		return fmt.Errorf("unsafe configuration path blocked: %s", logDir)
	}
	cleaned := filepath.Clean(logDir)
	if fs.IsSystemProtectedPath(cleaned) {
		return fmt.Errorf("system-protected path blocked: %s", cleaned)
	}
	if simulate {
		return nil
	}
	return removeAllSafe(cleaned)
}

// logRmOperation delegates to the shared structured logging system.
func logRmOperation(action, target string, size int64, success bool) {
	logging.LogDestructiveOperation("remove", action, target, size, success)
}

func runSilentRemove(currentExe string) {
	logDir := getDusterDir()

	err := cleanDusterDir(logDir, rmDryRun)
	logRmOperation("silent-uninstall", currentExe, 0, err == nil)

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

	// The --json / piped path emits a plan by default and must NOT delete unless
	// the caller explicitly opts in with --force. This matches the flag's
	// documented "plan" semantics and prevents `du remove | tee log` — where the
	// interactive confirmation is bypassed — from silently uninstalling.
	performDelete := rmForce && !rmDryRun

	var err error
	if performDelete {
		err = cleanDusterDir(logDir, false)
		if err == nil {
			// SECURITY: Uses safe delayed delete instead of cmd.exe /C shell injection
			scheduleDelayedDelete(currentExe)
		}
	}
	// Log after the work with the real outcome (a success logged up front would
	// also be written into the directory we are about to delete).
	logRmOperation("headless-uninstall", currentExe, 0, err == nil)

	statusStr := "SUCCESS"
	if err != nil {
		statusStr = fmt.Sprintf("FAILED: %v", err)
	} else if !performDelete {
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

	data, jsonErr := json.MarshalIndent(payload, "", "  ")
	if jsonErr != nil {
		fmt.Fprintf(os.Stderr, "Error encoding JSON: %v\n", jsonErr)
		os.Exit(1)
	}
	fmt.Println(string(data))

	if err != nil {
		os.Exit(1) // scripts must be able to detect a failed uninstall
	}
	if !rmDryRun {
		os.Exit(0)
	}
}

func rmCyanText(s string) string {
	return lipgloss.NewStyle().Foreground(rmCyanColor).Render(s)
}

func rmWhiteText(s string) string {
	return lipgloss.NewStyle().Foreground(rmWhiteColor).Render(s)
}
