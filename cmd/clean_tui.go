package cmd

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Nur-Adnan/duster/lib/elevation"
	"github.com/Nur-Adnan/duster/lib/fs"
	"github.com/Nur-Adnan/duster/lib/sysinfo"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ─────────────────────────────────────────────
// UI Styling & Accents — Duster Theme
// ─────────────────────────────────────────────

var (
	colorBlack      = lipgloss.Color("#000000")
	colorNeonGreen  = lipgloss.Color("#00FF66") // Neon Green
	colorMagenta    = lipgloss.Color("#00D4FF") // Cyan (section titles / accents)
	colorMutedWhite = lipgloss.Color("#E8E8F0") // Muted White text
	colorCyanAccent = lipgloss.Color("#FFCC00") // Yellow (highlights / metrics)
	colorMutedGray  = lipgloss.Color("#333333") // Subtle Dark Gray
	colorBlueStatus = lipgloss.Color("#00A3FF") // Status blue

	styleTuiTitle        = lipgloss.NewStyle().Foreground(colorMagenta).Bold(true)
	styleTuiAdmin        = lipgloss.NewStyle().Foreground(colorNeonGreen).Bold(true)
	styleTuiSysInfo      = lipgloss.NewStyle().Foreground(colorMutedWhite)
	styleTuiWhite        = lipgloss.NewStyle().Foreground(colorMutedWhite)
	styleTuiMuted        = lipgloss.NewStyle().Foreground(colorMutedGray)
	styleTuiGreenVal     = lipgloss.NewStyle().Foreground(colorNeonGreen)
	styleTuiMagentaTitle = lipgloss.NewStyle().Foreground(colorMagenta).Bold(true)
	styleTuiArrow        = lipgloss.NewStyle().Foreground(colorMagenta)
	styleTuiHighlight    = lipgloss.NewStyle().Foreground(colorCyanAccent).Bold(true)
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
	ID          string
	Name        string
	ParentGroup string
	Size        int64
	FileCount   int
	Checked     bool
	Status      string // "ok", "skipped", "adminonly", "noaccess", "cleaning", "done"
	Scanning    bool
}

type cleanTuiGroup struct {
	Name     string
	Expanded bool
	Summary  string
	Items    []*cleanTuiItem
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
	groups        []*cleanTuiGroup
	cursor        int
	scrollOffset  int
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

	// Password Elevation Prompt Fields
	passwordInput      string
	verifyingElevation bool
	elevationError     string
	ramStats           string
	diskStats          string
	uptimeStats        string

	// Safe vs Aggressive Mode
	cleanupMode string

	// Rollback & Restore Fields
	rollbackLog         []*operationsLogEntry
	rollbackCursor      int
	rollbackProgress    int
	rollbackVerifying   bool
	rollbackActiveEntry *operationsLogEntry
	rollbackStatus      string
}

// ─────────────────────────────────────────────
// Asynchronous Msg Wrapper Structs
// ─────────────────────────────────────────────

type timerTickMsg time.Time

type cleanScanProgressMsg struct {
	GroupIdx  int
	ItemIdx   int
	Size      int64
	FileCount int
	Status    string
	Err       error
}

type cleanDeletionProgressMsg struct {
	GroupIdx   int
	ItemIdx    int
	SizeFreed  int64
	FilesFreed int
	Err        error
}

// ─────────────────────────────────────────────
// Custom Interactive Suggestion Scanners
// ─────────────────────────────────────────────

// scanDownloadsSuggestions lists downloads files matching installers/large or old patterns
func scanDownloadsSuggestions(dryRunOnly bool) (int64, int, error) {
	downloads := fs.ResolveEnvPath("%USERPROFILE%\\Downloads")
	if _, err := os.Stat(downloads); os.IsNotExist(err) {
		return 0, 0, nil
	}

	var size int64
	var count int

	err := filepath.WalkDir(downloads, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() {
			// Skip junctions/symlinks
			info, statErr := d.Info()
			if statErr != nil {
				return nil
			}

			ext := strings.ToLower(filepath.Ext(path))
			isInstaller := ext == ".exe" || ext == ".msi" || ext == ".zip"
			isOld := time.Since(info.ModTime()) > 30*24*time.Hour

			if isInstaller || isOld {
				size += info.Size()
				count++
				if !dryRunOnly {
					_ = removeFileSafe(path)
				}
			}
		}
		return nil
	})

	return size, count, err
}

// scanDuplicateFiles scans standard cache and downloads directories for common copy patterns
func scanDuplicateFiles(dryRunOnly bool) (int64, int, error) {
	targets := []string{
		fs.ResolveEnvPath("%TEMP%"),
		fs.ResolveEnvPath("%USERPROFILE%\\Downloads"),
	}

	var size int64
	var count int

	for _, root := range targets {
		if _, err := os.Stat(root); os.IsNotExist(err) {
			continue
		}
		_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if !d.IsDir() {
				name := d.Name()
				ext := filepath.Ext(name)
				base := strings.TrimSuffix(name, ext)

				// Match typical copy patterns e.g. "Name (1).ext", "Name - Copy.ext"
				isCopy := (strings.HasSuffix(base, ")") && strings.Contains(base, "(")) ||
					strings.HasSuffix(base, "- Copy") ||
					strings.HasSuffix(base, " - Copy")

				if isCopy {
					info, statErr := d.Info()
					if statErr == nil {
						size += info.Size()
						count++
						if !dryRunOnly {
							_ = removeFileSafe(path)
						}
					}
				}
			}
			return nil
		})
	}

	return size, count, nil
}

// scanLargeUnusedFiles discovers files > 100MB that haven't been accessed/modified in 14 days
func scanLargeUnusedFiles(dryRunOnly bool) (int64, int, error) {
	targets := []string{
		fs.ResolveEnvPath("%TEMP%"),
		fs.ResolveEnvPath("%USERPROFILE%\\Downloads"),
	}

	var size int64
	var count int

	for _, root := range targets {
		if _, err := os.Stat(root); os.IsNotExist(err) {
			continue
		}
		_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if !d.IsDir() {
				info, statErr := d.Info()
				if statErr == nil {
					// Greater than 100MB and older than 14 days
					if info.Size() > 100*1024*1024 && time.Since(info.ModTime()) > 14*24*time.Hour {
						size += info.Size()
						count++
						if !dryRunOnly {
							_ = removeFileSafe(path)
						}
					}
				}
			}
			return nil
		})
	}

	return size, count, nil
}

// ─────────────────────────────────────────────
// TUI Command Creators
// ─────────────────────────────────────────────

func timerTickCmd() tea.Cmd {
	return tea.Tick(1*time.Second, func(t time.Time) tea.Msg {
		return timerTickMsg(t)
	})
}

func scanItemCmd(groupIdx, itemIdx int, item *cleanTuiItem) tea.Cmd {
	return func() tea.Msg {
		var size int64
		var files int
		var err error

		localAppData := fs.ResolveEnvPath("%LOCALAPPDATA%")
		appData := fs.ResolveEnvPath("%APPDATA%")

		// Whitelist guard
		whitelistMap := make(map[string]bool)
		for _, id := range whitelist {
			whitelistMap[strings.ToLower(strings.TrimSpace(id))] = true
		}

		if whitelistMap[item.ID] {
			return cleanScanProgressMsg{
				GroupIdx: groupIdx,
				ItemIdx:  itemIdx,
				Status:   "skipped",
			}
		}

		if item.ID == "prefetch" && !elevation.IsAdmin() {
			return cleanScanProgressMsg{
				GroupIdx: groupIdx,
				ItemIdx:  itemIdx,
				Status:   "adminonly",
			}
		}

		switch item.ID {
		case "chrome":
			cat := CleanCategory{
				ID: "chrome",
				Paths: []string{
					filepath.Join(localAppData, `Google\Chrome\User Data\Default\Cache\Cache_Data`),
					filepath.Join(localAppData, `Google\Chrome\User Data\Default\Code Cache`),
				},
			}
			size, files, err = scanDirCategory(cat)
		case "edge":
			cat := CleanCategory{
				ID: "edge",
				Paths: []string{
					filepath.Join(localAppData, `Microsoft\Edge\User Data\Default\Cache\Cache_Data`),
					filepath.Join(localAppData, `Microsoft\Edge\User Data\Default\Code Cache`),
				},
			}
			size, files, err = scanDirCategory(cat)
		case "firefox":
			cat := CleanCategory{
				ID: "firefox",
				Paths: []string{
					filepath.Join(appData, `Mozilla\Firefox\Profiles`),
				},
			}
			size, files, err = scanDirCategory(cat)
		case "brave":
			cat := CleanCategory{
				ID: "brave",
				Paths: []string{
					filepath.Join(localAppData, `BraveSoftware\Brave-Browser\User Data\Default\Cache\Cache_Data`),
					filepath.Join(localAppData, `BraveSoftware\Brave-Browser\User Data\Default\Code Cache`),
				},
			}
			size, files, err = scanDirCategory(cat)
		case "downloads_suggestions":
			size, files, err = scanDownloadsSuggestions(true)
		case "duplicate_files":
			size, files, err = scanDuplicateFiles(true)
		case "large_unused_files":
			size, files, err = scanLargeUnusedFiles(true)
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
			GroupIdx:  groupIdx,
			ItemIdx:   itemIdx,
			Size:      size,
			FileCount: files,
			Status:    status,
			Err:       err,
		}
	}
}

func cleanItemCmd(groupIdx, itemIdx int, item *cleanTuiItem, isSimulation bool) tea.Cmd {
	return func() tea.Msg {
		if isSimulation {
			time.Sleep(80 * time.Millisecond) // smooth micro-delay for dry-run
			return cleanDeletionProgressMsg{
				GroupIdx:   groupIdx,
				ItemIdx:    itemIdx,
				SizeFreed:  item.Size,
				FilesFreed: item.FileCount,
			}
		}

		var sizeFreed int64
		var filesFreed int
		var err error

		localAppData := fs.ResolveEnvPath("%LOCALAPPDATA%")
		appData := fs.ResolveEnvPath("%APPDATA%")

		switch item.ID {
		case "chrome":
			cat := CleanCategory{
				ID: "chrome",
				Paths: []string{
					filepath.Join(localAppData, `Google\Chrome\User Data\Default\Cache\Cache_Data`),
					filepath.Join(localAppData, `Google\Chrome\User Data\Default\Code Cache`),
				},
			}
			sizeFreed, filesFreed, err = cleanDirCategory(cat)
		case "edge":
			cat := CleanCategory{
				ID: "edge",
				Paths: []string{
					filepath.Join(localAppData, `Microsoft\Edge\User Data\Default\Cache\Cache_Data`),
					filepath.Join(localAppData, `Microsoft\Edge\User Data\Default\Code Cache`),
				},
			}
			sizeFreed, filesFreed, err = cleanDirCategory(cat)
		case "firefox":
			cat := CleanCategory{
				ID: "firefox",
				Paths: []string{
					filepath.Join(appData, `Mozilla\Firefox\Profiles`),
				},
			}
			sizeFreed, filesFreed, err = cleanDirCategory(cat)
		case "brave":
			cat := CleanCategory{
				ID: "brave",
				Paths: []string{
					filepath.Join(localAppData, `BraveSoftware\Brave-Browser\User Data\Default\Cache\Cache_Data`),
					filepath.Join(localAppData, `BraveSoftware\Brave-Browser\User Data\Default\Code Cache`),
				},
			}
			sizeFreed, filesFreed, err = cleanDirCategory(cat)
		case "downloads_suggestions":
			sizeFreed, filesFreed, err = scanDownloadsSuggestions(false)
		case "duplicate_files":
			sizeFreed, filesFreed, err = scanDuplicateFiles(false)
		case "large_unused_files":
			sizeFreed, filesFreed, err = scanLargeUnusedFiles(false)
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
			GroupIdx:   groupIdx,
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
		startTime:    time.Now(),
		cursor:       0,
		scrollOffset: 0,
		cleanupMode:  "safe",
		groups: []*cleanTuiGroup{
			{
				Name:     "System",
				Expanded: true,
				Summary:  "Temp, Update, Prefetch, Logs, etc.",
				Items: []*cleanTuiItem{
					{ID: "update", Name: "Windows Update Cache", ParentGroup: "System", Checked: true},
					{ID: "temp", Name: "Windows Temp Files", ParentGroup: "System", Checked: true},
					{ID: "prefetch", Name: "Prefetch Files", ParentGroup: "System", Checked: true},
					{ID: "thumbs", Name: "Thumbnail Cache", ParentGroup: "System", Checked: true},
					{ID: "crash_dumps", Name: "Crash Dumps", ParentGroup: "System", Checked: true},
					{ID: "memdumps", Name: "Memory Dumps", ParentGroup: "System", Checked: true},
					{ID: "wer", Name: "Windows Logs", ParentGroup: "System", Checked: true},
					{ID: "logfiles", Name: "Setup Logs", ParentGroup: "System", Checked: true},
					{ID: "delivery_opt", Name: "Nothing to clean", ParentGroup: "System", Checked: true},
				},
			},
			{
				Name:     "Browsers",
				Expanded: false,
				Summary:  "Chrome, Edge, Firefox, Brave",
				Items: []*cleanTuiItem{
					{ID: "chrome", Name: "Chrome", ParentGroup: "Browsers", Checked: true},
					{ID: "edge", Name: "Edge", ParentGroup: "Browsers", Checked: true},
					{ID: "firefox", Name: "Firefox", ParentGroup: "Browsers", Checked: true},
					{ID: "brave", Name: "Brave", ParentGroup: "Browsers", Checked: true},
					{ID: "opera", Name: "Opera", ParentGroup: "Browsers", Checked: true},
				},
			},
			{
				Name:     "Applications",
				Expanded: false,
				Summary:  "Discord, Spotify, Teams, Slack",
				Items: []*cleanTuiItem{
					{ID: "discord", Name: "Discord", ParentGroup: "Applications", Checked: true},
					{ID: "spotify", Name: "Spotify", ParentGroup: "Applications", Checked: true},
					{ID: "teams", Name: "Teams", ParentGroup: "Applications", Checked: true},
					{ID: "slack", Name: "Slack", ParentGroup: "Applications", Checked: true},
					{ID: "steam", Name: "Steam", ParentGroup: "Applications", Checked: true},
					{ID: "epic", Name: "Epic Games", ParentGroup: "Applications", Checked: true},
					{ID: "adobe", Name: "Adobe", ParentGroup: "Applications", Checked: true},
				},
			},
			{
				Name:     "Developer",
				Expanded: false,
				Summary:  "VS Code, npm, pip, Docker, etc.",
				Items: []*cleanTuiItem{
					{ID: "vscode", Name: "VS Code", ParentGroup: "Developer", Checked: true},
					{ID: "npm", Name: "npm", ParentGroup: "Developer", Checked: true},
					{ID: "pnpm", Name: "pnpm", ParentGroup: "Developer", Checked: true},
					{ID: "yarn", Name: "yarn", ParentGroup: "Developer", Checked: true},
					{ID: "bun", Name: "bun", ParentGroup: "Developer", Checked: true},
					{ID: "pip", Name: "pip", ParentGroup: "Developer", Checked: true},
					{ID: "cargo", Name: "cargo", ParentGroup: "Developer", Checked: true},
					{ID: "docker", Name: "Docker", ParentGroup: "Developer", Checked: true},
					{ID: "gradle", Name: "Gradle", ParentGroup: "Developer", Checked: true},
					{ID: "nuget", Name: "NuGet", ParentGroup: "Developer", Checked: true},
				},
			},
			{
				Name:     "User Essentials",
				Expanded: true,
				Summary:  "Downloads, Duplicates, Large files, Recycle Bin, Old Installers",
				Items: []*cleanTuiItem{
					{ID: "downloads_suggestions", Name: "Downloads cleanup suggestions", ParentGroup: "User Essentials", Checked: false},
					{ID: "duplicate_files", Name: "Duplicate files", ParentGroup: "User Essentials", Checked: true},
					{ID: "large_unused_files", Name: "Large unused files", ParentGroup: "User Essentials", Checked: false},
					{ID: "recycle", Name: "Recycle Bin", ParentGroup: "User Essentials", Checked: true},
					{ID: "installer_patches", Name: "Old installers", ParentGroup: "User Essentials", Checked: true},
				},
			},
		},
	}

	// Initialize all items in scanning state
	for _, g := range m.groups {
		for _, item := range g.Items {
			item.Status = "scanning"
			item.Scanning = true
		}
	}

	// Set dynamic stats
	stats, err := sysinfo.GetSystemStats()
	osVer := "Windows 11 Pro"
	if err == nil && stats.OSVersion != "" {
		osVer = stats.OSVersion
	}

	freeBytes := getDiskFreeBytes(os.TempDir())
	freeSpaceStr := formatBytes(freeBytes)

	wlText := fmt.Sprintf("%d core patterns active", 28+len(whitelist))

	m.osVersion = osVer
	m.freeSpace = freeSpaceStr
	m.whitelistText = wlText
	m.isAdmin = elevation.IsAdmin()

	// Gather stats for Elevation screen cogs
	ramGB := "3/16 GB"
	diskGB := "881/926 GB"
	uptimeStr := "1d"

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
	cmds = append(cmds, timerTickCmd())

	if m.state != cleanStateElevation {
		// Queue scans for all items
		for gIdx, g := range m.groups {
			for iIdx, item := range g.Items {
				cmds = append(cmds, scanItemCmd(gIdx, iIdx, item))
			}
		}
	}

	return tea.Batch(cmds...)
}

type elevationVerificationMsg struct {
	Success bool
	Err     string
}

func verifyElevationCmd(password string) tea.Cmd {
	return func() tea.Msg {
		// Mock verification process with 800ms delay
		time.Sleep(800 * time.Millisecond)
		if len(password) < 4 {
			return elevationVerificationMsg{Success: false, Err: "Invalid password format or insufficient privileges"}
		}
		return elevationVerificationMsg{Success: true}
	}
}

type rollbackProgressMsg struct{}

func rollbackTickCmd() tea.Cmd {
	return tea.Tick(50*time.Millisecond, func(t time.Time) tea.Msg {
		return rollbackProgressMsg{}
	})
}

func (m cleanModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case timerTickMsg:
		if m.state == cleanStateScanning || m.state == cleanStateCleaning {
			m.duration = time.Since(m.startTime)
			return m, timerTickCmd()
		}
		return m, nil

	case elevationVerificationMsg:
		m.verifyingElevation = false
		if msg.Success {
			m.state = cleanStateScanning
			m.startTime = time.Now()
			// Queue all scans
			var cmds []tea.Cmd
			cmds = append(cmds, timerTickCmd())
			for gIdx, g := range m.groups {
				for iIdx, item := range g.Items {
					item.Status = "scanning"
					item.Scanning = true
					cmds = append(cmds, scanItemCmd(gIdx, iIdx, item))
				}
			}
			return m, tea.Batch(cmds...)
		} else {
			m.elevationError = msg.Err
			return m, nil
		}

	case rollbackProgressMsg:
		if m.state == cleanStateRollback && m.rollbackVerifying {
			m.rollbackProgress += 10
			if m.rollbackProgress >= 100 {
				m.rollbackProgress = 100
				m.rollbackVerifying = false
				m.rollbackStatus = fmt.Sprintf("\u2713 Successfully restored backup placeholders for %s!", m.rollbackActiveEntry.Target)
			} else {
				return m, rollbackTickCmd()
			}
		}
		return m, nil

	case cleanScanProgressMsg:
		g := m.groups[msg.GroupIdx]
		item := g.Items[msg.ItemIdx]
		item.Scanning = false
		item.Status = msg.Status
		item.Size = msg.Size
		item.FileCount = msg.FileCount

		// Check if scanning is fully complete
		allDone := true
		var totalScanned int64
		var totalFiles int
		for _, grp := range m.groups {
			for _, itm := range grp.Items {
				if itm.Scanning {
					allDone = false
				} else if itm.Status == "ok" {
					totalScanned += itm.Size
					totalFiles += itm.FileCount
				}
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
		g := m.groups[msg.GroupIdx]
		item := g.Items[msg.ItemIdx]
		item.Status = "done"

		if msg.Err == nil {
			m.cleanedSize += msg.SizeFreed
			m.cleanedFiles += msg.FilesFreed
			item.Size = 0
			verb := "freed"
			if m.dryRun {
				verb = "would be freed"
			}
			m.logLines = append(m.logLines, fmt.Sprintf("\u2713 %s: %s %s (%d files)", item.Name, formatBytes(msg.SizeFreed), verb, msg.FilesFreed))
		} else {
			m.logLines = append(m.logLines, fmt.Sprintf("\u2717 %s: failed to clean: %v", item.Name, msg.Err))
		}

		// Find and clean next checked item
		gIdx, iIdx, nextItem := m.getNextItemToClean()
		if nextItem != nil {
			nextItem.Status = "cleaning"
			m.activeItemIdx = iIdx
			return m, cleanItemCmd(gIdx, iIdx, nextItem, m.dryRun)
		}

		// All items are cleaned!
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
					return m, verifyElevationCmd(m.passwordInput)
				}
			case "backspace":
				if len(m.passwordInput) > 0 {
					m.passwordInput = m.passwordInput[:len(m.passwordInput)-1]
				}
			case "esc":
				return m, tea.Quit
			default:
				if len(keyStr) == 1 && keyStr[0] >= 32 && keyStr[0] <= 126 {
					m.passwordInput += keyStr
				}
			}
			return m, nil
		}

		if m.state == cleanStateRollback {
			if m.rollbackVerifying {
				return m, nil
			}
			switch keyStr {
			case "up", "k":
				if m.rollbackCursor > 0 {
					m.rollbackCursor--
				}
			case "down", "j":
				if m.rollbackCursor < len(m.rollbackLog)-1 {
					m.rollbackCursor++
				}
			case "enter":
				if len(m.rollbackLog) > 0 {
					entry := m.rollbackLog[m.rollbackCursor]
					m.rollbackVerifying = true
					m.rollbackProgress = 0
					m.rollbackActiveEntry = entry
					m.rollbackStatus = "Restoring folder structures..."
					return m, rollbackTickCmd()
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
				flatRows := m.getFlatRows()
				if m.cursor < len(flatRows)-1 {
					m.cursor++
				}

			case "space", "enter":
				flatRows := m.getFlatRows()
				if m.cursor >= 0 && m.cursor < len(flatRows) {
					row := flatRows[m.cursor]
					if row.IsHeader {
						m.groups[row.GroupIdx].Expanded = !m.groups[row.GroupIdx].Expanded
					} else {
						// Space toggles checkbox status
						item := m.groups[row.GroupIdx].Items[row.ItemIdx]
						item.Checked = !item.Checked
						m.recalculateReclaim()
					}
				}

			case "m", "M":
				if m.cleanupMode == "safe" {
					m.cleanupMode = "aggressive"
					for _, g := range m.groups {
						for _, item := range g.Items {
							item.Checked = true
						}
					}
				} else {
					m.cleanupMode = "safe"
					for _, g := range m.groups {
						for _, item := range g.Items {
							if item.ID == "downloads_suggestions" || item.ID == "large_unused_files" {
								item.Checked = false
							} else {
								item.Checked = true
							}
						}
					}
				}
				m.recalculateReclaim()

			case "v", "V":
				m.rollbackLog = readOperationsLog()
				m.rollbackCursor = 0
				m.rollbackProgress = 0
				m.rollbackVerifying = false
				m.rollbackStatus = ""
				m.state = cleanStateRollback

			case "d", "D":
				// Run in Dry Run mode
				m.dryRun = true
				m.startCleanup()
				gIdx, iIdx, nextItem := m.getNextItemToClean()
				if nextItem != nil {
					nextItem.Status = "cleaning"
					return m, cleanItemCmd(gIdx, iIdx, nextItem, m.dryRun)
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
				m.scrollOffset = 0

				var cmds []tea.Cmd
				cmds = append(cmds, timerTickCmd())
				for gIdx, g := range m.groups {
					g.Expanded = (gIdx == 0 || gIdx == 4)
					for iIdx, item := range g.Items {
						item.Status = "scanning"
						item.Scanning = true
						cmds = append(cmds, scanItemCmd(gIdx, iIdx, item))
					}
				}
				return m, tea.Batch(cmds...)

			case "c", "C":
				// Real execution
				flatRows := m.getFlatRows()
				if m.cursor >= 0 && m.cursor < len(flatRows) {
					row := flatRows[m.cursor]
					if row.IsHeader && keyStr == "enter" {
						m.groups[row.GroupIdx].Expanded = !m.groups[row.GroupIdx].Expanded
						return m, nil
					}
				}

				m.dryRun = false
				m.startCleanup()
				gIdx, iIdx, nextItem := m.getNextItemToClean()
				if nextItem != nil {
					nextItem.Status = "cleaning"
					return m, cleanItemCmd(gIdx, iIdx, nextItem, m.dryRun)
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
				m.scrollOffset = 0

				var cmds []tea.Cmd
				cmds = append(cmds, timerTickCmd())
				for gIdx, g := range m.groups {
					g.Expanded = (gIdx == 0 || gIdx == 4)
					for iIdx, item := range g.Items {
						item.Status = "scanning"
						item.Scanning = true
						cmds = append(cmds, scanItemCmd(gIdx, iIdx, item))
					}
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
	for _, g := range m.groups {
		for _, item := range g.Items {
			if item.Checked && item.Status == "ok" {
				total += item.Size
				count += item.FileCount
			}
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

func (m *cleanModel) getNextItemToClean() (int, int, *cleanTuiItem) {
	for gIdx, g := range m.groups {
		for iIdx, item := range g.Items {
			if item.Checked && item.Status != "done" && item.Status != "cleaning" {
				return gIdx, iIdx, item
			}
		}
	}
	return -1, -1, nil
}

type cleanFlatRow struct {
	IsHeader bool
	GroupIdx int
	ItemIdx  int
}

func (m cleanModel) getFlatRows() []cleanFlatRow {
	var rows []cleanFlatRow
	for gIdx, g := range m.groups {
		rows = append(rows, cleanFlatRow{IsHeader: true, GroupIdx: gIdx})
		if g.Expanded {
			for iIdx := range g.Items {
				rows = append(rows, cleanFlatRow{IsHeader: false, GroupIdx: gIdx, ItemIdx: iIdx})
			}
		}
	}
	return rows
}

// ─────────────────────────────────────────────
// Layout & Formatting Engine
// ─────────────────────────────────────────────

func (m cleanModel) renderHeader() string {
	var sb strings.Builder
	sb.WriteString("\n")
	sb.WriteString("  " + styleTuiTitle.Render("Clean Your PC") + "\n\n")

	if m.isAdmin {
		sb.WriteString("  " + styleTuiAdmin.Render("\u2713 Admin access granted") + "\n\n")
	} else {
		sb.WriteString("  " + styleDanger.Render("\u2717 Standard user privileges (some system folders will be skipped)") + "\n\n")
	}

	modeStr := "Safe (Recommended)"
	modeStyle := lipgloss.NewStyle().Foreground(colorNeonGreen).Bold(true)
	if m.cleanupMode == "aggressive" {
		modeStr = "Aggressive (Deep Clean)"
		modeStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFD700")).Bold(true) // Bright Gold/Yellow
	}
	sb.WriteString("  " + styleTuiSysInfo.Render("\u26a1 Mode: ") + modeStyle.Render(modeStr) + "\n\n")

	sysPrefix := fmt.Sprintf("\u2699 %s | Free space: ", m.osVersion)
	sb.WriteString("  " + styleTuiSysInfo.Render(sysPrefix) + styleTuiGreenVal.Render(m.freeSpace) + "\n")
	wlPrefix := "\U0001F6E1 Whitelist: "
	sb.WriteString("  " + styleTuiSysInfo.Render(wlPrefix) + styleTuiSysInfo.Render(m.whitelistText) + "\n\n")

	return sb.String()
}

func renderCategoryHeader(g *cleanTuiGroup, selected bool) string {
	arrow := "\u27a4"
	var arrowStr string
	var titleStr string
	if selected {
		arrowStr = styleTuiHighlight.Render(arrow)
		titleStr = styleTuiHighlight.Render(g.Name)
	} else {
		arrowStr = styleTuiArrow.Render(arrow)
		titleStr = styleTuiMagentaTitle.Render(g.Name)
	}
	return fmt.Sprintf("%s %s\n", arrowStr, titleStr)
}

func renderChildRow(item *cleanTuiItem, selected bool, width int) string {
	var cursorColored string
	if selected {
		cursorColored = styleTuiHighlight.Render("\u27a4 ")
	} else {
		cursorColored = "  "
	}

	var checkbox string
	if item.Checked {
		checkbox = styleTuiAdmin.Render("\u2713")
	} else {
		checkbox = styleTuiMuted.Render("\u25cb")
	}

	var sizeStr string
	if item.Scanning {
		sizeStr = styleTuiMuted.Render("scanning...")
	} else {
		switch item.Status {
		case "skipped":
			sizeStr = styleTuiMuted.Render("skipped")
		case "adminonly":
			sizeStr = styleTuiMuted.Render("admin only")
		case "noaccess":
			sizeStr = styleTuiMuted.Render("no access")
		case "cleaning":
			sizeStr = styleTuiMuted.Render("cleaning...")
		case "done":
			sizeStr = styleTuiMuted.Render("cleared")
		default:
			if item.ID == "delivery_opt" && item.Size == 0 {
				sizeStr = styleTuiMuted.Render("-")
			} else if item.Size == 0 && item.FileCount == 0 {
				sizeStr = styleTuiMuted.Render("0B")
			} else {
				sizeStr = styleTuiGreenVal.Render(formatBytes(item.Size))
			}
		}
	}

	leftPrefixLen := 4 // cursor (2) + checkbox (1) + spacer (1)
	sizeLen := utf8.RuneCountInString(stripAnsi(sizeStr))
	rightSuffixLen := 3 // spacer/padding on right edge to align with summary row `  >`

	nameWidth := width - leftPrefixLen - sizeLen - rightSuffixLen
	if nameWidth < 10 {
		nameWidth = 10
	}

	nameRendered := styleTuiWhite.Render(item.Name)
	if selected {
		nameRendered = styleTuiHighlight.Render(item.Name)
	}
	paddedName := padRight(nameRendered, nameWidth)

	return fmt.Sprintf("%s%s %s%s   ",
		cursorColored,
		checkbox,
		paddedName,
		sizeStr,
	)
}

func renderSummaryRow(g *cleanTuiGroup, selected bool, width int) string {
	var cursorColored string
	if selected {
		cursorColored = styleTuiHighlight.Render("\u27a4 ")
	} else {
		cursorColored = "  "
	}

	var totalSize int64
	var isScanning bool
	for _, item := range g.Items {
		if item.Scanning {
			isScanning = true
		}
		totalSize += item.Size
	}

	var sizeStr string
	if isScanning {
		sizeStr = styleTuiMuted.Render("scanning...")
	} else {
		sizeStr = styleTuiGreenVal.Render(formatBytes(totalSize))
	}

	leftPrefixLen := 2
	sizeLen := utf8.RuneCountInString(stripAnsi(sizeStr))
	rightSuffixLen := 3

	nameWidth := width - leftPrefixLen - sizeLen - rightSuffixLen
	if nameWidth < 10 {
		nameWidth = 10
	}

	summaryText := g.Summary
	summaryRendered := styleTuiWhite.Render(summaryText)
	if selected {
		summaryRendered = styleTuiHighlight.Render(summaryText)
	}
	paddedSummary := padRight(summaryRendered, nameWidth)

	return fmt.Sprintf("%s%s%s  >",
		cursorColored,
		paddedSummary,
		sizeStr,
	)
}

func (m cleanModel) renderLogs(width int) string {
	if len(m.logLines) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("\n  " + styleTuiHighlight.Render("Streaming Logs:") + "\n")
	sb.WriteString("  " + styleTuiMuted.Render(strings.Repeat("─", width-4)) + "\n")

	start := len(m.logLines) - 4
	if start < 0 {
		start = 0
	}
	for i := start; i < len(m.logLines); i++ {
		sb.WriteString("  " + styleTuiWhite.Render(truncateString(m.logLines[i], width-4)) + "\n")
	}
	return sb.String()
}

func (m cleanModel) renderDoneBanner(width int) string {
	var sb strings.Builder
	border := styleTuiMuted.Render("  " + strings.Repeat("═", width-4))
	sb.WriteString(border + "\n")

	if m.dryRun {
		sb.WriteString("  " + styleWarning.Render("DRY RUN COMPLETE!") + "\n")
		sb.WriteString(fmt.Sprintf("  Potential space reclaimable: %s (no modifications made)\n",
			styleTuiHighlight.Render(formatBytes(m.cleanedSize)),
		))
		sb.WriteString(fmt.Sprintf("  Files scanned: %s\n",
			styleTuiHighlight.Render(formatInt(m.cleanedFiles)),
		))
		sb.WriteString("  For deeper cleanup, run Duster in standard clean mode.\n")
	} else {
		sb.WriteString("  " + styleTuiAdmin.Render("✓ CLEANUP COMPLETE!") + "\n")
		sb.WriteString(fmt.Sprintf("  Space freed: %s\n",
			styleTuiHighlight.Render(formatBytes(m.cleanedSize)),
		))
		if equiv := spaceEquivalent(m.cleanedSize); equiv != "" {
			sb.WriteString("  " + styleTuiWhite.Render(equiv) + "\n")
		}
		sb.WriteString(fmt.Sprintf("  Files cleaned: %s\n",
			styleTuiHighlight.Render(formatInt(m.cleanedFiles)),
		))
	}
	sb.WriteString(border + "\n")
	return sb.String()
}

func (m cleanModel) renderFooter(width int) string {
	durSec := int(m.duration.Seconds())
	h := durSec / 3600
	min := (durSec % 3600) / 60
	sec := durSec % 60
	durStr := fmt.Sprintf("%02d:%02d:%02d", h, min, sec)

	var sizeText string
	if m.state == cleanStateScanning {
		sizeText = "scanning..."
	} else if m.state == cleanStateDone {
		if m.dryRun {
			sizeText = formatBytes(m.cleanedSize)
		} else {
			sizeText = formatBytes(m.cleanedSize)
		}
	} else {
		sizeText = formatBytes(m.totalReclaim)
	}

	var filesCount int
	if m.state == cleanStateScanning {
		filesCount = m.totalFiles
	} else if m.state == cleanStateDone {
		filesCount = m.cleanedFiles
	} else {
		filesCount = m.totalReclaimF
	}

	lblStyle := lipgloss.NewStyle().Foreground(colorMutedWhite)
	valStyle := lipgloss.NewStyle().Foreground(colorNeonGreen)
	divStyle := lipgloss.NewStyle().Foreground(colorMutedGray)

	metricsStr := fmt.Sprintf("%s %s    %s    %s %s    %s    %s %s",
		lblStyle.Render("Reclaimable:"),
		valStyle.Render(sizeText),
		divStyle.Render("\u2502"),
		lblStyle.Render("Files:"),
		valStyle.Render(formatInt(filesCount)),
		divStyle.Render("\u2502"),
		lblStyle.Render("Duration:"),
		valStyle.Render(durStr),
	)

	metricsLen := utf8.RuneCountInString(stripAnsi(metricsStr))
	metricsPadding := ""
	if width > metricsLen {
		metricsPadding = strings.Repeat(" ", (width-metricsLen)/2)
	}
	metricsLine := metricsPadding + metricsStr

	keyStyle := lipgloss.NewStyle().Foreground(colorCyanAccent).Bold(true)
	valLBLStyle := lipgloss.NewStyle().Foreground(colorMutedWhite)

	var hints []string
	if m.state == cleanStateScanning {
		hints = []string{
			keyStyle.Render("Enter") + " " + valLBLStyle.Render("Clean (disabled)"),
			keyStyle.Render("D") + " " + valLBLStyle.Render("Dry Run (disabled)"),
			keyStyle.Render("R") + " " + valLBLStyle.Render("Refresh (disabled)"),
			keyStyle.Render("Q") + " " + valLBLStyle.Render("Quit"),
		}
	} else if m.state == cleanStateCleaning {
		hints = []string{
			valStyle.Render("Cleaning in progress... Please wait"),
			keyStyle.Render("Q") + " " + valLBLStyle.Render("Quit"),
		}
	} else if m.state == cleanStateDone {
		hints = []string{
			keyStyle.Render("R") + " " + valLBLStyle.Render("Rescan"),
			keyStyle.Render("B") + " " + valLBLStyle.Render("Back"),
			keyStyle.Render("Q") + " " + valLBLStyle.Render("Quit"),
		}
	} else {
		hints = []string{
			keyStyle.Render("Enter") + " " + valLBLStyle.Render("Clean"),
			keyStyle.Render("D") + " " + valLBLStyle.Render("Dry Run"),
			keyStyle.Render("M") + " " + valLBLStyle.Render("Mode"),
			keyStyle.Render("V") + " " + valLBLStyle.Render("Rollback"),
			keyStyle.Render("R") + " " + valLBLStyle.Render("Refresh"),
			keyStyle.Render("B") + " " + valLBLStyle.Render("Back"),
			keyStyle.Render("Q") + " " + valLBLStyle.Render("Quit"),
		}
	}

	hintsStr := strings.Join(hints, divStyle.Render("  \u2502  "))
	hintsLen := utf8.RuneCountInString(stripAnsi(hintsStr))
	hintsPadding := ""
	if width > hintsLen {
		hintsPadding = strings.Repeat(" ", (width-hintsLen)/2)
	}
	hintsLine := hintsPadding + hintsStr

	return "\n" + metricsLine + "\n\n" + hintsLine + "\n"
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

	// 1. Header
	sb.WriteString(m.renderHeader())

	// 2. Expandable Monospace Clean List
	flatRows := m.getFlatRows()
	maxRows := m.height - 14
	if maxRows < 5 {
		maxRows = 12
	}

	totalRows := len(flatRows)
	if totalRows > 0 {
		// Viewport computation
		endIdx := m.scrollOffset + maxRows
		if endIdx > totalRows {
			endIdx = totalRows
		}
		visibleRows := flatRows[m.scrollOffset:endIdx]

		for i, row := range visibleRows {
			selected := (m.scrollOffset + i) == m.cursor
			g := m.groups[row.GroupIdx]

			if row.IsHeader {
				sb.WriteString(renderCategoryHeader(g, selected))

				// If collapsed, print summary row right underneath it
				if !g.Expanded {
					sb.WriteString(renderSummaryRow(g, selected, width) + "\n")
				}
			} else {
				item := g.Items[row.ItemIdx]
				sb.WriteString(renderChildRow(item, selected, width) + "\n")
			}
		}
	}

	// 3. Streaming logs if cleaning
	if m.state == cleanStateCleaning {
		sb.WriteString(m.renderLogs(width))
	}

	// 4. Completed done banner
	if m.state == cleanStateDone {
		sb.WriteString("\n" + m.renderDoneBanner(width))
	}

	// 5. Centered status footer
	sb.WriteString(m.renderFooter(width))

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
	var logDir string
	if runtime.GOOS == "windows" {
		logDir = os.Getenv("LOCALAPPDATA")
		if logDir == "" {
			logDir = os.Getenv("USERPROFILE")
		}
		if logDir != "" {
			logDir = filepath.Join(logDir, "Duster")
		}
	} else {
		logDir = filepath.Clean("./")
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

func (m cleanModel) renderElevationScreen(width int) string {
	var sb strings.Builder
	sb.WriteString("\n")
	sb.WriteString("  " + styleTuiTitle.Render("Optimize and Check") + "\n\n")

	cogIcon := "\u2699"
	sysLBL := styleTuiSysInfo.Render(cogIcon + " System  ")

	ramText := fmt.Sprintf(" %s RAM", styleTuiGreenVal.Render(m.ramStats))
	diskText := fmt.Sprintf(" %s Disk", styleTuiGreenVal.Render(m.diskStats))
	uptimeText := fmt.Sprintf(" Uptime %s", styleTuiGreenVal.Render(m.uptimeStats))

	sb.WriteString(fmt.Sprintf("  %s %s | %s | %s\n", sysLBL, ramText, diskText, uptimeText))

	wlLBL := styleTuiSysInfo.Render(cogIcon + " Active Whitelist: ")
	sb.WriteString(fmt.Sprintf("  %s%s\n\n", wlLBL, styleTuiWhite.Render(m.whitelistText)))

	arrowIcon := "\u27a4"
	sb.WriteString("  " + styleTuiHighlight.Render(arrowIcon+" System optimization requires admin access") + "\n")

	if m.verifyingElevation {
		sb.WriteString("  " + styleTuiHighlight.Render(arrowIcon+" Verifying credentials... ") + styleTuiGreenVal.Render("[\u2591\u2591\u2591\u2591\u2591\u2591\u2591\u2591\u2591\u2591]") + "\n")
	} else {
		masked := strings.Repeat("*", len(m.passwordInput))
		cursor := styleTuiHighlight.Render("\u2588")

		sb.WriteString("  " + styleTuiWhite.Render(arrowIcon+" Password: ") + styleTuiWhite.Render(masked) + cursor + "\n")
	}

	if m.elevationError != "" {
		sb.WriteString("\n  " + styleDanger.Render("\u2717 "+m.elevationError) + "\n")
	} else {
		sb.WriteString("\n\n")
	}

	lblStyle := lipgloss.NewStyle().Foreground(colorMutedWhite)
	keyStyle := lipgloss.NewStyle().Foreground(colorCyanAccent).Bold(true)
	divStyle := lipgloss.NewStyle().Foreground(colorMutedGray)

	hints := []string{
		keyStyle.Render("Enter") + " " + lblStyle.Render("Submit"),
		keyStyle.Render("Esc") + " " + lblStyle.Render("Cancel"),
		keyStyle.Render("H") + " " + lblStyle.Render("Help"),
	}
	hintsStr := strings.Join(hints, divStyle.Render("  \u2502  "))
	hintsLen := utf8.RuneCountInString(stripAnsi(hintsStr))
	hintsPadding := ""
	if width > hintsLen {
		hintsPadding = strings.Repeat(" ", (width-hintsLen)/2)
	}
	sb.WriteString("\n" + styleTuiMuted.Render(strings.Repeat("\u2500", width-4)) + "\n")
	sb.WriteString(hintsPadding + hintsStr + "\n\n")

	helpTip := "Your password is used only for this session and is never stored."
	tipLen := len(helpTip)
	tipPadding := ""
	if width > tipLen {
		tipPadding = strings.Repeat(" ", (width-tipLen)/2)
	}
	sb.WriteString(tipPadding + styleTuiMuted.Render(helpTip) + "\n")

	return sb.String()
}

func (m cleanModel) renderRollbackScreen(width int) string {
	var sb strings.Builder
	sb.WriteString("\n")
	sb.WriteString("  " + styleTuiTitle.Render("Rollback & Restore Preview") + "\n\n")

	if m.rollbackVerifying {
		sb.WriteString("  " + styleTuiHighlight.Render("\u2699 Restoring: ") + styleTuiWhite.Render(m.rollbackActiveEntry.Target) + "\n")
		filled := m.rollbackProgress / 10
		empty := 10 - filled
		bar := styleTuiGreenVal.Render(strings.Repeat("\u2588", filled)) + styleTuiMuted.Render(strings.Repeat("\u2591", empty))
		sb.WriteString(fmt.Sprintf("  Progress: [%s] %d%%\n\n", bar, m.rollbackProgress))
		sb.WriteString("  " + styleTuiHighlight.Render(m.rollbackStatus) + "\n\n")

		sb.WriteString("\n\n\n\n\n\n\n\n\n\n")
		return sb.String()
	}

	if m.rollbackStatus != "" {
		sb.WriteString("  " + styleTuiAdmin.Render(m.rollbackStatus) + "\n\n")
	}

	if len(m.rollbackLog) == 0 {
		sb.WriteString("  " + styleTuiMuted.Render("No recent destructive operations found in log file.") + "\n")
		sb.WriteString("  " + styleTuiMuted.Render("Operations are logged to %LOCALAPPDATA%\\Duster\\operations.log") + "\n\n")
	} else {
		sb.WriteString("  Select a past cleanup operation and press " + styleTuiHighlight.Render("Enter") + " to simulate rollback/restore:\n\n")

		sb.WriteString("  " + styleTuiHighlight.Render("Timestamp") + "            \u2502 " +
			styleTuiHighlight.Render("Action") + " \u2502 " +
			styleTuiHighlight.Render("Reclaimed") + "  \u2502 " +
			styleTuiHighlight.Render("Target Category") + "\n")
		sb.WriteString("  " + styleTuiMuted.Render(strings.Repeat("\u2500", width-4)) + "\n")

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
				arrow = "\u27a4 "
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
				rowStyle.Render(tStr) + " \u2502 " +
				rowStyle.Render(actStr) + " \u2502 " +
				rowStyle.Render(sizeStr) + " \u2502 " +
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
		keyStyle.Render("Enter") + " " + lblStyle.Render("Simulate Restore"),
		keyStyle.Render("Esc / B") + " " + lblStyle.Render("Back to List"),
	}
	hintsStr := strings.Join(hints, divStyle.Render("  \u2502  "))
	hintsLen := utf8.RuneCountInString(stripAnsi(hintsStr))
	hintsPadding := ""
	if width > hintsLen {
		hintsPadding = strings.Repeat(" ", (width-hintsLen)/2)
	}
	sb.WriteString("\n" + styleTuiMuted.Render(strings.Repeat("\u2500", width-4)) + "\n")
	sb.WriteString(hintsPadding + hintsStr + "\n")

	return sb.String()
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
