//go:build !windows

// Non-Windows stubs so the package compiles and unit tests run on dev
// machines and Linux CI. The shipped binary is Windows-only; every stub
// fails gracefully or returns an inert zero value.
package cmd

import (
	"errors"
	"os/exec"
	"time"
)

var errNotWindows = errors.New("only supported on Windows")

type cpuTimes struct {
	user time.Duration
	sys  time.Duration
}

func getProcessCPUTime() (cpuTimes, error) { return cpuTimes{}, errNotWindows }

func getDiskFreeBytesOS(string) int64 { return 0 }

type driverInfo struct {
	Name         string `json:"Name"`
	Version      string `json:"Version"`
	Manufacturer string `json:"Manufacturer"`
	Signed       bool   `json:"Signed"`
	Class        string `json:"Class"`
}

func scanInstalledDrivers() ([]driverInfo, error) { return nil, errNotWindows }

func recyclePathNative(string) error               { return errNotWindows }
func queryRecycleBinNative() (int64, int64, error) { return 0, 0, errNotWindows }
func emptyRecycleBinNative() error                 { return errNotWindows }

func queryPowerShellExecutionPolicy() (string, error) { return "", errNotWindows }
func getWindowsBuildInfo() (string, string, error)    { return "", "", errNotWindows }
func queryLongPathsEnabled() (bool, error)            { return false, errNotWindows }

func verifyRegistrySafety() (bool, string) {
	return false, "registry checks are only supported on Windows"
}

type securityCheckResult struct {
	Name    string
	Details string
	Status  string
}

func runSecurityAudit() ([]securityCheckResult, int, error) { return nil, 0, errNotWindows }

type startupEntry struct {
	Name     string
	Command  string
	Location string
	Enabled  bool
	IsAdmin  bool
}

func getStartupEntries() ([]startupEntry, error) { return nil, errNotWindows }
func toggleStartupApproval(startupEntry) error   { return errNotWindows }
func removeStartupEntry(startupEntry) error      { return errNotWindows }

func setProcessGroup(*exec.Cmd) {}
