//go:build !windows

package cmd

import (
	"os/exec"
)

// setProcessGroup is a stub for non-Windows platforms.
func setProcessGroup(c *exec.Cmd) {
	// No-op on Unix
}
