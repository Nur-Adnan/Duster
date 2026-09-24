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
