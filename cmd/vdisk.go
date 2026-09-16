package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/Nur-Adnan/duster/internal/logging"
)

// WSL 2 and Docker Desktop keep their Linux file systems in dynamically
// expanding VHDX files. They grow when the guest writes and never shrink when
// it deletes, so a developer machine quietly loses tens of gigabytes. Windows
// can hand the space back, but only through diskpart's "compact vdisk", and
// only while nothing has the disk open.
//
// Duster never edits, moves or deletes one of these disks. It compacts them in
// place, which rewrites the container around the same guest file system.

const (
	// Compaction rewrites the whole container. A 100 GB disk on a slow drive
	// is the case this budget has to cover.
	vdiskCompactTimeout = 60 * time.Minute

	// Detaching is a metadata operation, but it runs after a failure or a
	// cancellation, so it gets its own budget rather than an expired one.
	vdiskDetachTimeout = 2 * time.Minute

	vdiskShutdownTimeout = 2 * time.Minute
	vdiskProbeTimeout    = 20 * time.Second
)

// wslDistro is one registered WSL distribution, read from the Lxss registry
// key. Version is 1 or 2; only WSL 2 distributions have a VHDX.
type wslDistro struct {
	Name     string
	BasePath string
	Version  int
}

// virtualDisk is one VHDX that Duster can compact.
//
// Bytes is the disk's logical size and OnDiskBytes what it really occupies:
// they differ once a disk is sparse, and then the gap is space Windows has
// already handed back. UsedBytes comes from the guest and is only known for a
// distribution that was already running, so it is never required.
type virtualDisk struct {
	Kind        string `json:"kind"`
	Label       string `json:"label"`
	Path        string `json:"path"`
	Bytes       int64  `json:"bytes"`
	OnDiskBytes int64  `json:"on_disk_bytes"`
	UsedBytes   int64  `json:"used_bytes,omitempty"`
	UsedKnown   bool   `json:"used_known"`
	Sparse      bool   `json:"sparse,omitempty"`
	Compressed  bool   `json:"compressed,omitempty"`
	Encrypted   bool   `json:"encrypted,omitempty"`
	Blocked     string `json:"blocked,omitempty"`

	selected bool
}

const (
	vdiskKindWSL    = "wsl"
	vdiskKindDocker = "docker"
)

// recoverable estimates what compaction could return: everything the container
// occupies beyond what the guest reports as used. It is only an estimate, and
// only when the guest could be asked at all.
func (d virtualDisk) recoverable() (int64, bool) {
	if !d.UsedKnown || d.Blocked != "" {
		return 0, false
	}
	free := d.OnDiskBytes - d.UsedBytes
	if free < 0 {
		free = 0
	}
	return free, true
}

// ─────────────────────────────────────────────
// Discovery
// ─────────────────────────────────────────────

// findVirtualDisks collects every VHDX that WSL or Docker Desktop owns.
// Callers pass the roots so the scan can be exercised off Windows; on Windows
// they come from wslDistroEntries and dockerDiskRoots.
func findVirtualDisks(distros []wslDistro, dockerRoots []string) []virtualDisk {
	seen := make(map[string]bool)
	var disks []virtualDisk

	add := func(path, kind, label string) {
		key := strings.ToLower(filepath.Clean(path))
		if seen[key] || !isCompactableVhdx(path) {
			return
		}
		seen[key] = true
		disks = append(disks, describeVirtualDisk(path, kind, label))
	}

	for _, d := range distros {
		if d.BasePath == "" {
			continue
		}
		for _, path := range vhdxFilesIn(d.BasePath, false) {
			add(path, vdiskKindWSL, d.Name)
		}
	}

	// Docker Desktop has moved its disk more than once (data\ext4.vhdx,
	// disk\docker_data.vhdx, and the Hyper-V backend's own file), so the
	// layout is discovered rather than hard-coded to one file name.
	for _, root := range dockerRoots {
		for _, path := range vhdxFilesIn(root, true) {
			add(path, vdiskKindDocker, dockerDiskLabel(path))
		}
	}

	// Biggest first: the whole point is finding where the space went.
	sort.SliceStable(disks, func(i, j int) bool {
		return disks[i].OnDiskBytes > disks[j].OnDiskBytes
	})
	return disks
}

// vhdxFilesIn lists the VHDX files directly in root, and one level below it
// when nested is set. The depth is fixed rather than a walk: these directories
// are known shapes, and an unbounded walk over a developer's AppData is slow.
func vhdxFilesIn(root string, nested bool) []string {
	patterns := []string{filepath.Join(root, "*.vhdx")}
	if nested {
		patterns = append(patterns, filepath.Join(root, "*", "*.vhdx"))
	}
	var found []string
	for _, pattern := range patterns {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			continue
		}
		found = append(found, matches...)
	}
	sort.Strings(found)
	return found
}

// isCompactableVhdx is the gate every candidate passes before its path reaches
// diskpart: a real .vhdx file, never a link, and never WSL's swap disk, which
// is rebuilt on every boot and so has nothing to reclaim.
func isCompactableVhdx(path string) bool {
	if !strings.EqualFold(filepath.Ext(path), ".vhdx") {
		return false
	}
	if strings.EqualFold(filepath.Base(path), "swap.vhdx") {
		return false
	}
	// Lstat, never Stat: a link at the target is refused outright rather than
	// followed, the same rule the deletion paths use.
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return false
	}
	return true
}

func describeVirtualDisk(path, kind, label string) virtualDisk {
	disk := virtualDisk{Kind: kind, Label: label, Path: path}
	if info, err := os.Lstat(path); err == nil {
		disk.Bytes = info.Size()
	}

	allocated, sparse, compressed, encrypted := fileDiskUsage(path)
	disk.OnDiskBytes = allocated
	if disk.OnDiskBytes == 0 {
		// No allocation figure available (non-Windows, or the call failed):
		// the logical size is the honest fallback.
		disk.OnDiskBytes = disk.Bytes
	}
	disk.Sparse, disk.Compressed, disk.Encrypted = sparse, compressed, encrypted
	disk.Blocked = compactionBlocker(disk)
	return disk
}

// compactionBlocker names the reason diskpart would refuse this disk, so the
// scan can say so before the user waits on a run that cannot succeed. diskpart
// rejects these with "Virtual hard disk files must be uncompressed and
// unencrypted and must not be sparse."
func compactionBlocker(d virtualDisk) string {
	switch {
	case d.Sparse:
		return "sparse disk: Windows already reclaims this one automatically"
	case d.Compressed:
		return "NTFS-compressed: diskpart cannot compact it"
	case d.Encrypted:
		return "EFS-encrypted: diskpart cannot compact it"
	}
	return ""
}

func dockerDiskLabel(path string) string {
	return "Docker Desktop (" + filepath.Base(path) + ")"
}

// dockerDiskRoots lists where Docker Desktop keeps its disks. %LOCALAPPDATA%
// comes from the standard library rather than a raw environment read, and
// nothing here is trusted: every candidate still has to be a real .vhdx file.
func dockerDiskRoots() []string {
	var roots []string
	if local, err := os.UserCacheDir(); err == nil && local != "" {
		roots = append(roots, filepath.Join(local, "Docker", "wsl"))
	}
	if programData := os.Getenv("ProgramData"); programData != "" {
		roots = append(roots, filepath.Join(programData, "DockerDesktop", "vm-data"))
	}
	return roots
}

// ─────────────────────────────────────────────
// Measuring the guest
// ─────────────────────────────────────────────

// wslExecutable returns wsl.exe's path, or "" when WSL is not installed.
func wslExecutable() string {
	path := systemExecutable("wsl.exe")
	if info, err := os.Stat(path); err != nil || info.IsDir() {
		return ""
	}
	return path
}

// measureRunningDistros fills in UsedBytes for distributions that are already
// running. Stopped ones are left unknown on purpose: booting a distribution to
// measure it would be a side effect of a preview.
func measureRunningDistros(ctx context.Context, disks []virtualDisk, distros []wslDistro) []virtualDisk {
	wsl := wslExecutable()
	if wsl == "" {
		return disks
	}
	running := runningWSLDistros(ctx, wsl)
	if len(running) == 0 {
		return disks
	}

	base := make(map[string]string, len(distros)) // lowercased dir -> distro name
	for _, d := range distros {
		if d.BasePath != "" {
			base[strings.ToLower(filepath.Clean(d.BasePath))] = d.Name
		}
	}

	for i, disk := range disks {
		name, ok := base[strings.ToLower(filepath.Dir(disk.Path))]
		if !ok || !running[name] {
			continue
		}
		if used, ok := distroUsedBytes(ctx, wsl, name); ok {
			disks[i].UsedBytes = used
			disks[i].UsedKnown = true
		}
	}
	return disks
}

// runningWSLDistros returns the names WSL reports as running.
func runningWSLDistros(ctx context.Context, wsl string) map[string]bool {
	probeCtx, cancel := context.WithTimeout(ctx, vdiskProbeTimeout)
	defer cancel()

	out, err := wslCommand(probeCtx, wsl, "--list", "--running", "--quiet").Output()
	if err != nil {
		return nil
	}

	running := make(map[string]bool)
	for _, line := range strings.Split(decodeWSLOutput(out), "\n") {
		if name := strings.TrimSpace(line); name != "" {
			running[name] = true
		}
	}
	return running
}

// distroUsedBytes asks the guest how much of its root file system is in use.
// df -k is the POSIX spelling, so it also works on the busybox df that minimal
// distributions ship.
func distroUsedBytes(ctx context.Context, wsl, distro string) (int64, bool) {
	probeCtx, cancel := context.WithTimeout(ctx, vdiskProbeTimeout)
	defer cancel()

	out, err := wslCommand(probeCtx, wsl, "--distribution", distro, "--exec", "df", "-k", "/").Output()
	if err != nil {
		return 0, false
	}
	return parseDfUsedBytes(string(out))
}

// parseDfUsedBytes reads the used column out of `df -k /`. It keys off the
// mount point in the last column rather than a line number, because df wraps
// long device names onto a second line.
func parseDfUsedBytes(out string) (int64, bool) {
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		// filesystem, 1K-blocks, used, available, capacity, mounted-on. The
		// filesystem is missing when df wrapped a long device name onto its
		// own line, so the columns are counted back from the mount point.
		if len(fields) < 5 || fields[len(fields)-1] != "/" {
			continue
		}
		blocks, err := strconv.ParseInt(fields[len(fields)-4], 10, 64)
		if err != nil || blocks < 0 {
			continue
		}
		return blocks * 1024, true
	}
	return 0, false
}

// decodeWSLOutput converts wsl.exe's own output to UTF-8. wsl.exe writes
// UTF-16LE with no BOM when its output is redirected, so a plain string
// conversion yields NUL-separated characters that match nothing.
func decodeWSLOutput(b []byte) string {
	if len(b) >= 2 && b[0] != 0 && b[1] == 0 {
		units := make([]uint16, 0, len(b)/2)
		for i := 0; i+1 < len(b); i += 2 {
			units = append(units, uint16(b[i])|uint16(b[i+1])<<8)
		}
		return string(utf16.Decode(units))
	}
	return string(b)
}

// ─────────────────────────────────────────────
// Compaction
// ─────────────────────────────────────────────

// shutdownWSL stops the WSL VM so nothing holds a disk open. Without it every
// compaction fails on a sharing violation. It is a no-op when WSL is absent.
func shutdownWSL(ctx context.Context) error {
	wsl := wslExecutable()
	if wsl == "" {
		return nil
	}
	shutdownCtx, cancel := context.WithTimeout(ctx, vdiskShutdownTimeout)
	defer cancel()

	if out, err := wslCommand(shutdownCtx, wsl, "--shutdown").CombinedOutput(); err != nil {
		return fmt.Errorf("wsl --shutdown failed: %s", firstLine(decodeWSLOutput(out)))
	}
	return nil
}

// compactVirtualDisk shrinks one disk in place and reports the space returned.
//
// The disk is attached read-only for the scan, which is what lets Windows find
// the unused blocks, and detached again afterwards. The detach is guaranteed:
// a disk left attached is a disk WSL can no longer start.
func compactVirtualDisk(ctx context.Context, disk virtualDisk) (int64, error) {
	if disk.Blocked != "" {
		return 0, errors.New(disk.Blocked)
	}
	if !isCompactableVhdx(disk.Path) {
		return 0, errors.New("not a virtual disk file any more")
	}

	target, err := diskpartTarget(disk.Path)
	if err != nil {
		return 0, err
	}

	before, _, _, _ := fileDiskUsage(disk.Path)

	compactCtx, cancel := context.WithTimeout(ctx, vdiskCompactTimeout)
	defer cancel()

	runErr := runDiskpart(compactCtx,
		`select vdisk file="`+target+`"`,
		"attach vdisk readonly",
		"compact vdisk",
		"detach vdisk",
	)
	if runErr != nil {
		// The script stops at the first failing command, so the disk may
		// still be attached. Detach on a fresh budget, because the failure
		// may well have been the parent context expiring.
		detachVirtualDisk(target)
		logVdiskOperation(disk.Path, 0, false)
		return 0, runErr
	}

	after, _, _, _ := fileDiskUsage(disk.Path)
	freed := before - after
	if freed < 0 {
		freed = 0
	}
	logVdiskOperation(disk.Path, freed, true)
	return freed, nil
}

// detachVirtualDisk is the best-effort cleanup after a failed compaction. It
// runs on its own context so a cancelled or timed-out parent cannot skip it,
// and its own failure is not worth reporting: the disk was already either
// detached or beyond Duster's reach.
func detachVirtualDisk(target string) {
	detachCtx, cancel := context.WithTimeout(context.Background(), vdiskDetachTimeout)
	defer cancel()
	_ = runDiskpart(detachCtx, `select vdisk file="`+target+`"`, "detach vdisk")
}

// wslCommand builds a wsl.exe invocation. The executable is System32's
// wsl.exe, located through the kernel-resolved system directory rather than
// %PATH%, and every argument is a literal above except the distribution name,
// which comes from WSL's own registry and is passed as its own argv entry, so
// there is no command line for it to break out of.
func wslCommand(ctx context.Context, wsl string, args ...string) *exec.Cmd {
	c := exec.CommandContext(ctx, wsl, args...) // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command
	setProcessGroup(c)
	return c
}

// diskpartCommand builds the diskpart invocation. The executable is System32's
// diskpart.exe from the kernel-resolved system directory, and the only
// argument is the path of a script Duster just wrote itself.
func diskpartCommand(ctx context.Context, script string) *exec.Cmd {
	c := exec.CommandContext(ctx, systemExecutable("diskpart.exe"), "/s", script) // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command
	setProcessGroup(c)
	return c
}

// runDiskpart feeds one script to diskpart. diskpart is only ever given vdisk
// commands against a verified .vhdx path: no disk, partition or volume is ever
// selected, so there is nothing here that can reach a physical disk.
func runDiskpart(ctx context.Context, commands ...string) error {
	script, err := writeDiskpartScript(commands)
	if err != nil {
		return err
	}
	defer os.Remove(script)

	out, err := diskpartCommand(ctx, script).CombinedOutput()

	// diskpart has been known to exit 0 on a script it could not complete, so
	// its output decides as well as its exit code.
	if msg := diskpartError(string(out)); msg != "" {
		return errors.New(msg)
	}
	if err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("compaction was stopped before it finished: %w", ctx.Err())
		}
		return fmt.Errorf("diskpart failed: %v", err)
	}
	return nil
}

// writeDiskpartScript writes the commands to a private temporary file. The
// script is deleted by the caller; it holds no secrets, only a path already
// visible to the user.
func writeDiskpartScript(commands []string) (string, error) {
	f, err := os.CreateTemp("", "duster-vdisk-*.txt")
	if err != nil {
		return "", fmt.Errorf("could not write a diskpart script: %w", err)
	}
	body := strings.Join(append(append([]string{}, commands...), "exit", ""), "\r\n")
	if _, err := f.WriteString(body); err != nil {
		f.Close()
		os.Remove(f.Name())
		return "", fmt.Errorf("could not write a diskpart script: %w", err)
	}
	if err := f.Close(); err != nil {
		os.Remove(f.Name())
		return "", fmt.Errorf("could not write a diskpart script: %w", err)
	}
	return f.Name(), nil
}

// diskpartTarget returns the path to put in the script. A diskpart script is
// read in the system's ANSI code page, so a path with non-ASCII characters
// (a profile named in Cyrillic, say) would not name the same file once
// written as UTF-8. The 8.3 short name is the ASCII-only way to say it; when
// the volume has short names turned off there is no safe spelling, and
// refusing beats compacting whatever the mangled path happened to hit.
func diskpartTarget(path string) (string, error) {
	if isASCII(path) {
		return path, nil
	}
	if short := shortPathName(path); short != "" && isASCII(short) {
		return short, nil
	}
	return "", fmt.Errorf("the path to this disk cannot be written to a diskpart script (it is outside ASCII and the volume has 8.3 names disabled); compact it by hand with: diskpart, then select vdisk file=%q", path)
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] > 0x7F {
			return false
		}
	}
	return true
}

// diskpartError pulls the useful line out of diskpart's output. diskpart
// prints a banner and a copyright notice before anything else, so the last
// thing it says is the thing worth repeating.
func diskpartError(out string) string {
	markers := []string{
		"DiskPart has encountered an error",
		"Virtual Disk Service error",
		"The system cannot find the file",
		"must not be sparse",
		"access is denied",
	}
	var lines []string
	for _, line := range strings.Split(out, "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			lines = append(lines, trimmed)
		}
	}
	for i, line := range lines {
		for _, marker := range markers {
			if !strings.Contains(strings.ToLower(line), strings.ToLower(marker)) {
				continue
			}
			// diskpart puts the explanation on the line after the banner.
			if i+1 < len(lines) && !strings.HasPrefix(lines[i+1], "See the System Event Log") {
				return line + " " + lines[i+1]
			}
			return line
		}
	}
	return ""
}

func firstLine(s string) string {
	if idx := strings.IndexAny(s, "\r\n"); idx >= 0 {
		return strings.TrimSpace(s[:idx])
	}
	return strings.TrimSpace(s)
}

func logVdiskOperation(path string, freed int64, success bool) {
	logging.LogDestructiveOperation("vdisk", "compact", path, freed, success)
}
