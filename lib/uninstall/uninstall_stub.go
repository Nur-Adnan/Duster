//go:build !windows

package uninstall

import "errors"

// GetInstalledApps enumerates installed programs from the Windows registry; unsupported elsewhere.
func GetInstalledApps() ([]InstalledApp, error) {
	return nil, errors.New("listing installed apps is only supported on Windows")
}
