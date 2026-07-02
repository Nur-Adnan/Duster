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
	Status    string // "scanning", "ok", "cleaning", "deleting", "done", "skipped", "adminonly", "noaccess", "failed"
	Scanning  bool
	Progress  float64
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
	width         int
	height        int
	dryRun        bool
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

func scanItemCmd(itemIdx int, item *cleanTuiItem) tea.Cmd {
	return func() tea.Msg {
		var size int64
		var files int
		var err error

		// Whitelist guard
		whitelistMap := make(map[string]bool)
		for _, id := range whitelist {
			whitelistMap[strings.ToLower(strings.TrimSpace(id))] = true
		}

		// The --whitelist flag documents getCategories() IDs, but this TUI's
		// combined "logs" item spans two of them, and users may pass browser
		// names. Map both vocabularies onto TUI item IDs so protection always
		// errs on the side of skipping more, never less.
		aliases := map[string][]string{
			"chrome":   {"browsers"},
			"edge":     {"browsers"},
			"brave":    {"browsers"},
			"firefox":  {"browsers"},
			"wer":      {"logs"},
			"logfiles": {"logs"},
		}
		for id := range whitelistMap {
			for _, tuiID := range aliases[id] {
				whitelistMap[tuiID] = true
			}
		}

		if whitelistMap[item.ID] {
			return cleanScanProgressMsg{
				ItemIdx: itemIdx,
				Status:  "skipped",
			}
		}

		if item.ID == "prefetch" && !elevation.IsAdmin() {
			return cleanScanProgressMsg{
				ItemIdx: itemIdx,
				Status:  "adminonly",
			}
		}

		switch item.ID {
		case "logs":
			// Scan both "wer" and "logfiles" and sum them up!
			var size1, size2 int64
			var files1, files2 int
			var err1, err2 error

			// wer
			var werCat *CleanCategory
			for _, c := range getCategories() {
				if c.ID == "wer" {
					werCat = &c
					break
				}
			}
			if werCat != nil {
				size1, files1, err1 = scanDirCategory(*werCat)
			}

			// logfiles
			var logfilesCat *CleanCategory
			for _, c := range getCategories() {
				if c.ID == "logfiles" {
					logfilesCat = &c
					break
				}
			}
			if logfilesCat != nil {
				size2, files2, err2 = scanDirCategory(*logfilesCat)
			}

			size = size1 + size2
			files = files1 + files2
			if err1 != nil {
				err = err1
			} else if err2 != nil {
				err = err2
			}
		default:
			var matchedCat *CleanCategory
			for _, c := range getCategories() {
				if c.ID == item.ID {
					matchedCat = &c
					break
				}
			}
			if matchedCat != nil {
				if matchedCat.CustomScan != nil {
					size, files, err = matchedCat.CustomScan(true, false)
				} else {
					size, files, err = scanDirCategory(*matchedCat)
				}
			}
		}

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

		var sizeFreed int64
		var filesFreed int
		var err error

		switch item.ID {
		case "logs":
			var size1, size2 int64
			var files1, files2 int
			var err1, err2 error

			// wer
			var werCat *CleanCategory
			for _, c := range getCategories() {
				if c.ID == "wer" {
					werCat = &c
					break
				}
			}
			if werCat != nil {
				size1, files1, err1 = cleanDirCategory(*werCat)
			}

			// logfiles
			var logfilesCat *CleanCategory
			for _, c := range getCategories() {
				if c.ID == "logfiles" {
					logfilesCat = &c
					break
				}
			}
			if logfilesCat != nil {
				size2, files2, err2 = cleanDirCategory(*logfilesCat)
			}

			sizeFreed = size1 + size2
			filesFreed = files1 + files2
			if err1 != nil {
				err = err1
			} else if err2 != nil {
				err = err2
			}
		default:
			var matchedCat *CleanCategory
			for _, c := range getCategories() {
				if c.ID == item.ID {
					matchedCat = &c
					break
				}
			}
			if matchedCat != nil {
				if matchedCat.CustomScan != nil {
					sizeFreed, filesFreed, err = matchedCat.CustomScan(false, false)
				} else {
					sizeFreed, filesFreed, err = cleanDirCategory(*matchedCat)
				}
			}
		}

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
		state:     startState,
		dryRun:    startDryRun,
		startTime: time.Now(),
		cursor:    0,
		items: []*cleanTuiItem{
			{ID: "temp", Name: "Windows Temp Files", Checked: true, Status: "scanning", Scanning: true, Progress: 0.0},
			{ID: "prefetch", Name: "Prefetch Files", Checked: true, Status: "scanning", Scanning: true, Progress: 0.0},
			{ID: "update", Name: "Windows Update Cache", Checked: true, Status: "scanning", Scanning: true, Progress: 0.0},
			// One item backed by the canonical "browsers" category so the TUI
			// cleans the same set as CLI mode (Chrome, Edge, Brave, Firefox);
			// the old hardcoded chrome/edge items silently skipped the rest.
			{ID: "browsers", Name: "Browser Caches", Checked: true, Status: "scanning", Scanning: true, Progress: 0.0},
			{ID: "thumbs", Name: "Thumbnail Cache", Checked: true, Status: "scanning", Scanning: true, Progress: 0.0},
			{ID: "delivery_opt", Name: "Delivery Optimization Cache", Checked: true, Status: "scanning", Scanning: true, Progress: 0.0},
			{ID: "dns", Name: "DNS Cache", Checked: true, Status: "scanning", Scanning: true, Progress: 0.0},
			{ID: "recycle", Name: "Recycle Bin", Checked: true, Status: "scanning", Scanning: true, Progress: 0.0},
			{ID: "logs", Name: "Logs (System & Apps)", Checked: true, Status: "scanning", Scanning: true, Progress: 0.0},
		},
	}

	// Set dynamic stats
	stats, err := sysinfo.GetSystemStats()
	osVer := "Windows"
	if err == nil && stats.OSVersion != "" {
		osVer = stats.OSVersion
	}

	freeBytes := getDiskFreeBytes(os.TempDir())
	freeSpaceStr := formatBytes(freeBytes)

	wlText := fmt.Sprintf("%d protected paths, %d whitelisted categories", len(getCategories()), len(whitelist))

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
		// Queue scans for all items
		for iIdx, item := range m.items {
			cmds = append(cmds, scanItemCmd(iIdx, item))
		}
	}

	return tea.Batch(cmds...)
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
			// Find the active item being cleaned and animate its bar
			if m.activeItemIdx >= 0 && m.activeItemIdx < len(m.items) {
				item := m.items[m.activeItemIdx]
				if item.Status == "cleaning" {
					item.Progress += 8.0
					if item.Progress >= 100 {
						item.Progress = 100
						// Animation finished, trigger actual deletion
						item.Status = "deleting"
						return m, cleanItemCmd(m.activeItemIdx, item, m.dryRun)
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

		if msg.Err == nil {
			m.cleanedSize += msg.SizeFreed
			m.cleanedFiles += msg.FilesFreed
			verb := "freed"
			if m.dryRun {
				verb = "would be freed"
			}
			m.logLines = append(m.logLines, fmt.Sprintf("✓ %s: %s %s (%d files)", item.Name, formatBytes(msg.SizeFreed), verb, msg.FilesFreed))
		} else {
			item.Status = "failed"
			m.logLines = append(m.logLines, fmt.Sprintf("✗ %s: failed to clean: %v", item.Name, msg.Err))
		}

		// Find and clean next checked item
		nextIdx, nextItem := m.getNextItemToClean()
		if nextItem != nil {
			nextItem.Status = "cleaning"
			nextItem.Progress = 0.0
			m.activeItemIdx = nextIdx
			// Re-arm the animation chain; the tick that reached 100% returned
			// cleanItemCmd instead of a new tick, so without this the next item
			// would never animate or delete (the whole run would stall here).
			return m, animateTickCmd()
		}
		m.state = cleanStateDone
		return m, nil

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
			case "down", "j":
				if m.cursor < len(m.items)-1 {
					m.cursor++
				}

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
				nextIdx, nextItem := m.getNextItemToClean()
				if nextItem != nil {
					nextItem.Status = "cleaning"
					nextItem.Progress = 0.0
					m.activeItemIdx = nextIdx
					return m, animateTickCmd()
				} else {
					m.state = cleanStateDone
				}

			case "v", "V":
				m.rollbackLog = readOperationsLog()
				m.rollbackCursor = 0
				m.state = cleanStateRollback

			case "d", "D":
				// Run in Dry Run mode
				m.dryRun = true
				m.startCleanup()
				nextIdx, nextItem := m.getNextItemToClean()
				if nextItem != nil {
					nextItem.Status = "cleaning"
					nextItem.Progress = 0.0
					m.activeItemIdx = nextIdx
					return m, animateTickCmd()
				} else {
					m.state = cleanStateDone
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
				cmds = append(cmds, timerTickCmd(), animateTickCmd())
				for iIdx, item := range m.items {
					item.Status = "scanning"
					item.Scanning = true
					item.Progress = 0.0
					cmds = append(cmds, scanItemCmd(iIdx, item))
				}
				return m, tea.Batch(cmds...)

			case "c", "C":
				// Real execution
				m.dryRun = false
				m.startCleanup()
				nextIdx, nextItem := m.getNextItemToClean()
				if nextItem != nil {
					nextItem.Status = "cleaning"
					nextItem.Progress = 0.0
					m.activeItemIdx = nextIdx
					return m, animateTickCmd()
				} else {
					m.state = cleanStateDone
				}
			}
		} else if m.state == cleanStateDone {
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
				cmds = append(cmds, timerTickCmd(), animateTickCmd())
				for iIdx, item := range m.items {
					item.Status = "scanning"
					item.Scanning = true
					item.Progress = 0.0
					cmds = append(cmds, scanItemCmd(iIdx, item))
				}
				return m, tea.Batch(cmds...)
			}
		}
	}
	return m, nil
}

// ─────────────────────────────────────────────
// TUI Controller Helpers
// ─────────────────────────────────────────────

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
		if item.Checked && item.Status != "done" && item.Status != "cleaning" && item.Status != "deleting" && item.Status != "skipped" && item.Status != "adminonly" && item.Status != "noaccess" && item.Status != "failed" {
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
	for index, item := range m.items {
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
		case "cleaning", "deleting":
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

	sb.WriteString(fmt.Sprintf("  🗑  %s :  %s\n", padLabel("Total space recovered", 22), valStyle.Render(spaceStr)))
	sb.WriteString(fmt.Sprintf("  📄  %s :  %s\n", padLabel("Total files removed", 22), valStyle.Render(filesStr)))
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
			formatShortcut("R", "Run again"),
			formatShortcut("B", "Back to menu"),
			formatShortcut("Q", "Quit"),
		}
	} else {
		hints = []string{
			formatShortcut("Space", "Toggle"),
			formatShortcut("Enter", "Clean"),
			formatShortcut("D", "Dry Run"),
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
	return filepath.Join(logDir, "operations.log")
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
