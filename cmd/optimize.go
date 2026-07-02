package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/Nur-Adnan/duster/internal/logging"
	"github.com/Nur-Adnan/duster/lib/elevation"
	"github.com/Nur-Adnan/duster/lib/fs"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

var (
	optCtx    context.Context
	optCancel context.CancelFunc
)

// Flags
var (
	optJSON      bool
	optDryRun    bool
	optAssumeYes bool
)

// Premium Lipgloss Styles (Zero-Allocation, prefixed to avoid package conflicts)
var (
	optTealColor   = lipgloss.Color("#008080")
	optCyanColor   = lipgloss.Color("#00FFFF")
	optGrayColor   = lipgloss.Color("#666666")
	optRedColor    = lipgloss.Color("#FF0000")
	optGreenColor  = lipgloss.Color("#00FF00")
	optYellowColor = lipgloss.Color("#FFFF00")

	optHeaderStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(optCyanColor).
			Padding(0, 1)

	optBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(optTealColor).
			Padding(1, 2).
			Width(80)

	optFooterStyle = lipgloss.NewStyle().
			Foreground(optGrayColor).
			PaddingTop(1).
			PaddingLeft(2)

	optDividerStyle = lipgloss.NewStyle().
			Foreground(optGrayColor)

	optSuccessStyle = lipgloss.NewStyle().Bold(true).Foreground(optGreenColor)
	optFailStyle    = lipgloss.NewStyle().Bold(true).Foreground(optRedColor)
	optWarnStyle    = lipgloss.NewStyle().Bold(true).Foreground(optYellowColor)
)

type taskStatus int

const (
	statusPending taskStatus = iota
	statusRunning
	statusCompleted
	statusFailed
	statusSkipped
)

type optimizeTask struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Status      taskStatus `json:"status"`
	Reclaimed   int64      `json:"reclaimed_bytes"`
	ErrorMsg    string     `json:"error_message,omitempty"`
}

var OptimizeCmd = &cobra.Command{
	Use:   "optimize",
	Short: "Optimize PC performance (flush DNS, SSD trim, clean caches)",
	Long: `Flushes the local DNS resolver cache to improve networking latency, runs SSD TRIM 
(ReTrim) on all fixed NTFS system drives to combat write amplification, and purges delivery optimization caches.`,
	Run: executeOptimize,
}

func init() {
	OptimizeCmd.Flags().BoolVar(&optJSON, "json", false, "Output optimization task list and statistics as JSON and exit immediately")
	OptimizeCmd.Flags().BoolVarP(&optDryRun, "dry-run", "d", false, "Simulate system optimizations without applying changes")
	OptimizeCmd.Flags().BoolVarP(&optAssumeYes, "yes", "y", false, "Actually run optimizations in non-interactive (piped/--json) mode")
}

func executeOptimize(cmd *cobra.Command, args []string) {
	optCtx, optCancel = context.WithCancel(context.Background())

	// Headless / JSON snapshot execution
	if optJSON || isPiped() {
		runHeadlessOptimize()
		return
	}

	m := initialOptimizeModel()
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running optimizer TUI: %v\n", err)
		os.Exit(1)
	}
}

type optimizeModel struct {
	tasks      []optimizeTask
	currentIdx int
	running    bool
	width      int
	height     int
	isAdmin    bool
}

type optTaskProgressMsg struct {
	idx       int
	status    taskStatus
	reclaimed int64
	err       error
}

func initialOptimizeModel() optimizeModel {
	isAdmin := elevation.IsAdmin()
	return optimizeModel{
		tasks: []optimizeTask{
			{
				ID:          "dns",
				Name:        "Flush DNS Resolver Cache",
				Description: "Flushes the local Windows DNS cache to clean obsolete routing entries",
				Status:      statusPending,
			},
			{
				ID:          "delivery_opt",
				Name:        "Clear Delivery Optimization Cache",
				Description: "Purges cached Windows Update and store delivery optimization buffers",
				Status:      statusPending,
			},
			{
				ID:          "ssd_trim",
				Name:        "SSD Volume Optimization (TRIM)",
				Description: "Runs SSD ReTrim on NTFS volumes to prevent performance degradation",
				Status:      statusPending,
			},
		},
		currentIdx: 0,
		running:    false,
		isAdmin:    isAdmin,
	}
}

func (m optimizeModel) Init() tea.Cmd {
	return tea.Batch(
		tea.Tick(500*time.Millisecond, func(t time.Time) tea.Msg {
			return optTaskProgressMsg{idx: -1, status: statusRunning}
		}),
	)
}

func runTaskCmd(idx int, task optimizeTask, dry bool, isAdmin bool) tea.Cmd {
	return func() tea.Msg {
		status := statusCompleted
		var reclaimed int64
		var runErr error

		if dry {
			return optTaskProgressMsg{idx: idx, status: statusSkipped, reclaimed: 0}
		}

		switch task.ID {
		case "dns":
			c := exec.CommandContext(optCtx, "ipconfig", "/flushdns")
			setProcessGroup(c)
			runErr = c.Run()
			if runErr != nil {
				status = statusFailed
			}
			logOptOperation("flushdns", "DNS Resolver Cache", 0, runErr == nil)

		case "delivery_opt":
			windir := os.Getenv("WINDIR")
			if windir == "" {
				windir = `C:\Windows`
			}
			cacheDir := filepath.Join(windir, "SoftwareDistribution", "DeliveryOptimization", "Download")

			if fs.IsValidPath(cacheDir) {
				_ = filepath.WalkDir(cacheDir, func(path string, d os.DirEntry, err error) error {
					if err == nil && !d.IsDir() {
						info, err := d.Info()
						if err == nil {
							reclaimed += info.Size()
						}
					}
					return nil
				})

				// Delete contents
				entries, err := os.ReadDir(cacheDir)
				if err == nil {
					for _, entry := range entries {
						entryPath := filepath.Join(cacheDir, entry.Name())
						_ = removeAllSafe(entryPath)
					}
				} else {
					runErr = err
					status = statusFailed
				}
			} else {
				// Path blocked by the safety policy — report it as skipped rather
				// than the default "completed", which would claim a no-op succeeded.
				status = statusSkipped
			}
			// Only log when the task actually ran; a skipped no-op must not be
			// recorded as a successful purge in the audit trail.
			if status != statusSkipped {
				logOptOperation("purge", cacheDir, reclaimed, runErr == nil)
			}

		case "ssd_trim":
			if !isAdmin {
				status = statusSkipped
			} else {
				c := exec.CommandContext(optCtx, "defrag.exe", "/O", "/C")
				setProcessGroup(c)
				runErr = c.Run()
				if runErr != nil {
					status = statusFailed
				}
				// Only log when the task actually ran; a skipped task must not be
				// recorded as FAILED in the audit trail.
				logOptOperation("trim", "All Fixed Volumes", 0, runErr == nil)
			}
		}

		return optTaskProgressMsg{idx: idx, status: status, reclaimed: reclaimed, err: runErr}
	}
}

func (m optimizeModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			optCancel()
			return m, tea.Quit
		case "enter", "y", "Y":
			if !m.running && m.currentIdx == 0 {
				m.running = true
				m.tasks[0].Status = statusRunning
				return m, runTaskCmd(0, m.tasks[0], optDryRun, m.isAdmin)
			}
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case optTaskProgressMsg:
		if msg.idx == -1 {
			// Triggered initial display delay, wait for user input unless already running
			return m, nil
		}

		// Update completed task status
		m.tasks[msg.idx].Status = msg.status
		m.tasks[msg.idx].Reclaimed = msg.reclaimed
		if msg.err != nil {
			m.tasks[msg.idx].ErrorMsg = msg.err.Error()
		}

		nextIdx := msg.idx + 1
		if nextIdx < len(m.tasks) {
			m.currentIdx = nextIdx
			m.tasks[nextIdx].Status = statusRunning
			return m, runTaskCmd(nextIdx, m.tasks[nextIdx], optDryRun, m.isAdmin)
		}

		m.running = false
		m.currentIdx = len(m.tasks)
		return m, nil
	}

	return m, nil
}

func (m optimizeModel) View() string {
	var doc strings.Builder

	// Top Title banner
	doc.WriteString("\n")
	doc.WriteString(optHeaderStyle.Render("Duster PC Performance Optimizer"))
	if optDryRun {
		doc.WriteString("  |  " + optFailStyle.Render("DRY RUN MODE (SIMULATION)"))
	} else {
		doc.WriteString("  |  " + optSuccessStyle.Render("LIVE ACTIVE MODE"))
	}
	doc.WriteString("\n")
	doc.WriteString(optDividerStyle.Render("  ═══════════════════════════════════════════════════════════════════════\n\n"))

	var boxLayout strings.Builder

	if !m.running && m.currentIdx == 0 {
		boxLayout.WriteString("  Optimize operations prepare to execute:\n\n")
		for _, task := range m.tasks {
			boxLayout.WriteString(fmt.Sprintf("    %-40s  %s\n", optWhiteText(task.Name), optGrayText(task.Description)))
		}
		boxLayout.WriteString("\n")
		if !m.isAdmin {
			boxLayout.WriteString(optWarnStyle.Render("  ⚠️  Notice: Duster is running in Standard user mode.\n"))
			boxLayout.WriteString(optGrayText("      Volume SSD TRIM optimization requires Administrative privileges and will be skipped.\n\n"))
		}
		boxLayout.WriteString("  Press [Enter] to run the optimization workflow, or [q] to Exit.")
	} else {
		boxLayout.WriteString("System performance tuning sequence:\n\n")

		var totalReclaimed int64
		for _, task := range m.tasks {
			var stateStr string
			switch task.Status {
			case statusPending:
				stateStr = optGrayText("⌛ Pending")
			case statusRunning:
				stateStr = optCyanText("⚡ Running...")
			case statusCompleted:
				if task.Reclaimed > 0 {
					stateStr = optSuccessStyle.Render(fmt.Sprintf("✓ Done (+%s cleared)", formatBytes(task.Reclaimed)))
				} else {
					stateStr = optSuccessStyle.Render("✓ Done")
				}
			case statusFailed:
				stateStr = optFailStyle.Render("✗ Failed")
				if task.ErrorMsg != "" {
					stateStr += " " + optGrayText("("+task.ErrorMsg+")")
				}
			case statusSkipped:
				if task.ID == "ssd_trim" && !m.isAdmin {
					stateStr = optWarnStyle.Render("⚠ Skipped (Needs Administrator)")
				} else if optDryRun {
					stateStr = optWarnStyle.Render("⚠ Skipped (Simulation)")
				} else {
					stateStr = optWarnStyle.Render("⚠ Skipped (Protected path)")
				}
			}

			boxLayout.WriteString(fmt.Sprintf("  %-36s  [ %s ]\n", optWhiteText(task.Name), stateStr))
			boxLayout.WriteString(fmt.Sprintf("      %s\n\n", optGrayText(task.Description)))
			totalReclaimed += task.Reclaimed
		}

		if m.currentIdx == len(m.tasks) {
			boxLayout.WriteString(optDividerStyle.Render("  ───────────────────────────────────────────────────────────────────────\n"))
			failed := 0
			for _, t := range m.tasks {
				if t.Status == statusFailed {
					failed++
				}
			}
			switch {
			case failed > 0:
				boxLayout.WriteString(fmt.Sprintf("  %s %d task(s) failed — see statuses above.\n\n",
					optFailStyle.Render("✗"), failed))
			case totalReclaimed > 0:
				boxLayout.WriteString(fmt.Sprintf("  %s All operations completed successfully! Reclaimed: %s\n\n",
					optSuccessStyle.Render("✓"), optSuccessStyle.Render(formatBytes(totalReclaimed))))
			default:
				boxLayout.WriteString(fmt.Sprintf("  %s All operations completed successfully!\n\n", optSuccessStyle.Render("✓")))
			}
			boxLayout.WriteString("  Press [q] or [esc] to exit to CLI shell.")
		} else {
			boxLayout.WriteString("  Processing tasks... Do NOT interrupt this process.")
		}
	}

	doc.WriteString(optBoxStyle.Render(boxLayout.String()))
	doc.WriteString("\n")

	// Render beautiful footer instructions
	if !m.running && m.currentIdx == 0 {
		doc.WriteString(optFooterStyle.Render("[Enter] Run Optimizations  |  [q/esc] Exit Optimizer"))
	} else if m.currentIdx == len(m.tasks) {
		doc.WriteString(optFooterStyle.Render("[q/esc] Exit to Shell"))
	} else {
		doc.WriteString(optFooterStyle.Render("Executing system operations... Please stand by."))
	}

	return doc.String()
}

// logOptOperation delegates to the shared structured logging system.
func logOptOperation(action, target string, size int64, success bool) {
	logging.LogDestructiveOperation("optimize", action, target, size, success)
}

func runHeadlessOptimize() {
	isAdmin := elevation.IsAdmin()
	tasks := []optimizeTask{
		{
			ID:          "dns",
			Name:        "Flush DNS Resolver Cache",
			Description: "Flushes the local Windows DNS cache to clean obsolete routing entries",
			Status:      statusPending,
		},
		{
			ID:          "delivery_opt",
			Name:        "Clear Delivery Optimization Cache",
			Description: "Purges cached Windows Update and store delivery optimization buffers",
			Status:      statusPending,
		},
		{
			ID:          "ssd_trim",
			Name:        "SSD Volume Optimization (TRIM)",
			Description: "Runs SSD ReTrim on NTFS volumes to prevent performance degradation",
			Status:      statusPending,
		},
	}

	// Non-interactive runs have no confirmation step, so require --yes before
	// applying changes (SSD ReTrim runs defrag.exe /O /C on every volume, which
	// can take a long time). Otherwise emit the task list as a preview.
	previewOnly := optDryRun || !optAssumeYes

	var totalReclaimed int64
	for i, task := range tasks {
		if previewOnly {
			tasks[i].Status = statusSkipped
			continue
		}

		var runErr error
		switch task.ID {
		case "dns":
			// Same cancellation/process-group behavior as the TUI path, so an
			// interrupted pipeline can't orphan child processes.
			c := exec.CommandContext(optCtx, "ipconfig", "/flushdns")
			setProcessGroup(c)
			runErr = c.Run()
			if runErr != nil {
				tasks[i].Status = statusFailed
				tasks[i].ErrorMsg = runErr.Error()
			} else {
				tasks[i].Status = statusCompleted
			}
			logOptOperation("flushdns", "DNS Resolver Cache", 0, runErr == nil)

		case "delivery_opt":
			windir := os.Getenv("WINDIR")
			if windir == "" {
				windir = `C:\Windows`
			}
			cacheDir := filepath.Join(windir, "SoftwareDistribution", "DeliveryOptimization", "Download")

			var reclaimed int64
			if fs.IsValidPath(cacheDir) {
				_ = filepath.WalkDir(cacheDir, func(path string, d os.DirEntry, err error) error {
					if err == nil && !d.IsDir() {
						info, err := d.Info()
						if err == nil {
							reclaimed += info.Size()
						}
					}
					return nil
				})

				entries, err := os.ReadDir(cacheDir)
				if err == nil {
					for _, entry := range entries {
						_ = removeAllSafe(filepath.Join(cacheDir, entry.Name()))
					}
					tasks[i].Status = statusCompleted
					tasks[i].Reclaimed = reclaimed
					totalReclaimed += reclaimed
				} else {
					tasks[i].Status = statusFailed
					tasks[i].ErrorMsg = err.Error()
					runErr = err
				}
			} else {
				tasks[i].Status = statusSkipped
			}
			if tasks[i].Status != statusSkipped {
				logOptOperation("purge", cacheDir, reclaimed, runErr == nil)
			}

		case "ssd_trim":
			if !isAdmin {
				tasks[i].Status = statusSkipped
			} else {
				c := exec.CommandContext(optCtx, "defrag.exe", "/O", "/C")
				setProcessGroup(c)
				runErr = c.Run()
				if runErr != nil {
					tasks[i].Status = statusFailed
					tasks[i].ErrorMsg = runErr.Error()
				} else {
					tasks[i].Status = statusCompleted
				}
				logOptOperation("trim", "All Fixed Volumes", 0, runErr == nil)
			}
		}
	}

	payload := struct {
		Tasks          []optimizeTask `json:"tasks"`
		TotalReclaimed int64          `json:"total_reclaimed_bytes"`
		AdminElevated  bool           `json:"admin_elevated"`
		Timestamp      string         `json:"timestamp"`
	}{
		Tasks:          tasks,
		TotalReclaimed: totalReclaimed,
		AdminElevated:  isAdmin,
		Timestamp:      time.Now().UTC().Format(time.RFC3339),
	}

	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error encoding JSON: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(string(data))
}

// Local helper style functions — delegate to canonical shared helpers
func optWhiteText(s string) string { return whiteText(s) }
func optCyanText(s string) string  { return cyanText(s) }
func optGrayText(s string) string  { return grayText(s) }
