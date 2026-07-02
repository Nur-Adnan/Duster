package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Nur-Adnan/duster/lib/sysinfo"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

var statusJSON bool

var StatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Real-time system health dashboard and live resource monitor",
	Long: `Display a real-time dashboard of system health, showing:
  - CPU usage (per-core and total)
  - RAM usage (used, total, available)
  - Disk utilization and read/write I/O speeds
  - Network upload/download activity
  - Power status (battery level and charging status)
  - System uptime

Top processes and the weighted health score are included in --json output.`,
	Run: executeStatus,
}

func init() {
	StatusCmd.Flags().BoolVar(&statusJSON, "json", false, "Output system stats as a single JSON snapshot and exit immediately")
}

func isPiped() bool {
	stat, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return (stat.Mode() & os.ModeCharDevice) == 0
}

func executeStatus(cmd *cobra.Command, args []string) {
	if statusJSON || isPiped() {
		stats, err := sysinfo.GetSystemStats()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error gathering stats: %v\n", err)
			os.Exit(1)
		}
		data, err := json.MarshalIndent(stats, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error encoding JSON: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(string(data))
		return
	}

	initialStats, initErr := sysinfo.GetSystemStats()
	m := statusModel{stats: initialStats, hasStats: initErr == nil}
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running TUI dashboard: %v\n", err)
		os.Exit(1)
	}
}

// ─────────────────────────────────────────────
// Bubble Tea Model
// ─────────────────────────────────────────────

type statusModel struct {
	stats     sysinfo.SystemStats
	err       error
	width     int
	height    int
	tickCount int
	fetching  bool
	hasStats  bool // true once at least one fetch has succeeded
}

type statusTickMsg time.Time
type statusStatsMsg struct {
	stats sysinfo.SystemStats
	err   error
}

func statusTickCmd() tea.Cmd {
	return tea.Tick(1*time.Second, func(t time.Time) tea.Msg {
		return statusTickMsg(t)
	})
}

func statusFetchCmd() tea.Cmd {
	return func() tea.Msg {
		stats, err := sysinfo.GetSystemStats()
		return statusStatsMsg{stats: stats, err: err}
	}
}

func (m statusModel) Init() tea.Cmd {
	// executeStatus pre-seeds m.stats; the first tick starts the fetch cycle.
	return statusTickCmd()
}

func (m statusModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "q" || msg.String() == "esc" || msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case statusTickMsg:
		m.tickCount++
		// Only one fetch in flight at a time: stats collection can outlast the
		// 1s tick on busy machines, and overlapping fetches apply out of order.
		cmds := []tea.Cmd{statusTickCmd()}
		if !m.fetching {
			m.fetching = true
			cmds = append(cmds, statusFetchCmd())
		}
		return m, tea.Batch(cmds...)
	case statusStatsMsg:
		m.fetching = false
		if msg.err == nil {
			m.stats = msg.stats
			m.hasStats = true
			m.err = nil // a recovered fetch must clear a prior transient error
		} else if !m.hasStats {
			// Only blank the dashboard if we have never had a good sample; once
			// data exists, a transient WMI/PDH hiccup keeps the last-good view.
			m.err = msg.err
		}
	}
	return m, nil
}

// ─────────────────────────────────────────────
// View — Duster two-column system dashboard layout
// ─────────────────────────────────────────────

func (m statusModel) View() string {
	if m.err != nil {
		return fmt.Sprintf("\n  %s: %v\n\n  Press [q] to exit.", styleDanger.Render("Error"), m.err)
	}

	s := m.stats
	var doc strings.Builder

	// Render the high-fidelity responsive header matching the screenshot
	// C:\>du status and DUSTER logo with tagline
	doc.WriteString(RenderHeaderWithSubtitle(m.width, "du status", "Real-time System Status", "Press Ctrl+C to stop"))

	// 1. CPU metrics
	cpuModel := s.CPUModel
	if cpuModel == "" {
		cpuModel = "Unknown CPU"
	}
	// Only values actually measured by sysinfo are shown — the dashboard must
	// never fabricate telemetry (temps, load averages, cache sizes) that the
	// collector does not report.
	cpuPercent := s.CPUPercent
	coresCount := len(s.CPUCores)
	if coresCount == 0 {
		coresCount = 1
	}
	coresStr := fmt.Sprintf("%d logical", coresCount)
	baseFreqStr := "N/A"
	if strings.Contains(s.CPUModel, "@") {
		parts := strings.Split(s.CPUModel, "@")
		baseFreqStr = strings.TrimSpace(parts[len(parts)-1])
	}
	peakCore := 0.0
	var sumCores float64
	for _, c := range s.CPUCores {
		sumCores += c
		if c > peakCore {
			peakCore = c
		}
	}
	avgCore := 0.0
	if len(s.CPUCores) > 0 {
		avgCore = sumCores / float64(len(s.CPUCores))
	}
	peakCoreStr := fmt.Sprintf("%.0f%%", peakCore)
	avgCoreStr := fmt.Sprintf("%.0f%%", avgCore)

	// 2. RAM metrics
	ramPercent := s.RAMPercent
	ramTotalGB := float64(s.RAMTotal) / (1024 * 1024 * 1024)
	ramUsedGB := float64(s.RAMUsed) / (1024 * 1024 * 1024)
	ramAvailGB := float64(s.RAMAvail) / (1024 * 1024 * 1024)

	// 3. DISK metrics
	diskTotalGB := 0.0
	diskUsedGB := 0.0
	diskPercent := 0.0
	driveLetter := "C:"
	if len(s.Disks) > 0 {
		d := s.Disks[0]
		diskTotalGB = float64(d.Total) / (1024 * 1024 * 1024)
		diskUsedGB = float64(d.Used) / (1024 * 1024 * 1024)
		if d.Total > 0 {
			diskPercent = float64(d.Used) / float64(d.Total) * 100
		}
		driveLetter = strings.TrimSuffix(d.Drive, `\`)
	}
	diskLabel := fmt.Sprintf("%.0f GB / %.0f GB (%s)", diskUsedGB, diskTotalGB, driveLetter)

	readSpeedMB := float64(s.DiskReadSec) / (1024 * 1024)
	writeSpeedMB := float64(s.DiskWriteSec) / (1024 * 1024)
	readSpeedStr := fmt.Sprintf("%.1f MB/s", readSpeedMB)
	writeSpeedStr := fmt.Sprintf("%.1f MB/s", writeSpeedMB)

	volumesStr := fmt.Sprintf("%d", len(s.Disks))
	diskFreeStr := fmt.Sprintf("%.0f GB", diskTotalGB-diskUsedGB)

	// 4. NETWORK metrics
	downSpeedMbps := float64(s.NetDownSec) * 8 / (1024 * 1024)
	upSpeedMbps := float64(s.NetUpSec) * 8 / (1024 * 1024)

	// 5. BATTERY metrics
	batteryLevel := s.BatteryLevel
	batteryStatus := s.BatteryStatus
	batteryHealth := s.BatteryHealth

	if batteryStatus == "" || batteryStatus == "Unknown" {
		batteryStatus = "N/A"
		batteryHealth = "N/A"
	}

	batteryPowerStr := "AC Connected"
	if batteryStatus == "Discharging" {
		batteryPowerStr = "Discharging"
	}
	batteryLabelLeft := fmt.Sprintf("%d%% (%s)", batteryLevel, batteryHealth)

	// Progress bars formatted beautifully
	pctStr := fmt.Sprintf("%3d%%", int(cpuPercent))
	leftCpuProgress := progressBar(cpuPercent, 30) + "   " + styleSuccess.Render(pctStr)

	pctRamStr := fmt.Sprintf("%3d%%", int(ramPercent))
	leftRamProgress := progressBar(ramPercent, 30) + "   " + styleSuccess.Render(pctRamStr)

	pctDiskStr := fmt.Sprintf("%3d%%", int(diskPercent))
	leftDiskProgress := progressBar(diskPercent, 30) + "   " + styleSuccess.Render(pctDiskStr)

	netDownCell := styleSuccess.Render("↓ ") + " " + styleValue.Render(fmt.Sprintf("%.1f Mbps", downSpeedMbps))
	netUpCell := styleSuccess.Render("↑ ") + " " + styleValue.Render(fmt.Sprintf("%.1f Mbps", upSpeedMbps))
	netSpeedLine := netDownCell + "         " + netUpCell

	// Define helper local function to render panel lines with printable index 70 alignment
	renderPanelLine := func(leftPart, label, value string) string {
		leftCell := lipgloss.PlaceHorizontal(56, lipgloss.Left, "  "+leftPart)
		if label == "" {
			return leftCell
		}
		rightLabel := styleLabel.Render(padRight(label, 14))
		colon := styleLabel.Render(": ")
		rightValue := styleValue.Render(value)
		return leftCell + rightLabel + colon + rightValue
	}

	dividerWidth := m.width - 4
	if dividerWidth < 80 {
		dividerWidth = 80
	}
	sepLine := styleDivider.Render("  " + strings.Repeat("─", dividerWidth))

	// CPU Panel (4 lines)
	doc.WriteString(renderPanelLine(styleAccent.Render("CPU"), "Cores", coresStr) + "\n")
	doc.WriteString(renderPanelLine(styleValue.Render(cpuModel), "Base Freq", baseFreqStr) + "\n")
	doc.WriteString(renderPanelLine(leftCpuProgress, "Peak Core", peakCoreStr) + "\n")
	doc.WriteString(renderPanelLine("", "Avg Core", avgCoreStr) + "\n")

	doc.WriteString(sepLine + "\n")

	// RAM Panel (3 lines)
	doc.WriteString(renderPanelLine(styleAccent.Render("RAM"), "Used", fmt.Sprintf("%.1f GB", ramUsedGB)) + "\n")
	doc.WriteString(renderPanelLine(styleValue.Render(fmt.Sprintf("%.1f GB / %.1f GB", ramUsedGB, ramTotalGB)), "Available", fmt.Sprintf("%.1f GB", ramAvailGB)) + "\n")
	doc.WriteString(renderPanelLine(leftRamProgress, "Total", fmt.Sprintf("%.1f GB", ramTotalGB)) + "\n")

	doc.WriteString(sepLine + "\n")

	// DISK Panel (4 lines)
	doc.WriteString(renderPanelLine(styleAccent.Render("DISK"), "Read Speed", readSpeedStr) + "\n")
	doc.WriteString(renderPanelLine(styleValue.Render(diskLabel), "Write Speed", writeSpeedStr) + "\n")
	doc.WriteString(renderPanelLine(leftDiskProgress, "Volumes", volumesStr) + "\n")
	doc.WriteString(renderPanelLine("", "Free", diskFreeStr) + "\n")

	doc.WriteString(sepLine + "\n")

	// NETWORK Panel (2 lines)
	doc.WriteString(renderPanelLine(styleAccent.Render("NETWORK"), "Download", fmt.Sprintf("%.1f Mbps", downSpeedMbps)) + "\n")
	doc.WriteString(renderPanelLine(netSpeedLine, "Upload", fmt.Sprintf("%.1f Mbps", upSpeedMbps)) + "\n")

	doc.WriteString(sepLine + "\n")

	// BATTERY Panel (3 lines)
	doc.WriteString(renderPanelLine(styleAccent.Render("BATTERY"), "Status", batteryStatus) + "\n")
	doc.WriteString(renderPanelLine(styleValue.Render(batteryPowerStr), "Health", batteryHealth) + "\n")
	doc.WriteString(renderPanelLine(styleSuccess.Render(batteryLabelLeft), "Level", fmt.Sprintf("%d%%", batteryLevel)) + "\n")

	doc.WriteString(sepLine + "\n\n")

	// System Footer
	uptime := time.Duration(s.UptimeSeconds) * time.Second
	days := int(uptime.Hours() / 24)
	hours := int(uptime.Hours()) % 24
	mins := int(uptime.Minutes()) % 60
	uptimeStr := fmt.Sprintf("%dd %02dh %02dm", days, hours, mins)

	sysTimeStr := time.Now().Format("2006-01-02 15:04:05")

	uptimeLabel := styleAccent.Render("UPTIME:")
	uptimeVal := styleValue.Render(" " + uptimeStr)
	sep := styleDivider.Render("  │  ")
	sysTimeLabel := styleAccent.Render("SYSTEM TIME:")
	sysTimeVal := styleValue.Render(" " + sysTimeStr)
	footerLine := "  " + uptimeLabel + uptimeVal + sep + sysTimeLabel + sysTimeVal

	doc.WriteString(footerLine + "\n\n")

	// Blinking shell cursor
	cursorChar := "█"
	if m.tickCount%2 != 0 {
		cursorChar = " "
	}
	doc.WriteString("  " + styleValue.Render("C:\\>") + styleSuccess.Render(cursorChar) + "\n")

	return doc.String()
}
