//go:build !windows

package cmd

func queryPowerShellExecutionPolicy() (string, error) {
	return "RemoteSigned", nil
}

func getWindowsBuildInfo() (string, string, error) {
	return "22631", "Windows 11 Home", nil
}

func queryLongPathsEnabled() (bool, error) {
	return true, nil
}

func verifyRegistrySafety() (bool, string) {
	return true, "Simulation: Registry operations isolated safely."
}
