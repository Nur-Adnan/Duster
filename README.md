<div align="center">
  <h1>🧹 Duster</h1>
  <p><strong>Windows-Native Deep Cleanup & Real-time System Optimization CLI Utility</strong></p>
  <p><em>Terminal-first Windows deep cleaning utility with 35+ cleanup categories, live system monitoring, and developer-focused tooling.</em></p>
</div>

<p align="center">
  <a href="https://github.com/Nur-Adnan/Duster/tags"><img src="https://img.shields.io/github/v/tag/Nur-Adnan/Duster?style=for-the-badge&color=00ADB5" alt="Version"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-blue.svg?style=for-the-badge" alt="License"></a>
  <a href="https://github.com/Nur-Adnan/Duster/actions/workflows/ci.yml"><img src="https://img.shields.io/github/actions/workflow/status/Nur-Adnan/Duster/ci.yml?branch=main&label=CI&style=for-the-badge" alt="CI Status"></a>
  <a href="https://github.com/Nur-Adnan/Duster/actions/workflows/release.yml"><img src="https://img.shields.io/github/actions/workflow/status/Nur-Adnan/Duster/release.yml?label=Release&style=for-the-badge" alt="Release Status"></a>
  <img src="https://img.shields.io/badge/Go-1.25.x-00ADD8?style=for-the-badge&logo=go" alt="Go Version">
  <img src="https://img.shields.io/badge/Platform-Windows-0078D6?style=for-the-badge&logo=windows" alt="Platform">
</p>

---

## 🌟 Introduction

**Duster** (`du`) is an enterprise-grade, memory-efficient, and security-hardened CLI cleaning and optimization tool built specifically for Windows. Utilizing direct Win32 APIs, native registry hooks, and low-level NTFS filesystem properties, Duster offers developer-focused deep cleanup, a visual disk space analyzer, and interactive real-time system performance monitoring—all in a stunning terminal-first interface.

Duster has been completely redesigned to match the absolute pinnacle of polish and usability:
- **Unified Terminal Design Language**: Sleek terminal widgets, compact progress indicators, high-contrast HSL Hues, custom layout styling, and micro-animations built via Charm Bracelet `bubbletea` & `lipgloss`.
- **Developer-First Cleanup Engine**: Instantly targets 20+ different cache categories across system files, web browsers, GPU configurations, and developer package managers.
- **Deep Windows-Native Mechanics**: Bypasses OneDrive offline placeholders, resolves symbolic link loops safely, implements secure child-process bounds, and performs cryptographically signed updates.

---

## ✨ Core Feature Showcase

### 1. Deep System Cleanup (`du clean`)
A multi-threaded scanning engine that safely purges app leftovers, old logs, and diagnostic files.
* **Smart Exclusions**: Built-in whitelist flags to prevent clearing specific caches.
* **OneDrive Shield**: Reads NTFS file attributes directly and automatically skips folders/files marked with `FILE_ATTRIBUTE_OFFLINE` to protect network bandwidth.
* **Safety First**: Supports `--dry-run` to preview deletions, files count, and reclaimed bytes safely.

### 2. Interactive Disk Space Analyzer (`du analyze`)
A high-performance storage explorer that runs directly in your terminal.
* **Dynamic Drills**: Traverse folder hierarchies using standard `hjkl` or arrow keys.
* **Aging Indicators**: Identifies stale files and displays relative size percentage bar widgets.
* **Action Shortcuts**: Instantly launch Windows Explorer (`e`), open/edit files, or safely delete files to the Recycle Bin (`x`).
* **Largest Files Viewer**: Displays a global top 10 list of your drive's absolute space hogs.

### 3. Live System Dashboard (`du status`)
A real-time, two-column system performance monitoring center refreshing every 1 second.
* **Hardware Telemetry**: Accurate gauges for multi-core CPU load, system RAM usage, and active disk queue I/O read/write rates.
* **System Vitality**: Shows weighted OS health scores, host details, motherboard properties, active network throughput, and real-time battery diagnostics.
* **Top Processes List**: Monitored live-feed of the top 5 CPU-consuming background processes.

### 4. Smart Developer Purge (`du purge`)
A lightning-fast crawler built to free hundreds of gigabytes from developer environments.
* **Build Artifact Cleanup**: Finds and purges `node_modules`, `target`, `bin`, `obj`, `.venv`, `.next`, `dist`, and build logs.
* **Scan Paths**: Pass any starting directory or scan your entire workspace recursively.

### 5. Smart Software Uninstaller (`du uninstall`)
Audits installed applications, triggers official silent uninstallation scripts, and performs automatic garbage collection on residual registry keys and AppData folders.

---

## 🛠️ Complete Cleanup Categories (35 Total)

Duster goes far deeper than generic clean tools, dividing files into four specific namespaces:

### 💻 System & Windows Core
1. **Temporary Files (`temp`)**: Wipes active `%TEMP%` and `C:\Windows\Temp` directories.
2. **Windows Update Cache (`update`)**: Clears old update catalog files in `SoftwareDistribution\Download`.
3. **Prefetch Files (`prefetch`)**: Speeds up system cleanups by purging Windows prefetch binaries *(requires admin rights)*.
4. **Thumbnail Database (`thumbs`)**: Sweeps explorer thumbnail databases (`thumbcache_*.db`).
5. **Windows Error Reporting (`wer`)**: Cleans dump reports and diagnostic logs.
6. **Recycle Bin (`recycle`)**: Natively empties standard user Recycle Bins.
7. **DNS Cache Flush (`dns`)**: Flushes dynamic local network DNS names resolver cache.
8. **Delivery Optimization (`delivery_opt`)**: Removes shared peer-to-peer Windows Update cached blocks.
9. **Crash Dumps & Logs (`crash_dumps`)**: Safely removes local minidumps and crash logs.

### 🚀 Developer Environments
10. **npm Cache (`npm`)**: Cleans global node package manager cache.
11. **pnpm Store Cache (`pnpm`)**: Wipes local content-addressable store indexes.
12. **Yarn Cache (`yarn`)**: Safely purges downloaded project package tars.
13. **Bun Cache (`bun`)**: Cleans local JS bun dependency cache folders.
14. **pip Cache (`pip`)**: Clears Python pip package installation metadata.
15. **Cargo Registry (`cargo`)**: Trims Rust cargo crate download indexes.
16. **Gradle Cache (`gradle`)**: Wipes local Java/Kotlin build runner caches.
17. **NuGet Cache (`nuget`)**: Removes old .NET assembly cached packages.
18. **Docker Temp Files (`docker`)**: Reclaims space from Docker Desktop build artifacts.
19. **VSCode Caches (`vscode`)**: Wipes extension logs and language server cache files.

### 🌐 Web & Browser Performance
20. **Browser Caches (`browsers`)**: Multi-profile scanner purging caches from Chrome, Edge, Firefox, and Brave.

### 🎮 Graphic & Hardware Shaders
21. **GPU Shader Cache (`gpu_shader`)**: Safely purges DirectX, GL, and NVIDIA compiled shader caches to resolve micro-stuttering.

---

## 💻 CLI Commands Directory

Duster exposes 12 specialized subcommands. Every command supports headless machine-readable `--json` formatting for automation scripts.

| Command | Arguments | Flags | Description |
| :--- | :--- | :--- | :--- |
| **`du`** | *None* | *None* | Launches the primary interactive console interface. |
| **`du clean`** | *None* | `-d, --dry-run`, `--debug`, `-w, --whitelist` | Deep scans and purges all 35 system and application cache zones. |
| **`du status`** | *None* | `--json` | Launches the live-updating dual-grid hardware dashboard or exits with stats JSON. |
| **`du analyze`** | `[path]` | `--json` | Runs the interactive TUI storage scanner starting at the specified path. |
| **`du purge`** | *None* | `--path`, `-d, --dry-run`, `--safe`, `--json`, `-y, --yes` | Deep scans directories to locate and purge heavy developer build directories. |
| **`du uninstall`**| *None* | `-d, --dry-run`, `--json` | Audits installed windows programs with interactive residual registry removal. |
| **`du optimize`** | *None* | `-d, --dry-run` | Initiates network interface resets, defrag hooks, and system service optimizations. |
| **`du installer`**| *None* | *None* | Sweeps local storage to detect and list heavy unused `.msi`/`.exe` files. |
| **`du doctor`** | *None* | *None* | Diagnoses UAC privilege level, Defender hook status, and filesystem registry policies. |
| **`du benchmark`**| *None* | *None* | Profiles disk read/write throughput rates and heap memory metrics. |
| **`du verify`** | *None* | *None* | Verifies binary digital signatures, checksums, and execution boundary scopes. |
| **`du update`** | *None* | *None* | Securely updates the CLI binary via RSA-2048/SHA-256 cryptosigns. |
| **`du remove`** | *None* | *None* | Safely cleans up local Duster user config files and debug execution logs. |

---

## 🎹 Navigation Keybindings

All interactive UI states (Disk Analyzer, System Dashboard, and Main Menu) share intuitive keyboard shortcuts:

| Key | Context | Action |
| :--- | :--- | :--- |
| **`↑ / ↓`** or **`k / j`** | List Panels | Move cursor highlight up/down. |
| **`Enter` / `→`** or **`l`** | Disk Analyzer | Expand folder and drill down one level deeper. |
| **`Esc` / `←`** or **`h`** | Disk Analyzer | Move back out to the parent folder. |
| **`Tab`** | Disk Analyzer | Toggle focus between the directory list and the file detail panel. |
| **`e`** | Disk Analyzer | Native Shell reveal: opens highlighted target in Windows Explorer. |
| **`x`** | Disk Analyzer | Delete highlighted file or folder directly to the Windows Recycle Bin. |
| **`q`** or **`Ctrl + C`** | All Interfaces | Instantly exits the current interface back to the shell. |

---

## 🔒 Security & Safe-by-Design Engineering

Duster guarantees absolute stability and data protection:

1. **UAC Guarding & Privileges Isolation**: Destructive zones like `C:\Windows\Prefetch` or low-level defragmentation hooks require high privileges. Duster leverages a custom UAC elevation launcher under `lib/elevation/` to prompt for credentials only when necessary.
2. **Deterministic Win32 Path Resolving**: System folders are queried programmatically using dynamic Win32 DLL loads (`GetSystemDirectoryW` via `kernel32.dll`), avoiding dependency on easily-spoofed environment variables.
3. **No Junction Loop Crawling**: Fully checks for re-parse points and NTFS junctions. Symbolically linked target maps are omitted during scans to prevent infinite directory recursion loops.
4. **Bandwidth Preservation**: Bypasses files carrying the `FILE_ATTRIBUTE_OFFLINE` (0x1000) attribute. This prevents automated cleanup scripts from triggering massive cloud sync downloads from OneDrive.
5. **Decoupled Process Isolation**: Defragmentation and optimization subprocesses run isolated inside secure process groups (`CREATE_NEW_PROCESS_GROUP`). Killing the `du` command guarantees all underlying optimizations exit cleanly.
6. **Detailed Operation Logs**: Every destructive write is captured to `%APPDATA%\duster\operations.log` containing time, path, bytes freed, and success metrics. Disable logging instantly by setting `DU_NO_OPLOG=1`.

---

## 📦 Installation

Duster compiles to a **single, statically-linked, zero-dependency** Windows executable (no runtime, no installer required).

---

### ⚡ Method 1 — PowerShell One-Liner (Recommended)

Open **PowerShell** and run:

```powershell
irm https://raw.githubusercontent.com/Nur-Adnan/Duster/main/scripts/install.ps1 | iex
```

This automatically:
- Detects your architecture (x64 or ARM64)
- Downloads the latest release from GitHub
- Verifies the SHA-256 checksum
- Installs to `%LOCALAPPDATA%\Duster\`
- Adds `du` to your user PATH
- No administrator rights required

**Options (download first, then run with flags):**

```powershell
# Download the installer
curl -L -o install.ps1 https://raw.githubusercontent.com/Nur-Adnan/Duster/main/scripts/install.ps1

# Install latest version (default)
.\install.ps1

# Install a specific version
.\install.ps1 -Version "1.0.2"

# Install to a custom directory
.\install.ps1 -InstallDir "C:\Tools\Duster"

# Silent install (no output, for scripts/automation)
.\install.ps1 -Silent

# Force reinstall even if same version is present
.\install.ps1 -Force
```

---

### 🖥️ Method 2 — CMD (Command Prompt)

Open **Command Prompt** and run:

```cmd
powershell -NoProfile -ExecutionPolicy Bypass -Command "irm https://raw.githubusercontent.com/Nur-Adnan/Duster/main/scripts/install.ps1 | iex"
```

Or download and run the batch installer (handles PATH refresh automatically):

```cmd
curl -L -o install.cmd https://raw.githubusercontent.com/Nur-Adnan/Duster/main/scripts/install.cmd
install.cmd
```

After installation, **open a new Command Prompt window**, then:

```cmd
du --version
du --help
```

---

### 📁 Method 3 — Portable (Manual)

1. Go to [Releases](https://github.com/Nur-Adnan/Duster/releases/latest)
2. Download the right binary for your system:
   - **Most PCs (Intel/AMD):** `duster-windows-amd64.exe`
   - **ARM devices (Surface Pro X, Snapdragon PCs):** `duster-windows-arm64.exe`
   - **With launcher + docs:** `Duster-x.x.x-Portable-x64.zip`
3. Rename the downloaded file to `du.exe`
4. Place it anywhere (e.g. `C:\Tools\`)
5. Add that folder to your PATH:

```powershell
# Add to PATH permanently (PowerShell):
$dir = "C:\Tools"
$path = [Environment]::GetEnvironmentVariable("Path","User")
[Environment]::SetEnvironmentVariable("Path","$path;$dir","User")
```

```cmd
:: Add to PATH permanently (CMD):
setx PATH "%PATH%;C:\Tools"
```

---

### 🍫 Method 4 — Scoop

```powershell
scoop bucket add duster https://github.com/Nur-Adnan/scoop-duster
scoop install duster
```

---

### 📦 Method 5 — winget

```powershell
winget install NurAdnan.Duster
```

---

### ✅ Verify Installation

In any terminal after install:

```powershell
du --version
du --help
du status
```

---

### ❌ Uninstall

```powershell
# Download and run the uninstaller:
irm https://raw.githubusercontent.com/Nur-Adnan/Duster/main/scripts/uninstall.ps1 | iex

# Or silently, also removing logs:
.\uninstall.ps1 -Silent -RemoveAppData
```

---

## 🛠️ Build from Source

### Prerequisites
- Go 1.25.x or higher ([download](https://go.dev/dl/))
- Git

### Build

```powershell
# Clone the repository
git clone https://github.com/Nur-Adnan/Duster.git
cd Duster

# Download dependencies
go mod download

# Build production binary (Windows AMD64)
$env:GOOS="windows"; $env:GOARCH="amd64"; $env:CGO_ENABLED="0"
go build -trimpath -ldflags="-s -w -X main.Version=dev" -o du.exe .

# Run tests
go test -v ./...
```

```cmd
:: CMD equivalent:
set GOOS=windows
set GOARCH=amd64
set CGO_ENABLED=0
go build -trimpath -ldflags="-s -w -X main.Version=dev" -o du.exe .
```

### Full Release Build (all architectures + portable ZIPs)

```powershell
make release
```

---

## 📂 Project Architecture

Duster is structured cleanly into strict boundary layers to protect system integrity:

```
Duster/
├── .github/workflows/   # CI/CD Release workflows
├── cmd/                 # Bubble Tea TUIs, command handlers & Lipgloss design system
│   ├── analyze.go       # Storage analyzer TUI & Vim navigation
│   ├── clean.go         # Scanning engine & 21 category definitions
│   ├── status.go        # Real-time health dashboard with 1s refresh tick
│   └── styles.go        # Unified canonical design systems & color palettes
├── lib/                 # Core low-level Windows integration layers
│   ├── elevation/       # UAC Privilege Escalation handlers
│   ├── fs/              # Safe path resolving, boundary assertions & NTFS check
│   └── sysinfo/         # Win32 host queries, registry analyzers & gopsutil feeds
├── go.mod               # Declares Go 1.25 runtime dependencies
└── README.md            # You are here!
```

---

## 📄 License

Duster is open-source software licensed under the **MIT License**.
* Copyright © 2026 Nur Adnan
