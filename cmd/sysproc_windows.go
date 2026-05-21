//go:build windows

package cmd

import (
	"os/exec"
	"syscall"
)

// setProcessGroup configures the process group so that the child terminates when the parent exits.
func setProcessGroup(c *exec.Cmd) {
	if c.SysProcAttr == nil {
		c.SysProcAttr = &syscall.SysProcAttr{}
	}
	c.SysProcAttr.CreationFlags = syscall.CREATE_NEW_PROCESS_GROUP
}
