package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/Nur-Adnan/duster/internal/logging"
	"github.com/Nur-Adnan/duster/lib/fs"
	"github.com/charmbracelet/lipgloss"
)

// ─────────────────────────────────────────────
// Canonical Duster Color Palette
// ─────────────────────────────────────────────

var (
	colorDimGray   = lipgloss.Color("#333333") // subtle dark gray separators/empty ticks
	colorWhite     = lipgloss.Color("#E8E8F0")
	colorMint      = lipgloss.Color("#00FF66") // neon green primary
	colorAmber     = lipgloss.Color("#FFCC00") // yellow metrics
	colorCoral     = lipgloss.Color("#FF4D4D") // danger/error
	colorGold      = lipgloss.Color("#FFCC00") // highlight/yellow
	colorSkyBlue   = lipgloss.Color("#00D4FF") // cyan secondary
	colorLimeGreen = lipgloss.Color("#00FF66") // neon green success
	colorSilver    = lipgloss.Color("#A0A0B0") // muted white labels
)

// ─────────────────────────────────────────────
// Shared Text Styles
// ─────────────────────────────────────────────

var (
	styleLabel   = lipgloss.NewStyle().Foreground(colorSilver)
	styleValue   = lipgloss.NewStyle().Foreground(colorWhite).Bold(true)
	styleAccent  = lipgloss.NewStyle().Foreground(colorSkyBlue).Bold(true)
	styleSuccess = lipgloss.NewStyle().Foreground(colorLimeGreen).Bold(true)
	styleWarning = lipgloss.NewStyle().Foreground(colorAmber).Bold(true)
	styleDanger  = lipgloss.NewStyle().Foreground(colorCoral).Bold(true)
	styleMuted   = lipgloss.NewStyle().Foreground(colorDimGray)
	styleDivider = lipgloss.NewStyle().Foreground(colorDimGray)
	styleHeader  = lipgloss.NewStyle().Foreground(colorSkyBlue).Bold(true)
	styleSub     = lipgloss.NewStyle().Foreground(colorSilver)
	styleTitle   = lipgloss.NewStyle().Foreground(colorSkyBlue).Bold(true)

	// Aliases still referenced by a few commands
	boldWhite        = lipgloss.NewStyle().Foreground(colorWhite).Bold(true)
	redColorStyle    = lipgloss.NewStyle().Foreground(colorCoral)
	yellowColorStyle = lipgloss.NewStyle().Foreground(colorAmber)
)

// ─────────────────────────────────────────────
// progressBar — compact ASCII progress bar
// Renders exactly like the screenshot: [██████████----------]
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

	filledStyle := lipgloss.NewStyle().Foreground(colorMint)
	emptyStyle := lipgloss.NewStyle().Foreground(colorDimGray)
	bracketStyle := lipgloss.NewStyle().Foreground(colorSkyBlue)

	return bracketStyle.Render("[") +
		filledStyle.Render(strings.Repeat("█", filled)) +
		emptyStyle.Render(strings.Repeat("-", empty)) +
		bracketStyle.Render("]")
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
// truncateString — rune-safe truncation with ellipsis
// ─────────────────────────────────────────────

func truncateString(s string, maxLen int) string {
	if maxLen <= 1 {
		return "…" // runes[:maxLen-1] would panic on narrow widths
	}
	if utf8.RuneCountInString(s) <= maxLen {
		return s
	}
	runes := []rune(s)
	return string(runes[:maxLen-1]) + "…"
}

// clampHead shortens s to at most `width` display columns, appending "..." when
// it was longer. Rune-aware, so a multi-byte path/name never truncates
// mid-character (byte slicing produced a � at the cut on non-ASCII input).
func clampHead(s string, width int) string {
	if width < 4 {
		return s
	}
	r := []rune(s)
	if len(r) <= width {
		return s
	}
	return string(r[:width-3]) + "..."
}

// clampTail keeps the tail of s within `width` columns, prepending "..." when it
// was longer — used for long paths where the leaf segment matters most.
func clampTail(s string, width int) string {
	if width < 4 {
		return s
	}
	r := []rune(s)
	if len(r) <= width {
		return s
	}
	return "..." + string(r[len(r)-(width-3):])
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
// Shared Text Rendering Helpers
// Canonical single-source color text renderers
// used across all TUI subcommands.
// ─────────────────────────────────────────────

func cyanText(s string) string  { return lipgloss.NewStyle().Foreground(colorMint).Render(s) }
func whiteText(s string) string { return styleValue.Render(s) }
func grayText(s string) string  { return styleMuted.Render(s) }

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
	// The target path is passed through an environment variable rather than as a
	// command argument. Trailing args after `powershell -Command "<string>"` are
	// appended to the command *text* (they do not populate $args), so an inline
	// $args[0] is always $null and any $(...) in the path would be executed. Reading
	// $env:DUSTER_DELETE_TARGET expands the value literally, and -LiteralPath keeps
	// wildcard/special characters inert.
	c := exec.Command(systemExecutable(`WindowsPowerShell\v1.0\powershell.exe`),
		"-NoProfile", "-WindowStyle", "Hidden", "-Command",
		"Start-Sleep -Seconds 2; Remove-Item -Force -ErrorAction SilentlyContinue -LiteralPath $env:DUSTER_DELETE_TARGET",
	)
	c.Env = append(os.Environ(), "DUSTER_DELETE_TARGET="+targetPath)
	if err := c.Start(); err != nil {
		logging.Logger.Error("failed to schedule delayed delete", "target", targetPath, "error", err)
		return
	}
	go func() { _ = c.Wait() }()
}

// systemExecutable returns the absolute System32 path of a Windows system
// binary. It resolves System32 from the kernel API (defeating %SystemRoot%
// spoofing) and always returns a fully-qualified path — never a bare name that
// would fall back to a PATH lookup a planted binary could hijack.
func systemExecutable(relPath string) string {
	if secure, err := fs.GetSecureSystemDirectory(); err == nil && secure != "" {
		return filepath.Join(secure, relPath)
	}
	systemRoot := os.Getenv("SystemRoot")
	if systemRoot == "" {
		systemRoot = `C:\Windows`
	}
	return filepath.Join(systemRoot, "System32", relPath)
}

// ─────────────────────────────────────────────
// RenderHeader — high-fidelity header layout
// Renders the command prompt and ASCII logo block matching the screenshot.
// ─────────────────────────────────────────────

func RenderHeader(width int, currentCommand string) string {
	return RenderHeaderWithSubtitle(width, currentCommand, "System Maintenance CLI v"+AppVersion, "Keep your system clean. Keep it running smooth.")
}

// RenderHeaderWithSubtitle — generalized header layout supporting custom title and subtitle on the right
func RenderHeaderWithSubtitle(width int, currentCommand, title, subtitle string) string {
	var sb strings.Builder
	// Top command prompt:
	// path is C:\> (white)
	// command is duster --status (green)
	prompt := "  " + styleValue.Render("C:\\>") + lipgloss.NewStyle().Foreground(colorMint).Render(currentCommand) + "\n\n"
	sb.WriteString(prompt)

	// If width is too narrow, render a compact version
	if width > 0 && width < 78 {
		sb.WriteString("  " + styleAccent.Render("DUSTER") + styleValue.Render(" | "+title) + "\n")
		if subtitle != "" {
			sb.WriteString("  " + styleSuccess.Render(subtitle) + "\n")
		}
		// Guard against a negative repeat count on very narrow terminals (width 1-4),
		// which would panic strings.Repeat.
		divWidth := width - 4
		if divWidth < 1 {
			divWidth = 1
		}
		sb.WriteString("  " + styleMuted.Render(strings.Repeat("─", divWidth)) + "\n\n")
		return sb.String()
	}

	// Otherwise, render the magnificent high-fidelity ASCII block
	broom := []string{
		"    / ",
		"   /  ",
		"  /   ",
		" /_   ",
		"\\--/  ",
		"/__/  ",
	}
	duster := []string{
		" ______   _    _   _____  _______  ______  _____  ",
		"|  __  \\ | |  | | / ____|__   __| |  ____||  __ \\ ",
		"| |  \\  \\| |  | || (___    | |    | |__   | |__) |",
		"| |  |  || |  | | \\___ \\   | |    |  __|  |  _  / ",
		"| |__/  /| |__| | ____) |  | |    | |____ | | \\ \\ ",
		"|______/  \\____/ |_____/   |_|    |______||_|  \\_\\",
	}

	styleBroom := lipgloss.NewStyle().Foreground(colorGold)
	styleDuster := lipgloss.NewStyle().Foreground(colorSkyBlue)
	styleSep := lipgloss.NewStyle().Foreground(colorDimGray)
	styleTagline := lipgloss.NewStyle().Foreground(colorMint)
	styleVer := lipgloss.NewStyle().Foreground(colorWhite).Bold(true)

	for i := 0; i < 6; i++ {
		bLine := styleBroom.Render(broom[i])
		dLine := styleDuster.Render(duster[i])
		sep := styleSep.Render(" │ ")

		var rLine string
		switch i {
		case 1:
			rLine = styleVer.Render(title)
		case 2:
			rLine = styleTagline.Render(subtitle)
		default:
			rLine = ""
		}

		sb.WriteString("  " + bLine + " " + dLine + sep + rLine + "\n")
	}

	dividerWidth := width - 4
	if dividerWidth < 80 {
		dividerWidth = 80
	}
	sb.WriteString("\n  " + styleSep.Render(strings.Repeat("─", dividerWidth)) + "\n\n")

	return sb.String()
}
