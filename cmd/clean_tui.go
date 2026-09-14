package cmd

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Nur-Adnan/duster/internal/logging"
	"github.com/Nur-Adnan/duster/lib/elevation"
	"github.com/Nur-Adnan/duster/lib/sysinfo"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ─────────────────────────────────────────────
// UI Styling & Accents — Duster Theme
// ─────────────────────────────────────────────

var (
	colorNeonGreen  = lipgloss.Color("#00FF66") // Neon Green
	colorMagenta    = lipgloss.Color("#00D4FF") // Cyan (section titles / accents)
	colorMutedWhite = lipgloss.Color("#E8E8F0") // Muted White text
	colorCyanAccent = lipgloss.Color("#FFCC00") // Yellow (highlights / metrics)
	colorMutedGray  = lipgloss.Color("#333333") // Subtle Dark Gray

	styleTuiTitle     = lipgloss.NewStyle().Foreground(colorMagenta).Bold(true)
	styleTuiSysInfo   = lipgloss.NewStyle().Foreground(colorMutedWhite)
	styleTuiWhite     = lipgloss.NewStyle().Foreground(colorMutedWhite)
	styleTuiMuted     = lipgloss.NewStyle().Foreground(colorMutedGray)
	styleTuiGreenVal  = lipgloss.NewStyle().Foreground(colorNeonGreen)
	styleTuiHighlight = lipgloss.NewStyle().Foreground(colorCyanAccent).Bold(true)
)

// ─────────────────────────────────────────────
// State Machine Types
// ─────────────────────────────────────────────

type cleanTuiState int

const (
	cleanStateElevation cleanTuiState = iota
	cleanStateScanning
	cleanStateReady
	cleanStateCleaning
	cleanStateDone
	cleanStateRollback
)

// ─────────────────────────────────────────────
// TUI Clean Model Definition
// ─────────────────────────────────────────────

type cleanTuiItem struct {
	ID        string
	Name      string
	Size      int64
	FileCount int
	Checked   bool
	Status    string // "scanning", "ok", "deleting", "done", "skipped", "adminonly", "noaccess", "failed"
	Scanning  bool
	Progress  float64

	cat CleanCategory // re-resolved by scanCmds, so a rescan sees new profile folders
}

type operationsLogEntry struct {
	Timestamp string
	Command   string
	Action    string
	Target    string
	Size      int64
	Status    string
}

type cleanModel struct {
	state         cleanTuiState
	items         []*cleanTuiItem
	cursor        int
	offset        int // first category row on screen when the list scrolls
	width         int
	height        int
	dryRun        bool
	launchDryRun  bool // started with --dry-run: real cleanup ("c") is disabled
	totalScanned  int64
	totalFiles    int
	totalReclaim  int64
	totalReclaimF int // file count
	duration      time.Duration
	startTime     time.Time
	cleanedSize   int64
	cleanedFiles  int
	logLines      []string
	activeItemIdx int

	// Cached startup stats
	osVersion     string
	freeSpace     string
	whitelistText string
	isAdmin       bool

	// Elevation screen fields
	verifyingElevation bool
	elevationError     string
	ramStats           string
	diskStats          string
	uptimeStats        string

	// Rollback log viewer fields
	rollbackLog    []*operationsLogEntry
	rollbackCursor int
}

// ─────────────────────────────────────────────
// Asynchronous Msg Wrapper Structs
// ─────────────────────────────────────────────

type timerTickMsg time.Time
type animateTickMsg time.Time

type cleanScanProgressMsg struct {
	ItemIdx   int
	Size      int64
	FileCount int
	Status    string
	Err       error
}

type cleanDeletionProgressMsg struct {
	ItemIdx    int
	SizeFreed  int64
	FilesFreed int
	Err        error
}

// ─────────────────────────────────────────────
// TUI Command Creators
// ─────────────────────────────────────────────

func timerTickCmd() tea.Cmd {
	return tea.Tick(1*time.Second, func(t time.Time) tea.Msg {
		return timerTickMsg(t)
	})
}

func animateTickCmd() tea.Cmd {
	return tea.Tick(30*time.Millisecond, func(t time.Time) tea.Msg {
		return animateTickMsg(t)
	})
}

// scanItemCmd scans one category. protected comes from whitelistSet, shared
// with the CLI so both accept the same names.
func scanItemCmd(itemIdx int, cat CleanCategory, protected bool) tea.Cmd {
	return func() tea.Msg {
		if protected {
			return cleanScanProgressMsg{
				ItemIdx: itemIdx,
				Status:  "skipped",
			}
		}

		if adminOnlyBlocked(cat) {
			return cleanScanProgressMsg{
				ItemIdx: itemIdx,
				Status:  "adminonly",
			}
		}

		size, files, err := runCategory(cat, true)

		status := "ok"
		if err != nil {
			status = "noaccess"
		}

		return cleanScanProgressMsg{
			ItemIdx:   itemIdx,
			Size:      size,
			FileCount: files,
			Status:    status,
			Err:       err,
		}
	}
}

func cleanItemCmd(itemIdx int, item *cleanTuiItem, isSimulation bool) tea.Cmd {
	return func() tea.Msg {
		if isSimulation {
			return cleanDeletionProgressMsg{
				ItemIdx:    itemIdx,
				SizeFreed:  item.Size,
				FilesFreed: item.FileCount,
			}
		}

		sizeFreed, filesFreed, err := runCategory(item.cat, false)

		return cleanDeletionProgressMsg{
			ItemIdx:    itemIdx,
			SizeFreed:  sizeFreed,
			FilesFreed: filesFreed,
			Err:        err,
		}
	}
}

// ─────────────────────────────────────────────
// Initial Model Creator
// ─────────────────────────────────────────────

func initialCleanModel(startDryRun bool) cleanModel {
	var startState cleanTuiState
	if elevation.IsAdmin() {
		startState = cleanStateScanning
	} else {
		startState = cleanStateElevation
	}

	m := cleanModel{
		state:        startState,
		dryRun:       startDryRun,
		launchDryRun: startDryRun,
		startTime:    time.Now(),
		cursor:       0,
		items:        cleanTuiItems(getCategories()),
	}

	// Set dynamic stats
	stats, err := sysinfo.GetSystemStats()
	osVer := "Windows"
	if err == nil && stats.OSVersion != "" {
		osVer = stats.OSVersion
	}

	freeBytes := getDiskFreeBytes(os.TempDir())
	freeSpaceStr := formatBytes(freeBytes)

	wlText := fmt.Sprintf("%d categories, %d whitelisted", len(m.items), len(whitelist))

	m.osVersion = osVer
	m.freeSpace = freeSpaceStr
	m.whitelistText = wlText
	m.isAdmin = elevation.IsAdmin()

	// Gather stats for Elevation screen cogs
	ramGB := "N/A"
	diskGB := "N/A"
	uptimeStr := "N/A"

	if err == nil {
		if stats.RAMTotal > 0 {
			usedRAM := float64(stats.RAMUsed) / (1024 * 1024 * 1024)
			totalRAM := float64(stats.RAMTotal) / (1024 * 1024 * 1024)
			ramGB = fmt.Sprintf("%.0f/%.0f GB", usedRAM, totalRAM)
		}

		var totalDisk, usedDisk uint64
		for _, disk := range stats.Disks {
			totalDisk += disk.Total
			usedDisk += disk.Used
		}
		if totalDisk > 0 {
			usedGB := float64(usedDisk) / (1024 * 1024 * 1024)
			totalGB := float64(totalDisk) / (1024 * 1024 * 1024)
			diskGB = fmt.Sprintf("%.0f/%.0f GB", usedGB, totalGB)
		}

		if stats.UptimeSeconds > 0 {
			uptimeStr = formatUptime(stats.UptimeSeconds)
		}
	}

	m.ramStats = ramGB
	m.diskStats = diskGB
	m.uptimeStats = uptimeStr

	return m
}

// ─────────────────────────────────────────────
// Bubble Tea Model Lifecycle Methods
// ─────────────────────────────────────────────

func (m cleanModel) Init() tea.Cmd {
	var cmds []tea.Cmd
	cmds = append(cmds, timerTickCmd(), animateTickCmd())

	if m.state != cleanStateElevation {
		cmds = append(cmds, m.scanCmds()...)
	}

	return tea.Batch(cmds...)
}

// cleanTuiItems lists every clean category, ticked, in the CLI's scan order,
// so the TUI offers exactly what `du clean --yes` cleans.
func cleanTuiItems(cats []CleanCategory) []*cleanTuiItem {
	var items []*cleanTuiItem
	for _, c := range groupedCategories(cats) {
		items = append(items, &cleanTuiItem{ID: c.ID, Name: c.Name, Checked: true, Status: "scanning", Scanning: true, cat: c})
	}
	return items
}

// scanCmds starts a scan of every item from the top of the list. Categories
// and the whitelist are resolved once per scan, not once per item.
func (m *cleanModel) scanCmds() []tea.Cmd {
	byID := map[string]CleanCategory{}
	for _, c := range getCategories() {
		byID[c.ID] = c
	}
	protected, _ := whitelistSet(whitelist)
	m.offset = 0

	var cmds []tea.Cmd
	for i, item := range m.items {
		if c, ok := byID[item.ID]; ok {
			item.cat = c
		}
		item.Status = "scanning"
		item.Scanning = true
		item.Progress = 0.0
		cmds = append(cmds, scanItemCmd(i, item.cat, protected[item.ID]))
	}
	return cmds
}

// listWindow returns the first category row to draw and how many rows fit.
// Bubble Tea drops lines above the top of the terminal, which would hide the
// header, so a list taller than the screen scrolls and keeps two lines for
// the "N more" hints.
func (m cleanModel) listWindow() (start, rows int) {
	n := len(m.items)
	if m.height <= 0 {
		return 0, n
	}
	probe := m
	probe.height, probe.items = 0, nil
	free := m.height - strings.Count(probe.View(), "\n") - 1 // View ends in "\n": one more line
	if n <= free {
		return 0, n
	}
	rows = max(1, free-2)
	return min(max(m.offset, 0), n-rows), rows
}

// scrollTo moves the list window just enough to show row idx.
func (m *cleanModel) scrollTo(idx int) {
	start, rows := m.listWindow()
	switch {
	case idx < start:
		m.offset = idx
	case idx >= start+rows:
		m.offset = idx - rows + 1
	default:
		m.offset = start
	}
}

func scrollHint(arrow string, n int) string {
	if n <= 0 {
		return "\n"
	}
	return "  " + styleMuted.Render(fmt.Sprintf("%s %d more", arrow, n)) + "\n"
}

type elevationVerificationMsg struct {
	Success bool
	Err     string
}

func verifyElevationCmd() tea.Cmd {
	return func() tea.Msg {
		err := elevation.RequestElevation()
		if err != nil {
			return elevationVerificationMsg{Success: false, Err: fmt.Sprintf("Elevation failed: %v", err)}
		}
		// A new elevated process has been launched via ShellExecuteW "runas".
		// Report success so Update can quit cleanly — never os.Exit here, which
		// would skip Bubble Tea's terminal restore and leave the console in
		// alt-screen raw mode.
		return elevationVerificationMsg{Success: true}
	}
}

func (m cleanModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		if m.state == cleanStateReady {
			m.scrollTo(m.cursor)
		}
		return m, nil

	case timerTickMsg:
		// Keep the tick chain alive across all states — it previously died on
		// the first tick in Ready and never re-armed, freezing "Time taken"
		// at the scan duration for the whole cleaning phase.
		if m.state == cleanStateScanning || m.state == cleanStateCleaning {
			m.duration = time.Since(m.startTime)
		}
		return m, timerTickCmd()

	case animateTickMsg:
		if m.state == cleanStateScanning {
			// Increment scanning progress for any item that is scanning
			for _, item := range m.items {
				if item.Scanning {
					item.Progress += 5.0
					if item.Progress > 95 {
						item.Progress = 95
					}
				}
			}
			return m, animateTickCmd()
		}

		if m.state == cleanStateCleaning {
			// The active item started deleting when it became active; its bar
			// only shows motion (capped at 95%) until the deletion reports.
			if m.activeItemIdx >= 0 && m.activeItemIdx < len(m.items) {
				item := m.items[m.activeItemIdx]
				if item.Status == "deleting" && item.Progress < 95 {
					item.Progress += 8.0
					if item.Progress > 95 {
						item.Progress = 95
					}
				}
			}
			return m, animateTickCmd()
		}
		return m, nil

	case elevationVerificationMsg:
		m.verifyingElevation = false
		if msg.Success {
			// The elevated instance now owns the session; this non-elevated
			// process exits after Bubble Tea restores the terminal.
			return m, tea.Quit
		}
		m.elevationError = msg.Err
		return m, nil

	case cleanScanProgressMsg:
		item := m.items[msg.ItemIdx]
		item.Scanning = false
		item.Status = msg.Status
		item.Size = msg.Size
		item.FileCount = msg.FileCount
		item.Progress = 100.0 // set progress to 100% since scan is done!

		// Check if scanning is fully complete
		allDone := true
		var totalScanned int64
		var totalFiles int
		for _, itm := range m.items {
			if itm.Scanning {
				allDone = false
			} else if itm.Status == "ok" {
				totalScanned += itm.Size
				totalFiles += itm.FileCount
			}
		}

		m.totalScanned = totalScanned
		m.totalFiles = totalFiles

		if allDone {
			m.state = cleanStateReady
			m.recalculateReclaim()
		} else {
			m.recalculateReclaim()
		}
		return m, nil

	case cleanDeletionProgressMsg:
		item := m.items[msg.ItemIdx]
		item.Status = "done"
		item.Progress = 100.0

		// Partial frees count even when the category reports failures.
		m.cleanedSize += msg.SizeFreed
		m.cleanedFiles += msg.FilesFreed
		if msg.Err == nil {
			verb := "freed"
			if m.dryRun {
				verb = "would be freed"
			}
			m.logLines = append(m.logLines, fmt.Sprintf("✓ %s: %s %s (%d files)", item.Name, formatBytes(msg.SizeFreed), verb, msg.FilesFreed))
		} else {
			item.Status = "failed"
			m.logLines = append(m.logLines, fmt.Sprintf("✗ %s: %s freed, incomplete: %v", item.Name, formatBytes(msg.SizeFreed), msg.Err))
		}

		// Start the next checked item. The animation tick chain is still
		// running, so arming another tick here would double the bar speed.
		return m, m.beginNextItem()

	case tea.KeyMsg:
		keyStr := msg.String()

		switch keyStr {
		case "q", "ctrl+c":
			return m, tea.Quit

		case "b", "esc":
			if m.state == cleanStateReady || m.state == cleanStateDone {
				return m, tea.Quit
			}
		}

		if m.state == cleanStateElevation {
			switch keyStr {
			case "enter":
				if !m.verifyingElevation {
					m.verifyingElevation = true
					m.elevationError = ""
					return m, verifyElevationCmd()
				}
			case "esc":
				return m, tea.Quit
			}
			return m, nil
		}

		if m.state == cleanStateRollback {
			switch keyStr {
			case "up", "k":
				if m.rollbackCursor > 0 {
					m.rollbackCursor--
				}
			case "down", "j":
				if m.rollbackCursor < len(m.rollbackLog)-1 {
					m.rollbackCursor++
				}
			case "esc", "b", "B":
				m.state = cleanStateReady
			}
			return m, nil
		}

		if m.state == cleanStateReady {
			switch keyStr {
			case "up", "k":
				if m.cursor > 0 {
					m.cursor--
				}
				m.scrollTo(m.cursor)
			case "down", "j":
				if m.cursor < len(m.items)-1 {
					m.cursor++
				}
				m.scrollTo(m.cursor)

			case " ", "space":
				// Bubble Tea reports the space key as " ", never "space"; the
				// old match made category deselection impossible.
				if m.cursor >= 0 && m.cursor < len(m.items) {
					item := m.items[m.cursor]
					item.Checked = !item.Checked
					m.recalculateReclaim()
				}

			case "enter":
				// Honor the launch flag: `clean --dry-run` seeds m.dryRun=true and
				// Enter must respect it. Users can still force a real clean with "c"
				// or an explicit dry run with "d".
				m.startCleanup()
				if cmd := m.beginNextItem(); cmd != nil {
					return m, tea.Batch(animateTickCmd(), cmd)
				}

			case "v", "V":
				m.rollbackLog = readOperationsLog()
				m.rollbackCursor = 0
				m.state = cleanStateRollback

			case "d", "D":
				// Run in Dry Run mode
				m.dryRun = true
				m.startCleanup()
				if cmd := m.beginNextItem(); cmd != nil {
					return m, tea.Batch(animateTickCmd(), cmd)
				}

			case "r", "R":
				// Rescan all
				m.state = cleanStateScanning
				m.totalScanned = 0
				m.totalFiles = 0
				m.totalReclaim = 0
				m.totalReclaimF = 0
				m.cursor = 0
				m.startTime = time.Now()

				var cmds []tea.Cmd
				// The timer chain from Init() is immortal (timerTickMsg always
				// re-arms), so only restart the animation here; re-adding
				// timerTickCmd would spawn a parallel forever-ticking chain.
				cmds = append(cmds, animateTickCmd())
				cmds = append(cmds, m.scanCmds()...)
				return m, tea.Batch(cmds...)

			case "c", "C":
				// A --dry-run launch is a promise that nothing is deleted.
				if m.launchDryRun {
					break
				}
				// Real execution
				m.dryRun = false
				m.startCleanup()
				if cmd := m.beginNextItem(); cmd != nil {
					return m, tea.Batch(animateTickCmd(), cmd)
				}
			}
		} else if m.state == cleanStateDone {
			// The window stays where cleaning left it; scrolling reaches the
			// other rows, such as a failed category above it.
			switch keyStr {
			case "up", "k":
				start, _ := m.listWindow()
				m.offset = max(start-1, 0)
			case "down", "j":
				start, rows := m.listWindow()
				m.offset = min(start+1, len(m.items)-rows)
			}
			if keyStr == "r" || keyStr == "R" {
				m.state = cleanStateScanning
				m.totalScanned = 0
				m.totalFiles = 0
				m.totalReclaim = 0
				m.totalReclaimF = 0
				m.cleanedSize = 0
				m.cleanedFiles = 0
				m.logLines = nil
				m.startTime = time.Now()
				m.cursor = 0

				var cmds []tea.Cmd
				// See the Ready-state rescan: reuse the immortal timer chain,
				// restart only the animation to avoid a duplicate ticker.
				cmds = append(cmds, animateTickCmd())
				cmds = append(cmds, m.scanCmds()...)
				return m, tea.Batch(cmds...)
			}
		}
	}
	return m, nil
}

// ─────────────────────────────────────────────
// TUI Controller Helpers
// ─────────────────────────────────────────────

// beginNextItem starts deleting the next checked item right away; its bar
// animates while the deletion runs. With nothing left it ends the run and
// returns nil. Callers arm the animation tick only when starting a run: the
// chain then lives until the run ends.
func (m *cleanModel) beginNextItem() tea.Cmd {
	idx, item := m.getNextItemToClean()
	if item == nil {
		m.state = cleanStateDone
		return nil
	}
	item.Status = "deleting"
	item.Progress = 0
	m.activeItemIdx = idx
	m.scrollTo(idx)
	return cleanItemCmd(idx, item, m.dryRun)
}

func (m *cleanModel) recalculateReclaim() {
	var total int64
	var count int
	for _, item := range m.items {
		if item.Checked && (item.Status == "ok" || item.Status == "done" || item.Status == "scanning") {
			total += item.Size
			count += item.FileCount
		}
	}
	m.totalReclaim = total
	m.totalReclaimF = count
}

func (m *cleanModel) startCleanup() {
	m.state = cleanStateCleaning
	m.startTime = time.Now()
	m.cleanedSize = 0
	m.cleanedFiles = 0
	m.logLines = []string{}
	m.logLines = append(m.logLines, "⚡ Starting Duster Deep Clean...")
}

func (m *cleanModel) getNextItemToClean() (int, *cleanTuiItem) {
	for iIdx, item := range m.items {
		if item.Checked && item.Status != "done" && item.Status != "deleting" && item.Status != "skipped" && item.Status != "adminonly" && item.Status != "noaccess" && item.Status != "failed" {
			return iIdx, item
		}
	}
	return -1, nil
}

// ─────────────────────────────────────────────
// Layout & Formatting Engine
// ─────────────────────────────────────────────

func padLeft(s string, width int) string {
	n := utf8.RuneCountInString(s)
	if n >= width {
		return s
	}
	return strings.Repeat(" ", width-n) + s
}

func (m cleanModel) View() string {
	var sb strings.Builder

	width := m.width
	if width <= 0 {
		width = 80
	}

	if m.state == cleanStateElevation {
		return m.renderElevationScreen(width)
	}

	if m.state == cleanStateRollback {
		return m.renderRollbackScreen(width)
	}

	// 1. Brand Header
	sb.WriteString(RenderHeaderWithSubtitle(width, "du clean", "Cache Cleaner", "Scanning & cleaning system cache..."))

	// 2. Status Banner
	var bannerText string
	if m.state == cleanStateScanning {
		bannerText = "  " + styleMuted.Render("[") + styleAccent.Render("i") + styleMuted.Render("]") + " " + styleValue.Render("Scanning system for cache files...") + "\n\n"
	} else if m.state == cleanStateReady {
		bannerText = "  " + styleMuted.Render("[") + styleAccent.Render("i") + styleMuted.Render("]") + " " + styleValue.Render("System scan complete. Ready for cleanup.") + "\n\n"
	} else if m.state == cleanStateCleaning {
		activeName := ""
		if m.activeItemIdx >= 0 && m.activeItemIdx < len(m.items) {
			activeName = m.items[m.activeItemIdx].Name
		}
		bannerText = "  " + styleMuted.Render("[") + styleAccent.Render("i") + styleMuted.Render("]") + " " + styleValue.Render("Cleaning: "+activeName+"...") + "\n\n"
	} else if m.state == cleanStateDone && m.dryRun {
		bannerText = "  " + styleMuted.Render("[") + styleAccent.Render("i") + styleMuted.Render("]") + " " + styleValue.Render("Dry run complete: nothing was deleted.") + "\n\n"
	} else if m.state == cleanStateDone {
		bannerText = "  " + styleMuted.Render("[") + styleSuccess.Render("✓") + styleMuted.Render("]") + " " + styleSuccess.Render("System cache cleaned successfully!") + "\n\n"
	}
	sb.WriteString(bannerText)

	// 3. Table Column Headers
	sb.WriteString("  " + styleHeader.Render("Category") + strings.Repeat(" ", 25) +
		styleHeader.Render("Status") + strings.Repeat(" ", 44) +
		styleHeader.Render("Files") + strings.Repeat(" ", 9) +
		styleHeader.Render("Size") + "\n")

	// 4. Flat Monospace Clean List Table
	start, rows := m.listWindow()
	if rows < len(m.items) {
		sb.WriteString(scrollHint("↑", start))
	}
	for index := start; index < start+rows; index++ {
		item := m.items[index]
		var catPrefix string
		var selected = m.state == cleanStateReady && index == m.cursor

		if m.state == cleanStateReady {
			if selected {
				if item.Checked {
					catPrefix = styleAccent.Render("\u27a4 [x] ")
				} else {
					catPrefix = styleAccent.Render("\u27a4 [ ] ")
				}
			} else {
				if item.Checked {
					catPrefix = styleSuccess.Render("  [x] ")
				} else {
					catPrefix = styleTuiMuted.Render("  [ ] ")
				}
			}
		} else {
			catPrefix = fmt.Sprintf(" %2d. ", index+1)
		}

		catStr := catPrefix + item.Name
		catStr = padRight(catStr, 32)

		var statusStr string
		switch item.Status {
		case "scanning":
			statusStr = styleSuccess.Render("Scanning")
		case "deleting":
			statusStr = styleSuccess.Render("Cleaning")
		case "done":
			statusStr = styleSuccess.Render("Done")
		case "skipped":
			statusStr = styleTuiMuted.Render("Skipped")
		case "adminonly", "noaccess":
			statusStr = styleWarning.Render("Protected")
		case "failed":
			statusStr = styleDanger.Render("Failed")
		default:
			statusStr = styleSuccess.Render("Done")
		}

		statusRendered := statusStr + strings.Repeat(" ", 12-utf8.RuneCountInString(stripAnsi(statusStr)))

		percentStr := fmt.Sprintf("%3d%%", int(item.Progress))

		// Formulating files and size metrics
		filesVal := "--"
		sizeVal := "--"
		if !item.Scanning && item.Status != "scanning" {
			if item.FileCount > 0 {
				filesVal = formatInt(item.FileCount)
			} else {
				filesVal = "0"
			}
			if item.Size > 0 {
				sizeVal = formatBytes(item.Size)
			} else {
				sizeVal = "0 B"
			}
		}

		filesRendered := padLeft(filesVal, 10)
		sizeRendered := padLeft(sizeVal, 12)

		var nameColorized = styleTuiWhite.Render(catStr)
		if selected {
			nameColorized = styleTuiHighlight.Render(catStr)
		}

		sb.WriteString(fmt.Sprintf("  %s %s %s   %s   %s %s\n",
			nameColorized,
			statusRendered,
			progressBar(item.Progress, 20),
			styleWarning.Render(percentStr),
			styleTuiWhite.Render(filesRendered),
			styleTuiWhite.Render(sizeRendered),
		))
	}
	if rows < len(m.items) {
		sb.WriteString(scrollHint("↓", len(m.items)-start-rows))
	}

	dividerWidth := width - 4
	if dividerWidth < 80 {
		dividerWidth = 80
	}
	sb.WriteString("\n  " + styleDivider.Render(strings.Repeat("─", dividerWidth)) + "\n\n")

	// 5. Cleanup Summary Section (with pixel-perfect aligned colons)
	durSec := int(m.duration.Seconds())
	h := durSec / 3600
	min := (durSec % 3600) / 60
	sec := durSec % 60
	durStr := fmt.Sprintf("%02d:%02d:%02d", h, min, sec)

	var spaceStr string
	var filesStr string
	var statusSummary string

	if m.state == cleanStateScanning {
		spaceStr = "--"
		filesStr = "--"
		statusSummary = "Scanning..."
	} else if m.state == cleanStateCleaning {
		spaceStr = formatBytes(m.cleanedSize)
		filesStr = formatInt(m.cleanedFiles)
		statusSummary = "Cleaning..."
	} else if m.state == cleanStateDone {
		spaceStr = formatBytes(m.cleanedSize)
		filesStr = formatInt(m.cleanedFiles)
		statusSummary = "Completed successfully!"
		if m.dryRun {
			statusSummary = "Dry run complete"
		}
	} else {
		// Ready state
		spaceStr = formatBytes(m.totalReclaim)
		filesStr = formatInt(m.totalReclaimF)
		statusSummary = "Ready to clean"
	}

	lblStyle := styleTuiWhite
	valStyle := styleSuccess // neon green

	padLabel := func(label string, width int) string {
		padded := padRight(label, width)
		return lblStyle.Render(padded)
	}

	// A dry run, or a scan not yet cleaned, has removed nothing.
	spaceLabel, filesLabel := "Total space recovered", "Total files removed"
	if m.dryRun || m.state == cleanStateScanning || m.state == cleanStateReady {
		spaceLabel, filesLabel = "Space to recover", "Files to remove"
	}
	sb.WriteString(fmt.Sprintf("  🗑  %s :  %s\n", padLabel(spaceLabel, 22), valStyle.Render(spaceStr)))
	sb.WriteString(fmt.Sprintf("  📄  %s :  %s\n", padLabel(filesLabel, 22), valStyle.Render(filesStr)))
	checked := 0
	for _, item := range m.items {
		if item.Checked {
			checked++
		}
	}
	sb.WriteString(fmt.Sprintf("  ☰  %s :  %s\n", padLabel("Categories", 22), valStyle.Render(fmt.Sprintf("%d of %d selected", checked, len(m.items)))))
	sb.WriteString(fmt.Sprintf("  🕒  %s :  %s\n", padLabel("Time taken", 22), valStyle.Render(durStr)))

	statusColored := valStyle.Render(statusSummary)
	if m.state == cleanStateScanning || m.state == cleanStateCleaning {
		statusColored = styleAccent.Render(statusSummary)
	}
	sb.WriteString(fmt.Sprintf("  ✓  %s :  %s\n\n", padLabel("Status", 22), statusColored))

	// 6. Footer Navigation
	sb.WriteString("  " + styleDivider.Render(strings.Repeat("─", dividerWidth)) + "\n\n")

	formatShortcut := func(key, name string) string {
		return styleAccent.Render("[") + styleSuccess.Render(key) + styleAccent.Render("] ") + styleTuiWhite.Render(name)
	}

	var hints []string
	if m.state == cleanStateScanning {
		hints = []string{
			formatShortcut("Q", "Quit"),
		}
	} else if m.state == cleanStateCleaning {
		hints = []string{
			formatShortcut("Q", "Quit"),
		}
	} else if m.state == cleanStateDone {
		hints = []string{
			formatShortcut("↑↓", "Scroll"),
			formatShortcut("R", "Run again"),
			formatShortcut("B", "Back to menu"),
			formatShortcut("Q", "Quit"),
		}
	} else {
		hints = []string{
			formatShortcut("Space", "Toggle"),
			formatShortcut("Enter", "Clean"),
			formatShortcut("D", "Dry Run"),
			formatShortcut("C", "Force real"),
			formatShortcut("R", "Rescan"),
			formatShortcut("V", "History"),
			formatShortcut("B", "Back to menu"),
			formatShortcut("Q", "Quit"),
		}
	}

	sb.WriteString("  " + strings.Join(hints, "    ") + "\n\n")

	// Emulate the authentic Windows Command prompt cursor at the very bottom
	sb.WriteString("  " + styleTuiWhite.Render("C:\\>") + styleSuccess.Render("█") + "\n")

	return sb.String()
}

// ─────────────────────────────────────────────
// Standard Secondary Screens (Elevation & Rollback)
// ─────────────────────────────────────────────

func (m cleanModel) renderElevationScreen(width int) string {
	if width < 24 {
		width = 24 // strings.Repeat(…, width-4) must never go negative
	}

	var sb strings.Builder
	sb.WriteString("\n")
	sb.WriteString("  " + styleTuiTitle.Render("Optimize and Check") + "\n\n")

	cogIcon := "⚙"
	sysLBL := styleTuiSysInfo.Render(cogIcon + " System  ")

	ramText := fmt.Sprintf(" %s RAM", styleTuiGreenVal.Render(m.ramStats))
	diskText := fmt.Sprintf(" %s Disk", styleTuiGreenVal.Render(m.diskStats))
	uptimeText := fmt.Sprintf(" Uptime %s", styleTuiGreenVal.Render(m.uptimeStats))

	sb.WriteString(fmt.Sprintf("  %s %s | %s | %s\n", sysLBL, ramText, diskText, uptimeText))

	wlLBL := styleTuiSysInfo.Render(cogIcon + " Active Whitelist: ")
	sb.WriteString(fmt.Sprintf("  %s%s\n\n", wlLBL, styleTuiWhite.Render(m.whitelistText)))

	arrowIcon := "➤"
	sb.WriteString("  " + styleTuiHighlight.Render(arrowIcon+" Deep cleaning requires administrator access") + "\n")

	// Elevation happens through the standard Windows UAC dialog — Duster
	// never reads a password itself, so never render a password prompt.
	if m.verifyingElevation {
		sb.WriteString("  " + styleTuiHighlight.Render(arrowIcon+" Waiting for Windows UAC approval... ") + styleTuiGreenVal.Render("[░░░░░░░░░░]") + "\n")
	} else {
		sb.WriteString("  " + styleTuiWhite.Render(arrowIcon+" Press Enter to relaunch elevated (a Windows UAC prompt will appear)") + "\n")
	}

	if m.elevationError != "" {
		sb.WriteString("\n  " + styleDanger.Render("✗ "+m.elevationError) + "\n")
	} else {
		sb.WriteString("\n\n")
	}

	lblStyle := lipgloss.NewStyle().Foreground(colorMutedWhite)
	keyStyle := lipgloss.NewStyle().Foreground(colorCyanAccent).Bold(true)
	divStyle := lipgloss.NewStyle().Foreground(colorMutedGray)

	hints := []string{
		keyStyle.Render("Enter") + " " + lblStyle.Render("Elevate"),
		keyStyle.Render("Esc") + " " + lblStyle.Render("Cancel"),
	}
	hintsStr := strings.Join(hints, divStyle.Render("  │  "))
	hintsLen := utf8.RuneCountInString(stripAnsi(hintsStr))
	hintsPadding := ""
	if width > hintsLen {
		hintsPadding = strings.Repeat(" ", (width-hintsLen)/2)
	}
	sb.WriteString("\n" + styleTuiMuted.Render(strings.Repeat("─", width-4)) + "\n")
	sb.WriteString(hintsPadding + hintsStr + "\n\n")

	helpTip := "Elevation uses the standard Windows UAC prompt; Duster never sees your password."
	tipLen := len(helpTip)
	tipPadding := ""
	if width > tipLen {
		tipPadding = strings.Repeat(" ", (width-tipLen)/2)
	}
	sb.WriteString(tipPadding + styleTuiMuted.Render(helpTip) + "\n")

	return sb.String()
}

func (m cleanModel) renderRollbackScreen(width int) string {
	if width < 50 {
		width = 50 // guards strings.Repeat(…, width-4) and truncateString(…, width-45)
	}

	var sb strings.Builder
	sb.WriteString("\n")
	sb.WriteString("  " + styleTuiTitle.Render("Operations History") + "\n\n")

	if len(m.rollbackLog) == 0 {
		sb.WriteString("  " + styleTuiMuted.Render("No recent destructive operations found in log file.") + "\n")
		sb.WriteString("  " + styleTuiMuted.Render("Operations are logged to %LOCALAPPDATA%\\Duster\\operations.log") + "\n\n")
	} else {
		// Read-only view. Deleted cache files cannot be restored, so this
		// screen must never advertise a rollback action it can't perform.
		sb.WriteString("  " + styleTuiMuted.Render("Read-only audit trail of past cleanup operations:") + "\n\n")

		sb.WriteString("  " + styleTuiHighlight.Render("Timestamp") + "            │ " +
			styleTuiHighlight.Render("Action") + " │ " +
			styleTuiHighlight.Render("Reclaimed") + "  │ " +
			styleTuiHighlight.Render("Target Category") + "\n")
		sb.WriteString("  " + styleTuiMuted.Render(strings.Repeat("─", width-4)) + "\n")

		maxVisible := 8
		start := m.rollbackCursor - maxVisible/2
		if start < 0 {
			start = 0
		}
		if start+maxVisible > len(m.rollbackLog) {
			start = len(m.rollbackLog) - maxVisible
			if start < 0 {
				start = 0
			}
		}

		for i := start; i < start+maxVisible && i < len(m.rollbackLog); i++ {
			entry := m.rollbackLog[i]
			selected := i == m.rollbackCursor

			arrow := "  "
			rowStyle := styleTuiWhite
			if selected {
				arrow = "➤ "
				rowStyle = styleTuiHighlight
			}

			tStr := entry.Timestamp
			if len(tStr) > 19 {
				tStr = tStr[:19]
			}

			actStr := padRight(entry.Action, 6)
			sizeStr := padRight(formatBytes(entry.Size), 10)
			tgtStr := truncateString(entry.Target, width-45)

			sb.WriteString("  " + rowStyle.Render(arrow) +
				rowStyle.Render(tStr) + " │ " +
				rowStyle.Render(actStr) + " │ " +
				rowStyle.Render(sizeStr) + " │ " +
				rowStyle.Render(tgtStr) + "\n")
		}

		if len(m.rollbackLog) > maxVisible {
			sb.WriteString("  " + styleTuiMuted.Render(fmt.Sprintf("  (Showing %d-%d of %d logged entries, use Up/Down to scroll)",
				start+1, min(start+maxVisible, len(m.rollbackLog)), len(m.rollbackLog))) + "\n")
		}
	}

	lblStyle := lipgloss.NewStyle().Foreground(colorMutedWhite)
	keyStyle := lipgloss.NewStyle().Foreground(colorCyanAccent).Bold(true)
	divStyle := lipgloss.NewStyle().Foreground(colorMutedGray)

	hints := []string{
		keyStyle.Render("Esc / B") + " " + lblStyle.Render("Back"),
	}
	hintsStr := strings.Join(hints, divStyle.Render("  │  "))
	hintsLen := utf8.RuneCountInString(stripAnsi(hintsStr))
	hintsPadding := ""
	if width > hintsLen {
		hintsPadding = strings.Repeat(" ", (width-hintsLen)/2)
	}
	sb.WriteString("\n" + styleTuiMuted.Render(strings.Repeat("─", width-4)) + "\n")
	sb.WriteString(hintsPadding + hintsStr + "\n")

	return sb.String()
}

// ─────────────────────────────────────────────
// Clean TUI Entry Point function
// ─────────────────────────────────────────────

func runCleanTUI(startDryRun bool) {
	m := initialCleanModel(startDryRun)
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running clean TUI: %v\n", err)
		os.Exit(1)
	}
}

// ─────────────────────────────────────────────
// Regex & ANSI helpers
// ─────────────────────────────────────────────

var ansiRegex = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

func stripAnsi(str string) string {
	return ansiRegex.ReplaceAllString(str, "")
}

func getOperationsLogPath() string {
	dir := logging.Dir()
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, "operations.log")
}

func readOperationsLog() []*operationsLogEntry {
	path := getOperationsLogPath()
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	var entries []*operationsLogEntry
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		entry := parseLogLine(line)
		if entry != nil {
			entries = append(entries, entry)
		}
	}

	// Reverse entries so the most recent is first
	for i, j := 0, len(entries)-1; i < j; i, j = i+1, j-1 {
		entries[i], entries[j] = entries[j], entries[i]
	}

	return entries
}

func parseLogLine(line string) *operationsLogEntry {
	parts := strings.Split(line, " | ")
	if len(parts) < 6 {
		return nil
	}

	entry := &operationsLogEntry{
		Timestamp: parts[0],
	}

	for _, part := range parts[1:] {
		subparts := strings.SplitN(part, ": ", 2)
		if len(subparts) != 2 {
			continue
		}
		key := strings.TrimSpace(subparts[0])
		val := strings.TrimSpace(subparts[1])

		switch key {
		case "Command":
			entry.Command = val
		case "Action":
			entry.Action = val
		case "Target":
			entry.Target = val
		case "Size":
			val = strings.TrimSuffix(val, " bytes")
			size, _ := strconv.ParseInt(val, 10, 64)
			entry.Size = size
		case "Status":
			entry.Status = val
		}
	}

	return entry
}

func formatUptime(uptimeSeconds uint64) string {
	days := uptimeSeconds / 86400
	if days > 0 {
		return fmt.Sprintf("%dd", days)
	}
	hours := (uptimeSeconds % 86400) / 3600
	if hours > 0 {
		return fmt.Sprintf("%dh", hours)
	}
	mins := (uptimeSeconds % 3600) / 60
	return fmt.Sprintf("%dm", mins)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
