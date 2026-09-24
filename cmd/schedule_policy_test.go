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
