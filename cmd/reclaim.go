package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Large, rarely visible space consumers that Duster reports but never touches
// itself: Windows.old is owned by TrustedInstaller and Microsoft documents no
// supported command line or API for removing it, and the hibernation file is a
// power setting rather than junk. Both are the user's call, so Duster measures
// them and says exactly how to act.

const (
	// A preview must not hang on a 30 GB tree, so the Windows.old walk gives
	// up after this and reports a lower bound.
	reclaimWalkBudget = 5 * time.Second

	dismAnalyzeTimeout = 15 * time.Minute
	dismCleanupTimeout = 90 * time.Minute
)

// reclaimItem is one reported space consumer. Bytes is a lower bound when
// Partial is set (the walk hit its budget, or a subtree was unreadable).
type reclaimItem struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Path    string   `json:"path"`
	Present bool     `json:"present"`
	Bytes   int64    `json:"bytes"`
	Partial bool     `json:"partial,omitempty"`
	Hints   []string `json:"hints,omitempty"`
	Advice  string   `json:"advice,omitempty"`
}

// componentStoreInfo holds what DISM's read-only analysis reports about the
// WinSxS component store. Unparsed means DISM ran but its output could not be
// read in full (a localized build that /English did not fully cover), in which
// case every number is unknown, never "nothing to gain": a half-read report
// must not look like a clean store.
type componentStoreInfo struct {
	Analyzed           bool   `json:"analyzed"`
	Unparsed           bool   `json:"unparsed,omitempty"`
	ActualBytes        int64  `json:"actual_bytes,omitempty"`
	SharedBytes        int64  `json:"shared_with_windows_bytes,omitempty"`
	BackupsBytes       int64  `json:"backups_and_disabled_features_bytes,omitempty"`
	CacheBytes         int64  `json:"cache_and_temporary_data_bytes,omitempty"`
	OverheadBytes      int64  `json:"overhead_bytes,omitempty"`
	ReclaimablePkgs    int    `json:"reclaimable_packages"`
	CleanupRecommended bool   `json:"cleanup_recommended"`
	LastCleanup        string `json:"last_cleanup,omitempty"`
	Error              string `json:"error,omitempty"`
}

// systemDriveRoot returns the root of the drive Windows is installed on, for
// example "C:\". Resolved through the kernel-backed Windows directory, never a
// literal, so it follows Windows to whatever drive it lives on.
func systemDriveRoot() string {
	if w := secureWindowsDir(); len(w) >= 2 && w[1] == ':' {
		return w[:2] + `\`
	}
	return `C:\`
}

const (
	windowsOldAdvice = "Removing it ends the 10-day option to go back to your previous Windows version. " +
		"Remove it in Settings > System > Storage > Cleanup recommendations (Windows 11) " +
		"or Storage > Temporary files (Windows 10). Duster never deletes it."
	hibernationAdvice = "Shrink it with `powercfg /hibernate /size 0` then `powercfg /hibernate /type reduced`, " +
		"or remove it with `powercfg /hibernate off`, which also turns off fast startup and hybrid sleep. " +
		"Both need an elevated terminal. Duster never changes power settings."
)

// Short versions for the 80-column optimize screen.
var (
	windowsOldHints = []string{
		"Remove in Settings > System > Storage > Cleanup recommendations.",
		"That ends the 10-day option to go back to your previous Windows.",
	}
	hibernationHints = []string{
		"Shrink: powercfg /hibernate /size 0 then /type reduced.",
		"Remove: powercfg /hibernate off (also turns off fast startup).",
	}
)

// buildReclaimItems measures the reported space consumers under a drive root.
// The root is a parameter so tests can point it at a temp directory.
func buildReclaimItems(driveRoot string) []reclaimItem {
	items := []reclaimItem{
		{ID: "windows_old", Name: "Previous Windows installation", Path: filepath.Join(driveRoot, "Windows.old"), Hints: windowsOldHints, Advice: windowsOldAdvice},
		{ID: "hibernation", Name: "Hibernation file", Path: filepath.Join(driveRoot, "hiberfil.sys"), Hints: hibernationHints, Advice: hibernationAdvice},
	}

	for i := range items {
		info, err := os.Lstat(items[i].Path)
		switch {
		case err == nil && info.IsDir():
			items[i].Present = true
			items[i].Bytes, items[i].Partial = dirSizeWithin(items[i].Path, reclaimWalkBudget)
		case err == nil:
			items[i].Present = true
			items[i].Bytes = info.Size()
		case os.IsNotExist(err):
			// Nothing to report: no feature update pending removal, or
			// hibernation is already off.
		default:
			// It exists but this process may not read it (hiberfil.sys and
			// Windows.old are both restricted). Report it without a size
			// rather than claiming it is absent.
			items[i].Present = true
			items[i].Partial = true
		}
	}
	return items
}

// dirSizeWithin sums a tree like calculateDirSize but stops once the budget
// expires, reporting whether the total is only a lower bound. Links are
// skipped, never followed.
func dirSizeWithin(root string, budget time.Duration) (int64, bool) {
	deadline := time.Now().Add(budget)
	var size int64
	var partial bool
	var seen int

	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			// An unreadable subtree means the real total is larger.
			partial = true
			return nil
		}
		if d.Type()&(os.ModeSymlink|os.ModeIrregular) != 0 {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		// Checking the clock per entry would cost more than the walk itself,
		// so check the first entry and then every 512th.
		if seen++; seen%512 == 1 && time.Now().After(deadline) {
			partial = true
			return filepath.SkipAll
		}
		if !d.IsDir() {
			if info, statErr := d.Info(); statErr == nil {
				size += info.Size()
			}
		}
		return nil
	})
	return size, partial
}

// dismResult reads a dism.exe exit status. Microsoft documents no exit code
// table for DISM, so only 0 and the standard Win32 3010 ("reboot required")
// count as success and every other code is surfaced verbatim.
func dismResult(err error) (code int, rebootRequired bool, ok bool) {
	if err == nil {
		return 0, false, true
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		code = exitErr.ExitCode()
		rebootRequired, ok = dismCodeOK(code)
		return code, rebootRequired, ok
	}
	return -1, false, false
}

// dismCodeOK reads one dism.exe exit code.
func dismCodeOK(code int) (rebootRequired, ok bool) {
	switch code {
	case 0:
		return false, true
	case 3010:
		// The standard Win32 "reboot required" success code. DISM's own codes
		// are undocumented, so nothing else counts as success.
		return true, true
	default:
		return false, false
	}
}

// dismCommand builds a DISM invocation. /English keeps the report labels
// parseable on non-English Windows, and /NoRestart stops DISM from restarting
// the machine under the user.
func dismCommand(ctx context.Context, args ...string) *exec.Cmd {
	full := append([]string{"/Online", "/English", "/NoRestart", "/Cleanup-Image"}, args...)
	// The executable comes from the kernel-resolved System32 directory (never
	// PATH or a mutable env var) and every argument is a constant above, so no
	// caller input reaches the command line.
	// nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command
	c := exec.CommandContext(ctx, systemExecutable("dism.exe"), full...)
	setProcessGroup(c)
	return c
}

// analyzeComponentStore runs DISM's read-only component store analysis.
func analyzeComponentStore(ctx context.Context) componentStoreInfo {
	runCtx, cancel := context.WithTimeout(ctx, dismAnalyzeTimeout)
	defer cancel()

	out, err := dismCommand(runCtx, "/AnalyzeComponentStore").CombinedOutput()
	if code, _, ok := dismResult(err); !ok {
		return componentStoreInfo{Error: dismErrorText(code, err, out)}
	}

	info := parseComponentStore(string(out))
	info.Analyzed = true
	return info
}

// startComponentCleanup removes superseded components. It never passes
// /ResetBase: that would block uninstalling updates that are already
// installed. Note this skips the 30-day grace period the Windows scheduled
// task applies, so previous component versions go immediately. Cancelling the
// context kills dism.exe mid-servicing, which Microsoft does not document as
// safely resumable, so callers must confirm before interrupting it.
func startComponentCleanup(ctx context.Context) (rebootRequired bool, err error) {
	runCtx, cancel := context.WithTimeout(ctx, dismCleanupTimeout)
	defer cancel()

	out, runErr := dismCommand(runCtx, "/StartComponentCleanup").CombinedOutput()
	code, reboot, ok := dismResult(runErr)
	if !ok {
		return false, errors.New(dismErrorText(code, runErr, out))
	}
	return reboot, nil
}

// dismErrorText keeps DISM's own last message, which names the real problem
// (for example "Error: 740" for a non-elevated run).
func dismErrorText(code int, err error, out []byte) string {
	msg := lastMeaningfulLine(string(out))
	switch {
	case msg != "" && code > 0:
		return fmt.Sprintf("dism exited %d: %s", code, msg)
	case code > 0:
		return fmt.Sprintf("dism exited %d", code)
	case msg != "":
		return fmt.Sprintf("dism failed: %s (%v)", msg, err)
	default:
		return fmt.Sprintf("dism failed: %v", err)
	}
}

func lastMeaningfulLine(out string) string {
	lines := strings.Split(out, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" || strings.HasPrefix(line, "[") {
			continue
		}
		return truncateString(line, 160)
	}
	return ""
}

// parseComponentStore reads DISM's AnalyzeComponentStore report. Its fields
// are "Label : Value" lines. Anything unrecognized leaves the sizes at zero
// and marks the report unparsed, so a localized build is never reported as
// "nothing to reclaim".
func parseComponentStore(out string) componentStoreInfo {
	var info componentStoreInfo

	// Every field a decision or a displayed number depends on. One missing
	// label means the report was only half understood, so the whole thing
	// counts as unreadable.
	seen := map[string]bool{}

	for _, line := range strings.Split(out, "\n") {
		label, value, found := strings.Cut(line, " : ")
		if !found {
			continue
		}
		label = strings.TrimSpace(label)
		value = strings.TrimSpace(strings.TrimRight(value, "\r"))

		switch label {
		case "Actual Size of Component Store":
			if b, ok := parseDismSize(value); ok {
				info.ActualBytes = b
				seen[label] = true
			}
		case "Shared with Windows":
			if b, ok := parseDismSize(value); ok {
				info.SharedBytes = b
			}
		case "Backups and Disabled Features":
			if b, ok := parseDismSize(value); ok {
				info.BackupsBytes = b
				seen[label] = true
			}
		case "Cache and Temporary Data":
			if b, ok := parseDismSize(value); ok {
				info.CacheBytes = b
				seen[label] = true
			}
		case "Number of Reclaimable Packages":
			if n, err := strconv.Atoi(value); err == nil && n >= 0 {
				info.ReclaimablePkgs = n
				seen[label] = true
			}
		case "Component Store Cleanup Recommended":
			if strings.EqualFold(value, "Yes") || strings.EqualFold(value, "No") {
				info.CleanupRecommended = strings.EqualFold(value, "Yes")
				seen[label] = true
			}
		case "Date of Last Cleanup":
			info.LastCleanup = value
		}
	}

	for _, required := range []string{
		"Actual Size of Component Store",
		"Backups and Disabled Features",
		"Cache and Temporary Data",
		"Number of Reclaimable Packages",
		"Component Store Cleanup Recommended",
	} {
		if !seen[required] {
			// Report nothing rather than a number built from half a report.
			return componentStoreInfo{Unparsed: true}
		}
	}

	// Microsoft documents the overhead as backups plus cache; the rest of the
	// store is either shared with Windows or in use.
	info.OverheadBytes = info.BackupsBytes + info.CacheBytes
	return info
}

// dismNumber matches the number formats DISM prints in English: plain
// ("512", "4.98") or comma-grouped thousands ("1,024", "1,024.50"). A comma
// used as a decimal separator by a partly localized build does not match, so
// "4,88 GB" is reported as unreadable rather than silently read as 488 GB.
var dismNumber = regexp.MustCompile(`^(\d+|\d{1,3}(,\d{3})+)(\.\d+)?$`)

// parseDismSize reads DISM's sizes, for example "4.98 GB" or "279.52 KB".
func parseDismSize(value string) (int64, bool) {
	fields := strings.Fields(value)
	if len(fields) != 2 || !dismNumber.MatchString(fields[0]) {
		return 0, false
	}
	amount, err := strconv.ParseFloat(strings.ReplaceAll(fields[0], ",", ""), 64)
	if err != nil || amount < 0 {
		return 0, false
	}
	units := map[string]float64{
		"B":  1,
		"KB": 1 << 10,
		"MB": 1 << 20,
		"GB": 1 << 30,
		"TB": 1 << 40,
	}
	mult, ok := units[strings.ToUpper(fields[1])]
	if !ok {
		return 0, false
	}
	return int64(amount * mult), true
}

// renderReclaimSection lists the reported-only space consumers on the
// optimize preview screen. Line breaks stay outside style.Render: lipgloss
// pads every line of a multi-line string to the widest one, which would push
// the next line far to the right.
func renderReclaimSection(items []reclaimItem, ready bool) string {
	var b strings.Builder
	b.WriteString(optWhiteText("  Reclaimable space (reported only, Duster never removes these):") + "\n")

	if !ready {
		b.WriteString(optGrayText("    Measuring...") + "\n\n")
		return b.String()
	}

	found := false
	for _, item := range items {
		if !item.Present {
			continue
		}
		found = true
		size := formatBytes(item.Bytes)
		switch {
		case item.Partial && item.Bytes == 0:
			size = "size unavailable"
		case item.Partial:
			size = "at least " + size
		}
		b.WriteString("    " + optWhiteText(padRight(item.Name, 32)) + optCyanText(size) + "\n")
		for _, hint := range item.Hints {
			b.WriteString("      " + optGrayText(hint) + "\n")
		}
	}
	if !found {
		b.WriteString(optGrayText("    Nothing to report: no previous Windows installation, no hibernation file.") + "\n")
	}

	b.WriteString("\n")
	return b.String()
}

// formatElapsed renders a task's running time, for a cleanup that can take
// tens of minutes.
func formatElapsed(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	return fmt.Sprintf("%dm%02ds", int(d.Minutes()), int(d.Seconds())%60)
}
