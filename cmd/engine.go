package cmd

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/Nur-Adnan/duster/lib/elevation"
	"github.com/Nur-Adnan/duster/lib/sysinfo"
	"github.com/spf13/cobra"
)

// EngineCmd is the private stdio host the Windows GUI (Duster.exe) drives.
// Contract: openspec/changes/add-windows-gui/specs/engine-protocol/spec.md.
// Stdio only: no socket or pipe name another process could open.
var EngineCmd = &cobra.Command{
	Use:    "engine",
	Short:  "Serve the Duster GUI over stdin/stdout (internal)",
	Hidden: true,
	Args:   cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		out := os.Stdout
		// A stray fmt.Print inside an engine function must not corrupt the
		// protocol stream, so everything but protocol writes goes to stderr.
		os.Stdout = os.Stderr
		newEngine(out).serve(os.Stdin)
	},
}

const (
	engineProtocol = 1
	maxEngineLine  = 1 << 20
)

type engineRequest struct {
	ID     int64           `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

// engineError is both the wire error and a Go error a method can return to
// pick its own code.
type engineError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

func (e *engineError) Error() string { return e.Code + ": " + e.Message }

func badRequest(msg string) error { return &engineError{Code: "bad_request", Message: msg} }

type engineMessage struct {
	ID     int64        `json:"id"`
	Result any          `json:"result,omitempty"`
	Error  *engineError `json:"error,omitempty"`
	Event  string       `json:"event,omitempty"`
	Data   any          `json:"data,omitempty"`
}

type engineMethod struct {
	mutates bool // state-changing: at most one runs at a time
	run     func(e *engine, ctx context.Context, id int64, params json.RawMessage) (any, error)
}

var engineMethods = map[string]engineMethod{
	"hello":      {run: engineHello},
	"cancel":     {run: engineCancel},
	"status.get": {run: engineStatus},
	"doctor.run": {run: engineDoctor},
	"clean.scan": {run: engineCleanScan},
	"clean.run":  {mutates: true, run: engineCleanRun},

	"restore.list":  {run: engineRestoreList},
	"restore.run":   {mutates: true, run: engineRestoreRun},
	"restore.empty": {mutates: true, run: engineRestoreEmpty},

	"analyze.scan":     {run: engineAnalyzeScan},
	"analyze.children": {run: engineAnalyzeChildren},
	"analyze.recycle":  {mutates: true, run: engineAnalyzeRecycle},
}

type engine struct {
	out   sync.Mutex // serializes writes to enc
	enc   *json.Encoder
	busy  sync.Mutex // held by the running state-changing request
	wg    sync.WaitGroup
	cats  func() []CleanCategory // getCategories; tests substitute temp roots
	state sync.Mutex             // guards running and scanned
	// running maps in-flight request ids to their cancel funcs.
	running map[int64]context.CancelFunc
	// scanned holds the category IDs the latest clean.scan returned;
	// clean.run accepts nothing else.
	scanned map[string]bool
	// restoreIDs holds the session IDs the latest restore.list returned.
	restoreIDs map[string]bool
	// analysis is the latest analyze.scan: its tree and the item IDs handed out.
	analysis *engineAnalysis
}

func newEngine(w io.Writer) *engine {
	return &engine{enc: json.NewEncoder(w), cats: getCategories, running: map[int64]context.CancelFunc{}}
}

// serve handles requests until in reaches EOF, then cancels everything still
// running and waits for it to stop at a safe point.
func (e *engine) serve(in io.Reader) {
	ctx, stop := context.WithCancel(context.Background())
	defer func() { stop(); e.wg.Wait() }()
	r := bufio.NewReaderSize(in, maxEngineLine)
	for {
		line, isPrefix, err := r.ReadLine()
		if isPrefix {
			for isPrefix && err == nil {
				_, isPrefix, err = r.ReadLine()
			}
			e.send(engineMessage{Error: &engineError{Code: "bad_request", Message: "line longer than 1 MiB"}})
		} else if len(line) > 0 {
			var req engineRequest
			if json.Unmarshal(line, &req) != nil || req.ID <= 0 {
				e.send(engineMessage{Error: &engineError{Code: "bad_request", Message: "each line must be a JSON object with a positive integer id"}})
			} else {
				e.start(ctx, req)
			}
		}
		if err != nil {
			return
		}
	}
}

func (e *engine) start(ctx context.Context, req engineRequest) {
	m, ok := engineMethods[req.Method]
	if !ok {
		e.reply(req.ID, nil, &engineError{Code: "unknown_method", Message: req.Method})
		return
	}
	rctx, cancel := context.WithCancel(ctx)
	e.state.Lock()
	_, dup := e.running[req.ID]
	if !dup {
		e.running[req.ID] = cancel
	}
	e.state.Unlock()
	if dup {
		cancel()
		e.reply(req.ID, nil, badRequest("id already in flight"))
		return
	}
	done := func() {
		e.state.Lock()
		delete(e.running, req.ID)
		e.state.Unlock()
		cancel()
	}
	if m.mutates && !e.busy.TryLock() {
		done()
		e.reply(req.ID, nil, &engineError{Code: "busy", Message: "another operation is running"})
		return
	}
	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		res, err := m.run(e, rctx, req.ID, req.Params)
		if m.mutates {
			e.busy.Unlock()
		}
		done()
		e.reply(req.ID, res, err)
	}()
}

func (e *engine) reply(id int64, res any, err error) {
	if err == nil {
		if res == nil {
			res = struct{}{}
		}
		e.send(engineMessage{ID: id, Result: res})
		return
	}
	var ee *engineError
	switch {
	case errors.As(err, &ee):
	case errors.Is(err, context.Canceled):
		ee = &engineError{Code: "canceled", Message: "canceled", Data: res}
	default:
		ee = &engineError{Code: "failed", Message: err.Error()}
	}
	e.send(engineMessage{ID: id, Error: ee})
}

func (e *engine) event(id int64, name string, data any) {
	e.send(engineMessage{ID: id, Event: name, Data: data})
}

// send writes one line. A write error means the GUI is gone; stdin EOF follows
// and shuts the engine down, so it is not reported here.
func (e *engine) send(m engineMessage) {
	e.out.Lock()
	defer e.out.Unlock()
	_ = e.enc.Encode(m)
}

func decodeParams(raw json.RawMessage, v any) error {
	if len(raw) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, v); err != nil {
		return badRequest("params: " + err.Error())
	}
	return nil
}

func engineHello(_ *engine, _ context.Context, _ int64, _ json.RawMessage) (any, error) {
	return map[string]any{"protocol": engineProtocol, "version": AppVersion, "admin": elevation.IsAdmin()}, nil
}

func engineCancel(e *engine, _ context.Context, _ int64, raw json.RawMessage) (any, error) {
	var p struct {
		ID int64 `json:"id"`
	}
	if err := decodeParams(raw, &p); err != nil {
		return nil, err
	}
	e.state.Lock()
	if c := e.running[p.ID]; c != nil {
		c()
	}
	e.state.Unlock()
	return nil, nil
}

func engineStatus(_ *engine, _ context.Context, _ int64, raw json.RawMessage) (any, error) {
	var p struct {
		TopProcesses bool `json:"top_processes"`
	}
	if err := decodeParams(raw, &p); err != nil {
		return nil, err
	}
	stats, err := sysinfo.GetSystemStats()
	if err != nil {
		return nil, err
	}
	if p.TopProcesses {
		stats.TopProcesses = sysinfo.TopProcesses(time.Second)
	}
	return stats, nil
}

func engineDoctor(_ *engine, _ context.Context, _ int64, _ json.RawMessage) (any, error) {
	return runDoctorDiagnostics(), nil
}

// engineCategory is one clean category as the GUI sees it, in scans and runs.
type engineCategory struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Description   string `json:"description,omitempty"`
	Group         string `json:"group"`
	Bytes         int64  `json:"bytes"`
	Files         int    `json:"files"`
	AdminRequired bool   `json:"admin_required,omitempty"`
	Error         string `json:"error,omitempty"`
}

type engineCleanResult struct {
	Categories []engineCategory `json:"categories"`
	Bytes      int64            `json:"bytes"`
	Files      int              `json:"files"`
}

func cleanGroupName(id string) string {
	for _, g := range cleanGroups {
		if g.catID[id] {
			return g.name
		}
	}
	return ""
}

// cleanCategoryEntry scans or cleans one category; admin-only categories are
// reported, never attempted, while unelevated.
func cleanCategoryEntry(cat CleanCategory, scanOnly bool) engineCategory {
	c := engineCategory{ID: cat.ID, Name: cat.Name, Description: cat.Description, Group: cleanGroupName(cat.ID)}
	if adminOnlyBlocked(cat) {
		c.AdminRequired = true
		if !scanOnly {
			c.Error = "admin_required"
		}
		return c
	}
	var err error
	c.Bytes, c.Files, err = runCategory(cat, scanOnly)
	if err != nil {
		c.Error = err.Error()
	}
	return c
}

// runCleanCategories scans or cleans cats in order, reporting each start and
// finish. Cancellation is honored between categories, never inside one.
func (e *engine) runCleanCategories(ctx context.Context, id int64, cats []CleanCategory, scanOnly bool) (engineCleanResult, error) {
	res := engineCleanResult{Categories: []engineCategory{}}
	for i, cat := range cats {
		if err := ctx.Err(); err != nil {
			return res, err
		}
		e.event(id, "progress", map[string]any{"category": cat.ID, "state": "start", "index": i, "total": len(cats)})
		c := cleanCategoryEntry(cat, scanOnly)
		res.Categories = append(res.Categories, c)
		res.Bytes += c.Bytes
		res.Files += c.Files
		e.event(id, "progress", map[string]any{"category": cat.ID, "state": "done", "index": i, "total": len(cats), "bytes": c.Bytes})
	}
	return res, nil
}

func engineCleanScan(e *engine, ctx context.Context, id int64, _ json.RawMessage) (any, error) {
	cats := groupedCategories(e.cats())
	res, err := e.runCleanCategories(ctx, id, cats, true)
	if err != nil {
		return res, err
	}
	scanned := map[string]bool{}
	for _, c := range cats {
		scanned[c.ID] = true
	}
	e.state.Lock()
	e.scanned = scanned
	e.state.Unlock()
	return res, nil
}

func engineCleanRun(e *engine, ctx context.Context, id int64, raw json.RawMessage) (any, error) {
	var p struct {
		IDs []string `json:"ids"`
	}
	if err := decodeParams(raw, &p); err != nil {
		return nil, err
	}
	if len(p.IDs) == 0 {
		return nil, badRequest("ids is empty")
	}
	want := map[string]bool{}
	e.state.Lock()
	for _, cid := range p.IDs {
		if !e.scanned[cid] || want[cid] {
			e.state.Unlock()
			return nil, badRequest("category " + cid + " is not from the latest clean.scan, or is repeated")
		}
		want[cid] = true
	}
	e.state.Unlock()
	var cats []CleanCategory
	for _, c := range groupedCategories(e.cats()) {
		if want[c.ID] {
			cats = append(cats, c)
		}
	}
	return e.runCleanCategories(ctx, id, cats, false)
}

func engineRestoreList(e *engine, _ context.Context, _ int64, _ json.RawMessage) (any, error) {
	// ponytail: no expiry sweep here, unlike `du restore`: listing stays
	// read-only, and purge/installer/schedule runs still apply retention.
	rs := groupSessions(loadKeptSessions(quarantineRoots()))
	ids := map[string]bool{}
	for _, r := range rs {
		ids[r.ID] = true
	}
	e.state.Lock()
	e.restoreIDs = ids
	e.state.Unlock()
	return map[string]any{"sessions": restoreListJSON(rs)}, nil
}

// listedSessions reloads the quarantine and returns the sessions named by ids,
// each of which must come from the latest restore.list, with their 1-based
// numbers. A listed session that is gone since (restored or emptied
// elsewhere) is refused rather than guessed at.
func (e *engine) listedSessions(ids []string) ([]restoreSession, []int, error) {
	if len(ids) == 0 {
		return nil, nil, badRequest("no session given")
	}
	e.state.Lock()
	for _, id := range ids {
		if !e.restoreIDs[id] {
			e.state.Unlock()
			return nil, nil, badRequest("session " + id + " is not from the latest restore.list")
		}
	}
	e.state.Unlock()
	rs := groupSessions(loadKeptSessions(quarantineRoots()))
	var out []restoreSession
	var numbers []int
	for _, id := range ids {
		found := false
		for i, r := range rs {
			if r.ID == id {
				out, numbers, found = append(out, r), append(numbers, i+1), true
				break
			}
		}
		if !found {
			return nil, nil, &engineError{Code: "failed", Message: "session " + id + " is no longer kept; refresh the list"}
		}
	}
	return out, numbers, nil
}

func engineRestoreRun(e *engine, _ context.Context, _ int64, raw json.RawMessage) (any, error) {
	var p struct {
		ID   string `json:"id"`
		Item int    `json:"item"` // 0 = the whole session
	}
	if err := decodeParams(raw, &p); err != nil {
		return nil, err
	}
	rs, numbers, err := e.listedSessions([]string{p.ID})
	if err != nil {
		return nil, err
	}
	session, n := rs[0], numbers[0]
	if err := restoreRequestError(session, n, p.Item, p.Item != 0); err != nil {
		return nil, badRequest(err.Error())
	}
	results := restoreItems(session, p.Item, false)
	if p.Item == 0 {
		results = append(results, damagedPartResults(session, n)...)
	}
	failed := false
	for _, r := range results {
		failed = failed || r.Status == "failed"
	}
	return map[string]any{"results": results, "failed": failed}, nil
}

func engineRestoreEmpty(e *engine, _ context.Context, _ int64, raw json.RawMessage) (any, error) {
	var p struct {
		IDs []string `json:"ids"`
	}
	if err := decodeParams(raw, &p); err != nil {
		return nil, err
	}
	rs, _, err := e.listedSessions(p.IDs)
	if err != nil {
		return nil, err
	}
	var size int64
	for _, r := range rs {
		size += r.Size()
	}
	if err := emptySessions(rs); err != nil {
		return nil, err
	}
	return map[string]any{"emptied": len(rs), "size": size}, nil
}

// maxAnalyzeEntries caps one folder listing; a folder with more entries lists
// its largest ones and reports the rest in "more".
const maxAnalyzeEntries = 500

type engineAnalysis struct {
	root  *FolderNode
	items map[int64]engineAnalyzeRef
	next  int64
	undo  *quarantineSession // created on the first recycle the bin refuses
}

type engineAnalyzeRef struct {
	path string
	size int64
	dir  bool
}

type engineAnalyzeItem struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Path  string `json:"path"`
	Size  int64  `json:"size"`
	IsDir bool   `json:"is_dir"`
	Items int    `json:"items,omitempty"`
}

type engineAnalyzeFolder struct {
	ID      int64               `json:"id"`
	Path    string              `json:"path"`
	Size    int64               `json:"size"`
	Entries []engineAnalyzeItem `json:"entries"`
	More    int                 `json:"more,omitempty"`
	Largest []engineAnalyzeItem `json:"largest"`
}

// issue hands out an ID for path; only issued IDs are accepted back.
func (a *engineAnalysis) issue(path string, size int64, dir bool) int64 {
	a.next++
	a.items[a.next] = engineAnalyzeRef{path: path, size: size, dir: dir}
	return a.next
}

func (a *engineAnalysis) item(path string, size int64, dir bool, items int) engineAnalyzeItem {
	return engineAnalyzeItem{ID: a.issue(path, size, dir), Name: filepath.Base(path), Path: path, Size: size, IsDir: dir, Items: items}
}

// folder lists n: its largest entries and the largest files below it.
func (a *engineAnalysis) folder(n *FolderNode) engineAnalyzeFolder {
	buildEntries(n)
	f := engineAnalyzeFolder{ID: a.issue(n.Path, n.Size, true), Path: n.Path, Size: n.Size, Entries: []engineAnalyzeItem{}, Largest: []engineAnalyzeItem{}}
	for i, en := range n.Entries {
		if i == maxAnalyzeEntries {
			f.More = len(n.Entries) - i
			break
		}
		f.Entries = append(f.Entries, a.item(en.Path, en.Size, en.IsDir, en.Items))
	}
	for _, file := range topFiles(n, 10) {
		f.Largest = append(f.Largest, a.item(file.Path, file.Size, false, 0))
	}
	return f
}

// findFolder walks from the root to the scanned folder at path.
func findFolder(n *FolderNode, path string) *FolderNode {
	for n != nil && n.Path != path {
		var next *FolderNode
		for _, sub := range n.SubFolders {
			if path == sub.Path || strings.HasPrefix(path, sub.Path+string(filepath.Separator)) {
				next = sub
				break
			}
		}
		n = next
	}
	return n
}

// analyzeRef returns an issued item that is still in the tree.
func (e *engine) analyzeRef(id int64) (*engineAnalysis, engineAnalyzeRef, error) {
	a := e.analysis
	if a == nil {
		return nil, engineAnalyzeRef{}, badRequest("no analyze.scan yet")
	}
	ref, ok := a.items[id]
	if !ok {
		return nil, engineAnalyzeRef{}, badRequest("item is not from the latest analyze.scan")
	}
	return a, ref, nil
}

func engineAnalyzeScan(e *engine, ctx context.Context, id int64, raw json.RawMessage) (any, error) {
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

	// Progress at most every 100 ms: a disk walk visits thousands of entries a second.
	ch := make(chan scanProgressInfo, 100)
	drained := make(chan struct{})
	go func() {
		defer close(drained)
		var last time.Time
		for pr := range ch {
			if time.Since(last) >= 100*time.Millisecond {
				last = time.Now()
				e.event(id, "progress", map[string]any{"dirs": pr.DirsScanned, "files": pr.FilesScanned, "bytes": pr.TotalSize, "path": pr.CurrentPath})
			}
		}
	}()
	root, _, err := scanDirectoryCtx(ctx, p.Path, ch)
	close(ch)
	<-drained
	if err != nil {
		return nil, err
	}

	files, dirs := countFilesAndFolders(root)
	res := map[string]any{"dirs": dirs, "files": files, "changes": nil}
	baseline, notes := recordScanHistory(root, 0, time.Now())
	if baseline != nil {
		report := explainChanges(baseline, root)
		res["changes"] = &report
	}
	if len(notes) > 0 {
		res["history_notes"] = notes
	}

	a := &engineAnalysis{root: root, items: map[int64]engineAnalyzeRef{}}
	e.state.Lock()
	res["root"] = a.folder(root)
	e.analysis = a
	e.state.Unlock()
	return res, nil
}

func engineAnalyzeChildren(e *engine, _ context.Context, _ int64, raw json.RawMessage) (any, error) {
	var p struct {
		ID int64 `json:"id"`
	}
	if err := decodeParams(raw, &p); err != nil {
		return nil, err
	}
	e.state.Lock()
	defer e.state.Unlock()
	a, ref, err := e.analyzeRef(p.ID)
	if err != nil {
		return nil, err
	}
	n := findFolder(a.root, ref.path)
	if !ref.dir || n == nil {
		return nil, badRequest("item is not a scanned folder")
	}
	return a.folder(n), nil
}

// engineAnalyzeRecycle sends one scanned item to the Recycle Bin (Duster's
// quarantine when the bin refuses it), through the same recyclePath the TUI
// uses, then drops it from the in-memory tree so listings stay current.
func engineAnalyzeRecycle(e *engine, _ context.Context, _ int64, raw json.RawMessage) (any, error) {
	var p struct {
		ID int64 `json:"id"`
	}
	if err := decodeParams(raw, &p); err != nil {
		return nil, err
	}
	e.state.Lock()
	a, ref, err := e.analyzeRef(p.ID)
	if err == nil && ref.path == a.root.Path {
		err = badRequest("the scanned folder itself cannot be recycled")
	}
	if err == nil {
		// The size at issue time is stale once something below was recycled.
		var ok bool
		if ref.size, ok = a.liveSize(ref); !ok {
			err = badRequest("item is no longer in the scan")
		}
	}
	if err == nil && a.undo == nil {
		a.undo = newQuarantineSession("analyze")
	}
	e.state.Unlock()
	if err != nil {
		return nil, err
	}

	kept, err := recyclePath(a.undo, ref.path, ref.size)
	if err != nil {
		return nil, err
	}

	e.state.Lock()
	a.remove(ref)
	e.state.Unlock()
	return map[string]any{"kept": kept, "bytes": ref.size}, nil
}

// liveSize is the item's size in the tree now, or false once it is gone.
func (a *engineAnalysis) liveSize(ref engineAnalyzeRef) (int64, bool) {
	parent := findFolder(a.root, filepath.Dir(ref.path))
	if parent == nil {
		return 0, false
	}
	if ref.dir {
		for _, f := range parent.SubFolders {
			if f.Path == ref.path {
				return f.Size, true
			}
		}
		return 0, false
	}
	for _, f := range parent.Files {
		if f.Path == ref.path {
			return f.Size, true
		}
	}
	return 0, false
}

// remove drops a recycled item from the tree and every ID at or below it.
func (a *engineAnalysis) remove(ref engineAnalyzeRef) {
	parent := findFolder(a.root, filepath.Dir(ref.path))
	if parent != nil {
		if ref.dir {
			parent.SubFolders = slices.DeleteFunc(parent.SubFolders, func(f *FolderNode) bool { return f.Path == ref.path })
		} else {
			parent.Files = slices.DeleteFunc(parent.Files, func(f FileNode) bool { return f.Path == ref.path })
		}
		// Every ancestor's cached listing shows the old size: rebuild them all.
		for n := parent; n != nil; n = n.Parent {
			n.Size -= ref.size
			n.Entries = nil
		}
	}
	prefix := ref.path + string(filepath.Separator)
	for id, r := range a.items {
		if r.path == ref.path || strings.HasPrefix(r.path, prefix) {
			delete(a.items, id)
		}
	}
}
