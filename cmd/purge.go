package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

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
// full file name. Names absent from this map (.serverless, .sst) are
// unambiguous enough to match on name alone. node_modules, .gradle and .m2 need
// a marker too: without one they are the global npm prefix, the Gradle home or
// the local Maven repository, none of which is disposable build output.
var artifactMarkers = map[string][]string{
	"node_modules": {"package.json"},
	".gradle":      {"build.gradle", "build.gradle.kts", "settings.gradle", "settings.gradle.kts"},
	".m2":          {"pom.xml"},
	"target":       {"Cargo.toml", "pom.xml", "build.sbt"}, // Rust (cargo), Maven, sbt
	"bin":          {".csproj", ".sln", ".vbproj", ".fsproj"},
	"obj":          {".csproj", ".sln", ".vbproj", ".fsproj"},
	"build":        {"package.json", "build.gradle", "build.gradle.kts", "pom.xml", "CMakeLists.txt"},
	"dist":         {"package.json"},
	"vendor":       {"go.mod", "composer.json"},
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
	purgePath      string
	purgeDryRun    bool
	purgeSafe      bool
	purgeJSON      bool
	purgeYes       bool
	purgePermanent bool
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
	PurgeCmd.Flags().BoolVarP(&purgeSafe, "safe", "s", false, "Move folders to the Windows Recycle Bin instead of Duster's 7-day quarantine")
	PurgeCmd.Flags().BoolVar(&purgeJSON, "json", false, "Print the scan as JSON; with --yes also purge and report the outcome")
	PurgeCmd.Flags().BoolVarP(&purgeYes, "yes", "y", false, "Skip interactive prompts and remove all detected build artifacts (restorable for 7 days unless --permanent)")
	PurgeCmd.Flags().BoolVar(&purgePermanent, "permanent", false, "Delete permanently instead of keeping items restorable for 7 days")
}

func executePurge(cmd *cobra.Command, args []string) {
	if purgePermanent && purgeSafe {
		fmt.Fprintln(os.Stderr, "Error: --permanent and --safe cannot be combined.")
		os.Exit(1)
	}

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
	if err := scanTargetError(absPath, true); err != nil {
		fmt.Fprintf(os.Stderr, "Error: cannot scan %s: %v\n", absPath, err)
		os.Exit(1)
	}

	// Explicit --json: a bare --json previews the scan; --json --yes also acts
	// (keeps, recycles or deletes per the flags) and reports the outcome.
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
	purgeFailed  int
	purgeTally   purgeTally
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
	tally purgeTally
	count int
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
		list, _ := scanArtifacts(root, func(a DiscoveredArtifact, count int) {
			ch <- scanProgressMsg{LatestFound: a.Path, Size: a.Size, Count: count}
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

func runPurgeCmd(artifacts []DiscoveredArtifact, ch chan purgeProgressMsg, safe, permanent, dry bool) tea.Cmd {
	return func() tea.Msg {
		var t purgeTally
		count := 0

		var s *quarantineSession
		if !dry {
			s = newQuarantineSession("purge")
			t.sweep = sweepQuarantine(time.Now(), sweepFull)
		}

		for _, a := range artifacts {
			if !a.Selected {
				continue
			}

			count++
			var err error
			if !dry {
				var q bool
				q, err = purgeOne(s, a.Path, a.Size, safe, permanent)
				t.add(a.Path, a.Size, q, permanent, err)
			}

			ch <- purgeProgressMsg{
				Index: count,
				Path:  a.Path,
				Size:  a.Size,
				Err:   err,
			}
		}

		close(ch)
		return purgeCompleteMsg{tally: t, count: count}
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
					runPurgeCmd(m.artifacts, m.purgeChan, purgeSafe, purgePermanent, purgeDryRun),
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
		m.purgeTally = msg.tally
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
	} else if purgePermanent {
		doc.WriteString("  |  " + purgeFailStyle.Render("PERMANENT CLEAN MODE"))
	} else {
		doc.WriteString("  |  " + purgeSuccessStyle.Render("QUARANTINE MODE (7-DAY UNDO)"))
	}
	doc.WriteString("\n")
	doc.WriteString(purgeDividerStyle.Render("  ═══════════════════════════════════════════════════════════════════════") + "\n\n")

	var boxContent strings.Builder

	switch m.state {
	case stateScanning:
		boxContent.WriteString(fmt.Sprintf("🔍  Scanning for developer build artifacts under:\n    %s\n\n", purgeWhiteText(m.rootPath)))
		boxContent.WriteString(fmt.Sprintf("    Discovered targets : %s\n", purgeWhiteText(fmt.Sprintf("%d folders", m.totalFound))))
		boxContent.WriteString(fmt.Sprintf("    Recoverable space  : %s\n\n", purgeWhiteText(formatBytes(m.totalSize))))
		if m.latestFound != "" {
			shortPath := clampTail(m.latestFound, 55)
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
			boxContent.WriteString(purgeDividerStyle.Render("     ───────────────────────────────────────────────────────────────────────") + "\n")

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

				shortPath := clampTail(art.Path, 42)

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
		} else if purgePermanent {
			boxContent.WriteString(fmt.Sprintf("  "+purgeFailStyle.Render("WARNING:")+" This operation will permanently delete %d selected cache directories!\n", countSelected(m.artifacts)))
			boxContent.WriteString(fmt.Sprintf("  Total space to destroy: %s\n", formatBytes(m.selectedSize)))
			boxContent.WriteString("  This action is native, fast, and cannot be undone!\n\n")
		} else {
			boxContent.WriteString(fmt.Sprintf("  This moves %d selected folders to Duster's quarantine (restorable for 7 days with du restore).\n\n", countSelected(m.artifacts)))
		}
		boxContent.WriteString("  Are you absolutely sure you want to proceed? [y to Purge / n to Cancel]")

	case statePurging:
		boxContent.WriteString("🔥  " + purgeFailStyle.Render("PURGING DEVELOPER CACHES & ARTIFACTS") + "\n\n")
		selected := countSelected(m.artifacts)
		boxContent.WriteString(fmt.Sprintf("  Purging progress: %d / %d folders cleaned\n\n", m.currentPurge, selected))

		if m.latestFound != "" {
			shortPath := clampTail(m.latestFound, 55)
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
			for _, line := range m.purgeTally.lines() {
				boxContent.WriteString("  " + line + "\n")
			}
			boxContent.WriteString("\n")
			if w := sweepNotice(m.purgeTally.sweep); w != "" {
				boxContent.WriteString(purgeGrayText("  "+w) + "\n\n")
			}
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

// purgeOne removes one selected artifact per the requested mode:
//   - permanent: purgePermanentPath deletes it for good (it logs "delete").
//   - safe: recycleOrQuarantine sends it to the Recycle Bin, falling back to
//     the quarantine when the bin won't take it.
//   - default: quarantinePath keeps it restorable for quarantineKeep.
//
// It reports whether the item ended up in the quarantine (so the caller can
// tell the user how many items are restorable) and logs the outcome exactly
// once.
func purgeOne(s *quarantineSession, path string, size int64, safe, permanent bool) (quarantined bool, err error) {
	if !fs.IsValidPath(path) {
		action := "quarantine"
		switch {
		case permanent:
			action = "delete"
		case safe:
			action = "recycle"
		}
		logPurgeOperation(action, path, size, false)
		return false, fmt.Errorf("deleting system protected paths is blocked for safety")
	}
	switch {
	case permanent:
		return false, purgePermanentPath(path, size)
	case safe:
		q, err := recycleOrQuarantine(s, path, size)
		action := "recycle"
		if q {
			action = "quarantine"
		}
		logPurgeOperation(action, path, size, err == nil)
		return q, err
	default:
		err := quarantinePath(s, path, size)
		logPurgeOperation("quarantine", path, size, err == nil)
		if errors.Is(err, errNoQuarantine) {
			err = fmt.Errorf("%w; use --permanent to delete it for good", err)
		}
		return err == nil, err
	}
}

// purgeTally is one purge run's outcome by where each item went. Kept items
// are still on disk (keeping is a same-volume rename) until the sweep, so
// they are never counted as freed.
type purgeTally struct {
	freed     int64 // deleted for good (--permanent)
	recycled  int64 // in the Recycle Bin (--safe)
	kept      int64 // in Duster's quarantine
	keptCount int
	failed    int
	errs      []string
	sweep     sweepReport // what the sweep before the run removed, and what it could not
}

func (t *purgeTally) add(path string, size int64, quarantined, permanent bool, err error) {
	switch {
	case err != nil:
		t.failed++
		t.errs = append(t.errs, fmt.Sprintf("%s: %v", path, err))
	case quarantined:
		t.kept += size
		t.keptCount++
	case permanent:
		t.freed += size
	default:
		t.recycled += size
	}
}

// lines is the summary the TUI and du purge --yes print, one line per place
// the items went.
func (t purgeTally) lines() []string {
	var out []string
	if t.freed > 0 {
		out = append(out, fmt.Sprintf("Freed %s (deleted for good).", strings.TrimSpace(formatBytes(t.freed))))
	}
	if t.recycled > 0 {
		out = append(out, fmt.Sprintf("Moved %s to the Recycle Bin (freed when the bin is emptied).", strings.TrimSpace(formatBytes(t.recycled))))
	}
	if t.keptCount > 0 {
		out = append(out,
			fmt.Sprintf("Kept %s for 7 days (freed then, or sooner if the drive runs low; --permanent frees it now).", strings.TrimSpace(formatBytes(t.kept))),
			"du restore lists it, du restore 1 puts it back.")
	}
	return out
}

// sweptLowSpaceJSON is du purge --json --yes's report of the kept sessions the
// sweep before the run removed early for space.
type sweptLowSpaceJSON struct {
	Sessions int      `json:"sessions"`
	Bytes    int64    `json:"bytes"`
	Volumes  []string `json:"volumes"`
	Note     string   `json:"note"`
}

// logPurgeOperation delegates to the shared structured logging system,
// which also rotates the log; the old local copy grew operations.log unbounded.
func logPurgeOperation(action, target string, size int64, success bool) {
	logging.LogDestructiveOperation("purge", action, target, size, success)
}

// runHeadlessPurge previews the scan as JSON. When called with --yes it also
// performs the purge first (one quarantine session, swept before deleting,
// purgeOne per artifact) and reports where the bytes went (kept_bytes is
// still on disk; reclaimed counts only bytes deleted for good) and what
// failed, exiting 1 when any item failed. Without --yes (a bare --json, or
// JSON emitted because output is piped) it only ever previews and never
// deletes.
func runHeadlessPurge(target string) {
	list, err := scanArtifacts(target, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error scanning: %v\n", err)
		os.Exit(1)
	}

	type JSONPurgeOutput struct {
		ScannedPath    string               `json:"scanned_path"`
		TotalFound     int                  `json:"total_found"`
		TotalSizeBytes int64                `json:"total_size_bytes"`
		Artifacts      []DiscoveredArtifact `json:"artifacts"`
		Kept           int                  `json:"kept,omitempty"`
		Undo           string               `json:"undo,omitempty"`
		// With --yes: bytes deleted for good (--permanent), moved to the
		// Recycle Bin (--safe) and kept in the quarantine (still on disk
		// until the sweep), plus what failed. A preview reports zeros.
		Reclaimed     int64    `json:"reclaimed"`
		RecycledBytes int64    `json:"recycled_bytes,omitempty"`
		KeptBytes     int64    `json:"kept_bytes"`
		Failed        int      `json:"failed"`
		Errors        []string `json:"errors"`
		// Kept sessions the sweep before the run removed early because their
		// drive was below 10% free (absent when none).
		SweptLowSpace *sweptLowSpaceJSON `json:"swept_low_space,omitempty"`
	}

	var totalSize int64
	for _, a := range list {
		totalSize += a.Size
	}

	var t purgeTally
	if purgeYes && !purgeDryRun && len(list) > 0 {
		s := newQuarantineSession("purge")
		t.sweep = sweepQuarantine(time.Now(), sweepFull)
		for _, a := range list {
			q, perr := purgeOne(s, a.Path, a.Size, purgeSafe, purgePermanent)
			t.add(a.Path, a.Size, q, purgePermanent, perr)
		}
	}

	out := JSONPurgeOutput{
		ScannedPath:    target,
		TotalFound:     len(list),
		TotalSizeBytes: totalSize,
		Artifacts:      list,
		Kept:           t.keptCount,
		Reclaimed:      t.freed,
		RecycledBytes:  t.recycled,
		KeptBytes:      t.kept,
		Failed:         t.failed,
		Errors:         append([]string{}, t.errs...), // [] rather than null
	}
	if w := sweepWarning(t.sweep.Errs); w != "" {
		// Reported, but a failed sweep does not fail the purge.
		out.Errors = append(out.Errors, w)
	}
	if t.sweep.LowSpace > 0 {
		out.SweptLowSpace = &sweptLowSpaceJSON{Sessions: t.sweep.LowSpace, Bytes: t.sweep.LowSpaceBytes,
			Volumes: t.sweep.LowSpaceVols, Note: t.sweep.lowSpaceLine()}
	}
	if t.keptCount > 0 {
		out.Undo = "du restore 1"
	}

	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to marshal purge data: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(string(data))
	if t.failed > 0 {
		os.Exit(1)
	}
}

// scanArtifacts walks root for purgeable build artifacts, calling onFound (if
// non-nil) with each artifact and the running count. It is the single scanner
// for the TUI and headless paths, so the two can never disagree about what is
// eligible for deletion.
func scanArtifacts(root string, onFound func(DiscoveredArtifact, int)) ([]DiscoveredArtifact, error) {
	list := []DiscoveredArtifact{} // non-nil so JSON renders [] instead of null
	// A long-form root keeps "~" out of every child path, so the per-folder
	// IsValidPath below skips its GetLongPathNameW disk lookup.
	root = fs.LongPath(root)
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() {
			return nil
		}
		name := d.Name()
		switch strings.ToLower(name) {
		case ".git", ".svn", ".vscode", "$recycle.bin", "system volume information":
			return filepath.SkipDir
		case "appdata":
			// Installed apps (Electron resources\app\node_modules, global npm)
			// live here, not workspaces; only scan it if explicitly targeted.
			if path != root {
				return filepath.SkipDir
			}
		}
		if !fs.IsValidPath(path) {
			return filepath.SkipDir
		}
		// Kept items are the user's undo window: a scan of %LOCALAPPDATA% or
		// of X:\.duster-quarantine must never offer them (--permanent would
		// delete them for good).
		if insideQuarantine(path) {
			return filepath.SkipDir
		}

		if framework, exists := developerArtifacts[strings.ToLower(name)]; exists {
			if !isLikelyBuildArtifact(strings.ToLower(name), path) {
				return nil // ambiguous folder with no project marker — keep scanning inside
			}
			a := DiscoveredArtifact{
				Path:      path,
				Name:      name,
				Type:      strings.ToLower(name),
				Framework: framework,
				Size:      calculateDirSize(path),
				Selected:  true,
			}
			list = append(list, a)
			if onFound != nil {
				onFound(a, len(list))
			}
			return filepath.SkipDir
		}
		return nil
	})
	return list, err
}

// Bulk headless non-interactive deletion if -y/--yes is flagged
func runNonInteractivePurge(target string) {
	list, err := scanArtifacts(target, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error scanning target: %v\n", err)
		os.Exit(1)
	}

	if len(list) == 0 {
		fmt.Println("No developer build artifacts found. Workspace is already clean.")
		return
	}

	var reclaimed int64 // dry run: what would be purged
	var t purgeTally
	cleaned := 0
	fmt.Printf("Discovered %d developer artifacts under %s. Starting purge...\n\n", len(list), target)

	var s *quarantineSession
	if !purgeDryRun {
		s = newQuarantineSession("purge")
		if w := sweepNotice(sweepQuarantine(time.Now(), sweepFull)); w != "" {
			fmt.Fprintln(os.Stderr, w)
		}
	}

	for _, a := range list {
		fmt.Printf("  Purging %s (%s)... ", a.Path, formatBytes(a.Size))
		var errDelete error
		if !purgeDryRun {
			var q bool
			q, errDelete = purgeOne(s, a.Path, a.Size, purgeSafe, purgePermanent)
			t.add(a.Path, a.Size, q, purgePermanent, errDelete)
		}

		switch {
		case errDelete != nil:
			fmt.Printf("%s: %v\n", purgeFailStyle.Render("FAILED"), errDelete)
			continue
		case purgeDryRun:
			fmt.Println(purgeSuccessStyle.Render("WOULD PURGE"))
		default:
			fmt.Println(purgeSuccessStyle.Render("SUCCESS"))
		}
		reclaimed += a.Size
		cleaned++
	}

	if purgeDryRun {
		fmt.Printf("\n✓ Dry run: %d / %d directories would be purged; nothing was deleted.\n", cleaned, len(list))
		fmt.Printf("Simulated reclaiming of %s.\n", formatBytes(reclaimed))
	} else {
		fmt.Printf("\n✓ Purged %d / %d directories.\n", cleaned, len(list))
		for _, line := range t.lines() {
			fmt.Println(line)
		}
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
