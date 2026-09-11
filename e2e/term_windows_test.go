package e2e

import (
	"bytes"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	kernel32                      = windows.NewLazySystemDLL("kernel32.dll")
	user32                        = windows.NewLazySystemDLL("user32.dll")
	procUpdateProcThreadAttribute = kernel32.NewProc("UpdateProcThreadAttribute")
	procPostMessage               = user32.NewProc("PostMessageW")
	procGetWindowText             = user32.NewProc("GetWindowTextW")
	procIsWindow                  = user32.NewProc("IsWindow")
)

// Keys as a terminal sends them.
const (
	keyEnter     = "\r"
	keyBackspace = "\x7f"
	keyDown      = "\x1b[B"
)

const cols, rows = 120, 40

// term is du.exe running in a pseudo console, as it would in Windows Terminal.
type term struct {
	t    *testing.T
	pc   windows.Handle
	in   *os.File
	proc windows.Handle
	pid  uint32

	mu  sync.Mutex
	out bytes.Buffer
}

// duBin is the binary under test. Unset means this is not a throwaway runner.
func duBin(t *testing.T) string {
	t.Helper()
	bin := os.Getenv("DU_E2E_BIN")
	if bin == "" {
		t.Skip("DU_E2E_BIN is not set: these tests change the machine and run only in windows-smoke.yml")
	}
	return bin
}

// start runs du.exe with args in a new pseudo console.
func start(t *testing.T, args ...string) *term {
	t.Helper()
	bin := duBin(t)

	var inR, inW, outR, outW windows.Handle
	if err := windows.CreatePipe(&inR, &inW, nil, 0); err != nil {
		t.Fatal(err)
	}
	if err := windows.CreatePipe(&outR, &outW, nil, 0); err != nil {
		t.Fatal(err)
	}
	var pc windows.Handle
	if err := windows.CreatePseudoConsole(windows.Coord{X: cols, Y: rows}, inR, outW, 0, &pc); err != nil {
		t.Fatalf("CreatePseudoConsole: %v", err)
	}
	// The pseudo console keeps its own copies of these ends.
	windows.CloseHandle(inR)
	windows.CloseHandle(outW)

	attrs, err := windows.NewProcThreadAttributeList(1)
	if err != nil {
		t.Fatal(err)
	}
	defer attrs.Delete()
	// This attribute's value is the HPCON itself, not a pointer to it, which
	// attrs.Update can't express.
	if r, _, err := procUpdateProcThreadAttribute.Call(uintptr(unsafe.Pointer(attrs.List())), 0,
		windows.PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE, uintptr(pc), unsafe.Sizeof(pc), 0, 0); r == 0 {
		t.Fatalf("UpdateProcThreadAttribute: %v", err)
	}

	si := windows.StartupInfoEx{ProcThreadAttributeList: attrs.List()}
	si.Cb = uint32(unsafe.Sizeof(si))
	// Invalid std handles make the child use the pseudo console; otherwise it
	// can inherit this test's redirected output instead.
	si.Flags = windows.STARTF_USESTDHANDLES
	si.StdInput, si.StdOutput, si.StdErr = windows.InvalidHandle, windows.InvalidHandle, windows.InvalidHandle

	line, err := windows.UTF16PtrFromString(windows.ComposeCommandLine(append([]string{bin}, args...)))
	if err != nil {
		t.Fatal(err)
	}
	var pi windows.ProcessInformation
	if err := windows.CreateProcess(nil, line, nil, nil, false, windows.EXTENDED_STARTUPINFO_PRESENT, nil, nil, &si.StartupInfo, &pi); err != nil {
		t.Fatalf("start du %v: %v", args, err)
	}
	windows.CloseHandle(pi.Thread)

	tm := &term{t: t, pc: pc, in: os.NewFile(uintptr(inW), "pty-in"), proc: pi.Process, pid: pi.ProcessId}
	out := os.NewFile(uintptr(outR), "pty-out")
	go func() {
		b := make([]byte, 8192)
		for {
			n, err := out.Read(b)
			tm.mu.Lock()
			tm.out.Write(b[:n])
			tm.mu.Unlock()
			if err != nil {
				return
			}
		}
	}()
	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("du %s, last screen:\n%s", strings.Join(args, " "), tm.screen())
		}
		_ = windows.TerminateProcess(tm.proc, 1) // no-op once it has exited
		windows.CloseHandle(tm.proc)
		windows.ClosePseudoConsole(tm.pc)
		tm.in.Close()
		out.Close()
	})
	return tm
}

// send types keys one at a time, the way a person does. Bubble Tea reads a
// burst of characters as one paste, which a search box ignores.
func (tm *term) send(keys ...string) {
	tm.t.Helper()
	for _, k := range keys {
		if _, err := tm.in.WriteString(k); err != nil {
			tm.t.Fatalf("send %q: %v", k, err)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func (tm *term) typeText(s string) {
	tm.t.Helper()
	for _, r := range s {
		tm.send(string(r))
	}
}

// screen is what the terminal shows now. The pseudo console redraws only what
// changed, so the raw stream can't answer that; it is replayed onto a grid.
// ponytail: replays the whole stream per call; fine for minutes of output.
func (tm *term) screen() string {
	tm.mu.Lock()
	raw := tm.out.String()
	tm.mu.Unlock()
	return render(raw)
}

// onScreen reports whether s is on screen, ignoring spacing and line wraps.
func (tm *term) onScreen(s string) bool {
	return strings.Contains(squash(tm.screen()), squash(s))
}

func (tm *term) waitFor(want string, timeout time.Duration) {
	tm.t.Helper()
	for end := time.Now().Add(timeout); !tm.onScreen(want); time.Sleep(100 * time.Millisecond) {
		if time.Now().After(end) {
			tm.t.Fatalf("%q was not on screen within %v", want, timeout)
		}
	}
}

// waitExit waits for du to exit and returns its exit code.
func (tm *term) waitExit(timeout time.Duration) uint32 {
	tm.t.Helper()
	if ev, err := windows.WaitForSingleObject(tm.proc, uint32(timeout.Milliseconds())); ev != windows.WAIT_OBJECT_0 {
		tm.t.Fatalf("du did not exit within %v (%v)", timeout, err)
	}
	var code uint32
	if err := windows.GetExitCodeProcess(tm.proc, &code); err != nil {
		tm.t.Fatal(err)
	}
	return code
}

// squash drops whitespace and box-drawing characters, so text that wraps
// inside a bordered box still reads as one string.
func squash(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) || (r >= 0x2500 && r <= 0x257f) {
			return -1
		}
		return r
	}, s)
}

// render replays VT output onto a cols x rows grid: printable text, cursor
// moves, erases and scrolling, which is what the pseudo console emits.
func render(s string) string {
	blank := func() []rune { return []rune(strings.Repeat(" ", cols)) }
	g := make([][]rune, rows)
	for i := range g {
		g[i] = blank()
	}
	x, y := 0, 0
	scrollUp := func() {
		copy(g, g[1:])
		g[rows-1] = blank()
	}
	erase := func(row, from, to int) {
		for i := max(from, 0); i < min(to, cols); i++ {
			g[row][i] = ' '
		}
	}
	newline := func() {
		if y++; y >= rows {
			scrollUp()
			y = rows - 1
		}
	}

	rs := []rune(s)
	for i := 0; i < len(rs); i++ {
		r := rs[i]
		switch {
		case r == 0x1b && i+1 < len(rs) && rs[i+1] == '[':
			j := i + 2
			for j < len(rs) && (rs[j] < 0x40 || rs[j] > 0x7e) {
				j++
			}
			if j >= len(rs) {
				i = len(rs)
				break
			}
			params, final := string(rs[i+2:j]), rs[j]
			i = j
			p := csiParams(params)
			n := func(k, def int) int {
				if k < len(p) && p[k] > 0 {
					return p[k]
				}
				return def
			}
			mode := 0
			if len(p) > 0 {
				mode = p[0]
			}
			switch final {
			case 'H', 'f':
				y, x = n(0, 1)-1, n(1, 1)-1
			case 'A':
				y -= n(0, 1)
			case 'B':
				y += n(0, 1)
			case 'C':
				x += n(0, 1)
			case 'D':
				x -= n(0, 1)
			case 'G':
				x = n(0, 1) - 1
			case 'd':
				y = n(0, 1) - 1
			case 'K':
				switch mode {
				case 0:
					erase(y, x, cols)
				case 1:
					erase(y, 0, x+1)
				case 2:
					erase(y, 0, cols)
				}
			case 'J':
				from, to := 0, rows
				switch mode {
				case 0:
					erase(y, x, cols)
					from = y + 1
				case 1:
					erase(y, 0, x+1)
					to = y
				}
				for row := from; row < to; row++ {
					erase(row, 0, cols)
				}
			case 'X':
				erase(y, x, x+n(0, 1))
			case 'P':
				k := min(n(0, 1), cols-x)
				copy(g[y][x:], g[y][x+k:])
				erase(y, cols-k, cols)
			case '@':
				k := min(n(0, 1), cols-x)
				copy(g[y][x+k:], g[y][x:cols-k])
				erase(y, x, x+k)
			case 'S':
				for k := 0; k < n(0, 1); k++ {
					scrollUp()
				}
			case 'h', 'l':
				if params == "?1049" { // alternate screen on or off: a fresh page
					for row := range g {
						g[row] = blank()
					}
					x, y = 0, 0
				}
			}
			x, y = min(max(x, 0), cols-1), min(max(y, 0), rows-1)
		case r == 0x1b && i+1 < len(rs) && rs[i+1] == ']': // OSC, up to BEL or ESC \
			j := i + 2
			for j < len(rs) && rs[j] != 0x07 && rs[j] != 0x1b {
				j++
			}
			if j < len(rs) && rs[j] == 0x1b {
				j++
			}
			i = j
		case r == 0x1b:
			i++ // two-character escape (ESC 7, ESC =, ...)
		case r == '\r':
			x = 0
		case r == '\n':
			newline()
		case r == '\b':
			x = max(x-1, 0)
		case r == '\t':
			x = min((x/8+1)*8, cols-1)
		case r < 0x20 || r == 0x7f:
		default:
			if x >= cols {
				x = 0
				newline()
			}
			g[y][x] = r
			x++
		}
	}
	lines := make([]string, rows)
	for i := range g {
		lines[i] = strings.TrimRight(string(g[i]), " ")
	}
	return strings.TrimRight(strings.Join(lines, "\n"), "\n")
}

func csiParams(s string) []int {
	var out []int
	for _, f := range strings.Split(strings.TrimLeft(s, "?<=>"), ";") {
		v, _ := strconv.Atoi(f)
		out = append(out, v)
	}
	return out
}

// Windows dialog boxes (class #32770), answered with the button a person would click.
const (
	wmClose   = 0x0010
	wmCommand = 0x0111
	idOK      = 1
	idYes     = 6
	idNo      = 7
)

type dialog struct {
	hwnd  windows.HWND
	title string
}

// One callback for the whole run: Windows caps how many a process may create.
var (
	enumMu    sync.Mutex
	enumPID   uint32
	enumFound []dialog
	enumCB    = windows.NewCallback(func(h windows.HWND, _ uintptr) uintptr {
		var owner uint32
		if _, err := windows.GetWindowThreadProcessId(h, &owner); err != nil || (enumPID != 0 && owner != enumPID) {
			return 1
		}
		cls := make([]uint16, 64)
		if n, _ := windows.GetClassName(h, &cls[0], int32(len(cls))); windows.UTF16ToString(cls[:n]) != "#32770" || !windows.IsWindowVisible(h) {
			return 1
		}
		buf := make([]uint16, 256)
		n, _, _ := procGetWindowText.Call(uintptr(h), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
		enumFound = append(enumFound, dialog{h, windows.UTF16ToString(buf[:n])})
		return 1
	})
)

// dialogs lists the visible dialog boxes on this desktop; pid 0 means any process.
func dialogs(pid uint32) []dialog {
	enumMu.Lock()
	defer enumMu.Unlock()
	enumPID, enumFound = pid, nil
	_ = windows.EnumWindows(enumCB, nil)
	return enumFound
}

func isWindow(h windows.HWND) bool {
	r, _, _ := procIsWindow.Call(uintptr(h))
	return r != 0
}

// answer waits for a dialog whose title contains title (from process pid, or
// any process when pid is 0), clicks button, and waits for it to close.
// It returns the dialog's title.
func answer(t *testing.T, pid uint32, title string, button uintptr, timeout time.Duration) string {
	t.Helper()
	closed := func(h windows.HWND) bool {
		for i := 0; i < 50 && isWindow(h); i++ {
			time.Sleep(100 * time.Millisecond)
		}
		return !isWindow(h)
	}
	for end := time.Now().Add(timeout); ; time.Sleep(250 * time.Millisecond) {
		for _, d := range dialogs(pid) {
			if !strings.Contains(d.title, title) {
				continue
			}
			procPostMessage.Call(uintptr(d.hwnd), wmCommand, button, 0)
			if closed(d.hwnd) {
				return d.title
			}
			procPostMessage.Call(uintptr(d.hwnd), wmClose, 0, 0)
			if closed(d.hwnd) {
				return d.title
			}
			t.Fatalf("dialog %q did not close", d.title)
		}
		if time.Now().After(end) {
			t.Fatalf("no dialog titled %q within %v (open dialogs: %v)", title, timeout, dialogs(0))
		}
	}
}
