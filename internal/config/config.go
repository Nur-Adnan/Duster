package config

import (
	"os"
	"path/filepath"
	"strings"
)

// GetConfigDir returns the Windows application directory for Duster.
func GetConfigDir() string {
	configDir := os.Getenv("LOCALAPPDATA")
	if configDir == "" {
		configDir = os.Getenv("USERPROFILE")
	}
	if configDir != "" {
		resolved := filepath.Clean(configDir)
		lower := strings.ToLower(resolved)
		if strings.HasPrefix(lower, `c:\windows`) || strings.HasPrefix(lower, `c:\program files`) {
			configDir = ""
		} else {
			configDir = filepath.Join(resolved, "Duster")
		}
	}
	if configDir == "" {
		configDir = filepath.Clean("./")
	}
	return configDir
}
