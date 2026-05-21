package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
)

// ─────────────────────────────────────────────
// Canonical Duster Color Palette
// ─────────────────────────────────────────────

var (
	colorDimGray   = lipgloss.Color("#4A4A5A")
	colorSlate     = lipgloss.Color("#2D2D3D")
	colorWhite     = lipgloss.Color("#E8E8F0")
	colorMint      = lipgloss.Color("#00D4AA") // primary accent — teal/mint
	colorAmber     = lipgloss.Color("#FFB347") // warning
	colorCoral     = lipgloss.Color("#FF6B6B") // danger/error
	colorGold      = lipgloss.Color("#FFD700") // highlight
	colorSkyBlue   = lipgloss.Color("#87CEEB") // secondary accent
	colorLimeGreen = lipgloss.Color("#7FFF00") // success
	colorSilver    = lipgloss.Color("#A0A0B0") // muted text
)

// ─────────────────────────────────────────────
// Shared Text Styles
// ─────────────────────────────────────────────

var (
	styleLabel    = lipgloss.NewStyle().Foreground(colorSilver)
	styleValue    = lipgloss.NewStyle().Foreground(colorWhite).Bold(true)
	styleAccent   = lipgloss.NewStyle().Foreground(colorMint).Bold(true)
	styleSuccess  = lipgloss.NewStyle().Foreground(colorLimeGreen).Bold(true)
	styleWarning  = lipgloss.NewStyle().Foreground(colorAmber).Bold(true)
	styleDanger   = lipgloss.NewStyle().Foreground(colorCoral).Bold(true)
	styleMuted    = lipgloss.NewStyle().Foreground(colorDimGray)
	styleDivider  = lipgloss.NewStyle().Foreground(colorDimGray)
	styleSelected = lipgloss.NewStyle().Foreground(colorMint).Bold(true)
	styleHeader   = lipgloss.NewStyle().Foreground(colorWhite).Bold(true)
	styleSub      = lipgloss.NewStyle().Foreground(colorSilver)
	styleNumber   = lipgloss.NewStyle().Foreground(colorDimGray)
	styleAge      = lipgloss.NewStyle().Foreground(colorAmber)
	styleTitle    = lipgloss.NewStyle().Foreground(colorMint).Bold(true)

	// Re-exported aliases used by older commands to avoid renaming every ref
	boldWhite        = lipgloss.NewStyle().Foreground(colorWhite).Bold(true)
	grayColorStyle   = lipgloss.NewStyle().Foreground(colorSilver)
	redColorStyle    = lipgloss.NewStyle().Foreground(colorCoral)
	yellowColorStyle = lipgloss.NewStyle().Foreground(colorAmber)
	errorStyle       = lipgloss.NewStyle().Foreground(colorCoral).Bold(true)
	divStyle         = lipgloss.NewStyle().Foreground(colorDimGray)
	footerStyle      = lipgloss.NewStyle().Foreground(colorDimGray).PaddingTop(1).PaddingLeft(2)
	dirStyle         = lipgloss.NewStyle().Foreground(colorSkyBlue)
	fileStyle        = lipgloss.NewStyle().Foreground(colorSilver)
	selectedStyle    = lipgloss.NewStyle().Foreground(colorMint).Bold(true)
)

// ─────────────────────────────────────────────
// progressBar — compact ASCII progress bar
// Renders: [████████░░░░]
// ─────────────────────────────────────────────

func progressBar(percent float64, width int) string {
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}
	filled := int(percent / 100.0 * float64(width))
	if filled > width {
		filled = width
	}
	empty := width - filled

	var filledStyle lipgloss.Style
	switch {
	case percent >= 85:
		filledStyle = lipgloss.NewStyle().Foreground(colorCoral)
	case percent >= 60:
		filledStyle = lipgloss.NewStyle().Foreground(colorAmber)
	default:
		filledStyle = lipgloss.NewStyle().Foreground(colorMint)
	}

	return filledStyle.Render(strings.Repeat("█", filled)) +
		styleMuted.Render(strings.Repeat("░", empty))
}

// MiniBar — 5-dot mini I/O sparkline (▪▪▪▪□ disk read/write indicator)
func miniBar(value, maxValue float64, width int) string {
	if maxValue <= 0 {
		return styleMuted.Render(strings.Repeat("□", width))
	}
	filled := int((value / maxValue) * float64(width))
	if filled > width {
		filled = width
	}
	empty := width - filled
	return styleAccent.Render(strings.Repeat("▪", filled)) +
		styleMuted.Render(strings.Repeat("□", empty))
}

// ─────────────────────────────────────────────
// Status Header — one-liner system overview
// e.g. "Duster Status  Health ● 92  DESKTOP-NUR · Intel i7 · 16GB · Windows 11"
// ─────────────────────────────────────────────

func statusHeader(healthScore int, hostname, cpuModel, ramGB, osVer string) string {
	var healthStyle lipgloss.Style
	switch {
	case healthScore >= 80:
		healthStyle = lipgloss.NewStyle().Foreground(colorLimeGreen).Bold(true)
	case healthScore >= 50:
		healthStyle = lipgloss.NewStyle().Foreground(colorAmber).Bold(true)
	default:
		healthStyle = lipgloss.NewStyle().Foreground(colorCoral).Bold(true)
	}

	title := styleTitle.Render("Duster Status")
	health := healthStyle.Render(fmt.Sprintf("Health ● %d", healthScore))
	meta := styleSub.Render(fmt.Sprintf("%s · %s · %s · %s", hostname, cpuModel, ramGB, osVer))

	return fmt.Sprintf("%s  %s  %s", title, health, meta)
}

// ─────────────────────────────────────────────
// Analyze Header
// e.g. "Analyze Disk  C:\Users\Nur  |  Total: 156.8GB"
// ─────────────────────────────────────────────

func analyzeHeader(path, totalSize string) string {
	title := styleTitle.Render("Analyze Disk")
	pathStr := styleValue.Render(path)
	sizeStr := styleAccent.Render(totalSize)
	return fmt.Sprintf("%s  %s  %s  %s", title, pathStr, styleMuted.Render("|"), sizeStr)
}

// ─────────────────────────────────────────────
// Keyboard Hints Footer
// e.g. "↑↓ Navigate  │  O Open  │  F Show  │  ⌫ Delete  │  L Large files  │  Q Quit"
// ─────────────────────────────────────────────

func kbHints(pairs ...string) string {
	parts := make([]string, 0, len(pairs))
	sep := styleMuted.Render("  │  ")
	for _, p := range pairs {
		parts = append(parts, styleMuted.Render(p))
	}
	return strings.Join(parts, sep)
}

// ─────────────────────────────────────────────
// Aging Label — ">6mo", ">1yr", ">3yr"
// ─────────────────────────────────────────────

func getAgeLabel(path string) string {
	info, err := os.Stat(path)
	if err != nil {
		return ""
	}
	age := time.Since(info.ModTime())
	days := age.Hours() / 24
	switch {
	case days > 1095: // >3 years
		return styleAge.Render(">3yr")
	case days > 365: // >1 year
		return styleAge.Render(">1yr")
	case days > 180: // >6 months
		return styleAge.Render(">6mo")
	default:
		return ""
	}
}

// ─────────────────────────────────────────────
// CLEANUP COMPLETE Banner — Duster summary
// ─────────────────────────────────────────────

func printCleanupBanner(dryRun bool, freedBytes int64, freeNowBytes int64, filesCount, categoriesCount int) {
	border := styleDivider.Render("  " + strings.Repeat("═", 66))
	fmt.Println(border)

	if dryRun {
		fmt.Println("  " + styleWarning.Render("DRY RUN COMPLETE!"))
		fmt.Printf("  Potential reclaimable space: %s %s  |  Free space now: %s\n",
			styleAccent.Render(formatBytes(freedBytes)),
			styleMuted.Render("(no changes made)"),
			styleValue.Render(formatBytes(freeNowBytes)),
		)
		fmt.Printf("  Files scanned: %s  |  Categories processed: %s\n",
			styleAccent.Render(formatInt(filesCount)),
			styleAccent.Render(fmt.Sprintf("%d", categoriesCount)),
		)
		fmt.Println("  " + styleMuted.Render("For deeper cleanup, run without --dry-run flag"))
	} else {
		fmt.Println("  " + styleSuccess.Render("CLEANUP COMPLETE!"))
		fmt.Printf("  Space freed: %s  |  Free space now: %s\n",
			styleAccent.Render(formatBytes(freedBytes)),
			styleValue.Render(formatBytes(freeNowBytes)),
		)
		if equiv := spaceEquivalent(freedBytes); equiv != "" {
			fmt.Println("  " + styleSub.Render(equiv))
		}
		fmt.Printf("  Files cleaned: %s  |  Categories processed: %s\n",
			styleAccent.Render(formatInt(filesCount)),
			styleAccent.Render(fmt.Sprintf("%d", categoriesCount)),
		)
	}

	fmt.Println(border)
}

// Space equivalent fun facts
func spaceEquivalent(bytes int64) string {
	gb := float64(bytes) / (1024 * 1024 * 1024)
	switch {
	case gb >= 100:
		return fmt.Sprintf("That's like ~%.0f 4K movies worth of space!", gb/4.5)
	case gb >= 20:
		return fmt.Sprintf("That's like ~%.0f 4K movies worth of space!", gb/4.5)
	case gb >= 4:
		return fmt.Sprintf("That's like ~%.0f hours of HD video!", gb*0.7)
	case gb >= 1:
		return fmt.Sprintf("That's like ~%.0f thousand photos!", gb*333)
	default:
		return ""
	}
}

// ─────────────────────────────────────────────
// explorerProgressBar — kept for analyze compat
// ─────────────────────────────────────────────

func explorerProgressBar(percent float64, width int) string {
	return progressBar(percent, width)
}

// ─────────────────────────────────────────────
// formatBytes — already defined in utils.go,
// formatInt helper for commas
// ─────────────────────────────────────────────

func formatInt(n int) string {
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}
	s := fmt.Sprintf("%d", n)
	result := ""
	for i, ch := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			result += ","
		}
		result += string(ch)
	}
	return result
}

// ─────────────────────────────────────────────
// getDiskFreeBytes — cross-platform wrapper
// Returns free bytes on the disk containing path
// ─────────────────────────────────────────────

func getDiskFreeBytes(path string) int64 {
	return getDiskFreeBytesOS(path)
}

// ─────────────────────────────────────────────
// truncateString (already in analyze.go — shared here for safety)
// ─────────────────────────────────────────────

func truncateString(s string, maxLen int) string {
	if utf8.RuneCountInString(s) <= maxLen {
		return s
	}
	runes := []rune(s)
	return string(runes[:maxLen-1]) + "…"
}

// ─────────────────────────────────────────────
// padRight — right-pad a string to width
// ─────────────────────────────────────────────

func padRight(s string, width int) string {
	n := utf8.RuneCountInString(s)
	if n >= width {
		return s
	}
	return s + strings.Repeat(" ", width-n)
}

// ─────────────────────────────────────────────
// getParentPath — returns parent directory
// ─────────────────────────────────────────────

func getParentPath(p string) string {
	return filepath.Dir(p)
}

// ─────────────────────────────────────────────
// Shared TUI Layout Styles
// Used by optimize, purge, update, remove, installer, doctor
// to render consistent boxed layouts without per-file duplication.
// ─────────────────────────────────────────────

var (
	sharedBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorMint).
			Padding(1, 2).
			Width(80)

	sharedHeaderStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(colorMint).
				Padding(0, 1)

	sharedFooterStyle = lipgloss.NewStyle().
				Foreground(colorDimGray).
				PaddingTop(1).
				PaddingLeft(2)

	sharedDividerLine = lipgloss.NewStyle().
				Foreground(colorDimGray)
)

// ─────────────────────────────────────────────
// Shared Text Rendering Helpers
// Canonical single-source color text renderers
// used across all TUI subcommands.
// ─────────────────────────────────────────────

func cyanText(s string) string  { return lipgloss.NewStyle().Foreground(colorMint).Render(s) }
func whiteText(s string) string { return styleValue.Render(s) }
func grayText(s string) string  { return styleMuted.Render(s) }
func greenText(s string) string { return styleSuccess.Render(s) }
func redText(s string) string   { return styleDanger.Render(s) }
func amberText(s string) string { return styleWarning.Render(s) }

// ─────────────────────────────────────────────
// scheduleDelayedDelete — safely schedule a file deletion
// after a brief delay without shell injection risk.
//
// On Windows, this spawns a PowerShell process that sleeps
// then deletes the target file. On other OS, it deletes
// immediately since running binaries can be unlinked.
//
// SECURITY: This replaces the previous pattern of passing
// user-controlled paths into `cmd.exe /C` format strings,
// which was vulnerable to command injection.
// ─────────────────────────────────────────────

func scheduleDelayedDelete(targetPath string) {
	if runtime.GOOS == "windows" {
		// Use PowerShell with discrete arguments — no string interpolation into a shell.
		// PowerShell's -Command receives the script as a single argument, but we
		// avoid any path concatenation into the script string by using $args.
		c := exec.Command("powershell.exe",
			"-NoProfile", "-WindowStyle", "Hidden", "-Command",
			"Start-Sleep -Seconds 2; Remove-Item -Force -ErrorAction SilentlyContinue -LiteralPath $args[0]",
			targetPath,
		)
		_ = c.Start()
	} else {
		_ = os.Remove(targetPath)
	}
}
