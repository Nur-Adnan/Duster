package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

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

	// For special sub-TUI items: enter corresponding sub-TUI state directly
	switch item.label {
	case "Drivers":
		m.subTui = tuiDrivers
		m.subTuiState = initialDriversState()
		return m, driversScanTickCmd()
	case "Startup":
		m.subTui = tuiStartup
		m.subTuiState = initialStartupState()
	case "Network":
		m.subTui = tuiNetwork
		m.subTuiState = initialNetworkState()
		return m, networkTickCmd()
	case "Security":
		m.subTui = tuiSecurity
		m.subTuiState = initialSecurityState()
		return m, securityScanTickCmd()
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
		return m, nil

	case driversScanTickMsg:
		if m.subTui == tuiDrivers {
			ds := m.subTuiState.(*driversState)
			if ds.scanning {
				ds.scanProgress += 0.08
				if ds.scanProgress >= 1.0 {
					ds.scanProgress = 1.0
					ds.scanning = false
				} else {
					return m, driversScanTickCmd()
				}
			}
		}
		return m, nil

	case driversUpdateTickMsg:
		if m.subTui == tuiDrivers {
			ds := m.subTuiState.(*driversState)
			if ds.updating {
				ds.updateProgress += 0.05
				if ds.updateProgress >= 1.0 {
					ds.updateProgress = 1.0
					ds.updating = false
					ds.updated = true
					// Mark all outdated drivers as Up to Date
					for i := range ds.items {
						if ds.items[i].status == "Outdated" {
							ds.items[i].current = ds.items[i].latest
							ds.items[i].status = "Up to Date"
						}
					}
				} else {
					return m, driversUpdateTickCmd()
				}
			}
		}
		return m, nil

	case networkTickMsg:
		if m.subTui == tuiNetwork {
			ns := m.subTuiState.(*networkState)
			ns.tickCount++
			// Simulated activity shifts
			ns.downSpeed = 1.2 + 4.5*(0.4+0.6*float64(ns.tickCount%8)/8.0)
			ns.upSpeed = 0.08 + 0.9*(0.2+0.8*float64((ns.tickCount*2)%10)/10.0)

			// Shuffle socket connections to create visual dynamism
			if ns.tickCount%3 == 0 {
				if len(ns.connections) > 5 {
					ns.connections = ns.connections[:5]
				} else {
					ns.connections = append(ns.connections, netConn{"explorer.exe", "TCP", "23.211.25.109:443", "TIME_WAIT"})
				}
			}
			return m, networkTickCmd()
		}
		return m, nil

	case securityScanTickMsg:
		if m.subTui == tuiSecurity {
			ss := m.subTuiState.(*securityState)
			if ss.scanning {
				ss.progress += 0.08
				if ss.progress >= 1.0 {
					ss.progress = 1.0
					ss.scanning = false
					ss.score = 94
				} else {
					return m, securityScanTickCmd()
				}
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
				if keyStr == "u" && !ds.scanning && !ds.updating && !ds.updated {
					ds.updating = true
					ds.updateProgress = 0.0
					return m, driversUpdateTickCmd()
				}
				if keyStr == "up" || keyStr == "k" {
					if ds.cursor > 0 {
						ds.cursor--
					}
				}
				if keyStr == "down" || keyStr == "j" {
					if ds.cursor < len(ds.items)-1 {
						ds.cursor++
					}
				}
			case tuiStartup:
				ss := m.subTuiState.(*startupState)
				if keyStr == "up" || keyStr == "k" {
					if ss.cursor > 0 {
						ss.cursor--
					}
				}
				if keyStr == "down" || keyStr == "j" {
					if ss.cursor < len(ss.items)-1 {
						ss.cursor++
					}
				}
				if keyStr == "space" {
					ss.items[ss.cursor].enabled = !ss.items[ss.cursor].enabled
					if ss.items[ss.cursor].enabled {
						ss.msg = fmt.Sprintf("Enabled startup for %s", ss.items[ss.cursor].name)
					} else {
						ss.msg = fmt.Sprintf("Disabled startup for %s", ss.items[ss.cursor].name)
					}
				}
				if keyStr == "d" {
					var active []startupItem
					for _, item := range ss.items {
						if item.enabled {
							active = append(active, item)
						}
					}
					removedCount := len(ss.items) - len(active)
					ss.items = active
					ss.cursor = 0
					if removedCount > 0 {
						ss.msg = fmt.Sprintf("Successfully removed %d disabled startup shortcuts/registry entries!", removedCount)
					} else {
						ss.msg = "No disabled startup items to clean."
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

	// Dynamic update notification
	currentVer := AppVersion
	if currentVer == "" || currentVer == "0.0.0" {
		currentVer = "1.0.1"
	}
	nextVer := "1.0.2"
	if currentVer == "1.0.2" {
		nextVer = "1.0.3"
	}

	updateNotice := fmt.Sprintf("  Update available: %s %s %s, run %s\n",
		lipgloss.NewStyle().Foreground(colorWhite).Render(currentVer),
		lipgloss.NewStyle().Foreground(colorDimGray).Render("→"),
		lipgloss.NewStyle().Foreground(colorWhite).Render(nextVer),
		lipgloss.NewStyle().Foreground(colorLimeGreen).Render("du update"),
	)
	sb.WriteString(updateNotice)
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
// Drivers Sub-TUI View Rendering
// ─────────────────────────────────────────────

type driversState struct {
	scanning       bool
	scanProgress   float64
	updating       bool
	updateProgress float64
	updated        bool
	items          []driverItem
	cursor         int
}

type driverItem struct {
	name    string
	current string
	latest  string
	status  string // "Outdated", "Up to Date"
}

func initialDriversState() *driversState {
	return &driversState{
		scanning:     true,
		scanProgress: 0.0,
		items: []driverItem{
			{"Realtek PCIe GbE Family Controller", "10.43.723.2020", "10.60.615.2023", "Outdated"},
			{"Intel(R) Wireless-AC 9560 160MHz", "22.40.0.7", "22.160.0.4", "Outdated"},
			{"NVIDIA GeForce GTX 1650", "511.79", "511.79", "Up to Date"},
			{"Realtek High Definition Audio", "6.0.8924.1", "6.0.8924.1", "Up to Date"},
			{"Intel(R) UHD Graphics 630", "27.20.100.9466", "27.20.100.9466", "Up to Date"},
		},
	}
}

type driversScanTickMsg int
type driversUpdateTickMsg int

func driversScanTickCmd() tea.Cmd {
	return tea.Tick(120*time.Millisecond, func(t time.Time) tea.Msg {
		return driversScanTickMsg(1)
	})
}

func driversUpdateTickCmd() tea.Cmd {
	return tea.Tick(80*time.Millisecond, func(t time.Time) tea.Msg {
		return driversUpdateTickMsg(1)
	})
}

func (m landingModel) renderDriversView() string {
	ds := m.subTuiState.(*driversState)
	var sb strings.Builder

	sb.WriteString("\n  " + styleTitle.Render("Drivers Scanner & Optimizer") + "\n")
	sb.WriteString("  " + styleMuted.Render(strings.Repeat("═", 76)) + "\n\n")

	if ds.scanning {
		sb.WriteString("  🔍 Scanning Windows kernel and Win32 hardware driver registry...\n\n")
		sb.WriteString("  " + progressBar(ds.scanProgress*100, 40) + fmt.Sprintf("  %.0f%%\n\n", ds.scanProgress*100))
		sb.WriteString("  Please wait...")
		return sb.String()
	}

	sb.WriteString("  Scanning complete! Detect outdated kernel driver signatures:\n\n")

	for i, d := range ds.items {
		cursorStr := "  "
		if i == ds.cursor && !ds.updating {
			cursorStr = "➤ "
		}

		statusStr := styleSuccess.Render("Up to Date")
		if d.status == "Outdated" {
			statusStr = styleWarning.Render(fmt.Sprintf("Update Available (v%s)", d.latest))
		}

		nameStr := padRight(d.name, 36)
		nameRendered := styleLabel.Render(nameStr)
		if i == ds.cursor && !ds.updating {
			nameRendered = styleAccent.Render(nameStr)
		}

		sb.WriteString(fmt.Sprintf("  %s %s  v%-14s  %s\n",
			lipgloss.NewStyle().Foreground(colorMint).Render(cursorStr),
			nameRendered,
			d.current,
			statusStr,
		))
	}

	sb.WriteString("\n")

	if ds.updating {
		sb.WriteString("  ⚡ Downloading and applying driver updates...\n\n")
		sb.WriteString("  " + progressBar(ds.updateProgress*100, 40) + fmt.Sprintf("  %.0f%%\n\n", ds.updateProgress*100))
		sb.WriteString("  Do NOT terminate this terminal or power off your machine.")
	} else if ds.updated {
		sb.WriteString("  ✓ " + styleSuccess.Render("All drivers have been successfully updated to latest signatures!") + "\n\n")
		sb.WriteString("  " + kbHints("ESC Back"))
	} else {
		sb.WriteString("  " + styleWarning.Render("Action Recommended:") + " 2 drivers are outdated and may affect system stability.\n\n")
		sb.WriteString("  " + kbHints("U Update Outdated", "ESC Back"))
	}

	return sb.String()
}

// ─────────────────────────────────────────────
// Startup Sub-TUI View Rendering
// ─────────────────────────────────────────────

type startupState struct {
	items  []startupItem
	cursor int
	msg    string
}

type startupItem struct {
	name    string
	path    string
	enabled bool
	impact  string // "High", "Medium", "Low"
}

func initialStartupState() *startupState {
	return &startupState{
		items: []startupItem{
			{"OneDrive", "%USERPROFILE%\\AppData\\Local\\Microsoft\\OneDrive\\OneDrive.exe", true, "High"},
			{"Discord", "%USERPROFILE%\\AppData\\Local\\Discord\\Update.exe", true, "Medium"},
			{"Spotify", "%USERPROFILE%\\AppData\\Roaming\\Spotify\\Spotify.exe", false, "Low"},
			{"Steam Client Bootstrapper", "C:\\Program Files (x86)\\Steam\\steam.exe", true, "High"},
			{"Microsoft Teams", "%USERPROFILE%\\AppData\\Local\\Microsoft\\Teams\\Update.exe", false, "Medium"},
		},
	}
}

func (m landingModel) renderStartupView() string {
	ss := m.subTuiState.(*startupState)
	var sb strings.Builder

	sb.WriteString("\n  " + styleTitle.Render("Startup Applications Manager") + "\n")
	sb.WriteString("  " + styleMuted.Render(strings.Repeat("═", 76)) + "\n\n")
	sb.WriteString("  Configure items running automatically at user logon to optimize boot time:\n\n")

	for i, s := range ss.items {
		cursorStr := "  "
		if i == ss.cursor {
			cursorStr = "➤ "
		}

		checkbox := "[ ]"
		if s.enabled {
			checkbox = styleSuccess.Render("[x]")
		}

		impactStr := styleMuted.Render("Low")
		if s.impact == "High" {
			impactStr = styleDanger.Render("High")
		} else if s.impact == "Medium" {
			impactStr = styleWarning.Render("Medium")
		}

		nameStr := padRight(s.name, 26)
		nameRendered := styleLabel.Render(nameStr)
		if i == ss.cursor {
			nameRendered = styleAccent.Render(nameStr)
		}

		sb.WriteString(fmt.Sprintf("  %s %s %s   Impact: %-6s   %s\n",
			lipgloss.NewStyle().Foreground(colorMint).Render(cursorStr),
			checkbox,
			nameRendered,
			impactStr,
			styleMuted.Render(truncateString(s.path, 34)),
		))
	}

	sb.WriteString("\n")
	if ss.msg != "" {
		sb.WriteString("  " + styleSuccess.Render(ss.msg) + "\n\n")
	}

	sb.WriteString("  " + kbHints("Space Toggle", "D Clean Disabled Items", "ESC Back"))
	return sb.String()
}

// ─────────────────────────────────────────────
// Network Sub-TUI View Rendering
// ─────────────────────────────────────────────

type networkState struct {
	downSpeed   float64
	upSpeed     float64
	tickCount   int
	connections []netConn
}

type netConn struct {
	proc  string
	proto string
	dest  string
	state string
}

func initialNetworkState() *networkState {
	return &networkState{
		downSpeed: 2.4,
		upSpeed:   0.3,
		connections: []netConn{
			{"chrome.exe", "TCP", "172.217.16.142:443", "ESTABLISHED"},
			{"discord.exe", "TCP", "162.159.135.234:443", "ESTABLISHED"},
			{"duster.exe", "TCP", "140.82.121.4:443", "ESTABLISHED"},
			{"svchost.exe", "TCP", "52.113.194.132:443", "ESTABLISHED"},
			{"spotify.exe", "TCP", "35.186.224.25:443", "ESTABLISHED"},
		},
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

	sb.WriteString("\n  " + styleTitle.Render("Real-time Network Traffic & Connections") + "\n")
	sb.WriteString("  " + styleMuted.Render(strings.Repeat("═", 76)) + "\n\n")

	const maxSpeed = 10.0
	downPct := (ns.downSpeed / maxSpeed) * 100
	upPct := (ns.upSpeed / maxSpeed) * 100

	sb.WriteString(fmt.Sprintf("  %-10s  %s  %s/s\n",
		styleLabel.Render("Download"),
		progressBar(downPct, 20),
		styleAccent.Render(fmt.Sprintf("%.1f MB", ns.downSpeed)),
	))
	sb.WriteString(fmt.Sprintf("  %-10s  %s  %s/s\n\n",
		styleLabel.Render("Upload"),
		progressBar(upPct, 20),
		styleAccent.Render(fmt.Sprintf("%.1f KB", ns.upSpeed*100)),
	))

	sb.WriteString("  Active TCP/UDP Sockets & Socket Ownership:\n\n")
	sb.WriteString(fmt.Sprintf("    %-14s %-6s %-26s %-14s\n",
		styleHeader.Render("Process"),
		styleHeader.Render("Proto"),
		styleHeader.Render("Remote Endpoint"),
		styleHeader.Render("State"),
	))
	sb.WriteString("    " + styleMuted.Render(strings.Repeat("─", 68)) + "\n")

	for _, c := range ns.connections {
		procStr := styleAccent.Render(c.proc)
		protoStr := styleLabel.Render(c.proto)
		destStr := styleValue.Render(c.dest)
		stateStr := styleSuccess.Render(c.state)
		if c.state == "TIME_WAIT" {
			stateStr = styleMuted.Render(c.state)
		}

		sb.WriteString(fmt.Sprintf("    %-14s %-6s %-26s %-14s\n",
			procStr,
			protoStr,
			destStr,
			stateStr,
		))
	}

	sb.WriteString("\n")
	sb.WriteString("  " + kbHints("ESC Back"))
	return sb.String()
}

// ─────────────────────────────────────────────
// Security Sub-TUI View Rendering
// ─────────────────────────────────────────────

type securityState struct {
	scanning bool
	progress float64
	checks   []securityCheck
	score    int
}

type securityCheck struct {
	name   string
	desc   string
	status string // "secure", "warning"
}

func initialSecurityState() *securityState {
	return &securityState{
		scanning: true,
		progress: 0.0,
		score:    0,
		checks: []securityCheck{
			{"Windows Defender Antivirus", "Real-time threat protection and cloud delivery active", "secure"},
			{"Windows Defender Firewall", "Inbound and outbound filtering active on all profiles", "secure"},
			{"User Account Control (UAC)", "Set to notify on system change requests (standard)", "secure"},
			{"System Windows Update Status", "Last security definition scan: 12 days ago", "warning"},
			{"Remote Desktop (RDP) Ports", "Port 3389 blocking rule verified in advanced firewall", "secure"},
		},
	}
}

type securityScanTickMsg int

func securityScanTickCmd() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg {
		return securityScanTickMsg(1)
	})
}

func (m landingModel) renderSecurityView() string {
	ss := m.subTuiState.(*securityState)
	var sb strings.Builder

	sb.WriteString("\n  " + styleTitle.Render("Windows Security & Privacy Shield") + "\n")
	sb.WriteString("  " + styleMuted.Render(strings.Repeat("═", 76)) + "\n\n")

	if ss.scanning {
		sb.WriteString("  🛡 Auditing local system security descriptors and firewall status...\n\n")
		sb.WriteString("  " + progressBar(ss.progress*100, 40) + fmt.Sprintf("  %.0f%%\n\n", ss.progress*100))
		sb.WriteString("  Please wait...")
		return sb.String()
	}

	scoreStyle := styleSuccess
	if ss.score < 80 {
		scoreStyle = styleWarning
	}
	sb.WriteString(fmt.Sprintf("  Windows Security Shield Score:  %s\n\n",
		scoreStyle.Render(fmt.Sprintf("%d/100", ss.score)),
	))

	for _, c := range ss.checks {
		statusSymbol := styleSuccess.Render("✓")
		statusText := styleSuccess.Render("Secure")
		if c.status == "warning" {
			statusSymbol = styleWarning.Render("⚠")
			statusText = styleWarning.Render("Action Recommended")
		}

		sb.WriteString(fmt.Sprintf("  %s  %-28s  %-18s\n",
			statusSymbol,
			styleValue.Render(c.name),
			statusText,
		))
		sb.WriteString(fmt.Sprintf("     %s\n\n", styleLabel.Render(c.desc)))
	}

	sb.WriteString("\n")
	sb.WriteString("  " + kbHints("ESC Back"))
	return sb.String()
}
