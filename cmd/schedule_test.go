package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
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

func TestRemoveScheduleAndLauncher(t *testing.T) {
	dir := t.TempDir()
	duw := filepath.Join(dir, "duw.exe")
	if err := os.WriteFile(duw, []byte("MZ"), 0o755); err != nil {
		t.Fatal(err)
	}
	removeScheduleAndLauncher(filepath.Join(dir, "du.exe"))
	if _, err := os.Stat(duw); !os.IsNotExist(err) {
		t.Errorf("duw.exe still exists after removeScheduleAndLauncher: %v", err)
	}
}
