package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/Nur-Adnan/duster/lib/fs"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
)

var analyzeJSON bool

var AnalyzeCmd = &cobra.Command{
	Use:   "analyze [path]",
	Short: "Interactive TUI disk space explorer and visual analyzer",
	Long: `Recursively scan directories to analyze storage allocation, showing:
  - Total folder size and relative size percentage bars
  - Interactive drill-down navigation
  - Quick launch native Windows Explorer
  - Safe Recycle Bin integration
  - Global top 10 largest files viewer`,
	Args: cobra.MaximumNArgs(1),
	Run:  executeAnalyze,
}

func init() {
	AnalyzeCmd.Flags().BoolVar(&analyzeJSON, "json", false, "Output folder analysis metrics as a single JSON snapshot and exit immediately")
}

func executeAnalyze(cmd *cobra.Command, args []string) {
	targetPath := "."
	if len(args) > 0 {
		targetPath = args[0]
	}

	// Resolve environmental paths safely
	targetPath = fs.ResolveEnvPath(targetPath)
	absPath, err := filepath.Abs(targetPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error resolving path: %v\n", err)
		os.Exit(1)
	}

	// Read from pipe or explicit JSON flag
	if analyzeJSON || isPiped() {
		runHeadlessAnalyze(absPath)
		return
	}

	m := initialAnalyzeModel(absPath)
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running TUI analysis: %v\n", err)
		os.Exit(1)
	}
}

// JSON Snapshot Headless Mode Structures
type AnalyzeJSONOutput struct {
	Path          string          `json:"path"`
	TotalSize     int64           `json:"total_size"`
	TotalSizeText string          `json:"total_size_text"`
	DirsCount     int             `json:"dirs_count"`
	FilesCount    int             `json:"files_count"`
	Entries       []EntryJSONInfo `json:"entries"`
	TopLargeFiles []FileNode      `json:"top_large_files"`
}

type EntryJSONInfo struct {
	Name     string  `json:"name"`
	Path     string  `json:"path"`
	Size     int64   `json:"size"`
	SizeText string  `json:"size_text"`
	IsDir    bool    `json:"is_dir"`
	Percent  float64 `json:"percent"`
	SubItems int     `json:"sub_items,omitempty"`
}

func runHeadlessAnalyze(target string) {
	ch := make(chan scanProgressInfo, 100)
	go func() {
		for range ch {
			// Drain progress events silently in headless mode
		}
	}()

	root, large, err := scanDirectory(target, ch)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error analyzing path: %v\n", err)
		os.Exit(1)
	}

	var jsonEntries []EntryJSONInfo
	for _, entry := range root.Entries {
		percent := 0.0
		if root.Size > 0 {
			percent = (float64(entry.Size) / float64(root.Size)) * 100
		}
		jsonEntries = append(jsonEntries, EntryJSONInfo{
			Name:     entry.Name,
			Path:     entry.Path,
			Size:     entry.Size,
			SizeText: formatBytes(entry.Size),
			IsDir:    entry.IsDir,
			Percent:  percent,
			SubItems: entry.Items,
		})
	}

	// Count child directories and files
	dirsCount := 0
	filesCount := 0
	_ = filepath.WalkDir(target, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			dirsCount++
		} else {
			filesCount++
		}
		return nil
	})

	output := AnalyzeJSONOutput{
		Path:          target,
		TotalSize:     root.Size,
		TotalSizeText: formatBytes(root.Size),
		DirsCount:     dirsCount,
		FilesCount:    filesCount,
		Entries:       jsonEntries,
		TopLargeFiles: large,
	}

	data, _ := json.MarshalIndent(output, "", "  ")
	fmt.Println(string(data))
}

// Directory Tree Data Structs
type FileNode struct {
	Path string `json:"path"`
	Size int64  `json:"size"`
}

type FolderNode struct {
	Path       string
	Name       string
	Size       int64
	IsDir      bool
	Files      []FileNode
	SubFolders []*FolderNode
	Entries    []EntryInfo
	Parent     *FolderNode
}

type EntryInfo struct {
	Name  string
	Path  string
	Size  int64
	IsDir bool
	Items int
}

// Bubble Tea Analysis State Model
type analyzeModel struct {
	targetPath     string
	scanning       bool
	scanChan       chan scanProgressInfo
	progress       scanProgressInfo
	tree           *FolderNode
	selectedIdx    int
	historyStack   []*FolderNode
	showLargeFiles bool
	largeFiles     []FileNode
	confirmRecycle bool
	errorMsg       string
	width, height  int
}

type scanProgressInfo struct {
	DirsScanned  int
	FilesScanned int
	TotalSize    int64
	CurrentPath  string
}

func initialAnalyzeModel(path string) analyzeModel {
	return analyzeModel{
		targetPath: path,
		scanning:   true,
		scanChan:   make(chan scanProgressInfo, 100),
	}
}

// Bubble Tea Message Channels
type analyzeScanProgressMsg scanProgressInfo
type analyzeScanCompleteMsg struct {
	Root       *FolderNode
	LargeFiles []FileNode
	Err        error
}

func analyzeListenToScanProgress(ch chan scanProgressInfo) tea.Cmd {
	return func() tea.Msg {
		progress, ok := <-ch
		if !ok {
			return nil
		}
		return analyzeScanProgressMsg(progress)
	}
}

func analyzeRunScanCmd(path string, ch chan scanProgressInfo) tea.Cmd {
	return func() tea.Msg {
		root, large, err := scanDirectory(path, ch)
		close(ch)
		return analyzeScanCompleteMsg{Root: root, LargeFiles: large, Err: err}
	}
}

func (m analyzeModel) Init() tea.Cmd {
	return tea.Batch(
		analyzeRunScanCmd(m.targetPath, m.scanChan),
		analyzeListenToScanProgress(m.scanChan),
	)
}

func (m analyzeModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		// If confirming Recycle Bin removal, trap keyboard events
		if m.confirmRecycle {
			switch msg.String() {
			case "y", "Y":
				var path string
				var size int64
				if m.showLargeFiles {
					entry := m.largeFiles[m.selectedIdx]
					path = entry.Path
					size = entry.Size
				} else {
					entry := m.tree.Entries[m.selectedIdx]
					path = entry.Path
					size = entry.Size
				}

				m.confirmRecycle = false
				err := recyclePath(path, size)
				if err != nil {
					m.errorMsg = fmt.Sprintf("Error recycling: %v", err)
				} else {
					m.errorMsg = ""
					m.selectedIdx = 0
					// Re-trigger scanning on the current folder node's path to refresh sizes
					m.scanning = true
					m.scanChan = make(chan scanProgressInfo, 100)
					scanTarget := m.targetPath
					if m.tree != nil {
						scanTarget = m.tree.Path
					}
					return m, tea.Batch(
						analyzeRunScanCmd(scanTarget, m.scanChan),
						analyzeListenToScanProgress(m.scanChan),
					)
				}
				return m, nil

			case "n", "N", "esc":
				m.confirmRecycle = false
				return m, nil
			}
			return m, nil
		}

		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit

		case "up", "k":
			if m.selectedIdx > 0 {
				m.selectedIdx--
			}

		case "down", "j":
			limit := 0
			if m.showLargeFiles {
				limit = len(m.largeFiles)
			} else if m.tree != nil {
				limit = len(m.tree.Entries)
			}
			if m.selectedIdx < limit-1 {
				m.selectedIdx++
			}

		case "enter", "l", "right":
			if m.showLargeFiles || m.tree == nil {
				return m, nil
			}
			if len(m.tree.Entries) > 0 {
				entry := m.tree.Entries[m.selectedIdx]
				if entry.IsDir {
					// Move downward, save history stack
					m.historyStack = append(m.historyStack, m.tree)
					m.scanning = true
					m.selectedIdx = 0
					m.scanChan = make(chan scanProgressInfo, 100)
					return m, tea.Batch(
						analyzeRunScanCmd(entry.Path, m.scanChan),
						analyzeListenToScanProgress(m.scanChan),
					)
				}
			}

		case "backspace", "h", "left":
			if m.showLargeFiles {
				m.showLargeFiles = false
				m.selectedIdx = 0
				return m, nil
			}
			if len(m.historyStack) > 0 {
				// Navigate backward
				lastIdx := len(m.historyStack) - 1
				parent := m.historyStack[lastIdx]
				m.historyStack = m.historyStack[:lastIdx]

				m.scanning = true
				m.selectedIdx = 0
				m.scanChan = make(chan scanProgressInfo, 100)
				return m, tea.Batch(
					analyzeRunScanCmd(parent.Path, m.scanChan),
					analyzeListenToScanProgress(m.scanChan),
				)
			}

		case "o", "O":
			var path string
			if m.showLargeFiles {
				if len(m.largeFiles) > 0 {
					path = m.largeFiles[m.selectedIdx].Path
				}
			} else if m.tree != nil && len(m.tree.Entries) > 0 {
				path = m.tree.Entries[m.selectedIdx].Path
			}
			if path != "" {
				_ = openInExplorer(path)
			}

		case "d", "D":
			if m.showLargeFiles {
				if len(m.largeFiles) > 0 {
					m.confirmRecycle = true
				}
			} else if m.tree != nil && len(m.tree.Entries) > 0 {
				m.confirmRecycle = true
			}

		case "L":
			m.showLargeFiles = !m.showLargeFiles
			m.selectedIdx = 0
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case analyzeScanProgressMsg:
		m.progress = scanProgressInfo(msg)
		return m, analyzeListenToScanProgress(m.scanChan)

	case analyzeScanCompleteMsg:
		m.scanning = false
		if msg.Err != nil {
			m.errorMsg = fmt.Sprintf("Scan failed: %v", msg.Err)
		} else {
			m.tree = msg.Root
			m.largeFiles = msg.LargeFiles
			m.errorMsg = ""
		}
		return m, nil
	}

	return m, nil
}

func (m analyzeModel) View() string {
	// ── Scanning state ─────────────────────────────────────────────────
	if m.scanning {
		var s strings.Builder
		s.WriteString("\n  " + styleAccent.Render("Analyze Disk"))
		s.WriteString("  " + styleMuted.Render(m.targetPath) + "\n\n")
		s.WriteString("  " + styleMuted.Render("Scanning...") + "\n\n")
		s.WriteString(fmt.Sprintf("  Dirs    %s\n", styleValue.Render(fmt.Sprintf("%d", m.progress.DirsScanned))))
		s.WriteString(fmt.Sprintf("  Files   %s\n", styleValue.Render(fmt.Sprintf("%d", m.progress.FilesScanned))))
		s.WriteString(fmt.Sprintf("  Size    %s\n\n", styleAccent.Render(formatBytes(m.progress.TotalSize))))

		currPath := m.progress.CurrentPath
		if len(currPath) > 70 {
			currPath = "…" + currPath[len(currPath)-68:]
		}
		s.WriteString("  " + styleMuted.Render(currPath) + "\n\n")
		s.WriteString("  " + styleMuted.Render("[q] Abort"))
		return s.String()
	}

	// ── Recycle confirm dialog ─────────────────────────────────────────
	if m.confirmRecycle {
		var path string
		var size int64
		if m.showLargeFiles {
			entry := m.largeFiles[m.selectedIdx]
			path = entry.Path
			size = entry.Size
		} else {
			entry := m.tree.Entries[m.selectedIdx]
			path = entry.Path
			size = entry.Size
		}
		return fmt.Sprintf(
			"\n  %s\n\n  Send to Recycle Bin?\n\n  Target: %s\n  Size  : %s\n\n  %s  %s",
			styleWarning.Render("⚠  RECYCLE CONFIRMATION"),
			redColorStyle.Render(path),
			yellowColorStyle.Render(formatBytes(size)),
			styleAccent.Render("[y] Yes, Recycle"),
			styleMuted.Render("[n] Cancel"),
		)
	}

	if m.tree == nil {
		return fmt.Sprintf("\n  %s: %s\n\n  Press [q] to exit.", styleDanger.Render("Error"), m.errorMsg)
	}

	var s strings.Builder

	// ── Header line ────────────────────────────────────────────────────
	// "Analyze Disk  C:\Users\Nur\Documents  |  Total: 156.8GB"
	s.WriteString("\n  " + analyzeHeader(m.tree.Path, formatBytes(m.tree.Size)) + "\n\n")

	if m.errorMsg != "" {
		s.WriteString("  " + styleDanger.Render(" "+m.errorMsg+" ") + "\n\n")
	}

	// ── Large Files Top-10 view ─────────────────────────────────────────
	if m.showLargeFiles {
		s.WriteString("  " + styleHeader.Render("Top 10 Largest Files") + "  " + styleMuted.Render("(L to return)") + "\n")
		s.WriteString("  " + styleMuted.Render(strings.Repeat("─", 72)) + "\n")
		if len(m.largeFiles) == 0 {
			s.WriteString("  " + styleMuted.Render("No files found.") + "\n")
		} else {
			for i, f := range m.largeFiles {
				selMark := "   "
				if i == m.selectedIdx {
					selMark = styleAccent.Render("►  ")
				}
				name := truncateString(f.Path, 60)
				line := fmt.Sprintf("%s %-62s  %s",
					selMark,
					name,
					styleAccent.Render(formatBytes(f.Size)),
				)
				if i == m.selectedIdx {
					s.WriteString("  " + styleSelected.Render(line) + "\n")
				} else {
					s.WriteString("  " + fileStyle.Render(line) + "\n")
				}
			}
		}
		s.WriteString("\n  " + kbHints("↑↓ Navigate", "⌫ Recycle", "O Open location", "L Normal view", "Q Quit"))
		return s.String()
	}

	// ── Numbered ranked list ───────────────────────────────────────────────
	// Scrolling window
	const barWidth = 18
	const maxVisible = 16
	entries := m.tree.Entries
	total := len(entries)
	start := 0
	end := total
	if total > maxVisible {
		// Keep selected in middle of window
		start = m.selectedIdx - maxVisible/2
		if start < 0 {
			start = 0
		}
		end = start + maxVisible
		if end > total {
			end = total
			start = end - maxVisible
			if start < 0 {
				start = 0
			}
		}
	}

	for i := start; i < end; i++ {
		entry := entries[i]
		pct := 0.0
		if m.tree.Size > 0 {
			pct = float64(entry.Size) / float64(m.tree.Size) * 100
		}

		// Row cursor: ► for selected, blank for others
		var cursor string
		if i == m.selectedIdx {
			cursor = styleAccent.Render("►")
		} else {
			cursor = " "
		}

		// Sequential number
		num := styleNumber.Render(fmt.Sprintf("%2d.", i+1))

		// Compact bar
		bar := progressBar(pct, barWidth)

		// Percentage
		pctStr := styleValue.Render(fmt.Sprintf("%4.1f%%", pct))

		// Icon + name
		icon := "📄"
		var nameStyle = fileStyle
		if entry.IsDir {
			icon = "📁"
			nameStyle = dirStyle
		}
		name := truncateString(entry.Name, 22)
		nameStr := nameStyle.Render(fmt.Sprintf("%s %-23s", icon, name))

		// Size
		sizeStr := styleAccent.Render(fmt.Sprintf("%8s", formatBytes(entry.Size)))

		// Aging indicator
		ageStr := getAgeLabel(entry.Path)
		if ageStr != "" {
			ageStr = "  " + ageStr
		}

		line := fmt.Sprintf("%s %s %s %s  %s  %s%s",
			cursor, num, bar, pctStr, nameStr, sizeStr, ageStr)

		if i == m.selectedIdx {
			s.WriteString("  " + styleSelected.Render(line) + "\n")
		} else {
			s.WriteString("  " + line + "\n")
		}
	}

	if total == 0 {
		s.WriteString("  " + styleMuted.Render("[Empty directory]") + "\n")
	}

	s.WriteString("\n  " + kbHints("↑↓ Navigate", "→ Drill down", "← Back", "O Open", "⌫ Delete", "L Large files", "Q Quit"))
	return s.String()
}

// Traversal Engine Helper Methods
func scanDirectory(root string, progressChan chan<- scanProgressInfo) (*FolderNode, []FileNode, error) {
	root = filepath.Clean(root)
	folderMap := make(map[string]*FolderNode)
	var allFiles []FileNode

	rootNode := &FolderNode{
		Path:  root,
		Name:  filepath.Base(root),
		IsDir: true,
	}
	if rootNode.Name == "" || rootNode.Name == "." || rootNode.Name == "/" || rootNode.Name == "\\" {
		rootNode.Name = root
	}
	folderMap[root] = rootNode

	scannedDirs := 0
	scannedFiles := 0
	var totalSize int64

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			if d != nil && d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		name := d.Name()
		if d.IsDir() && (name == "$Recycle.Bin" || name == "System Volume Information" || name == ".git" || name == "node_modules") {
			return filepath.SkipDir
		}

		if d.IsDir() {
			scannedDirs++
			if _, exists := folderMap[path]; !exists {
				node := &FolderNode{
					Path:  path,
					Name:  name,
					IsDir: true,
				}
				folderMap[path] = node

				parentPath := filepath.Dir(path)
				if parent, pExists := folderMap[parentPath]; pExists {
					node.Parent = parent
					parent.SubFolders = append(parent.SubFolders, node)
				}
			}
		} else {
			scannedFiles++
			info, err := d.Info()
			if err == nil {
				size := info.Size()
				totalSize += size
				fileNode := FileNode{
					Path: path,
					Size: size,
				}
				allFiles = append(allFiles, fileNode)

				parentPath := filepath.Dir(path)
				if parent, pExists := folderMap[parentPath]; pExists {
					parent.Files = append(parent.Files, fileNode)
				}
			}
		}

		select {
		case progressChan <- scanProgressInfo{
			DirsScanned:  scannedDirs,
			FilesScanned: scannedFiles,
			TotalSize:    totalSize,
			CurrentPath:  path,
		}:
		default:
		}

		return nil
	})

	if err != nil {
		return nil, nil, err
	}

	var paths []string
	for p := range folderMap {
		paths = append(paths, p)
	}

	// Sort paths descending by length to compute node sizes from bottom up
	for i := 0; i < len(paths); i++ {
		for j := i + 1; j < len(paths); j++ {
			if len(paths[i]) < len(paths[j]) {
				paths[i], paths[j] = paths[j], paths[i]
			}
		}
	}

	for _, p := range paths {
		node := folderMap[p]
		var size int64
		for _, f := range node.Files {
			size += f.Size
		}
		for _, sub := range node.SubFolders {
			size += sub.Size
		}
		node.Size = size

		var entries []EntryInfo
		for _, sub := range node.SubFolders {
			entries = append(entries, EntryInfo{
				Name:  sub.Name + "\\",
				Path:  sub.Path,
				Size:  sub.Size,
				IsDir: true,
				Items: len(sub.SubFolders) + len(sub.Files),
			})
		}
		for _, f := range node.Files {
			entries = append(entries, EntryInfo{
				Name:  filepath.Base(f.Path),
				Path:  f.Path,
				Size:  f.Size,
				IsDir: false,
			})
		}

		// Sort items inside folder by size descending
		for i := 0; i < len(entries); i++ {
			for j := i + 1; j < len(entries); j++ {
				if entries[i].Size < entries[j].Size {
					entries[i], entries[j] = entries[j], entries[i]
				}
			}
		}
		node.Entries = entries
	}

	// Sort large files list globally
	for i := 0; i < len(allFiles); i++ {
		for j := i + 1; j < len(allFiles); j++ {
			if allFiles[i].Size < allFiles[j].Size {
				allFiles[i], allFiles[j] = allFiles[j], allFiles[i]
			}
		}
	}
	if len(allFiles) > 10 {
		allFiles = allFiles[:10]
	}

	return rootNode, allFiles, nil
}

// Premium System Operations & Safety Controls
func openInExplorer(path string) error {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		info, err := os.Stat(path)
		if err == nil && info.IsDir() {
			cmd = exec.Command("explorer.exe", path)
		} else {
			cmd = exec.Command("explorer.exe", "/select,", path)
		}
	} else {
		cmd = exec.Command("xdg-open", path)
	}
	return cmd.Start()
}

func recyclePath(path string, size int64) error {
	// Guard #1: Absolute Safety boundaries verification
	if !fs.IsValidPath(path) {
		logDestructiveOperation("recycle", path, size, false)
		return fmt.Errorf("deleting system protected paths or drive letters is non-negotiably blocked for security reasons")
	}

	err := recyclePathNative(path)

	success := err == nil
	logDestructiveOperation("recycle", path, size, success)
	return err
}

func logDestructiveOperation(action, target string, size int64, success bool) {
	if os.Getenv("DU_NO_OPLOG") == "1" {
		return
	}

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

	_ = os.MkdirAll(logDir, 0755)
	logPath := filepath.Join(logDir, "operations.log")

	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()

	status := "SUCCESS"
	if !success {
		status = "FAILED"
	}

	timestamp := time.Now().UTC().Format(time.RFC3339)
	entry := fmt.Sprintf("%s | Command: analyze | Action: %s | Target: %s | Size: %d bytes | Status: %s\n",
		timestamp, action, target, size, status)
	_, _ = f.WriteString(entry)
}
