package uninstall

import (
	"testing"
)

func TestDeduplicateApps(t *testing.T) {
	apps := []InstalledApp{
		{
			Name:            "Google Chrome",
			UninstallString: `"C:\Program Files\Google\Chrome\Application\Helper\uninstall.exe" --uninstall`,
			Publisher:       "Google LLC",
		},
		{
			Name:            "Google Chrome",
			UninstallString: `"C:\Program Files\Google\Chrome\Application\Helper\uninstall.exe" --uninstall`,
			Publisher:       "Google LLC",
		},
		{
			Name:            "google chrome", // check case-insensitivity
			UninstallString: `"c:\program files\google\chrome\application\helper\uninstall.exe" --uninstall`,
			Publisher:       "Google LLC",
		},
		{
			Name:            "Slack",
			UninstallString: `C:\Users\User\AppData\Local\slack\Update.exe --uninstall -s`,
			Publisher:       "Slack Technologies",
		},
	}

	deduped := DeduplicateApps(apps)

	if len(deduped) != 2 {
		t.Errorf("Expected 2 deduplicated apps, got %d", len(deduped))
	}

	if deduped[0].Name != "Google Chrome" {
		t.Errorf("Expected first app to be Google Chrome, got %s", deduped[0].Name)
	}

	if deduped[1].Name != "Slack" {
		t.Errorf("Expected second app to be Slack, got %s", deduped[1].Name)
	}
}
