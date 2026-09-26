package cmd

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Nur-Adnan/duster/lib/elevation"
)

type testEngineMsg struct {
	ID     int64           `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  *engineError    `json:"error"`
	Event  string          `json:"event"`
}

type testEngine struct {
	t    *testing.T
	in   *io.PipeWriter
	msgs chan testEngineMsg
	done chan struct{}
}

// startTestEngine serves an engine over pipes; cats replaces getCategories.
func startTestEngine(t *testing.T, cats []CleanCategory) *testEngine {
	t.Helper()
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	e := newEngine(outW)
	if cats != nil {
		e.cats = func() []CleanCategory { return cats }
	}
	te := &testEngine{t: t, in: inW, msgs: make(chan testEngineMsg, 100), done: make(chan struct{})}
	go func() { e.serve(inR); outW.Close(); close(te.done) }()
	go func() {
		dec := json.NewDecoder(outR)
		for {
			var m testEngineMsg
			if dec.Decode(&m) != nil {
				close(te.msgs)
				return
			}
			te.msgs <- m
		}
	}()
	t.Cleanup(func() { inW.Close(); <-te.done })
	return te
}

func (te *testEngine) send(line string) {
	te.t.Helper()
	if _, err := io.WriteString(te.in, line+"\n"); err != nil {
		te.t.Fatal(err)
	}
}

// reply returns the final message for id, skipping events and other ids.
func (te *testEngine) reply(id int64) testEngineMsg {
	te.t.Helper()
	timeout := time.After(10 * time.Second)
	for {
		select {
		case m, ok := <-te.msgs:
			if !ok {
				te.t.Fatalf("engine closed before replying to %d", id)
			}
			if m.ID == id && m.Event == "" {
				return m
			}
		case <-timeout:
			te.t.Fatalf("no reply to %d", id)
		}
	}
}

func TestEngineFraming(t *testing.T) {
	te := startTestEngine(t, nil)
	te.send(`not json`)
	if m := te.reply(0); m.Error == nil || m.Error.Code != "bad_request" {
		t.Fatalf("bad JSON: got %+v, want bad_request", m)
	}
	te.send(`{"id":0,"method":"hello"}`)
	if m := te.reply(0); m.Error == nil || m.Error.Code != "bad_request" {
		t.Fatalf("id 0: got %+v, want bad_request", m)
	}
	te.send(`{"id":7,"method":"nope"}`)
	if m := te.reply(7); m.Error == nil || m.Error.Code != "unknown_method" {
		t.Fatalf("unknown method: got %+v", m)
	}
	te.send(`{"id":8,"method":"hello"}` + strings.Repeat(" ", maxEngineLine))
	if m := te.reply(0); m.Error == nil || !strings.Contains(m.Error.Message, "1 MiB") {
		t.Fatalf("long line: got %+v", m)
	}
	te.send(`{"id":9,"method":"hello"}`)
	m := te.reply(9)
	var hello struct {
		Protocol int    `json:"protocol"`
		Version  string `json:"version"`
	}
	if m.Error != nil || json.Unmarshal(m.Result, &hello) != nil || hello.Protocol != engineProtocol || hello.Version != AppVersion {
		t.Fatalf("hello after errors: got %+v", m)
	}
	te.send(`{"id":10,"method":"cancel","params":{"id":12345}}`)
	if m := te.reply(10); m.Error != nil {
		t.Fatalf("cancel of an unknown id failed: %+v", m.Error)
	}
}

func TestEngineStdoutReserved(t *testing.T) {
	// The Run func swaps os.Stdout; engine output must only ever go to the
	// writer it was built with, which serve never replaces.
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	old := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = old }()
	te := startTestEngine(t, nil)
	te.send(`{"id":1,"method":"hello"}`)
	te.reply(1)
	w.Close()
	if b, _ := io.ReadAll(r); len(b) != 0 {
		t.Fatalf("engine wrote to os.Stdout: %q", b)
	}
}

func TestEngineStatusAndDoctor(t *testing.T) {
	te := startTestEngine(t, nil)
	te.send(`{"id":1,"method":"status.get"}`)
	m := te.reply(1)
	if runtime.GOOS == "windows" {
		var s struct {
			Top    []any `json:"TopProcesses"` // SystemStats has no json tags
			Health *int  `json:"HealthScore"`
		}
		if m.Error != nil || json.Unmarshal(m.Result, &s) != nil || s.Top != nil || s.Health == nil {
			t.Fatalf("status.get: got %+v, want stats without processes", m)
		}
	} else if m.Error == nil || m.Error.Code != "failed" {
		t.Fatalf("status.get off Windows: got %+v, want failed", m)
	}
	te.send(`{"id":2,"method":"doctor.run"}`)
	var snap DoctorSnapshot
	if m := te.reply(2); m.Error != nil || json.Unmarshal(m.Result, &snap) != nil || len(snap.Results) == 0 {
		t.Fatalf("doctor.run: got %+v", m)
	}
}

func TestEngineCleanScanAndRun(t *testing.T) {
	npm, pip, outside := t.TempDir(), t.TempDir(), t.TempDir()
	for _, f := range []string{filepath.Join(npm, "a.bin"), filepath.Join(pip, "b.bin"), filepath.Join(outside, "keep.bin")} {
		if err := os.WriteFile(f, make([]byte, 10), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	link := filepath.Join(npm, "link")
	hasLink := os.Symlink(outside, link) == nil
	te := startTestEngine(t, []CleanCategory{
		{ID: "npm", Name: "npm", Paths: []string{npm}},
		{ID: "pip", Name: "pip", Paths: []string{pip}},
	})

	te.send(`{"id":1,"method":"clean.run","params":{"ids":["npm"]}}`)
	if m := te.reply(1); m.Error == nil || m.Error.Code != "bad_request" {
		t.Fatalf("clean.run before a scan: got %+v, want bad_request", m)
	}

	te.send(`{"id":2,"method":"clean.scan"}`)
	var scan engineCleanResult
	if m := te.reply(2); m.Error != nil || json.Unmarshal(m.Result, &scan) != nil {
		t.Fatalf("clean.scan: %+v", m)
	}
	if len(scan.Categories) != 2 || scan.Categories[0].Group != "Developer Tools" || scan.Bytes < 20 {
		t.Fatalf("scan = %+v", scan)
	}

	for _, bad := range []string{`[]`, `["nope"]`, `["npm","npm"]`} {
		te.send(`{"id":3,"method":"clean.run","params":{"ids":` + bad + `}}`)
		if m := te.reply(3); m.Error == nil || m.Error.Code != "bad_request" {
			t.Fatalf("clean.run %s: got %+v, want bad_request", bad, m)
		}
	}

	te.send(`{"id":4,"method":"clean.run","params":{"ids":["npm"]}}`)
	var run engineCleanResult
	if m := te.reply(4); m.Error != nil || json.Unmarshal(m.Result, &run) != nil {
		t.Fatalf("clean.run: %+v", m)
	}
	if len(run.Categories) != 1 || run.Categories[0].ID != "npm" {
		t.Fatalf("run = %+v, want only npm", run)
	}
	if _, err := os.Stat(filepath.Join(npm, "a.bin")); !os.IsNotExist(err) {
		t.Error("npm file survived the clean")
	}
	if _, err := os.Stat(filepath.Join(pip, "b.bin")); err != nil {
		t.Error("unselected pip category was touched")
	}
	if _, err := os.Stat(filepath.Join(outside, "keep.bin")); hasLink && err != nil {
		t.Error("clean followed a symlink out of its root")
	}
}

func TestEngineCleanAdminOnly(t *testing.T) {
	if elevation.IsAdmin() {
		t.Skip("elevated: prefetch is not blocked")
	}
	dir := t.TempDir()
	f := filepath.Join(dir, "x.pf")
	if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	te := startTestEngine(t, []CleanCategory{{ID: "prefetch", Name: "Prefetch", Paths: []string{dir}}})
	te.send(`{"id":1,"method":"clean.scan"}`)
	te.reply(1)
	te.send(`{"id":2,"method":"clean.run","params":{"ids":["prefetch"]}}`)
	var run engineCleanResult
	if m := te.reply(2); m.Error != nil || json.Unmarshal(m.Result, &run) != nil {
		t.Fatalf("clean.run: %+v", m)
	}
	if c := run.Categories[0]; !c.AdminRequired || c.Error != "admin_required" {
		t.Fatalf("prefetch entry = %+v, want admin_required", c)
	}
	if _, err := os.Stat(f); err != nil {
		t.Error("admin-only category was touched while unelevated")
	}
}

// blockingCats returns two categories; cleaning the first blocks until
// release is closed, and ran records which ones were cleaned.
func blockingCats(started, release chan struct{}, ran *[]string) []CleanCategory {
	mk := func(id string, block bool) CleanCategory {
		return CleanCategory{ID: id, Name: id, CustomScan: func(scanOnly, _ bool) (int64, int, error) {
			if scanOnly {
				return 1, 1, nil
			}
			*ran = append(*ran, id)
			if block {
				close(started)
				<-release
			}
			return 1, 1, nil
		}}
	}
	return []CleanCategory{mk("npm", true), mk("pip", false)}
}

func TestEngineCancelBusyAndConcurrentReads(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	var ran []string
	te := startTestEngine(t, blockingCats(started, release, &ran))
	te.send(`{"id":1,"method":"clean.scan"}`)
	te.reply(1)
	te.send(`{"id":2,"method":"clean.run","params":{"ids":["npm","pip"]}}`)
	<-started

	te.send(`{"id":3,"method":"clean.run","params":{"ids":["pip"]}}`)
	if m := te.reply(3); m.Error == nil || m.Error.Code != "busy" {
		t.Fatalf("second clean.run: got %+v, want busy", m)
	}
	te.send(`{"id":4,"method":"hello"}`)
	if m := te.reply(4); m.Error != nil {
		t.Fatalf("hello during a clean: %+v", m.Error)
	}
	te.send(`{"id":5,"method":"cancel","params":{"id":2}}`)
	te.reply(5)
	close(release)

	m := te.reply(2)
	if m.Error == nil || m.Error.Code != "canceled" {
		t.Fatalf("canceled clean.run: got %+v", m)
	}
	data, _ := json.Marshal(m.Error.Data)
	var partial engineCleanResult
	if json.Unmarshal(data, &partial) != nil || len(partial.Categories) != 1 || partial.Categories[0].ID != "npm" {
		t.Fatalf("partial result = %s, want only npm", data)
	}
	if len(ran) != 1 {
		t.Fatalf("ran %v after cancel, want only npm", ran)
	}
}

func TestEngineStdinEOFStopsSafely(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	var ran []string
	te := startTestEngine(t, blockingCats(started, release, &ran))
	te.send(`{"id":1,"method":"clean.scan"}`)
	te.reply(1)
	te.send(`{"id":2,"method":"clean.run","params":{"ids":["npm","pip"]}}`)
	<-started
	te.in.Close()
	select {
	case <-te.done:
		t.Fatal("engine exited while a category was mid-clean")
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	<-te.done
	if len(ran) != 1 {
		t.Fatalf("ran %v after stdin closed, want only the in-flight npm", ran)
	}
}
