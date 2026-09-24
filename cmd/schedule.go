package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/Nur-Adnan/duster/internal/logging"
	"github.com/Nur-Adnan/duster/lib/elevation"
	"github.com/Nur-Adnan/duster/lib/fs"
	"github.com/spf13/cobra"
)

const scheduleRecordName = "schedule.json"

// scheduleRecord is what scheduled runs remember, in logging.Dir()\schedule.json.
type scheduleRecord struct {
	LastCheck *scheduleCheck `json:"last_check,omitempty"`
	LastClean *scheduleClean `json:"last_clean,omitempty"`
	// LastSuccess is the last clean where at least one category did not fail.
	// The cadence counts from it, so a run where everything failed retries.
	LastSuccess time.Time `json:"last_success,omitzero"`
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
		rep := sweepQuarantine(time.Now(), sweepFull)
		for _, l := range []string{rep.expiredLine(), rep.lowSpaceLine()} {
			if l != "" {
				fmt.Println("quarantine sweep:", l)
			}
		}
		for _, err := range rep.Errs {
			fmt.Println("quarantine sweep:", err)
		}
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
	switch {
	case st.DryRun:
		fmt.Fprintln(w, "Scheduled clean: PREVIEW (dry run, nothing registered)")
	case st.Enabled:
		fmt.Fprintln(w, "Scheduled clean: ON")
	default:
		fmt.Fprintln(w, "Scheduled clean: OFF")
	}
	if st.DryRun || st.Enabled {
		if st.Every != "" {
			early := ""
			if st.LowSpacePercent != nil {
				early = fmt.Sprintf(", or early when %s has under %d%% free", drive, *st.LowSpacePercent)
			}
			fmt.Fprintf(w, "  Cleans %s%s. Checks daily at %s.\n", st.Every, early, st.At)
			fmt.Fprintf(w, "  Cleans: %s\n", scheduleCategoryList(st.Categories))
		}
		if st.NextCheck != nil {
			fmt.Fprintf(w, "  Next check:  %s\n", st.NextCheck.Local().Format(scheduleTimeLayout))
		}
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
	switch {
	case st.DryRun:
		fmt.Fprintln(w, "Register it with: du schedule on (same flags, without --dry-run)")
	case st.Enabled:
		fmt.Fprintln(w, "Turn off with: du schedule off")
	default:
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
		scheduleFail(fmt.Errorf("duw.exe is missing from %s: run du update --force (or reinstall Duster)", filepath.Dir(exe)))
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
		_, st.Enabled = queryScheduleTask(name)
		if elevation.IsAdmin() {
			st.Warnings = append(st.Warnings, fmt.Sprintf("would be registered for %s; runs without administrator rights", u.Username))
		}
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
	if !st.Enabled {
		// The re-query failed (schtasks flakiness, a slow registration): the
		// task is there, Duster just could not read it back.
		st.Enabled = true
		st.TaskName = name
		st.Warnings = append(st.Warnings, "the task was registered, but Duster could not read it back, so some details below may be missing")
	}
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
	alreadyOff := false
	if deleteErr := deleteScheduleTask(name); deleteErr != nil {
		names, listErr := listScheduleTaskNames()
		var ok bool
		ok, err = offOutcome(deleteErr, names, listErr, name)
		if err != nil {
			scheduleFail(err)
		}
		alreadyOff = ok
	}
	if alreadyOff {
		if !schedJSON {
			fmt.Println("Scheduled clean is already off.")
			return
		}
	} else if !schedJSON {
		fmt.Printf("Scheduled clean is off. The record of past runs stays in %s.\n", filepath.Join(logging.Dir(), scheduleRecordName))
		return
	}
	printScheduleStatus(readScheduleStatus(name, loadScheduleRecord(logging.Dir()), time.Now()))
}

// offOutcome decides what executeScheduleOff reports after always attempting
// to delete the task first, so a running task is never mistaken for one that
// is already off. deleteErr is deleteScheduleTask's result; when it succeeds
// the task is off and names/listErr are unused. When it fails, names and
// listErr come from listScheduleTaskNames, consulted only then: if the list
// succeeded and name is not in it, the task really was already gone (the
// delete raced it, or schtasks reported a phantom failure) and that is not an
// error. Any other case (the list failed too, or name is still there) keeps
// the delete error, since parsing schtasks' localized "task not found" text
// would be wrong on non-English Windows.
func offOutcome(deleteErr error, names []string, listErr error, name string) (alreadyOff bool, err error) {
	if deleteErr == nil {
		return false, nil
	}
	if listErr == nil && !slices.Contains(names, name) {
		return true, nil
	}
	return false, deleteErr
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
