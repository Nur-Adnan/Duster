//go:build windows

package uninstall

import (
	"golang.org/x/sys/windows/registry"
)

// GetInstalledApps reads standard 32-bit, 64-bit, and HKCU registry directories to aggregate installed programs.
func GetInstalledApps() ([]InstalledApp, error) {
	var allApps []InstalledApp

	// 1. HKLM 64-bit / standard keys
	apps1, _ := getInstalledAppsFromHive(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall`, "HKLM")
	if len(apps1) > 0 {
		allApps = append(allApps, apps1...)
	}

	// 2. HKLM 32-bit WoW64 keys
	apps2, _ := getInstalledAppsFromHive(registry.LOCAL_MACHINE, `SOFTWARE\Wow6432Node\Microsoft\Windows\CurrentVersion\Uninstall`, "HKLM (WoW64)")
	if len(apps2) > 0 {
		allApps = append(allApps, apps2...)
	}

	// 3. HKCU user local keys
	apps3, _ := getInstalledAppsFromHive(registry.CURRENT_USER, `SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall`, "HKCU")
	if len(apps3) > 0 {
		allApps = append(allApps, apps3...)
	}

	// Filter duplicates by name & uninstall string to avoid double listings
	return DeduplicateApps(allApps), nil
}

func getInstalledAppsFromHive(hive registry.Key, path string, hiveName string) ([]InstalledApp, error) {
	var list []InstalledApp
	k, err := registry.OpenKey(hive, path, registry.ENUMERATE_SUB_KEYS|registry.QUERY_VALUE)
	if err != nil {
		return nil, err
	}
	defer k.Close()

	names, err := k.ReadSubKeyNames(-1)
	if err != nil {
		return nil, err
	}

	for _, name := range names {
		subKeyPath := path + "\\" + name
		subKey, err := registry.OpenKey(hive, subKeyPath, registry.QUERY_VALUE)
		if err != nil {
			continue
		}

		displayName, _, errName := subKey.GetStringValue("DisplayName")
		uninstallString, valType, errUninst := subKey.GetStringValue("UninstallString")

		if errName != nil || errUninst != nil || displayName == "" || uninstallString == "" {
			subKey.Close()
			continue
		}

		// REG_EXPAND_SZ values arrive unexpanded (e.g. "%SystemRoot%\...\msiexec.exe");
		// exec.Command on the literal string always fails, so expand them here.
		if valType == registry.EXPAND_SZ {
			if expanded, expErr := registry.ExpandString(uninstallString); expErr == nil {
				uninstallString = expanded
			}
		}

		publisher, _, _ := subKey.GetStringValue("Publisher")
		displayVersion, _, _ := subKey.GetStringValue("DisplayVersion")
		installDate, _, _ := subKey.GetStringValue("InstallDate")

		var estimatedSize int64
		sizeVal, _, errSize := subKey.GetIntegerValue("EstimatedSize")
		if errSize == nil {
			estimatedSize = int64(sizeVal) * 1024 // EstimatedSize is saved in KB in registry
		}

		list = append(list, InstalledApp{
			Name:            displayName,
			RegistryPath:    subKeyPath,
			RegistryHive:    hiveName,
			UninstallString: uninstallString,
			Publisher:       publisher,
			DisplayVersion:  displayVersion,
			InstallDate:     installDate,
			EstimatedSize:   estimatedSize,
		})

		subKey.Close()
	}

	return list, nil
}
