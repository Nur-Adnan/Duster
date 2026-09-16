package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// writeSizedFile creates a file of exactly size bytes and returns its path.
func writeSizedFile(t *testing.T, dir, name string, size int64) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, make([]byte, size), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

func TestParseDfUsedBytes(t *testing.T) {
	tests := []struct {
		name string
		out  string
		want int64
		ok   bool
	}{
		{
			name: "Standard df output",
			out: "Filesystem     1K-blocks    Used Available Use% Mounted on\n" +
				"/dev/sdc      1055762868 6285452 995725472   1% /\n",
			want: 6285452 * 1024,
			ok:   true,
		},
		{
			// df wraps a long device name onto its own line, which is why the
			// mount point rather than a line number selects the row.
			name: "Wrapped device name",
			out: "Filesystem     1K-blocks    Used Available Use% Mounted on\n" +
				"/dev/mapper/a-very-long-device-name-here\n" +
				"               1055762868 4194304 995725472   1% /\n",
			want: 4194304 * 1024,
			ok:   true,
		},
		{
			name: "Only other mount points",
			out: "Filesystem 1K-blocks Used Available Use% Mounted on\n" +
				"drvfs      100 50 50  50% /mnt/c\n",
			ok: false,
		},
		{name: "Empty output", out: "", ok: false},
		{
			name: "Non-numeric used column",
			out: "Filesystem 1K-blocks Used Available Use% Mounted on\n" +
				"/dev/sdc   100       n/a  50        50% /\n",
			ok: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := parseDfUsedBytes(tt.out)
			if ok != tt.ok {
				t.Fatalf("parseDfUsedBytes ok = %v, want %v", ok, tt.ok)
			}
			if ok && got != tt.want {
				t.Errorf("parseDfUsedBytes = %d, want %d", got, tt.want)
			}
		})
	}
}

// wsl.exe writes UTF-16LE with no BOM when redirected, so a plain string cast
// produces NUL-separated characters that match no distribution name.
func TestDecodeWSLOutput(t *testing.T) {
	utf16le := []byte{'U', 0, 'b', 0, 'u', 0, 'n', 0, 't', 0, 'u', 0, '\r', 0, '\n', 0}
	if got := decodeWSLOutput(utf16le); got != "Ubuntu\r\n" {
		t.Errorf("UTF-16LE decode = %q, want %q", got, "Ubuntu\r\n")
	}
	if got := decodeWSLOutput([]byte("Ubuntu\n")); got != "Ubuntu\n" {
		t.Errorf("UTF-8 passthrough = %q, want %q", got, "Ubuntu\n")
	}
	if got := decodeWSLOutput(nil); got != "" {
		t.Errorf("nil decode = %q, want empty", got)
	}
}

func TestFindVirtualDisks(t *testing.T) {
	distroDir := t.TempDir()
	dockerRoot := t.TempDir()

	writeSizedFile(t, distroDir, "ext4.vhdx", 4096)
	// WSL rebuilds the swap disk on every boot, so there is nothing in it to
	// reclaim and it must never be offered.
	writeSizedFile(t, distroDir, "swap.vhdx", 8192)
	writeSizedFile(t, distroDir, "notes.txt", 10)
	// Docker has used more than one layout, so the scan looks one level down.
	writeSizedFile(t, dockerRoot, filepath.Join("disk", "docker_data.vhdx"), 16384)

	disks := findVirtualDisks(
		[]wslDistro{{Name: "Ubuntu", BasePath: distroDir, Version: 2}},
		[]string{dockerRoot},
	)

	if len(disks) != 2 {
		t.Fatalf("found %d disks, want 2: %+v", len(disks), disks)
	}
	// Biggest first: the point of the command is finding where space went.
	if disks[0].Kind != vdiskKindDocker || disks[0].OnDiskBytes != 16384 {
		t.Errorf("first disk = %+v, want the 16 KB Docker disk", disks[0])
	}
	if disks[1].Kind != vdiskKindWSL || disks[1].Label != "Ubuntu" {
		t.Errorf("second disk = %+v, want the Ubuntu disk", disks[1])
	}
	for _, d := range disks {
		if strings.EqualFold(filepath.Base(d.Path), "swap.vhdx") {
			t.Error("the WSL swap disk must not be offered for compaction")
		}
	}
}

func TestFindVirtualDisksDeduplicates(t *testing.T) {
	dir := t.TempDir()
	writeSizedFile(t, dir, "ext4.vhdx", 2048)

	// Docker Desktop registers its own WSL distributions, so the same file is
	// reachable from both sources.
	disks := findVirtualDisks([]wslDistro{{Name: "docker-desktop", BasePath: dir}}, []string{dir})
	if len(disks) != 1 {
		t.Fatalf("found %d disks, want 1 after deduplication", len(disks))
	}
	if disks[0].Kind != vdiskKindWSL {
		t.Errorf("kind = %q, want the first source to win", disks[0].Kind)
	}
}

func TestIsCompactableVhdx(t *testing.T) {
	dir := t.TempDir()
	real := writeSizedFile(t, dir, "ext4.vhdx", 16)
	writeSizedFile(t, dir, "image.iso", 16)

	if !isCompactableVhdx(real) {
		t.Error("a real .vhdx file should be compactable")
	}
	if isCompactableVhdx(filepath.Join(dir, "image.iso")) {
		t.Error("a non-.vhdx file must be refused")
	}
	if isCompactableVhdx(filepath.Join(dir, "missing.vhdx")) {
		t.Error("a missing file must be refused")
	}
	if err := os.Mkdir(filepath.Join(dir, "dir.vhdx"), 0o755); err == nil {
		if isCompactableVhdx(filepath.Join(dir, "dir.vhdx")) {
			t.Error("a directory named like a disk must be refused")
		}
	}

	// A link at the target is refused outright rather than followed, the same
	// rule the deletion paths use.
	link := filepath.Join(dir, "link.vhdx")
	if err := os.Symlink(real, link); err != nil {
		t.Skip("symlinks unsupported here")
	}
	if isCompactableVhdx(link) {
		t.Error("a symlink must never be handed to diskpart")
	}
}

func TestCompactionBlocker(t *testing.T) {
	tests := []struct {
		name string
		disk virtualDisk
		want bool
	}{
		{"Plain disk", virtualDisk{}, false},
		{"Sparse", virtualDisk{Sparse: true}, true},
		{"Compressed", virtualDisk{Compressed: true}, true},
		{"Encrypted", virtualDisk{Encrypted: true}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := compactionBlocker(tt.disk) != ""; got != tt.want {
				t.Errorf("blocked = %v, want %v", got, tt.want)
			}
		})
	}
}

// A blocked disk must never reach diskpart: the run would take minutes and
// then fail with "must not be sparse".
func TestCompactRefusesBlockedDisk(t *testing.T) {
	dir := t.TempDir()
	path := writeSizedFile(t, dir, "ext4.vhdx", 32)

	_, err := compactVirtualDisk(t.Context(), virtualDisk{Path: path, Blocked: "sparse disk"})
	if err == nil {
		t.Fatal("compacting a blocked disk must fail before running diskpart")
	}
	if !strings.Contains(err.Error(), "sparse") {
		t.Errorf("error = %q, want it to name the blocker", err)
	}
}

func TestDiskpartError(t *testing.T) {
	tests := []struct {
		name string
		out  string
		want string
	}{
		{
			name: "Clean run",
			out: "Microsoft DiskPart version 10.0.26100.1\n\nDiskPart successfully selected the virtual disk file.\n" +
				"  100 percent completed\nDiskPart successfully compacted the virtual disk file.\n",
		},
		{
			name: "Sparse disk rejected",
			out: "Microsoft DiskPart version 10.0.26100.1\n\nDiskPart has encountered an error: The parameter is incorrect.\n" +
				"Virtual hard disk files must be uncompressed and unencrypted and must not be sparse.\n",
			want: "must not be sparse",
		},
		{
			name: "Missing file",
			out:  "Microsoft DiskPart version 10.0.26100.1\n\nThe system cannot find the file specified.\n",
			want: "cannot find the file",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := diskpartError(tt.out)
			if tt.want == "" {
				if got != "" {
					t.Errorf("diskpartError = %q, want no error", got)
				}
				return
			}
			if !strings.Contains(got, tt.want) {
				t.Errorf("diskpartError = %q, want it to contain %q", got, tt.want)
			}
		})
	}
}

// A diskpart script is read in the system ANSI code page, so a UTF-8 path with
// non-ASCII bytes would not name the same file. Refusing beats compacting
// whatever the mangled path happened to hit.
func TestDiskpartTargetRefusesUnspellablePath(t *testing.T) {
	ascii := `C:\Users\dev\AppData\Local\wsl\ext4.vhdx`
	got, err := diskpartTarget(ascii)
	if err != nil || got != ascii {
		t.Fatalf("diskpartTarget(%q) = %q, %v; want the path unchanged", ascii, got, err)
	}

	// The short name lookup fails for a path that does not exist, on every
	// platform, so there is no safe spelling to fall back to.
	if _, err := diskpartTarget(`C:\Users\Пользователь\ext4.vhdx`); err == nil {
		t.Error("a path that cannot be written to a script must be refused")
	}
}

func TestWriteDiskpartScript(t *testing.T) {
	path, err := writeDiskpartScript([]string{`select vdisk file="C:\a.vhdx"`, "compact vdisk"})
	if err != nil {
		t.Fatalf("writeDiskpartScript: %v", err)
	}
	defer os.Remove(path)

	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read script: %v", err)
	}
	text := string(body)
	if !strings.HasSuffix(text, "exit\r\n") {
		t.Errorf("script = %q, want it to end with a CRLF-terminated exit", text)
	}
	if !strings.Contains(text, `select vdisk file="C:\a.vhdx"`) {
		t.Errorf("script = %q, want the select line verbatim", text)
	}
}

func TestRecoverable(t *testing.T) {
	tests := []struct {
		name string
		disk virtualDisk
		want int64
		ok   bool
	}{
		{"Guest not measured", virtualDisk{OnDiskBytes: 100}, 0, false},
		{"Measured", virtualDisk{OnDiskBytes: 100, UsedBytes: 30, UsedKnown: true}, 70, true},
		{"Guest reports more than the container", virtualDisk{OnDiskBytes: 30, UsedBytes: 100, UsedKnown: true}, 0, true},
		{"Blocked disks promise nothing", virtualDisk{OnDiskBytes: 100, UsedBytes: 30, UsedKnown: true, Blocked: "sparse"}, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := tt.disk.recoverable()
			if ok != tt.ok || got != tt.want {
				t.Errorf("recoverable() = %d, %v; want %d, %v", got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestVdiskAdvice(t *testing.T) {
	none := vdiskAdvice(nil)
	if len(none) != 0 {
		t.Errorf("advice with no disks = %v, want none", none)
	}

	advice := strings.Join(vdiskAdvice([]virtualDisk{
		{Kind: vdiskKindWSL}, {Kind: vdiskKindDocker},
	}), "\n")
	if !strings.Contains(advice, "docker system prune") {
		t.Error("a Docker disk should suggest pruning first")
	}
	if !strings.Contains(advice, "sparseVhd") {
		t.Error("a non-sparse WSL disk should mention the sparseVhd setting")
	}

	// Already sparse: Windows reclaims it, so there is nothing to suggest.
	sparse := strings.Join(vdiskAdvice([]virtualDisk{{Kind: vdiskKindWSL, Sparse: true}}), "\n")
	if strings.Contains(sparse, "sparseVhd") {
		t.Errorf("advice for an already-sparse disk = %q, want no sparseVhd tip", sparse)
	}
}

// selectingModel is a model parked on the list screen with two disks.
func selectingModel() vdiskModel {
	return vdiskModel{
		state:   vdiskStateSelecting,
		isAdmin: true,
		current: -1,
		disks: []virtualDisk{
			{Kind: vdiskKindWSL, Label: "Ubuntu", Path: `C:\a\ext4.vhdx`, OnDiskBytes: 100, selected: true},
			{Kind: vdiskKindWSL, Label: "Alpine", Path: `C:\b\ext4.vhdx`, OnDiskBytes: 50, Blocked: "sparse disk"},
		},
		progress: make(chan vdiskProgressMsg, 4),
	}
}

func TestBlockedDiskCannotBeSelected(t *testing.T) {
	m := selectingModel()
	m.cursor = 1

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	got := next.(vdiskModel)
	if got.disks[1].selected {
		t.Error("a disk diskpart would refuse must not become selected")
	}
	if got.notice == "" {
		t.Error("the screen should say why the disk cannot be chosen")
	}

	// Toggle-all must respect the same rule.
	all, _ := got.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	if all.(vdiskModel).disks[1].selected {
		t.Error("toggle-all must skip blocked disks")
	}
}

func TestCompactingNeedsAdmin(t *testing.T) {
	m := selectingModel()
	m.isAdmin = false

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got := next.(vdiskModel)
	if got.state != vdiskStateSelecting {
		t.Fatalf("state = %v, want to stay on the list without admin rights", got.state)
	}
	if !strings.Contains(got.notice, "administrator") {
		t.Errorf("notice = %q, want it to explain the missing rights", got.notice)
	}

	m.isAdmin = true
	elevated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if elevated.(vdiskModel).state != vdiskStateConfirming {
		t.Error("an elevated run should reach the confirmation screen")
	}
}

// Killing diskpart mid-compaction would leave the disk attached to Windows and
// WSL unable to start it, so leaving takes two keypresses and then waits for
// the worker to detach.
func TestQuitDuringCompactionNeedsConfirmation(t *testing.T) {
	m := selectingModel()
	m.state = vdiskStateWorking

	first, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	asking := first.(vdiskModel)
	if !asking.quitConfirm {
		t.Fatal("the first q should ask rather than stop")
	}
	if isQuitCmd(cmd) {
		t.Fatal("the first q must not quit the program")
	}

	second, cmd := asking.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	stopping := second.(vdiskModel)
	if !stopping.stopping {
		t.Fatal("the second q should start stopping")
	}
	if isQuitCmd(cmd) {
		t.Fatal("the program must stay up until the disk is detached")
	}

	// Only the worker's final message ends the screen, and it reports that
	// the run was cut short.
	done, _ := stopping.Update(vdiskProgressMsg{idx: -1, done: true})
	finished := done.(vdiskModel)
	if finished.state != vdiskStateFinished || !finished.aborted {
		t.Errorf("state = %v, aborted = %v; want a finished, aborted run", finished.state, finished.aborted)
	}
}

func TestQuitConfirmationCanBeCancelled(t *testing.T) {
	m := selectingModel()
	m.state = vdiskStateWorking

	asking, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	cancelled, _ := asking.(vdiskModel).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	got := cancelled.(vdiskModel)
	if got.quitConfirm || got.stopping {
		t.Error("any other key should cancel the quit prompt")
	}
}

func TestVdiskProgressAccumulatesAndReportsFailures(t *testing.T) {
	m := selectingModel()
	m.state = vdiskStateWorking

	started, _ := m.Update(vdiskProgressMsg{idx: 0, start: true})
	if started.(vdiskModel).current != 0 {
		t.Error("a start message should name the disk being worked on")
	}

	freed, _ := started.(vdiskModel).Update(vdiskProgressMsg{idx: 0, freed: 4096})
	if got := freed.(vdiskModel).freed; got != 4096 {
		t.Errorf("freed = %d, want 4096", got)
	}

	failedMsg := vdiskProgressMsg{idx: 0, err: os.ErrPermission}
	failed, _ := freed.(vdiskModel).Update(failedMsg)
	if len(failed.(vdiskModel).failures) != 1 {
		t.Error("a failure should be kept and shown on the summary screen")
	}
}

func TestVdiskViewsStartAtTheLeftMargin(t *testing.T) {
	base := selectingModel()
	states := map[string]vdiskState{
		"scanning":   vdiskStateScanning,
		"selecting":  vdiskStateSelecting,
		"confirming": vdiskStateConfirming,
		"working":    vdiskStateWorking,
		"finished":   vdiskStateFinished,
	}
	for name, state := range states {
		m := base
		m.state = state
		m.current = 0
		m.failures = []string{"Ubuntu: diskpart failed"}
		for i, line := range strings.Split(sgr.ReplaceAllString(m.View(), ""), "\n") {
			text := strings.TrimLeft(line, " ")
			if indent := len(line) - len(text); text != "" && indent > 20 {
				t.Errorf("%s: line %d starts at column %d: %q", name, i, indent, text)
			}
		}
	}
}

// A dry run must not touch a disk, stop anything, or record a compaction in
// the audit trail: it only reports what a real run could return.
func TestVdiskDryRunChangesNothing(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DU_NO_OPLOG", "")
	t.Setenv("LOCALAPPDATA", dir)
	path := writeSizedFile(t, dir, "ext4.vhdx", 4096)

	disks := []virtualDisk{{
		Path: path, Label: "Ubuntu", Kind: vdiskKindWSL,
		OnDiskBytes: 4096, UsedBytes: 1024, UsedKnown: true, selected: true,
	}}

	ch := make(chan vdiskProgressMsg, 8)
	msg, ok := runCompactCmd(disks, true, ch)().(vdiskProgressMsg)
	if !ok {
		t.Fatal("the worker should report progress")
	}
	var estimated int64
	for !msg.done {
		estimated += msg.freed
		if msg.err != nil {
			t.Fatalf("a dry run should not fail: %v", msg.err)
		}
		msg = <-ch
	}

	if estimated != 3072 {
		t.Errorf("estimated %d bytes, want 3072 (on disk minus used)", estimated)
	}
	if info, err := os.Stat(path); err != nil || info.Size() != 4096 {
		t.Errorf("a dry run changed the disk: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "Duster", "operations.log")); err == nil {
		t.Error("a dry run wrote a compaction to the operations log")
	}
}
