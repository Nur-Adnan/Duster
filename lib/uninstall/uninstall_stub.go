//go:build !windows

package uninstall

// GetInstalledApps stub returning simulated data for development and cross-compilation environments.
func GetInstalledApps() ([]InstalledApp, error) {
	return []InstalledApp{
		{
			Name:            "Google Chrome",
			RegistryPath:    `SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\Google Chrome`,
			RegistryHive:    "HKLM",
			UninstallString: `"C:\Program Files\Google\Chrome\Application\Helper\uninstall.exe" --uninstall`,
			Publisher:       "Google LLC",
			DisplayVersion:  "125.0.6422.60",
			InstallDate:     "2026-05-15",
			EstimatedSize:   450 * 1024 * 1024,
		},
		{
			Name:            "Visual Studio Code",
			RegistryPath:    `SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\{9DD24D61-AB2A-4422-B1F7-CCFCD4A1D54B}_is1`,
			RegistryHive:    "HKLM",
			UninstallString: `"C:\Program Files\Microsoft VS Code\unins000.exe"`,
			Publisher:       "Microsoft Corporation",
			DisplayVersion:  "1.89.1",
			InstallDate:     "2026-04-10",
			EstimatedSize:   320 * 1024 * 1024,
		},
		{
			Name:            "Slack",
			RegistryPath:    `SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\Slack`,
			RegistryHive:    "HKCU",
			UninstallString: `C:\Users\User\AppData\Local\slack\Update.exe --uninstall -s`,
			Publisher:       "Slack Technologies",
			DisplayVersion:  "4.38.115",
			InstallDate:     "2026-03-22",
			EstimatedSize:   180 * 1024 * 1024,
		},
		{
			Name:            "Git version 2.45.0",
			RegistryPath:    `SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\Git_is1`,
			RegistryHive:    "HKLM",
			UninstallString: `"C:\Program Files\Git\unins000.exe"`,
			Publisher:       "The Git Development Community",
			DisplayVersion:  "2.45.0",
			InstallDate:     "2026-05-01",
			EstimatedSize:   115 * 1024 * 1024,
		},
		{
			Name:            "Discord",
			RegistryPath:    `SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\Discord`,
			RegistryHive:    "HKCU",
			UninstallString: `C:\Users\User\AppData\Local\Discord\Update.exe --uninstall`,
			Publisher:       "Discord Inc.",
			DisplayVersion:  "1.0.9042",
			InstallDate:     "2026-02-14",
			EstimatedSize:   92 * 1024 * 1024,
		},
	}, nil
}
