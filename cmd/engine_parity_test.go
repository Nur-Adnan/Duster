package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Nur-Adnan/duster/lib/uninstall"
)

// call sends one request and returns its final reply.
func (te *testEngine) call(id int64, method, params string) testEngineMsg {
	te.t.Helper()
	line := `{"id":` + itoa(id) + `,"method":"` + method + `"`
	if params != "" {
		line += `,"params":` + params
	}
	te.send(line + "}")
	return te.reply(id)
}

func itoa(n int64) string { b, _ := json.Marshal(n); return string(b) }

func wantCode(t *testing.T, what string, m testEngineMsg, code string) {
	t.Helper()
	if m.Error == nil || m.Error.Code != code {
		t.Fatalf("%s: want %s, got %+v (result %s)", what, code, m.Error, m.Result)
	}
}

func decodeResult[T any](t *testing.T, what string, m testEngineMsg) T {
	t.Helper()
	var v T
	if m.Error != nil {
		t.Fatalf("%s: %+v", what, m.Error)
	}
	if err := json.Unmarshal(m.Result, &v); err != nil {
		t.Fatalf("%s: %v in %s", what, err, m.Result)
	}
	return v
}

func TestEngineParityMethodsRegistered(t *testing.T) {
	mutating := []string{"startup.toggle", "startup.remove", "purge.run", "installer.run", "uninstall.run", "uninstall.sweep",
		"optimize.run", "vdisk.run", "schedule.set", "schedule.off", "update.install", "remove.run", "restore.list"}
	readOnly := []string{"verify.run", "benchmark.run", "security.run", "drivers.list", "oplog.list", "startup.list", "purge.scan",
		"installer.scan", "uninstall.list", "optimize.list", "vdisk.scan", "schedule.get", "update.check", "remove.plan", "analyze.reveal"}
	for _, name := range mutating {
		if m, ok := engineMethods[name]; !ok || !m.mutates {
			t.Errorf("%s: registered %v, mutates %v; want a busy-holding method", name, ok, m.mutates)
		}
	}
	for _, name := range readOnly {
		if m, ok := engineMethods[name]; !ok || m.mutates {
			t.Errorf("%s: registered %v, mutates %v; want read-only", name, ok, m.mutates)
		}
	}
}

func TestEnginePurgeScanRun(t *testing.T) {
	work := tempQuarantine(t)
	for _, proj := range []string{"a", "b"} {
		writeTree(t, filepath.Join(work, proj, "node_modules"))
		if err := os.WriteFile(filepath.Join(work, proj, "package.json"), []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	te := startTestEngine(t, nil)

	wantCode(t, "relative path", te.call(1, "purge.scan", `{"path":"relative"}`), "bad_request")
	wantCode(t, "run before scan", te.call(2, "purge.run", `{"ids":[1]}`), "bad_request")

	type item struct {
		ID   int    `json:"id"`
		Path string `json:"path"`
		Size int64  `json:"size"`
	}
	scan := decodeResult[struct {
		Artifacts []item `json:"artifacts"`
		Bytes     int64  `json:"bytes"`
	}](t, "purge.scan", te.call(3, "purge.scan", `{"path":`+jsonString(work)+`}`))
	if len(scan.Artifacts) != 2 || scan.Bytes != 10 {
		t.Fatalf("purge.scan: %+v", scan)
	}
	byName := map[string]item{}
	for _, a := range scan.Artifacts {
		byName[filepath.Base(filepath.Dir(a.Path))] = a
	}

	wantCode(t, "bad mode", te.call(4, "purge.run", `{"ids":[1],"mode":"shred"}`), "bad_request")
	wantCode(t, "repeated id", te.call(5, "purge.run", `{"ids":[1,1]}`), "bad_request")
	wantCode(t, "unlisted id", te.call(6, "purge.run", `{"ids":[3]}`), "bad_request")

	type tally struct {
		Freed     int64 `json:"freed"`
		Kept      int64 `json:"kept"`
		KeptCount int   `json:"kept_count"`
		Failed    int   `json:"failed"`
	}
	kept := decodeResult[tally](t, "keep", te.call(7, "purge.run", `{"ids":[`+itoa(int64(byName["a"].ID))+`]}`))
	if kept.KeptCount != 1 || kept.Kept != 5 || kept.Freed != 0 {
		t.Fatalf("keep reported %+v: kept bytes must not count as freed", kept)
	}
	perm := decodeResult[tally](t, "permanent", te.call(8, "purge.run", `{"ids":[`+itoa(int64(byName["b"].ID))+`],"mode":"permanent"}`))
	if perm.Freed != 5 || perm.KeptCount != 0 {
		t.Fatalf("permanent reported %+v", perm)
	}
	for _, p := range []string{byName["a"].Path, byName["b"].Path} {
		if _, err := os.Lstat(p); !os.IsNotExist(err) {
			t.Fatalf("%s still there", p)
		}
	}
	if s := groupSessions(loadKeptSessions(quarantineRoots())); len(s) != 1 {
		t.Fatalf("want one kept session, got %d", len(s))
	}

	// Still listed but gone from disk: reported failed, never guessed at.
	again := decodeResult[tally](t, "gone", te.call(9, "purge.run", `{"ids":[`+itoa(int64(byName["a"].ID))+`]}`))
	if again.Failed != 1 {
		t.Fatalf("a vanished artifact was not reported failed: %+v", again)
	}
}

func TestScanArtifactsCtxStopsWhenCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := scanArtifactsCtx(ctx, t.TempDir(), nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("want context.Canceled, got %v", err)
	}
}

func TestEngineInstallerScanRun(t *testing.T) {
	tempQuarantine(t)
	profile := t.TempDir()
	t.Setenv("USERPROFILE", profile)
	downloads := filepath.Join(profile, "Downloads")
	old := time.Now().Add(-10 * 24 * time.Hour)
	for name, size := range map[string]int{"old.exe": 2 << 20, "small.msi": 100, "notes.txt": 2 << 20} {
		p := filepath.Join(downloads, name)
		if err := os.MkdirAll(downloads, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, make([]byte, size), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(p, old, old); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(downloads, "new.exe"), make([]byte, 2<<20), 0o644); err != nil {
		t.Fatal(err)
	}
	te := startTestEngine(t, nil)

	scan := decodeResult[struct {
		Items []struct {
			ID   int    `json:"id"`
			Name string `json:"name"`
		} `json:"items"`
	}](t, "installer.scan", te.call(1, "installer.scan", `{"min_size_mb":1}`))
	if len(scan.Items) != 1 || scan.Items[0].Name != "old.exe" {
		t.Fatalf("want only old.exe (>= 7 days, >= 1 MB, installer type): %+v", scan.Items)
	}
	wantCode(t, "unlisted", te.call(2, "installer.run", `{"ids":[2]}`), "bad_request")
	res := decodeResult[engineKeepResult](t, "installer.run", te.call(3, "installer.run", `{"ids":[1]}`))
	if res.Kept != 1 || res.Failed != 0 {
		t.Fatalf("installer.run: %+v", res)
	}
	if _, err := os.Lstat(filepath.Join(downloads, "old.exe")); !os.IsNotExist(err) {
		t.Fatal("old.exe was not kept in the quarantine")
	}
}

func TestEngineUninstallRefusals(t *testing.T) {
	e := newEngine(io.Discard)
	ctx := context.Background()
	if _, err := engineUninstallRun(e, ctx, 1, json.RawMessage(`{"id":1}`)); !isCode(err, "bad_request") {
		t.Fatalf("uninstall.run before a list: %v", err)
	}
	// A protected app is refused before anything is looked up or run.
	setListing(e, "apps", []uninstall.InstalledApp{{Name: "Microsoft Visual C++ 2015 Redistributable", UninstallString: "never-run.exe"}})
	if _, err := engineUninstallRun(e, ctx, 1, json.RawMessage(`{"id":1}`)); !isCode(err, "bad_request") {
		t.Fatalf("protected app: %v", err)
	}
	if _, err := engineUninstallSweep(e, ctx, 1, json.RawMessage(`{"ids":[1]}`)); !isCode(err, "bad_request") {
		t.Fatalf("sweep without leftovers: %v", err)
	}
	for name, want := range map[string]bool{"Microsoft Visual C++ 2019 x64": true, "Security Update for Windows (KB5034441)": true, "7-Zip 23.01": false} {
		if got := isProtectedApp(name); got != want {
			t.Errorf("isProtectedApp(%q) = %v, want %v", name, got, want)
		}
	}
}

func isCode(err error, code string) bool {
	var ee *engineError
	return errors.As(err, &ee) && ee.Code == code
}

func TestEngineStartupRemoveOnlyDisabled(t *testing.T) {
	e := newEngine(io.Discard)
	setListing(e, "startup", []startupEntry{{Name: "App", Command: `C:\app.exe`, Location: "HKCU\\Run", Enabled: true}})
	if _, err := engineStartupRemove(e, context.Background(), 1, json.RawMessage(`{"ids":[1]}`)); !isCode(err, "bad_request") {
		t.Fatalf("removing an enabled entry: %v", err)
	}
	if _, err := engineStartupToggle(e, context.Background(), 1, json.RawMessage(`{"id":2}`)); !isCode(err, "bad_request") {
		t.Fatalf("toggling an unlisted entry: %v", err)
	}
}

func TestEngineOptimizeVdiskScheduleRefusals(t *testing.T) {
	tempQuarantine(t)
	te := startTestEngine(t, nil)
	wantCode(t, "no ids", te.call(1, "optimize.run", `{"ids":[]}`), "bad_request")
	wantCode(t, "unknown task", te.call(2, "optimize.run", `{"ids":["dns","defrag-everything"]}`), "bad_request")

	// A preview skips everything but the component store, so nothing runs here.
	prev := decodeResult[struct {
		Tasks []engineOptimizeTask `json:"tasks"`
	}](t, "preview", te.call(3, "optimize.run", `{"ids":["delivery_opt","dns"],"dry_run":true}`))
	if len(prev.Tasks) != 2 || prev.Tasks[0].ID != "dns" || prev.Tasks[0].Status != "skipped" || prev.Tasks[1].Status != "skipped" {
		t.Fatalf("preview must keep the CLI's task order and skip both: %+v", prev.Tasks)
	}

	wantCode(t, "vdisk.run unlisted", te.call(4, "vdisk.run", `{"ids":[1]}`), "bad_request")
	for i, params := range []string{`{"every":"hourly","at":"19:00","low_space":"10%"}`, `{"every":"weekly","at":"25:00","low_space":"10%"}`,
		`{"every":"weekly","at":"19:00","low_space":"90%"}`, `{"every":"weekly","at":"19:00","low_space":"10%","add":["prefetch"]}`} {
		wantCode(t, "schedule.set "+params, te.call(int64(10+i), "schedule.set", params), "bad_request")
	}
}

func TestEngineUpdateUsesTheReleaseCheck(t *testing.T) {
	prev, prevVersion := engineFetchRelease, AppVersion
	AppVersion = "1.4.0"
	t.Cleanup(func() { engineFetchRelease, AppVersion = prev, prevVersion })
	te := startTestEngine(t, nil)
	cases := []struct {
		tag       string
		available bool
	}{{"v0.0.1", false}, {"v" + AppVersion, false}, {"v99.0.0", true}, {"v99.0.0-rc.1", false}}
	for i, c := range cases {
		engineFetchRelease = func() (releaseMetadata, error) { return releaseMetadata{TagName: c.tag}, nil }
		got := decodeResult[struct {
			Available bool `json:"available"`
		}](t, c.tag, te.call(int64(i+1), "update.check", ""))
		if got.Available != c.available {
			t.Errorf("%s: available %v, want %v", c.tag, got.Available, c.available)
		}
	}
	engineFetchRelease = func() (releaseMetadata, error) { return releaseMetadata{TagName: "v" + AppVersion}, nil }
	wantCode(t, "install when up to date", te.call(10, "update.install", ""), "bad_request")
	// Newer but with no archive or checksums: refused before any download or swap.
	engineFetchRelease = func() (releaseMetadata, error) { return releaseMetadata{TagName: "v99.0.0"}, nil }
	wantCode(t, "install without assets", te.call(11, "update.install", ""), "failed")
}

func TestEngineRemovePlanAndOplog(t *testing.T) {
	tempQuarantine(t)
	logDir := filepath.Join(os.Getenv("LOCALAPPDATA"), "Duster")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		t.Fatal(err)
	}
	log := "2026-09-27 01:00:00 | Command: purge | Action: quarantine | Target: C:\\p\\node_modules | Size: 5 bytes | Status: SUCCESS\n" +
		"2026-09-27 02:00:00 | Command: clean | Action: delete | Target: C:\\t | Size: 7 bytes | Status: SUCCESS\n"
	if err := os.WriteFile(filepath.Join(logDir, "operations.log"), []byte(log), 0o644); err != nil {
		t.Fatal(err)
	}
	te := startTestEngine(t, nil)
	ops := decodeResult[struct {
		Entries []engineOplogEntry `json:"entries"`
	}](t, "oplog.list", te.call(1, "oplog.list", ""))
	if len(ops.Entries) != 2 || ops.Entries[0].Command != "clean" || ops.Entries[0].Size != 7 {
		t.Fatalf("oplog.list must be newest first: %+v", ops.Entries)
	}
	plan := decodeResult[struct {
		Exe     string `json:"exe"`
		DataDir string `json:"data_dir"`
	}](t, "remove.plan", te.call(2, "remove.plan", ""))
	if plan.Exe == "" || plan.DataDir != logDir {
		t.Fatalf("remove.plan: %+v", plan)
	}
}

func TestEngineWindowsOnlyViewsFailCleanly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the real views read the registry and run PowerShell; e2e covers them")
	}
	te := startTestEngine(t, nil)
	for i, m := range []string{"security.run", "drivers.list", "startup.list", "uninstall.list"} {
		wantCode(t, m, te.call(int64(i+1), m, ""), "failed")
	}
}

func TestEngineAnalyzeOptionsAndRestorePreview(t *testing.T) {
	work := tempQuarantine(t)
	root := filepath.Join(work, "root")
	writeTree(t, filepath.Join(root, "grow"))
	te := startTestEngine(t, nil)
	scanOf := func(id int64, extra string) testEngineMsg {
		return te.call(id, "analyze.scan", `{"path":`+jsonString(root)+extra+`}`)
	}
	wantCode(t, "bad since", scanOf(1, `,"since":"soon"`), "bad_request")

	decodeResult[json.RawMessage](t, "first scan", scanOf(2, ""))
	if err := os.WriteFile(filepath.Join(root, "grow", "sub", "big.bin"), make([]byte, 5<<20), 0o644); err != nil {
		t.Fatal(err)
	}
	quiet := decodeResult[struct {
		Changes *json.RawMessage `json:"changes"`
	}](t, "no_history", scanOf(3, `,"no_history":true`))
	if quiet.Changes != nil {
		t.Fatal("no_history must neither compare nor report changes")
	}
	res := decodeResult[struct {
		Root struct {
			Entries []engineAnalyzeItem `json:"entries"`
		} `json:"root"`
		Changes *struct {
			Entries []struct {
				ID   int64  `json:"id"`
				Path string `json:"path"`
			} `json:"entries"`
		} `json:"changes"`
	}](t, "second scan", scanOf(4, ""))
	if res.Changes == nil || len(res.Changes.Entries) == 0 {
		t.Fatalf("second scan reported no changes")
	}
	// As the TUI's Enter: a change opens its parent folder.
	deepest := res.Changes.Entries[0]
	for _, c := range res.Changes.Entries {
		if len(c.Path) > len(deepest.Path) {
			deepest = c
		}
	}
	if deepest.ID == 0 || !strings.HasPrefix(deepest.Path, filepath.Join(root, "grow")) {
		t.Fatalf("the deepest change carries no folder to open: %+v", res.Changes.Entries)
	}
	folder := decodeResult[struct {
		Path  string               `json:"path"`
		Trail []engineAnalyzeCrumb `json:"trail"`
	}](t, "open change", te.call(5, "analyze.children", `{"id":`+itoa(deepest.ID)+`}`))
	if folder.Path != filepath.Dir(deepest.Path) {
		t.Fatalf("change %s opened %s; want its parent", deepest.Path, folder.Path)
	}
	// Every listing below the scanned folder carries its breadcrumb from there.
	var grow int64
	for _, en := range res.Root.Entries {
		if en.Name == "grow" {
			grow = en.ID
		}
	}
	sub := decodeResult[struct {
		Trail []engineAnalyzeCrumb `json:"trail"`
	}](t, "open grow", te.call(15, "analyze.children", `{"id":`+itoa(grow)+`}`))
	if len(sub.Trail) != 1 || sub.Trail[0].Path != root {
		t.Fatalf("grow's breadcrumb: %+v", sub.Trail)
	}
	wantCode(t, "reveal unknown change", te.call(6, "analyze.reveal", `{"change":99}`), "bad_request")
	wantCode(t, "reveal unknown id", te.call(7, "analyze.reveal", `{"id":99999}`), "bad_request")

	// Restore preview: reported, nothing moved.
	target := filepath.Join(work, "kept")
	writeTree(t, target)
	if err := quarantinePath(newQuarantineSession("purge"), target, 5); err != nil {
		t.Fatal(err)
	}
	list := decodeResult[struct {
		Sessions []restoreSessionJSON `json:"sessions"`
	}](t, "restore.list", te.call(8, "restore.list", ""))
	if len(list.Sessions) != 1 {
		t.Fatalf("restore.list: %+v", list)
	}
	prev := decodeResult[struct {
		Results []restoreResult `json:"results"`
	}](t, "preview", te.call(9, "restore.run", `{"id":`+jsonString(list.Sessions[0].ID)+`,"dry_run":true}`))
	if len(prev.Results) != 1 || prev.Results[0].Status != "would restore" {
		t.Fatalf("preview: %+v", prev.Results)
	}
	if _, err := os.Lstat(target); !os.IsNotExist(err) {
		t.Fatal("a preview moved the item back")
	}
}

func jsonString(s string) string { b, _ := json.Marshal(s); return string(b) }
