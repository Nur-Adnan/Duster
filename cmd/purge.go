package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Nur-Adnan/duster/internal/logging"
	"github.com/Nur-Adnan/duster/lib/fs"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

// Developer build artifact names mapped to their respective platform tags.
var developerArtifacts = map[string]string{
	"node_modules": "Node.js",
	"target":       "Rust/Cargo",
	"bin":          ".NET/Build",
	"obj":          ".NET/Build",
	"build":        "Build Output",
	"dist":         "Dist Output",
	".gradle":      "Gradle Cache",
	".m2":          "Maven Cache",
	"vendor":       "Vendor Cache",
	".serverless":  "Serverless",
	".sst":         "SST Framework",
}

// artifactMarkers lists the project manifests that must sit next to an ambiguous
// directory before it is treated as build output. Generic names like bin/build/
// dist/target/vendor are also perfectly ordinary user folders (e.g.
// %USERPROFILE%\go\bin holds installed tools, Documents\build may be real data),
// so without this check `purge -y` would permanently delete them. Entries
// starting with "." are matched against a file extension; others against the
// full file name. Names absent from this map (node_modules, .m2, .gradle, ...)
// are unambiguous enough to match on name alone.
var artifactMarkers = map[string][]string{
	"target": {"Cargo.toml"},
	"bin":    {".csproj", ".sln", ".vbproj", ".fsproj"},
	"obj":    {".csproj", ".sln", ".vbproj", ".fsproj"},
	"build":  {"package.json", "build.gradle", "build.gradle.kts", "pom.xml", "CMakeLists.txt"},
	"dist":   {"package.json"},
	"vendor": {"go.mod", "composer.json"},
}

// isLikelyBuildArtifact reports whether a directory named `name` (lowercased) at
// `path` is genuinely build output. Ambiguous names require a sibling project
// manifest; if the parent can't be read the answer is "no" so we fail closed.
func isLikelyBuildArtifact(name, path string) bool {
	markers, ambiguous := artifactMarkers[name]
	if !ambiguous {
		return true
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		return false
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		en := e.Name()
		for _, mk := range markers {
			if strings.HasPrefix(mk, ".") {
				if strings.EqualFold(filepath.Ext(en), mk) {
					return true
				}
			} else if strings.EqualFold(en, mk) {
				return true
			}
		}
	}
	return false
}

// Purge command flags
var (
	purgePath   string
	purgeDryRun bool
	purgeSafe   bool
	purgeJSON   bool
	purgeYes    bool
)

// Premium Lipgloss Styles (Zero-Allocation, prefixed to avoid package conflicts)
var (
	purgeTealColor  = lipgloss.Color("#008080")
	purgeCyanColor  = lipgloss.Color("#00FFFF")
	purgeGrayColor  = lipgloss.Color("#666666")
	purgeWhiteColor = lipgloss.Color("#FFFFFF")
	purgeRedColor   = lipgloss.Color("#FF0000")
	purgeGreenColor = lipgloss.Color("#00FF00")

	purgeHeaderStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(purgeCyanColor).
				Padding(0, 1)

	purgeBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(purgeTealColor).
			Padding(1, 2).
			Width(83)

	purgeFooterStyle = lipgloss.NewStyle().
				Foreground(purgeGrayColor).
				PaddingTop(1).
				PaddingLeft(2)

	purgeDividerStyle = lipgloss.NewStyle().
				Foreground(purgeGrayColor)

	purgeSuccessStyle = lipgloss.NewStyle().Bold(true).Foreground(purgeGreenColor)
	purgeFailStyle    = lipgloss.NewStyle().Bold(true).Foreground(purgeRedColor)
)

var PurgeCmd = &cobra.Command{
	Use:   "purge",
	Short: "Find and clean developer build artifacts recursively to reclaim space",
	Long: `Recursively scans a specified workspace or path for developer build output and caches
such as node_modules, target, bin, obj, build, dist, .gradle, and vendor folders.
Presents an interactive checkbox interface to selectively purge these targets in bulk.`,
	Run: executePurge,
}

func init() {
	PurgeCmd.Flags().StringVarP(&purgePath, "path", "p", ".", "Starting directory path for recursive developer artifact scan")
	PurgeCmd.Flags().BoolVarP(&purgeDryRun, "dry-run", "d", false, "Simulate scanning and deletion without modifying filesystem")
	PurgeCmd.Flags().BoolVarP(&purgeSafe, "safe", "s", false, "Move target folders to Windows Recycle Bin instead of deleting permanently")
	PurgeCmd.Flags().BoolVar(&purgeJSON, "json", false, "Output discovered developer build artifacts as a structured JSON snapshot and exit immediately")
	PurgeCmd.Flags().BoolVarP(&purgeYes, "yes", "y", false, "Skip interactive prompts and permanently delete all detected build artifacts")
}

func executePurge(cmd *cobra.Command, args []string) {
	// Resolve path safely
	resolvedPath := fs.ResolveEnvPath(purgePath)
	absPath, err := filepath.Abs(resolvedPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error resolving path: %v\n", err)
		os.Exit(1)
	}

	// Verify target path meets basic safety rules (e.g. not deleting system files or drive roots)
	if !fs.IsValidPath(absPath) {
		fmt.Fprintf(os.Stderr, "Error: The target path '%s' is critical/system protected. Scanning is blocked for safety.\n", absPath)
		os.Exit(1)
	}

	// Explicit --json always wins: snapshot and exit
	if purgeJSON {
		runHeadlessPurge(absPath)
		return
	}

	// Direct bulk non-interactive deletion if -y is specified. This must take
	// precedence over pipe detection: `du purge -y | tee log` documents an
	// explicit intent to delete, not to emit a JSON plan.
	if purgeYes {
		runNonInteractivePurge(absPath)
		return
	}

	// Piped without --yes: emit the JSON plan, never delete
	if isPiped() {
		runHeadlessPurge(absPath)
		return
	}

	// Start TUI flow
	m := initialPurgeModel(absPath)
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running purge interface: %v\n", err)
		os.Exit(1)
	}
}

// DiscoveredArtifact represents a single target build directory
type DiscoveredArtifact struct {
	Path      string `json:"path"`
	Name      string `json:"name"`
	Type      string `json:"type"`
	Framework string `json:"framework"`
	Size      int64  `json:"size"`
	Selected  bool   `json:"selected"`
}

// Bubble Tea State Machine
type purgeState int

const (
	stateScanning purgeState = iota
	stateSelecting
	stateConfirming
	statePurging
	stateFinished
)

type purgeModel struct {
	rootPath     string
	state        purgeState
	artifacts    []DiscoveredArtifact
	cursor       int
	scrollOffset int
	latestFound  string
	totalFound   int
	totalSize    int64
	selectedSize int64
	currentPurge int
	purgeErr     error
	purgedCount  int
	purgedBytes  int64
	purgeFailed  int
	width        int
	height       int
	scanChan     chan scanProgressMsg
	purgeChan    chan purgeProgressMsg
}

// Bubble Tea custom messages
type scanProgressMsg struct {
	LatestFound string
	Size        int64
	Count       int
}

type scanCompleteMsg struct {
	artifacts []DiscoveredArtifact
}

type purgeProgressMsg struct {
	Index int
	Path  string
	Size  int64
	Err   error
}

type purgeCompleteMsg struct {
	reclaimed int64
	count     int
}

func initialPurgeModel(root string) purgeModel {
	return purgeModel{
		rootPath:  root,
		state:     stateScanning,
		cursor:    0,
		scanChan:  make(chan scanProgressMsg, 200),
		purgeChan: make(chan purgeProgressMsg, 200),
	}
}

func listenToScanProgress(ch chan scanProgressMsg) tea.Cmd {
	return func() tea.Msg {
		progress, ok := <-ch
		if !ok {
			return nil
		}
		return progress
	}
}

func runScanCmd(root string, ch chan scanProgressMsg) tea.Cmd {
	return func() tea.Msg {
		var list []DiscoveredArtifact
		_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if !d.IsDir() {
				return nil
			}
			name := d.Name()
			if name == ".git" || name == ".svn" || name == ".vscode" || name == "$Recycle.Bin" || name == "System Volume Information" {
				return filepath.SkipDir
			}
			if !fs.IsValidPath(path) {
				return filepath.SkipDir
			}

			if framework, exists := developerArtifacts[strings.ToLower(name)]; exists {
				if !isLikelyBuildArtifact(strings.ToLower(name), path) {
					return nil // ambiguous folder with no project marker — keep scanning inside
				}
				size := calculateDirSize(path)
				art := DiscoveredArtifact{
					Path:      path,
					Name:      name,
					Type:      strings.ToLower(name),
					Framework: framework,
					Size:      size,
					Selected:  true,
				}
				list = append(list, art)
				ch <- scanProgressMsg{
					LatestFound: path,
					Size:        size,
					Count:       len(list),
				}
				return filepath.SkipDir
			}
			return nil
		})
		close(ch)
		return scanCompleteMsg{artifacts: list}
	}
}

func listenToPurgeProgress(ch chan purgeProgressMsg) tea.Cmd {
	return func() tea.Msg {
		prog, ok := <-ch
		if !ok {
			return nil
		}
		return prog
	}
}

func runPurgeCmd(artifacts []DiscoveredArtifact, ch chan purgeProgressMsg, safe, dry bool) tea.Cmd {
	return func() tea.Msg {
		var reclaimed int64
		count := 0

		for _, a := range artifacts {
			if !a.Selected {
				continue
			}

			count++
			var err error
			if !dry {
				if safe {
					err = purgeRecyclePath(a.Path, a.Size)
				} else {
					err = purgePermanentPath(a.Path, a.Size)
				}
			}

			ch <- purgeProgressMsg{
				Index: count,
				Path:  a.Path,
				Size:  a.Size,
				Err:   err,
			}

			if err == nil {
				reclaimed += a.Size
			}
		}

		close(ch)
		return purgeCompleteMsg{
			reclaimed: reclaimed,
			count:     count,
		}
	}
}

func (m purgeModel) Init() tea.Cmd {
	return tea.Batch(
		runScanCmd(m.rootPath, m.scanChan),
		listenToScanProgress(m.scanChan),
	)
}

func (m purgeModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			// Emergency exit is always available, even mid-purge.
			return m, tea.Quit

		case "q", "esc":
			if m.state == stateConfirming {
				m.state = stateSelecting
				return m, nil
			}
			if m.state == statePurging {
				return m, nil // deletions in flight; footer says do not interrupt
			}
			return m, tea.Quit

		case "n", "N":
			if m.state == stateConfirming {
				m.state = stateSelecting
				return m, nil
			}

		case "up", "k":
			if m.state == stateSelecting && len(m.artifacts) > 0 {
				m.cursor--
				if m.cursor < 0 {
					m.cursor = len(m.artifacts) - 1
				}
				m.adjustScroll()
			}

		case "down", "j":
			if m.state == stateSelecting && len(m.artifacts) > 0 {
				m.cursor++
				if m.cursor >= len(m.artifacts) {
					m.cursor = 0
				}
				m.adjustScroll()
			}

		case " ":
			if m.state == stateSelecting && len(m.artifacts) > 0 {
				m.artifacts[m.cursor].Selected = !m.artifacts[m.cursor].Selected
				m.recalculateSelected()
			}

		case "a", "A":
			if m.state == stateSelecting && len(m.artifacts) > 0 {
				anyUnselected := false
				for _, a := range m.artifacts {
					if !a.Selected {
						anyUnselected = true
						break
					}
				}
				for i := range m.artifacts {
					m.artifacts[i].Selected = anyUnselected
				}
				m.recalculateSelected()
			}

		case "enter", "y", "Y":
			if m.state == stateSelecting {
				if m.selectedSize > 0 {
					m.state = stateConfirming
				}
			} else if m.state == stateConfirming {
				m.state = statePurging
				return m, tea.Batch(
					runPurgeCmd(m.artifacts, m.purgeChan, purgeSafe, purgeDryRun),
					listenToPurgeProgress(m.purgeChan),
				)
			}
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case scanProgressMsg:
		m.latestFound = msg.LatestFound
		m.totalFound = msg.Count
		m.totalSize += msg.Size
		return m, listenToScanProgress(m.scanChan)

	case scanCompleteMsg:
		m.artifacts = msg.artifacts
		m.state = stateSelecting
		var total int64
		for _, a := range m.artifacts {
			total += a.Size
		}
		m.totalSize = total
		m.recalculateSelected()
		return m, nil

	case purgeProgressMsg:
		m.currentPurge = msg.Index
		m.latestFound = msg.Path
		if msg.Err != nil {
			m.purgeErr = msg.Err
			m.purgeFailed++
		}
		return m, listenToPurgeProgress(m.purgeChan)

	case purgeCompleteMsg:
		m.state = stateFinished
		m.purgedCount = msg.count
		m.purgedBytes = msg.reclaimed
		return m, nil
	}

	return m, nil
}

func (m *purgeModel) adjustScroll() {
	maxVisible := 12
	if m.cursor < m.scrollOffset {
		m.scrollOffset = m.cursor
	} else if m.cursor >= m.scrollOffset+maxVisible {
		m.scrollOffset = m.cursor - maxVisible + 1
	}
}

func (m *purgeModel) recalculateSelected() {
	var size int64
	for _, a := range m.artifacts {
		if a.Selected {
			size += a.Size
		}
	}
	m.selectedSize = size
}

func (m purgeModel) View() string {
	var doc strings.Builder

	// Top title bar
	doc.WriteString("\n")
	doc.WriteString(purgeHeaderStyle.Render("Duster Developer Purge Dashboard"))
	if purgeDryRun {
		doc.WriteString("  |  " + purgeFailStyle.Render("DRY RUN MODE (SIMULATION)"))
	} else if purgeSafe {
		doc.WriteString("  |  " + purgeSuccessStyle.Render("SAFE MODE (RECYCLE BIN)"))
	} else {
		doc.WriteString("  |  " + purgeFailStyle.Render("PERMANENT CLEAN MODE"))
	}
	doc.WriteString("\n")
	doc.WriteString(purgeDividerStyle.Render("  ═══════════════════════════════════════════════════════════════════════\n\n"))

	var boxContent strings.Builder

	switch m.state {
	case stateScanning:
		boxContent.WriteString(fmt.Sprintf("🔍  Scanning for developer build artifacts under:\n    %s\n\n", purgeWhiteText(m.rootPath)))
		boxContent.WriteString(fmt.Sprintf("    Discovered targets : %s\n", purgeWhiteText(fmt.Sprintf("%d folders", m.totalFound))))
		boxContent.WriteString(fmt.Sprintf("    Recoverable space  : %s\n\n", purgeWhiteText(formatBytes(m.totalSize))))
		if m.latestFound != "" {
			shortPath := m.latestFound
			if len(shortPath) > 55 {
				shortPath = "..." + shortPath[len(shortPath)-52:]
			}
			boxContent.WriteString(purgeGrayText(fmt.Sprintf("    Reading: %s", shortPath)))
		} else {
			boxContent.WriteString(purgeGrayText("    Analyzing workspace directories..."))
		}

	case stateSelecting:
		if len(m.artifacts) == 0 {
			boxContent.WriteString("  ✓ No developer build cache directories found in this workspace!\n\n")
			boxContent.WriteString("  Either this folder is already clean, or no matching directories\n")
			boxContent.WriteString("  (node_modules, target, bin, obj, build, dist, .gradle, vendor) exist.\n")
		} else {
			boxContent.WriteString(fmt.Sprintf("Discovered %d build artifact directories. Select folders to purge:\n\n", len(m.artifacts)))

			boxContent.WriteString(purgeGrayText("     Target Path                                      Tech Tag       Size\n"))
			boxContent.WriteString(purgeDividerStyle.Render("     ───────────────────────────────────────────────────────────────────────\n"))

			maxVisible := 12
			endIdx := m.scrollOffset + maxVisible
			if endIdx > len(m.artifacts) {
				endIdx = len(m.artifacts)
			}

			for i := m.scrollOffset; i < endIdx; i++ {
				art := m.artifacts[i]

				chk := "[ ]"
				if art.Selected {
					chk = "[x]"
				}

				linePrefix := "  "
				if i == m.cursor {
					linePrefix = "▸ "
					chk = purgeCyanText(chk)
				}

				shortPath := art.Path
				if len(shortPath) > 42 {
					shortPath = "..." + shortPath[len(shortPath)-39:]
				}

				line := fmt.Sprintf("%s%s  %-45s %-12s %10s\n",
					linePrefix,
					chk,
					shortPath,
					fmt.Sprintf("[%s]", art.Framework),
					formatBytes(art.Size),
				)

				if i == m.cursor {
					boxContent.WriteString(purgeWhiteText(line))
				} else {
					boxContent.WriteString(line)
				}
			}

			if len(m.artifacts) > maxVisible {
				boxContent.WriteString(purgeGrayText(fmt.Sprintf("\n  [Line %d of %d]  ──────────────────────────────────────────────────────────", m.cursor+1, len(m.artifacts))))
			}

			boxContent.WriteString(fmt.Sprintf("\n\n  Selected for Purging: %s to reclaim", purgeSuccessStyle.Render(formatBytes(m.selectedSize))))
		}

	case stateConfirming:
		boxContent.WriteString("⚠️  " + purgeFailStyle.Render("CONFIRM BULK PURGE TRANSACTION") + "\n\n")
		if purgeDryRun {
			boxContent.WriteString(fmt.Sprintf("  You are about to simulate purging %d selected build artifact folders.\n", countSelected(m.artifacts)))
			boxContent.WriteString("  No actual files will be deleted in dry-run simulation mode.\n\n")
		} else if purgeSafe {
			boxContent.WriteString(fmt.Sprintf("  You are about to move %d selected folders to the Recycle Bin.\n", countSelected(m.artifacts)))
			boxContent.WriteString(fmt.Sprintf("  Reclaimable space: %s\n\n", formatBytes(m.selectedSize)))
		} else {
			boxContent.WriteString(fmt.Sprintf("  "+purgeFailStyle.Render("WARNING:")+" This operation will permanently delete %d selected cache directories!\n", countSelected(m.artifacts)))
			boxContent.WriteString(fmt.Sprintf("  Total space to destroy: %s\n", formatBytes(m.selectedSize)))
			boxContent.WriteString("  This action is native, fast, and cannot be undone!\n\n")
		}
		boxContent.WriteString("  Are you absolutely sure you want to proceed? [y to Purge / n to Cancel]")

	case statePurging:
		boxContent.WriteString("🔥  " + purgeFailStyle.Render("PURGING DEVELOPER CACHES & ARTIFACTS") + "\n\n")
		selected := countSelected(m.artifacts)
		boxContent.WriteString(fmt.Sprintf("  Purging progress: %d / %d folders cleaned\n\n", m.currentPurge, selected))

		if m.latestFound != "" {
			shortPath := m.latestFound
			if len(shortPath) > 55 {
				shortPath = "..." + shortPath[len(shortPath)-52:]
			}
			boxContent.WriteString(fmt.Sprintf("  Current Target: %s\n", purgeWhiteText(shortPath)))
		}

	case stateFinished:
		if m.purgeFailed > 0 {
			boxContent.WriteString("⚠️  " + purgeFailStyle.Render("DEVELOPER WORKSPACE PURGE COMPLETED WITH ERRORS") + "\n\n")
		} else {
			boxContent.WriteString("✓  " + purgeSuccessStyle.Render("DEVELOPER WORKSPACE PURGE COMPLETED") + "\n\n")
		}
		if purgeDryRun {
			boxContent.WriteString("  Dry-run scan completed successfully.\n")
			boxContent.WriteString(fmt.Sprintf("  Simulated cleaning of %d developer directories.\n", countSelected(m.artifacts)))
			boxContent.WriteString(fmt.Sprintf("  Total simulated reclaimed space: %s\n\n", formatBytes(m.selectedSize)))
		} else {
			boxContent.WriteString(fmt.Sprintf("  Cleaned %d of %d selected artifact folders.\n", m.purgedCount-m.purgeFailed, m.purgedCount))
			boxContent.WriteString(fmt.Sprintf("  Total active disk space reclaimed: %s\n\n", purgeSuccessStyle.Render(formatBytes(m.purgedBytes))))
			if m.purgeFailed > 0 {
				boxContent.WriteString(purgeFailStyle.Render(fmt.Sprintf("  %d folder(s) could not be removed.", m.purgeFailed)))
				if m.purgeErr != nil {
					boxContent.WriteString(purgeGrayText(fmt.Sprintf("\n  Last error: %v", m.purgeErr)))
				}
				boxContent.WriteString("\n\n")
			}
		}
		boxContent.WriteString("  Press [q] or [esc] to return to the console.")
	}

	doc.WriteString(purgeBoxStyle.Render(boxContent.String()))
	doc.WriteString("\n")

	switch m.state {
	case stateScanning:
		doc.WriteString(purgeFooterStyle.Render("Analyzing directory tree structures... Please wait."))
	case stateSelecting:
		if len(m.artifacts) == 0 {
			doc.WriteString(purgeFooterStyle.Render("[q] Close and Exit"))
		} else {
			doc.WriteString(purgeFooterStyle.Render("[↑/↓/j/k] Scroll  |  [Space] Select  |  [a] Toggle All  |  [Enter/y] Proceed  |  [q] Quit"))
		}
	case stateConfirming:
		doc.WriteString(purgeFooterStyle.Render("[y] Confirm and Clean  |  [n/esc] Cancel and Go Back"))
	case statePurging:
		doc.WriteString(purgeFooterStyle.Render("Deleting file trees. Do NOT interrupt this operation."))
	case stateFinished:
		doc.WriteString(purgeFooterStyle.Render("[q/esc] Exit to Shell"))
	}

	return doc.String()
}

func countSelected(list []DiscoveredArtifact) int {
	c := 0
	for _, a := range list {
		if a.Selected {
			c++
		}
	}
	return c
}

// Shell-independent secure permanent removal utilizing permissions stripping
func purgePermanentPath(path string, size int64) error {
	if !fs.IsValidPath(path) {
		logPurgeOperation("delete", path, size, false)
		return fmt.Errorf("deleting system protected paths is blocked for safety")
	}

	err := removeAllSafe(path)
	success := err == nil
	logPurgeOperation("delete", path, size, success)
	return err
}

// Shell-independent Recycle Bin removal using hardened native WinAPI
func purgeRecyclePath(path string, size int64) error {
	if !fs.IsValidPath(path) {
		logPurgeOperation("recycle", path, size, false)
		return fmt.Errorf("deleting system protected paths is blocked for safety")
	}

	err := recyclePathNative(path)
	success := err == nil
	logPurgeOperation("recycle", path, size, success)
	return err
}

// logPurgeOperation delegates to the shared structured logging system,
// which also rotates the log; the old local copy grew operations.log unbounded.
func logPurgeOperation(action, target string, size int64, success bool) {
	logging.LogDestructiveOperation("purge", action, target, size, success)
}

// Headless non-interactive execution supporting pipes/snapshots
func runHeadlessPurge(target string) {
	list, err := scanDeveloperArtifactsHeadless(target)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error scanning: %v\n", err)
		os.Exit(1)
	}

	type JSONPurgeOutput struct {
		ScannedPath    string               `json:"scanned_path"`
		TotalFound     int                  `json:"total_found"`
		TotalSizeBytes int64                `json:"total_size_bytes"`
		Artifacts      []DiscoveredArtifact `json:"artifacts"`
	}

	var totalSize int64
	for _, a := range list {
		totalSize += a.Size
	}

	out := JSONPurgeOutput{
		ScannedPath:    target,
		TotalFound:     len(list),
		TotalSizeBytes: totalSize,
		Artifacts:      list,
	}

	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to marshal purge data: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(string(data))
}

func scanDeveloperArtifactsHeadless(root string) ([]DiscoveredArtifact, error) {
	list := []DiscoveredArtifact{} // non-nil so JSON renders [] instead of null
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() {
			return nil
		}
		name := d.Name()
		// Keep this skip list identical to the TUI scan (runScanCmd): the two
		// modes must never disagree about what is eligible for deletion.
		if name == ".git" || name == ".svn" || name == ".vscode" || name == "$Recycle.Bin" || name == "System Volume Information" {
			return filepath.SkipDir
		}
		if !fs.IsValidPath(path) {
			return filepath.SkipDir
		}

		if framework, exists := developerArtifacts[strings.ToLower(name)]; exists {
			if !isLikelyBuildArtifact(strings.ToLower(name), path) {
				return nil // ambiguous folder with no project marker — keep scanning inside
			}
			size := calculateDirSize(path)
			list = append(list, DiscoveredArtifact{
				Path:      path,
				Name:      name,
				Type:      strings.ToLower(name),
				Framework: framework,
				Size:      size,
				Selected:  true,
			})
			return filepath.SkipDir
		}
		return nil
	})
	return list, err
}

// Bulk headless non-interactive deletion if -y/--yes is flagged
func runNonInteractivePurge(target string) {
	list, err := scanDeveloperArtifactsHeadless(target)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error scanning target: %v\n", err)
		os.Exit(1)
	}

	if len(list) == 0 {
		fmt.Println("No developer build artifacts found. Workspace is already clean.")
		return
	}

	var reclaimed int64
	cleaned := 0
	fmt.Printf("Discovered %d developer artifacts under %s. Starting purge...\n\n", len(list), target)

	for _, a := range list {
		fmt.Printf("  Purging %s (%s)... ", a.Path, formatBytes(a.Size))
		var errDelete error
		if !purgeDryRun {
			if purgeSafe {
				errDelete = purgeRecyclePath(a.Path, a.Size)
			} else {
				errDelete = purgePermanentPath(a.Path, a.Size)
			}
		}

		if errDelete == nil {
			reclaimed += a.Size
			cleaned++
			fmt.Println(purgeSuccessStyle.Render("SUCCESS"))
		} else {
			fmt.Printf("%s: %v\n", purgeFailStyle.Render("FAILED"), errDelete)
		}
	}

	fmt.Printf("\n✓ Purged %d / %d directories.\n", cleaned, len(list))
	if purgeDryRun {
		fmt.Printf("Simulated reclaiming of %s.\n", formatBytes(reclaimed))
	} else {
		fmt.Printf("Total active disk space reclaimed: %s\n", formatBytes(reclaimed))
	}
}

// Local visual helper functions for clean string manipulation inside View() loop
func purgeCyanText(s string) string {
	return lipgloss.NewStyle().Foreground(purgeCyanColor).Render(s)
}

func purgeWhiteText(s string) string {
	return lipgloss.NewStyle().Foreground(purgeWhiteColor).Render(s)
}

func purgeGrayText(s string) string {
	return lipgloss.NewStyle().Foreground(purgeGrayColor).Render(s)
}
