<div align="center">
  <h1>Duster</h1>
  <p><strong>Windows-native deep cleaner & system optimization CLI</strong></p>
  <p>A single-binary, zero-dependency terminal utility that cleans caches, analyzes disk usage, monitors system health, and purges developer artifacts — all from your terminal.</p>
</div>

<p align="center">
  <a href="https://github.com/Nur-Adnan/Duster/releases/latest"><img src="https://img.shields.io/github/v/tag/Nur-Adnan/Duster?style=flat-square&color=00ADB5&label=version" alt="Version"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-blue.svg?style=flat-square" alt="License"></a>
  <a href="https://github.com/Nur-Adnan/Duster/actions/workflows/ci.yml"><img src="https://img.shields.io/github/actions/workflow/status/Nur-Adnan/Duster/ci.yml?branch=main&label=CI&style=flat-square" alt="CI"></a>
  <img src="https://img.shields.io/badge/Go-1.25-00ADD8?style=flat-square&logo=go" alt="Go">
  <img src="https://img.shields.io/badge/Windows-10%2F11-0078D6?style=flat-square&logo=windows11" alt="Platform">
</p>

<p align="center">
  <img width="1448" height="1086" alt="ChatGPT Image May 21, 2026, 12_01_51 PM" src="https://github.com/user-attachments/assets/259dfe0e-fdb9-4501-a4d6-880ff26d1ca0" />
</p>

---

## Quick Start

```powershell
# Install (PowerShell one-liner)
irm https://raw.githubusercontent.com/Nur-Adnan/Duster/main/scripts/install.ps1 | iex

# Run
du status      # Live system dashboard
du clean       # Deep cache cleanup
du analyze .   # Interactive disk explorer
```

---

## Features

| Command | What it does |
|:---|:---|
| `du` | Interactive landing screen with system overview |
| `du clean` | Scans & cleans 34 cache categories (temp, browsers, dev tools, GPU shaders) |
| `du status` | Real-time CPU (usage, temperature), RAM, disk, network, battery dashboard (1s refresh) |
| `du analyze [path]` | Drill-down disk usage explorer with delete & open actions; shows what grew since the last scan of the same folder |
| `du purge` | Finds `node_modules`, `target`, `dist`, `.gradle`, `vendor` and keeps them, restorable for 7 days |
| `du restore` | Lists and brings back what `purge`, `uninstall` and `installer` kept |
| `du uninstall` | App uninstaller + leftover AppData sweeper |
| `du installer` | Detects bulky old `.exe`/`.msi` installers in Downloads |
| `du vdisk` | Shrinks the WSL and Docker virtual disks (`.vhdx`) that grow but never shrink (admin) |
| `du schedule` | Cleans caches automatically: weekly (or daily/monthly) and early when the system drive runs low; never as administrator |
| `du optimize` | Flushes DNS, clears the Delivery Optimization cache, optimizes drives (SSD TRIM, admin); reports big reclaimable space; `--deep` cleans the component store (WinSxS) |
| `du doctor` | System diagnostics (UAC, Defender, filesystem policies) |
| `du benchmark` | Disk I/O throughput & memory profiling |
| `du update` | Self-update with SHA-256 verification |
| `du remove` | Uninstall Duster and delete all its config/logs |

> `du analyze` answers "where did my space go?". Each scan keeps a small snapshot of the folder's largest items, and the next scan of the same folder says what changed: `Change: +6.9 GB since Sep 7 (17 days ago)`. Press `c` for the explanation, for example `Downloads\ubuntu.iso +5.7 GB new` or `Videos\old -900 MB gone`, and Enter to jump straight to the item. It names the most specific folders or files behind the change rather than every parent folder. `--since 7d` compares with an older scan, `--no-history` turns it off, and `--json` includes it as `changes`. Snapshots stay on this PC in `%LOCALAPPDATA%\Duster\history`: they hold the names and sizes of the biggest folders and files, never their contents, and `du remove` deletes them.

> `du vdisk` is for developer machines: a WSL 2 distribution and Docker Desktop each keep a virtual disk that grows as you work and never shrinks when you delete, which routinely costs 20 to 100 GB. It finds those disks, shows what they hold, and compacts them in place. Nothing inside a disk is read, changed or deleted, but compacting stops every running distribution and container first, so it asks before it starts.

> `du schedule` keeps caches tidy without you ever opening Duster: a daily Task Scheduler check, running as you and never as administrator, cleans a safe set (temp files, browser caches, never cookies, history or sessions, thumbnails, error reports, crash dumps, GPU shader caches) on your chosen interval (weekly by default) or early once the system drive runs low on space. `--add npm,gradle,docker` and other developer or app caches opt in, since the next build or launch just downloads them again. The Recycle Bin, Recent files, Spotify's offline downloads and anything needing administrator rights are never scheduled, each refused with its reason. It runs silently through a separate windowless launcher, `duw.exe`, so no console or terminal window ever appears; `du schedule` shows what the last run did, and `du schedule off` turns it off.

> `du optimize` also reports the space that only you can reclaim: a previous Windows installation (`Windows.old`) and the hibernation file. Duster measures them and tells you how to remove them, but never touches either.

> `du purge`, the uninstall leftover sweep and the `installer` sweep no longer delete for good: they keep what they remove in a quarantine on the same drive, so no copy is made and a nearly full drive still gets its space back. `du restore` lists what's kept, `du restore <n>` brings a session back, and it never overwrites a file or folder that already exists at the original path, skipping it instead. Kept items are swept away after 7 days, sooner and oldest first if a drive runs low on space. `--permanent` on `purge` deletes for good right away, when you're sure and want the space now.

> Most commands support `--json` for scripting. `clean`, `purge`, `installer`, `optimize`, `vdisk`, `uninstall`, `remove`, `restore` and `schedule on` support `--dry-run` for safe previews.

---

## Cleanup Categories

Duster cleans **34 categories**. The main groups (run `du clean --dry-run` for the full list):

<details>
<summary><strong>💻 System & Windows</strong> (9 categories)</summary>

| Category | Target |
|:---|:---|
| Temp Files | `%TEMP%`, `C:\Windows\Temp` |
| Update Cache | `SoftwareDistribution\Download` |
| Prefetch | Windows prefetch binaries *(admin)* |
| Thumbnails | Explorer `thumbcache_*.db` files |
| Error Reports | Windows Error Reporting dumps |
| Recycle Bin | Native Recycle Bin cleanup |
| DNS Cache | Flush local DNS resolver |
| Delivery Optimization | Peer-to-peer update cache |
| Crash Dumps | Minidumps and crash logs |

</details>

<details>
<summary><strong>🚀 Developer Tools</strong> (10 categories)</summary>

| Category | Target |
|:---|:---|
| npm | Global npm cache |
| pnpm | Content-addressable store |
| Yarn | Downloaded package tarballs |
| Bun | JS runtime cache |
| pip | Python package metadata |
| Cargo | Rust crate indexes |
| Gradle | Java/Kotlin build cache |
| NuGet | .NET assembly cache |
| Docker | Desktop build artifacts |
| VS Code | Extension logs & language server cache |

</details>

<details>
<summary><strong>🌐 Browsers</strong> (1 multi-profile scanner)</summary>

Clears cache from **Chrome**, **Edge**, **Firefox**, and **Brave** across all user profiles.

</details>

<details>
<summary><strong>🎮 GPU & Shaders</strong> (1 category)</summary>

Purges DirectX, OpenGL, and NVIDIA compiled shader caches to fix micro-stuttering.

</details>

---

## Installation

### PowerShell (Recommended)

```powershell
irm https://raw.githubusercontent.com/Nur-Adnan/Duster/main/scripts/install.ps1 | iex
```

<details>
<summary>Advanced options</summary>

```powershell
.\install.ps1 -Version "1.0.2"        # Specific version
.\install.ps1 -InstallDir "C:\Tools"   # Custom directory
.\install.ps1 -Silent                  # No output (CI/automation)
.\install.ps1 -Force                   # Reinstall same version
```

</details>

### CMD

```cmd
powershell -NoProfile -ExecutionPolicy Bypass -Command "irm https://raw.githubusercontent.com/Nur-Adnan/Duster/main/scripts/install.ps1 | iex"
```

### Scoop / winget

> Not yet published — the Scoop bucket and winget package are planned.
> Manifests are staged in [scripts/manifests/](scripts/manifests/).
> Use the PowerShell one-liner above until then.

### Manual Download

Download from [Releases](https://github.com/Nur-Adnan/Duster/releases/latest), rename to `du.exe`, and add to PATH.

### Verify

```powershell
du --version
```

### Uninstall

```powershell
irm https://raw.githubusercontent.com/Nur-Adnan/Duster/main/scripts/uninstall.ps1 | iex
```

---

## Keyboard Shortcuts

| Key | Action |
|:---|:---|
| `↑↓` / `jk` | Navigate lists |
| `Enter` / `→` | Drill into folder |
| `Esc` / `←` / `⌫` | Go back |
| `O` | Open in Explorer |
| `D` | Delete to Recycle Bin (asks first) |
| `L` | Toggle large files view |
| `Space` | Toggle selection |
| `Q` | Quit |

---

## Security

| Protection | How |
|:---|:---|
| **UAC Elevation** | Prompts for admin only when needed (prefetch, defrag) |
| **Path Safety** | System folders resolved via Win32 API, not env vars |
| **Junction Protection** | Detects NTFS reparse points to prevent infinite recursion |
| **OneDrive Shield** | Skips `FILE_ATTRIBUTE_OFFLINE` files to prevent cloud sync |
| **Process Isolation** | Subprocesses run in separate process groups for clean exit |
| **Audit Log** | All deletions logged to `%LOCALAPPDATA%\Duster\operations.log` |

> Disable logging: `set DU_NO_OPLOG=1`

---

## Code signing policy

Free code signing provided by [SignPath.io](https://about.signpath.io/), certificate by [SignPath Foundation](https://signpath.org/). Signing starts once the project's SignPath Foundation application is approved; releases before that are unsigned. Who approves releases, what gets signed, and the privacy statement: [docs/code-signing.md](docs/code-signing.md).

---

## Build from Source

```powershell
git clone https://github.com/Nur-Adnan/Duster.git
cd Duster
go build -trimpath -ldflags="-s -w" -o du.exe .
go test ./...
```

---

## Project Structure

```
Duster/
├── cmd/              # CLI commands, Bubble Tea TUIs, Lipgloss styles
├── lib/
│   ├── elevation/    # UAC privilege escalation
│   ├── fs/           # Safe path resolution & NTFS checks
│   ├── sysinfo/      # Win32 system queries (gopsutil)
│   └── uninstall/    # Registry-based app discovery
├── internal/
│   ├── config/       # Configuration management
│   ├── logging/      # Structured operation logging
│   └── security/     # Security policy enforcement
├── scripts/          # Install/uninstall scripts (PS1, CMD, batch)
├── installer/        # Inno Setup configuration
└── main.go           # Entry point
```

---

## License

MIT License — Copyright © 2026 [Nur Adnan](https://github.com/Nur-Adnan)
