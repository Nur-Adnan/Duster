//go:build windows

package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// schtasksCommand runs System32's schtasks.exe (never a PATH lookup).
func schtasksCommand(args ...string) *exec.Cmd {
	return exec.Command(systemExecutable("schtasks.exe"), args...) // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command
}

// registerScheduleTask creates or replaces the task. The XML goes through a
// private temporary file that is deleted afterwards.
func registerScheduleTask(name string, taskXML []byte) error {
	f, err := os.CreateTemp("", "duster-task-*.xml")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	_, werr := f.Write(taskXML)
	if cerr := f.Close(); werr == nil {
		werr = cerr
	}
	if werr != nil {
		return werr
	}
	out, err := schtasksCommand("/Create", "/XML", f.Name(), "/TN", name, "/F").CombinedOutput()
	if err != nil {
		return fmt.Errorf("schtasks could not create the task: %s", strings.TrimSpace(decodeWSLOutput(out)))
	}
	return nil
}

// queryScheduleTask returns the task's XML, or false when it does not exist
// (or cannot be read, which status treats the same way).
func queryScheduleTask(name string) ([]byte, bool) {
	out, err := schtasksCommand("/Query", "/TN", name, "/XML").Output()
	if err != nil {
		return nil, false
	}
	return out, true
}

func deleteScheduleTask(name string) error {
	out, err := schtasksCommand("/Delete", "/TN", name, "/F").CombinedOutput()
	if err != nil {
		return fmt.Errorf("schtasks could not delete %q: %s", name, strings.TrimSpace(decodeWSLOutput(out)))
	}
	return nil
}

// listScheduleTaskNames returns every Duster task this account can see.
func listScheduleTaskNames() ([]string, error) {
	out, err := schtasksCommand("/Query", "/FO", "CSV", "/NH").Output()
	if err != nil {
		return nil, fmt.Errorf("schtasks could not list tasks: %w", err)
	}
	return parseTaskNames(out), nil
}
