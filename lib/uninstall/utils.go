package uninstall

import "strings"

// DeduplicateApps filters duplicate apps by name & uninstall string to avoid double listings.
func DeduplicateApps(apps []InstalledApp) []InstalledApp {
	seen := make(map[string]bool)
	var unique []InstalledApp

	for _, app := range apps {
		// Unique key composed of name + uninstall string to avoid duplicates across 32-bit and 64-bit hives
		key := strings.ToLower(app.Name) + "::" + strings.ToLower(app.UninstallString)
		if !seen[key] {
			seen[key] = true
			unique = append(unique, app)
		}
	}

	return unique
}
