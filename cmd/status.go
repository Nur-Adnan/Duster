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
  - Top 5 CPU-hungry processes
  - Power status (battery level and charging status)
  - System uptime and weighted health score`,
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
		data, _ := json.MarshalIndent(stats, "", "  ")
		fmt.Println(string(data))
		return
	}

	initialStats, _ := sysinfo.GetSystemStats()
	m := statusModel{stats: initialStats}
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
	stats  sysinfo.SystemStats
	err    error
	width  int
	height int
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
	return tea.Batch(statusFetchCmd(), statusTickCmd())
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
		return m, tea.Batch(statusFetchCmd(), statusTickCmd())
	case statusStatsMsg:
		if msg.err == nil {
			m.stats = msg.stats
		} else {
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

	// ── Header ──────────────────────────────────────────────────────────
	ramGB := fmt.Sprintf("%.0fGB", float64(s.RAMTotal)/(1024*1024*1024))
	doc.WriteString("\n  ")
	doc.WriteString(statusHeader(s.HealthScore, s.HostName, s.CPUModel, ramGB, s.OSVersion))
	doc.WriteString("\n\n")

	// ── Column widths ────────────────────────────────────────────────────
	const colW = 38 // chars per column content
	const barW = 14 // progress bar width

	// ── CPU section ─────────────────────────────────────────────────────
	var cpuLines, memLines []string

	cpuLines = append(cpuLines, styleHeader.Render("⚙ CPU"))
	cpuLines = append(cpuLines, fmt.Sprintf("%-6s %s  %.1f%%",
		styleLabel.Render("Total"),
		progressBar(s.CPUPercent, barW),
		s.CPUPercent,
	))

	uptime := time.Duration(s.UptimeSeconds) * time.Second
	days := int(uptime.Hours() / 24)
	hours := int(uptime.Hours()) % 24
	mins := int(uptime.Minutes()) % 60
	cpuLines = append(cpuLines, fmt.Sprintf("%-6s %s",
		styleLabel.Render("Uptime"),
		styleMuted.Render(fmt.Sprintf("%dd %dh %dm", days, hours, mins)),
	))
	for i, c := range s.CPUCores {
		if i >= 4 {
			cpuLines = append(cpuLines, styleMuted.Render(fmt.Sprintf("  … +%d more cores", len(s.CPUCores)-4)))
			break
		}
		cpuLines = append(cpuLines, fmt.Sprintf("%-6s %s  %.1f%%",
			styleLabel.Render(fmt.Sprintf("Core %d", i+1)),
			progressBar(c, barW),
			c,
		))
	}

	// ── Memory section ───────────────────────────────────────────────────
	memLines = append(memLines, styleHeader.Render("▦ Memory"))
	memLines = append(memLines, fmt.Sprintf("%-6s %s  %.1f%%",
		styleLabel.Render("Used"),
		progressBar(s.RAMPercent, barW),
		s.RAMPercent,
	))
	memLines = append(memLines, fmt.Sprintf("%-6s %s  /  %s",
		styleLabel.Render("Total"),
		styleValue.Render(formatBytes(int64(s.RAMUsed))),
		styleValue.Render(formatBytes(int64(s.RAMTotal))),
	))
	memLines = append(memLines, fmt.Sprintf("%-6s %s  %.1f%%",
		styleLabel.Render("Free"),
		progressBar(100-s.RAMPercent, barW),
		100-s.RAMPercent,
	))
	memLines = append(memLines, fmt.Sprintf("%-6s %s",
		styleLabel.Render("Avail"),
		styleValue.Render(formatBytes(int64(s.RAMAvail))),
	))

	// ── Disk section ─────────────────────────────────────────────────────
	var diskLines, powerLines []string
	diskLines = append(diskLines, styleHeader.Render("▤ Disk"))
	for _, d := range s.Disks {
		pct := 0.0
		if d.Total > 0 {
			pct = float64(d.Used) / float64(d.Total) * 100
		}
		diskLines = append(diskLines, fmt.Sprintf("%-6s %s  %.1f%%",
			styleLabel.Render(d.Drive),
			progressBar(pct, barW),
			pct,
		))
	}
	diskLines = append(diskLines, fmt.Sprintf("%-6s %s",
		styleLabel.Render("Free"),
		styleValue.Render(func() string {
			if len(s.Disks) > 0 {
				return formatBytes(int64(s.Disks[0].Free))
			}
			return "N/A"
		}()),
	))

	// Mini I/O bars — scale to max 100 MB/s
	const maxIOBytes = 100 * 1024 * 1024
	diskLines = append(diskLines, fmt.Sprintf("%-6s %s  %s/s",
		styleLabel.Render("Read"),
		miniBar(float64(s.DiskReadSec), maxIOBytes, 5),
		styleValue.Render(formatBytes(int64(s.DiskReadSec))),
	))
	diskLines = append(diskLines, fmt.Sprintf("%-6s %s  %s/s",
		styleLabel.Render("Write"),
		miniBar(float64(s.DiskWriteSec), maxIOBytes, 5),
		styleValue.Render(formatBytes(int64(s.DiskWriteSec))),
	))

	// ── Power section ────────────────────────────────────────────────────
	powerLines = append(powerLines, styleHeader.Render("⚡ Power"))
	powerLines = append(powerLines, fmt.Sprintf("%-6s %s  %d%%",
		styleLabel.Render("Level"),
		progressBar(float64(s.BatteryLevel), barW),
		s.BatteryLevel,
	))
	powerLines = append(powerLines, fmt.Sprintf("%-6s %s",
		styleLabel.Render("Status"),
		styleValue.Render(s.BatteryStatus),
	))
	powerLines = append(powerLines, fmt.Sprintf("%-6s %s",
		styleLabel.Render("Health"),
		styleSub.Render(s.BatteryHealth),
	))

	// ── Network section ───────────────────────────────────────────────────
	var netLines, procLines []string
	netLines = append(netLines, styleHeader.Render("⇅ Network"))
	const maxNetBytes = 10 * 1024 * 1024 // scale to 10 MB/s
	netLines = append(netLines, fmt.Sprintf("%-4s  %s  %s/s",
		styleLabel.Render("Down"),
		miniBar(float64(s.NetDownSec), maxNetBytes, 10),
		styleValue.Render(formatBytes(int64(s.NetDownSec))),
	))
	netLines = append(netLines, fmt.Sprintf("%-4s  %s  %s/s",
		styleLabel.Render("Up"),
		miniBar(float64(s.NetUpSec), maxNetBytes, 10),
		styleValue.Render(formatBytes(int64(s.NetUpSec))),
	))

	// ── Processes section ─────────────────────────────────────────────────
	procLines = append(procLines, styleHeader.Render("▶ Processes"))
	for _, p := range s.TopProcesses {
		name := p.Name
		if len(name) > 12 {
			name = name[:12]
		}
		procLines = append(procLines, fmt.Sprintf("%-13s %s  %.1f%%",
			styleLabel.Render(name),
			progressBar(p.CPU, 6),
			p.CPU,
		))
	}

	// ── Render two-column grid ────────────────────────────────────────────
	colStyle := lipgloss.NewStyle().Width(colW).PaddingRight(4)

	row1 := lipgloss.JoinHorizontal(lipgloss.Top,
		colStyle.Render(strings.Join(cpuLines, "\n")),
		colStyle.Render(strings.Join(memLines, "\n")),
	)
	row2 := lipgloss.JoinHorizontal(lipgloss.Top,
		colStyle.Render(strings.Join(diskLines, "\n")),
		colStyle.Render(strings.Join(powerLines, "\n")),
	)
	row3 := lipgloss.JoinHorizontal(lipgloss.Top,
		colStyle.Render(strings.Join(netLines, "\n")),
		colStyle.Render(strings.Join(procLines, "\n")),
	)

	sep := "\n" + styleDivider.Render("  "+strings.Repeat("─", 76)) + "\n\n"

	doc.WriteString("  " + strings.ReplaceAll(row1, "\n", "\n  "))
	doc.WriteString(sep)
	doc.WriteString("  " + strings.ReplaceAll(row2, "\n", "\n  "))
	doc.WriteString(sep)
	doc.WriteString("  " + strings.ReplaceAll(row3, "\n", "\n  "))
	doc.WriteString("\n\n")
	doc.WriteString("  " + kbHints("Q Quit", "Refreshes every 1s"))

	return doc.String()
}
