// Command duw is Duster's windowless launcher. Task Scheduler starts it for a
// scheduled clean; it is built with -H=windowsgui, so Windows gives it no
// console, and it starts du.exe from its own folder without one, appending
// du's output to %LOCALAPPDATA%\Duster\schedule.log. It only ever runs
// `du.exe schedule run ...`: it is not a general hidden runner.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/Nur-Adnan/duster/internal/logging"
)

const maxLogBytes = 1 << 20

func main() {
	exe, err := os.Executable()
	if err != nil {
		os.Exit(1)
	}
	os.Exit(run(os.Args[1:], exe, logging.Dir()))
}

func allowedArgs(args []string) bool {
	return len(args) >= 2 && args[0] == "schedule" && args[1] == "run"
}

func run(args []string, exe, logDir string) int {
	if !allowedArgs(args) {
		return 2
	}
	var logw io.Writer = io.Discard
	if f, err := openScheduleLog(logDir); err == nil {
		defer f.Close()
		logw = f
	}
	fmt.Fprintf(logw, "=== %s %s\n", time.Now().Format(time.RFC3339), strings.Join(args, " "))

	// du.exe from duw.exe's own folder, never a PATH lookup.
	du := filepath.Join(filepath.Dir(exe), "du.exe")
	c := exec.Command(du, args...) // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command
	c.Stdout, c.Stderr = logw, logw
	hideWindow(c)
	err := c.Run()
	var exitErr *exec.ExitError
	switch {
	case err == nil:
		return 0
	case errors.As(err, &exitErr):
		return exitErr.ExitCode()
	default:
		fmt.Fprintf(logw, "could not start %s: %v\n", du, err)
		return 1
	}
}

// openScheduleLog opens schedule.log for appending, keeping one rotated .old
// once it passes 1 MB. It refuses a linked folder or log, so a planted link
// cannot redirect the writes.
func openScheduleLog(dir string) (*os.File, error) {
	if dir == "" {
		return nil, errors.New("no profile folder")
	}
	if err := os.Mkdir(dir, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
		return nil, err
	}
	if info, err := os.Lstat(dir); err != nil || !info.IsDir() || info.Mode()&(os.ModeSymlink|os.ModeIrregular) != 0 {
		return nil, fmt.Errorf("%s is not a plain folder", dir)
	}
	path := filepath.Join(dir, "schedule.log")
	if info, err := os.Lstat(path); err == nil {
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("%s is not a plain file", path)
		}
		if info.Size() > maxLogBytes {
			_ = os.Remove(path + ".old")
			if err := os.Rename(path, path+".old"); err != nil {
				return nil, err
			}
		}
	}
	return os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
}
