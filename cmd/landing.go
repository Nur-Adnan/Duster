package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/Nur-Adnan/duster/lib/sysinfo"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ─────────────────────────────────────────────
// UI Constants & State Types
// ─────────────────────────────────────────────

type subTuiType int

const (
	tuiNone subTuiType = iota
	tuiDrivers
	tuiStartup
	tuiNetwork
	tuiSecurity
)

// Msg wrappers for Bubble Tea loop
type subprocessFinishedMsg struct{ err error }

// ─────────────────────────────────────────────
// Bubble Tea Model for Duster Landing Menu
// ─────────────────────────────────────────────

type landingModel struct {
	cursor      int
	items       []menuItem
	subTui      subTuiType
	subTuiState interface{}
	width       int
	height      int
	runningSub  bool
	subErr      string
}

type menuItem struct {
	label       string
	description string
	cmdArgs     []string
}

func initialLandingModel() landingModel {
	return landingModel{
		cursor: 0,
		items: []menuItem{
			{"Clean", "Free up disk space", []string{"clean"}},
			{"Uninstall", "Remove apps completely", []string{"uninstall"}},
			{"Optimize", "Improve Windows performance", []string{"optimize"}},
			{"Analyze", "Explore disk usage", []string{"analyze"}},
			{"Status", "Monitor system health", []string{"status"}},
			{"Drivers", "Detect outdated drivers", nil},        // handled via sub-TUI
			{"Startup", "Manage startup applications", nil},    // handled via sub-TUI
			{"Network", "Analyze network activity", nil},       // handled via sub-TUI
			{"Security", "Check Windows security status", nil}, // handled via sub-TUI
			{"Exit", "Exit Duster utility", nil},               // terminates program
		},
		subTui: tuiNone,
	}
}

// ─────────────────────────────────────────────
// Bubble Tea Entry Point
// ─────────────────────────────────────────────

// ExecuteLanding starts the main interactive landing page
func ExecuteLanding() {
	m := initialLandingModel()
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running Duster TUI menu: %v\n", err)
		os.Exit(1)
	}
}

// ─────────────────────────────────────────────
// Init, Update, and Commands
// ─────────────────────────────────────────────

func (m landingModel) Init() tea.Cmd {
	return nil
}

func getDusterExecutable() string {
	exe, err := os.Executable()
	if err == nil {
		return exe
	}
	return os.Args[0]
}

func (m landingModel) runItem(item menuItem) (landingModel, tea.Cmd) {
	if item.label == "Exit" {
		return m, tea.Quit
	}

	// For standard subcommands (Clean, Uninstall, Optimize, Analyze, Status):
	// run as separate subprocess using ExecProcess to safely swap term control
	if item.cmdArgs != nil {
		m.runningSub = true
		exe := getDusterExecutable()
		c := exec.Command(exe, item.cmdArgs...)
		return m, tea.ExecProcess(c, func(err error) tea.Msg {
			return subprocessFinishedMsg{err: err}
		})
	}

	switch item.label {
	case "Drivers":
		m.subTui = tuiDrivers
		m.subTuiState = &driversState{scanning: true}
		return m, scanDriversAsyncCmd()
	case "Startup":
		m.subTui = tuiStartup
		m.subTuiState = &startupState{}
		return m, loadStartupAsyncCmd()
	case "Network":
		m.subTui = tuiNetwork
		m.subTuiState = initialNetworkState()
		return m, tea.Batch(networkTickCmd(), fetchNetworkStatsCmd())
	case "Security":
		m.subTui = tuiSecurity
		m.subTuiState = &securityState{scanning: true}
		return m, runSecurityAuditAsyncCmd()
	}

	return m, nil
}

func (m landingModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case subprocessFinishedMsg:
		m.runningSub = false
		m.subErr = ""
		if msg.err != nil {
			m.subErr = fmt.Sprintf("Failed to launch subcommand: %v", msg.err)
		}
		return m, nil

	case driversScanDoneMsg:
		if m.subTui == tuiDrivers {
			ds := m.subTuiState.(*driversState)
			ds.scanning = false
			if msg.err != nil {
				ds.err = msg.err.Error()
			} else {
				ds.drivers = msg.drivers
			}
		}
		return m, nil

	case startupLoadDoneMsg:
		if m.subTui == tuiStartup {
			ss := m.subTuiState.(*startupState)
			if msg.err != nil {
				ss.err = msg.err.Error()
			} else {
				ss.items = msg.entries
			}
		}
		return m, nil

	case startupMutationDoneMsg:
		if m.subTui == tuiStartup {
			ss := m.subTuiState.(*startupState)
			if msg.err != nil {
				ss.msg = fmt.Sprintf("Error: %v", msg.err)
			} else {
				ss.msg = msg.msg
				ss.items = msg.entries
				if ss.cursor >= len(ss.items) && ss.cursor > 0 {
					ss.cursor = len(ss.items) - 1
				}
			}
		}
		return m, nil

	case securityAuditDoneMsg:
		if m.subTui == tuiSecurity {
			sc := m.subTuiState.(*securityState)
			sc.scanning = false
			if msg.err != nil {
				sc.err = msg.err.Error()
			} else {
				sc.checks = msg.checks
				sc.score = msg.score
			}
		}
		return m, nil

	case networkTickMsg:
		if m.subTui == tuiNetwork {
			ns := m.subTuiState.(*networkState)
			ns.tickCount++
			cmds := []tea.Cmd{networkTickCmd()}
			// Fetch stats off the update loop; GetSystemStats can take hundreds
			// of milliseconds on busy machines and must never block keystrokes.
			if !ns.fetching {
				ns.fetching = true
				cmds = append(cmds, fetchNetworkStatsCmd())
			}
			return m, tea.Batch(cmds...)
		}
		return m, nil

	case networkStatsMsg:
		if m.subTui == tuiNetwork {
			ns := m.subTuiState.(*networkState)
			ns.fetching = false
			if msg.ok {
				ns.downSpeed = msg.down
				ns.upSpeed = msg.up
			}
		}
		return m, nil

	case tea.KeyMsg:
		keyStr := msg.String()

		// Block keystrokes during subprocess runtime
		if m.runningSub {
			return m, nil
		}

		// Keystroke handlers when in a sub-TUI view
		if m.subTui != tuiNone {
			if keyStr == "esc" || keyStr == "q" {
				m.subTui = tuiNone
				m.subTuiState = nil
				return m, nil
			}

			switch m.subTui {
			case tuiDrivers:
				ds := m.subTuiState.(*driversState)
				if !ds.scanning && ds.err == "" {
					switch keyStr {
					case "up", "k":
						if ds.cursor > 0 {
							ds.cursor--
						}
					case "down", "j":
						if ds.cursor < len(ds.drivers)-1 {
							ds.cursor++
						}
					}
				}
			case tuiStartup:
				ss := m.subTuiState.(*startupState)
				if ss.err == "" && len(ss.items) > 0 {
					switch keyStr {
					case "up", "k":
						if ss.cursor > 0 {
							ss.cursor--
						}
					case "down", "j":
						if ss.cursor < len(ss.items)-1 {
							ss.cursor++
						}
					case " ", "space":
						// Bubble Tea reports the space key as " ", never "space"
						entry := ss.items[ss.cursor]
						return m, toggleStartupAsyncCmd(entry)
					case "d":
						var toRemove []startupEntry
						for _, item := range ss.items {
							if !item.Enabled {
								toRemove = append(toRemove, item)
							}
						}
						if len(toRemove) > 0 {
							return m, removeStartupAsyncCmd(toRemove)
						} else {
							ss.msg = "No disabled startup items to remove."
						}
					}
				}
			}

			return m, nil
		}

		// Main menu keystroke handlers
		switch keyStr {
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.items)-1 {
				m.cursor++
			}
		case "enter":
			return m.runItem(m.items[m.cursor])
		case "q", "ctrl+c":
			return m, tea.Quit

		// Direct numeric selections (1 to 9, 0 for 10)
		case "1", "2", "3", "4", "5", "6", "7", "8", "9":
			idx := int(keyStr[0] - '1')
			if idx >= 0 && idx < len(m.items) {
				m.cursor = idx
				return m.runItem(m.items[idx])
			}
		case "0":
			m.cursor = 9
			return m.runItem(m.items[9])

		// Global instant hotkeys mapped from the hints footer
		case "d": // Doctor
			m.runningSub = true
			c := exec.Command(getDusterExecutable(), "doctor")
			return m, tea.ExecProcess(c, func(err error) tea.Msg {
				return subprocessFinishedMsg{err: err}
			})
		case "v": // Verify
			m.runningSub = true
			c := exec.Command(getDusterExecutable(), "verify")
			return m, tea.ExecProcess(c, func(err error) tea.Msg {
				return subprocessFinishedMsg{err: err}
			})
		case "b": // Benchmark
			m.runningSub = true
			c := exec.Command(getDusterExecutable(), "benchmark")
			return m, tea.ExecProcess(c, func(err error) tea.Msg {
				return subprocessFinishedMsg{err: err}
			})
		}
	}
	return m, nil
}

// ─────────────────────────────────────────────
// View & Sub-TUI View Layout Renderers
// ─────────────────────────────────────────────

func (m landingModel) View() string {
	if m.runningSub {
		return ""
	}

	switch m.subTui {
	case tuiDrivers:
		return m.renderDriversView()
	case tuiStartup:
		return m.renderStartupView()
	case tuiNetwork:
		return m.renderNetworkView()
	case tuiSecurity:
		return m.renderSecurityView()
	}

	var sb strings.Builder

	// Render the high-fidelity responsive header matching the screenshot
	sb.WriteString(RenderHeader(m.width, "duster"))

	sb.WriteString("\n")

	// Interactive two-column menu
	for i, item := range m.items {
		symbol := "  "
		if i == m.cursor {
			symbol = "➤ "
		}

		numStr := fmt.Sprintf("%d.", i+1)
		numRendered := lipgloss.NewStyle().Foreground(colorDimGray).Render(numStr)
		if i == m.cursor {
			numRendered = lipgloss.NewStyle().Foreground(colorMint).Render(numStr)
		}

		labelRendered := lipgloss.NewStyle().Foreground(colorWhite).Render(item.label)
		if i == m.cursor {
			labelRendered = lipgloss.NewStyle().Foreground(colorMint).Bold(true).Render(item.label)
		}

		rawNumLabel := fmt.Sprintf("%d. %s", i+1, item.label)
		padLen := 16 - len(rawNumLabel)
		if padLen < 1 {
			padLen = 1
		}
		padding := strings.Repeat(" ", padLen)

		descRendered := lipgloss.NewStyle().Foreground(colorSilver).Render(item.description)
		if i == m.cursor {
			descRendered = lipgloss.NewStyle().Foreground(colorWhite).Render(item.description)
		}

		sb.WriteString(fmt.Sprintf("  %s %s %s%s%s\n",
			lipgloss.NewStyle().Foreground(colorMint).Render(symbol),
			numRendered,
			labelRendered,
			padding,
			descRendered,
		))
	}

	sb.WriteString("\n")

	if m.subErr != "" {
		sb.WriteString("  " + styleDanger.Render("✗ "+m.subErr) + "\n\n")
	}

	// Center-balanced footer
	sb.WriteString("  " + kbHints(
		"↑↓ Navigate",
		"Enter Select",
		"D Doctor",
		"V Verify",
		"B Benchmark",
		"Q Quit",
	) + "\n")

	return sb.String()
}

// ─────────────────────────────────────────────
// Drivers Sub-TUI
// ─────────────────────────────────────────────

type driversState struct {
	scanning bool
	drivers  []driverInfo
	err      string
	cursor   int
}

type driversScanDoneMsg struct {
	drivers []driverInfo
	err     error
}

func scanDriversAsyncCmd() tea.Cmd {
	return func() tea.Msg {
		drivers, err := scanInstalledDrivers()
		return driversScanDoneMsg{drivers: drivers, err: err}
	}
}

func (m landingModel) renderDriversView() string {
	ds := m.subTuiState.(*driversState)
	var sb strings.Builder

	sb.WriteString("\n  " + styleTitle.Render("Installed Drivers Scanner") + "\n")
	sb.WriteString("  " + styleMuted.Render(strings.Repeat("═", 76)) + "\n\n")

	if ds.scanning {
		sb.WriteString("  Scanning PnP signed drivers via WMI...\n\n")
		sb.WriteString("  Please wait...")
		return sb.String()
	}

	if ds.err != "" {
		sb.WriteString("  " + styleDanger.Render("Error: "+ds.err) + "\n\n")
		sb.WriteString("  " + kbHints("ESC Back"))
		return sb.String()
	}

	if len(ds.drivers) == 0 {
		sb.WriteString("  No signed PnP drivers found.\n\n")
		sb.WriteString("  " + kbHints("ESC Back"))
		return sb.String()
	}

	unsigned := 0
	for _, d := range ds.drivers {
		if !d.Signed {
			unsigned++
		}
	}

	sb.WriteString(fmt.Sprintf("  Found %d installed drivers", len(ds.drivers)))
	if unsigned > 0 {
		sb.WriteString(fmt.Sprintf(" (%s)", styleWarning.Render(fmt.Sprintf("%d unsigned", unsigned))))
	}
	sb.WriteString("\n\n")

	maxVisible := 15
	if m.height > 20 {
		maxVisible = m.height - 10
	}

	start := 0
	if ds.cursor >= maxVisible {
		start = ds.cursor - maxVisible + 1
	}
	end := start + maxVisible
	if end > len(ds.drivers) {
		end = len(ds.drivers)
	}

	for i := start; i < end; i++ {
		d := ds.drivers[i]
		cursor := "  "
		if i == ds.cursor {
			cursor = ">"
		}

		signedStr := styleSuccess.Render("Signed")
		if !d.Signed {
			signedStr = styleWarning.Render("Unsigned")
		}

		nameStr := padRight(truncateString(d.Name, 34), 35)
		nameRendered := styleLabel.Render(nameStr)
		if i == ds.cursor {
			nameRendered = styleAccent.Render(nameStr)
		}

		verStr := padRight(truncateString(d.Version, 16), 17)

		sb.WriteString(fmt.Sprintf("  %s %s v%-17s %s\n",
			lipgloss.NewStyle().Foreground(colorMint).Render(cursor),
			nameRendered,
			verStr,
			signedStr,
		))
	}

	if len(ds.drivers) > maxVisible {
		sb.WriteString(fmt.Sprintf("\n  %s",
			styleMuted.Render(fmt.Sprintf("Showing %d-%d of %d", start+1, end, len(ds.drivers)))))
	}

	sb.WriteString("\n\n  " + kbHints("↑↓ Navigate", "ESC Back"))
	return sb.String()
}

// ─────────────────────────────────────────────
// Startup Sub-TUI
// ─────────────────────────────────────────────

type startupState struct {
	items  []startupEntry
	cursor int
	msg    string
	err    string
}

type startupLoadDoneMsg struct {
	entries []startupEntry
	err     error
}

type startupMutationDoneMsg struct {
	entries []startupEntry
	msg     string
	err     error
}

func loadStartupAsyncCmd() tea.Cmd {
	return func() tea.Msg {
		entries, err := getStartupEntries()
		return startupLoadDoneMsg{entries: entries, err: err}
	}
}

func toggleStartupAsyncCmd(entry startupEntry) tea.Cmd {
	return func() tea.Msg {
		err := toggleStartupApproval(entry)
		if err != nil {
			return startupMutationDoneMsg{err: err}
		}
		entries, _ := getStartupEntries()
		action := "Disabled"
		if !entry.Enabled {
			action = "Enabled"
		}
		return startupMutationDoneMsg{
			entries: entries,
			msg:     fmt.Sprintf("%s startup for %s", action, entry.Name),
		}
	}
}

func removeStartupAsyncCmd(toRemove []startupEntry) tea.Cmd {
	return func() tea.Msg {
		removed := 0
		for _, entry := range toRemove {
			if err := removeStartupEntry(entry); err == nil {
				removed++
			}
		}
		entries, _ := getStartupEntries()
		return startupMutationDoneMsg{
			entries: entries,
			msg:     fmt.Sprintf("Removed %d disabled startup entries", removed),
		}
	}
}

func (m landingModel) renderStartupView() string {
	ss := m.subTuiState.(*startupState)
	var sb strings.Builder

	sb.WriteString("\n  " + styleTitle.Render("Startup Applications Manager") + "\n")
	sb.WriteString("  " + styleMuted.Render(strings.Repeat("═", 76)) + "\n\n")

	if ss.err != "" {
		sb.WriteString("  " + styleDanger.Render("Error: "+ss.err) + "\n\n")
		sb.WriteString("  " + kbHints("ESC Back"))
		return sb.String()
	}

	if len(ss.items) == 0 {
		sb.WriteString("  No startup entries found.\n\n")
		sb.WriteString("  " + kbHints("ESC Back"))
		return sb.String()
	}

	sb.WriteString("  Manage applications that run at user logon:\n\n")

	for i, s := range ss.items {
		cursor := "  "
		if i == ss.cursor {
			cursor = ">"
		}

		checkbox := "[ ]"
		if s.Enabled {
			checkbox = styleSuccess.Render("[x]")
		}

		nameStr := padRight(truncateString(s.Name, 22), 23)
		nameRendered := styleLabel.Render(nameStr)
		if i == ss.cursor {
			nameRendered = styleAccent.Render(nameStr)
		}

		locStr := padRight(s.Location, 16)

		sb.WriteString(fmt.Sprintf("  %s %s %s %s %s\n",
			lipgloss.NewStyle().Foreground(colorMint).Render(cursor),
			checkbox,
			nameRendered,
			styleMuted.Render(locStr),
			styleMuted.Render(truncateString(s.Command, 30)),
		))
	}

	sb.WriteString("\n")
	if ss.msg != "" {
		sb.WriteString("  " + styleSuccess.Render(ss.msg) + "\n\n")
	}

	sb.WriteString("  " + kbHints("Space Toggle", "D Remove Disabled", "ESC Back"))
	return sb.String()
}

// ─────────────────────────────────────────────
// Network Sub-TUI View Rendering
// ─────────────────────────────────────────────

type networkState struct {
	downSpeed float64
	upSpeed   float64
	tickCount int
	fetching  bool
}

func initialNetworkState() *networkState {
	return &networkState{fetching: true}
}

type networkStatsMsg struct {
	down float64
	up   float64
	ok   bool
}

func fetchNetworkStatsCmd() tea.Cmd {
	return func() tea.Msg {
		stats, err := sysinfo.GetSystemStats()
		if err != nil {
			return networkStatsMsg{ok: false}
		}
		return networkStatsMsg{
			down: float64(stats.NetDownSec) / (1024 * 1024),
			up:   float64(stats.NetUpSec) / (1024 * 1024),
			ok:   true,
		}
	}
}

type networkTickMsg time.Time

func networkTickCmd() tea.Cmd {
	return tea.Tick(500*time.Millisecond, func(t time.Time) tea.Msg {
		return networkTickMsg(t)
	})
}

func (m landingModel) renderNetworkView() string {
	ns := m.subTuiState.(*networkState)
	var sb strings.Builder

	sb.WriteString("\n  " + styleTitle.Render("Real-time Network Traffic") + "\n")
	sb.WriteString("  " + styleMuted.Render(strings.Repeat("═", 76)) + "\n\n")

	const maxSpeed = 10.0
	downPct := (ns.downSpeed / maxSpeed) * 100
	upPct := (ns.upSpeed / maxSpeed) * 100

	sb.WriteString(fmt.Sprintf("  %-10s  %s  %s/s\n",
		styleLabel.Render("Download"),
		progressBar(downPct, 20),
		styleAccent.Render(fmt.Sprintf("%.1f MB", ns.downSpeed)),
	))
	sb.WriteString(fmt.Sprintf("  %-10s  %s  %s/s\n",
		styleLabel.Render("Upload"),
		progressBar(upPct, 20),
		styleAccent.Render(fmt.Sprintf("%.1f KB", ns.upSpeed*1024)),
	))

	sb.WriteString("\n")
	sb.WriteString("  " + kbHints("ESC Back"))
	return sb.String()
}

// ─────────────────────────────────────────────
// Security Sub-TUI
// ─────────────────────────────────────────────

type securityState struct {
	scanning bool
	checks   []securityCheckResult
	score    int
	err      string
}

type securityAuditDoneMsg struct {
	checks []securityCheckResult
	score  int
	err    error
}

func runSecurityAuditAsyncCmd() tea.Cmd {
	return func() tea.Msg {
		checks, score, err := runSecurityAudit()
		return securityAuditDoneMsg{checks: checks, score: score, err: err}
	}
}

func (m landingModel) renderSecurityView() string {
	sc := m.subTuiState.(*securityState)
	var sb strings.Builder

	sb.WriteString("\n  " + styleTitle.Render("Windows Security & Privacy Shield") + "\n")
	sb.WriteString("  " + styleMuted.Render(strings.Repeat("═", 76)) + "\n\n")

	if sc.scanning {
		sb.WriteString("  Auditing system security via WMI, registry, and firewall rules...\n\n")
		sb.WriteString("  Please wait...")
		return sb.String()
	}

	if sc.err != "" {
		sb.WriteString("  " + styleDanger.Render("Error: "+sc.err) + "\n\n")
		sb.WriteString("  " + kbHints("ESC Back"))
		return sb.String()
	}

	scoreStyle := styleSuccess
	if sc.score < 70 {
		scoreStyle = styleDanger
	} else if sc.score < 90 {
		scoreStyle = styleWarning
	}
	sb.WriteString(fmt.Sprintf("  Windows Security Score:  %s\n\n",
		scoreStyle.Render(fmt.Sprintf("%d/100", sc.score)),
	))

	for _, c := range sc.checks {
		statusSymbol := styleSuccess.Render("OK")
		statusText := styleSuccess.Render("Secure")
		if c.Status == "warning" {
			statusSymbol = styleWarning.Render("!!")
			statusText = styleWarning.Render("Warning")
		} else if c.Status == "critical" {
			statusSymbol = styleDanger.Render("XX")
			statusText = styleDanger.Render("Critical")
		}

		sb.WriteString(fmt.Sprintf("  %s  %-30s  %s\n",
			statusSymbol,
			styleValue.Render(c.Name),
			statusText,
		))
		sb.WriteString(fmt.Sprintf("      %s\n\n", styleLabel.Render(c.Details)))
	}

	sb.WriteString("  " + kbHints("ESC Back"))
	return sb.String()
}
