package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Nur-Adnan/duster/internal/logging"
	"github.com/Nur-Adnan/duster/lib/elevation"
	"github.com/Nur-Adnan/duster/lib/fs"
	"github.com/Nur-Adnan/duster/lib/uninstall"
)

// Engine methods that give the GUI the rest of the CLI (OpenSpec change
// expand-windows-gui-cli-parity). Each one calls the function the CLI uses;
// state-changing ones take only 1-based IDs into the engine's latest listing.
func init() {
	for name, m := range map[string]engineMethod{
		"verify.run":    {run: engineVerify},
		"benchmark.run": {run: engineBenchmark},
		"security.run":  {run: engineSecurity},
		"drivers.list":  {run: engineDrivers},
		"oplog.list":    {run: engineOplog},

		"startup.list":   {run: engineStartupList},
		"startup.toggle": {mutates: true, run: engineStartupToggle},
		"startup.remove": {mutates: true, run: engineStartupRemove},

		"purge.scan":     {run: enginePurgeScan},
		"purge.run":      {mutates: true, run: enginePurgeRun},
		"installer.scan": {run: engineInstallerScan},
		"installer.run":  {mutates: true, run: engineInstallerRun},

		"uninstall.list":  {run: engineUninstallList},
		"uninstall.run":   {mutates: true, run: engineUninstallRun},
		"uninstall.sweep": {mutates: true, run: engineUninstallSweep},

		"optimize.list": {run: engineOptimizeList},
		"optimize.run":  {mutates: true, run: engineOptimizeRun},
		"vdisk.scan":    {run: engineVdiskScan},
		"vdisk.run":     {mutates: true, run: engineVdiskRun},

		"schedule.get": {run: engineScheduleGet},
		"schedule.set": {mutates: true, run: engineScheduleSet},
		"schedule.off": {mutates: true, run: engineScheduleOff},

		"update.check":   {run: engineUpdateCheck},
		"update.install": {mutates: true, run: engineUpdateInstall},
		"remove.plan":    {run: engineRemovePlan},
		"remove.run":     {mutates: true, run: engineRemoveRun},

		"analyze.reveal": {run: engineAnalyzeReveal},
	} {
		engineMethods[name] = m
	}
}

// engineFetchRelease is the release lookup; tests replace it so they never reach GitHub.
var engineFetchRelease = fetchLatestRelease

// setListing records kind's latest listing; actions accept only IDs into it.
func setListing[T any](e *engine, kind string, items []T) {
	e.state.Lock()
	e.lists[kind] = items
	e.state.Unlock()
}

// pickListed returns the listed items for 1-based ids, refusing anything
// outside the latest listing of kind or repeated.
func pickListed[T any](e *engine, kind string, ids []int) ([]T, error) {
	if len(ids) == 0 {
		return nil, badRequest("no items given")
	}
	e.state.Lock()
	items, _ := e.lists[kind].([]T)
	e.state.Unlock()
	seen := map[int]bool{}
	out := make([]T, 0, len(ids))
	for _, id := range ids {
		if id < 1 || id > len(items) || seen[id] {
			return nil, badRequest(fmt.Sprintf("%s item %d is not from the latest list, or is repeated", kind, id))
		}
		seen[id] = true
		out = append(out, items[id-1])
	}
	return out, nil
}

type idsParams struct {
	IDs []int `json:"ids"`
}

func decodeIDs(raw json.RawMessage) ([]int, error) {
	var p idsParams
	if err := decodeParams(raw, &p); err != nil {
		return nil, err
	}
	return p.IDs, nil
}

func decodeID(raw json.RawMessage) (int, error) {
	var p struct {
		ID int `json:"id"`
	}
	if err := decodeParams(raw, &p); err != nil {
		return 0, err
	}
	return p.ID, nil
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// ── Diagnostics ─────────────────────────────────────────────

func engineVerify(_ *engine, _ context.Context, _ int64, _ json.RawMessage) (any, error) {
	return runIntegrityVerification(), nil
}

func engineBenchmark(_ *engine, _ context.Context, _ int64, _ json.RawMessage) (any, error) {
	return runSystemBenchmark(), nil
}

type engineSecurityCheck struct {
	Name    string `json:"name"`
	Details string `json:"details"`
	Status  string `json:"status"`
}

func engineSecurity(_ *engine, _ context.Context, _ int64, _ json.RawMessage) (any, error) {
	checks, score, err := runSecurityAudit()
	if err != nil {
		return nil, err
	}
	out := make([]engineSecurityCheck, 0, len(checks))
	for _, c := range checks {
		out = append(out, engineSecurityCheck(c))
	}
	return map[string]any{"checks": out, "score": score}, nil
}

type engineDriver struct {
	Name         string `json:"name"`
	Version      string `json:"version"`
	Manufacturer string `json:"manufacturer"`
	Signed       bool   `json:"signed"`
	Class        string `json:"class"`
}

func engineDrivers(_ *engine, _ context.Context, _ int64, _ json.RawMessage) (any, error) {
	drivers, err := scanInstalledDrivers()
	if err != nil {
		return nil, err
	}
	out := make([]engineDriver, 0, len(drivers))
	for _, d := range drivers {
		out = append(out, engineDriver(d))
	}
	return map[string]any{"drivers": out}, nil
}

// maxOplogEntries caps one operations log listing (newest first).
const maxOplogEntries = 500

type engineOplogEntry struct {
	Time    string `json:"time"`
	Command string `json:"command"`
	Action  string `json:"action"`
	Target  string `json:"target"`
	Size    int64  `json:"size"`
	Status  string `json:"status"`
}

func engineOplog(_ *engine, _ context.Context, _ int64, _ json.RawMessage) (any, error) {
	entries := readOperationsLog()
	out := []engineOplogEntry{}
	for i, en := range entries {
		if i == maxOplogEntries {
			break
		}
		out = append(out, engineOplogEntry{Time: en.Timestamp, Command: en.Command, Action: en.Action, Target: en.Target, Size: en.Size, Status: en.Status})
	}
	return map[string]any{"entries": out, "more": max(0, len(entries)-maxOplogEntries)}, nil
}

// ── Startup ─────────────────────────────────────────────────

type engineStartupEntry struct {
	ID            int    `json:"id"`
	Name          string `json:"name"`
	Command       string `json:"command"`
	Location      string `json:"location"`
	Enabled       bool   `json:"enabled"`
	AdminRequired bool   `json:"admin_required,omitempty"`
}

func engineStartupList(e *engine, _ context.Context, _ int64, _ json.RawMessage) (any, error) {
	entries, err := getStartupEntries()
	if err != nil {
		return nil, err
	}
	setListing(e, "startup", entries)
	out := make([]engineStartupEntry, 0, len(entries))
	for i, en := range entries {
		out = append(out, engineStartupEntry{ID: i + 1, Name: en.Name, Command: en.Command, Location: en.Location, Enabled: en.Enabled, AdminRequired: en.IsAdmin})
	}
	return map[string]any{"entries": out, "admin": elevation.IsAdmin()}, nil
}

// liveStartupEntry re-reads the entries and returns the one matching listed,
// so an entry changed since the listing is never acted on.
func liveStartupEntry(listed startupEntry) (startupEntry, error) {
	entries, err := getStartupEntries()
	if err != nil {
		return startupEntry{}, err
	}
	for _, en := range entries {
		if en.Name == listed.Name && en.Location == listed.Location && en.Command == listed.Command {
			return en, nil
		}
	}
	return startupEntry{}, badRequest(listed.Name + " changed or was removed since the list; refresh it")
}

func engineStartupToggle(e *engine, _ context.Context, _ int64, raw json.RawMessage) (any, error) {
	id, err := decodeID(raw)
	if err != nil {
		return nil, err
	}
	picked, err := pickListed[startupEntry](e, "startup", []int{id})
	if err != nil {
		return nil, err
	}
	live, err := liveStartupEntry(picked[0])
	if err != nil {
		return nil, err
	}
	if err := toggleStartupApproval(live); err != nil {
		return nil, err
	}
	return map[string]any{"enabled": !live.Enabled}, nil
}

// engineStartupRemove deletes disabled entries only, as the landing TUI does:
// disabling first is the reversible step.
func engineStartupRemove(e *engine, _ context.Context, _ int64, raw json.RawMessage) (any, error) {
	ids, err := decodeIDs(raw)
	if err != nil {
		return nil, err
	}
	picked, err := pickListed[startupEntry](e, "startup", ids)
	if err != nil {
		return nil, err
	}
	live := make([]startupEntry, 0, len(picked))
	for _, p := range picked {
		if p.Enabled {
			return nil, badRequest(p.Name + " is enabled: disable it before removing it")
		}
		en, err := liveStartupEntry(p)
		if err != nil {
			return nil, err
		}
		if en.Enabled {
			return nil, badRequest(en.Name + " is enabled: disable it before removing it")
		}
		live = append(live, en)
	}
	removed, errs := 0, []string{}
	for _, en := range live {
		if err := removeStartupEntry(en); err != nil {
			errs = append(errs, en.Name+": "+err.Error())
			continue
		}
		removed++
	}
	return map[string]any{"removed": removed, "errors": errs}, nil
}

// ── Purge ───────────────────────────────────────────────────

type enginePurgeItem struct {
	ID int `json:"id"`
	DiscoveredArtifact
}

func enginePurgeScan(e *engine, ctx context.Context, id int64, raw json.RawMessage) (any, error) {
	var p struct {
		Path string `json:"path"`
	}
	if err := decodeParams(raw, &p); err != nil {
		return nil, err
	}
	if !filepath.IsAbs(p.Path) {
		return nil, badRequest("path must be absolute")
	}
	if err := scanTargetError(p.Path, true); err != nil {
		return nil, &engineError{Code: "failed", Message: err.Error()}
	}
	list, err := scanArtifactsCtx(ctx, p.Path, func(a DiscoveredArtifact, n int) {
		e.event(id, "progress", map[string]any{"found": n, "path": a.Path})
	})
	if err != nil {
		return nil, err
	}
	setListing(e, "purge", list)
	out := make([]enginePurgeItem, 0, len(list))
	var total int64
	for i, a := range list {
		out = append(out, enginePurgeItem{ID: i + 1, DiscoveredArtifact: a})
		total += a.Size
	}
	return map[string]any{"path": p.Path, "artifacts": out, "bytes": total}, nil
}

type enginePurgeResult struct {
	Freed     int64    `json:"freed"`
	Recycled  int64    `json:"recycled"`
	Kept      int64    `json:"kept"`
	KeptCount int      `json:"kept_count"`
	Done      int      `json:"done"`
	Failed    int      `json:"failed"`
	Errors    []string `json:"errors"`
	Notice    string   `json:"notice,omitempty"`
}

// enginePurgeRun is runPurgeCmd's loop: one session, the retention sweep
// first, purgeOne per item, Stop between items.
func enginePurgeRun(e *engine, ctx context.Context, id int64, raw json.RawMessage) (any, error) {
	var p struct {
		IDs  []int  `json:"ids"`
		Mode string `json:"mode"` // keep (default), recycle, permanent
	}
	if err := decodeParams(raw, &p); err != nil {
		return nil, err
	}
	safe, permanent := false, false
	switch p.Mode {
	case "", "keep":
	case "recycle":
		safe = true
	case "permanent":
		permanent = true
	default:
		return nil, badRequest("mode must be keep, recycle or permanent")
	}
	items, err := pickListed[DiscoveredArtifact](e, "purge", p.IDs)
	if err != nil {
		return nil, err
	}
	var t purgeTally
	t.sweep = sweepQuarantine(time.Now(), sweepFull)
	s := newQuarantineSession("purge")
	res := func(done int) enginePurgeResult {
		return enginePurgeResult{Freed: t.freed, Recycled: t.recycled, Kept: t.kept, KeptCount: t.keptCount,
			Done: done, Failed: t.failed, Errors: append([]string{}, t.errs...), Notice: sweepNotice(t.sweep)}
	}
	for i, a := range items {
		if err := ctx.Err(); err != nil {
			return res(i), err
		}
		e.event(id, "progress", map[string]any{"index": i, "total": len(items), "path": a.Path})
		q, perr := purgeOne(s, a.Path, a.Size, safe, permanent)
		t.add(a.Path, a.Size, q, permanent, perr)
	}
	return res(len(items)), nil
}

// ── Installers ──────────────────────────────────────────────

type engineInstallerItem struct {
	ID int `json:"id"`
	installerItem
}

func engineInstallerScan(e *engine, _ context.Context, _ int64, raw json.RawMessage) (any, error) {
	p := struct {
		MinSizeMB int64 `json:"min_size_mb"`
	}{MinSizeMB: 50}
	if err := decodeParams(raw, &p); err != nil {
		return nil, err
	}
	items := scanInstallerItems(p.MinSizeMB)
	setListing(e, "installer", items)
	out := make([]engineInstallerItem, 0, len(items))
	for i, it := range items {
		out = append(out, engineInstallerItem{ID: i + 1, installerItem: it})
	}
	return map[string]any{"items": out}, nil
}

type engineKeepResult struct {
	Bytes  int64  `json:"bytes"`
	Kept   int    `json:"kept"`
	Failed int    `json:"failed"`
	Notice string `json:"notice,omitempty"`
}

func engineInstallerRun(e *engine, _ context.Context, _ int64, raw json.RawMessage) (any, error) {
	ids, err := decodeIDs(raw)
	if err != nil {
		return nil, err
	}
	items, err := pickListed[installerItem](e, "installer", ids)
	if err != nil {
		return nil, err
	}
	for i := range items {
		items[i].Selected = true
	}
	size, kept, failed, warn := sweepInstallerItems(items, false)
	return engineKeepResult{Bytes: size, Kept: kept, Failed: failed, Notice: warn}, nil
}

// ── Uninstall ───────────────────────────────────────────────

type engineApp struct {
	ID        int    `json:"id"`
	Protected bool   `json:"protected,omitempty"`
	PerUser   bool   `json:"per_user,omitempty"`
	Name      string `json:"name"`
	Publisher string `json:"publisher,omitempty"`
	Version   string `json:"version,omitempty"`
	Installed string `json:"install_date,omitempty"`
	Size      int64  `json:"size,omitempty"`
}

func engineUninstallList(e *engine, _ context.Context, _ int64, _ json.RawMessage) (any, error) {
	apps, err := uninstall.GetInstalledApps()
	if err != nil {
		return nil, err
	}
	setListing(e, "apps", apps)
	out := make([]engineApp, 0, len(apps))
	for i, a := range apps {
		out = append(out, engineApp{ID: i + 1, Protected: isProtectedApp(a.Name), PerUser: a.RegistryHive == "HKCU",
			Name: a.Name, Publisher: a.Publisher, Version: a.DisplayVersion, Installed: a.InstallDate, Size: a.EstimatedSize})
	}
	return map[string]any{"apps": out, "admin": elevation.IsAdmin()}, nil
}

type engineLeftover struct {
	ID   int    `json:"id"`
	Path string `json:"path"`
	Size int64  `json:"size"`
}

// engineUninstallRun runs the app's own uninstaller (uninstallApp, as the TUI)
// and, only once the app is gone, lists its leftovers, none preselected.
func engineUninstallRun(e *engine, _ context.Context, _ int64, raw json.RawMessage) (any, error) {
	id, err := decodeID(raw)
	if err != nil {
		return nil, err
	}
	picked, err := pickListed[uninstall.InstalledApp](e, "apps", []int{id})
	if err != nil {
		return nil, err
	}
	app := picked[0]
	if isProtectedApp(app.Name) {
		return nil, badRequest(app.Name + " is a protected system component")
	}
	if installed, err := appStillInstalled(app); err != nil || !installed {
		return nil, badRequest(app.Name + " is no longer installed; refresh the list")
	}
	still, err := uninstallApp(app, false)
	if err != nil {
		return nil, err
	}
	out := map[string]any{"still_installed": still, "leftovers": []engineLeftover{}}
	if still {
		return out, nil
	}
	var items []leftoverItem
	var list []engineLeftover
	for i, f := range scanAppLeftovers(app.Name, app.Publisher) {
		it := leftoverItem{Path: f, Size: calculateDirSize(f)}
		items = append(items, it)
		list = append(list, engineLeftover{ID: i + 1, Path: it.Path, Size: it.Size})
	}
	setListing(e, "leftovers", items)
	if list != nil {
		out["leftovers"] = list
	}
	return out, nil
}

func engineUninstallSweep(e *engine, _ context.Context, _ int64, raw json.RawMessage) (any, error) {
	ids, err := decodeIDs(raw)
	if err != nil {
		return nil, err
	}
	items, err := pickListed[leftoverItem](e, "leftovers", ids)
	if err != nil {
		return nil, err
	}
	for i := range items {
		items[i].Selected = true
	}
	size, kept, failed, warn := sweepLeftoverItems(items, false)
	return engineKeepResult{Bytes: size, Kept: kept, Failed: failed, Notice: warn}, nil
}

// ── Optimize ────────────────────────────────────────────────

func optimizeNeedsAdmin(id string) bool { return id == "ssd_trim" || id == "component_store" }

var taskStatusNames = map[taskStatus]string{statusPending: "pending", statusRunning: "running",
	statusCompleted: "completed", statusFailed: "failed", statusSkipped: "skipped"}

type engineOptimizeTask struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Description   string `json:"description"`
	AdminRequired bool   `json:"admin_required,omitempty"`
	Status        string `json:"status,omitempty"`
	Reclaimed     int64  `json:"reclaimed,omitempty"`
	Note          string `json:"note,omitempty"`
	Error         string `json:"error,omitempty"`
}

func engineOptimizeList(_ *engine, _ context.Context, _ int64, _ json.RawMessage) (any, error) {
	tasks := []engineOptimizeTask{}
	for _, t := range optimizeTasks(true) {
		tasks = append(tasks, engineOptimizeTask{ID: t.ID, Name: t.Name, Description: t.Description, AdminRequired: optimizeNeedsAdmin(t.ID)})
	}
	return map[string]any{"tasks": tasks, "admin": elevation.IsAdmin(), "reclaim": buildReclaimItems(systemDriveRoot())}, nil
}

// engineOptimizeRun runs the named tasks in the CLI's order through
// execOptimizeTask. dry_run is the CLI's preview: only the component store
// runs, as DISM's read-only analysis.
func engineOptimizeRun(e *engine, ctx context.Context, id int64, raw json.RawMessage) (any, error) {
	var p struct {
		IDs    []string `json:"ids"`
		DryRun bool     `json:"dry_run"`
	}
	if err := decodeParams(raw, &p); err != nil {
		return nil, err
	}
	want := map[string]bool{}
	for _, tid := range p.IDs {
		want[tid] = true
	}
	var tasks []optimizeTask
	for _, t := range optimizeTasks(true) {
		if want[t.ID] {
			tasks = append(tasks, t)
			delete(want, t.ID)
		}
	}
	if len(tasks) == 0 || len(want) > 0 {
		return nil, badRequest("ids must name optimize tasks from optimize.list")
	}
	// ponytail: optCtx is the package's cancel hook; busy keeps it to one run.
	optCtx = ctx
	defer func() { optCtx = nil }()
	admin := elevation.IsAdmin()
	res := map[string]any{"tasks": []engineOptimizeTask{}, "reclaimed": int64(0)}
	var out []engineOptimizeTask
	var total int64
	for i, t := range tasks {
		if err := ctx.Err(); err != nil {
			res["tasks"], res["reclaimed"] = out, total
			return res, err
		}
		e.event(id, "progress", map[string]any{"task": t.ID, "state": "start", "index": i, "total": len(tasks)})
		r := optTaskResult{status: statusSkipped}
		if !p.DryRun || t.ID == "component_store" {
			r = execOptimizeTask(t, admin, p.DryRun)
		}
		if r.status == statusSkipped && r.note == "" && optimizeNeedsAdmin(t.ID) && !admin {
			r.note = "needs Administrator"
		}
		out = append(out, engineOptimizeTask{ID: t.ID, Name: t.Name, Status: taskStatusNames[r.status], Reclaimed: r.reclaimed, Note: r.note, Error: errText(r.err)})
		total += r.reclaimed
		e.event(id, "progress", map[string]any{"task": t.ID, "state": "done", "index": i, "total": len(tasks)})
	}
	res["tasks"], res["reclaimed"] = out, total
	return res, nil
}

// ── Virtual disks ───────────────────────────────────────────

type engineVdisk struct {
	ID int `json:"id"`
	virtualDisk
	Estimate      int64 `json:"estimate,omitempty"`
	EstimateKnown bool  `json:"estimate_known"`
}

func engineVdiskScan(e *engine, ctx context.Context, _ int64, _ json.RawMessage) (any, error) {
	disks := scanVirtualDisks(ctx)
	setListing(e, "vdisk", disks)
	out := make([]engineVdisk, 0, len(disks))
	for i, d := range disks {
		est, ok := d.recoverable()
		out = append(out, engineVdisk{ID: i + 1, virtualDisk: d, Estimate: est, EstimateKnown: ok})
	}
	return map[string]any{"disks": out, "admin": elevation.IsAdmin(), "advice": vdiskAdvice(disks)}, nil
}

// engineVdiskRun is runHeadlessVdisk's --yes path for the listed disks.
// Canceling stops diskpart through ctx; compactVirtualDisk detaches the disk.
func engineVdiskRun(e *engine, ctx context.Context, id int64, raw json.RawMessage) (any, error) {
	ids, err := decodeIDs(raw)
	if err != nil {
		return nil, err
	}
	if !elevation.IsAdmin() {
		return nil, badRequest("compacting virtual disks needs administrator rights")
	}
	disks, err := pickListed[virtualDisk](e, "vdisk", ids)
	if err != nil {
		return nil, err
	}
	res := map[string]any{"results": []vdiskResult{}, "freed": int64(0)}
	if werr := shutdownWSL(ctx); werr != nil {
		res["warning"] = werr.Error() // not fatal: a disk still held open fails on its own
	}
	var results []vdiskResult
	var freed int64
	for i, d := range disks {
		if err := ctx.Err(); err != nil {
			res["results"], res["freed"] = results, freed
			return res, err
		}
		e.event(id, "progress", map[string]any{"index": i, "total": len(disks), "path": d.Path})
		r := vdiskResult{Path: d.Path, Label: d.Label}
		if d.Blocked != "" {
			r.Status, r.Note = "skipped", d.Blocked
		} else if got, cerr := compactVirtualDisk(ctx, d); cerr != nil {
			r.Status, r.Note = "failed", cerr.Error()
		} else {
			r.Status, r.FreedBytes = "compacted", got
			freed += got
		}
		results = append(results, r)
	}
	res["results"], res["freed"] = results, freed
	return res, nil
}

// ── Schedule ────────────────────────────────────────────────

type engineChoice struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func engineScheduleGet(_ *engine, _ context.Context, _ int64, _ json.RawMessage) (any, error) {
	name, _, err := currentScheduleTask()
	if err != nil {
		return nil, err
	}
	names := scheduleCategoryNames()
	choices := func(ids []string) []engineChoice {
		out := []engineChoice{}
		for _, id := range ids {
			out = append(out, engineChoice{ID: id, Name: names[id]})
		}
		return out
	}
	return map[string]any{
		"status": readScheduleStatus(name, loadScheduleRecord(logging.Dir()), time.Now()),
		"safe":   choices(scheduleSafe), "opt_in": choices(scheduleOptIn),
	}, nil
}

func engineScheduleSet(_ *engine, _ context.Context, _ int64, raw json.RawMessage) (any, error) {
	var p struct {
		Every    string   `json:"every"`
		At       string   `json:"at"`
		LowSpace string   `json:"low_space"`
		Add      []string `json:"add"`
		DryRun   bool     `json:"dry_run"`
	}
	if err := decodeParams(raw, &p); err != nil {
		return nil, err
	}
	cfg, notes, err := parseScheduleOn(p.Every, p.At, p.LowSpace, p.Add)
	if err != nil {
		return nil, badRequest(err.Error())
	}
	return applyScheduleOn(cfg, notes, p.DryRun)
}

func engineScheduleOff(_ *engine, _ context.Context, _ int64, _ json.RawMessage) (any, error) {
	_, alreadyOff, err := turnScheduleOff()
	if err != nil {
		return nil, err
	}
	return map[string]any{"already_off": alreadyOff}, nil
}

// ── Update and remove ───────────────────────────────────────

func engineUpdateCheck(_ *engine, _ context.Context, _ int64, _ json.RawMessage) (any, error) {
	rel, err := engineFetchRelease()
	if err != nil {
		return nil, err
	}
	latest := trimV(rel.TagName)
	return map[string]any{"current": AppVersion, "latest": latest, "available": isNewerVersion(latest, AppVersion),
		"published_at": rel.PublishedAt, "notes": rel.Body}, nil
}

// engineUpdateInstall is `du update --yes` (force: `--force`): the release is
// fetched again here, never taken from the client, and the SHA-256 check in
// downloadVerifiedBinary is mandatory.
func engineUpdateInstall(_ *engine, _ context.Context, _ int64, raw json.RawMessage) (any, error) {
	var p struct {
		Force bool `json:"force"`
	}
	if err := decodeParams(raw, &p); err != nil {
		return nil, err
	}
	rel, err := engineFetchRelease()
	if err != nil {
		return nil, err
	}
	latest := trimV(rel.TagName)
	if !p.Force && !isNewerVersion(latest, AppVersion) {
		return nil, badRequest("Duster " + AppVersion + " is up to date")
	}
	bins, err := downloadVerifiedBinary(rel)
	if err != nil {
		return nil, err
	}
	if err := swapBinary(bins); err != nil {
		return nil, err
	}
	return map[string]any{"version": latest, "gui_updated": bins.gui != nil}, nil
}

func trimV(tag string) string {
	if len(tag) > 0 && (tag[0] == 'v' || tag[0] == 'V') {
		return tag[1:]
	}
	return tag
}

func engineRemovePlan(_ *engine, _ context.Context, _ int64, _ json.RawMessage) (any, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	setup, _ := filepath.Glob(filepath.Join(filepath.Dir(exe), "unins*.exe"))
	return map[string]any{"exe": exe, "data_dir": logging.Dir(), "kept_bytes": quarantineHeld(),
		"protected": fs.IsSystemProtectedPath(exe), "setup_installed": len(setup) > 0}, nil
}

// engineRemoveRun is `du remove --force`'s sequence. The GUI exits after it;
// du.exe and Duster.exe are deleted once they stop running.
func engineRemoveRun(_ *engine, _ context.Context, _ int64, _ json.RawMessage) (any, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	if fs.IsSystemProtectedPath(exe) {
		return nil, badRequest("Duster is installed in a system-protected path; self-removal is blocked for safety")
	}
	err = emptyAllQuarantines()
	if err == nil {
		err = cleanDusterDir(logging.Dir(), exe, false)
	}
	if err == nil {
		removeScheduleAndLauncher(exe)
		removeGUI(exe)
		scheduleDelayedDelete(exe)
	}
	logRmOperation("gui-uninstall", exe, 0, err == nil)
	if err != nil {
		return nil, err
	}
	return nil, nil
}

// ── Analyze reveal ──────────────────────────────────────────

// engineAnalyzeReveal opens an issued item, or a listed change's nearest
// existing folder, in Explorer (the analyze TUI's o key).
func engineAnalyzeReveal(e *engine, _ context.Context, _ int64, raw json.RawMessage) (any, error) {
	var p struct {
		ID     int64 `json:"id"`
		Change int   `json:"change"` // 1-based index into the latest scan's changes
	}
	if err := decodeParams(raw, &p); err != nil {
		return nil, err
	}
	e.state.Lock()
	a := e.analysis
	var path string
	var err error
	switch {
	case a == nil:
		err = badRequest("no analyze.scan yet")
	case p.Change > 0:
		if p.Change > len(a.changePaths) {
			err = badRequest("change is not from the latest analyze.scan")
		} else {
			path = nearestExisting(a.changePaths[p.Change-1], a.root.Path)
		}
	default:
		var ref engineAnalyzeRef
		if _, ref, err = e.analyzeRef(p.ID); err == nil {
			path = ref.path
		}
	}
	e.state.Unlock()
	if err != nil {
		return nil, err
	}
	if err := openInExplorer(path); err != nil {
		return nil, errors.New("cannot open Explorer: " + err.Error())
	}
	return nil, nil
}
