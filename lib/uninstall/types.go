package uninstall

// InstalledApp represents registry metadata for an installed application.
type InstalledApp struct {
	Name            string `json:"name"`
	RegistryPath    string `json:"registry_path"`
	RegistryHive    string `json:"registry_hive"`
	UninstallString string `json:"uninstall_string"`
	Publisher       string `json:"publisher"`
	DisplayVersion  string `json:"display_version"`
	InstallDate     string `json:"install_date"`
	EstimatedSize   int64  `json:"estimated_size"` // Size in bytes
}
