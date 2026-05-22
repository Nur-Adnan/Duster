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
	CleanCmd.Flags().StringSliceVarP(&whitelist, "whitelist", "w", []string{}, "Categories to protect/skip from scanning (e.g. browsers,prefetch)")
}

func getCategories() []CleanCategory {
	localAppData := fs.ResolveEnvPath("%LOCALAPPDATA%")
	appData := fs.ResolveEnvPath("%APPDATA%")

	return []CleanCategory{
		{
			ID:          "temp",
			Name:        "Temporary Files",
			Description: "System temp folders and active user temp caches",
			Paths: []string{
				fs.ResolveEnvPath("%TEMP%"),
				`C:\Windows\Temp`,
			},
		},
		{
			ID:          "update",
			Name:        "Windows Update Cache",
			Description: "Downloaded system installer leftovers",
			Paths: []string{
				`C:\Windows\SoftwareDistribution\Download`,
			},
		},
		{
			ID:          "prefetch",
			Name:        "Prefetch Files",
			Description: "Windows pre-cached applications loading data",
			Paths: []string{
				`C:\Windows\Prefetch`,
			},
		},
		{
			ID:          "browsers",
			Name:        "Browser Caches",
			Description: "Caches from Chrome, Edge, Firefox, and Brave",
			Paths: []string{
				filepath.Join(localAppData, `Google\Chrome\User Data\Default\Cache\Cache_Data`),
				filepath.Join(localAppData, `Google\Chrome\User Data\Default\Code Cache`),
				filepath.Join(localAppData, `Microsoft\Edge\User Data\Default\Cache\Cache_Data`),
				filepath.Join(localAppData, `Microsoft\Edge\User Data\Default\Code Cache`),
				filepath.Join(localAppData, `BraveSoftware\Brave-Browser\User Data\Default\Cache\Cache_Data`),
				filepath.Join(localAppData, `BraveSoftware\Brave-Browser\User Data\Default\Code Cache`),
				filepath.Join(appData, `Mozilla\Firefox\Profiles`), // Custom scanner handles profiles recursion
			},
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
				filepath.Join(localAppData, `yarn\cache`),
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
				filepath.Join(localAppData, `Docker\tmp`),
				filepath.Join(localAppData, `Docker\wsl\data`),
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
				`C:\Windows\SoftwareDistribution\DeliveryOptimization`,
			},
		},
		{
			ID:          "crash_dumps",
			Name:        "Crash Dumps & Logs",
			Description: "Application crash minidumps and diagnostic logs",
			Paths: []string{
				filepath.Join(localAppData, `CrashDumps`),
				filepath.Join(localAppData, `Microsoft\Windows\WER`),
				fs.ResolveEnvPath(`%PROGRAMDATA%\Microsoft\Windows\WER`),
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
				`C:\Windows\Minidump`,
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
				`C:\Windows\Logs\CBS`,
				`C:\Windows\Logs\DISM`,
			},
			Pattern:   "*.log",
			FilesOnly: true,
		},
		{
			ID:          "recent",
			Name:        "Recent Items Cache",
			Description: "Windows Explorer recent file shortcuts and jump list cache",
			Paths: []string{
				filepath.Join(appData, `Microsoft\Windows\Recent`),
				filepath.Join(appData, `Microsoft\Windows\Recent\AutomaticDestinations`),
				filepath.Join(appData, `Microsoft\Windows\Recent\CustomDestinations`),
			},
			Pattern:   "*.lnk",
			FilesOnly: true,
		},
		{
			ID:          "installer_patches",
			Name:        "Installer Patch Cache",
			Description: "Orphaned MSI installer patch cache files",
			Paths: []string{
				filepath.Join(localAppData, `Temp\msohtmlclip`),
			},
		},
	}
}

func executeClean(cmd *cobra.Command, args []string) {
	if isPiped() || debug {
		executeCleanCLI(cmd, args)
		return
	}
	runCleanTUI(dryRun)
}

func executeCleanCLI(cmd *cobra.Command, args []string) {
	// ── Header ──────────────────────────────────────────────────────────
	fmt.Println()
	fmt.Println("  " + styleTitle.Render("Duster — Deep Clean"))
	fmt.Println("  " + styleMuted.Render(strings.Repeat("═", 60)))
	fmt.Println()

	categories := getCategories()
	var totalSize int64
	var totalFiles int

	whitelistMap := make(map[string]bool)
	for _, id := range whitelist {
		whitelistMap[strings.ToLower(strings.TrimSpace(id))] = true
	}

	type resultRow struct {
		name      string
		sizeText  string
		fileCount int
		status    string // "ok", "skipped", "noaccess", "adminonly"
	}
	var rows []resultRow

	// Setup spinner usage detection
	useSpinner := !debug
	if isPiped() {
		stat, err := os.Stdout.Stat()
		if err != nil || stat.Mode().IsRegular() || os.Getenv("TERM") == "" {
			useSpinner = false
		}
	}

	if useSpinner {
		fmt.Println("  " + styleAccent.Render("🔍 [1/2] Scanning cache directories..."))
		fmt.Println()
	}

	// ── Scan phase ────────────────────────────────────────────────────────
	for _, cat := range categories {
		if whitelistMap[cat.ID] {
			rows = append(rows, resultRow{name: cat.Name, sizeText: "protected", status: "skipped"})
			if useSpinner {
				fmt.Printf("  %s  %s  %s\n",
					styleMuted.Render("○"),
					styleLabel.Render(padRight(cat.Name, 32)),
					styleWarning.Render("skipped"),
				)
			}
			continue
		}
		if cat.ID == "prefetch" && !elevation.IsAdmin() {
			rows = append(rows, resultRow{name: cat.Name, sizeText: "admin required", status: "adminonly"})
			if useSpinner {
				fmt.Printf("  %s  %s  %s\n",
					styleMuted.Render("○"),
					styleLabel.Render(padRight(cat.Name, 32)),
					styleWarning.Render("admin only"),
				)
			}
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

		var size int64
		var files int
		var err error
		if cat.CustomScan != nil {
			size, files, err = cat.CustomScan(true, debug)
		} else {
			size, files, err = scanDirCategory(cat)
		}

		if useSpinner {
			spinner.stop()
			onScanProgress = nil
		}

		if err != nil {
			if debug {
				fmt.Printf("  [debug] scan %s: %v\n", cat.Name, err)
			}
			rows = append(rows, resultRow{name: cat.Name, sizeText: "no access", status: "noaccess"})
			if useSpinner {
				fmt.Printf("  %s  %s  %s\n",
					styleDanger.Render("✗"),
					styleLabel.Render(padRight(cat.Name, 32)),
					styleDanger.Render("no access"),
				)
			}
			continue
		}

		totalSize += size
		totalFiles += files
		rows = append(rows, resultRow{name: cat.Name, sizeText: formatBytes(size), fileCount: files, status: "ok"})

		if useSpinner {
			if files == 0 && size == 0 {
				fmt.Printf("  %s  %s  %s\n",
					styleMuted.Render("-"),
					styleLabel.Render(padRight(cat.Name, 32)),
					styleMuted.Render("empty"),
				)
			} else {
				fmt.Printf("  %s  %s  %s  %s\n",
					styleSuccess.Render("✓"),
					styleLabel.Render(padRight(cat.Name, 32)),
					styleAccent.Render(fmt.Sprintf("%10s", formatBytes(size))),
					styleMuted.Render(fmt.Sprintf("(%s files)", formatInt(files))),
				)
			}
		}
	}

	// ── Print scan table (Only if spinner wasn't used, to avoid duplication!) ─
	if !useSpinner {
		const nameW = 32
		const sizeW = 10
		for _, row := range rows {
			name := padRight(row.name, nameW)
			switch row.status {
			case "skipped":
				fmt.Printf("  %s  %s  %s\n",
					styleMuted.Render("○"),
					styleLabel.Render(name),
					styleWarning.Render("skipped"),
				)
			case "adminonly":
				fmt.Printf("  %s  %s  %s\n",
					styleMuted.Render("○"),
					styleLabel.Render(name),
					styleWarning.Render("admin only"),
				)
			case "noaccess":
				fmt.Printf("  %s  %s  %s\n",
					styleDanger.Render("✗"),
					styleLabel.Render(name),
					styleDanger.Render("no access"),
				)
			default: // "ok"
				if row.fileCount == 0 && row.sizeText == "0 B" {
					fmt.Printf("  %s  %s  %s\n",
						styleMuted.Render("-"),
						styleLabel.Render(name),
						styleMuted.Render("empty"),
					)
				} else {
					fmt.Printf("  %s  %s  %s  %s\n",
						styleSuccess.Render("✓"),
						styleLabel.Render(name),
						styleAccent.Render(fmt.Sprintf("%*s", sizeW, row.sizeText)),
						styleMuted.Render(fmt.Sprintf("(%s files)", formatInt(row.fileCount))),
					)
				}
			}
		}
	}

	// ── Dry run path ──────────────────────────────────────────────────────
	if dryRun {
		fmt.Println()
		freeNow := getDiskFreeBytes(os.TempDir())
		printCleanupBanner(true, totalSize, freeNow, totalFiles, len(rows))
		return
	}

	// ── Delete phase ──────────────────────────────────────────────────────
	fmt.Println()
	if useSpinner {
		fmt.Println("  " + styleAccent.Render("⚡ [2/2] Reclaiming system space..."))
		fmt.Println()
	} else {
		fmt.Println("  " + styleMuted.Render("Cleaning..."))
		fmt.Println()
	}

	var freedSize int64
	var freedFiles int
	for _, cat := range categories {
		if whitelistMap[cat.ID] {
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

		var sizeFreed int64
		var filesFreed int
		var err error
		if cat.CustomScan != nil {
			sizeFreed, filesFreed, err = cat.CustomScan(false, debug)
		} else {
			sizeFreed, filesFreed, err = cleanDirCategory(cat)
		}

		if useSpinner {
			spinner.stop()
			onCleanProgress = nil
		}

		if err != nil {
			if debug {
				fmt.Printf("  [debug] clean %s: %v\n", cat.Name, err)
			}
			continue
		}

		freedSize += sizeFreed
		freedFiles += filesFreed
		if sizeFreed > 0 {
			fmt.Printf("  %s  %s  %s freed  %s\n",
				styleSuccess.Render("✓"),
				styleLabel.Render(padRight(cat.Name, 32)),
				styleAccent.Render(formatBytes(sizeFreed)),
				styleMuted.Render(fmt.Sprintf("(%s files)", formatInt(freedFiles))),
			)
		}
	}

	fmt.Println()
	freeNow := getDiskFreeBytes(os.TempDir())
	printCleanupBanner(false, freedSize, freeNow, freedFiles, len(rows))
}

func scanDirCategory(cat CleanCategory) (int64, int, error) {
	var totalSize int64
	var fileCount int

	for _, root := range cat.Paths {
		if !fs.IsValidPath(root) {
			continue
		}

		// Perform physical validation of directory existence
		if _, err := os.Stat(root); os.IsNotExist(err) {
			continue
		}

		err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return nil // Skip items with access issues gracefully
			}

			if !d.IsDir() {
				// Skip offline OneDrive placeholders to prevent forced network downloads
				if fs.IsOfflineFile(path) {
					if debug {
						fmt.Printf("[Debug] Skipping offline OneDrive placeholder: %s\n", path)
					}
					return nil
				}

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

	for _, root := range cat.Paths {
		if !fs.IsValidPath(root) {
			continue
		}

		// Check directory existence
		if _, err := os.Stat(root); os.IsNotExist(err) {
			continue
		}

		if cat.FilesOnly || cat.Pattern != "" {
			// Selectively delete matching files in folders rather than wiping directory root
			err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
				if err != nil {
					return nil
				}
				if !d.IsDir() {
					// Skip offline OneDrive placeholders to prevent forced network downloads
					if fs.IsOfflineFile(path) {
						return nil
					}

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

					// Verify target safety check
					if fs.IsValidPath(path) {
						fileSize := info.Size()
						if onCleanProgress != nil {
							onCleanProgress(path, info)
						}
						if removeFileSafe(path) == nil {
							sizeFreed += fileSize
							filesFreed++
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
			if err == nil {
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

					removeErr := removeAllSafe(fullPath)
					if removeErr == nil {
						sizeFreed += entrySize
						filesFreed += entryFiles
					}
				}
			}
		}
	}

	// Log the destructive operation
	logging.LogDestructiveOperation("clean", "purge", cat.Name, sizeFreed, true)

	return sizeFreed, filesFreed, nil
}

func scanAndEmptyRecycleBin(dryRunOnly bool, debug bool) (int64, int, error) {
	size, count, err := queryRecycleBinNative()
	if err != nil || (size == 0 && count == 0) {
		// Dynamic Fallback in case of non-Windows testing or WinAPI failure
		recycleBinPath := `C:\$Recycle.Bin`
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

	err = emptyRecycleBinNative()
	return size, int(count), err
}

func flushDNSCache(dryRunOnly bool, debug bool) (int64, int, error) {
	if dryRunOnly {
		return 0, 1, nil // 1 task pending
	}

	// Runs flushdns using Windows native executable
	cmd := exec.Command("ipconfig", "/flushdns")
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

// scanJetBrainsCaches dynamically discovers JetBrains IDE cache directories.
// JetBrains products store caches in %LOCALAPPDATA%/JetBrains/<ProductVersion>/caches/
func scanJetBrainsCaches(dryRun bool, debugMode bool) (int64, int, error) {
	jetbrainsRoot := filepath.Join(fs.ResolveEnvPath("%LOCALAPPDATA%"), "JetBrains")
	if _, err := os.Stat(jetbrainsRoot); os.IsNotExist(err) {
		return 0, 0, nil
	}

	var totalSize int64
	var totalFiles int

	products, err := os.ReadDir(jetbrainsRoot)
	if err != nil {
		return 0, 0, nil
	}

	for _, product := range products {
		if !product.IsDir() {
			continue
		}
		cachePaths := []string{
			filepath.Join(jetbrainsRoot, product.Name(), "caches"),
			filepath.Join(jetbrainsRoot, product.Name(), "index"),
			filepath.Join(jetbrainsRoot, product.Name(), "tmp"),
		}

		for _, cachePath := range cachePaths {
			if _, statErr := os.Stat(cachePath); os.IsNotExist(statErr) {
				continue
			}

			_ = filepath.WalkDir(cachePath, func(path string, d os.DirEntry, walkErr error) error {
				if walkErr != nil {
					return nil
				}
				if !d.IsDir() {
					if fs.IsOfflineFile(path) {
						return nil
					}
					info, infoErr := d.Info()
					if infoErr == nil {
						if dryRun {
							if onScanProgress != nil {
								onScanProgress(path, info)
							}
							totalSize += info.Size()
							totalFiles++
						} else {
							fileSize := info.Size()
							if onCleanProgress != nil {
								onCleanProgress(path, info)
							}
							if removeFileSafe(path) == nil {
								totalSize += fileSize
								totalFiles++
							}
						}
					}
				}
				return nil
			})
		}
	}

	return totalSize, totalFiles, nil
}

// scanResult holds the outcome of a single category scan.
type scanResult struct {
	idx       int
	size      int64
	fileCount int
	err       error
	status    string
}

// scanCategoriesConcurrent scans multiple categories in parallel using a bounded worker pool.
// Used during the scan phase to dramatically accelerate category enumeration.
func scanCategoriesConcurrent(categories []CleanCategory, whitelistMap map[string]bool) []scanResult {
	results := make([]scanResult, len(categories))
	var wg sync.WaitGroup

	// Bounded semaphore: limit parallel filesystem walks
	const maxWorkers = 4
	sem := make(chan struct{}, maxWorkers)

	for i, cat := range categories {
		if whitelistMap[cat.ID] {
			results[i] = scanResult{idx: i, status: "skipped"}
			continue
		}
		if cat.ID == "prefetch" && !elevation.IsAdmin() {
			results[i] = scanResult{idx: i, status: "adminonly"}
			continue
		}

		wg.Add(1)
		go func(idx int, c CleanCategory) {
			defer wg.Done()
			sem <- struct{}{}        // acquire
			defer func() { <-sem }() // release

			var size int64
			var files int
			var err error
			if c.CustomScan != nil {
				size, files, err = c.CustomScan(true, false)
			} else {
				size, files, err = scanDirCategory(c)
			}

			if err != nil {
				results[idx] = scanResult{idx: idx, err: err, status: "noaccess"}
			} else {
				results[idx] = scanResult{idx: idx, size: size, fileCount: files, status: "ok"}
			}
		}(i, cat)
	}

	wg.Wait()

	// The scanResult type and results are used internally;
	// callers currently don't use this function yet (wired in Phase 4 follow-up).
	_ = results
	return nil
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
