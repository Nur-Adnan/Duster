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
