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

// Flags
var (
	instJSON    bool
	instDryRun  bool
	instMinSize int64 // Min size in MB
)

// Premium Lipgloss Styles (Zero-Allocation, prefixed to avoid package conflicts)
var (
	instTealColor  = lipgloss.Color("#008080")
	instCyanColor  = lipgloss.Color("#00FFFF")
	instGrayColor  = lipgloss.Color("#666666")
	instWhiteColor = lipgloss.Color("#FFFFFF")
	instRedColor   = lipgloss.Color("#FF0000")
	instGreenColor = lipgloss.Color("#00FF00")

	instHeaderStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(instCyanColor).
			Padding(0, 1)

	instLeftBoxStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(instTealColor).
				Padding(1, 2).
				Width(83)

	instFooterStyle = lipgloss.NewStyle().
			Foreground(instGrayColor).
			PaddingTop(1).
			PaddingLeft(2)

	instDividerStyle = lipgloss.NewStyle().
				Foreground(instGrayColor)

	instSuccessStyle = lipgloss.NewStyle().Bold(true).Foreground(instGreenColor)
	instFailStyle    = lipgloss.NewStyle().Bold(true).Foreground(instRedColor)
)

type installerItem struct {
	Path     string    `json:"path"`
	Name     string    `json:"name"`
	Size     int64     `json:"size_bytes"`
	AgeDays  int       `json:"age_days"`
	ModTime  time.Time `json:"modified_time"`
	Selected bool      `json:"selected"`
}

type installerState int

const (
	instStateScanning installerState = iota
	instStateSelecting
	instStateConfirming
	instStateSweeping
	instStateFinished
)

var InstallerCmd = &cobra.Command{
	Use:   "installer",
	Short: "Find and remove large installer files",
	Long: `Recursively scans the local user Downloads folder and temp download caches 
for bulky installer setups (.exe, .msi, .msix, .appx, .pkg) older than 7 days, 
providing an interactive checkbox TUI to sweep them and reclaim space.`,
	Run: executeInstaller,
}

func init() {
	InstallerCmd.Flags().BoolVar(&instJSON, "json", false, "Output list of discovered outdated setups as JSON and exit immediately")
	InstallerCmd.Flags().BoolVarP(&instDryRun, "dry-run", "d", false, "Simulate setup sweeps without deleting any files")
	InstallerCmd.Flags().Int64Var(&instMinSize, "min-size", 50, "Minimum installer file size in megabytes (MB) to filter")
}

func executeInstaller(cmd *cobra.Command, args []string) {
	// Headless / JSON execution
	if instJSON || isPiped() {
		runHeadlessInstaller()
		return
	}

	m := initialInstallerModel()
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running installer TUI: %v\n", err)
		os.Exit(1)
	}
}

type installerModel struct {
	state        installerState
	items        []installerItem
	cursor       int
	scrollOffset int
	sweepSize    int64
	reclaimed    int64
	width        int
	height       int
}

type installerScanCompleteMsg struct {
	items []installerItem
}

type setupSweepCompleteMsg struct {
	reclaimed int64
}

func initialInstallerModel() installerModel {
	return installerModel{
		state:        instStateScanning,
		cursor:       0,
		scrollOffset: 0,
	}
}

func (m installerModel) Init() tea.Cmd {
	return scanInstallersCmd(instMinSize)
}

// scanInstallerItems walks the user's Downloads folder for bulky, week-old
// installer files. Shared by the TUI scan command and the headless JSON path.
func scanInstallerItems(minSizeMB int64) []installerItem {
	list := []installerItem{} // non-nil so headless JSON renders [] instead of null
	minSizeBytes := minSizeMB * 1024 * 1024
	now := time.Now()

	userProfile := os.Getenv("USERPROFILE")
	if userProfile == "" {
		userProfile = `C:\Users\Default`
	}
	downloadsDir := filepath.Join(userProfile, "Downloads")

	_ = filepath.WalkDir(downloadsDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}

		if d.IsDir() {
			// Don't recurse into hidden or config subfolders to speed up walk
			if strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return nil
		}

		// Validate size
		if info.Size() < minSizeBytes {
			return nil
		}

		// Validate age (must be older than 7 days)
		age := now.Sub(info.ModTime())
		ageDays := int(age.Hours() / 24)
		if ageDays < 7 {
			return nil
		}

		// Validate extension
		ext := strings.ToLower(filepath.Ext(d.Name()))
		if ext == ".exe" || ext == ".msi" || ext == ".msix" || ext == ".appx" || ext == ".pkg" {
			if fs.IsValidPath(path) {
				list = append(list, installerItem{
					Path:     path,
					Name:     d.Name(),
					Size:     info.Size(),
					AgeDays:  ageDays,
					ModTime:  info.ModTime(),
					Selected: true,
				})
			}
		}
		return nil
	})

	return list
}

func scanInstallersCmd(minSizeMB int64) tea.Cmd {
	return func() tea.Msg {
		return installerScanCompleteMsg{items: scanInstallerItems(minSizeMB)}
	}
}

func runSetupSweepCmd(items []installerItem, dry bool) tea.Cmd {
	return func() tea.Msg {
		var reclaimed int64
		for _, item := range items {
			if !item.Selected {
				continue
			}

			var err error
			if !dry {
				err = removeFileSafe(item.Path)
			}

			success := err == nil
			logInstOperation("sweep", item.Path, item.Size, success)
			if success {
				reclaimed += item.Size
			}
		}
		return setupSweepCompleteMsg{reclaimed: reclaimed}
	}
}

func (m installerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit

		case "q":
			if m.state == instStateSweeping {
				return m, nil // deletions in flight
			}
			if m.state == instStateConfirming {
				m.state = instStateSelecting
				return m, nil
			}
			return m, tea.Quit

		case "esc", "n", "N":
			if m.state == instStateConfirming {
				m.state = instStateSelecting
				return m, nil
			}
			if msg.String() == "esc" {
				if m.state == instStateSweeping {
					return m, nil
				}
				return m, tea.Quit
			}

		case "up", "k":
			if m.state == instStateSelecting && len(m.items) > 0 {
				m.cursor--
				if m.cursor < 0 {
					m.cursor = len(m.items) - 1
				}
				m.adjustScroll()
			}

		case "down", "j":
			if m.state == instStateSelecting && len(m.items) > 0 {
				m.cursor++
				if m.cursor >= len(m.items) {
					m.cursor = 0
				}
				m.adjustScroll()
			}

		case " ":
			if m.state == instStateSelecting && len(m.items) > 0 {
				m.items[m.cursor].Selected = !m.items[m.cursor].Selected
				m.recalculateSweep()
			}

		case "a", "A":
			if m.state == instStateSelecting && len(m.items) > 0 {
				anyUnselected := false
				for _, it := range m.items {
					if !it.Selected {
						anyUnselected = true
						break
					}
				}
				for i := range m.items {
					m.items[i].Selected = anyUnselected
				}
				m.recalculateSweep()
			}

		case "enter", "y", "Y":
			if m.state == instStateSelecting {
				if len(m.items) > 0 && m.sweepSize > 0 {
					m.state = instStateConfirming
				}
			} else if m.state == instStateConfirming {
				m.state = instStateSweeping
				return m, runSetupSweepCmd(m.items, instDryRun)
			} else if m.state == instStateFinished {
				return m, tea.Quit
			}
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case installerScanCompleteMsg:
		m.items = msg.items
		m.state = instStateSelecting
		m.cursor = 0
		m.scrollOffset = 0
		m.recalculateSweep()
		return m, nil

	case setupSweepCompleteMsg:
		m.reclaimed = msg.reclaimed
		m.state = instStateFinished
		return m, nil
	}

	return m, nil
}

func (m *installerModel) adjustScroll() {
	maxVisible := 12
	if m.cursor < m.scrollOffset {
		m.scrollOffset = m.cursor
	} else if m.cursor >= m.scrollOffset+maxVisible {
		m.scrollOffset = m.cursor - maxVisible + 1
	}
}

func (m *installerModel) recalculateSweep() {
	var size int64
	for _, it := range m.items {
		if it.Selected {
			size += it.Size
		}
	}
	m.sweepSize = size
}

func (m installerModel) View() string {
	var doc strings.Builder

	// Top Title
	doc.WriteString("\n")
	doc.WriteString(instHeaderStyle.Render("Duster Bulky Installer Cleaner"))
	if instDryRun {
		doc.WriteString("  |  " + instFailStyle.Render("DRY RUN MODE (SIMULATION)"))
	} else {
		doc.WriteString("  |  " + instSuccessStyle.Render("LIVE ACTIVE MODE"))
	}
	doc.WriteString("\n")
	doc.WriteString(instDividerStyle.Render("  ═══════════════════════════════════════════════════════════════════════\n\n"))

	var boxLayout string

	switch m.state {
	case instStateScanning:
		var scanBox strings.Builder
		scanBox.WriteString("🔍  Scanning user Downloads and cache directories...\n\n")
		scanBox.WriteString(fmt.Sprintf("    Analyzing files larger than %d MB...\n", instMinSize))
		scanBox.WriteString("    Filtering installer setups older than 7 days...")
		boxLayout = instLeftBoxStyle.Render(scanBox.String())

	case instStateSelecting:
		var leftBox strings.Builder
		if len(m.items) == 0 {
			leftBox.WriteString("  ✓ No bulky outdated setup installers identified!\n")
			leftBox.WriteString("  Your Downloads directories are clean and optimized.\n\n")
			leftBox.WriteString("  Press [q] to exit.")
		} else {
			leftBox.WriteString(instWhiteText("Discovered bulky outdated installers (older than 7 days):\n\n"))
			leftBox.WriteString(instGrayText("     Target Setup Name                             Size            Age\n"))
			leftBox.WriteString(instDividerStyle.Render("     ───────────────────────────────────────────────────────────────────────\n"))

			maxVisible := 12
			endIdx := m.scrollOffset + maxVisible
			if endIdx > len(m.items) {
				endIdx = len(m.items)
			}

			for i := m.scrollOffset; i < endIdx; i++ {
				item := m.items[i]
				chk := "[ ]"
				if item.Selected {
					chk = "[x]"
				}

				prefix := "  "
				if i == m.cursor {
					prefix = "▸ "
					chk = instCyanText(chk)
				}

				shortName := item.Name
				if len(shortName) > 42 {
					shortName = shortName[:39] + "..."
				}

				line := fmt.Sprintf("%s%s  %-44s %12s  %4d days\n",
					prefix,
					chk,
					shortName,
					formatBytes(item.Size),
					item.AgeDays,
				)

				if i == m.cursor {
					leftBox.WriteString(instWhiteText(line))
				} else {
					leftBox.WriteString(line)
				}
			}

			if len(m.items) > maxVisible {
				leftBox.WriteString(instGrayText(fmt.Sprintf("\n  [Line %d of %d]  ──────────────────────────────────────────────────────────", m.cursor+1, len(m.items))))
			}

			leftBox.WriteString(fmt.Sprintf("\n\n  Selected Setup Files: %s to purge", instSuccessStyle.Render(formatBytes(m.sweepSize))))
		}
		boxLayout = instLeftBoxStyle.Render(leftBox.String())

	case instStateConfirming:
		var confBox strings.Builder
		confBox.WriteString("⚠️  " + instFailStyle.Render("CONFIRM SETUPS PURGE WORKFLOW") + "\n\n")
		confBox.WriteString(fmt.Sprintf("  You are about to permanently delete %d selected setup files.\n", countSelectedInstallers(m.items)))
		confBox.WriteString(fmt.Sprintf("  Total space to reclaim: %s\n\n", instSuccessStyle.Render(formatBytes(m.sweepSize))))
		confBox.WriteString("  This operation will bypass the Recycle Bin. Proceed? [y to Deconstruct / n to Go Back]")
		boxLayout = instLeftBoxStyle.Render(confBox.String())

	case instStateSweeping:
		var sweepBox strings.Builder
		sweepBox.WriteString("🔥  " + instFailStyle.Render("DESTROYING BULKY INSTALLERS") + "\n\n")
		sweepBox.WriteString("  Purging setup packages from local storage. Please stand by...")
		boxLayout = instLeftBoxStyle.Render(sweepBox.String())

	case instStateFinished:
		var finBox strings.Builder
		finBox.WriteString("✓  " + instSuccessStyle.Render("INSTALLER SWEEP TRANSACTION COMPLETED") + "\n\n")
		if instDryRun {
			finBox.WriteString(fmt.Sprintf("  Status      : %s (Simulation only)\n", instSuccessStyle.Render("SIMULATED")))
			finBox.WriteString(fmt.Sprintf("  Est Reclaim : %s simulated\n\n", formatBytes(m.reclaimed)))
		} else {
			finBox.WriteString(fmt.Sprintf("  Status      : %s\n", instSuccessStyle.Render("SWEPT CLEAN")))
			finBox.WriteString(fmt.Sprintf("  Reclaimed   : %s reclaimed successfully\n\n", instSuccessStyle.Render(formatBytes(m.reclaimed))))
		}
		finBox.WriteString("  Press [q] or [esc] to return to the CLI shell.")
		boxLayout = instLeftBoxStyle.Render(finBox.String())
	}

	doc.WriteString(boxLayout)
	doc.WriteString("\n")

	// Footer instructions
	switch m.state {
	case instStateScanning:
		doc.WriteString(instFooterStyle.Render("Crawling local system Downloads directories... Please wait."))
	case instStateSelecting:
		if len(m.items) == 0 {
			doc.WriteString(instFooterStyle.Render("[q] Exit"))
		} else {
			doc.WriteString(instFooterStyle.Render("[↑/↓/j/k] Scroll  |  [Space] Select  |  [a] Toggle All  |  [Enter] Proceed  |  [q] Quit"))
		}
	case instStateConfirming:
		doc.WriteString(instFooterStyle.Render("[y] Confirm sweep and delete  |  [n/esc] Cancel"))
	case instStateSweeping:
		doc.WriteString(instFooterStyle.Render("Sweeping setups... Do NOT interrupt."))
	case instStateFinished:
		doc.WriteString(instFooterStyle.Render("[q/esc] Exit to Shell"))
	}

	return doc.String()
}

func countSelectedInstallers(list []installerItem) int {
	c := 0
	for _, it := range list {
		if it.Selected {
			c++
		}
	}
	return c
}

// logInstOperation delegates to the shared structured logging system,
// which also rotates the log; the old local copy grew operations.log unbounded.
func logInstOperation(action, target string, size int64, success bool) {
	logging.LogDestructiveOperation("installer", action, target, size, success)
}

func runHeadlessInstaller() {
	list := scanInstallerItems(instMinSize)

	payload := struct {
		DiscoveredSetups []installerItem `json:"discovered_setups"`
		TotalBytes       int64           `json:"total_bytes"`
		Timestamp        string          `json:"timestamp"`
	}{
		DiscoveredSetups: list,
		TotalBytes: func() int64 {
			var s int64
			for _, i := range list {
				s += i.Size
			}
			return s
		}(),
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}

	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to marshal installer data: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(string(data))
}

// Local helper style functions
func instWhiteText(s string) string {
	return lipgloss.NewStyle().Foreground(instWhiteColor).Render(s)
}

func instCyanText(s string) string {
	return lipgloss.NewStyle().Foreground(instCyanColor).Render(s)
}

func instGrayText(s string) string {
	return lipgloss.NewStyle().Foreground(instGrayColor).Render(s)
}
