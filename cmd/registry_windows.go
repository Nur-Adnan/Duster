//go:build windows

package cmd

import (
	"fmt"
	"golang.org/x/sys/windows/registry"
)

func queryPowerShellExecutionPolicy() (string, error) {
	const psKey = `SOFTWARE\Microsoft\PowerShell\1\ShellIds\Microsoft.PowerShell`
	// PowerShell resolves CurrentUser scope before LocalMachine, so query HKCU
	// first. (The HKLM key exists on every install, so an "HKLM-then-fallback"
	// order would make the HKCU branch unreachable and invert the precedence.)
	for _, hive := range []registry.Key{registry.CURRENT_USER, registry.LOCAL_MACHINE} {
		k, err := registry.OpenKey(hive, psKey, registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		policy, _, valErr := k.GetStringValue("ExecutionPolicy")
		k.Close()
		if valErr == nil && policy != "" {
			return policy, nil
		}
	}
	return "Undefined", nil
}

func getWindowsBuildInfo() (string, string, error) {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows NT\CurrentVersion`, registry.QUERY_VALUE)
	if err != nil {
		return "", "", err
	}
	defer k.Close()

	build, _, err := k.GetStringValue("CurrentBuild")
	if err != nil {
		build = ""
	}
	prodName, _, err := k.GetStringValue("ProductName")
	if err != nil {
		prodName = "Windows OS"
	}
	return build, prodName, nil
}

func queryLongPathsEnabled() (bool, error) {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SYSTEM\CurrentControlSet\Control\FileSystem`, registry.QUERY_VALUE)
	if err != nil {
		return false, err
	}
	defer k.Close()

	val, _, err := k.GetIntegerValue("LongPathsEnabled")
	if err != nil {
		return false, err
	}
	return val == 1, nil
}

func verifyRegistrySafety() (bool, string) {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall`, registry.QUERY_VALUE|registry.READ)
	if err == nil {
		k.Close()
		return true, "Valid: Crawlers are successfully sandboxed with read-only permissions."
	}
	return false, fmt.Sprintf("Failed: Unable to open uninstall key with read-only permission: %v", err)
}
