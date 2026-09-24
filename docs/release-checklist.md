# Release checklist

CI already runs:
- unit tests on Windows and Linux
- cross-builds
- govulncheck
- install.ps1 and uninstall.ps1 on Windows PowerShell 5.1, and install.cmd from a folder with a space and an apostrophe
- the setup exe and the Chocolatey package: build, install, uninstall

The steps below touch real user data, UAC or the Recycle Bin. The Windows Smoke Test runs them on a throwaway runner. By hand, use a throwaway Windows 10/11 VM with a snapshot, never a daily machine.

## 1. Prepare

- [ ] CI is green on the commit you will tag.
- [ ] `CHANGELOG.md` has an entry for the version.
- [ ] Build it: `GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -o du.exe .`
- [ ] Copy `du.exe` and `scripts/` to the VM, then take a snapshot.

## 2. Smoke test

Run **Windows Smoke Test** (Actions tab > Run workflow, or push a `smoke/**` branch). On a real Windows runner it checks:
- `--version`, the `verify` and `doctor` exit codes, and `analyze`
- `clean` against a junction root and a locked file
- `purge` project markers, and `--safe` sending to the Recycle Bin
- the Downloads `installer` scan
- an end-to-end `update`
- `du schedule`: `on --dry-run` registers nothing, an opt-in-only category is refused, `on` registers the task, `schtasks /Run` starts `duw.exe`, the run is recorded and `off` removes the task; the same for a standard (non-admin) user managing their own task
- the undo window: `du purge` keeps a project instead of deleting it, `du restore` lists and brings it back, on both the profile volume and a second drive; a restore over a recreated folder is skipped, never overwritten; a standard user's quarantine folder on that second drive is unreadable to another standard user; `du restore --empty` empties it
- in a real terminal (`e2e/`, a Windows pseudo console):
  - `status` renders with a Temp line (a value or N/A) and `q` quits; `status --json` has a plausible `CPUTempC` or none
  - `analyze`: Enter and Backspace, then `d` sends a file to the Recycle Bin
  - `analyze`: after a first scan and a new file, `c` names the file and Enter then `d` targets it; `--json` reports `changes` (null on a first scan), `--no-history` saves nothing
  - the Recycle Bin size prompt: with the bin set to 1 MB, Windows asks before deleting a larger file, and No keeps it
  - `c` does nothing in a `clean --dry-run` screen, while `d` still runs the dry run
  - the landing Startup view: `d` asks, any other key cancels, `d` twice removes
- real uninstalls of an Inno Setup app:
  - the uninstaller runs, its entry disappears, and leftovers are listed with nothing selected
  - a cancelled wizard shows "UNINSTALL NOT CONFIRMED" and "LEFTOVER SWEEP SKIPPED"
  - a per-user app is refused while elevated
- install.ps1's admin path, from a folder with a space and an apostrophe: the elevated re-launch (file and piped), and a denied prompt
- upgrading a v1.0.2 install by reinstalling
- `du remove`

What the runner can't do, so check these by hand:
- click a real UAC prompt (the runner is already elevated, so none appears)
- uninstall a per-user app from a non-admin terminal
- MSI (7-Zip) and InstallShield or rundll32 uninstallers
- a real CPU temperature: runners are VMs that usually expose no thermal zone, and they run English Windows
- an unplugged laptop skipping a scheduled clean, and one asleep at the check time catching up when it wakes (`StartWhenAvailable`)
- Windows Terminal set as the default console still showing no window when `duw.exe` runs
- two users signed in on one PC each getting their own scheduled clean
- uninstalling with the setup exe deleting the scheduled task
- a USB drive holding a kept session, unplugged and then replugged: `du restore` must list it again rather than losing track of it
- a low-space sweep: purge or delete enough on a nearly full drive that its quarantine is swept before the 7-day window, oldest kept session first, and confirm the drive is back at or above 10% free

The full list below stays, so a failure can be reproduced by hand.

Run everything from a normal (non-admin) terminal unless a step says elevated. Each step lists its expected result.

**Read-only**
- [ ] `du --version`: prints the new version.
- [ ] `du doctor --json; $LASTEXITCODE`: prints JSON. Exit code is 0 when `healthy` is true and 1 when any check has `FAIL`.
- [ ] `du verify --json; $LASTEXITCODE`: all 8 cases pass, exit code 0.
- [ ] `du status`: the dashboard renders, and `q` quits.
- [ ] `du status` on a laptop: Temp in the CPU panel shows a value, and it rises while `du benchmark` runs in a second terminal. Do it once on a non-English Windows too. A desktop or VM with no thermal zone shows N/A.
- [ ] `du analyze $env:USERPROFILE`:
  - Enter a folder and go back (Enter, then Backspace). The view is instant with no rescan.
  - `d` on a scratch file sends it to the Recycle Bin.
- [ ] What changed since last time, on a real profile:
  - Run `du analyze $env:USERPROFILE`, quit, download or copy a file of a few hundred MB into Downloads, and run it again. The `Change:` line shows the growth, `c` names the file, and Enter lands on it in Downloads.
  - The same on a whole drive (`du analyze C:\`): the scan is no slower than with `--no-history`, and `%LOCALAPPDATA%\Duster\history` stays under a few MB.
  - Run it once from an elevated terminal and once from a normal one: the second run warns that administrator rights differ.
  - A USB stick: scan `E:\`, swap in a different stick that gets the same letter, scan again. Duster must say it is a different disk and compare nothing.
  - `du remove` deletes the history folder along with the rest of `%LOCALAPPDATA%\Duster`.
- [ ] Set the Recycle Bin's maximum size to 1 MB. Then `d` on a larger file: Windows asks before permanently deleting it. Restore the size afterwards.

**Clean, purge, installer, optimize, vdisk**
- [ ] `du clean --dry-run`: lists sizes and deletes nothing. In its TUI, `c` does nothing.
- [ ] Locked file: `$h = [IO.File]::Open("$env:TEMP\duster-locked.txt", 'Create', 'ReadWrite', 'None')`, then `du clean --yes --debug`.
  - The temp category prints a ✗ line saying items could not be deleted, and the run ends with a warning.
  - Run `$h.Close()` afterwards.
- [ ] Junction root:
  1. `mkdir C:\JTarget; "keep" > C:\JTarget\keep.txt`
  2. Remove `%LOCALAPPDATA%\pip\Cache` if it exists.
  3. `cmd /c mklink /J "%LOCALAPPDATA%\pip\Cache" C:\JTarget`
  4. `du clean --yes --debug`

  `C:\JTarget\keep.txt` must survive.
- [ ] `du purge --path <dir> --dry-run`: a `node_modules` with no `package.json` beside it is not listed; add a `package.json` and it is. `--safe` sends items to the Recycle Bin.
- [ ] `du installer --dry-run`: lists only top-level Downloads files that are at least 7 days old and at least 50 MB. Files in subfolders are ignored.
- [ ] Elevated `du optimize`: flushes DNS, reports the Delivery Optimization space it freed, then runs `defrag /O`.
- [ ] `du optimize --json`: the `reclaimable` section lists `windows_old` and `hibernation`. On a PC with a recent feature update, `Windows.old` shows a size in the tens of GB; on a PC with hibernation on, the hibernation file shows a size. Duster must not delete either.
- [ ] Elevated `du optimize --deep --dry-run`: the component store task prints the analysis (overhead size and superseded package count) and removes nothing.
- [ ] Elevated `du optimize --deep` on a VM with pending cleanup: the task runs DISM, shows its elapsed time, and finishes. Afterwards `dism /Online /Cleanup-Image /AnalyzeComponentStore` reports a smaller overhead, and an installed update can still be uninstalled from Settings (Duster never passes `/ResetBase`).
- [ ] While `du optimize --deep` runs DISM, press `q`: it asks to confirm, any other key keeps going, and a second `q` stops it and prints that the cleanup was stopped part-way.
- [ ] Non-English Windows: `du optimize --deep --dry-run` either reports the analysis or fails with "its component store report could not be read". It must never report 0 bytes of overhead.
- [ ] `du vdisk --dry-run` on a machine with a WSL 2 distribution and Docker Desktop: both disks are listed, biggest first, with a size that matches the `.vhdx` in Explorer. Nothing is stopped and nothing changes.
- [ ] With a distribution already running, `du vdisk --dry-run` fills in its "Used" and "Recover" columns from the guest. With every distribution stopped, both read `?` and no distribution is started to find out.
- [ ] Elevated `du vdisk` with Docker Desktop still open: it runs `wsl --shutdown`, the Docker disk fails with a sharing violation naming the locked file, and the WSL disks still compact. Docker Desktop restarts normally afterwards.
- [ ] Elevated `du vdisk` with Docker Desktop quit: every selected disk reports "compacted", the `.vhdx` files are smaller in Explorer, and `wsl -l -v` plus `docker run --rm hello-world` both still work. Data inside the distribution is untouched.
- [ ] A sparse disk (`wsl --manage <distro> --set-sparse true --allow-unsafe` on a spare distribution): Duster lists it as sparse, refuses to select it, and never hands it to diskpart. Duster must never turn sparse mode on by itself.
- [ ] While `du vdisk` is compacting, press `q`: it asks to confirm, any other key keeps going, and a second `q` stops it. Afterwards `diskpart` → `select vdisk file=...` → `detail vdisk` shows the disk is **not** attached, and the distribution starts normally.
- [ ] If Windows offers to format a disk while `du vdisk` is compacting, dismissing it must not affect the run, and the disk must be unchanged afterwards (it is attached read-only).
- [ ] A Windows profile whose name is not ASCII: `du vdisk` either compacts the disk (8.3 names on) or refuses it with the manual diskpart instructions. It must never report success without shrinking anything.
- [ ] Landing screen: run `du`, open Startup, press `d`. It asks first; any other key cancels; `d` twice removes the entries.

**Uninstall**
- [ ] Install 7-Zip (MSI), then uninstall it with `du uninstall`. The uninstaller runs, the app entry disappears, and its leftover folders are listed with nothing selected.
- [ ] Start an uninstall and cancel the vendor's wizard. Duster shows "UNINSTALL NOT CONFIRMED" and "LEFTOVER SWEEP SKIPPED".
- [ ] An InstallShield or rundll32-based app, if you have one: its uninstaller starts with its arguments intact.
- [ ] A per-user app (for example the VS Code user installer):
  - From an elevated terminal, `du uninstall` refuses with "per-user app: run Duster without administrator rights".
  - From a normal terminal, it uninstalls.

**Undo window (`du restore`)**
- [ ] A USB drive: purge a project on it, unplug the drive, then run `du restore`. It must not lose track of the session; unplug it before the session is 7 days old, plug it back in, and `du restore` lists it again with the item still restorable.
- [ ] A low-space sweep: fill a drive to under 10% free with several kept sessions already on it, then run `du restore` or `du schedule run` again. The oldest session on that drive is swept first, one at a time, until the drive is back at or above 10% free or its quarantine is empty. Sessions on other drives, and anything not yet 7 days old, are left alone.

**Scheduled cleaning**

Known gap: the winget, Scoop and Chocolatey manifests (`scripts/manifests`) install the standalone `du.exe` only, with no `duw.exe` beside it. `du schedule on` refuses on those installs until the manifests switch to the portable zip (Chocolatey would also need a `duw.exe.ignore` to avoid a shim).

- [ ] Update an older install to this release, run `du schedule on` (expect it to refuse with the `du update --force` advice), run `du update --force`, then `du schedule on` succeeds.
- [ ] On a Windows install with a legacy (non-UTF-8) system code page and a non-ASCII user profile: `du schedule on` then `du schedule` shows the task command path correctly (runners use UTF-8 and cannot reproduce this).
- [ ] `du schedule on`, on a laptop: unplug it and let the check time pass. `du schedule` shows the check but no clean. Plug it back in for the next check and it cleans normally.
- [ ] `du schedule on`, then put the PC to sleep before the check time and wake it after: `StartWhenAvailable` catches the check up instead of skipping it.
- [ ] Set Windows Terminal as the default console host (Settings > Privacy & security > For developers, or Windows Terminal's own settings), then `schtasks /Run /TN "Duster Scheduled Clean (<you>)"`: no console or Windows Terminal window appears.
- [ ] Two users signed in on the same PC each run `du schedule on`: `schtasks /Query` lists two separate tasks, one per account, and each user's `du schedule off` removes only their own.
- [ ] Install with the setup exe, run `du schedule on`, then uninstall with the setup exe: `schtasks /Query /TN "Duster Scheduled Clean (<you>)"` fails afterwards, the task is gone.

**Update and remove**
- [ ] Build with `-ldflags "-X main.Version=1.0.1"` and run `du update --json`. It reports an update and installs nothing.
- [ ] `du update --json --yes`: downloads the latest release, verifies its SHA-256 and swaps the binary. `du --version` then shows the release.
- [ ] Last: `du remove`. The binary and `%LOCALAPPDATA%\Duster` are gone, and the exit code is 0.

**Installer script** (restore the snapshot first)
- [ ] `.\scripts\install.ps1 -Version 1.0.2`: installs to `%LOCALAPPDATA%\Duster`, and the user PATH contains that folder exactly once.
- [ ] Admin path:
  1. In a copy of `install.ps1`, make `Test-WDACBlocked` return `$true`.
  2. Run the copy from a folder whose path contains a space. You get a UAC prompt, then it installs to `C:\Program Files\Duster`.
  3. Run it again and deny UAC. You get a clear error.

  The piped (`irm | iex`) admin path downloads main's script, so it can only be checked after merging.

## 3. Release

1. Merge to main and wait for CI to pass.
2. `git tag vX.Y.Z && git push origin vX.Y.Z`. This runs `release.yml`.
   - Once signing is set up ([code-signing.md](code-signing.md)), approve its two SignPath signing requests, the exes and then the setup exe, each within an hour, or the job times out.
3. Check the release:
   - both portable zips, both exes and the setup exe are attached
   - once signing is set up: the release notes say the files are signed, and on Windows `Get-AuthenticodeSignature` reports `Valid` for a downloaded exe and setup exe
   - `checksums-sha256.txt` lists all of them
   - it is marked Latest (unless it is a pre-release)
   - the **Attest Release Files** job passed (it runs `gh attestation verify` on every file)
4. On the VM, run `irm https://raw.githubusercontent.com/Nur-Adnan/Duster/main/scripts/install.ps1 | iex`. It installs the new version.
5. Afterwards:
   - set `$FallbackVersion` in `scripts/install.ps1` to the new version
   - update `scripts/manifests/*` (version, URLs, SHA-256 values from `checksums-sha256.txt`), including `$checksum64` in `scripts/manifests/tools/chocolateyinstall.ps1`
   - date the CHANGELOG entry
