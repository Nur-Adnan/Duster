package cmd

import (
	"context"
	"encoding/json"
	"errors"
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
	optDeep      bool
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
	Note        string     `json:"note,omitempty"`
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
	OptimizeCmd.Flags().BoolVar(&optDeep, "deep", false, "Also clean the Windows component store (WinSxS) with DISM: needs administrator rights and can run for 10+ minutes")
}

// optimizeTasks is the task list for both the TUI and the headless path, so
// the two can never offer different work. The component store cleanup is
// opt-in: it needs administrator rights and can run for tens of minutes,
// while the default three finish in seconds.
func optimizeTasks(deep bool) []optimizeTask {
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
	if deep {
		tasks = append(tasks, optimizeTask{
			ID:          "component_store",
			Name:        "Windows Component Store Cleanup",
			Description: "Removes superseded components with DISM (admin)",
			Status:      statusPending,
		})
	}
	return tasks
}

// optTaskResult is what one task reports back. note carries user-facing
// detail that is not an error, such as the component store analysis or a
// pending restart.
type optTaskResult struct {
	status    taskStatus
	reclaimed int64
	note      string
	err       error
}

// optContext returns the command's cancellable context, or a background one
// when a task runs outside the command (tests).
func optContext() context.Context {
	if optCtx != nil {
		return optCtx
	}
	return context.Background()
}

// execOptimizeTask runs one task. Both the TUI and the headless path call it,
// so their behavior cannot drift. Callers decide which tasks to skip; dryRun
// only reaches the component store task, whose dry run is DISM's read-only
// analysis.
func execOptimizeTask(task optimizeTask, isAdmin, dryRun bool) optTaskResult {
	switch task.ID {
	case "dns":
		c := exec.CommandContext(optContext(), systemExecutable("ipconfig.exe"), "/flushdns")
		setProcessGroup(c)
		err := c.Run()
		logOptOperation("flushdns", "DNS Resolver Cache", 0, err == nil)
		if err != nil {
			return optTaskResult{status: statusFailed, err: err}
		}
		return optTaskResult{status: statusCompleted}

	case "delivery_opt":
		cacheDir := filepath.Join(secureWindowsDir(), "SoftwareDistribution", "DeliveryOptimization", "Download")
		if !fs.IsValidPath(cacheDir) {
			// Path blocked by the safety policy: report it as skipped rather
			// than the default "completed", which would claim a no-op
			// succeeded. A skipped no-op is not logged either.
			return optTaskResult{status: statusSkipped}
		}
		reclaimed, err := purgeDeliveryOptimization(cacheDir)
		logOptOperation("purge", cacheDir, reclaimed, err == nil)
		if err != nil {
			// Partial frees still count.
			return optTaskResult{status: statusFailed, reclaimed: reclaimed, err: err}
		}
		return optTaskResult{status: statusCompleted, reclaimed: reclaimed}

	case "ssd_trim":
		if !isAdmin {
			// A skipped task must not be recorded as FAILED in the audit trail.
			return optTaskResult{status: statusSkipped}
		}
		c := exec.CommandContext(optContext(), systemExecutable("defrag.exe"), "/O", "/C")
		setProcessGroup(c)
		err := c.Run()
		logOptOperation("trim", "All Fixed Volumes", 0, err == nil)
		if err != nil {
			return optTaskResult{status: statusFailed, err: err}
		}
		return optTaskResult{status: statusCompleted}

	case "component_store":
		return runComponentStoreTask(isAdmin, dryRun)
	}

	return optTaskResult{status: statusCompleted}
}

// runComponentStoreTask analyzes the component store first: the numbers are
// what the user came for, and they say whether the much longer cleanup is
// worth running at all. The cleanup never passes /ResetBase, which would block
// uninstalling updates that are already installed.
func runComponentStoreTask(isAdmin, dryRun bool) optTaskResult {
	if !isAdmin {
		return optTaskResult{status: statusSkipped, note: "needs Administrator"}
	}

	info := analyzeComponentStore(optContext())
	switch {
	case info.Error != "":
		return optTaskResult{status: statusFailed, err: errors.New(info.Error)}
	case info.Unparsed:
		// Sizes stay unknown rather than zero, so this must not be reported
		// as "nothing to reclaim".
		return optTaskResult{status: statusFailed, err: errors.New("DISM ran but its component store report could not be read")}
	}

	summary := fmt.Sprintf("%s of overhead, %d superseded package(s)",
		formatBytes(info.OverheadBytes), info.ReclaimablePkgs)

	if dryRun {
		return optTaskResult{status: statusSkipped, note: summary + "; analysis only, nothing removed"}
	}
	if !info.CleanupRecommended && info.ReclaimablePkgs == 0 {
		return optTaskResult{status: statusCompleted, note: "Windows reports no cleanup is needed (" + summary + ")"}
	}

	driveRoot := systemDriveRoot()
	freeBefore := getDiskFreeBytes(driveRoot)

	rebootRequired, err := startComponentCleanup(optContext())
	if err != nil {
		logOptOperation("component-cleanup", "Component Store (WinSxS)", 0, false)
		return optTaskResult{status: statusFailed, err: err}
	}

	// DISM reports no byte count, and re-running the analysis would repeat a
	// multi-minute operation, so report the free-space change instead.
	freed := getDiskFreeBytes(driveRoot) - freeBefore
	if freed < 0 {
		freed = 0
	}
	if rebootRequired {
		summary += "; restart to finish"
	}
	logOptOperation("component-cleanup", "Component Store (WinSxS)", freed, true)
	return optTaskResult{status: statusCompleted, reclaimed: freed, note: summary}
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
	final, err := p.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error running optimizer TUI: %v\n", err)
		os.Exit(1)
	}
	// The user quit while DISM was servicing Windows: say so, rather than
	// leaving them to guess what state the component store is in.
	if fm, ok := final.(optimizeModel); ok && fm.abortedCleanup {
		fmt.Fprintln(os.Stderr, "Component store cleanup was stopped part-way. Nothing is broken: run `du optimize --deep` again to finish it, or leave it to Windows' own StartComponentCleanup maintenance task.")
	}
}

type optimizeModel struct {
	tasks      []optimizeTask
	currentIdx int
	running    bool
	width      int
	height     int
	isAdmin    bool

	// Reported-only space consumers, measured in the background so the
	// preview screen appears at once.
	reclaim      []reclaimItem
	reclaimReady bool

	// Start of the task now running, for the elapsed-time display: the
	// component store cleanup can run for tens of minutes.
	taskStarted time.Time

	// Quitting mid-cleanup kills DISM inside a servicing operation, so it
	// takes a second keypress. abortedCleanup is reported after the screen
	// closes.
	quitConfirm    bool
	abortedCleanup bool
}

// runningComponentStore reports whether the long DISM servicing task is the
// one currently running. Every other task is a sub-second call that can be
// interrupted without consequence.
func (m optimizeModel) runningComponentStore() bool {
	return m.running &&
		m.currentIdx < len(m.tasks) &&
		m.tasks[m.currentIdx].ID == "component_store" &&
		m.tasks[m.currentIdx].Status == statusRunning
}

type optTaskProgressMsg struct {
	idx       int
	status    taskStatus
	reclaimed int64
	note      string
	err       error
}

// reclaimReportMsg carries the measured Windows.old and hibernation sizes.
type reclaimReportMsg []reclaimItem

func reclaimReportCmd() tea.Cmd {
	return func() tea.Msg {
		return reclaimReportMsg(buildReclaimItems(systemDriveRoot()))
	}
}

// optTickMsg refreshes the elapsed time of the running task.
type optTickMsg time.Time

func optTickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg { return optTickMsg(t) })
}

func initialOptimizeModel() optimizeModel {
	isAdmin := elevation.IsAdmin()
	return optimizeModel{
		tasks:      optimizeTasks(optDeep),
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
		reclaimReportCmd(),
	)
}

func runTaskCmd(idx int, task optimizeTask, dry bool, isAdmin bool) tea.Cmd {
	return func() tea.Msg {
		// A dry run simulates the acting tasks. The component store task is
		// the exception: its dry run is DISM's read-only analysis, which is
		// the whole point of previewing it.
		if dry && task.ID != "component_store" {
			return optTaskProgressMsg{idx: idx, status: statusSkipped, reclaimed: 0}
		}

		res := execOptimizeTask(task, isAdmin, dry)
		return optTaskProgressMsg{idx: idx, status: res.status, reclaimed: res.reclaimed, note: res.note, err: res.err}
	}
}

// purgeDeliveryOptimization empties cacheDir, shared by the TUI and headless
// paths. It reports only bytes actually deleted (the old code counted the
// whole cache up front and ignored delete errors) and returns the first
// failure, e.g. files locked by the Delivery Optimization service.
func purgeDeliveryOptimization(cacheDir string) (int64, error) {
	entries, err := os.ReadDir(cacheDir)
	if err != nil {
		return 0, err
	}
	var reclaimed int64
	var firstErr error
	for _, e := range entries {
		p := filepath.Join(cacheDir, e.Name())
		size := calculateDirSize(p)
		if err := removeAllSafe(p); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			if left := calculateDirSize(p); left < size {
				reclaimed += size - left
			}
			continue
		}
		reclaimed += size
	}
	return reclaimed, firstErr
}

func (m optimizeModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		key := msg.String()
		isQuit := key == "q" || key == "ctrl+c" || key == "esc"
		if m.quitConfirm && !isQuit {
			// Any other key means "keep going".
			m.quitConfirm = false
		}

		switch key {
		case "q", "ctrl+c", "esc":
			// Stopping DISM in the middle of servicing Windows is not
			// something Microsoft documents as safely resumable, so ask first.
			if m.runningComponentStore() && !m.quitConfirm {
				m.quitConfirm = true
				return m, nil
			}
			if m.runningComponentStore() {
				m.abortedCleanup = true
			}
			// optCancel is nil until executeOptimize wires the context; guard
			// so a stray key in a non-running model never panics.
			if optCancel != nil {
				optCancel()
			}
			return m, tea.Quit
		case "enter", "y", "Y":
			if !m.running && m.currentIdx == 0 {
				m.running = true
				m.tasks[0].Status = statusRunning
				m.taskStarted = time.Now()
				return m, tea.Batch(runTaskCmd(0, m.tasks[0], optDryRun, m.isAdmin), optTickCmd())
			}
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case reclaimReportMsg:
		m.reclaim = msg
		m.reclaimReady = true
		return m, nil

	case optTickMsg:
		// Only re-arm while a task is running: the elapsed time is the only
		// thing this tick redraws.
		if m.running {
			return m, optTickCmd()
		}
		return m, nil

	case optTaskProgressMsg:
		if msg.idx == -1 {
			// Triggered initial display delay, wait for user input unless already running
			return m, nil
		}

		// Update completed task status
		m.tasks[msg.idx].Status = msg.status
		m.tasks[msg.idx].Reclaimed = msg.reclaimed
		m.tasks[msg.idx].Note = msg.note
		if msg.err != nil {
			m.tasks[msg.idx].ErrorMsg = msg.err.Error()
		}

		nextIdx := msg.idx + 1
		if nextIdx < len(m.tasks) {
			m.currentIdx = nextIdx
			m.tasks[nextIdx].Status = statusRunning
			m.taskStarted = time.Now()
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
	doc.WriteString(optDividerStyle.Render("  ═══════════════════════════════════════════════════════════════════════") + "\n\n")

	var boxLayout strings.Builder

	if !m.running && m.currentIdx == 0 {
		boxLayout.WriteString("  Optimize operations prepare to execute:\n\n")
		for _, task := range m.tasks {
			boxLayout.WriteString(fmt.Sprintf("    %-40s  %s\n", optWhiteText(task.Name), optGrayText(task.Description)))
		}
		boxLayout.WriteString("\n")
		boxLayout.WriteString(renderReclaimSection(m.reclaim, m.reclaimReady))
		if optDeep {
			boxLayout.WriteString(optWarnStyle.Render("  ⚠️  Component store cleanup can run for tens of minutes.") + "\n")
			boxLayout.WriteString("      " + optGrayText("It removes superseded Windows components at once, without the") + "\n")
			boxLayout.WriteString("      " + optGrayText("30-day grace period Windows' own maintenance task uses.") + "\n")
			boxLayout.WriteString("      " + optGrayText("Updates already installed stay uninstallable.") + "\n\n")
		}
		if !m.isAdmin {
			boxLayout.WriteString(optWarnStyle.Render("  ⚠️  Notice: Duster is running in Standard user mode.") + "\n")
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
				if !m.taskStarted.IsZero() {
					stateStr = optCyanText(fmt.Sprintf("⚡ Running... (%s)", formatElapsed(time.Since(m.taskStarted))))
				}
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
				if (task.ID == "ssd_trim" || task.ID == "component_store") && !m.isAdmin {
					stateStr = optWarnStyle.Render("⚠ Skipped (Needs Administrator)")
				} else if optDryRun {
					stateStr = optWarnStyle.Render("⚠ Skipped (Simulation)")
				} else {
					stateStr = optWarnStyle.Render("⚠ Skipped (Protected path)")
				}
			}

			boxLayout.WriteString(fmt.Sprintf("  %-36s  [ %s ]\n", optWhiteText(task.Name), stateStr))
			boxLayout.WriteString(fmt.Sprintf("      %s\n", optGrayText(task.Description)))
			if task.Note != "" {
				boxLayout.WriteString(fmt.Sprintf("      %s\n", optCyanText(task.Note)))
			}
			boxLayout.WriteString("\n")
			totalReclaimed += task.Reclaimed
		}

		if m.currentIdx == len(m.tasks) {
			boxLayout.WriteString(optDividerStyle.Render("  ───────────────────────────────────────────────────────────────────────") + "\n")
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
		} else if m.quitConfirm {
			boxLayout.WriteString(optWarnStyle.Render("  ⚠️  DISM is servicing Windows right now.") + "\n")
			boxLayout.WriteString("      " + optGrayText("Press q again to stop it, or any other key to keep going.") + "\n")
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
	tasks := optimizeTasks(optDeep)

	// Non-interactive runs have no confirmation step, so require --yes before
	// applying changes (SSD ReTrim runs defrag.exe /O /C on every volume, and
	// the component store cleanup runs for minutes). Otherwise emit the task
	// list as a preview.
	previewOnly := optDryRun || !optAssumeYes

	var totalReclaimed int64
	for i, task := range tasks {
		// The component store task still runs in a preview, because its
		// preview is DISM's read-only analysis: those numbers are the reason
		// to ask for --deep in the first place.
		if previewOnly && task.ID != "component_store" {
			tasks[i].Status = statusSkipped
			continue
		}

		res := execOptimizeTask(task, isAdmin, previewOnly)
		tasks[i].Status = res.status
		tasks[i].Reclaimed = res.reclaimed
		tasks[i].Note = res.note
		if res.err != nil {
			tasks[i].ErrorMsg = res.err.Error()
		}
		totalReclaimed += res.reclaimed
	}

	payload := struct {
		Tasks          []optimizeTask `json:"tasks"`
		TotalReclaimed int64          `json:"total_reclaimed_bytes"`
		AdminElevated  bool           `json:"admin_elevated"`
		Reclaimable    []reclaimItem  `json:"reclaimable"`
		Timestamp      string         `json:"timestamp"`
	}{
		Tasks:          tasks,
		TotalReclaimed: totalReclaimed,
		AdminElevated:  isAdmin,
		Reclaimable:    buildReclaimItems(systemDriveRoot()),
		Timestamp:      time.Now().UTC().Format(time.RFC3339),
	}

	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error encoding JSON: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(string(data))
	// Like doctor, verify and update: a failed task must fail the command, so
	// scripts can tell a failed DNS flush or TRIM from success.
	if anyTaskFailed(tasks) {
		os.Exit(1)
	}
}

func anyTaskFailed(tasks []optimizeTask) bool {
	for _, t := range tasks {
		if t.Status == statusFailed {
			return true
		}
	}
	return false
}

// Local helper style functions — delegate to canonical shared helpers
func optWhiteText(s string) string { return whiteText(s) }
func optCyanText(s string) string  { return cyanText(s) }
func optGrayText(s string) string  { return grayText(s) }
