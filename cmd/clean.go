package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Nur-Adnan/duster/internal/logging"
	"github.com/Nur-Adnan/duster/lib/elevation"
	"github.com/Nur-Adnan/duster/lib/fs"
	"github.com/spf13/cobra"
)

// CleanCategory represents a distinct system cache target.
type CleanCategory struct {
	ID          string
	Name        string
	Description string
	Paths       []string
	FilesOnly   bool // If true, deletes files within directories but leaves subdirs
	Pattern     string
	CustomScan  func(dryRun bool, debug bool) (int64, int, error)
}

var (
	dryRun    bool
	debug     bool
	assumeYes bool
	whitelist []string

	onScanProgress  func(path string, info os.FileInfo)
	onCleanProgress func(path string, info os.FileInfo)
)

var CleanCmd = &cobra.Command{
	Use:   "clean",
	Short: "Deep clean system temp files, prefetch, browser caches, and error logs",
	Long: `Scan and safely remove system and application leftovers including:
  - User and system temp files
  - Windows Update cache
  - Prefetch folder (requires Administrator privileges)
  - Profile-aware browser caches (Chrome, Edge, Firefox, Brave)
  - Thumbnail cache database files
  - Windows Error Reporting diagnostic dumps
  - Recycle Bin
  - System DNS Cache flush`,
	Run: executeClean,
}

func init() {
	CleanCmd.Flags().BoolVarP(&dryRun, "dry-run", "d", false, "Preview what files will be deleted and sizes without actual modifications")
	CleanCmd.Flags().BoolVar(&debug, "debug", false, "Print verbose execution logs and scanned file targets")
	CleanCmd.Flags().BoolVarP(&assumeYes, "yes", "y", false, "Skip confirmation and delete in non-interactive (piped/--debug) mode")
	CleanCmd.Flags().StringSliceVarP(&whitelist, "whitelist", "w", []string{}, "Categories to protect/skip from scanning (e.g. browsers,prefetch)")
}

// firefoxCachePaths returns the cache subdirectories inside each local Firefox
// profile. Firefox keeps caches under %LOCALAPPDATA%\Mozilla\Firefox\Profiles\<p>\,
// while the Roaming profile root holds bookmarks, passwords, and history —
// deleting a profile root would destroy user data, so only cache subdirs qualify.
func firefoxCachePaths(localAppData string) []string {
	profilesRoot := filepath.Join(localAppData, `Mozilla`, `Firefox`, `Profiles`)
	entries, err := os.ReadDir(profilesRoot)
	if err != nil {
		return nil
	}
	var paths []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		paths = append(paths,
			filepath.Join(profilesRoot, e.Name(), "cache2"),
			filepath.Join(profilesRoot, e.Name(), "startupCache"),
		)
	}
	return paths
}

func getCategories() []CleanCategory {
	localAppData := fs.ResolveEnvPath("%LOCALAPPDATA%")
	appData := fs.ResolveEnvPath("%APPDATA%")
	winDir := secureWindowsDir() // kernel-resolved: follows Windows off C: and ignores a spoofed %WINDIR%

	browserPaths := []string{
		filepath.Join(localAppData, `Google\Chrome\User Data\Default\Cache\Cache_Data`),
		filepath.Join(localAppData, `Google\Chrome\User Data\Default\Code Cache`),
		filepath.Join(localAppData, `Microsoft\Edge\User Data\Default\Cache\Cache_Data`),
		filepath.Join(localAppData, `Microsoft\Edge\User Data\Default\Code Cache`),
		filepath.Join(localAppData, `BraveSoftware\Brave-Browser\User Data\Default\Cache\Cache_Data`),
		filepath.Join(localAppData, `BraveSoftware\Brave-Browser\User Data\Default\Code Cache`),
	}
	browserPaths = append(browserPaths, firefoxCachePaths(localAppData)...)

	return []CleanCategory{
		{
			ID:          "temp",
			Name:        "Temporary Files",
			Description: "System temp folders and active user temp caches",
			Paths: []string{
				fs.ResolveEnvPath("%TEMP%"),
				winDir + `\Temp`,
			},
		},
		{
			ID:          "update",
			Name:        "Windows Update Cache",
			Description: "Downloaded system installer leftovers",
			Paths: []string{
				winDir + `\SoftwareDistribution\Download`,
			},
		},
		{
			ID:          "prefetch",
			Name:        "Prefetch Files",
			Description: "Windows pre-cached applications loading data",
			Paths: []string{
				winDir + `\Prefetch`,
			},
		},
		{
			ID:          "browsers",
			Name:        "Browser Caches",
			Description: "Caches from Chrome, Edge, Firefox, and Brave",
			Paths:       browserPaths,
		},
		{
			ID:          "thumbs",
			Name:        "Thumbnail Cache",
			Description: "Windows explorer thumbnail databases",
			Paths: []string{
				filepath.Join(localAppData, `Microsoft\Windows\Explorer`),
			},
			Pattern:   "thumbcache_*.db",
			FilesOnly: true,
		},
		{
			ID:          "wer",
			Name:        "Windows Error Reports",
			Description: "Application crash logs and diagnostic dumps",
			Paths: []string{
				fs.ResolveEnvPath("%PROGRAMDATA%") + `\Microsoft\Windows\WER`,
				filepath.Join(localAppData, `Microsoft\Windows\WER`),
			},
		},
		{
			ID:          "recycle",
			Name:        "Recycle Bin",
			Description: "Files currently stored in the system Recycle Bin",
			CustomScan:  scanAndEmptyRecycleBin,
		},
		{
			ID:          "dns",
			Name:        "DNS Cache Flush",
			Description: "Clear local system DNS queries catalog",
			CustomScan:  flushDNSCache,
		},
		// ── Developer caches ──────────────────────────────────────────────
		{
			ID:          "npm",
			Name:        "npm Cache",
			Description: "Node.js npm package manager cache",
			Paths: []string{
				filepath.Join(appData, `npm-cache`),
				filepath.Join(localAppData, `npm-cache`),
			},
		},
		{
			ID:          "pnpm",
			Name:        "pnpm Store Cache",
			Description: "pnpm content-addressable store cache",
			Paths: []string{
				filepath.Join(localAppData, `pnpm\store`),
			},
		},
		{
			ID:          "yarn",
			Name:        "Yarn Cache",
			Description: "Yarn package manager local cache",
			Paths: []string{
				filepath.Join(localAppData, `Yarn\Cache`),
			},
		},
		{
			ID:          "bun",
			Name:        "Bun Cache",
			Description: "Bun JavaScript runtime package cache",
			Paths: []string{
				fs.ResolveEnvPath(`%USERPROFILE%\.bun\cache`),
			},
		},
		{
			ID:          "pip",
			Name:        "pip Cache",
			Description: "Python pip package installer cache",
			Paths: []string{
				filepath.Join(localAppData, `pip\Cache`),
			},
		},
		{
			ID:          "cargo",
			Name:        "Cargo Registry Cache",
			Description: "Rust/Cargo registry download cache",
			Paths: []string{
				fs.ResolveEnvPath(`%USERPROFILE%\.cargo\registry\cache`),
			},
		},
		{
			ID:          "gradle",
			Name:        "Gradle Build Cache",
			Description: "Gradle build system and dependency cache",
			Paths: []string{
				fs.ResolveEnvPath(`%USERPROFILE%\.gradle\caches`),
			},
		},
		{
			ID:          "nuget",
			Name:        "NuGet Package Cache",
			Description: ".NET NuGet package manager local cache",
			Paths: []string{
				fs.ResolveEnvPath(`%USERPROFILE%\.nuget\packages`),
			},
		},
		{
			ID:          "docker",
			Name:        "Docker Temp Files",
			Description: "Docker Desktop temporary build files",
			Paths: []string{
				// SAFETY: never include Docker\wsl\data here — it holds ext4.vhdx,
				// the WSL2 VM disk containing all Docker images, containers, and volumes.
				filepath.Join(localAppData, `Docker\tmp`),
				filepath.Join(localAppData, `Docker\log`),
			},
		},
		{
			ID:          "vscode",
			Name:        "VSCode Cache",
			Description: "Visual Studio Code extension and language server caches",
			Paths: []string{
				filepath.Join(appData, `Code\CachedData`),
				filepath.Join(appData, `Code\CachedExtensionVSIXs`),
				filepath.Join(appData, `Code\logs`),
				filepath.Join(appData, `Code\Cache`),
			},
		},
		// ── GPU & system ──────────────────────────────────────────────────
		{
			ID:          "gpu_shader",
			Name:        "GPU Shader Cache",
			Description: "DirectX and NVIDIA compiled GPU shader cache",
			Paths: []string{
				filepath.Join(localAppData, `D3DSCache`),
				filepath.Join(localAppData, `NVIDIA\DXCache`),
				filepath.Join(localAppData, `NVIDIA\GLCache`),
			},
		},
		{
			ID:          "delivery_opt",
			Name:        "Delivery Optimization",
			Description: "Windows peer-to-peer update delivery cache",
			Paths: []string{
				winDir + `\SoftwareDistribution\DeliveryOptimization`,
			},
		},
		{
			ID:          "crash_dumps",
			Name:        "Crash Dumps & Logs",
			Description: "Application crash minidumps and diagnostic logs",
			Paths: []string{
				// WER paths live in the "wer" category; listing them here too
				// double-counted their size in scan totals.
				filepath.Join(localAppData, `CrashDumps`),
			},
		},
		// ── Application caches ──────────────────────────────────────────
		{
			ID:          "opera",
			Name:        "Opera Browser Cache",
			Description: "Opera browser rendering and code cache",
			Paths: []string{
				filepath.Join(appData, `Opera Software\Opera Stable\Cache\Cache_Data`),
				filepath.Join(appData, `Opera Software\Opera Stable\Code Cache`),
				filepath.Join(appData, `Opera Software\Opera GX Stable\Cache\Cache_Data`),
			},
		},
		{
			ID:          "discord",
			Name:        "Discord Cache",
			Description: "Discord application rendering and code cache",
			Paths: []string{
				filepath.Join(appData, `discord\Cache\Cache_Data`),
				filepath.Join(appData, `discord\Code Cache`),
				filepath.Join(appData, `discord\GPUCache`),
			},
		},
		{
			ID:          "spotify",
			Name:        "Spotify Cache",
			Description: "Spotify offline data and local storage cache",
			Paths: []string{
				filepath.Join(localAppData, `Spotify\Storage`),
				filepath.Join(localAppData, `Spotify\Data`),
			},
		},
		{
			ID:          "slack",
			Name:        "Slack Cache",
			Description: "Slack desktop application cache data",
			Paths: []string{
				filepath.Join(appData, `Slack\Cache\Cache_Data`),
				filepath.Join(appData, `Slack\Code Cache`),
				filepath.Join(appData, `Slack\GPUCache`),
			},
		},
		{
			ID:          "teams",
			Name:        "Microsoft Teams Cache",
			Description: "Teams application rendering and blob storage cache",
			Paths: []string{
				filepath.Join(localAppData, `Packages\MSTeams_8wekyb3d8bbwe\LocalCache`),
				filepath.Join(appData, `Microsoft\Teams\Cache`),
				filepath.Join(appData, `Microsoft\Teams\Code Cache`),
				filepath.Join(appData, `Microsoft\Teams\blob_storage`),
				filepath.Join(appData, `Microsoft\Teams\GPUCache`),
			},
		},
		{
			ID:          "steam",
			Name:        "Steam Cache",
			Description: "Steam client HTML and download cache",
			Paths: []string{
				filepath.Join(localAppData, `Steam\htmlcache`),
				filepath.Join(localAppData, `Steam\appcache`),
			},
		},
		{
			ID:          "epic",
			Name:        "Epic Games Cache",
			Description: "Epic Games Launcher web cache and saved data",
			Paths: []string{
				filepath.Join(localAppData, `EpicGamesLauncher\Saved\webcache`),
				filepath.Join(localAppData, `EpicGamesLauncher\Saved\webcache_4147`),
			},
		},
		{
			ID:          "jetbrains",
			Name:        "JetBrains IDE Caches",
			Description: "IntelliJ, WebStorm, PyCharm, GoLand local caches",
			CustomScan:  scanJetBrainsCaches,
		},
		{
			ID:          "adobe",
			Name:        "Adobe Cache Files",
			Description: "Adobe Creative Cloud media and rendering cache",
			Paths: []string{
				filepath.Join(appData, `Adobe\Common\Media Cache Files`),
				filepath.Join(appData, `Adobe\Common\Media Cache`),
				filepath.Join(localAppData, `Adobe\AcroCef\DC\Acrobat\Cache`),
			},
		},
		// ── Additional system targets ──────────────────────────────────
		{
			ID:          "memdumps",
			Name:        "Memory Dump Files",
			Description: "System crash memory dumps and minidumps",
			Paths: []string{
				winDir + `\Minidump`,
			},
		},
		{
			ID:          "fontcache",
			Name:        "Font Cache",
			Description: "Windows font rendering cache database",
			Paths: []string{
				filepath.Join(localAppData, `Microsoft\FontCache`),
			},
		},
		{
			ID:          "logfiles",
			Name:        "System Log Files",
			Description: "CBS, DISM, and setup diagnostic log files",
			Paths: []string{
				winDir + `\Logs\CBS`,
				winDir + `\Logs\DISM`,
			},
			Pattern:   "*.log",
			FilesOnly: true,
		},
		{
			ID:   "recent",
			Name: "Recent Items Cache",
			// Only the Recent shortcuts: the jump-list files next to them also
			// hold the user's pinned items, so they are never targeted.
			Description: "Windows Explorer recent file shortcuts",
			Paths: []string{
				filepath.Join(appData, `Microsoft\Windows\Recent`),
			},
			Pattern:   "*.lnk",
			FilesOnly: true,
		},
	}
}

// cleanGroup is one stage of the clean scan.
type cleanGroup struct {
	name  string
	icon  string
	catID map[string]bool
}

// cleanGroups orders the categories for both the CLI scan and the clean TUI.
// Every getCategories ID must be in exactly one group (TestCleanGroupsCoverEveryCategory).
var cleanGroups = []cleanGroup{
	{name: "System Core", icon: "⚙", catID: map[string]bool{
		"temp": true, "update": true, "prefetch": true, "wer": true,
		"recycle": true, "dns": true, "delivery_opt": true, "memdumps": true,
		"logfiles": true, "recent": true, "fontcache": true,
	}},
	{name: "Web Browsers", icon: "🌐", catID: map[string]bool{
		"browsers": true, "opera": true,
	}},
	{name: "Developer Tools", icon: "🛠", catID: map[string]bool{
		"npm": true, "pnpm": true, "yarn": true, "bun": true, "pip": true,
		"cargo": true, "gradle": true, "nuget": true, "docker": true,
		"vscode": true, "jetbrains": true,
	}},
	{name: "Applications", icon: "📦", catID: map[string]bool{
		"discord": true, "spotify": true, "slack": true, "teams": true,
		"steam": true, "epic": true, "adobe": true,
	}},
	{name: "GPU & Graphics", icon: "🎮", catID: map[string]bool{
		"gpu_shader": true, "thumbs": true,
	}},
	{name: "Crash & Diagnostic Data", icon: "🔍", catID: map[string]bool{
		"crash_dumps": true,
	}},
}

// groupedCategories orders cats as the CLI scan shows them: by group, then in
// getCategories order.
func groupedCategories(cats []CleanCategory) []CleanCategory {
	var out []CleanCategory
	for _, g := range cleanGroups {
		for _, c := range cats {
			if g.catID[c.ID] {
				out = append(out, c)
			}
		}
	}
	return out
}

// runCategory scans (scanOnly) or cleans one category, through its custom
// handler or the shared directory engine. Callers handle the whitelist,
// adminOnlyBlocked and progress reporting.
func runCategory(cat CleanCategory, scanOnly bool) (int64, int, error) {
	if cat.CustomScan != nil {
		return cat.CustomScan(scanOnly, debug)
	}
	if scanOnly {
		return scanDirCategory(cat)
	}
	return cleanDirCategory(cat)
}

// adminOnlyBlocked reports a category this process cannot touch without admin.
func adminOnlyBlocked(cat CleanCategory) bool {
	return cat.ID == "prefetch" && !elevation.IsAdmin()
}

func executeClean(cmd *cobra.Command, args []string) {
	// --yes must bypass the interactive TUI: its whole point is unattended
	// cleaning, and the TUI never consults it.
	if isPiped() || debug || assumeYes {
		executeCleanCLI(cmd, args)
		return
	}
	runCleanTUI(dryRun)
}

func executeCleanCLI(cmd *cobra.Command, args []string) {
	startTime := time.Now()

	categories := getCategories()
	totalCategories := len(categories)

	whitelistMap, unknownIDs := whitelistSet(whitelist)
	for _, id := range unknownIDs {
		fmt.Fprintf(os.Stderr, "  warning: --whitelist %q matches no category, so it protects nothing\n", id)
	}

	// Spinners only make sense on a live terminal; carriage-return animation
	// frames would corrupt piped or redirected output.
	useSpinner := !debug && !isPiped()

	groups := cleanGroups

	// ── Full System Scan Header ──────────────────────────────────────────
	modeLabel := "DEEP CLEAN"
	if dryRun {
		modeLabel = "DRY RUN SCAN"
	}

	fmt.Println()
	fmt.Println("  " + styleDivider.Render(strings.Repeat("═", 66)))
	fmt.Printf("  %s  %s\n",
		styleAccent.Render("DUSTER"),
		styleValue.Render("Full System "+modeLabel),
	)
	fmt.Printf("  %s\n",
		styleMuted.Render("Scanning entire Windows environment for reclaimable space"),
	)
	fmt.Println("  " + styleDivider.Render(strings.Repeat("═", 66)))
	fmt.Println()

	// Scan scope summary
	fmt.Printf("  %s  %s categories across %s scan groups\n",
		styleAccent.Render("▸ Scope:"),
		styleValue.Render(fmt.Sprintf("%d", totalCategories)),
		styleValue.Render(fmt.Sprintf("%d", len(groups))),
	)

	scanLocations := []string{
		"C:\\Windows\\Temp", "C:\\Windows\\SoftwareDistribution",
		"%LOCALAPPDATA%", "%APPDATA%", "%TEMP%",
		"%USERPROFILE%", "C:\\$Recycle.Bin",
	}
	fmt.Printf("  %s  %s\n",
		styleAccent.Render("▸ Targets:"),
		styleMuted.Render(strings.Join(scanLocations, ", ")),
	)
	fmt.Println()

	type resultRow struct {
		name      string
		sizeText  string
		fileCount int
		status    string // "ok", "skipped", "noaccess", "adminonly"
		size      int64
	}
	var rows []resultRow
	var totalSize int64
	var totalFiles int
	completedCategories := 0

	// ── Scan phase ────────────────────────────────────────────────────────
	for gi, group := range groups {
		// Collect categories that belong to this group in the original order
		var groupCats []CleanCategory
		for _, cat := range categories {
			if group.catID[cat.ID] {
				groupCats = append(groupCats, cat)
			}
		}
		if len(groupCats) == 0 {
			continue
		}

		// Print stage header
		stageNum := gi + 1
		fmt.Printf("  %s %s %s %s\n",
			styleAccent.Render(fmt.Sprintf("━━━ STAGE %d/%d ━━━", stageNum, len(groups))),
			styleMuted.Render(group.icon),
			styleValue.Render(group.name),
			styleMuted.Render(strings.Repeat("━", 40-len(group.name))),
		)
		fmt.Println()

		var stageSize int64
		var stageFiles int

		for _, cat := range groupCats {
			completedCategories++
			progressPct := float64(completedCategories) / float64(totalCategories) * 100.0

			if whitelistMap[cat.ID] {
				rows = append(rows, resultRow{name: cat.Name, sizeText: "protected", status: "skipped"})
				fmt.Printf("  %s  %s  %s\n",
					styleMuted.Render("○"),
					styleLabel.Render(padRight(cat.Name, 32)),
					styleWarning.Render("skipped"),
				)
				continue
			}
			if adminOnlyBlocked(cat) {
				rows = append(rows, resultRow{name: cat.Name, sizeText: "admin required", status: "adminonly"})
				fmt.Printf("  %s  %s  %s\n",
					styleMuted.Render("○"),
					styleLabel.Render(padRight(cat.Name, 32)),
					styleWarning.Render("admin only"),
				)
				continue
			}

			var spinner *cliSpinner
			if useSpinner {
				spinner = newCliSpinner("Scan", cat.Name)
				onScanProgress = func(path string, info os.FileInfo) {
					spinner.updateProgress(path, info.Size())
				}
				spinner.start()
			}

			size, files, err := runCategory(cat, true)

			if useSpinner {
				spinner.stop()
				onScanProgress = nil
			}

			if err != nil {
				if debug {
					fmt.Printf("  [debug] scan %s: %v\n", cat.Name, err)
				}
				rows = append(rows, resultRow{name: cat.Name, sizeText: "no access", status: "noaccess"})
				fmt.Printf("  %s  %s  %s\n",
					styleDanger.Render("✗"),
					styleLabel.Render(padRight(cat.Name, 32)),
					styleDanger.Render("no access"),
				)
				continue
			}

			totalSize += size
			totalFiles += files
			stageSize += size
			stageFiles += files
			rows = append(rows, resultRow{name: cat.Name, sizeText: formatBytes(size), fileCount: files, status: "ok", size: size})

			if files == 0 && size == 0 {
				fmt.Printf("  %s  %s  %s\n",
					styleMuted.Render("·"),
					styleLabel.Render(padRight(cat.Name, 32)),
					styleMuted.Render("clean"),
				)
			} else {
				fmt.Printf("  %s  %s  %s  %s\n",
					styleSuccess.Render("✓"),
					styleLabel.Render(padRight(cat.Name, 32)),
					styleAccent.Render(fmt.Sprintf("%10s", formatBytes(size))),
					styleMuted.Render(fmt.Sprintf("(%s files)", formatInt(files))),
				)
			}

			// Show running progress after each category
			if useSpinner && completedCategories%5 == 0 {
				fmt.Printf("  %s  %s %s scanned so far  %s\n",
					styleMuted.Render("  "),
					styleMuted.Render("↳"),
					styleAccent.Render(formatBytes(totalSize)),
					styleMuted.Render(fmt.Sprintf("[%d%%]", int(progressPct))),
				)
			}
		}

		// Stage summary
		if stageSize > 0 || stageFiles > 0 {
			fmt.Printf("  %s  %s: %s in %s files\n",
				styleMuted.Render("  "),
				styleMuted.Render("Stage total"),
				styleAccent.Render(formatBytes(stageSize)),
				styleAccent.Render(formatInt(stageFiles)),
			)
		}
		fmt.Println()
	}

	// ── Scan complete summary line ─────────────────────────────────────
	scanDuration := time.Since(startTime)
	fmt.Println("  " + styleDivider.Render(strings.Repeat("─", 66)))
	fmt.Printf("  %s  %s reclaimable across %s files in %s categories\n",
		styleSuccess.Render("SCAN COMPLETE"),
		styleAccent.Render(formatBytes(totalSize)),
		styleAccent.Render(formatInt(totalFiles)),
		styleAccent.Render(fmt.Sprintf("%d", totalCategories)),
	)
	fmt.Printf("  %s  Scan completed in %s\n",
		styleMuted.Render("  "),
		styleAccent.Render(fmt.Sprintf("%.1fs", scanDuration.Seconds())),
	)
	fmt.Println("  " + styleDivider.Render(strings.Repeat("─", 66)))
	fmt.Println()

	// ── Dry run path ──────────────────────────────────────────────────────
	// Non-interactive runs (piped stdout or --debug) never show the TUI's
	// Enter-to-confirm gate, so require an explicit --yes before deleting.
	// Without it, fall back to a preview so `du clean | tee log` cannot wipe
	// 34 categories silently.
	if dryRun || !assumeYes {
		if !dryRun {
			fmt.Printf("  %s  Preview only — pass %s to actually delete.\n\n",
				styleWarning.Render("!"),
				styleAccent.Render("--yes"),
			)
		}
		freeNow := getDiskFreeBytes(os.TempDir())
		printCleanupBanner(true, totalSize, freeNow, totalFiles, len(rows))
		return
	}

	// ── Delete phase ──────────────────────────────────────────────────────
	fmt.Printf("  %s\n",
		styleAccent.Render("⚡ RECLAIMING SYSTEM SPACE..."),
	)
	fmt.Println()

	var freedSize int64
	var freedFiles int
	failedCats := 0
	cleanStart := time.Now()

	for _, cat := range categories {
		if whitelistMap[cat.ID] {
			continue
		}

		// Mirror the scan phase: prefetch needs admin, so don't attempt it.
		if adminOnlyBlocked(cat) {
			continue
		}

		var spinner *cliSpinner
		if useSpinner {
			spinner = newCliSpinner("Clean", cat.Name)
			onCleanProgress = func(path string, info os.FileInfo) {
				spinner.updateProgress(path, info.Size())
			}
			spinner.start()
		}

		sizeFreed, filesFreed, err := runCategory(cat, false)

		if useSpinner {
			spinner.stop()
			onCleanProgress = nil
		}

		// Partial frees count even when a category reports failures.
		freedSize += sizeFreed
		freedFiles += filesFreed
		if err != nil {
			failedCats++
			fmt.Printf("  %s  %s  %s\n",
				styleDanger.Render("✗"),
				styleLabel.Render(padRight(cat.Name, 32)),
				styleMuted.Render(err.Error()),
			)
		}
		if sizeFreed > 0 {
			fmt.Printf("  %s  %s  %s freed  %s\n",
				styleSuccess.Render("✓"),
				styleLabel.Render(padRight(cat.Name, 32)),
				styleAccent.Render(formatBytes(sizeFreed)),
				styleMuted.Render(fmt.Sprintf("(%s files)", formatInt(filesFreed))),
			)
		}
	}

	if failedCats > 0 {
		fmt.Printf("\n  %s %d category(ies) could not be fully cleaned; see the ✗ lines above.\n",
			styleWarning.Render("⚠"), failedCats)
	}

	cleanDuration := time.Since(cleanStart)
	totalDuration := time.Since(startTime)

	// ── Final completion banner ───────────────────────────────────────
	fmt.Println()
	fmt.Println("  " + styleDivider.Render(strings.Repeat("═", 66)))
	fmt.Println("  " + styleSuccess.Render("✓ FULL SYSTEM CLEANUP COMPLETE"))
	fmt.Println("  " + styleDivider.Render(strings.Repeat("─", 66)))

	freeNow := getDiskFreeBytes(os.TempDir())

	fmt.Printf("  %s  %s  %s  %s\n",
		styleLabel.Render("Space freed:"),
		styleAccent.Render(formatBytes(freedSize)),
		styleMuted.Render("│"),
		styleLabel.Render("Free now: ")+styleValue.Render(formatBytes(freeNow)),
	)
	fmt.Printf("  %s  %s  %s  %s\n",
		styleLabel.Render("Files cleaned:"),
		styleAccent.Render(formatInt(freedFiles)),
		styleMuted.Render("│"),
		styleLabel.Render("Categories: ")+styleAccent.Render(fmt.Sprintf("%d", len(rows))),
	)
	fmt.Printf("  %s  %s  %s  %s\n",
		styleLabel.Render("Scan time:"),
		styleAccent.Render(fmt.Sprintf("%.1fs", scanDuration.Seconds())),
		styleMuted.Render("│"),
		styleLabel.Render("Clean time: ")+styleAccent.Render(fmt.Sprintf("%.1fs", cleanDuration.Seconds())),
	)
	fmt.Printf("  %s  %s\n",
		styleLabel.Render("Total time:"),
		styleAccent.Render(fmt.Sprintf("%.1fs", totalDuration.Seconds())),
	)

	// Show visual recovery bar
	if totalSize > 0 {
		recoverPct := float64(freedSize) / float64(totalSize) * 100.0
		fmt.Println()
		fmt.Printf("  %s  %s %s\n",
			styleLabel.Render("Recovery:"),
			progressBar(recoverPct, 30),
			styleAccent.Render(fmt.Sprintf("%.0f%% of detected junk removed", recoverPct)),
		)
	}

	if equiv := spaceEquivalent(freedSize); equiv != "" {
		fmt.Println("  " + styleSub.Render(equiv))
	}

	fmt.Println("  " + styleDivider.Render(strings.Repeat("═", 66)))
	fmt.Println()
}

func scanDirCategory(cat CleanCategory) (int64, int, error) {
	var totalSize int64
	var fileCount int

	for _, root := range cat.Paths {
		if !fs.IsValidPath(root) {
			continue
		}

		if skipCategoryRoot(root) {
			continue
		}

		err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return nil // Skip items with access issues gracefully
			}

			if !d.IsDir() {
				// Match pattern if configured (e.g. thumbcache_*.db)
				if cat.Pattern != "" {
					matched, globErr := filepath.Match(cat.Pattern, d.Name())
					if globErr != nil || !matched {
						return nil
					}
				}

				info, err := d.Info()
				if err != nil {
					return nil
				}
				// Skip offline OneDrive placeholders to prevent forced network
				// downloads; the directory listing already carries the attributes.
				if fs.IsOfflineInfo(info) {
					if debug {
						fmt.Printf("[Debug] Skipping offline OneDrive placeholder: %s\n", path)
					}
					return nil
				}

				if debug {
					fmt.Printf("[Debug] Found cache target: %s (%s)\n", path, formatBytes(info.Size()))
				}
				if onScanProgress != nil {
					onScanProgress(path, info)
				}
				totalSize += info.Size()
				fileCount++
			}
			return nil
		})
		if err != nil {
			return 0, 0, err
		}
	}

	return totalSize, fileCount, nil
}

func cleanDirCategory(cat CleanCategory) (int64, int, error) {
	var sizeFreed int64
	var filesFreed int
	failures := 0
	var firstErr error
	fail := func(err error) {
		failures++
		if firstErr == nil {
			firstErr = err
		}
	}

	for _, root := range cat.Paths {
		// A long-form root keeps "~" out of every child path, so the per-file
		// IsValidPath below skips its GetLongPathNameW disk lookup.
		root = fs.LongPath(root)
		if !fs.IsValidPath(root) || skipCategoryRoot(root) {
			continue
		}

		if cat.FilesOnly || cat.Pattern != "" {
			// Selectively delete matching files in folders rather than wiping directory root
			err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
				if err != nil {
					return nil
				}
				if !d.IsDir() {
					if cat.Pattern != "" {
						matched, globErr := filepath.Match(cat.Pattern, d.Name())
						if globErr != nil || !matched {
							return nil
						}
					}

					info, err := d.Info()
					// Skip offline OneDrive placeholders to prevent forced network downloads
					if err != nil || fs.IsOfflineInfo(info) {
						return nil
					}

					// Verify target safety check
					if fs.IsValidPath(path) {
						fileSize := info.Size()
						if onCleanProgress != nil {
							onCleanProgress(path, info)
						}
						if err := removeFileSafe(path); err == nil {
							sizeFreed += fileSize
							filesFreed++
						} else {
							fail(err)
						}
					}
				}
				return nil
			})
			if err != nil {
				return sizeFreed, filesFreed, err
			}
		} else {
			// Walk and delete items inside but keep top level folders when possible
			dirEntries, err := os.ReadDir(root)
			// An unlistable root (C:\Windows\Temp for a standard user) was never
			// attempted: skip it as the scan does rather than report a failure.
			if err != nil && !os.IsPermission(err) {
				fail(err)
			}
			for _, entry := range dirEntries {
				fullPath := filepath.Join(root, entry.Name())
				if !fs.IsValidPath(fullPath) {
					continue
				}

				// Calculate recursive size BEFORE deletion for accurate reporting
				var entrySize int64
				var entryFiles int
				if entry.IsDir() {
					_ = filepath.WalkDir(fullPath, func(p string, d os.DirEntry, walkErr error) error {
						if walkErr != nil {
							return nil
						}
						if !d.IsDir() {
							if info, infoErr := d.Info(); infoErr == nil {
								entrySize += info.Size()
								entryFiles++
								if onCleanProgress != nil {
									onCleanProgress(p, info)
								}
							}
						}
						return nil
					})
				} else {
					if info, infoErr := entry.Info(); infoErr == nil {
						entrySize = info.Size()
						entryFiles = 1
						if onCleanProgress != nil {
							onCleanProgress(fullPath, info)
						}
					}
				}

				if err := removeAllSafe(fullPath); err == nil {
					sizeFreed += entrySize
					filesFreed += entryFiles
				} else {
					fail(err)
					// Credit what was removed before the failure (e.g. one
					// locked file inside a large cache directory).
					if entry.IsDir() {
						if left := calculateDirSize(fullPath); left < entrySize {
							sizeFreed += entrySize - left
						}
					}
				}
			}
		}
	}

	// Log the destructive operation with the real outcome
	logging.LogDestructiveOperation("clean", "purge", cat.Name, sizeFreed, failures == 0)

	if failures > 0 {
		return sizeFreed, filesFreed, fmt.Errorf("%d item(s) could not be deleted: %w", failures, firstErr)
	}
	return sizeFreed, filesFreed, nil
}

// skipCategoryRoot reports whether a category root must not be walked: it is
// missing, unreadable, or a symlink/junction. os.Stat and os.ReadDir follow
// links, so a junction planted at a cache path would get its target emptied.
func skipCategoryRoot(root string) bool {
	info, err := os.Lstat(root)
	return err != nil || info.Mode()&(os.ModeSymlink|os.ModeIrregular) != 0
}

func scanAndEmptyRecycleBin(dryRunOnly bool, debug bool) (int64, int, error) {
	size, count, err := queryRecycleBinNative()
	if err != nil {
		// Fallback walk only when the WinAPI itself failed (or on non-Windows
		// test hosts). A successful "0 items" answer must NOT trigger it: the
		// walk counts $I metadata and desktop.ini as phantom reclaimable junk.
		recycleBinPath := secureWindowsDir()[:2] + `\$Recycle.Bin` // the system drive, wherever Windows is
		if fs.IsValidPath(recycleBinPath) {
			_ = filepath.WalkDir(recycleBinPath, func(path string, d os.DirEntry, walkErr error) error {
				if walkErr != nil {
					return nil
				}
				if !d.IsDir() {
					info, err := d.Info()
					if err == nil {
						if onScanProgress != nil {
							onScanProgress(path, info)
						}
						size += info.Size()
						count++
					}
				}
				return nil
			})
		}
	}

	if dryRunOnly {
		return size, int(count), nil
	}

	if err == nil && size == 0 && count == 0 {
		return 0, 0, nil // already empty — nothing to do
	}

	err = emptyRecycleBinNative()
	return size, int(count), err
}

func flushDNSCache(dryRunOnly bool, debug bool) (int64, int, error) {
	if dryRunOnly {
		return 0, 1, nil // 1 task pending
	}

	// Runs flushdns using Windows native executable
	cmd := exec.Command(systemExecutable("ipconfig.exe"), "/flushdns")
	err := cmd.Run()
	if err != nil {
		return 0, 0, err
	}

	return 0, 1, nil
}

func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.2f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// scanJetBrainsCaches cleans each JetBrains product's caches, index and tmp
// folders (%LOCALAPPDATA%\JetBrains\<ProductVersion>\...) through the shared
// category engine, so they get its root, link and per-file safety checks and
// its failure reporting.
func scanJetBrainsCaches(dryRun bool, _ bool) (int64, int, error) {
	cat := CleanCategory{Name: "JetBrains IDE Caches", Paths: jetBrainsCachePaths(), FilesOnly: true}
	if dryRun {
		return scanDirCategory(cat)
	}
	return cleanDirCategory(cat)
}

// jetBrainsCachePaths lists the cache folders of every installed JetBrains
// product. A linked JetBrains root or product folder is skipped, never followed.
func jetBrainsCachePaths() []string {
	root := filepath.Join(fs.ResolveEnvPath("%LOCALAPPDATA%"), "JetBrains")
	if skipCategoryRoot(root) {
		return nil
	}
	products, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	var paths []string
	for _, product := range products {
		if !product.IsDir() {
			continue
		}
		for _, sub := range []string{"caches", "index", "tmp"} {
			paths = append(paths, filepath.Join(root, product.Name(), sub))
		}
	}
	return paths
}

type cliSpinner struct {
	mu           sync.Mutex
	active       bool
	frames       []string
	frameIdx     int
	categoryName string
	prefix       string
	filesScanned int64
	sizeScanned  int64
	currentPath  string
	stopChan     chan struct{}
	lastPrinted  string
}

func newCliSpinner(prefix, categoryName string) *cliSpinner {
	return &cliSpinner{
		frames: []string{
			"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏",
		},
		prefix:       prefix,
		categoryName: categoryName,
		stopChan:     make(chan struct{}),
	}
}

func (s *cliSpinner) start() {
	s.active = true
	go func() {
		ticker := time.NewTicker(80 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-s.stopChan:
				return
			case <-ticker.C:
				s.mu.Lock()
				if !s.active {
					s.mu.Unlock()
					return
				}
				frame := s.frames[s.frameIdx]
				s.frameIdx = (s.frameIdx + 1) % len(s.frames)

				// Colorize spinner frame using Lipgloss
				spinnerCol := styleAccent.Render(frame)

				// Format dynamic feedback text
				nameW := 32
				paddedName := padRight(s.categoryName, nameW)

				var msg string
				if s.prefix == "Scan" {
					// Scanning: show file count and a shortened path if available
					pathStr := ""
					if s.currentPath != "" {
						pathStr = truncateString(filepath.Base(s.currentPath), 25)
						pathStr = styleMuted.Render(" -> " + pathStr)
					}
					msg = fmt.Sprintf("\r  %s  %s  %s  %s%s",
						spinnerCol,
						styleLabel.Render(paddedName),
						styleAccent.Render(fmt.Sprintf("%10s", formatBytes(s.sizeScanned))),
						styleMuted.Render(fmt.Sprintf("(%s files)", formatInt(int(s.filesScanned)))),
						pathStr,
					)
				} else {
					// Cleaning: show active progress
					msg = fmt.Sprintf("\r  %s  %s  %s",
						spinnerCol,
						styleLabel.Render(paddedName),
						styleMuted.Render(fmt.Sprintf("purging %s files...", formatInt(int(s.filesScanned)))),
					)
				}

				// Clear previous line character length if shorter, to avoid ghost characters
				clearLen := len(s.lastPrinted) - len(msg)
				if clearLen > 0 {
					msg += strings.Repeat(" ", clearLen)
				}
				s.lastPrinted = msg

				fmt.Print(msg)
				s.mu.Unlock()
			}
		}
	}()
}

func (s *cliSpinner) stop() {
	s.mu.Lock()
	if !s.active {
		s.mu.Unlock()
		return
	}
	s.active = false
	close(s.stopChan)
	s.mu.Unlock()

	// Clear the spinner line completely
	if s.lastPrinted != "" {
		fmt.Printf("\r%s\r", strings.Repeat(" ", len(s.lastPrinted)+5))
	}
}

func (s *cliSpinner) updateProgress(path string, size int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.filesScanned++
	s.sizeScanned += size
	s.currentPath = path
}
