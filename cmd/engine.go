package cmd

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
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
