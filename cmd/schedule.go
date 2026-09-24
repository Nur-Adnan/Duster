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
