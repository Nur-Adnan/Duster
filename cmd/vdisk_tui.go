package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Nur-Adnan/duster/lib/elevation"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

var (
	vdiskJSON      bool
	vdiskDryRun    bool
	vdiskAssumeYes bool
)

var (
	vdiskCtx    context.Context
	vdiskCancel context.CancelFunc
)

var (
	vdiskHeaderStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#00FFFF")).Padding(0, 1)
	vdiskBoxStyle    = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("#008080")).
				Padding(1, 2).
				Width(84)
	vdiskFooterStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#666666")).PaddingTop(1).PaddingLeft(2)
	vdiskDividerStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#666666"))
	vdiskSuccessStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#00FF00"))
	vdiskFailStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FF0000"))
	vdiskWarnStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFF00"))
)

var VdiskCmd = &cobra.Command{
	Use:   "vdisk",
	Short: "Shrink WSL and Docker virtual disks",
	Long: `Finds the virtual hard disks (.vhdx) that WSL 2 distributions and Docker Desktop
grow on this machine and compacts them in place with diskpart, returning the space
they took but no longer use. Nothing inside a disk is read, changed or deleted.

Compacting needs administrator rights and stops every running distribution and
container first. Without --yes or the confirmation screen, this only reports.`,
	Run: executeVdisk,
}

func init() {
	VdiskCmd.Flags().BoolVar(&vdiskJSON, "json", false, "Output the discovered virtual disks as JSON and exit immediately")
	VdiskCmd.Flags().BoolVarP(&vdiskDryRun, "dry-run", "d", false, "Report sizes only; never compact anything")
	VdiskCmd.Flags().BoolVarP(&vdiskAssumeYes, "yes", "y", false, "Actually compact in non-interactive (piped/--json) mode")
}

// scanVirtualDisks finds every WSL and Docker disk and measures it. Shared by
// the TUI and the headless path so the two can never report different disks.
func scanVirtualDisks(ctx context.Context) []virtualDisk {
	distros := wslDistroEntries()
	disks := findVirtualDisks(distros, dockerDiskRoots())
	if disks == nil {
		disks = []virtualDisk{} // headless JSON renders [] rather than null
	}
	return measureRunningDistros(ctx, disks, distros)
}

// vdiskAdvice is the reported-only half of this command: the settings that
// would stop the disks growing back. Duster never applies them. Sparse mode in
// particular is gated behind --allow-unsafe in WSL itself, which documents it
// as a potential data-corruption risk, so it stays the user's decision.
func vdiskAdvice(disks []virtualDisk) []string {
	var advice []string
	hasWSL, hasDocker, hasSparse := false, false, false
	for _, d := range disks {
		switch d.Kind {
		case vdiskKindWSL:
			hasWSL = true
		case vdiskKindDocker:
			hasDocker = true
		}
		hasSparse = hasSparse || d.Sparse
	}
	if hasDocker {
		advice = append(advice, "Run `docker system prune -a` first: compacting only returns space the guest has already freed.")
	}
	if hasWSL && !hasSparse {
		advice = append(advice, "To stop disks growing back, add sparseVhd=true under [experimental] in %UserProfile%\\.wslconfig. It applies to newly created disks only.")
	}
	return advice
}

func executeVdisk(cmd *cobra.Command, args []string) {
	vdiskCtx, vdiskCancel = context.WithCancel(context.Background())
	defer vdiskCancel()

	if vdiskJSON || isPiped() {
		runHeadlessVdisk()
		return
	}

	p := tea.NewProgram(initialVdiskModel(), tea.WithAltScreen())
	final, err := p.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error running virtual disk TUI: %v\n", err)
		os.Exit(1)
	}
	// Stopping part-way is safe, but the user should know the disk was put
	// back rather than left attached to Windows.
	if fm, ok := final.(vdiskModel); ok && fm.aborted {
		fmt.Fprintln(os.Stderr, "Compaction was stopped part-way. The disk was detached again, so WSL and Docker will start normally; run `du vdisk` again to finish.")
	}
}

// ─────────────────────────────────────────────
// TUI
// ─────────────────────────────────────────────

type vdiskState int

const (
	vdiskStateScanning vdiskState = iota
	vdiskStateSelecting
	vdiskStateConfirming
	vdiskStateWorking
	vdiskStateFinished
)

type vdiskModel struct {
	state    vdiskState
	disks    []virtualDisk
	cursor   int
	isAdmin  bool
	notice   string
	freed    int64
	failures []string

	// Index of the disk being compacted and when it started: a 100 GB disk
	// can take many minutes, and a screen with no clock looks hung.
	current int
	started time.Time

	// Quitting mid-compaction has to leave the disk detached, so the first q
	// asks and the second one cancels and waits for the worker to clean up.
	quitConfirm bool
	stopping    bool
	aborted     bool

	progress chan vdiskProgressMsg
	width    int
	height   int
}

type vdiskScanMsg struct{ disks []virtualDisk }

type vdiskProgressMsg struct {
	idx   int
	freed int64
	err   error
	start bool
	done  bool
}

type vdiskTickMsg time.Time

func initialVdiskModel() vdiskModel {
	return vdiskModel{
		state:    vdiskStateScanning,
		isAdmin:  elevation.IsAdmin(),
		current:  -1,
		progress: make(chan vdiskProgressMsg, 8),
	}
}

func (m vdiskModel) Init() tea.Cmd {
	return scanVdisksCmd()
}

func scanVdisksCmd() tea.Cmd {
	return func() tea.Msg {
		return vdiskScanMsg{disks: scanVirtualDisks(vdiskContext())}
	}
}

// vdiskContext is the command's cancellable context, or a background one when
// a task runs outside the command (tests).
func vdiskContext() context.Context {
	if vdiskCtx != nil {
		return vdiskCtx
	}
	return context.Background()
}

func listenVdiskProgress(ch chan vdiskProgressMsg) tea.Cmd {
	return func() tea.Msg { return <-ch }
}

func vdiskTickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg { return vdiskTickMsg(t) })
}

// runCompactCmd compacts the selected disks one at a time. WSL is stopped once
// for the whole batch rather than per disk, because every shutdown costs the
// user their running containers and shells.
func runCompactCmd(disks []virtualDisk, dry bool, ch chan vdiskProgressMsg) tea.Cmd {
	return func() tea.Msg {
		go func() {
			if !dry {
				// A failed shutdown is a warning, not a stop. WSL reports an
				// error on a machine that has the feature but no distribution
				// installed, which is exactly the Docker-only case this
				// command is for. If something really does hold a disk open,
				// diskpart says so for that disk, which is the better
				// diagnosis anyway.
				if err := shutdownWSL(vdiskContext()); err != nil {
					ch <- vdiskProgressMsg{idx: -1, err: err}
				}
			}
			for i, disk := range disks {
				if !disk.selected {
					continue
				}
				// Stopping means stopping: without this, every remaining
				// disk would fail instantly on the cancelled context and
				// fill the summary with noise.
				if vdiskContext().Err() != nil {
					break
				}
				ch <- vdiskProgressMsg{idx: i, start: true}
				if dry {
					estimate, _ := disk.recoverable()
					ch <- vdiskProgressMsg{idx: i, freed: estimate}
					continue
				}
				freed, err := compactVirtualDisk(vdiskContext(), disk)
				ch <- vdiskProgressMsg{idx: i, freed: freed, err: err}
			}
			ch <- vdiskProgressMsg{idx: -1, done: true}
		}()
		return <-ch
	}
}

func (m vdiskModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height

	case vdiskScanMsg:
		m.disks = msg.disks
		m.state = vdiskStateSelecting
		// Everything that can actually be compacted starts selected: the
		// command has one job and the blocked disks are the exception.
		for i := range m.disks {
			m.disks[i].selected = m.disks[i].Blocked == ""
		}

	case vdiskProgressMsg:
		return m.applyProgress(msg)

	case vdiskTickMsg:
		if m.state == vdiskStateWorking {
			return m, vdiskTickCmd()
		}

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m vdiskModel) applyProgress(msg vdiskProgressMsg) (tea.Model, tea.Cmd) {
	if msg.done {
		m.state = vdiskStateFinished
		m.aborted = m.stopping
		if msg.err != nil {
			m.failures = append(m.failures, msg.err.Error())
		}
		return m, nil
	}

	if msg.start {
		m.current = msg.idx
		m.started = time.Now()
		return m, listenVdiskProgress(m.progress)
	}

	m.freed += msg.freed
	if msg.err != nil {
		if msg.idx >= 0 && msg.idx < len(m.disks) {
			m.failures = append(m.failures, m.disks[msg.idx].Label+": "+msg.err.Error())
		} else {
			m.failures = append(m.failures, msg.err.Error())
		}
	}
	return m, listenVdiskProgress(m.progress)
}

func (m vdiskModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	// While diskpart is running, leaving means cancelling and then waiting
	// for the disk to be detached again. Nothing else on the keyboard applies.
	if m.state == vdiskStateWorking {
		switch key {
		case "q", "ctrl+c", "esc":
			if m.stopping {
				return m, nil
			}
			if !m.quitConfirm {
				m.quitConfirm = true
				return m, nil
			}
			m.stopping = true
			if vdiskCancel != nil {
				vdiskCancel()
			}
			return m, nil
		default:
			m.quitConfirm = false
			return m, nil
		}
	}

	switch key {
	case "ctrl+c":
		return m, tea.Quit

	case "q", "esc":
		if m.state == vdiskStateConfirming {
			m.state = vdiskStateSelecting
			return m, nil
		}
		return m, tea.Quit

	case "n", "N":
		if m.state == vdiskStateConfirming {
			m.state = vdiskStateSelecting
		}

	case "up", "k":
		if m.state == vdiskStateSelecting && len(m.disks) > 0 {
			m.cursor = (m.cursor - 1 + len(m.disks)) % len(m.disks)
		}

	case "down", "j":
		if m.state == vdiskStateSelecting && len(m.disks) > 0 {
			m.cursor = (m.cursor + 1) % len(m.disks)
		}

	case " ":
		if m.state == vdiskStateSelecting && m.cursor < len(m.disks) {
			// A disk diskpart would refuse cannot be selected: offering it
			// would only buy the user a long wait and an error.
			if m.disks[m.cursor].Blocked != "" {
				m.notice = m.disks[m.cursor].Blocked
				return m, nil
			}
			m.disks[m.cursor].selected = !m.disks[m.cursor].selected
			m.notice = ""
		}

	case "a", "A":
		if m.state == vdiskStateSelecting {
			target := !m.allSelectableChosen()
			for i := range m.disks {
				if m.disks[i].Blocked == "" {
					m.disks[i].selected = target
				}
			}
		}

	case "enter":
		return m.confirmOrStart()

	case "y", "Y":
		if m.state == vdiskStateConfirming {
			m.state = vdiskStateWorking
			m.current = 0
			m.started = time.Now()
			return m, tea.Batch(runCompactCmd(m.disks, vdiskDryRun, m.progress), vdiskTickCmd())
		}
	}
	return m, nil
}

func (m vdiskModel) confirmOrStart() (tea.Model, tea.Cmd) {
	if m.state == vdiskStateFinished {
		return m, tea.Quit
	}
	if m.state != vdiskStateSelecting {
		return m, nil
	}
	if m.countSelected() == 0 {
		m.notice = "Nothing selected."
		return m, nil
	}
	if !m.isAdmin && !vdiskDryRun {
		m.notice = "Compacting needs administrator rights. Reopen this terminal as Administrator, or run `du vdisk --dry-run` to report only."
		return m, nil
	}
	m.state = vdiskStateConfirming
	m.notice = ""
	return m, nil
}

func (m vdiskModel) countSelected() int {
	n := 0
	for _, d := range m.disks {
		if d.selected {
			n++
		}
	}
	return n
}

func (m vdiskModel) selectedBytes() int64 {
	var total int64
	for _, d := range m.disks {
		if d.selected {
			total += d.OnDiskBytes
		}
	}
	return total
}

func (m vdiskModel) allSelectableChosen() bool {
	any := false
	for _, d := range m.disks {
		if d.Blocked != "" {
			continue
		}
		any = true
		if !d.selected {
			return false
		}
	}
	return any
}

func (m vdiskModel) View() string {
	var doc strings.Builder

	doc.WriteString("\n")
	doc.WriteString(vdiskHeaderStyle.Render("Duster Virtual Disk Compactor"))
	if vdiskDryRun {
		doc.WriteString("  |  " + vdiskWarnStyle.Render("DRY RUN (REPORT ONLY)"))
	} else if !m.isAdmin {
		doc.WriteString("  |  " + vdiskWarnStyle.Render("NOT ELEVATED"))
	} else {
		doc.WriteString("  |  " + vdiskSuccessStyle.Render("LIVE ACTIVE MODE"))
	}
	doc.WriteString("\n")
	doc.WriteString(vdiskDividerStyle.Render("  ═══════════════════════════════════════════════════════════════════════"))
	doc.WriteString("\n\n")

	var body string
	switch m.state {
	case vdiskStateScanning:
		body = vdiskBoxStyle.Render("🔍  Looking for WSL and Docker virtual disks...\n\n    Reading registered distributions and Docker Desktop's disk folders.")
	case vdiskStateSelecting:
		body = vdiskBoxStyle.Render(m.selectView())
	case vdiskStateConfirming:
		body = vdiskBoxStyle.Render(m.confirmView())
	case vdiskStateWorking:
		body = vdiskBoxStyle.Render(m.workingView())
	case vdiskStateFinished:
		body = vdiskBoxStyle.Render(m.finishedView())
	}
	doc.WriteString(body)
	doc.WriteString("\n")
	doc.WriteString(vdiskFooterStyle.Render(m.footer()))

	return doc.String()
}

func (m vdiskModel) selectView() string {
	var b strings.Builder
	if len(m.disks) == 0 {
		b.WriteString("✓  No WSL or Docker virtual disks found on this machine.\n\n")
		b.WriteString(grayText("Nothing to compact. This command only applies once WSL 2 or"))
		b.WriteString("\n")
		b.WriteString(grayText("Docker Desktop has created a .vhdx disk."))
		return b.String()
	}

	b.WriteString(whiteText("Virtual disks found on this machine:"))
	b.WriteString("\n\n")
	b.WriteString(grayText("     Disk                              On disk        Used     Recover"))
	b.WriteString("\n")
	b.WriteString(vdiskDividerStyle.Render("     ───────────────────────────────────────────────────────────────────"))
	b.WriteString("\n")

	for i, d := range m.disks {
		check := "[ ]"
		if d.selected {
			check = "[x]"
		}
		if d.Blocked != "" {
			check = " - "
		}
		prefix := "  "
		if i == m.cursor {
			prefix = "▸ "
		}

		used, recover := "      ?", "      ?"
		if d.UsedKnown {
			used = fmt.Sprintf("%9s", formatBytes(d.UsedBytes))
		}
		if est, ok := d.recoverable(); ok {
			recover = fmt.Sprintf("%9s", formatBytes(est))
		}

		line := fmt.Sprintf("%s%s  %-30s %10s %9s %9s",
			prefix, check, clampHead(d.Label, 30), formatBytes(d.OnDiskBytes), used, recover)
		if i == m.cursor {
			b.WriteString(whiteText(line))
		} else {
			b.WriteString(line)
		}
		b.WriteString("\n")
		if d.Blocked != "" {
			b.WriteString(grayText("        " + clampHead(d.Blocked, 64)))
			b.WriteString("\n")
		}
	}

	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("  Selected: %d disk(s), %s on disk",
		m.countSelected(), vdiskSuccessStyle.Render(formatBytes(m.selectedBytes()))))
	b.WriteString("\n")
	b.WriteString(grayText("  \"Used\" is only known for a distribution that was already running."))

	for _, tip := range vdiskAdvice(m.disks) {
		b.WriteString("\n")
		b.WriteString(grayText("  • " + clampHead(tip, 72)))
	}
	if m.notice != "" {
		b.WriteString("\n\n")
		b.WriteString(vdiskWarnStyle.Render("  " + clampHead(m.notice, 74)))
	}
	return b.String()
}

func (m vdiskModel) confirmView() string {
	var b strings.Builder
	b.WriteString("⚠️  " + vdiskFailStyle.Render("THIS STOPS YOUR RUNNING DISTROS AND CONTAINERS"))
	b.WriteString("\n\n")
	b.WriteString(fmt.Sprintf("  About to compact %d disk(s) holding %s.\n\n",
		m.countSelected(), formatBytes(m.selectedBytes())))
	b.WriteString("  Duster will run `wsl --shutdown` first. Every running distribution,\n")
	b.WriteString("  shell and container stops immediately, and unsaved work in them is\n")
	b.WriteString("  lost. Quit Docker Desktop too, or its disk stays locked and is\n")
	b.WriteString("  skipped.\n\n")
	b.WriteString(grayText("  Nothing inside a disk is read, changed or deleted: only the space"))
	b.WriteString("\n")
	b.WriteString(grayText("  the container no longer uses is handed back to Windows."))
	b.WriteString("\n")
	// Each disk is attached read-only while Windows scans it, and Windows
	// cannot read ext4, so Explorer sometimes offers to format it. The
	// read-only attach makes that harmless, but it is alarming unannounced.
	b.WriteString(grayText("  Windows may offer to format a disk while this runs. Say no; it is"))
	b.WriteString("\n")
	b.WriteString(grayText("  attached read-only, so nothing could be written to it anyway."))
	b.WriteString("\n\n")
	if vdiskDryRun {
		b.WriteString(vdiskWarnStyle.Render("  Dry run: nothing will be stopped or changed."))
		b.WriteString("\n\n")
	}
	b.WriteString("  Proceed? [y to compact / n to go back]")
	return b.String()
}

func (m vdiskModel) workingView() string {
	var b strings.Builder
	b.WriteString("⚙️  " + vdiskWarnStyle.Render("COMPACTING VIRTUAL DISKS"))
	b.WriteString("\n\n")

	if m.current >= 0 && m.current < len(m.disks) {
		b.WriteString(fmt.Sprintf("  Working on : %s\n", whiteText(m.disks[m.current].Label)))
		b.WriteString(fmt.Sprintf("  Elapsed    : %s\n\n", formatElapsed(time.Since(m.started))))
	}
	b.WriteString(fmt.Sprintf("  Reclaimed so far: %s\n\n", vdiskSuccessStyle.Render(formatBytes(m.freed))))
	b.WriteString(grayText("  A large disk can take many minutes. diskpart is rewriting the"))
	b.WriteString("\n")
	b.WriteString(grayText("  container; the file system inside it is not being touched."))

	switch {
	case m.stopping:
		b.WriteString("\n\n")
		b.WriteString(vdiskWarnStyle.Render("  Stopping: detaching the disk before exiting. Please wait."))
	case m.quitConfirm:
		b.WriteString("\n\n")
		b.WriteString(vdiskWarnStyle.Render("  Press q again to stop. The disk will be detached first."))
	}
	return b.String()
}

func (m vdiskModel) finishedView() string {
	var b strings.Builder
	if len(m.failures) == 0 {
		b.WriteString("✓  " + vdiskSuccessStyle.Render("COMPACTION COMPLETE"))
	} else {
		b.WriteString("!  " + vdiskWarnStyle.Render("COMPACTION FINISHED WITH PROBLEMS"))
	}
	b.WriteString("\n\n")

	if vdiskDryRun {
		b.WriteString(fmt.Sprintf("  Estimated recoverable : %s\n", vdiskSuccessStyle.Render(formatBytes(m.freed))))
		b.WriteString(grayText("  Dry run: nothing was stopped or changed."))
		b.WriteString("\n")
	} else {
		b.WriteString(fmt.Sprintf("  Space returned : %s\n", vdiskSuccessStyle.Render(formatBytes(m.freed))))
	}
	if m.aborted {
		b.WriteString("\n")
		b.WriteString(vdiskWarnStyle.Render("  Stopped early. The disk was detached, so WSL starts normally."))
		b.WriteString("\n")
	}
	for _, f := range m.failures {
		b.WriteString("\n")
		b.WriteString(vdiskFailStyle.Render("  " + clampHead(f, 74)))
	}
	return b.String()
}

func (m vdiskModel) footer() string {
	switch m.state {
	case vdiskStateScanning:
		return "Scanning... Please wait."
	case vdiskStateSelecting:
		if len(m.disks) == 0 {
			return "[q] Exit"
		}
		return "[↑/↓/j/k] Move  |  [Space] Select  |  [a] Toggle All  |  [Enter] Continue  |  [q] Quit"
	case vdiskStateConfirming:
		return "[y] Compact  |  [n/esc] Go back"
	case vdiskStateWorking:
		if m.stopping {
			return "Stopping safely..."
		}
		return "[q] Stop (asks twice)"
	default:
		return "[q/esc] Exit to shell"
	}
}

// ─────────────────────────────────────────────
// Headless
// ─────────────────────────────────────────────

func runHeadlessVdisk() {
	isAdmin := elevation.IsAdmin()
	disks := scanVirtualDisks(vdiskContext())

	// A non-interactive run has no confirmation screen, and compacting stops
	// every running container, so it takes an explicit --yes.
	previewOnly := vdiskDryRun || !vdiskAssumeYes

	var freed int64
	var warning string
	results := make([]vdiskResult, 0, len(disks))
	failed := false

	if !previewOnly && len(disks) > 0 {
		if !isAdmin {
			fmt.Fprintln(os.Stderr, "Compacting virtual disks needs administrator rights.")
			os.Exit(1)
		}
		// Not fatal: see the note in runCompactCmd. A disk that really is
		// held open fails on its own with diskpart's own explanation.
		if err := shutdownWSL(vdiskContext()); err != nil {
			warning = err.Error()
		}
	}

	for _, disk := range disks {
		res := vdiskResult{Path: disk.Path, Label: disk.Label}
		switch {
		case disk.Blocked != "":
			res.Status = "skipped"
			res.Note = disk.Blocked
		case previewOnly:
			res.Status = "reported"
			if est, ok := disk.recoverable(); ok {
				res.EstimatedBytes = est
			}
		default:
			got, err := compactVirtualDisk(vdiskContext(), disk)
			if err != nil {
				res.Status = "failed"
				res.Note = err.Error()
				failed = true
			} else {
				res.Status = "compacted"
				res.FreedBytes = got
				freed += got
			}
		}
		results = append(results, res)
	}

	payload := struct {
		Disks         []virtualDisk `json:"disks"`
		Results       []vdiskResult `json:"results"`
		TotalFreed    int64         `json:"total_freed_bytes"`
		PreviewOnly   bool          `json:"preview_only"`
		AdminElevated bool          `json:"admin_elevated"`
		Warning       string        `json:"warning,omitempty"`
		Advice        []string      `json:"advice,omitempty"`
		Timestamp     string        `json:"timestamp"`
	}{
		Disks:         disks,
		Results:       results,
		TotalFreed:    freed,
		PreviewOnly:   previewOnly,
		AdminElevated: isAdmin,
		Warning:       warning,
		Advice:        vdiskAdvice(disks),
		Timestamp:     time.Now().UTC().Format(time.RFC3339),
	}

	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error encoding JSON: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(string(data))
	if failed {
		os.Exit(1)
	}
}

// vdiskResult is what happened to one disk in a headless run.
type vdiskResult struct {
	Path           string `json:"path"`
	Label          string `json:"label"`
	Status         string `json:"status"`
	FreedBytes     int64  `json:"freed_bytes,omitempty"`
	EstimatedBytes int64  `json:"estimated_recoverable_bytes,omitempty"`
	Note           string `json:"note,omitempty"`
}
