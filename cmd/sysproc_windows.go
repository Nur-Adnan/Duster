//go:build windows

package cmd

import (
	"os/exec"
	"syscall"
)

// setProcessGroup detaches the child into its own process group so a Ctrl+C
// in Duster's console is not delivered to it directly; cancellation happens
// explicitly through exec.CommandContext killing the child. Note this does
// NOT auto-terminate children if the parent dies unexpectedly — that would
// require a Job object with JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE.
func setProcessGroup(c *exec.Cmd) {
	if c.SysProcAttr == nil {
		c.SysProcAttr = &syscall.SysProcAttr{}
	}
	c.SysProcAttr.CreationFlags = syscall.CREATE_NEW_PROCESS_GROUP
}

// setRawCmdLine hands line to CreateProcess as-is instead of re-escaping
// c.Args. c.Path still names the executable, so the line cannot redirect it.
func setRawCmdLine(c *exec.Cmd, line string) {
	if c.SysProcAttr == nil {
		c.SysProcAttr = &syscall.SysProcAttr{}
	}
	c.SysProcAttr.CmdLine = line
}
