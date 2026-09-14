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
- in a real terminal (`e2e/`, a Windows pseudo console):
  - `status` renders with a Temp line (a value or N/A) and `q` quits; `status --json` has a plausible `CPUTempC` or none
  - `analyze`: Enter and Backspace, then `d` sends a file to the Recycle Bin
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
- [ ] Set the Recycle Bin's maximum size to 1 MB. Then `d` on a larger file: Windows asks before permanently deleting it. Restore the size afterwards.

**Clean, purge, installer, optimize**
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
- [ ] Landing screen: run `du`, open Startup, press `d`. It asks first; any other key cancels; `d` twice removes the entries.

**Uninstall**
- [ ] Install 7-Zip (MSI), then uninstall it with `du uninstall`. The uninstaller runs, the app entry disappears, and its leftover folders are listed with nothing selected.
- [ ] Start an uninstall and cancel the vendor's wizard. Duster shows "UNINSTALL NOT CONFIRMED" and "LEFTOVER SWEEP SKIPPED".
- [ ] An InstallShield or rundll32-based app, if you have one: its uninstaller starts with its arguments intact.
- [ ] A per-user app (for example the VS Code user installer):
  - From an elevated terminal, `du uninstall` refuses with "per-user app: run Duster without administrator rights".
  - From a normal terminal, it uninstalls.

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
