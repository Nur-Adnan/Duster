# Scheduled Cleaning (`du schedule`) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `du schedule on|off|status` registers one per-user Task Scheduler task that runs a windowless `duw.exe`, which runs `du.exe schedule run`; that cleans a safe category set (plus opt-ins) when the cadence is due or the system drive is low on space, and records what it did.

**Architecture:** Pure policy/parsing in `cmd/schedule_policy.go`; task XML build/parse in `cmd/schedule_task.go` (untagged, so it is unit-tested on any OS) and the `schtasks.exe` calls in `cmd/schedule_task_windows.go` (stubs in `cmd/stubs.go`); the run engine, record file and cobra commands in `cmd/schedule.go`. `launcher/duw` is a separate tiny `package main` built with `-H=windowsgui`. Shipping touches update, remove, the installer, install.ps1 and the three packaging paths.

**Tech Stack:** Go 1.25 stdlib (`encoding/xml`, `encoding/csv`, `encoding/json`, `os/user`, `unicode/utf16`), cobra, `golang.org/x/sys/windows` (already a dependency). No new dependencies.

**Spec:** `docs/superpowers/specs/2026-09-24-scheduled-cleaning-design.md` (read it before your task).

## Global Constraints

- Go 1.25, `CGO_ENABLED=0`. Only GOOS=windows ships; every Windows-only symbol gets a stub in `cmd/stubs.go` (`//go:build !windows`).
- Gates before every commit: `gofmt -s -l .` prints nothing; `go vet ./...`; `GOOS=windows go vet ./...`; `~/go/bin/staticcheck ./...`; `GOOS=windows ~/go/bin/staticcheck ./...`; `DU_NO_OPLOG=1 go test ./...`.
- Never run destructive `du` commands on this Mac or any workstation. Real runs happen only in the `Windows Smoke Test` workflow.
- Tests that touch `logging.Dir()` must `t.Setenv("LOCALAPPDATA", t.TempDir())` first.
- Every `exec.Command` with a non-constant argument carries, on the same line: `// nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command` (a pre-commit hook blocks it otherwise).
- package `cmd` defines `func min(a, b int) int` (clean_tui.go), which shadows the builtin: never call `min`/`max` on non-int values.
- The task never runs elevated: `RunLevel` is always `LeastPrivilege`, `LogonType` always `InteractiveToken`.
- No localized `schtasks` text is ever parsed. Allowed: `/Query /XML` (schema-defined) and the task-name column of `/Query /FO CSV /NH` (names are not translated).
- Category policy: safe `temp, browsers, opera, thumbs, wer, crash_dumps, gpu_shader`; opt-in `npm, pnpm, yarn, bun, pip, cargo, gradle, nuget, docker, vscode, jetbrains, discord, slack, teams, steam, epic, adobe`; never `recycle, recent, spotify, dns, update, prefetch, delivery_opt, memdumps, logfiles, fontcache`.
- Cadence minus 4 h slack: daily 20 h, weekly 164 h, monthly 716 h.
- Windows errors 5, 32, 33 → `partial`; any other error → `failed`; exit 0 for cleaned/partial/not due, 1 for refused or any failed.
- Match surrounding code: comment density, naming, table-driven tests with `t.Run`. No em-dashes in user-visible strings or docs.

## Review Focus

1. **Task arguments edited by hand in Task Scheduler** (e.g. `--add recycle`, junk flags): `run` must refuse, clean nothing, record the refusal and exit 1. Test in Task 4.
2. **`schedule.json` damaged or its folder replaced by a junction**: reads as "no history", writes refuse the link. Tests in Task 4.
3. **Account names with spaces, non-ASCII or only non-ASCII characters**: task names must be valid and must not collide. Test in Task 2.
4. **Install paths with spaces and XML-special characters** (`C:\Program Files\A & B <x>\duw.exe`): XML must escape them, and the command path must survive a round trip. Test in Task 3.
5. **Updating from a release whose zip has no `duw.exe`, or an install that never had one**: update succeeds and installs `duw.exe` when present; a missing one is not an error. Tests in Task 7.

## Deviations from the spec (decided while planning; Task 9 folds them into the spec)

- XML build/parse lives in an untagged `cmd/schedule_task.go` so it is tested on every OS; only the `schtasks` calls are Windows-only.
- The due decision keys on `last_success` (the last clean where at least one category did not fail), so a run where everything failed retries next day. Its reason text is "no successful clean yet", not "first scheduled clean".
- When an account name has to be sanitised, the task name gets `-<6 hex of sha256(account)>` so `José` and `Jos` never share a task.
- The task `Command` is quoted (`"C:\Program Files\Duster\duw.exe"`), as Task Scheduler's own exports do.
- `off --uninstall` finds tasks by the name column of `schtasks /Query /FO CSV /NH`.
- `status --json` also carries `task_name`.

---

### Task 1: Spike: can a standard user register and run the task? (controller runs this; not a subagent)

Gates everything else. Throwaway: the branch is deleted afterwards and nothing is merged.

**Files:**
- Create (on a throwaway branch `spike/schedule` only): `.github/workflows/schedule-spike.yml`

- [ ] **Step 1: Create the throwaway branch and workflow**

```bash
git switch -c spike/schedule
```

`.github/workflows/schedule-spike.yml`:

```yaml
name: Schedule Spike
on:
  push:
    branches: ['spike/**']
permissions:
  contents: read
jobs:
  spike:
    runs-on: windows-latest
    timeout-minutes: 15
    steps:
      - name: Register and run the task shape from the spec
        shell: pwsh
        run: |
          $ErrorActionPreference = 'Stop'
          function TaskXml($sid, $cmd, $arguments) {
            $a = [Security.SecurityElement]::Escape($arguments)
            $c = [Security.SecurityElement]::Escape("`"$cmd`"")
            @"
          <?xml version="1.0" encoding="UTF-16"?>
          <Task version="1.2" xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task">
            <RegistrationInfo><Author>Duster</Author><Description>spike</Description></RegistrationInfo>
            <Triggers><CalendarTrigger><StartBoundary>2026-01-01T03:00:00</StartBoundary><Enabled>true</Enabled><ScheduleByDay><DaysInterval>1</DaysInterval></ScheduleByDay></CalendarTrigger></Triggers>
            <Principals><Principal id="Author"><UserId>$sid</UserId><LogonType>InteractiveToken</LogonType><RunLevel>LeastPrivilege</RunLevel></Principal></Principals>
            <Settings><MultipleInstancesPolicy>IgnoreNew</MultipleInstancesPolicy><DisallowStartIfOnBatteries>true</DisallowStartIfOnBatteries><StopIfGoingOnBatteries>true</StopIfGoingOnBatteries><StartWhenAvailable>true</StartWhenAvailable><ExecutionTimeLimit>PT1H</ExecutionTimeLimit><Priority>7</Priority><Enabled>true</Enabled></Settings>
            <Actions Context="Author"><Exec><Command>$c</Command><Arguments>$a</Arguments></Exec></Actions>
          </Task>
          "@.Trim()
          }
          $me = [Security.Principal.WindowsIdentity]::GetCurrent()
          Write-Host "runner: $($me.Name) session $([Diagnostics.Process]::GetCurrentProcess().SessionId)"
          $pwsh = (Get-Command pwsh).Source   # C:\Program Files\PowerShell\7\pwsh.exe: a path with a space
          Write-Host "command: $pwsh"

          # A. The runner's own user: register, read back, run, delete.
          $marker = Join-Path $env:LOCALAPPDATA 'duster-spike.txt'
          Remove-Item $marker -ErrorAction SilentlyContinue
          $f = Join-Path $env:RUNNER_TEMP 'spike.xml'
          [IO.File]::WriteAllText($f, (TaskXml $me.User.Value $pwsh "-NoProfile -Command Set-Content -LiteralPath '$marker' ok"), [Text.Encoding]::Unicode)
          schtasks /Create /XML $f /TN 'Duster Spike (runner)' /F
          if ($LASTEXITCODE) { throw "runner create failed: $LASTEXITCODE" }
          cmd /c "schtasks /Query /TN ""Duster Spike (runner)"" /XML > ""$env:RUNNER_TEMP\q.bin"""
          Write-Host '--- /Query /XML first bytes (encoding check):'
          Format-Hex "$env:RUNNER_TEMP\q.bin" | Select-Object -First 2 | Out-String | Write-Host
          Get-Content "$env:RUNNER_TEMP\q.bin" -Raw | Write-Host
          Write-Host '--- /Query /FO CSV /NH rows naming Duster:'
          schtasks /Query /FO CSV /NH | Select-String 'Duster' | Out-String | Write-Host
          schtasks /Run /TN 'Duster Spike (runner)'
          for ($i = 0; $i -lt 60 -and -not (Test-Path $marker); $i++) { Start-Sleep 1 }
          $ranA = Test-Path $marker
          Write-Host "A. runner-user task ran: $ranA"
          schtasks /Delete /TN 'Duster Spike (runner)' /F

          # B. A new standard (non-admin) user registers, queries and deletes its own task.
          $pw = 'Sp!' + [guid]::NewGuid().ToString('N').Substring(0, 8) + 'aA1'   # net user prompts (Y/N) above 14 characters
          net user dspike $pw /add | Out-Null
          $sid = (New-Object Security.Principal.NTAccount('dspike')).Translate([Security.Principal.SecurityIdentifier]).Value
          $f2 = Join-Path $env:PUBLIC 'spike2.xml'
          [IO.File]::WriteAllText($f2, (TaskXml $sid $pwsh '-NoProfile -Command exit 0'), [Text.Encoding]::Unicode)
          $cred = New-Object PSCredential('dspike', (ConvertTo-SecureString $pw -AsPlainText -Force))
          function AsUser($argList) {
            $p = Start-Process schtasks.exe -Credential $cred -LoadUserProfile -ArgumentList $argList -Wait -PassThru `
              -WorkingDirectory $env:PUBLIC -RedirectStandardOutput "$env:PUBLIC\o.txt" -RedirectStandardError "$env:PUBLIC\e.txt"
            Write-Host "exit $($p.ExitCode): $(Get-Content "$env:PUBLIC\o.txt" -Raw) $(Get-Content "$env:PUBLIC\e.txt" -Raw)"
            $p.ExitCode
          }
          $create = AsUser @('/Create', '/XML', "`"$f2`"", '/TN', '"Duster Spike (dspike)"', '/F')
          $query = AsUser @('/Query', '/TN', '"Duster Spike (dspike)"', '/XML')
          $delete = AsUser @('/Delete', '/TN', '"Duster Spike (dspike)"', '/F')
          Write-Host "B. standard user: create=$create query=$query delete=$delete"
          net user dspike /delete | Out-Null
          if (-not $ranA) { throw 'the runner-user task did not run' }
          if ($create -ne 0 -or $query -ne 0 -or $delete -ne 0) { throw 'a standard user could not manage its own task' }
```

- [ ] **Step 2: Push and watch**

```bash
git add .github/workflows/schedule-spike.yml
git commit -m "spike: scheduled task as runner and standard user"
git push -u origin spike/schedule
gh run list --branch spike/schedule --limit 1   # then: gh run watch <id> --exit-status
```

Expected: success. Record from the log: (a) whether `/Query /XML` output is UTF-16LE (bytes `FF FE` or `3C 00`) or single-byte, (b) the CSV row format of the task name, (c) that a quoted command path with a space ran.

- [ ] **Step 3: Decide and clean up**

If A or B fails, STOP and bring the log to the user: the design's registration approach must be revisited. On success:

```bash
git switch feat/scheduled-clean
git branch -D spike/schedule && git push origin --delete spike/schedule
```

Write the three recorded facts into this plan under "Spike findings" (append at the end) and commit on `feat/scheduled-clean`.

---

### Task 2: Category policy and argument parsing (`cmd/schedule_policy.go`)

**Files:**
- Create: `cmd/schedule_policy.go`
- Test: `cmd/schedule_policy_test.go`

**Interfaces:**
- Consumes: `getCategories() []CleanCategory`, `groupedCategories([]CleanCategory) []CleanCategory` (clean.go), `sha256Hex([]byte) string` (utils.go).
- Produces:
  - `var scheduleSafe, scheduleOptIn []string`, `var scheduleNever map[string]string`
  - `type scheduleConfig struct { Every, At string; LowSpace int; Add []string }`
  - `func parseEvery(string) (string, error)`, `func parseAt(string) (string, error)`, `func parseLowSpace(string) (int, error)` (0 = off)
  - `func resolveAdd(ids []string) (add, notes []string, err error)`
  - `func (scheduleConfig) taskArguments() []string` (starts with `schedule`, `run`)
  - `func parseRunArgs(args []string) (scheduleConfig, []string, error)` (args after `schedule run`; returns notes)
  - `func scheduledCategories(add []string) []CleanCategory`
  - `func scheduleCategoryNames() map[string]string`
  - `func scheduleDue(now, lastSuccess time.Time, every string, freePct float64, lowSpace int) (bool, string)`
  - `func scheduleResultStatus(err error) string` → `"cleaned"|"partial"|"failed"`
  - `func scheduleTaskName(account string) string`
  - `func nextScheduleCheck(now time.Time, at string) time.Time`

- [ ] **Step 1: Write the failing tests**

`cmd/schedule_policy_test.go`:

```go
package cmd

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestSchedulePolicyCoversEveryCategory(t *testing.T) {
	seen := map[string]int{}
	for _, id := range scheduleSafe {
		seen[id]++
	}
	for _, id := range scheduleOptIn {
		seen[id]++
	}
	for id := range scheduleNever {
		seen[id]++
	}
	for _, c := range getCategories() {
		if seen[c.ID] != 1 {
			t.Errorf("category %q is in %d schedule policy lists, want exactly 1", c.ID, seen[c.ID])
		}
		delete(seen, c.ID)
	}
	for id := range seen {
		t.Errorf("schedule policy names %q, which is not a clean category", id)
	}
	// prefetch is the one adminOnlyBlocked category: a scheduled run is never admin.
	if scheduleNever["prefetch"] == "" {
		t.Error("prefetch must be on the never-list")
	}
}

func TestParseScheduleFlags(t *testing.T) {
	t.Run("every", func(t *testing.T) {
		for in, want := range map[string]string{"daily": "daily", "Weekly": "weekly", " monthly ": "monthly"} {
			if got, err := parseEvery(in); err != nil || got != want {
				t.Errorf("parseEvery(%q) = %q, %v", in, got, err)
			}
		}
		for _, in := range []string{"", "hourly", "7d"} {
			if _, err := parseEvery(in); err == nil {
				t.Errorf("parseEvery(%q) accepted", in)
			}
		}
	})
	t.Run("at", func(t *testing.T) {
		for _, in := range []string{"00:00", "07:05", "19:00", "23:59"} {
			if got, err := parseAt(in); err != nil || got != in {
				t.Errorf("parseAt(%q) = %q, %v", in, got, err)
			}
		}
		for _, in := range []string{"24:00", "7:00", "19:60", "1900", "19:00:00", ""} {
			if _, err := parseAt(in); err == nil {
				t.Errorf("parseAt(%q) accepted", in)
			}
		}
	})
	t.Run("low space", func(t *testing.T) {
		for in, want := range map[string]int{"10%": 10, "10": 10, "1%": 1, "50%": 50, "off": 0, "OFF": 0} {
			if got, err := parseLowSpace(in); err != nil || got != want {
				t.Errorf("parseLowSpace(%q) = %d, %v", in, got, err)
			}
		}
		for _, in := range []string{"0%", "51%", "-5", "ten", ""} {
			if _, err := parseLowSpace(in); err == nil {
				t.Errorf("parseLowSpace(%q) accepted", in)
			}
		}
	})
}

func TestResolveAdd(t *testing.T) {
	add, notes, err := resolveAdd([]string{"gradle", " NPM ", "npm", "", "temp"})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(add, []string{"gradle", "npm"}) {
		t.Errorf("add = %v, want sorted, deduplicated [gradle npm]", add)
	}
	if len(notes) != 1 || !strings.Contains(notes[0], "always included") {
		t.Errorf("notes = %v, want one note that temp is always included", notes)
	}

	_, _, err = resolveAdd([]string{"recycle"})
	if err == nil || !strings.Contains(err.Error(), "can't be scheduled: it's how you undo a delete") {
		t.Errorf("recycle: err = %v", err)
	}
	_, _, err = resolveAdd([]string{"nope"})
	if err == nil || !strings.Contains(err.Error(), `"nope"`) {
		t.Errorf("unknown id: err = %v", err)
	}
}

func TestTaskArgumentsRoundTrip(t *testing.T) {
	for _, cfg := range []scheduleConfig{
		{Every: "weekly", At: "19:00", LowSpace: 10, Add: []string{"gradle", "npm"}},
		{Every: "daily", At: "03:00", LowSpace: 0},
		{Every: "monthly", At: "12:30", LowSpace: 50, Add: []string{"docker"}},
	} {
		args := cfg.taskArguments()
		if args[0] != "schedule" || args[1] != "run" {
			t.Fatalf("taskArguments = %v, want to start with schedule run", args)
		}
		got, _, err := parseRunArgs(args[2:])
		if err != nil {
			t.Fatalf("parseRunArgs(%v): %v", args, err)
		}
		got.At = cfg.At // --at lives in the trigger, not the arguments
		if got.Every != cfg.Every || got.LowSpace != cfg.LowSpace || !slices.Equal(got.Add, cfg.Add) {
			t.Errorf("round trip: %+v -> %v -> %+v", cfg, args, got)
		}
	}
}

func TestParseRunArgsRefusesEditedTasks(t *testing.T) {
	for _, args := range [][]string{
		{"--every", "weekly", "--low-space", "10", "--add", "recycle"},
		{"--every", "yearly", "--low-space", "10"},
		{"--every", "weekly", "--low-space"},
		{"--every", "weekly", "--delete", "C:\\"},
	} {
		if _, _, err := parseRunArgs(args); err == nil {
			t.Errorf("parseRunArgs(%v) accepted", args)
		}
	}
}

func TestScheduledCategories(t *testing.T) {
	var ids []string
	for _, c := range scheduledCategories([]string{"npm"}) {
		ids = append(ids, c.ID)
	}
	want := append(slices.Clone(scheduleSafe), "npm")
	if len(ids) != len(want) {
		t.Fatalf("got %v, want the safe set plus npm", ids)
	}
	for _, id := range want {
		if !slices.Contains(ids, id) {
			t.Errorf("missing %q in %v", id, ids)
		}
	}
	if slices.Contains(ids, "gradle") {
		t.Error("gradle included without --add")
	}
}

func TestScheduleDue(t *testing.T) {
	now := time.Date(2026, 9, 24, 19, 0, 0, 0, time.Local)
	tests := []struct {
		name        string
		lastSuccess time.Time
		every       string
		free        float64
		low         int
		want        bool
		reason      string
	}{
		{"never cleaned", time.Time{}, "weekly", 40, 10, true, "no successful clean yet"},
		{"daily inside slack", now.Add(-20 * time.Hour), "daily", 40, 10, true, "daily"},
		{"daily too soon", now.Add(-19 * time.Hour), "daily", 40, 10, false, ""},
		{"weekly boundary", now.Add(-164 * time.Hour), "weekly", 40, 10, true, "weekly"},
		{"weekly too soon", now.Add(-163 * time.Hour), "weekly", 40, 10, false, ""},
		{"monthly boundary", now.Add(-716 * time.Hour), "monthly", 40, 10, true, "monthly"},
		{"low space", now.Add(-time.Hour), "weekly", 8, 10, true, "low space (8% free)"},
		{"low space off", now.Add(-time.Hour), "weekly", 8, 0, false, ""},
		{"free unknown", now.Add(-time.Hour), "weekly", -1, 10, false, ""},
		{"exactly at threshold", now.Add(-time.Hour), "weekly", 10, 10, false, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, reason := scheduleDue(now, tt.lastSuccess, tt.every, tt.free, tt.low)
			if got != tt.want || reason != tt.reason {
				t.Errorf("scheduleDue = %v, %q; want %v, %q", got, reason, tt.want, tt.reason)
			}
		})
	}
}

func TestScheduleResultStatus(t *testing.T) {
	wrap := func(e syscall.Errno) error {
		return fmt.Errorf("3 item(s) could not be deleted: %w", &os.PathError{Op: "remove", Path: "x", Err: e})
	}
	tests := []struct {
		err  error
		want string
	}{
		{nil, "cleaned"},
		{wrap(5), "partial"},
		{wrap(32), "partial"},
		{wrap(33), "partial"},
		{wrap(2), "failed"},
		{errors.New("boom"), "failed"},
	}
	for _, tt := range tests {
		if got := scheduleResultStatus(tt.err); got != tt.want {
			t.Errorf("scheduleResultStatus(%v) = %q, want %q", tt.err, got, tt.want)
		}
	}
}

func TestScheduleTaskName(t *testing.T) {
	if got := scheduleTaskName(`PC\alice`); got != "Duster Scheduled Clean (alice)" {
		t.Errorf("plain name: %q", got)
	}
	jose, jos := scheduleTaskName(`PC\José`), scheduleTaskName(`PC\Jos`)
	if jose == jos {
		t.Errorf("José and Jos share a task name: %q", jose)
	}
	if !strings.HasPrefix(jose, "Duster Scheduled Clean (Jos-") {
		t.Errorf("sanitised name: %q", jose)
	}
	a, b := scheduleTaskName(`PC\李雷`), scheduleTaskName(`PC\王芳`)
	if a == b || !strings.HasPrefix(a, "Duster Scheduled Clean (user-") {
		t.Errorf("non-ASCII names: %q, %q", a, b)
	}
	for _, name := range []string{jose, a, scheduleTaskName(`PC\a b"c`)} {
		inner := strings.TrimSuffix(strings.TrimPrefix(name, "Duster Scheduled Clean ("), ")")
		if strings.ContainsFunc(inner, func(r rune) bool { return !strings.ContainsRune(taskNameChars, r) }) {
			t.Errorf("%q has characters outside %q", name, taskNameChars)
		}
	}
}

func TestNextScheduleCheck(t *testing.T) {
	now := time.Date(2026, 9, 24, 12, 0, 0, 0, time.Local)
	if got := nextScheduleCheck(now, "19:00"); !got.Equal(time.Date(2026, 9, 24, 19, 0, 0, 0, time.Local)) {
		t.Errorf("later today: %v", got)
	}
	if got := nextScheduleCheck(now, "12:00"); !got.Equal(time.Date(2026, 9, 25, 12, 0, 0, 0, time.Local)) {
		t.Errorf("now exactly: %v", got)
	}
	if got := nextScheduleCheck(now, "03:00"); !got.Equal(time.Date(2026, 9, 25, 3, 0, 0, 0, time.Local)) {
		t.Errorf("tomorrow: %v", got)
	}
}
```

- [ ] **Step 2: Run to verify they fail**

Run: `DU_NO_OPLOG=1 go test ./cmd -run 'Schedule|ResolveAdd|TaskArguments|ParseRunArgs|ScheduledCategories' 2>&1 | tail -5`
Expected: build failure, undefined `scheduleSafe` etc.

- [ ] **Step 3: Implement `cmd/schedule_policy.go`**

```go
package cmd

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// Scheduled-clean category policy. Every getCategories ID is in exactly one of
// the three lists (TestSchedulePolicyCoversEveryCategory). It is compiled in
// and applied on every run, because the task's arguments are user-editable.

// scheduleSafe is cleaned by every scheduled run: these rebuild themselves and
// hold nothing the user made.
var scheduleSafe = []string{"temp", "browsers", "opera", "thumbs", "wer", "crash_dumps", "gpu_shader"}

// scheduleOptIn is safe too, but the next build or launch downloads it again,
// which costs offline or on a metered connection: included only with --add.
var scheduleOptIn = []string{
	"npm", "pnpm", "yarn", "bun", "pip", "cargo", "gradle", "nuget", "docker",
	"vscode", "jetbrains", "discord", "slack", "teams", "steam", "epic", "adobe",
}

const scheduleAdminReason = "it needs administrator rights, and scheduled cleans never run as administrator"

// scheduleNever is refused by `du schedule`, with the reason the user sees.
var scheduleNever = map[string]string{
	"recycle":      "it's how you undo a delete",
	"recent":       "it's your recent-files list",
	"spotify":      "it holds Spotify's offline downloads",
	"dns":          "flushing the DNS cache frees no space",
	"update":       scheduleAdminReason,
	"prefetch":     scheduleAdminReason,
	"delivery_opt": scheduleAdminReason,
	"memdumps":     scheduleAdminReason,
	"logfiles":     scheduleAdminReason,
	"fontcache":    "a Windows service holds it during a session; clean it by hand with du clean",
}

// scheduleCadence is how often each --every value cleans.
var scheduleCadence = map[string]time.Duration{
	"daily":   24 * time.Hour,
	"weekly":  7 * 24 * time.Hour,
	"monthly": 30 * 24 * time.Hour,
}

// scheduleSlack absorbs start-time jitter: a clean at 19:02 must not make the
// next day's 19:00 check "not yet".
const scheduleSlack = 4 * time.Hour

// scheduleConfig is everything a scheduled clean needs. It lives only in the
// task itself: --every, --low-space and --add in its arguments, --at in its trigger.
type scheduleConfig struct {
	Every    string   // daily, weekly or monthly
	At       string   // HH:MM, 24-hour
	LowSpace int      // clean early below this free %; 0 = off
	Add      []string // opt-in category IDs, sorted and deduplicated
}

func parseEvery(s string) (string, error) {
	v := strings.ToLower(strings.TrimSpace(s))
	if _, ok := scheduleCadence[v]; !ok {
		return "", fmt.Errorf("--every must be daily, weekly or monthly, not %q", s)
	}
	return v, nil
}

var atPattern = regexp.MustCompile(`^([01][0-9]|2[0-3]):[0-5][0-9]$`)

func parseAt(s string) (string, error) {
	if !atPattern.MatchString(s) {
		return "", fmt.Errorf("--at must be HH:MM on a 24-hour clock, like 19:00, not %q", s)
	}
	return s, nil
}

// parseLowSpace accepts 1%-50% (the % is optional) or off, which returns 0.
func parseLowSpace(s string) (int, error) {
	v := strings.ToLower(strings.TrimSpace(s))
	if v == "off" {
		return 0, nil
	}
	n, err := strconv.Atoi(strings.TrimSuffix(v, "%"))
	if err != nil || n < 1 || n > 50 {
		return 0, fmt.Errorf("--low-space must be 1%%-50%% or off, not %q", s)
	}
	return n, nil
}

// scheduleCategoryNames maps each category ID to its display name.
func scheduleCategoryNames() map[string]string {
	names := map[string]string{}
	for _, c := range getCategories() {
		names[c.ID] = c.Name
	}
	return names
}

// resolveAdd validates --add IDs against the policy. Safe IDs are already in
// every run, so they come back as notes rather than errors.
func resolveAdd(ids []string) (add, notes []string, err error) {
	names := scheduleCategoryNames()
	for _, raw := range ids {
		id := strings.ToLower(strings.TrimSpace(raw))
		switch {
		case id == "":
		case slices.Contains(scheduleSafe, id):
			notes = append(notes, names[id]+" is always included")
		case slices.Contains(scheduleOptIn, id):
			if !slices.Contains(add, id) {
				add = append(add, id)
			}
		case scheduleNever[id] != "":
			return nil, nil, fmt.Errorf("%s can't be scheduled: %s", names[id], scheduleNever[id])
		default:
			return nil, nil, fmt.Errorf("--add %q is not a clean category", raw)
		}
	}
	slices.Sort(add)
	return add, notes, nil
}

// taskArguments is what the task passes to duw.exe. Every token is validated,
// so none needs quoting on a Windows command line.
func (c scheduleConfig) taskArguments() []string {
	low := "off"
	if c.LowSpace > 0 {
		low = strconv.Itoa(c.LowSpace)
	}
	args := []string{"schedule", "run", "--every", c.Every, "--low-space", low}
	if len(c.Add) > 0 {
		args = append(args, "--add", strings.Join(c.Add, ","))
	}
	return args
}

// parseRunArgs reads the flags after `schedule run`. It is the only parser of
// that format: `run` applies it to its own arguments and status to the
// arguments read back from the task, so both refuse the same edits.
func parseRunArgs(args []string) (scheduleConfig, []string, error) {
	cfg := scheduleConfig{Every: "weekly", LowSpace: 10}
	var notes []string
	for i := 0; i < len(args); i += 2 {
		if i+1 >= len(args) {
			return cfg, nil, fmt.Errorf("%s needs a value", args[i])
		}
		var err error
		switch v := args[i+1]; args[i] {
		case "--every":
			cfg.Every, err = parseEvery(v)
		case "--low-space":
			cfg.LowSpace, err = parseLowSpace(v)
		case "--add":
			cfg.Add, notes, err = resolveAdd(strings.Split(v, ","))
		default:
			err = fmt.Errorf("unknown argument %q", args[i])
		}
		if err != nil {
			return cfg, nil, err
		}
	}
	return cfg, notes, nil
}

// scheduledCategories returns the safe set plus add, in cleanGroups order.
func scheduledCategories(add []string) []CleanCategory {
	var out []CleanCategory
	for _, c := range groupedCategories(getCategories()) {
		if slices.Contains(scheduleSafe, c.ID) || slices.Contains(add, c.ID) {
			out = append(out, c)
		}
	}
	return out
}

// scheduleDue decides whether a scheduled run cleans. lastSuccess is zero when
// no clean has succeeded yet; freePct < 0 means free space could not be read.
// The task fires daily, so low space triggers at most one clean a day.
func scheduleDue(now, lastSuccess time.Time, every string, freePct float64, lowSpace int) (bool, string) {
	if lastSuccess.IsZero() {
		return true, "no successful clean yet"
	}
	if now.Sub(lastSuccess) >= scheduleCadence[every]-scheduleSlack {
		return true, every
	}
	if lowSpace > 0 && freePct >= 0 && freePct < float64(lowSpace) {
		return true, fmt.Sprintf("low space (%.0f%% free)", freePct)
	}
	return false, ""
}

// scheduleResultStatus classifies one category's outcome. Files in use (32, 33)
// or protected (5) are normal in %TEMP% and WER's machine-wide half: the rest
// was cleaned, so that is "partial", not a failure.
func scheduleResultStatus(err error) string {
	if err == nil {
		return "cleaned"
	}
	var errno syscall.Errno
	if errors.As(err, &errno) && (errno == 5 || errno == 32 || errno == 33) {
		return "partial"
	}
	return "failed"
}

const taskNameChars = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789._-"

// scheduleTaskName is this account's task. Task names are machine-wide, so each
// user gets their own. A name that loses characters to sanitising gets a short
// hash of the full account name, so José and Jos never share a task.
func scheduleTaskName(account string) string {
	if i := strings.LastIndexByte(account, '\\'); i >= 0 {
		account = account[i+1:]
	}
	clean := strings.Map(func(r rune) rune {
		if strings.ContainsRune(taskNameChars, r) {
			return r
		}
		return -1
	}, account)
	if clean != account {
		if clean == "" {
			clean = "user"
		}
		clean += "-" + sha256Hex([]byte(account))[:6]
	}
	return "Duster Scheduled Clean (" + clean + ")"
}

// nextScheduleCheck is the next time the daily trigger fires after now.
func nextScheduleCheck(now time.Time, at string) time.Time {
	h, _ := strconv.Atoi(at[:2])
	m, _ := strconv.Atoi(at[3:])
	t := time.Date(now.Year(), now.Month(), now.Day(), h, m, 0, 0, now.Location())
	if !t.After(now) {
		t = t.AddDate(0, 0, 1)
	}
	return t
}
```

- [ ] **Step 4: Run tests and gates**

Run: `DU_NO_OPLOG=1 go test ./cmd -run 'Schedule|ResolveAdd|TaskArguments|ParseRunArgs|ScheduledCategories' -v 2>&1 | tail -30`, then all gates from Global Constraints.
Expected: PASS, gates clean (staticcheck counts test usage).

- [ ] **Step 5: Commit**

```bash
git add cmd/schedule_policy.go cmd/schedule_policy_test.go
git commit -m "feat(schedule): category policy, flag parsing and due decision"
```

---

### Task 3: Task XML (`cmd/schedule_task.go`)

**Files:**
- Create: `cmd/schedule_task.go` (untagged: XML build/parse, CSV name parse)
- Test: `cmd/schedule_task_test.go`

**Interfaces:**
- Consumes: `scheduleConfig.taskArguments()` (Task 2), `decodeWSLOutput([]byte) string` (vdisk.go), `systemExecutable(string) string` (styles.go).
- Produces:
  - `type taskDoc struct` with fields `Start string`, `Principal taskPrincipal`, `Settings taskSettings` (`Enabled *bool`), `Actions taskActions` (`Command`, `Arguments string`)
  - `func buildTaskXML(cfg scheduleConfig, duw, userSID string, now time.Time) ([]byte, error)` → UTF-16LE with BOM
  - `func parseTaskXML(b []byte) (taskDoc, error)`
  - `func parseTaskNames(csvOut []byte) []string`

- [ ] **Step 1: Write the failing tests** (`cmd/schedule_task_test.go`)

```go
package cmd

import (
	"strings"
	"testing"
	"time"
	"unicode/utf16"
)

func utf16Text(t *testing.T, b []byte) string {
	t.Helper()
	if len(b) < 2 || b[0] != 0xFF || b[1] != 0xFE {
		t.Fatalf("task XML must start with a UTF-16LE BOM, got % x", b[:min(len(b), 4)])
	}
	units := make([]uint16, 0, len(b)/2)
	for i := 2; i+1 < len(b); i += 2 {
		units = append(units, uint16(b[i])|uint16(b[i+1])<<8)
	}
	return string(utf16.Decode(units))
}

func TestBuildTaskXML(t *testing.T) {
	cfg := scheduleConfig{Every: "weekly", At: "19:00", LowSpace: 10, Add: []string{"gradle", "npm"}}
	duw := `C:\Program Files\A & B <x> "q" 'y'\duw.exe`
	now := time.Date(2026, 9, 24, 8, 0, 0, 0, time.Local)
	b, err := buildTaskXML(cfg, duw, "S-1-5-21-1-2-3-1001", now)
	if err != nil {
		t.Fatal(err)
	}
	text := utf16Text(t, b)
	for _, want := range []string{
		`<?xml version="1.0" encoding="UTF-16"?>`,
		`xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task"`,
		`<StartBoundary>2026-09-24T19:00:00</StartBoundary>`,
		`<DaysInterval>1</DaysInterval>`,
		`<UserId>S-1-5-21-1-2-3-1001</UserId>`,
		`<LogonType>InteractiveToken</LogonType>`,
		`<RunLevel>LeastPrivilege</RunLevel>`,
		`<DisallowStartIfOnBatteries>true</DisallowStartIfOnBatteries>`,
		`<StopIfGoingOnBatteries>true</StopIfGoingOnBatteries>`,
		`<StartWhenAvailable>true</StartWhenAvailable>`,
		`<ExecutionTimeLimit>PT1H</ExecutionTimeLimit>`,
		`<MultipleInstancesPolicy>IgnoreNew</MultipleInstancesPolicy>`,
		`<Priority>7</Priority>`,
		`<Arguments>schedule run --every weekly --low-space 10 --add gradle,npm</Arguments>`,
		`A &amp; B &lt;x&gt;`,
	} {
		if !strings.Contains(text, want) {
			t.Errorf("task XML lacks %s\n%s", want, text)
		}
	}
	if strings.Contains(text, "HighestAvailable") {
		t.Error("the task must never ask for the highest run level")
	}

	doc, err := parseTaskXML(b)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Trim(doc.Actions.Command, `"`); got != duw {
		t.Errorf("command round trip: %q", got)
	}
	if doc.Start != "2026-09-24T19:00:00" || doc.Principal.RunLevel != "LeastPrivilege" {
		t.Errorf("round trip: %+v", doc)
	}
	if doc.Settings.Enabled == nil || !*doc.Settings.Enabled {
		t.Error("Settings.Enabled must round-trip as true")
	}
}

func TestParseTaskXMLAcceptsWindowsEncodings(t *testing.T) {
	plain := `<?xml version="1.0" encoding="UTF-16"?>
<Task version="1.2" xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task">
  <Triggers><CalendarTrigger><StartBoundary>2026-09-24T03:00:00</StartBoundary></CalendarTrigger></Triggers>
  <Settings><Enabled>false</Enabled></Settings>
  <Actions Context="Author"><Exec><Command>C:\x\duw.exe</Command><Arguments>schedule run --every daily --low-space off</Arguments></Exec></Actions>
</Task>`
	le := func(s string, bom bool) []byte {
		var out []byte
		if bom {
			out = []byte{0xFF, 0xFE}
		}
		for _, u := range utf16.Encode([]rune(s)) {
			out = append(out, byte(u), byte(u>>8))
		}
		return out
	}
	for name, b := range map[string][]byte{
		"single-byte":        []byte(plain),
		"UTF-16LE":           le(plain, false),
		"UTF-16LE with BOM":  le(plain, true),
		"UTF-8 with BOM":     append([]byte{0xEF, 0xBB, 0xBF}, plain...),
	} {
		t.Run(name, func(t *testing.T) {
			doc, err := parseTaskXML(b)
			if err != nil {
				t.Fatal(err)
			}
			if doc.Actions.Command != `C:\x\duw.exe` || doc.Start != "2026-09-24T03:00:00" {
				t.Errorf("parsed %+v", doc)
			}
			if doc.Settings.Enabled == nil || *doc.Settings.Enabled {
				t.Error("a disabled task must read as disabled")
			}
		})
	}
	// A task exported without <Enabled> is enabled (the schema default).
	doc, err := parseTaskXML([]byte(strings.Replace(plain, "<Enabled>false</Enabled>", "", 1)))
	if err != nil || doc.Settings.Enabled != nil {
		t.Errorf("missing Enabled: %v, %v", doc.Settings.Enabled, err)
	}
}

func TestParseTaskNames(t *testing.T) {
	out := []byte("\"\\Duster Scheduled Clean (alice)\",\"9/25/2026 7:00:00 PM\",\"Ready\"\r\n" +
		"\"\\Microsoft\\Windows\\Defrag\\ScheduledDefrag\",\"N/A\",\"Ready\"\r\n" +
		"\"\\Duster Scheduled Clean (bob)\",\"N/A\",\"Disabled\"\r\n" +
		"\"\\Duster Scheduled Clean (alice)\",\"9/26/2026 7:00:00 PM\",\"Ready\"\r\n")
	got := parseTaskNames(out)
	if strings.Join(got, "|") != "Duster Scheduled Clean (alice)|Duster Scheduled Clean (bob)" {
		t.Errorf("parseTaskNames = %q", got)
	}
}
```

(`min` here is package cmd's int `min`: both arguments are int, so it compiles.)

- [ ] **Step 2: Run to verify failure**

Run: `DU_NO_OPLOG=1 go test ./cmd -run 'TaskXML|TaskNames' 2>&1 | tail -5`
Expected: build failure, undefined `buildTaskXML`.

- [ ] **Step 3: Implement `cmd/schedule_task.go`**

```go
package cmd

import (
	"bytes"
	"encoding/csv"
	"encoding/xml"
	"io"
	"slices"
	"strings"
	"time"
	"unicode/utf16"
)

// taskDoc is the subset of the Task Scheduler schema Duster writes and reads
// back. Field order follows the schema (CalendarTrigger is a sequence).
type taskDoc struct {
	XMLName      xml.Name      `xml:"http://schemas.microsoft.com/windows/2004/02/mit/task Task"`
	Version      string        `xml:"version,attr"`
	Author       string        `xml:"RegistrationInfo>Author"`
	Description  string        `xml:"RegistrationInfo>Description"`
	Start        string        `xml:"Triggers>CalendarTrigger>StartBoundary"`
	TriggerOn    bool          `xml:"Triggers>CalendarTrigger>Enabled"`
	DaysInterval int           `xml:"Triggers>CalendarTrigger>ScheduleByDay>DaysInterval"`
	Principal    taskPrincipal `xml:"Principals>Principal"`
	Settings     taskSettings  `xml:"Settings"`
	Actions      taskActions   `xml:"Actions"`
}

type taskPrincipal struct {
	ID        string `xml:"id,attr"`
	UserID    string `xml:"UserId"`
	LogonType string `xml:"LogonType"`
	RunLevel  string `xml:"RunLevel"`
}

type taskSettings struct {
	MultipleInstancesPolicy    string `xml:"MultipleInstancesPolicy"`
	DisallowStartIfOnBatteries bool   `xml:"DisallowStartIfOnBatteries"`
	StopIfGoingOnBatteries     bool   `xml:"StopIfGoingOnBatteries"`
	StartWhenAvailable         bool   `xml:"StartWhenAvailable"`
	ExecutionTimeLimit         string `xml:"ExecutionTimeLimit"`
	Priority                   int    `xml:"Priority"`
	// Enabled is nil when an exported task leaves out the default (true).
	Enabled *bool `xml:"Enabled"`
}

type taskActions struct {
	Context   string `xml:"Context,attr"`
	Command   string `xml:"Exec>Command"`
	Arguments string `xml:"Exec>Arguments"`
}

// buildTaskXML returns the task definition for schtasks /Create /XML, encoded
// UTF-16LE with a BOM as Task Scheduler expects. The trigger fires daily at
// cfg.At from today; `run` decides whether a clean is due.
func buildTaskXML(cfg scheduleConfig, duw, userSID string, now time.Time) ([]byte, error) {
	enabled := true
	doc := taskDoc{
		Version:      "1.2",
		Author:       "Duster",
		Description:  "Cleans caches on a schedule. Manage it with: du schedule",
		Start:        now.Format("2006-01-02") + "T" + cfg.At + ":00",
		TriggerOn:    true,
		DaysInterval: 1,
		Principal: taskPrincipal{
			ID: "Author", UserID: userSID,
			LogonType: "InteractiveToken", RunLevel: "LeastPrivilege",
		},
		Settings: taskSettings{
			MultipleInstancesPolicy:    "IgnoreNew",
			DisallowStartIfOnBatteries: true,
			StopIfGoingOnBatteries:     true,
			StartWhenAvailable:         true,
			ExecutionTimeLimit:         "PT1H",
			Priority:                   7,
			Enabled:                    &enabled,
		},
		// Quoted like Task Scheduler's own exports, so a path with spaces is one program.
		Actions: taskActions{
			Context:   "Author",
			Command:   `"` + duw + `"`,
			Arguments: strings.Join(cfg.taskArguments(), " "),
		},
	}
	body, err := xml.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, err
	}
	units := utf16.Encode([]rune(`<?xml version="1.0" encoding="UTF-16"?>` + "\n" + string(body)))
	out := make([]byte, 0, 2+2*len(units))
	out = append(out, 0xFF, 0xFE)
	for _, u := range units {
		out = append(out, byte(u), byte(u>>8))
	}
	return out, nil
}

// parseTaskXML reads schtasks /Query /XML output, which may be UTF-16LE (with
// or without a BOM) or single-byte, while always declaring UTF-16.
func parseTaskXML(b []byte) (taskDoc, error) {
	b = bytes.TrimPrefix(b, []byte{0xFF, 0xFE})
	text := strings.TrimPrefix(decodeWSLOutput(b), "\uFEFF")
	d := xml.NewDecoder(strings.NewReader(text))
	d.CharsetReader = func(_ string, r io.Reader) (io.Reader, error) { return r, nil } // already decoded
	var doc taskDoc
	err := d.Decode(&doc)
	return doc, err
}

// parseTaskNames returns the Duster task names in schtasks /Query /FO CSV /NH
// output. Only the name column is read: names are not translated, the rest is.
func parseTaskNames(csvOut []byte) []string {
	r := csv.NewReader(strings.NewReader(decodeWSLOutput(csvOut)))
	r.FieldsPerRecord = -1
	r.LazyQuotes = true
	var names []string
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil || len(rec) == 0 {
			continue
		}
		name := strings.TrimPrefix(rec[0], `\`)
		if strings.HasPrefix(name, "Duster Scheduled Clean (") && !slices.Contains(names, name) {
			names = append(names, name)
		}
	}
	return names
}
```

- [ ] **Step 4: Run tests and gates**

Run: `DU_NO_OPLOG=1 go test ./cmd -run 'TaskXML|TaskNames' -v 2>&1 | tail -30`, then all gates. (staticcheck counts test usage, so tested helpers are not "unused".)
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/schedule_task.go cmd/schedule_task_test.go
git commit -m "feat(schedule): task XML build and parse"
```

---

### Task 4: Run engine and record (`cmd/schedule.go`, part 1)

**Files:**
- Create: `cmd/schedule.go`
- Modify: `cmd/diskfree_windows.go` (add `diskFreePercent`), `cmd/stubs.go`
- Test: `cmd/schedule_test.go`

**Interfaces:**
- Consumes: Task 2 (`parseRunArgs`, `scheduleDue`, `scheduledCategories`, `scheduleResultStatus`), `runCategory`, `systemDriveRoot`, `realDir`, `ensureRealDir`, `formatBytes`.
- Produces:
  - `type scheduleRecord struct { LastCheck *scheduleCheck; LastClean *scheduleClean; LastSuccess time.Time }` (json `last_check`, `last_clean`, `last_success`)
  - `type scheduleCheck struct { Time time.Time; Result string; FreePercent float64 }`
  - `type scheduleClean struct { Time time.Time; Reason string; Freed int64; FreeBefore, FreeAfter float64; Categories []scheduleCategoryResult }`
  - `type scheduleCategoryResult struct { ID string; Freed int64; Files int; Status, Error string }`
  - `func loadScheduleRecord(dir string) scheduleRecord`, `func saveScheduleRecord(dir string, rec scheduleRecord) error`
  - `func runScheduledClean(args []string, dir string, out io.Writer) int`
  - package vars `scheduleCleanCategory`, `scheduleFreePercent`, `scheduleNow` (tests swap them)

- [ ] **Step 1: Write the failing tests** (`cmd/schedule_test.go`)

```go
package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// fakeScheduleRun swaps the run engine's outside world for the test's duration.
func fakeScheduleRun(t *testing.T, now time.Time, free float64, clean func(CleanCategory) (int64, int, error)) *[]string {
	t.Helper()
	var cleaned []string
	oldClean, oldFree, oldNow := scheduleCleanCategory, scheduleFreePercent, scheduleNow
	t.Cleanup(func() { scheduleCleanCategory, scheduleFreePercent, scheduleNow = oldClean, oldFree, oldNow })
	scheduleCleanCategory = func(c CleanCategory) (int64, int, error) {
		cleaned = append(cleaned, c.ID)
		return clean(c)
	}
	scheduleFreePercent = func() float64 { return free }
	scheduleNow = func() time.Time { return now }
	return &cleaned
}

func TestRunScheduledCleanFirstRunCleansAndRecords(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "Duster")
	now := time.Date(2026, 9, 24, 19, 0, 0, 0, time.Local)
	cleaned := fakeScheduleRun(t, now, 40, func(c CleanCategory) (int64, int, error) {
		if c.ID == "temp" {
			return 100, 2, fmt.Errorf("1 item(s) could not be deleted: %w", &os.PathError{Op: "remove", Path: "x", Err: syscall.Errno(32)})
		}
		return 10, 1, nil
	})
	var out bytes.Buffer
	code := runScheduledClean([]string{"--every", "weekly", "--low-space", "10", "--add", "npm"}, dir, &out)
	if code != 0 {
		t.Fatalf("exit %d, want 0 (partial is not a failure)\n%s", code, out.String())
	}
	if len(*cleaned) != len(scheduleSafe)+1 {
		t.Errorf("cleaned %v, want the safe set plus npm", *cleaned)
	}
	rec := loadScheduleRecord(dir)
	if rec.LastClean == nil || rec.LastClean.Reason != "no successful clean yet" {
		t.Fatalf("record: %+v", rec.LastClean)
	}
	if !rec.LastSuccess.Equal(now) || rec.LastCheck.Result != "cleaned" {
		t.Errorf("last success %v, check %+v", rec.LastSuccess, rec.LastCheck)
	}
	var sawPartial bool
	for _, r := range rec.LastClean.Categories {
		sawPartial = sawPartial || (r.ID == "temp" && r.Status == "partial")
	}
	if !sawPartial {
		t.Errorf("temp not recorded as partial: %+v", rec.LastClean.Categories)
	}
	if rec.LastClean.Freed != 100+10*int64(len(scheduleSafe)) {
		t.Errorf("freed %d", rec.LastClean.Freed)
	}
}

func TestRunScheduledCleanNotDue(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "Duster")
	now := time.Date(2026, 9, 24, 19, 0, 0, 0, time.Local)
	if err := saveScheduleRecord(dir, scheduleRecord{LastSuccess: now.Add(-48 * time.Hour)}); err != nil {
		t.Fatal(err)
	}
	cleaned := fakeScheduleRun(t, now, 40, func(CleanCategory) (int64, int, error) { return 0, 0, nil })
	var out bytes.Buffer
	if code := runScheduledClean([]string{"--every", "weekly", "--low-space", "10"}, dir, &out); code != 0 {
		t.Fatalf("exit %d", code)
	}
	if len(*cleaned) != 0 {
		t.Errorf("cleaned %v while not due", *cleaned)
	}
	if rec := loadScheduleRecord(dir); rec.LastCheck == nil || rec.LastCheck.Result != "not due" || rec.LastCheck.FreePercent != 40 {
		t.Errorf("check: %+v", rec.LastCheck)
	}
}

func TestRunScheduledCleanRefusesEditedArguments(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "Duster")
	cleaned := fakeScheduleRun(t, time.Now(), 5, func(CleanCategory) (int64, int, error) { return 0, 0, nil })
	var out bytes.Buffer
	if code := runScheduledClean([]string{"--every", "weekly", "--low-space", "10", "--add", "recycle"}, dir, &out); code != 1 {
		t.Fatalf("exit %d, want 1", code)
	}
	if len(*cleaned) != 0 {
		t.Errorf("cleaned %v after refusing", *cleaned)
	}
	rec := loadScheduleRecord(dir)
	if rec.LastCheck == nil || !strings.HasPrefix(rec.LastCheck.Result, "refused: ") {
		t.Errorf("check: %+v", rec.LastCheck)
	}
}

func TestRunScheduledCleanFailedCategory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "Duster")
	now := time.Now()
	fakeScheduleRun(t, now, 40, func(c CleanCategory) (int64, int, error) {
		if c.ID == "thumbs" {
			return 0, 0, errors.New("disk on fire")
		}
		return 1, 1, nil
	})
	var out bytes.Buffer
	if code := runScheduledClean([]string{"--every", "daily", "--low-space", "off"}, dir, &out); code != 1 {
		t.Fatalf("exit %d, want 1 when a category fails", code)
	}
	if rec := loadScheduleRecord(dir); !rec.LastSuccess.Equal(now) {
		t.Error("a run where other categories cleaned still counts as a success")
	}
}

func TestRunScheduledCleanAllFailedRetries(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "Duster")
	fakeScheduleRun(t, time.Now(), 40, func(CleanCategory) (int64, int, error) { return 0, 0, errors.New("no") })
	runScheduledClean([]string{"--every", "weekly", "--low-space", "10"}, dir, &bytes.Buffer{})
	if rec := loadScheduleRecord(dir); !rec.LastSuccess.IsZero() {
		t.Error("a run where everything failed must not count as a success")
	}
}

func TestScheduleRecordFile(t *testing.T) {
	t.Run("damaged file reads as no history", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "schedule.json"), []byte("{not json"), 0o600); err != nil {
			t.Fatal(err)
		}
		if rec := loadScheduleRecord(dir); rec.LastCheck != nil || !rec.LastSuccess.IsZero() {
			t.Errorf("damaged record read as %+v", rec)
		}
	})
	t.Run("round trip", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "Duster")
		now := time.Date(2026, 9, 24, 19, 0, 0, 0, time.UTC)
		in := scheduleRecord{LastSuccess: now, LastCheck: &scheduleCheck{Time: now, Result: "cleaned", FreePercent: 12.5}}
		if err := saveScheduleRecord(dir, in); err != nil {
			t.Fatal(err)
		}
		got := loadScheduleRecord(dir)
		if !got.LastSuccess.Equal(now) || got.LastCheck.FreePercent != 12.5 {
			t.Errorf("round trip: %+v", got)
		}
		if leftovers, _ := filepath.Glob(filepath.Join(dir, ".schedule-*")); len(leftovers) != 0 {
			t.Errorf("temp files left behind: %v", leftovers)
		}
	})
	t.Run("refuses a linked folder", func(t *testing.T) {
		target := t.TempDir()
		link := filepath.Join(t.TempDir(), "Duster")
		if err := os.Symlink(target, link); err != nil {
			t.Skipf("symlinks unsupported: %v", err)
		}
		if err := saveScheduleRecord(link, scheduleRecord{}); err == nil {
			t.Error("saved through a link")
		}
		if _, err := os.Stat(filepath.Join(target, "schedule.json")); err == nil {
			t.Error("the record landed in the link's target")
		}
		if err := os.WriteFile(filepath.Join(target, "schedule.json"), []byte(`{"last_success":"2026-01-01T00:00:00Z"}`), 0o600); err != nil {
			t.Fatal(err)
		}
		if rec := loadScheduleRecord(link); !rec.LastSuccess.IsZero() {
			t.Error("read a record through a link")
		}
	})
}
```

- [ ] **Step 2: Run to verify failure**

Run: `DU_NO_OPLOG=1 go test ./cmd -run 'RunScheduledClean|ScheduleRecord' 2>&1 | tail -5`
Expected: build failure, undefined `runScheduledClean`.

- [ ] **Step 3: Implement `cmd/schedule.go` (part 1)**

```go
package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const scheduleRecordName = "schedule.json"

// scheduleRecord is what scheduled runs remember, in logging.Dir()\schedule.json.
type scheduleRecord struct {
	LastCheck *scheduleCheck `json:"last_check,omitempty"`
	LastClean *scheduleClean `json:"last_clean,omitempty"`
	// LastSuccess is the last clean where at least one category did not fail.
	// The cadence counts from it, so a run where everything failed retries.
	LastSuccess time.Time `json:"last_success,omitempty"`
}

type scheduleCheck struct {
	Time        time.Time `json:"time"`
	Result      string    `json:"result"`       // cleaned, not due, or refused: <reason>
	FreePercent float64   `json:"free_percent"` // system drive; -1 when unknown
}

type scheduleClean struct {
	Time       time.Time                `json:"time"`
	Reason     string                   `json:"reason"`
	Freed      int64                    `json:"freed"`
	FreeBefore float64                  `json:"free_percent_before"`
	FreeAfter  float64                  `json:"free_percent_after"`
	Categories []scheduleCategoryResult `json:"categories"`
}

type scheduleCategoryResult struct {
	ID     string `json:"id"`
	Freed  int64  `json:"freed"`
	Files  int    `json:"files"`
	Status string `json:"status"` // cleaned, partial or failed
	Error  string `json:"error,omitempty"`
}

// The run engine's outside world; tests swap these.
var (
	scheduleCleanCategory = func(c CleanCategory) (int64, int, error) { return runCategory(c, false) }
	scheduleFreePercent   = func() float64 { return diskFreePercent(systemDriveRoot()) }
	scheduleNow           = time.Now
)

// loadScheduleRecord reads the record. A missing, damaged or linked one reads
// as no history: the worst case is one early clean.
func loadScheduleRecord(dir string) scheduleRecord {
	var rec scheduleRecord
	if dir == "" || !realDir(dir) {
		return rec
	}
	path := filepath.Join(dir, scheduleRecordName)
	if info, err := os.Lstat(path); err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
		return rec
	}
	b, err := os.ReadFile(path)
	if err != nil || json.Unmarshal(b, &rec) != nil {
		return scheduleRecord{}
	}
	return rec
}

// saveScheduleRecord replaces the record atomically (temp file + rename) in a
// folder that must not be a link.
func saveScheduleRecord(dir string, rec scheduleRecord) error {
	if dir == "" {
		return errors.New("no profile folder to keep the schedule record in")
	}
	if err := ensureRealDir(dir); err != nil {
		return err
	}
	b, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".schedule-*")
	if err != nil {
		return err
	}
	_, werr := tmp.Write(b)
	if err := errors.Join(werr, tmp.Close()); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	if err := os.Rename(tmp.Name(), filepath.Join(dir, scheduleRecordName)); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return nil
}

// runScheduledClean is `du schedule run`, what the task starts through duw.exe.
// It re-validates its arguments against the compiled-in policy, cleans when
// due, records the outcome and returns the process exit code.
func runScheduledClean(args []string, dir string, out io.Writer) int {
	now := scheduleNow()
	rec := loadScheduleRecord(dir)
	free := scheduleFreePercent()
	rec.LastCheck = &scheduleCheck{Time: now, FreePercent: free}

	save := func(code int) int {
		if err := saveScheduleRecord(dir, rec); err != nil {
			fmt.Fprintf(out, "could not save the schedule record: %v\n", err)
			return 1
		}
		return code
	}

	cfg, _, err := parseRunArgs(args)
	if err != nil {
		rec.LastCheck.Result = "refused: " + err.Error()
		fmt.Fprintln(out, rec.LastCheck.Result)
		return save(1)
	}
	due, reason := scheduleDue(now, rec.LastSuccess, cfg.Every, free, cfg.LowSpace)
	if !due {
		rec.LastCheck.Result = "not due"
		fmt.Fprintf(out, "not due (%.0f%% free)\n", free)
		return save(0)
	}

	fmt.Fprintf(out, "cleaning: %s\n", reason)
	clean := &scheduleClean{Time: now, Reason: reason, FreeBefore: free, Categories: []scheduleCategoryResult{}}
	anyOK, anyFailed := false, false
	for _, c := range scheduledCategories(cfg.Add) {
		freed, files, err := scheduleCleanCategory(c)
		r := scheduleCategoryResult{ID: c.ID, Freed: freed, Files: files, Status: scheduleResultStatus(err)}
		if err != nil {
			r.Error = err.Error()
		}
		if r.Status == "failed" {
			anyFailed = true
		} else {
			anyOK = true
		}
		clean.Freed += freed
		clean.Categories = append(clean.Categories, r)
		fmt.Fprintf(out, "  %-8s %-32s %s\n", r.Status, c.Name, strings.TrimSpace(formatBytes(freed)))
	}
	clean.FreeAfter = scheduleFreePercent()
	rec.LastClean = clean
	if anyOK {
		rec.LastSuccess = now
	}
	rec.LastCheck.Result = "cleaned"
	fmt.Fprintf(out, "freed %s\n", strings.TrimSpace(formatBytes(clean.Freed)))
	if anyFailed {
		return save(1)
	}
	return save(0)
}
```

- [ ] **Step 3b: Add `diskFreePercent` to `cmd/diskfree_windows.go`** (first caller: `scheduleFreePercent` below) (after `getDiskFreeBytesOS`)

```go
// diskFreePercent returns the free share (0-100) of the volume holding path,
// as this user sees it, or -1 when it cannot be read.
func diskFreePercent(path string) float64 {
	pathPtr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return -1
	}
	var freeAvail, totalBytes, totalFree uint64
	ret, _, _ := _procGetDiskFreeSpace.Call(
		uintptr(unsafe.Pointer(pathPtr)),
		uintptr(unsafe.Pointer(&freeAvail)),
		uintptr(unsafe.Pointer(&totalBytes)),
		uintptr(unsafe.Pointer(&totalFree)),
	)
	if ret == 0 || totalBytes == 0 {
		return -1
	}
	return float64(freeAvail) * 100 / float64(totalBytes)
}
```

Stub in `cmd/stubs.go`:

```go
func diskFreePercent(string) float64 { return -1 }
```

- [ ] **Step 4: Run tests and gates**

Run: `DU_NO_OPLOG=1 go test ./cmd -run 'RunScheduledClean|ScheduleRecord' -v 2>&1 | tail -30`, then all gates.
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/schedule.go cmd/schedule_test.go cmd/diskfree_windows.go cmd/stubs.go
git commit -m "feat(schedule): scheduled run engine and schedule.json record"
```

---

### Task 5: `du schedule` command surface (`cmd/schedule.go`, part 2)

**Files:**
- Modify: `cmd/schedule.go` (append)
- Create: `cmd/schedule_task_windows.go`; Modify: `cmd/stubs.go`
- Modify: `main.go` (register `cmd.ScheduleCmd` after `cmd.VdiskCmd`)
- Test: `cmd/schedule_test.go` (append)

**Interfaces:**
- Consumes: everything from Tasks 2-4; `decodeWSLOutput([]byte) string` (vdisk.go), `systemExecutable(string) string` (styles.go); `elevation.IsAdmin()` (lib/elevation), `logging.Dir()`, `systemDriveRoot()`, `formatBytes`, `runCategory`.
- Produces:
  - `var ScheduleCmd *cobra.Command` with subcommands `status`, `on`, `off`, hidden `run`
  - `type scheduleStatus struct` (JSON: `enabled`, `task_name`, `every`, `at`, `low_space_percent` (null when off), `categories`, `next_check`, `last_check`, `last_clean`, `task_command`, `warnings`, `notes`, `dry_run`, `would_clean`)
  - `func statusFromDoc(st scheduleStatus, doc taskDoc, now time.Time) scheduleStatus`
  - `func readScheduleStatus(name string, rec scheduleRecord, now time.Time) scheduleStatus`
  - `func renderScheduleStatus(w io.Writer, st scheduleStatus)`
  - `func removeScheduleAndLauncher(currentExe string)` (used by Task 7)
  - Windows (stubbed): `registerScheduleTask(name string, taskXML []byte) error`, `queryScheduleTask(name string) ([]byte, bool)`, `deleteScheduleTask(name string) error`, `listScheduleTaskNames() ([]string, error)`

- [ ] **Step 1: Write the failing tests** (append to `cmd/schedule_test.go`; add imports `"encoding/json"` and `"slices"`)

```go
func docFor(t *testing.T, cfg scheduleConfig, duw string) taskDoc {
	t.Helper()
	b, err := buildTaskXML(cfg, duw, "S-1-5-21-1", time.Date(2026, 9, 24, 8, 0, 0, 0, time.Local))
	if err != nil {
		t.Fatal(err)
	}
	doc, err := parseTaskXML(b)
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func TestStatusFromDoc(t *testing.T) {
	duw := filepath.Join(t.TempDir(), "duw.exe")
	if err := os.WriteFile(duw, []byte("MZ"), 0o755); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 24, 12, 0, 0, 0, time.Local)
	cfg := scheduleConfig{Every: "weekly", At: "19:00", LowSpace: 10, Add: []string{"npm"}}

	st := statusFromDoc(scheduleStatus{Warnings: []string{}}, docFor(t, cfg, duw), now)
	if !st.Enabled || st.Every != "weekly" || st.At != "19:00" || st.LowSpacePercent == nil || *st.LowSpacePercent != 10 {
		t.Errorf("status: %+v", st)
	}
	if !slices.Contains(st.Categories, "npm") || !slices.Contains(st.Categories, "temp") {
		t.Errorf("categories: %v", st.Categories)
	}
	if st.NextCheck == nil || !st.NextCheck.Equal(time.Date(2026, 9, 24, 19, 0, 0, 0, time.Local)) {
		t.Errorf("next check: %v", st.NextCheck)
	}
	if len(st.Warnings) != 0 || st.TaskCommand != duw {
		t.Errorf("warnings %v, command %q", st.Warnings, st.TaskCommand)
	}

	t.Run("missing duw.exe", func(t *testing.T) {
		st := statusFromDoc(scheduleStatus{}, docFor(t, cfg, filepath.Join(t.TempDir(), "gone.exe")), now)
		if len(st.Warnings) != 1 || !strings.Contains(st.Warnings[0], "no longer exists") {
			t.Errorf("warnings: %v", st.Warnings)
		}
	})
	t.Run("disabled in Task Scheduler", func(t *testing.T) {
		doc := docFor(t, cfg, duw)
		off := false
		doc.Settings.Enabled = &off
		st := statusFromDoc(scheduleStatus{}, doc, now)
		if st.NextCheck != nil || len(st.Warnings) != 1 || !strings.Contains(st.Warnings[0], "disabled") {
			t.Errorf("disabled: next %v, warnings %v", st.NextCheck, st.Warnings)
		}
	})
	t.Run("arguments edited by hand", func(t *testing.T) {
		doc := docFor(t, cfg, duw)
		doc.Actions.Arguments = "schedule run --every weekly --add recycle"
		st := statusFromDoc(scheduleStatus{}, doc, now)
		if len(st.Warnings) == 0 || !strings.Contains(st.Warnings[0], "refuse") {
			t.Errorf("edited: %v", st.Warnings)
		}
	})
	t.Run("low space off is null in JSON", func(t *testing.T) {
		st := statusFromDoc(scheduleStatus{}, docFor(t, scheduleConfig{Every: "daily", At: "03:00"}, duw), now)
		b, _ := json.Marshal(st)
		if !strings.Contains(string(b), `"low_space_percent":null`) {
			t.Errorf("json: %s", b)
		}
	})
}

func TestRenderScheduleStatus(t *testing.T) {
	t.Setenv("LOCALAPPDATA", t.TempDir())
	low := 10
	next := time.Date(2026, 9, 25, 19, 0, 0, 0, time.Local)
	st := scheduleStatus{
		Enabled: true, Every: "weekly", At: "19:00", LowSpacePercent: &low,
		Categories: []string{"temp", "npm"}, NextCheck: &next, Warnings: []string{"w1"},
		LastClean: &scheduleClean{Time: next.Add(-72 * time.Hour), Reason: "weekly", Freed: 1 << 30,
			Categories: []scheduleCategoryResult{{ID: "temp", Status: "partial"}, {ID: "npm", Status: "cleaned"}}},
		LastCheck: &scheduleCheck{Time: next.Add(-24 * time.Hour), Result: "not due", FreePercent: 38},
	}
	var b bytes.Buffer
	renderScheduleStatus(&b, st)
	got := b.String()
	for _, want := range []string{
		"Scheduled clean: ON",
		"Cleans weekly, or early when",
		"Checks daily at 19:00.",
		"Temporary Files", "+ ",
		"Next check:  Fri Sep 25, 19:00",
		"Last clean:  Tue Sep 22, 19:00 (weekly): freed 1",
		"1 partly skipped",
		"Last check:  Thu Sep 24, 19:00: not due",
		"38% free",
		"Warning: w1",
		"Turn off with: du schedule off",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("status text lacks %q:\n%s", want, got)
		}
	}
	b.Reset()
	renderScheduleStatus(&b, scheduleStatus{})
	if !strings.Contains(b.String(), "Scheduled clean: OFF") || !strings.Contains(b.String(), "du schedule on") {
		t.Errorf("off text:\n%s", b.String())
	}
}
```

Note: check the real `Name` of category `temp` in `getCategories()` (clean.go:101) and use it in place of `"Temporary Files"` if it differs.

- [ ] **Step 2: Run to verify failure**

Run: `DU_NO_OPLOG=1 go test ./cmd -run 'StatusFromDoc|RenderScheduleStatus' 2>&1 | tail -5`
Expected: build failure.

- [ ] **Step 3: Append the command surface to `cmd/schedule.go`**

Add imports: `"os/user"`, `"slices"`, `"github.com/Nur-Adnan/duster/internal/logging"`, `"github.com/Nur-Adnan/duster/lib/elevation"`, `"github.com/Nur-Adnan/duster/lib/fs"`, `"github.com/spf13/cobra"`.

```go
var (
	schedJSON      bool
	schedEvery     string
	schedAt        string
	schedLowSpace  string
	schedAdd       []string
	schedDryRun    bool
	schedUninstall bool
)

var ScheduleCmd = &cobra.Command{
	Use:   "schedule",
	Short: "Clean caches automatically on a schedule (status, on, off)",
	Long: `Keep caches tidy without opening Duster. A daily Task Scheduler check, as you,
never as administrator, cleans a safe set of caches when your chosen interval has
passed or the system drive runs low on space.

Always cleaned: temp files, browser caches (never cookies, history or sessions),
thumbnails, error reports, crash dumps and GPU shader caches. Developer and app
caches only with --add. The Recycle Bin, Recent files, Spotify and anything that
needs administrator rights are never scheduled.`,
	Args: cobra.NoArgs,
	Run:  func(*cobra.Command, []string) { executeScheduleStatus() },
}

var scheduleStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show whether scheduled cleaning is on and what the last run did",
	Args:  cobra.NoArgs,
	Run:   func(*cobra.Command, []string) { executeScheduleStatus() },
}

var scheduleOnCmd = &cobra.Command{
	Use:   "on",
	Short: "Create or replace your scheduled clean",
	Args:  cobra.NoArgs,
	Run:   func(*cobra.Command, []string) { executeScheduleOn() },
}

var scheduleOffCmd = &cobra.Command{
	Use:   "off",
	Short: "Delete your scheduled clean (the record of past runs is kept)",
	Args:  cobra.NoArgs,
	Run:   func(*cobra.Command, []string) { executeScheduleOff() },
}

// scheduleRunCmd is what the task starts through duw.exe. Its arguments go to
// parseRunArgs, the same parser status uses, so cobra's flag parsing is off.
var scheduleRunCmd = &cobra.Command{
	Use:                "run",
	Hidden:             true,
	DisableFlagParsing: true,
	Run: func(_ *cobra.Command, args []string) {
		os.Exit(runScheduledClean(args, logging.Dir(), os.Stdout))
	},
}

func init() {
	ScheduleCmd.PersistentFlags().BoolVar(&schedJSON, "json", false, "Print machine-readable JSON")
	scheduleOnCmd.Flags().StringVar(&schedEvery, "every", "weekly", "How often to clean: daily, weekly or monthly")
	scheduleOnCmd.Flags().StringVar(&schedAt, "at", "19:00", "Daily check time, HH:MM on a 24-hour clock")
	scheduleOnCmd.Flags().StringVar(&schedLowSpace, "low-space", "10%", "Clean early when the system drive has less free space than this (1%-50%, or off)")
	scheduleOnCmd.Flags().StringSliceVar(&schedAdd, "add", nil, "Opt-in categories to include (e.g. npm,gradle,docker)")
	scheduleOnCmd.Flags().BoolVar(&schedDryRun, "dry-run", false, "Show what would be registered and what a run would clean now; change nothing")
	scheduleOffCmd.Flags().BoolVar(&schedUninstall, "uninstall", false, "Delete every Duster scheduled clean this account can see (used by the uninstaller)")
	ScheduleCmd.AddCommand(scheduleStatusCmd, scheduleOnCmd, scheduleOffCmd, scheduleRunCmd)
}

// scheduleStatus is what status, on and off report, as text or --json.
type scheduleStatus struct {
	Enabled         bool                     `json:"enabled"`
	TaskName        string                   `json:"task_name"`
	Every           string                   `json:"every,omitempty"`
	At              string                   `json:"at,omitempty"`
	LowSpacePercent *int                     `json:"low_space_percent"`
	Categories      []string                 `json:"categories"`
	NextCheck       *time.Time               `json:"next_check"`
	LastCheck       *scheduleCheck           `json:"last_check"`
	LastClean       *scheduleClean           `json:"last_clean"`
	TaskCommand     string                   `json:"task_command,omitempty"`
	Warnings        []string                 `json:"warnings"`
	Notes           []string                 `json:"notes,omitempty"`
	DryRun          bool                     `json:"dry_run,omitempty"`
	WouldClean      []scheduleCategoryResult `json:"would_clean,omitempty"`
}

// currentScheduleTask returns this account's task name.
func currentScheduleTask() (name string, u *user.User, err error) {
	u, err = user.Current()
	if err != nil {
		return "", nil, fmt.Errorf("cannot tell which account this is: %w", err)
	}
	return scheduleTaskName(u.Username), u, nil
}

// readScheduleStatus combines the task (the settings) with the record (the history).
func readScheduleStatus(name string, rec scheduleRecord, now time.Time) scheduleStatus {
	st := scheduleStatus{TaskName: name, LastCheck: rec.LastCheck, LastClean: rec.LastClean,
		Categories: []string{}, Warnings: []string{}}
	raw, found := queryScheduleTask(name)
	if !found {
		return st
	}
	doc, err := parseTaskXML(raw)
	if err != nil {
		st.Enabled = true
		st.Warnings = append(st.Warnings, "the task exists but Duster cannot read it: "+err.Error())
		return st
	}
	return statusFromDoc(st, doc, now)
}

// statusFromDoc fills st from the task definition. Settings come from the
// task itself (there is no config file), read with the parser `run` uses.
func statusFromDoc(st scheduleStatus, doc taskDoc, now time.Time) scheduleStatus {
	st.Enabled = true
	if st.Categories == nil {
		st.Categories = []string{}
	}
	if st.Warnings == nil {
		st.Warnings = []string{}
	}
	st.TaskCommand = strings.Trim(doc.Actions.Command, `"`)
	fields := strings.Fields(doc.Actions.Arguments)
	if len(fields) < 2 || fields[0] != "schedule" || fields[1] != "run" {
		st.Warnings = append(st.Warnings, "the task was changed outside Duster and no longer runs a scheduled clean: run du schedule on again")
		return st
	}
	cfg, _, err := parseRunArgs(fields[2:])
	if err != nil {
		st.Warnings = append(st.Warnings, "scheduled runs will refuse to clean because the task's settings were changed ("+err.Error()+"): run du schedule on again")
		return st
	}
	st.Every = cfg.Every
	if len(doc.Start) >= 16 {
		st.At = doc.Start[11:16]
	}
	if cfg.LowSpace > 0 {
		st.LowSpacePercent = &cfg.LowSpace
	}
	for _, c := range scheduledCategories(cfg.Add) {
		st.Categories = append(st.Categories, c.ID)
	}
	if doc.Settings.Enabled != nil && !*doc.Settings.Enabled {
		st.Warnings = append(st.Warnings, "the task is disabled in Task Scheduler, so it never runs")
	} else if _, err := parseAt(st.At); err == nil {
		next := nextScheduleCheck(now, st.At)
		st.NextCheck = &next
	}
	if info, err := os.Lstat(st.TaskCommand); err != nil || !info.Mode().IsRegular() {
		st.Warnings = append(st.Warnings, fmt.Sprintf("the task runs %s, which no longer exists: run du schedule on again", st.TaskCommand))
	}
	return st
}

const scheduleTimeLayout = "Mon Jan 2, 15:04"

func renderScheduleStatus(w io.Writer, st scheduleStatus) {
	drive := strings.TrimSuffix(systemDriveRoot(), `\`)
	if st.Enabled {
		fmt.Fprintln(w, "Scheduled clean: ON")
		early := ""
		if st.LowSpacePercent != nil {
			early = fmt.Sprintf(", or early when %s has under %d%% free", drive, *st.LowSpacePercent)
		}
		fmt.Fprintf(w, "  Cleans %s%s. Checks daily at %s.\n", st.Every, early, st.At)
		fmt.Fprintf(w, "  Cleans: %s\n", scheduleCategoryList(st.Categories))
		if st.NextCheck != nil {
			fmt.Fprintf(w, "  Next check:  %s\n", st.NextCheck.Local().Format(scheduleTimeLayout))
		}
	} else {
		fmt.Fprintln(w, "Scheduled clean: OFF")
	}
	if c := st.LastClean; c != nil {
		fmt.Fprintf(w, "  Last clean:  %s (%s): freed %s, %s\n", c.Time.Local().Format(scheduleTimeLayout),
			c.Reason, strings.TrimSpace(formatBytes(c.Freed)), scheduleCleanOutcome(c))
	}
	if c := st.LastCheck; c != nil {
		free := ""
		if c.FreePercent >= 0 {
			free = fmt.Sprintf(" (%s %.0f%% free)", drive, c.FreePercent)
		}
		fmt.Fprintf(w, "  Last check:  %s: %s%s\n", c.Time.Local().Format(scheduleTimeLayout), c.Result, free)
	}
	if d := logging.Dir(); d != "" {
		fmt.Fprintf(w, "  Log:         %s\n", filepath.Join(d, "schedule.log"))
	}
	for _, n := range st.Notes {
		fmt.Fprintf(w, "  Note: %s\n", n)
	}
	for _, warn := range st.Warnings {
		fmt.Fprintf(w, "  Warning: %s\n", warn)
	}
	if st.Enabled {
		fmt.Fprintln(w, "Turn off with: du schedule off")
	} else {
		fmt.Fprintln(w, "Turn on with: du schedule on")
	}
}

// scheduleCategoryList names the safe set, then " + " and the opt-ins.
func scheduleCategoryList(ids []string) string {
	names := scheduleCategoryNames()
	var safe, extra []string
	for _, id := range ids {
		if slices.Contains(scheduleSafe, id) {
			safe = append(safe, names[id])
		} else {
			extra = append(extra, names[id])
		}
	}
	s := strings.Join(safe, ", ")
	if len(extra) > 0 {
		s += " + " + strings.Join(extra, ", ")
	}
	return s
}

func scheduleCleanOutcome(c *scheduleClean) string {
	partial, failed := 0, 0
	for _, r := range c.Categories {
		switch r.Status {
		case "partial":
			partial++
		case "failed":
			failed++
		}
	}
	var parts []string
	if partial > 0 {
		parts = append(parts, fmt.Sprintf("%d partly skipped (files in use)", partial))
	}
	if failed > 0 {
		parts = append(parts, fmt.Sprintf("%d failed", failed))
	}
	if len(parts) == 0 {
		return "no errors"
	}
	return strings.Join(parts, ", ")
}

func printScheduleStatus(st scheduleStatus) {
	if schedJSON {
		b, _ := json.MarshalIndent(st, "", "  ")
		fmt.Println(string(b))
		return
	}
	renderScheduleStatus(os.Stdout, st)
}

func scheduleFail(err error) {
	if schedJSON {
		b, _ := json.MarshalIndent(map[string]string{"error": err.Error()}, "", "  ")
		fmt.Println(string(b))
	} else {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
	}
	os.Exit(1)
}

func executeScheduleStatus() {
	name, _, err := currentScheduleTask()
	if err != nil {
		scheduleFail(err)
	}
	printScheduleStatus(readScheduleStatus(name, loadScheduleRecord(logging.Dir()), time.Now()))
}

func executeScheduleOn() {
	every, err := parseEvery(schedEvery)
	if err != nil {
		scheduleFail(err)
	}
	at, err := parseAt(schedAt)
	if err != nil {
		scheduleFail(err)
	}
	low, err := parseLowSpace(schedLowSpace)
	if err != nil {
		scheduleFail(err)
	}
	add, notes, err := resolveAdd(schedAdd)
	if err != nil {
		scheduleFail(err)
	}
	cfg := scheduleConfig{Every: every, At: at, LowSpace: low, Add: add}

	exe, err := os.Executable()
	if err != nil {
		scheduleFail(err)
	}
	duw := filepath.Join(filepath.Dir(exe), "duw.exe")
	if info, err := os.Lstat(duw); err != nil || !info.Mode().IsRegular() {
		scheduleFail(fmt.Errorf("duw.exe is missing from %s: run du update or reinstall Duster", filepath.Dir(exe)))
	}
	name, u, err := currentScheduleTask()
	if err != nil {
		scheduleFail(err)
	}
	now := time.Now()
	taskXML, err := buildTaskXML(cfg, duw, u.Uid, now)
	if err != nil {
		scheduleFail(err)
	}

	rec := loadScheduleRecord(logging.Dir())
	if schedDryRun {
		doc, err := parseTaskXML(taskXML)
		if err != nil {
			scheduleFail(err)
		}
		st := statusFromDoc(scheduleStatus{TaskName: name, LastCheck: rec.LastCheck, LastClean: rec.LastClean}, doc, now)
		st.DryRun, st.Notes = true, notes
		for _, c := range scheduledCategories(cfg.Add) {
			size, files, _ := runCategory(c, true)
			st.WouldClean = append(st.WouldClean, scheduleCategoryResult{ID: c.ID, Freed: size, Files: files, Status: "would clean"})
		}
		if !schedJSON {
			fmt.Println("Dry run: nothing was registered. A run now would clean:")
			names := scheduleCategoryNames()
			for _, r := range st.WouldClean {
				fmt.Printf("  %-32s %s\n", names[r.ID], strings.TrimSpace(formatBytes(r.Freed)))
			}
			fmt.Println()
		}
		printScheduleStatus(st)
		return
	}

	if err := registerScheduleTask(name, taskXML); err != nil {
		scheduleFail(err)
	}
	st := readScheduleStatus(name, rec, now)
	st.Notes = notes
	if elevation.IsAdmin() {
		st.Warnings = append(st.Warnings, fmt.Sprintf("registered for %s; runs without administrator rights", u.Username))
	}
	printScheduleStatus(st)
}

func executeScheduleOff() {
	if schedUninstall {
		names, err := listScheduleTaskNames()
		if err != nil {
			scheduleFail(err)
		}
		var errs []error
		for _, n := range names {
			errs = append(errs, deleteScheduleTask(n))
		}
		if err := errors.Join(errs...); err != nil {
			scheduleFail(err)
		}
		if !schedJSON {
			fmt.Printf("Deleted %d scheduled clean(s).\n", len(names))
		} else {
			b, _ := json.MarshalIndent(map[string][]string{"deleted": append([]string{}, names...)}, "", "  ")
			fmt.Println(string(b))
		}
		return
	}
	name, _, err := currentScheduleTask()
	if err != nil {
		scheduleFail(err)
	}
	if _, found := queryScheduleTask(name); !found {
		if !schedJSON {
			fmt.Println("Scheduled clean is already off.")
			return
		}
	} else if err := deleteScheduleTask(name); err != nil {
		scheduleFail(err)
	} else if !schedJSON {
		fmt.Printf("Scheduled clean is off. The record of past runs stays in %s.\n", filepath.Join(logging.Dir(), scheduleRecordName))
		return
	}
	printScheduleStatus(readScheduleStatus(name, loadScheduleRecord(logging.Dir()), time.Now()))
}

// removeScheduleAndLauncher deletes this account's scheduled clean and the
// duw.exe beside du.exe, for `du remove`. Best-effort: a task left behind
// only fails to start.
func removeScheduleAndLauncher(currentExe string) {
	if name, _, err := currentScheduleTask(); err == nil {
		if _, found := queryScheduleTask(name); found {
			_ = deleteScheduleTask(name)
		}
	}
	if duw := filepath.Join(filepath.Dir(currentExe), "duw.exe"); fs.IsValidPath(duw) {
		_ = os.Remove(duw)
	}
}
```

Then in `main.go` after `rootCmd.AddCommand(cmd.VdiskCmd)`: `rootCmd.AddCommand(cmd.ScheduleCmd)`.

If any existing test counts or lists root commands (grep `VdiskCmd` and `"vdisk"` in `*_test.go` and `e2e/`), add `schedule` there too.

- [ ] **Step 3b: Implement `cmd/schedule_task_windows.go`** (its first callers are in this task)

```go
//go:build windows

package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// schtasksCommand runs System32's schtasks.exe (never a PATH lookup).
func schtasksCommand(args ...string) *exec.Cmd {
	return exec.Command(systemExecutable("schtasks.exe"), args...) // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command
}

// registerScheduleTask creates or replaces the task. The XML goes through a
// private temporary file that is deleted afterwards.
func registerScheduleTask(name string, taskXML []byte) error {
	f, err := os.CreateTemp("", "duster-task-*.xml")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	_, werr := f.Write(taskXML)
	if cerr := f.Close(); werr == nil {
		werr = cerr
	}
	if werr != nil {
		return werr
	}
	out, err := schtasksCommand("/Create", "/XML", f.Name(), "/TN", name, "/F").CombinedOutput()
	if err != nil {
		return fmt.Errorf("schtasks could not create the task: %s", strings.TrimSpace(decodeWSLOutput(out)))
	}
	return nil
}

// queryScheduleTask returns the task's XML, or false when it does not exist
// (or cannot be read, which status treats the same way).
func queryScheduleTask(name string) ([]byte, bool) {
	out, err := schtasksCommand("/Query", "/TN", name, "/XML").Output()
	if err != nil {
		return nil, false
	}
	return out, true
}

func deleteScheduleTask(name string) error {
	out, err := schtasksCommand("/Delete", "/TN", name, "/F").CombinedOutput()
	if err != nil {
		return fmt.Errorf("schtasks could not delete %q: %s", name, strings.TrimSpace(decodeWSLOutput(out)))
	}
	return nil
}

// listScheduleTaskNames returns every Duster task this account can see.
func listScheduleTaskNames() ([]string, error) {
	out, err := schtasksCommand("/Query", "/FO", "CSV", "/NH").Output()
	if err != nil {
		return nil, fmt.Errorf("schtasks could not list tasks: %w", err)
	}
	return parseTaskNames(out), nil
}
```

- [ ] **Step 3c: Stubs in `cmd/stubs.go`** (append; add `"errors"` to its imports if missing)

```go
var errScheduleNeedsWindows = errors.New("scheduled cleaning needs Windows")

func registerScheduleTask(string, []byte) error { return errScheduleNeedsWindows }
func queryScheduleTask(string) ([]byte, bool)   { return nil, false }
func deleteScheduleTask(string) error           { return errScheduleNeedsWindows }
func listScheduleTaskNames() ([]string, error)  { return nil, errScheduleNeedsWindows }
```

- [ ] **Step 4: Run tests and gates**

Run: `DU_NO_OPLOG=1 go test ./cmd -run 'Schedule|StatusFromDoc' -v 2>&1 | tail -30`, `go build ./... && GOOS=windows go build ./...`, `go run . schedule --help | head -5`, then all gates. No staticcheck U1000 may remain now.
Expected: PASS; help prints. On macOS `go run . schedule` prints OFF (stubs), which is fine.

- [ ] **Step 5: Commit**

```bash
git add cmd/schedule.go cmd/schedule_test.go cmd/schedule_task_windows.go cmd/stubs.go main.go
git commit -m "feat(schedule): du schedule status, on, off and run"
```

---

### Task 6: `duw.exe` windowless launcher

**Files:**
- Create: `launcher/duw/main.go`, `launcher/duw/hide_windows.go`, `launcher/duw/hide_other.go`
- Test: `launcher/duw/main_test.go`

**Interfaces:**
- Consumes: `logging.Dir()` (internal/logging).
- Produces: a binary; functions `allowedArgs([]string) bool`, `openScheduleLog(dir string) (*os.File, error)`, `run(args []string, exe string, logDir string) int`.

- [ ] **Step 1: Write the failing tests** (`launcher/duw/main_test.go`)

```go
package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAllowedArgs(t *testing.T) {
	for _, args := range [][]string{
		{"schedule", "run"},
		{"schedule", "run", "--every", "weekly", "--low-space", "10"},
	} {
		if !allowedArgs(args) {
			t.Errorf("refused %v", args)
		}
	}
	for _, args := range [][]string{
		nil, {"schedule"}, {"clean", "--yes"}, {"schedule", "on"}, {"remove", "--force"}, {"run", "schedule"},
	} {
		if allowedArgs(args) {
			t.Errorf("accepted %v", args)
		}
	}
}

func TestRunRefusesOtherCommandsWithoutStartingDu(t *testing.T) {
	dir := t.TempDir()
	if code := run([]string{"clean", "--yes"}, filepath.Join(dir, "duw.exe"), dir); code != 2 {
		t.Errorf("exit %d, want 2", code)
	}
	if _, err := os.Stat(filepath.Join(dir, "schedule.log")); err == nil {
		t.Error("a refused call wrote the log")
	}
}

func TestRunReportsMissingDu(t *testing.T) {
	dir := t.TempDir()
	code := run([]string{"schedule", "run"}, filepath.Join(dir, "duw.exe"), dir)
	if code == 0 {
		t.Error("exit 0 without a du.exe beside duw.exe")
	}
	b, _ := os.ReadFile(filepath.Join(dir, "schedule.log"))
	if !strings.Contains(string(b), "du.exe") {
		t.Errorf("log does not say what failed: %q", b)
	}
}

func TestOpenScheduleLogRotates(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "schedule.log")
	if err := os.WriteFile(path, make([]byte, maxLogBytes+1), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := openScheduleLog(dir)
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	if info, err := os.Stat(path + ".old"); err != nil || info.Size() != maxLogBytes+1 {
		t.Errorf("old log not kept: %v", err)
	}
	if info, _ := os.Stat(path); info.Size() != 0 {
		t.Errorf("new log has %d bytes", info.Size())
	}
}

func TestOpenScheduleLogRefusesLinks(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(t.TempDir(), "elsewhere.txt")
	if err := os.WriteFile(target, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(dir, "schedule.log")); err != nil {
		t.Skipf("symlinks unsupported: %v", err)
	}
	if f, err := openScheduleLog(dir); err == nil {
		f.Close()
		t.Error("opened a log through a link")
	}

	linkedDir := filepath.Join(t.TempDir(), "Duster")
	if err := os.Symlink(t.TempDir(), linkedDir); err != nil {
		t.Skipf("symlinks unsupported: %v", err)
	}
	if f, err := openScheduleLog(linkedDir); err == nil {
		f.Close()
		t.Error("opened a log in a linked folder")
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./launcher/duw 2>&1 | tail -5`
Expected: build failure.

- [ ] **Step 3: Implement**

`launcher/duw/main.go`:

```go
// Command duw is Duster's windowless launcher. Task Scheduler starts it for a
// scheduled clean; it is built with -H=windowsgui, so Windows gives it no
// console, and it starts du.exe from its own folder without one, appending
// du's output to %LOCALAPPDATA%\Duster\schedule.log. It only ever runs
// `du.exe schedule run ...`: it is not a general hidden runner.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/Nur-Adnan/duster/internal/logging"
)

const maxLogBytes = 1 << 20

func main() {
	exe, err := os.Executable()
	if err != nil {
		os.Exit(1)
	}
	os.Exit(run(os.Args[1:], exe, logging.Dir()))
}

func allowedArgs(args []string) bool {
	return len(args) >= 2 && args[0] == "schedule" && args[1] == "run"
}

func run(args []string, exe, logDir string) int {
	if !allowedArgs(args) {
		return 2
	}
	var logw io.Writer = io.Discard
	if f, err := openScheduleLog(logDir); err == nil {
		defer f.Close()
		logw = f
	}
	fmt.Fprintf(logw, "=== %s %s\n", time.Now().Format(time.RFC3339), strings.Join(args, " "))

	// du.exe from duw.exe's own folder, never a PATH lookup.
	du := filepath.Join(filepath.Dir(exe), "du.exe")
	c := exec.Command(du, args...) // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command
	c.Stdout, c.Stderr = logw, logw
	hideWindow(c)
	err := c.Run()
	var exitErr *exec.ExitError
	switch {
	case err == nil:
		return 0
	case errors.As(err, &exitErr):
		return exitErr.ExitCode()
	default:
		fmt.Fprintf(logw, "could not start %s: %v\n", du, err)
		return 1
	}
}

// openScheduleLog opens schedule.log for appending, keeping one rotated .old
// once it passes 1 MB. It refuses a linked folder or log, so a planted link
// cannot redirect the writes.
func openScheduleLog(dir string) (*os.File, error) {
	if dir == "" {
		return nil, errors.New("no profile folder")
	}
	if err := os.Mkdir(dir, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
		return nil, err
	}
	if info, err := os.Lstat(dir); err != nil || !info.IsDir() || info.Mode()&(os.ModeSymlink|os.ModeIrregular) != 0 {
		return nil, fmt.Errorf("%s is not a plain folder", dir)
	}
	path := filepath.Join(dir, "schedule.log")
	if info, err := os.Lstat(path); err == nil {
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("%s is not a plain file", path)
		}
		if info.Size() > maxLogBytes {
			_ = os.Remove(path + ".old")
			if err := os.Rename(path, path+".old"); err != nil {
				return nil, err
			}
		}
	}
	return os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
}
```

`launcher/duw/hide_windows.go`:

```go
//go:build windows

package main

import (
	"os/exec"
	"syscall"
)

// createNoWindow is CREATE_NO_WINDOW: du.exe gets no console, so neither
// conhost nor Windows Terminal opens a window.
const createNoWindow = 0x08000000

func hideWindow(c *exec.Cmd) {
	c.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
}
```

`launcher/duw/hide_other.go`:

```go
//go:build !windows

package main

import "os/exec"

func hideWindow(*exec.Cmd) {}
```

- [ ] **Step 4: Run tests, build and gates**

Run: `go test ./launcher/duw -v 2>&1 | tail -20`, `GOOS=windows go build -ldflags "-H=windowsgui" -o /tmp/duw.exe ./launcher/duw && rm /tmp/duw.exe`, then all gates.
Expected: PASS; builds.

- [ ] **Step 5: Commit**

```bash
git add launcher/duw
git commit -m "feat(schedule): duw.exe windowless launcher"
```

---

### Task 7: `du update` installs `duw.exe`; `du remove` removes the task and `duw.exe`

**Files:**
- Modify: `cmd/update.go` (`extractBinaryFromZip` → `extractFileFromZip`, `downloadVerifiedBinary`, `swapBinary`, `downloadCompleteMsg`, `runDownloadBinaryCmd`, `runSwapBinaryCmd`, headless path near line 680)
- Modify: `cmd/update_extract_test.go`
- Modify: `cmd/remove.go` (`runUninstallCmd`, `runSilentRemove`, `runHeadlessRemove`)
- Test: `cmd/update_replace_test.go` (new)

**Interfaces:**
- Consumes: `removeScheduleAndLauncher(currentExe string)` (Task 5), `scheduleDelayedDelete`.
- Produces: `type releaseBinaries struct{ du, duw []byte }`; `func extractFileFromZip(archive []byte, name string) ([]byte, error)` with sentinel `errNotInArchive`; `func replaceFile(target string, data []byte) (old string, err error)`; `func swapBinary(bins releaseBinaries) error`.

- [ ] **Step 1: Write the failing tests**

Update every `extractBinaryFromZip(x)` in `cmd/update_extract_test.go` to `extractFileFromZip(x, "du.exe")`, and append there:

```go
func TestExtractLauncherFromZip(t *testing.T) {
	mz := []byte("MZ launcher")
	got, err := extractFileFromZip(makeZip(t, map[string][]byte{
		"Duster-1.3.0-Portable-x64/du.exe":  []byte("MZ du"),
		"Duster-1.3.0-Portable-x64/duw.exe": mz,
	}), "duw.exe")
	if err != nil || !bytes.Equal(got, mz) {
		t.Fatalf("duw.exe: %q, %v", got, err)
	}
	// Releases before scheduled cleaning have no duw.exe: not an error for update.
	_, err = extractFileFromZip(makeZip(t, map[string][]byte{"du.exe": []byte("MZ du")}), "duw.exe")
	if !errors.Is(err, errNotInArchive) {
		t.Errorf("missing duw.exe: %v, want errNotInArchive", err)
	}
}
```

(Add `"bytes"`/`"errors"` imports if the file lacks them.)

`cmd/update_replace_test.go`:

```go
package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReplaceFile(t *testing.T) {
	t.Run("replaces and keeps the original aside", func(t *testing.T) {
		target := filepath.Join(t.TempDir(), "duw.exe")
		if err := os.WriteFile(target, []byte("old"), 0o755); err != nil {
			t.Fatal(err)
		}
		old, err := replaceFile(target, []byte("new"))
		if err != nil {
			t.Fatal(err)
		}
		if b, _ := os.ReadFile(target); string(b) != "new" {
			t.Errorf("target holds %q", b)
		}
		if b, _ := os.ReadFile(old); old != target+".old" || string(b) != "old" {
			t.Errorf("original at %q holds %q", old, b)
		}
		if _, err := os.Stat(target + ".new"); err == nil {
			t.Error("staged file left behind")
		}
	})
	t.Run("installs where there was none", func(t *testing.T) {
		target := filepath.Join(t.TempDir(), "duw.exe")
		old, err := replaceFile(target, []byte("new"))
		if err != nil || old != "" {
			t.Fatalf("old %q, err %v", old, err)
		}
		if b, _ := os.ReadFile(target); string(b) != "new" {
			t.Errorf("target holds %q", b)
		}
	})
	t.Run("a failed stage changes nothing", func(t *testing.T) {
		dir := t.TempDir()
		target := filepath.Join(dir, "duw.exe")
		if err := os.WriteFile(target, []byte("old"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(target+".new", 0o755); err != nil { // makes the staging write fail
			t.Fatal(err)
		}
		if _, err := replaceFile(target, []byte("new")); err == nil {
			t.Fatal("no error")
		}
		if b, _ := os.ReadFile(target); string(b) != "old" {
			t.Errorf("target changed to %q", b)
		}
	})
}
```

- [ ] **Step 2: Run to verify failure**

Run: `DU_NO_OPLOG=1 go test ./cmd -run 'Extract|ReplaceFile' 2>&1 | tail -5`
Expected: build failure.

- [ ] **Step 3: Implement in `cmd/update.go`**

Replace `extractBinaryFromZip` with (same body, parameterised; `errors` import needed):

```go
// errNotInArchive means the release archive lacks the requested file. For
// duw.exe that is normal: releases before scheduled cleaning did not ship it.
var errNotInArchive = errors.New("not in the release archive")

// extractFileFromZip pulls one executable (du.exe or duw.exe) out of the
// release archive, wherever it is nested.
func extractFileFromZip(archive []byte, name string) ([]byte, error) {
	reader, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		return nil, fmt.Errorf("release archive is not a valid zip: %w", err)
	}

	for _, f := range reader.File {
		entry := strings.ToLower(f.Name)
		if entry != name && !strings.HasSuffix(entry, "/"+name) {
			continue
		}
		// ... the existing size checks, read and MZ check, unchanged ...
		return data, nil
	}
	return nil, fmt.Errorf("%s: %w", name, errNotInArchive)
}
```

Keep the existing error strings for du.exe inside the loop. Then:

```go
// releaseBinaries are the verified executables from one release archive. duw
// is nil for releases that predate scheduled cleaning.
type releaseBinaries struct {
	du, duw []byte
}
```

`downloadVerifiedBinary(rel) (releaseBinaries, error)`: after the checksum check,

```go
	du, err := extractFileFromZip(archive, "du.exe")
	if err != nil {
		return releaseBinaries{}, err
	}
	duw, err := extractFileFromZip(archive, "duw.exe")
	if err != nil && !errors.Is(err, errNotInArchive) {
		return releaseBinaries{}, err
	}
	return releaseBinaries{du: du, duw: duw}, nil
```

(Early returns in that function become `return releaseBinaries{}, err`.)

Replace `swapBinary`:

```go
// swapBinary installs a verified release: duw.exe first, then du.exe. If duw.exe
// cannot be replaced nothing has changed yet; the launcher does not depend on
// du.exe's version, so a failure after it leaves a working install.
func swapBinary(bins releaseBinaries) error {
	currentExe, err := os.Executable()
	if err != nil {
		return err
	}
	if bins.duw != nil {
		old, err := replaceFile(filepath.Join(filepath.Dir(currentExe), "duw.exe"), bins.duw)
		if err != nil {
			return fmt.Errorf("cannot update duw.exe: %w", err)
		}
		if old != "" {
			scheduleDelayedDelete(old) // locked while a scheduled clean runs
		}
	}
	old, err := replaceFile(currentExe, bins.du)
	if err != nil {
		return err
	}
	// The old binary stays locked while this process runs; delete it after exit.
	scheduleDelayedDelete(old)
	logUpOperation("self-update", currentExe, int64(len(bins.du)), true)
	return nil
}

// replaceFile swaps target for data through a staged file beside it (same
// volume, so both renames are atomic), restoring the original if the swap
// fails. It returns where the original now is, or "" when there was none.
func replaceFile(target string, data []byte) (string, error) {
	staged, old := target+".new", target+".old"
	base := filepath.Base(target)
	if err := os.WriteFile(staged, data, 0o755); err != nil {
		return "", fmt.Errorf("cannot stage new %s: %w", base, err)
	}
	_ = os.Remove(old) // clear leftovers from a previous update
	moved := true
	if err := os.Rename(target, old); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			_ = os.Remove(staged)
			return "", fmt.Errorf("cannot move current %s aside: %w", base, err)
		}
		moved = false
	}
	if err := os.Rename(staged, target); err != nil {
		if moved {
			if rbErr := os.Rename(old, target); rbErr != nil {
				return "", fmt.Errorf("swap failed (%v) and rollback failed (%v): restore %s manually", err, rbErr, old)
			}
		}
		_ = os.Remove(staged)
		return "", fmt.Errorf("cannot activate new %s: %w", base, err)
	}
	if !moved {
		return "", nil
	}
	return old, nil
}
```

Callers: `downloadCompleteMsg{bytes []byte}` → `downloadCompleteMsg{bins releaseBinaries}`; `runDownloadBinaryCmd` returns `downloadCompleteMsg{bins: bins, err: err}`; `runSwapBinaryCmd(bins releaseBinaries)`; wherever the model stores the bytes between download and swap (find with `grep -n "bytes\b\|newBytes\|\.bytes" cmd/update.go`), store `releaseBinaries`. Headless (~line 682): `if bins, err := downloadVerifiedBinary(rel); err != nil { swapErr = err } else { swapErr = swapBinary(bins) }`. If any test used the old error strings "cannot stage new binary" / "cannot move current binary aside" / "cannot activate new binary", update it to the new `%s`-named forms (`grep -rn "stage new\|move current\|activate new" cmd`).

- [ ] **Step 4: `cmd/remove.go`**

In each of the three paths, right after `cleanDusterDir` succeeds and before `scheduleDelayedDelete(currentExe)`, and only when actually deleting (not dry-run), call `removeScheduleAndLauncher(currentExe)`:
- `runUninstallCmd`: inside `if !dryRun { removeScheduleAndLauncher(currentExe); scheduleDelayedDelete(currentExe) }`
- `runSilentRemove`: inside `if !rmDryRun { removeScheduleAndLauncher(currentExe); scheduleDelayedDelete(currentExe); os.Exit(0) }`
- `runHeadlessRemove`: inside `if err == nil { removeScheduleAndLauncher(currentExe); scheduleDelayedDelete(currentExe) }`

Add to `RemoveCmd`'s Long text: "It also deletes your scheduled clean, if any (du schedule)."

- [ ] **Step 5: Run tests and gates**

Run: `DU_NO_OPLOG=1 go test ./cmd -run 'Extract|ReplaceFile|Update|Remove' 2>&1 | tail -10`, then all gates.
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add cmd/update.go cmd/update_extract_test.go cmd/update_replace_test.go cmd/remove.go
git commit -m "feat(schedule): update installs duw.exe, remove deletes the task"
```

---

### Task 8: Ship `duw.exe` (packaging, installer, CI)

No Go code; verification is by CI. Every place that builds or packages `du.exe` also builds or packages `duw.exe`.

**Files:**
- Modify: `Makefile`, `scripts/build-release.sh`, `.github/workflows/release.yml`, `.github/workflows/ci.yml`, `.github/workflows/windows-smoke.yml` (Build step only), `installer/duster-setup.iss`, `scripts/install.ps1`, `docs/code-signing.md`

- [ ] **Step 1: Makefile**

Add under `LDFLAGS`:

```make
# duw.exe is the windowless launcher for scheduled cleans (launcher/duw):
# -H=windowsgui so Windows never gives it a console.
LAUNCHER_LDFLAGS = -ldflags="-s -w -H=windowsgui"
```

- `build`: add `GOOS=windows GOARCH=amd64 $(GO) build $(GOFLAGS) $(LAUNCHER_LDFLAGS) -o duw.exe ./launcher/duw`
- `build-amd64`: add `GOOS=windows GOARCH=amd64 $(GO) build $(GOFLAGS) $(LAUNCHER_LDFLAGS) -o $(DIST_DIR)/duw-windows-amd64.exe ./launcher/duw`
- `build-arm64`: same with arm64 → `$(DIST_DIR)/duw-windows-arm64.exe`
- `portable`: `@cp $(DIST_DIR)/duw-windows-amd64.exe $(DIST_DIR)/portable/Duster-$(VERSION)-Portable-x64/duw.exe` and the arm64 twin
- `installer`: `@cp $(DIST_DIR)/duw-windows-amd64.exe $(DIST_DIR)/installer/`
- `clean`: `@rm -f duw.exe`

- [ ] **Step 2: `scripts/build-release.sh`**

After each `go build ... duster-windows-<arch>.exe .`, add:

```bash
GOOS=windows GOARCH=<arch> CGO_ENABLED=0 \
    go build -trimpath -ldflags="-s -w -H=windowsgui" \
    -o "${DIST_DIR}/duw-windows-<arch>.exe" ./launcher/duw
```

In the portable section: `cp "${DIST_DIR}/duw-windows-amd64.exe" "${PORTABLE_AMD64}/duw.exe"` and the arm64 twin. In the installer section: `cp "${DIST_DIR}/duw-windows-amd64.exe" "${DIST_DIR}/installer/"`.

- [ ] **Step 3: `.github/workflows/release.yml`**

- `build` job, "Compile Binary", append:
  ```bash
          go build -trimpath -ldflags="-s -w -H=windowsgui" \
            -o duw-windows-${{ matrix.suffix }}.exe ./launcher/duw
  ```
- "Upload Unsigned Binary": `path:` becomes
  ```yaml
          path: |
            duster-windows-${{ matrix.suffix }}.exe
            duw-windows-${{ matrix.suffix }}.exe
  ```
- `sign` job, "Verify Signatures": the loop list becomes `'duster-windows-amd64.exe', 'duster-windows-arm64.exe', 'duw-windows-amd64.exe', 'duw-windows-arm64.exe'`.
- "Upload AMD64 Binary": `path:` becomes `signed/duster-windows-amd64.exe` and `signed/duw-windows-amd64.exe` (multi-line); same for ARM64.
- `release` job: in each "Create Portable ZIP" step after the `du.exe` copy: `cp dist/duw-windows-amd64.exe dist/portable/Duster-${{ steps.version.outputs.version }}-Portable-x64/duw.exe` (arm64 twin in the other step).
- The `installer` job already downloads the whole `duster-windows-amd64` artifact into `dist/`, which now includes `duw-windows-amd64.exe`: no change.

- [ ] **Step 4: `.github/workflows/ci.yml` and `windows-smoke.yml` Build steps**

ISCC now needs `dist\duw-windows-amd64.exe`. In ci.yml's "Setup Installer Test" after `go build -o dist\duster-windows-amd64.exe .`:

```powershell
          go build -ldflags "-H=windowsgui" -o dist\duw-windows-amd64.exe .\launcher\duw
          if ($LASTEXITCODE) { throw 'go build (duw) failed' }
```

In windows-smoke.yml's "Build" step after `Copy-Item du.exe dist\duster-windows-amd64.exe`:

```powershell
          go build -ldflags "-H=windowsgui" -o dist\duw-windows-amd64.exe .\launcher\duw
          if ($LASTEXITCODE) { throw 'go build (duw) failed' }
```

- [ ] **Step 5: `installer/duster-setup.iss`**

In `[Files]` after the main executable line:

```
; Windowless launcher for scheduled cleans (du schedule)
Source: "..\dist\duw-windows-amd64.exe"; DestDir: "{app}"; DestName: "duw.exe"; Flags: ignoreversion
```

New section after `[Run]`:

```
[UninstallRun]
; Delete the scheduled cleans that point at this install. Best-effort: a task
; left behind only fails to start.
Filename: "{app}\{#MyAppExeName}"; Parameters: "schedule off --uninstall"; Flags: runhidden waituntilterminated; RunOnceId: "RemoveScheduledClean"
```

- [ ] **Step 6: `scripts/install.ps1`**

Before `if ($DownloadMethod -eq "zip") {` (section 6), add `$TempLauncher = $null`. Inside the zip branch after `$DownloadedExePath = $TempExe.FullName`, add:

```powershell
    # duw.exe (scheduled cleans) sits beside du.exe in releases that have it.
    $TempLauncher = Join-Path $TempExe.DirectoryName "duw.exe"
```

After the `Copy-Item ... $ExePath` try/catch:

```powershell
if ($TempLauncher -and (Test-Path -LiteralPath $TempLauncher)) {
    Copy-Item -LiteralPath $TempLauncher -Destination (Join-Path $InstallDir "duw.exe") -Force
}
```

windows-smoke.yml's "Admin install path" step checks that a marker string occurs exactly once in install.ps1: do not add another occurrence of whatever string it searches for (read that step first).

- [ ] **Step 7: `docs/code-signing.md`**

Add `duw-windows-amd64.exe` and `duw-windows-arm64.exe` to the list of signed files, and two `<pe-file path="duw-windows-...exe"><authenticode-sign/></pe-file>` lines to the `binaries` artifact configuration, with one sentence: the SignPath project's `binaries` configuration must list them too, or the sign job's verification fails the release.

- [ ] **Step 8: Verify locally and commit**

Run: `make -n build-all portable | grep duw` (shows the new commands), `bash -n scripts/build-release.sh`, `python3 -c "import yaml,sys;[yaml.safe_load(open(f)) for f in sys.argv[1:]]" .github/workflows/release.yml .github/workflows/ci.yml .github/workflows/windows-smoke.yml`, then all gates.

```bash
git add Makefile scripts/build-release.sh scripts/install.ps1 installer/duster-setup.iss .github/workflows docs/code-signing.md
git commit -m "build: ship duw.exe in zips, installer, install.ps1 and releases"
```

---

### Task 9: Windows smoke steps, docs, spec touch-ups

**Files:**
- Modify: `.github/workflows/windows-smoke.yml` (new step before "Remove (self-uninstall)")
- Modify: `README.md`, `CHANGELOG.md`, `docs/security.md`, `docs/release-checklist.md`, `docs/superpowers/specs/2026-09-24-scheduled-cleaning-design.md`

- [ ] **Step 1: Smoke step** (insert before `- name: Remove (self-uninstall)`)

```yaml
      # Scheduled cleaning for real: register, let Task Scheduler start
      # duw.exe, which runs `du schedule run`, then turn it off; and a
      # standard (non-admin) user managing their own task.
      - name: Scheduled clean (register, run, turn off, standard user)
        timeout-minutes: 10
        run: |
          $dir = Join-Path $env:RUNNER_TEMP 'sched dir'
          New-Item -ItemType Directory -Force $dir | Out-Null
          Copy-Item du.exe "$dir\du.exe"
          Copy-Item dist\duw-windows-amd64.exe "$dir\duw.exe"
          $du = "$dir\du.exe"
          $data = Join-Path $env:LOCALAPPDATA 'Duster'
          $rec = Join-Path $data 'schedule.json'
          Remove-Item $rec -ErrorAction SilentlyContinue

          $j = & $du schedule on --dry-run --json | Out-String | ConvertFrom-Json
          if ($LASTEXITCODE -ne 0 -or -not $j.dry_run -or -not $j.would_clean) { throw "dry run: exit=$LASTEXITCODE" }
          $name = $j.task_name
          schtasks /Query /TN $name 2>$null | Out-Null
          if ($LASTEXITCODE -eq 0) { throw '--dry-run registered a task' }

          & $du schedule on --add recycle 2>$null | Out-Null
          if ($LASTEXITCODE -ne 1) { throw 'recycle was accepted' }

          $j = & $du schedule on --every daily --at 03:00 --add npm --json | Out-String | ConvertFrom-Json
          if ($LASTEXITCODE -ne 0 -or -not $j.enabled -or $j.every -ne 'daily' -or $j.categories -notcontains 'npm') { throw "on: $($j | ConvertTo-Json -Depth 5)" }

          schtasks /Run /TN $name | Out-Null
          for ($i = 0; $i -lt 180 -and -not (Test-Path $rec); $i++) { Start-Sleep 1 }
          Get-Content (Join-Path $data 'schedule.log') -ErrorAction SilentlyContinue | Write-Host
          if (-not (Test-Path $rec)) { throw 'the task did not record a run' }
          $j = & $du schedule --json | Out-String | ConvertFrom-Json
          if ($j.last_check.result -ne 'cleaned' -or -not $j.last_clean) { throw "after the run: $($j | ConvertTo-Json -Depth 6)" }

          $j = & $du schedule off --json | Out-String | ConvertFrom-Json
          if ($LASTEXITCODE -ne 0 -or $j.enabled) { throw 'off' }
          schtasks /Query /TN $name 2>$null | Out-Null
          if ($LASTEXITCODE -eq 0) { throw 'off left the task' }
          & $du schedule off | Out-Null
          if ($LASTEXITCODE -ne 0) { throw 'a second off failed' }

          # A standard user turns it on and off for themselves.
          $pw = 'Du!' + [guid]::NewGuid().ToString('N').Substring(0, 8) + 'aA1'   # net user prompts (Y/N) above 14 characters
          net user dsched $pw /add | Out-Null
          $cred = New-Object PSCredential('dsched', (ConvertTo-SecureString $pw -AsPlainText -Force))
          $pub = Join-Path $env:PUBLIC 'dsched'
          New-Item -ItemType Directory -Force $pub | Out-Null
          Copy-Item "$dir\du.exe", "$dir\duw.exe" $pub
          function AsUser($argList) {
            $p = Start-Process "$pub\du.exe" -Credential $cred -LoadUserProfile -ArgumentList $argList -Wait -PassThru `
              -WorkingDirectory $pub -RedirectStandardOutput "$pub\out.txt" -RedirectStandardError "$pub\err.txt"
            [pscustomobject]@{ Exit = $p.ExitCode; Out = (Get-Content "$pub\out.txt" -Raw); Err = (Get-Content "$pub\err.txt" -Raw) }
          }
          $r = AsUser @('schedule', 'on', '--json')
          if ($r.Exit -ne 0) { throw "standard user on: $($r.Out) $($r.Err)" }
          $r = AsUser @('schedule', '--json')
          if (-not ($r.Out | ConvertFrom-Json).enabled) { throw "standard user status: $($r.Out) $($r.Err)" }
          $r = AsUser @('schedule', 'off', '--json')
          if ($r.Exit -ne 0) { throw "standard user off: $($r.Out) $($r.Err)" }
          net user dsched /delete | Out-Null
          exit 0
```

Adjust anything the spike findings (end of this plan) contradict before pushing.

- [ ] **Step 2: Docs**

- `README.md`: add a command-table row next to `du vdisk`: `| \`du schedule\` | Cleans caches automatically: weekly (or daily/monthly) and early when the system drive runs low; never as administrator |`, plus a short `>` paragraph like the vdisk one covering: what is always cleaned, `--add` for developer/app caches, what is never scheduled and why, `du schedule off`, and that it runs silently through `duw.exe`.
- `CHANGELOG.md`, under `## [Unreleased]` `### Added`, first bullet: what `du schedule on/off/status` does, the defaults (weekly, 19:00 check, 10% low-space), the safe set, `--add`, the never-list with reasons, battery behaviour, `schedule.json`/`schedule.log`, `--dry-run`, `--json`, and that `du update`/the installer/`du remove` handle `duw.exe` and the task.
- `docs/security.md`: new `### I. Scheduled Cleaning` after `### H. Scan History`: least privilege (`LeastPrivilege`, `InteractiveToken`, never elevated), the policy re-applied on every run (edited task arguments are refused), `duw.exe` only runs `du.exe schedule run` from its own folder, no localized output parsed, the record and log refuse links, a task left behind after uninstall only fails to start.
- `docs/release-checklist.md` manual items: unplugged laptop skips the run; asleep at the check time catches up (StartWhenAvailable); Windows Terminal as default console shows no window; two users on one PC get separate tasks; uninstalling with the setup exe deletes the task.
- Spec: fold in the plan's "Deviations from the spec" section (§2 table gets `cmd/schedule_task.go`; §5 due table reason text and `last_success`; §6 task name hash and quoted command; §3 `--uninstall` enumeration and `task_name`), and change its Status line to "approved; implemented on feat/scheduled-clean".

- [ ] **Step 3: Gates and commit**

Run all gates; `python3 -c "import yaml;yaml.safe_load(open('.github/workflows/windows-smoke.yml'))"`.

```bash
git add .github/workflows/windows-smoke.yml README.md CHANGELOG.md docs
git commit -m "docs(schedule): smoke steps, README, changelog, security notes"
```

---

### After Task 9 (controller)

1. Whole-branch review (fresh reviewer on the full diff against `origin/main`).
2. Push `feat/scheduled-clean` (it was rebased: `git push --force-with-lease`), open a PR to main, and push the same SHA to `smoke/scheduled-clean` to run the Windows smoke test. Both must be green.
3. Update `CLAUDE.md` in the main checkout (gitignored, not committed): 14 commands, a `schedule` row in the command table, the policy as a safety invariant, the known gaps (battery/sleep/Terminal/multi-user are manual-only).

## Spike findings (Task 1, 2026-09-24, runs 36016759906 and 36016962505)

- `schtasks /Query /TN <name> /XML` writes **single-byte** XML (bytes `3C 3F 78 6D`) that still declares `encoding="UTF-16"`, and it **omits schema defaults**: `<Enabled>`, `<Priority>7</Priority>` and `<RunLevel>LeastPrivilege</RunLevel>` do not appear; element order differs from what was registered. Parse accordingly (`Settings.Enabled` nil means enabled; never require RunLevel on read).
- A new standard (non-admin) user registered, queried and deleted its own `InteractiveToken`/`LeastPrivilege` task: exit codes 0/0/0.
- The runner user's task ran from a quoted command path containing a space (`"C:\Program Files\PowerShell\7\pwsh.exe"`).
- `schtasks /Query /FO CSV /NH` rows look like `"\Duster Spike (runner)","9/25/2026 3:00:00 AM","Ready"`.
- `net user <name> <pw> /add` prompts Y/N (and fails non-interactively) for passwords over 14 characters.
