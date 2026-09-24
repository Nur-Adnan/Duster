package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Nur-Adnan/duster/internal/logging"
	"github.com/Nur-Adnan/duster/lib/fs"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

var analyzeJSON bool

var (
	analyzeNoHistory bool
	analyzeSinceFlag string
	analyzeSince     time.Duration
)

var AnalyzeCmd = &cobra.Command{
	Use:   "analyze [path]",
	Short: "Interactive TUI disk space explorer and visual analyzer",
	Long: `Recursively scan directories to analyze storage allocation, showing:
  - Total folder size and relative size percentage bars
  - Interactive drill-down navigation
  - Quick launch native Windows Explorer
  - Safe Recycle Bin integration
  - Global top 10 largest files viewer
  - What changed since the last scan of the same folder ([c] in the explorer,
    "changes" in --json): each scan keeps a small size snapshot under
    %LOCALAPPDATA%\Duster\history. Turn it off with --no-history.`,
	Args: cobra.MaximumNArgs(1),
	Run:  executeAnalyze,
}

func init() {
	AnalyzeCmd.Flags().BoolVar(&analyzeJSON, "json", false, "Output folder analysis metrics as a single JSON snapshot and exit immediately")
	AnalyzeCmd.Flags().BoolVar(&analyzeNoHistory, "no-history", false, "Neither compare with nor save a size snapshot of this folder")
	AnalyzeCmd.Flags().StringVar(&analyzeSinceFlag, "since", "", "Compare with the newest scan at least this old (e.g. 24h, 7d, 2w) instead of the previous one")
	AnalyzeCmd.MarkFlagsMutuallyExclusive("since", "no-history")
}

func executeAnalyze(cmd *cobra.Command, args []string) {
	targetPath := "."
	if len(args) > 0 {
		targetPath = args[0]
	}

	if analyzeSinceFlag != "" {
		d, err := parseSinceDuration(analyzeSinceFlag)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		analyzeSince = d
	}

	// Resolve environmental paths safely
	targetPath = fs.ResolveEnvPath(targetPath)
	absPath, err := filepath.Abs(targetPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error resolving path: %v\n", err)
		os.Exit(1)
	}
	if err := scanTargetError(absPath, false); err != nil {
		fmt.Fprintf(os.Stderr, "Error: cannot analyze %s: %v\n", absPath, err)
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

// scanTargetError reports why path can't be scanned: it doesn't exist, can't
// be read, or (needDir) isn't a folder. A mistyped path must fail with exit 1,
// not look like a successful scan of an empty folder.
func scanTargetError(path string, needDir bool) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if needDir && !info.IsDir() {
		return fmt.Errorf("%s is not a folder", path)
	}
	return nil
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
	// Changes is null on the first scan of a folder, and with --no-history.
	Changes      *changeReport `json:"changes"`
	HistoryNotes []string      `json:"history_notes,omitempty"`
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
	close(ch)
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
			SizeText: formatSize(entry.Size),
			IsDir:    entry.IsDir,
			Percent:  percent,
			SubItems: entry.Items,
		})
	}

	// Counts come from the scanned tree, so they agree with total_size (both
	// skip .git, node_modules, ...); a second full walk cost ~28% of runtime.
	filesCount, dirsCount := countFilesAndFolders(root)

	output := AnalyzeJSONOutput{
		Path:          target,
		TotalSize:     root.Size,
		TotalSizeText: formatSize(root.Size),
		DirsCount:     dirsCount,
		FilesCount:    filesCount,
		Entries:       jsonEntries,
		TopLargeFiles: large,
	}
	if !analyzeNoHistory {
		baseline, notes := recordScanHistory(root, analyzeSince, time.Now())
		if baseline != nil {
			report := explainChanges(baseline, root)
			output.Changes = &report
		}
		output.HistoryNotes = notes
	}

	data, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error encoding JSON: %v\n", err)
		os.Exit(1)
	}
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
	stale          bool // a recycle happened: in-memory ancestors may be out of date
	showLargeFiles bool
	largeFiles     []FileNode
	confirmRecycle bool
	errorMsg       string
	width, height  int
	// Cached once at scan completion: counting walks the whole tree, and
	// View runs on every keystroke, so recomputing there lagged large scans.
	fileCount   int
	folderCount int

	// What changed since the last scan. root is the first complete scan of
	// targetPath: jumps resolve against it. historyDone guards the single
	// snapshot a session saves; historyReady is set once the comparison has
	// loaded (or was skipped), so no half-state is ever drawn.
	root         *FolderNode
	baseline     *sizeSnapshot
	historyNotes []string
	historyDone  bool
	historyReady bool
	showChanges  bool
	changes      changeReport
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

type analyzeHistoryMsg struct {
	baseline *sizeSnapshot
	notes    []string
}

// analyzeHistoryCmd loads the comparison and saves this scan off the UI
// thread: both touch the disk.
func analyzeHistoryCmd(root *FolderNode) tea.Cmd {
	return func() tea.Msg {
		baseline, notes := recordScanHistory(root, analyzeSince, time.Now())
		return analyzeHistoryMsg{baseline: baseline, notes: notes}
	}
}

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
					m.stale = true // ancestors in historyStack still count the recycled item
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

		// While a (re)scan is in flight the tree/largeFiles are about to be
		// replaced; navigating or acting against the stale tree can index out
		// of range and also spawn concurrent scans. Only quitting is allowed.
		if m.scanning {
			if msg.String() == "q" || msg.String() == "ctrl+c" {
				return m, tea.Quit
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
			if m.selectedIdx < m.listLen()-1 {
				m.selectedIdx++
			}

		case "c", "C":
			m.showChanges = !m.showChanges
			m.showLargeFiles = false
			m.selectedIdx = 0

		case "enter", "l", "right":
			if m.showChanges {
				return m.jumpToChange()
			}
			if m.showLargeFiles || m.tree == nil {
				return m, nil
			}
			if len(m.tree.Entries) > 0 {
				entry := m.tree.Entries[m.selectedIdx]
				if entry.IsDir {
					// Descend in memory: the subtree was scanned with its parent,
					// so rescanning it (seconds on large trees) is wasted work.
					for _, sub := range m.tree.SubFolders {
						if sub.Path == entry.Path {
							m.historyStack = append(m.historyStack, m.tree)
							m.showNode(sub)
							return m, nil
						}
					}
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

		case "backspace", "h", "left", "b", "B":
			if m.showChanges {
				m.showChanges = false
				m.selectedIdx = 0
				return m, nil
			}
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

				// The parent is still in memory; rescan only when a recycle
				// since then may have left its sizes stale.
				if !m.stale {
					m.showNode(parent)
					return m, nil
				}
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
			if m.showChanges {
				if m.selectedIdx < len(m.changes.Entries) {
					path = nearestExisting(m.changes.Entries[m.selectedIdx].Path, m.targetPath)
				}
			} else if m.showLargeFiles {
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
			// The changes list can name things that no longer exist, so it
			// never deletes: Enter jumps to the item, and d works there.
			if m.showChanges {
				return m, nil
			}
			if m.showLargeFiles {
				if len(m.largeFiles) > 0 {
					m.confirmRecycle = true
				}
			} else if m.tree != nil && len(m.tree.Entries) > 0 {
				m.confirmRecycle = true
			}

		case "L":
			m.showLargeFiles = !m.showLargeFiles
			m.showChanges = false
			m.selectedIdx = 0
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case analyzeScanProgressMsg:
		// Ignore progress that arrives after the scan has finished (or after a
		// drill-down swapped scanning off): a stale frame would overwrite the
		// live view and re-attach a second listener to the channel.
		if !m.scanning {
			return m, nil
		}
		m.progress = scanProgressInfo(msg)
		return m, analyzeListenToScanProgress(m.scanChan)

	case analyzeScanCompleteMsg:
		m.scanning = false
		var cmd tea.Cmd
		if msg.Err != nil {
			m.errorMsg = fmt.Sprintf("Scan failed: %v", msg.Err)
		} else {
			m.tree = msg.Root
			m.largeFiles = msg.LargeFiles
			m.errorMsg = ""
			m.fileCount, m.folderCount = countFilesAndFolders(m.tree)
			// One snapshot per session, of the folder that was asked for:
			// rescans after a recycle or of a subfolder are not "a scan of
			// this folder" in the history's sense.
			if !m.historyDone && normalizeRoot(msg.Root.Path) == normalizeRoot(m.targetPath) {
				m.historyDone = true
				m.root = msg.Root
				if analyzeNoHistory {
					m.historyReady = true
				} else {
					cmd = analyzeHistoryCmd(msg.Root)
				}
			}
			m.refreshChanges()
		}
		// The new tree may be smaller than the cursor position from the old
		// one; an unclamped index panics on the next o/d/enter keypress.
		if m.selectedIdx >= m.listLen() {
			m.selectedIdx = 0
		}
		return m, cmd

	case analyzeHistoryMsg:
		m.baseline = msg.baseline
		m.historyNotes = msg.notes
		m.historyReady = true
		m.refreshChanges()
		if m.selectedIdx >= m.listLen() {
			m.selectedIdx = 0
		}
		return m, nil
	}

	return m, nil
}

func (m analyzeModel) View() string {
	// ── Scanning state ─────────────────────────────────────────────────
	if m.scanning {
		var s strings.Builder
		width := m.width
		if width < 96 {
			width = 96
		}

		s.WriteString(renderAnalyzeHeader(width, m.targetPath))
		s.WriteString("\n")
		s.WriteString("  " + styleSuccess.Render("Scanning filesystem in background...") + "\n\n")

		s.WriteString(fmt.Sprintf("  %-16s: %s\n", styleSilverText("Dirs Scanned"), styleValue.Render(formatInt(m.progress.DirsScanned))))
		s.WriteString(fmt.Sprintf("  %-16s: %s\n", styleSilverText("Files Scanned"), styleValue.Render(formatInt(m.progress.FilesScanned))))
		s.WriteString(fmt.Sprintf("  %-16s: %s\n\n", styleSilverText("Total Size"), styleWarning.Render(formatSize(m.progress.TotalSize))))

		currPath := clampTail(m.progress.CurrentPath, 80)
		s.WriteString("  " + styleMuted.Render(currPath) + "\n\n")

		s.WriteString("  " + styleWarning.Render("[q] Abort Scan") + "\n\n")
		s.WriteString(styleValue.Render("C:\\>") + lipgloss.NewStyle().Foreground(colorMint).Render("█") + "\n")
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
			yellowColorStyle.Render(formatSize(size)),
			styleAccent.Render("[y] Yes, Recycle"),
			styleMuted.Render("[n] Cancel"),
		)
	}

	if m.tree == nil {
		return fmt.Sprintf("\n  %s: %s\n\n  Press [q] to exit.", styleDanger.Render("Error"), m.errorMsg)
	}

	width := m.width
	if width < 96 {
		width = 96
	}

	var s strings.Builder

	// Part 1: Header Area (Command + ASCII + Meta + Solid Border)
	s.WriteString(renderAnalyzeHeader(width, m.tree.Path))
	s.WriteString("\n")

	if m.errorMsg != "" {
		s.WriteString("  " + styleDanger.Render(" "+m.errorMsg+" ") + "\n\n")
	}

	// Part 2: Path Analysis Summary (counts cached at scan completion)
	s.WriteString(renderPathSummary(formatSize(m.tree.Size), m.fileCount, m.folderCount))
	s.WriteString(m.renderChangeSummary())

	// Solid cyan divider line
	s.WriteString(lipgloss.NewStyle().Foreground(colorSkyBlue).Render(strings.Repeat("─", width)) + "\n\n")

	// Part 3: Disk Usage Table Headers
	s.WriteString(renderTableHeaders() + "\n")
	// Gray dashed separator line
	s.WriteString(lipgloss.NewStyle().Foreground(colorDimGray).Render(strings.Repeat("-", width)) + "\n")

	// Part 4: Disk Usage Table Rows (6 rows)
	const maxVisible = 6
	entries := m.tree.Entries
	total := len(entries)
	start := 0
	end := total
	if total > maxVisible {
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
		// Determine selection status
		isSelected := !m.showLargeFiles && !m.showChanges && i == m.selectedIdx
		s.WriteString(renderTableRow(i, entry, m.tree.Size, isSelected) + "\n")
	}

	if total == 0 {
		s.WriteString("  " + styleMuted.Render("[Empty directory]") + "\n")
	}

	// Solid cyan divider line
	s.WriteString("\n" + lipgloss.NewStyle().Foreground(colorSkyBlue).Render(strings.Repeat("─", width)) + "\n\n")

	// Part 5: Largest Files Panel, or what changed since the last scan
	if m.showChanges {
		s.WriteString(m.renderChangesPanel())
	} else {
		s.WriteString(lipgloss.NewStyle().Foreground(colorSkyBlue).Bold(true).Render("Largest Files:") + "\n")

		limit := len(m.largeFiles)
		if limit > 5 {
			limit = 5
		}
		for i := 0; i < limit; i++ {
			file := m.largeFiles[i]
			isSelected := false
			if m.showLargeFiles && i == m.selectedIdx {
				isSelected = true
			}
			s.WriteString(renderLargestFileRow(i, file, m.tree.Path, isSelected) + "\n")
		}
		if limit == 0 {
			s.WriteString("  " + styleMuted.Render("No files found.") + "\n")
		}
	}

	// Solid cyan divider line
	s.WriteString("\n" + lipgloss.NewStyle().Foreground(colorSkyBlue).Render(strings.Repeat("─", width)) + "\n\n")

	// Part 6: Footer Actions Bar
	s.WriteString(renderFooterActions() + "\n\n")

	// Authentic CMD Prompt at the bottom with blinking green cursor
	s.WriteString(styleValue.Render("C:\\>") + lipgloss.NewStyle().Foreground(colorMint).Render("█") + "\n")

	return s.String()
}

// Traversal Engine Helper Methods
func scanDirectory(root string, progressChan chan<- scanProgressInfo) (*FolderNode, []FileNode, error) {
	root = filepath.Clean(root)

	folderMap := make(map[string]*FolderNode)

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
	sort.Slice(paths, func(i, j int) bool {
		return len(paths[i]) > len(paths[j])
	})

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
	}

	// Entries are built per displayed folder (showNode). Building them for
	// every folder, plus a global list of all files, stored each file 3 times.
	buildEntries(rootNode)
	return rootNode, topFiles(rootNode, 10), nil
}

// buildEntries fills n.Entries (subfolders and files, largest first) once.
func buildEntries(n *FolderNode) {
	if n.Entries != nil {
		return
	}
	entries := make([]EntryInfo, 0, len(n.SubFolders)+len(n.Files))
	for _, sub := range n.SubFolders {
		entries = append(entries, EntryInfo{
			Name:  sub.Name + "\\",
			Path:  sub.Path,
			Size:  sub.Size,
			IsDir: true,
			Items: len(sub.SubFolders) + len(sub.Files),
		})
	}
	for _, f := range n.Files {
		entries = append(entries, EntryInfo{
			Name:  filepath.Base(f.Path),
			Path:  f.Path,
			Size:  f.Size,
			IsDir: false,
		})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Size > entries[j].Size
	})
	n.Entries = entries
}

// topFiles returns the k largest files under n, largest first, in one pass
// with a k-sized buffer instead of sorting every file.
func topFiles(n *FolderNode, k int) []FileNode {
	top := make([]FileNode, 0, k+1)
	var walk func(*FolderNode)
	walk = func(n *FolderNode) {
		for _, f := range n.Files {
			if len(top) == k && f.Size <= top[k-1].Size {
				continue
			}
			i := sort.Search(len(top), func(i int) bool { return top[i].Size < f.Size })
			top = append(top, FileNode{})
			copy(top[i+1:], top[i:])
			top[i] = f
			if len(top) > k {
				top = top[:k]
			}
		}
		for _, sub := range n.SubFolders {
			walk(sub)
		}
	}
	walk(n)
	return top
}

// showNode displays an already-scanned folder without touching the disk.
func (m *analyzeModel) showNode(n *FolderNode) {
	buildEntries(n)
	m.tree = n
	m.selectedIdx = 0
	m.largeFiles = topFiles(n, 10)
	m.fileCount, m.folderCount = countFilesAndFolders(n)
	m.refreshChanges()
}

// Premium System Operations & Safety Controls
func openInExplorer(path string) error {
	// Absolute path: a bare "explorer.exe" is resolved through PATH.
	explorer := filepath.Join(secureWindowsDir(), "explorer.exe")
	var cmd *exec.Cmd
	info, err := os.Stat(path)
	if err == nil && info.IsDir() {
		cmd = exec.Command(explorer, path)
	} else {
		cmd = exec.Command(explorer, "/select,", path)
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }() // reap the process handle
	return nil
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

// logDestructiveOperation delegates to the shared structured logging system,
// which also rotates the log; the old local copy grew operations.log unbounded.
func logDestructiveOperation(action, target string, size int64, success bool) {
	logging.LogDestructiveOperation("analyze", action, target, size, success)
}

// ── Redesigned Disk Usage Explorer Layout Helpers ─────────────────────────────

func formatSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	val := float64(bytes) / float64(div)
	suffix := fmt.Sprintf("%cB", "KMGTPE"[exp])

	if val == float64(int64(val)) {
		return fmt.Sprintf("%d %s", int64(val), suffix)
	}

	s := fmt.Sprintf("%.2f", val)
	s = strings.TrimSuffix(s, "0")
	s = strings.TrimSuffix(s, ".0")
	return fmt.Sprintf("%s %s", s, suffix)
}

func formatRelativePath(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	rel = strings.ReplaceAll(rel, "/", "\\")
	if !strings.HasPrefix(rel, ".\\") && !strings.HasPrefix(rel, "..\\") {
		rel = ".\\" + rel
	}
	return rel
}

func renderAnalyzeHeader(width int, path string) string {
	var sb strings.Builder

	promptPath := strings.ReplaceAll(path, "/", "\\")
	cmdPrompt := styleValue.Render("C:\\>") + lipgloss.NewStyle().Foreground(colorMint).Render("du analyze") + " " + styleValue.Render(promptPath) + "\n"
	sb.WriteString(cmdPrompt)

	broom := []string{
		"    / ",
		"   /  ",
		"  /   ",
		" /_   ",
		"\\--/  ",
		"/__/  ",
	}

	duster := []string{
		"  _ _ _ _ _    _       _    _ _ _ _    _ _ _ _ _   _ _ _ _ _   _ _ _ _ _  ",
		" / _ _ _ _ \\  | |     | |  / _ _ _ \\  |_ _ _ _ _| |  _ _ _ _| |  _ _ _  \\ ",
		"| |       \\ \\ | |     | | |  (_ _ _ _     | |     | |___      | |_ _ _/ / ",
		"| |        | || |     | |  \\_ _ _ _ \\     | |     |  _ _|     |  _ _ _ _/ ",
		"| |       / / | |_ _ _| |  _ _ _ _ ) |    | |     | |_ _ _ _  | |   \\ \\   ",
		" \\_ _ _ _ _/   \\_ _ _ _ /  \\_ _ _ _ /     |_|     |_ _ _ _ _| |_|    \\_\\  ",
	}

	styleBroom := lipgloss.NewStyle().Foreground(colorGold)
	styleDuster := lipgloss.NewStyle().Foreground(colorSkyBlue)
	styleSep := lipgloss.NewStyle().Foreground(colorDimGray)
	styleTitle := styleValue.Render("Disk Usage Analyzer")
	labelPath := styleSilverText("Path: ") + lipgloss.NewStyle().Foreground(colorMint).Render(promptPath)

	for i := 0; i < 6; i++ {
		bLine := styleBroom.Render(broom[i])
		dLine := styleDuster.Render(duster[i])
		sep := styleSep.Render(" │ ")

		var rightSide string
		switch i {
		case 1:
			rightSide = styleTitle
		case 2:
			rightSide = labelPath
		default:
			rightSide = ""
		}

		sb.WriteString(bLine + " " + dLine + sep + rightSide + "\n")
	}

	dividerWidth := width
	if dividerWidth < 96 {
		dividerWidth = 96
	}
	sb.WriteString(lipgloss.NewStyle().Foreground(colorSkyBlue).Render(strings.Repeat("─", dividerWidth)) + "\n")

	return sb.String()
}

func renderPathSummary(totalSize string, files, folders int) string {
	var sb strings.Builder
	labelStyle := lipgloss.NewStyle().Foreground(colorSilver)
	valueStyle := lipgloss.NewStyle().Foreground(colorMint).Bold(true)

	sb.WriteString(labelStyle.Render("Total Size:    ") + valueStyle.Render(totalSize) + "\n")
	sb.WriteString(labelStyle.Render("Total Files:   ") + valueStyle.Render(formatInt(files)) + "\n")
	sb.WriteString(labelStyle.Render("Total Folders: ") + valueStyle.Render(formatInt(folders)) + "\n")

	return sb.String()
}

func renderTableHeaders() string {
	headerStyle := lipgloss.NewStyle().Foreground(colorSkyBlue).Bold(true)
	sepStyle := lipgloss.NewStyle().Foreground(colorSkyBlue)

	h1 := headerStyle.Render("#   Path" + strings.Repeat(" ", 44))
	sep := sepStyle.Render("│")
	h2 := headerStyle.Render("     Size     ")
	h3 := headerStyle.Render("   % of Total   ")
	h4 := headerStyle.Render("   Files   ")

	return h1 + sep + h2 + sep + h3 + sep + h4
}

func renderTableRow(i int, entry EntryInfo, totalSize int64, isSelected bool) string {
	numStr := fmt.Sprintf("%-4d", i+1)

	name := entry.Name
	if entry.IsDir && !strings.HasSuffix(name, "\\") {
		name += "\\"
	}
	pathStr := padRight(truncateString(name, 47), 48)
	sizeVal := formatSize(entry.Size)
	sizeStr := fmt.Sprintf("%14s", sizeVal)

	pct := 0.0
	if totalSize > 0 {
		pct = float64(entry.Size) / float64(totalSize) * 100
	}
	pctStr := fmt.Sprintf("%15.1f%% ", pct)

	filesStr := fmt.Sprintf("%11s", formatInt(entry.Items))

	var numRendered, pathRendered, sizeRendered, pctRendered, filesRendered string
	sep := lipgloss.NewStyle().Foreground(colorDimGray).Render("│")

	if isSelected {
		selStyle := lipgloss.NewStyle().Foreground(colorSkyBlue).Bold(true)
		numRendered = selStyle.Render(numStr)
		pathRendered = selStyle.Render(pathStr)
		sizeRendered = selStyle.Render(sizeStr)
		pctRendered = selStyle.Render(pctStr)
		filesRendered = selStyle.Render(filesStr)
	} else {
		numRendered = lipgloss.NewStyle().Foreground(colorSilver).Render(numStr)
		if entry.IsDir {
			pathRendered = lipgloss.NewStyle().Foreground(colorSkyBlue).Render(pathStr)
		} else {
			pathRendered = lipgloss.NewStyle().Foreground(colorSilver).Render(pathStr)
		}
		sizeRendered = lipgloss.NewStyle().Foreground(colorAmber).Render(sizeStr)
		pctRendered = lipgloss.NewStyle().Foreground(colorAmber).Render(pctStr)
		filesRendered = lipgloss.NewStyle().Foreground(colorWhite).Render(filesStr)
	}

	return numRendered + pathRendered + sep + sizeRendered + sep + pctRendered + sep + filesRendered
}

func renderLargestFileRow(i int, file FileNode, rootPath string, isSelected bool) string {
	rankStr := fmt.Sprintf("%d. ", i+1)
	relPath := formatRelativePath(rootPath, file.Path)

	leftPart := rankStr + relPath
	leftRendered := padRight(leftPart, 76)

	sizeVal := formatSize(file.Size)
	rightRendered := fmt.Sprintf("%20s", sizeVal)

	if isSelected {
		selStyle := lipgloss.NewStyle().Foreground(colorSkyBlue).Bold(true)
		return selStyle.Render(leftRendered + rightRendered)
	} else {
		pathStyle := lipgloss.NewStyle().Foreground(colorSilver)
		sizeStyle := lipgloss.NewStyle().Foreground(colorAmber)
		if file.Size > 1024*1024*1024 {
			sizeStyle = lipgloss.NewStyle().Foreground(colorCoral).Bold(true)
		}
		return pathStyle.Render(leftRendered) + sizeStyle.Render(rightRendered)
	}
}

func renderFooterActions() string {
	actionLabel := lipgloss.NewStyle().Foreground(colorAmber).Bold(true).Render("Actions: ")

	bracketStyle := lipgloss.NewStyle().Foreground(colorSkyBlue)
	keyStyle := lipgloss.NewStyle().Foreground(colorMint).Bold(true)
	descStyle := lipgloss.NewStyle().Foreground(colorSilver)

	renderAction := func(k, desc string) string {
		return bracketStyle.Render("[") + keyStyle.Render(k) + bracketStyle.Render("]") + descStyle.Render(desc)
	}

	a1 := renderAction("D", "elete")
	a2 := renderAction("O", "pen")
	a5 := renderAction("C", "hanges")
	a3 := renderAction("B", "ack")
	a4 := renderAction("Q", "uit")

	return "  " + actionLabel + a1 + "  " + a2 + "  " + a5 + "  " + a3 + "  " + a4
}

func countFilesAndFolders(node *FolderNode) (int, int) {
	var files, folders int
	var walk func(n *FolderNode)
	walk = func(n *FolderNode) {
		folders++
		files += len(n.Files)
		for _, sub := range n.SubFolders {
			walk(sub)
		}
	}
	walk(node)
	return files, folders - 1
}

func styleSilverText(s string) string {
	return lipgloss.NewStyle().Foreground(colorSilver).Render(s)
}
