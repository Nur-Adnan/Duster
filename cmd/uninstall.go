package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Nur-Adnan/duster/internal/logging"
	"github.com/Nur-Adnan/duster/lib/fs"
	"github.com/Nur-Adnan/duster/lib/uninstall"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

// Protected Whitelist: Applications that are system critical and cannot be uninstalled
var systemProtectedKeywords = []string{
	"microsoft visual c++",
	"nvidia graphics",
	"intel(r) hd graphics",
	"amd radeon",
	"windows driver package",
	"realtek high definition audio",
	"windows update assistant",
	"system updates",
}

// kbUpdatePattern matches Windows update entries like "Update (KB123456)".
// This must be a real regex — as a literal keyword it never matched anything.
var kbUpdatePattern = regexp.MustCompile(`kb\d{6}`)

// Flags
var (
	uninstJSON   bool
	uninstDryRun bool
)

// Premium Lipgloss Styles (Zero-Allocation, prefixed to avoid package conflicts)
var (
	uninstTealColor  = lipgloss.Color("#008080")
	uninstCyanColor  = lipgloss.Color("#00FFFF")
	uninstGrayColor  = lipgloss.Color("#666666")
	uninstWhiteColor = lipgloss.Color("#FFFFFF")
	uninstRedColor   = lipgloss.Color("#FF0000")
	uninstGreenColor = lipgloss.Color("#00FF00")

	uninstHeaderStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(uninstCyanColor).
				Padding(0, 1)

	uninstLeftBoxStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(uninstTealColor).
				Padding(1, 2).
				Width(45)

	uninstRightBoxStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(uninstTealColor).
				Padding(1, 2).
				Width(36)

	uninstFooterStyle = lipgloss.NewStyle().
				Foreground(uninstGrayColor).
				PaddingTop(1).
				PaddingLeft(2)

	uninstDividerStyle = lipgloss.NewStyle().
				Foreground(uninstGrayColor)

	uninstSuccessStyle = lipgloss.NewStyle().Bold(true).Foreground(uninstGreenColor)
	uninstFailStyle    = lipgloss.NewStyle().Bold(true).Foreground(uninstRedColor)
)

var UninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Interactive application uninstaller and remnants clean sweeper",
	Long: `Queries the Windows registry to list installed applications, provides a search-and-select TUI,
triggers native uninstallers, and executes a deep sweep of leftover files in local cache/appdata paths.`,
	Run: executeUninstall,
}

func init() {
	UninstallCmd.Flags().BoolVar(&uninstJSON, "json", false, "Output list of installed applications as a structured JSON snapshot and exit immediately")
	UninstallCmd.Flags().BoolVarP(&uninstDryRun, "dry-run", "d", false, "Simulate uninstallation and leftover scan without writing changes")
}

func executeUninstall(cmd *cobra.Command, args []string) {
	// Piped / JSON snapshop execution
	if uninstJSON || isPiped() {
		runHeadlessUninstall()
		return
	}

	m := initialUninstallModel()
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running uninstaller: %v\n", err)
		os.Exit(1)
	}
}

// Bubble Tea state machine
type uninstallState int

const (
	uninstStateScanning uninstallState = iota
	uninstStateSelecting
	stateConfirmingNative
	stateRunningNative
	stateScanningLeftovers
	stateSelectingLeftovers
	stateConfirmingLeftovers
	stateSweepingLeftovers
	uninstStateFinished
)

type leftoverItem struct {
	Path     string
	Size     int64
	Selected bool
}

type uninstallModel struct {
	state        uninstallState
	apps         []uninstall.InstalledApp
	filteredApps []uninstall.InstalledApp
	leftovers    []leftoverItem
	cursor       int
	scrollOffset int
	searchQuery  string
	selectedApp  uninstall.InstalledApp
	selectedSize int64
	sweepSize    int64
	uninstErr    error
	width        int
	height       int
}

// Messages
type scanAppsCompleteMsg struct {
	apps []uninstall.InstalledApp
	err  error
}

type nativeUninstallDoneMsg struct {
	err error
}

type scanLeftoversCompleteMsg struct {
	items []leftoverItem
}

type sweepCompleteMsg struct {
	reclaimed int64
}

func initialUninstallModel() uninstallModel {
	return uninstallModel{
		state:  uninstStateScanning,
		cursor: 0,
	}
}

func (m uninstallModel) Init() tea.Cmd {
	return scanAppsCmd()
}

func scanAppsCmd() tea.Cmd {
	return func() tea.Msg {
		apps, err := uninstall.GetInstalledApps()
		return scanAppsCompleteMsg{apps: apps, err: err}
	}
}

// runNativeUninstallCmd launches the app's own uninstaller asynchronously.
// It must never run inside Update(): native uninstall wizards take minutes,
// and a blocking Update() freezes the whole TUI on the confirm screen.
func runNativeUninstallCmd(app uninstall.InstalledApp) tea.Cmd {
	return func() tea.Msg {
		var err error
		if !uninstDryRun {
			err = runNativeUninstaller(app.UninstallString)
		}
		return nativeUninstallDoneMsg{err: err}
	}
}

func scanLeftoversCmd(app uninstall.InstalledApp) tea.Cmd {
	return func() tea.Msg {
		folders := scanAppLeftovers(app.Name, app.Publisher)
		var list []leftoverItem
		for _, f := range folders {
			size := calculateDirSize(f)
			list = append(list, leftoverItem{
				Path:     f,
				Size:     size,
				Selected: true,
			})
		}
		return scanLeftoversCompleteMsg{items: list}
	}
}

func runSweepCmd(items []leftoverItem, dry bool) tea.Cmd {
	return func() tea.Msg {
		var reclaimed int64
		for _, item := range items {
			if !item.Selected {
				continue
			}

			var err error
			if !dry {
				err = removeAllSafe(item.Path)
			}

			success := err == nil
			logUninstOperation("sweep", item.Path, item.Size, success)
			if success {
				reclaimed += item.Size
			}
		}
		return sweepCompleteMsg{reclaimed: reclaimed}
	}
}

func (m uninstallModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		key := msg.String()

		// Emergency exit is always available.
		if key == "ctrl+c" {
			return m, tea.Quit
		}

		// Keys are dispatched per state. In the app-selection state nearly every
		// printable character must reach the search filter — the old key-first
		// switch swallowed q/k/j/y/a/space as hotkeys, making apps like
		// "qBittorrent" or "Java" impossible to search for.
		switch m.state {
		case uninstStateScanning:
			if key == "q" || key == "esc" {
				return m, tea.Quit
			}

		case uninstStateSelecting:
			switch key {
			case "esc":
				return m, tea.Quit
			case "up":
				if len(m.filteredApps) > 0 {
					m.cursor--
					if m.cursor < 0 {
						m.cursor = len(m.filteredApps) - 1
					}
					m.adjustScroll(len(m.filteredApps))
				}
			case "down":
				if len(m.filteredApps) > 0 {
					m.cursor++
					if m.cursor >= len(m.filteredApps) {
						m.cursor = 0
					}
					m.adjustScroll(len(m.filteredApps))
				}
			case "backspace":
				if len(m.searchQuery) > 0 {
					m.searchQuery = m.searchQuery[:len(m.searchQuery)-1]
					m.filterApps()
				}
			case "enter":
				if len(m.filteredApps) > 0 {
					app := m.filteredApps[m.cursor]
					if m.isProtected(app.Name) {
						return m, nil // Block system app uninstalls silently
					}
					m.selectedApp = app
					m.state = stateConfirmingNative
				}
			default:
				if len(key) == 1 {
					m.searchQuery += key
					m.filterApps()
				}
			}

		case stateConfirmingNative:
			switch key {
			case "esc", "n", "N", "q":
				m.state = uninstStateSelecting
			case "enter", "y", "Y":
				m.state = stateRunningNative
				return m, runNativeUninstallCmd(m.selectedApp)
			}

		case stateRunningNative, stateScanningLeftovers, stateSweepingLeftovers:
			// External work in flight — ignore input (ctrl+c handled above).

		case stateSelectingLeftovers:
			switch key {
			case "q", "esc":
				return m, tea.Quit
			case "up", "k":
				if len(m.leftovers) > 0 {
					m.cursor--
					if m.cursor < 0 {
						m.cursor = len(m.leftovers) - 1
					}
					m.adjustScroll(len(m.leftovers))
				}
			case "down", "j":
				if len(m.leftovers) > 0 {
					m.cursor++
					if m.cursor >= len(m.leftovers) {
						m.cursor = 0
					}
					m.adjustScroll(len(m.leftovers))
				}
			case " ":
				if len(m.leftovers) > 0 {
					m.leftovers[m.cursor].Selected = !m.leftovers[m.cursor].Selected
					m.recalculateSweep()
				}
			case "a", "A":
				if len(m.leftovers) > 0 {
					anyUnselected := false
					for _, l := range m.leftovers {
						if !l.Selected {
							anyUnselected = true
							break
						}
					}
					for i := range m.leftovers {
						m.leftovers[i].Selected = anyUnselected
					}
					m.recalculateSweep()
				}
			case "enter", "y", "Y":
				if len(m.leftovers) > 0 {
					m.state = stateConfirmingLeftovers
				} else {
					m.state = uninstStateFinished
				}
			}

		case stateConfirmingLeftovers:
			switch key {
			case "esc", "n", "N", "q":
				m.state = stateSelectingLeftovers
			case "enter", "y", "Y":
				m.state = stateSweepingLeftovers
				return m, runSweepCmd(m.leftovers, uninstDryRun)
			}

		case uninstStateFinished:
			switch key {
			case "q", "esc", "enter":
				return m, tea.Quit
			}
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case scanAppsCompleteMsg:
		m.state = uninstStateSelecting
		m.apps = msg.apps
		m.filterApps()
		return m, nil

	case nativeUninstallDoneMsg:
		m.uninstErr = msg.err
		m.state = stateScanningLeftovers
		return m, scanLeftoversCmd(m.selectedApp)

	case scanLeftoversCompleteMsg:
		m.leftovers = msg.items
		m.state = stateSelectingLeftovers
		m.cursor = 0
		m.scrollOffset = 0
		m.recalculateSweep()
		return m, nil

	case sweepCompleteMsg:
		m.selectedSize = msg.reclaimed
		m.state = uninstStateFinished
		return m, nil
	}

	return m, nil
}

func (m *uninstallModel) adjustScroll(size int) {
	maxVisible := 12
	if m.cursor < m.scrollOffset {
		m.scrollOffset = m.cursor
	} else if m.cursor >= m.scrollOffset+maxVisible {
		m.scrollOffset = m.cursor - maxVisible + 1
	}
}

func (m *uninstallModel) filterApps() {
	if m.searchQuery == "" {
		m.filteredApps = m.apps
	} else {
		var list []uninstall.InstalledApp
		q := strings.ToLower(m.searchQuery)
		for _, a := range m.apps {
			if strings.Contains(strings.ToLower(a.Name), q) || strings.Contains(strings.ToLower(a.Publisher), q) {
				list = append(list, a)
			}
		}
		m.filteredApps = list
	}
	m.cursor = 0
	m.scrollOffset = 0
}

func (m *uninstallModel) recalculateSweep() {
	var size int64
	for _, l := range m.leftovers {
		if l.Selected {
			size += l.Size
		}
	}
	m.sweepSize = size
}

func (m uninstallModel) isProtected(name string) bool {
	lowerName := strings.ToLower(name)
	for _, keyword := range systemProtectedKeywords {
		if strings.Contains(lowerName, keyword) {
			return true
		}
	}
	return kbUpdatePattern.MatchString(lowerName)
}

func (m uninstallModel) View() string {
	var doc strings.Builder

	// Top Title
	doc.WriteString("\n")
	doc.WriteString(uninstHeaderStyle.Render("Duster Application Uninstallation Manager"))
	if uninstDryRun {
		doc.WriteString("  |  " + uninstFailStyle.Render("DRY RUN MODE (SIMULATION)"))
	} else {
		doc.WriteString("  |  " + uninstSuccessStyle.Render("LIVE ACTIVE MODE"))
	}
	doc.WriteString("\n")
	doc.WriteString(uninstDividerStyle.Render("  ═══════════════════════════════════════════════════════════════════════\n\n"))

	var boxLayout string

	switch m.state {
	case uninstStateScanning:
		var scanBox strings.Builder
		scanBox.WriteString("🔍  Reading System registries for installed software...\n\n")
		scanBox.WriteString("    Querying local computer configurations...\n")
		scanBox.WriteString("    Gathering publisher and estimation size profiles...")
		boxLayout = uninstLeftBoxStyle.Render(scanBox.String())

	case uninstStateSelecting:
		var leftCol strings.Builder
		var rightCol strings.Builder

		if len(m.filteredApps) == 0 {
			leftCol.WriteString("  ✓ No matching installed programs found!\n\n")
			leftCol.WriteString("  Try typing a different name in the filter bar.")
		} else {
			leftCol.WriteString("Installed applications found:\n\n")
			maxVisible := 12
			endIdx := m.scrollOffset + maxVisible
			if endIdx > len(m.filteredApps) {
				endIdx = len(m.filteredApps)
			}

			for i := m.scrollOffset; i < endIdx; i++ {
				app := m.filteredApps[i]
				prefix := "  "
				if i == m.cursor {
					prefix = "▸ "
				}

				name := app.Name
				if len(name) > 30 {
					name = name[:27] + "..."
				}

				// Check system whitelisting
				var line string
				if m.isProtected(app.Name) {
					line = fmt.Sprintf("%s%s  (Protected OS Service)\n", prefix, uninstGrayText(name))
				} else if i == m.cursor {
					line = fmt.Sprintf("%s%s\n", prefix, uninstCyanText(name))
				} else {
					line = fmt.Sprintf("%s%s\n", prefix, name)
				}
				leftCol.WriteString(line)
			}

			if len(m.filteredApps) > maxVisible {
				leftCol.WriteString(uninstGrayText(fmt.Sprintf("\n  [Row %d of %d]  ─────────────────────────────────", m.cursor+1, len(m.filteredApps))))
			}
		}

		// Search Filter Bar
		leftCol.WriteString("\n\n" + uninstDividerStyle.Render("  ───────────────────────────────────────────\n"))
		leftCol.WriteString(fmt.Sprintf("  Filter: %s", uninstCyanText(m.searchQuery)))
		if len(m.searchQuery) > 0 {
			leftCol.WriteString(" █")
		}

		// Right Column (Sidebar Panel)
		if len(m.filteredApps) > 0 {
			app := m.filteredApps[m.cursor]
			rightCol.WriteString(uninstSuccessStyle.Render("⚙ APPLICATION META INFORMATION\n\n"))
			rightCol.WriteString(fmt.Sprintf("Name      : %s\n", uninstWhiteText(app.Name)))
			rightCol.WriteString(fmt.Sprintf("Publisher : %s\n", uninstWhiteText(app.Publisher)))
			rightCol.WriteString(fmt.Sprintf("Version   : %s\n", uninstWhiteText(app.DisplayVersion)))
			rightCol.WriteString(fmt.Sprintf("Installed : %s\n", uninstWhiteText(app.InstallDate)))
			rightCol.WriteString(fmt.Sprintf("Est. Size : %s\n", uninstWhiteText(formatBytes(app.EstimatedSize))))
			rightCol.WriteString(fmt.Sprintf("Hive      : %s\n\n", uninstWhiteText(app.RegistryHive)))

			rightCol.WriteString(uninstDividerStyle.Render("─────────────────────────────────\n"))
			if m.isProtected(app.Name) {
				rightCol.WriteString(uninstFailStyle.Render("⚠️  SYSTEM CRITICAL RUNTIME\n"))
				rightCol.WriteString("This software is whitelisted. Deletion is disabled to avoid OS instability.")
			} else {
				rightCol.WriteString(uninstSuccessStyle.Render("✓ Action Allowed\n"))
				rightCol.WriteString("Press [Enter] to run the native uninstaller process.")
			}
		} else {
			rightCol.WriteString("No application selected.\n")
		}

		boxLayout = lipgloss.JoinHorizontal(lipgloss.Top,
			uninstLeftBoxStyle.Render(leftCol.String()),
			uninstRightBoxStyle.Render(rightCol.String()),
		)

	case stateConfirmingNative:
		var confBox strings.Builder
		confBox.WriteString("⚠️  " + uninstFailStyle.Render("LAUNCH SYSTEM APP UNINSTALLER") + "\n\n")
		confBox.WriteString(fmt.Sprintf("  You are about to run the uninstaller for: %s\n\n", uninstWhiteText(m.selectedApp.Name)))
		if uninstDryRun {
			confBox.WriteString("  [Dry Run] Simulated launching of: " + m.selectedApp.UninstallString + "\n\n")
		} else {
			confBox.WriteString("  Duster will launch the native software uninstallation command:\n")
			confBox.WriteString(fmt.Sprintf("  %s\n\n", uninstCyanText(m.selectedApp.UninstallString)))
			confBox.WriteString("  Please complete any GUI uninstall wizards that appear.\n\n")
		}
		confBox.WriteString("  Do you wish to launch the uninstaller? [y to Proceed / n to Cancel]")
		boxLayout = uninstLeftBoxStyle.Width(83).Render(confBox.String())

	case stateRunningNative:
		var runBox strings.Builder
		runBox.WriteString("⌛  " + uninstSuccessStyle.Render("WAITING FOR UNINSTALLER TO COMPLETE") + "\n\n")
		runBox.WriteString(fmt.Sprintf("  Executing native uninstaller for: %s\n\n", uninstWhiteText(m.selectedApp.Name)))
		runBox.WriteString("  Standard wizard processes are active. Do NOT interrupt this window.")
		boxLayout = uninstLeftBoxStyle.Width(83).Render(runBox.String())

	case stateScanningLeftovers:
		var scanLeftBox strings.Builder
		scanLeftBox.WriteString("🔍  " + uninstSuccessStyle.Render("SCANNING SYSTEM PATHS FOR APPLICATION LEFTOVERS") + "\n\n")
		scanLeftBox.WriteString("  Scanning roaming registry configs, AppData local, and Program Files\n")
		scanLeftBox.WriteString(fmt.Sprintf("  for files and caches related to %s...\n", uninstWhiteText(m.selectedApp.Name)))
		boxLayout = uninstLeftBoxStyle.Width(83).Render(scanLeftBox.String())

	case stateSelectingLeftovers:
		var leftBox strings.Builder
		if m.uninstErr != nil {
			leftBox.WriteString(uninstFailStyle.Render("⚠ Native uninstaller reported an error: ") + uninstGrayText(m.uninstErr.Error()) + "\n\n")
		}
		leftBox.WriteString("Discovered application folder remnants & local caches:\n\n")

		if len(m.leftovers) == 0 {
			leftBox.WriteString("  ✓ No leftover folder caches or registry remnants identified!\n")
			leftBox.WriteString("  This system was cleaned cleanly by the native uninstaller.\n\n")
			leftBox.WriteString("  Press [Enter] to complete.")
		} else {
			leftBox.WriteString(uninstGrayText("     Target Leftover Path                             Size\n"))
			leftBox.WriteString(uninstDividerStyle.Render("     ───────────────────────────────────────────────────────────────────────\n"))

			maxVisible := 12
			endIdx := m.scrollOffset + maxVisible
			if endIdx > len(m.leftovers) {
				endIdx = len(m.leftovers)
			}

			for i := m.scrollOffset; i < endIdx; i++ {
				item := m.leftovers[i]
				chk := "[ ]"
				if item.Selected {
					chk = "[x]"
				}

				prefix := "  "
				if i == m.cursor {
					prefix = "▸ "
					chk = uninstCyanText(chk)
				}

				shortPath := item.Path
				if len(shortPath) > 50 {
					shortPath = "..." + shortPath[len(shortPath)-47:]
				}

				line := fmt.Sprintf("%s%s  %-52s %10s\n",
					prefix,
					chk,
					shortPath,
					formatBytes(item.Size),
				)

				if i == m.cursor {
					leftBox.WriteString(uninstWhiteText(line))
				} else {
					leftBox.WriteString(line)
				}
			}

			if len(m.leftovers) > maxVisible {
				leftBox.WriteString(uninstGrayText(fmt.Sprintf("\n  [Line %d of %d]  ──────────────────────────────────────────────────────────", m.cursor+1, len(m.leftovers))))
			}

			leftBox.WriteString(fmt.Sprintf("\n\n  Selected Remnants: %s to sweep clean", uninstSuccessStyle.Render(formatBytes(m.sweepSize))))
		}
		boxLayout = uninstLeftBoxStyle.Width(83).Render(leftBox.String())

	case stateConfirmingLeftovers:
		var confSweepBox strings.Builder
		confSweepBox.WriteString("⚠️  " + uninstFailStyle.Render("CONFIRM SYSTEM SWEEP TRANSACTION") + "\n\n")
		confSweepBox.WriteString(fmt.Sprintf("  You are about to permanently destroy %d selected leftovers.\n", countSelectedLeftovers(m.leftovers)))
		confSweepBox.WriteString(fmt.Sprintf("  Total space to reclaim: %s\n\n", uninstSuccessStyle.Render(formatBytes(m.sweepSize))))
		confSweepBox.WriteString("  This operation will bypass the Recycle Bin. Proceed? [y to Sweep / n to Cancel]")
		boxLayout = uninstLeftBoxStyle.Width(83).Render(confSweepBox.String())

	case stateSweepingLeftovers:
		var sweepBox strings.Builder
		sweepBox.WriteString("🔥  " + uninstFailStyle.Render("DESTROYING APPLICATION RESIDUAL FILES") + "\n\n")
		sweepBox.WriteString("  Purging folder trees from AppData and Program Files. Please stand by...")
		boxLayout = uninstLeftBoxStyle.Width(83).Render(sweepBox.String())

	case uninstStateFinished:
		var finBox strings.Builder
		finBox.WriteString("✓  " + uninstSuccessStyle.Render("SYSTEM CLEAN UNINSTALL COMPLETED") + "\n\n")
		finBox.WriteString(fmt.Sprintf("  Application : %s\n", uninstWhiteText(m.selectedApp.Name)))
		if uninstDryRun {
			finBox.WriteString(fmt.Sprintf("  Status      : %s (Simulation only)\n", uninstSuccessStyle.Render("SIMULATED")))
			finBox.WriteString(fmt.Sprintf("  Est Reclaim : %s simulated\n\n", formatBytes(m.selectedSize)))
		} else {
			finBox.WriteString(fmt.Sprintf("  Status      : %s\n", uninstSuccessStyle.Render("UNINSTALLED & SWEPT")))
			finBox.WriteString(fmt.Sprintf("  Reclaimed   : %s of leftovers cleared\n\n", uninstSuccessStyle.Render(formatBytes(m.selectedSize))))
		}
		finBox.WriteString("  Press [q] or [esc] to return to the CLI shell.")
		boxLayout = uninstLeftBoxStyle.Width(83).Render(finBox.String())
	}

	doc.WriteString(boxLayout)
	doc.WriteString("\n")

	// Footer instructions
	switch m.state {
	case uninstStateScanning:
		doc.WriteString(uninstFooterStyle.Render("Querying Windows system libraries... Please wait."))
	case uninstStateSelecting:
		doc.WriteString(uninstFooterStyle.Render("[↑/↓] Scroll  |  [Type] Filter apps  |  [Backspace] Erase  |  [Enter] Pick app  |  [Esc] Quit"))
	case stateConfirmingNative:
		doc.WriteString(uninstFooterStyle.Render("[y] Confirm and Launch uninstaller  |  [n/esc] Cancel and Go Back"))
	case stateRunningNative:
		doc.WriteString(uninstFooterStyle.Render("Complete the native uninstall dialogs that appear in Windows..."))
	case stateScanningLeftovers:
		doc.WriteString(uninstFooterStyle.Render("Searching AppData and Program Files for leftover remnants..."))
	case stateSelectingLeftovers:
		if len(m.leftovers) == 0 {
			doc.WriteString(uninstFooterStyle.Render("[Enter] Proceed"))
		} else {
			doc.WriteString(uninstFooterStyle.Render("[↑/↓/j/k] Scroll  |  [Space] Select  |  [a] Toggle All  |  [Enter] Proceed  |  [q] Quit"))
		}
	case stateConfirmingLeftovers:
		doc.WriteString(uninstFooterStyle.Render("[y] Confirm sweep  |  [n/esc] Cancel"))
	case stateSweepingLeftovers:
		doc.WriteString(uninstFooterStyle.Render("Deleting files... Do NOT interrupt."))
	case uninstStateFinished:
		doc.WriteString(uninstFooterStyle.Render("[q/esc] Exit to Shell"))
	}

	return doc.String()
}

func countSelectedLeftovers(list []leftoverItem) int {
	c := 0
	for _, l := range list {
		if l.Selected {
			c++
		}
	}
	return c
}

// Parses uninstaller command line into executable + args, handling quoted paths correctly
func parseUninstallString(uninstStr string) (string, []string) {
	uninstStr = strings.TrimSpace(uninstStr)
	if uninstStr == "" {
		return "", nil
	}

	var parts []string
	inQuotes := false
	var currentToken strings.Builder

	for i := 0; i < len(uninstStr); i++ {
		char := uninstStr[i]
		if char == '"' {
			inQuotes = !inQuotes
			continue
		}
		if char == ' ' && !inQuotes {
			if currentToken.Len() > 0 {
				parts = append(parts, currentToken.String())
				currentToken.Reset()
			}
			continue
		}
		currentToken.WriteByte(char)
	}
	if currentToken.Len() > 0 {
		parts = append(parts, currentToken.String())
	}

	if len(parts) == 0 {
		return "", nil
	}
	return parts[0], parts[1:]
}

func runNativeUninstaller(uninstStr string) error {
	execName, args := parseUninstallString(uninstStr)
	if execName == "" {
		return fmt.Errorf("empty uninstall string command")
	}

	// Native execution on Windows
	cmd := exec.Command(execName, args...)

	// Set default silent switches if applicable to maximize speed, though many are handled natively
	return cmd.Run()
}

func scanAppLeftovers(appName, publisher string) []string {
	var folders []string
	searchTerms := []string{strings.ToLower(appName)}
	if publisher != "" {
		pubWords := strings.Fields(strings.ToLower(publisher))
		if len(pubWords) > 0 {
			searchTerms = append(searchTerms, pubWords[0])
		}
	}

	for i, term := range searchTerms {
		term = strings.ReplaceAll(term, " llc", "")
		term = strings.ReplaceAll(term, " inc.", "")
		term = strings.ReplaceAll(term, " corporation", "")
		searchTerms[i] = strings.TrimSpace(term)
	}

	// Roaming/Local AppData + standard Program folders
	scanDirs := []string{
		os.Getenv("APPDATA"),
		os.Getenv("LOCALAPPDATA"),
		os.Getenv("ProgramFiles"),
		os.Getenv("ProgramFiles(x86)"),
	}

	var cleanedDirs []string
	seenDirs := make(map[string]bool)
	for _, d := range scanDirs {
		if d != "" {
			if abs, err := filepath.Abs(d); err == nil {
				// Dedupe: env vars can point at the same dir (e.g. ProgramFiles
				// and ProgramFiles(x86) on 32-bit Windows); paths are case-insensitive.
				key := strings.ToLower(abs)
				if !seenDirs[key] {
					seenDirs[key] = true
					cleanedDirs = append(cleanedDirs, abs)
				}
			}
		}
	}

	// Walk top-level subfolders to ensure fast scans
	for _, baseDir := range cleanedDirs {
		entries, err := os.ReadDir(baseDir)
		if err != nil {
			continue
		}

		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}

			entryName := strings.ToLower(entry.Name())
			if entryName == "microsoft" || entryName == "windows" || entryName == "common files" || entryName == "temp" {
				continue
			}

			matched := false
			for _, term := range searchTerms {
				// Terms shorter than 3 chars (an app named "Go", publisher "EA")
				// would substring-match half the filesystem; skip them.
				if len(term) < 3 {
					continue
				}
				if strings.Contains(entryName, term) || (len(entryName) >= 3 && strings.Contains(term, entryName)) {
					matched = true
					break
				}
			}

			if matched {
				fullPath := filepath.Join(baseDir, entry.Name())
				if fs.IsValidPath(fullPath) {
					folders = append(folders, fullPath)
				}
			}
		}
	}

	return folders
}

// logUninstOperation delegates to the shared structured logging system,
// which also rotates the log; the old local copy grew operations.log unbounded.
func logUninstOperation(action, target string, size int64, success bool) {
	logging.LogDestructiveOperation("uninstall", action, target, size, success)
}

func runHeadlessUninstall() {
	apps, err := uninstall.GetInstalledApps()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error scanning applications: %v\n", err)
		os.Exit(1)
	}

	data, err := json.MarshalIndent(apps, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to marshal uninstall data: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(string(data))
}

// Local helper style functions
func uninstCyanText(s string) string {
	return lipgloss.NewStyle().Foreground(uninstCyanColor).Render(s)
}

func uninstWhiteText(s string) string {
	return lipgloss.NewStyle().Foreground(uninstWhiteColor).Render(s)
}

func uninstGrayText(s string) string {
	return lipgloss.NewStyle().Foreground(uninstGrayColor).Render(s)
}
