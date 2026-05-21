package config

import (
	"os"
	"path/filepath"
	"runtime"
)

// GetConfigDir returns the OS-native application directory for Duster.
func GetConfigDir() string {
	var configDir string
	if runtime.GOOS == "windows" {
		configDir = os.Getenv("LOCALAPPDATA")
		if configDir == "" {
			configDir = os.Getenv("USERPROFILE")
		}
		if configDir != "" {
			configDir = filepath.Join(configDir, "Duster")
		}
	} else {
		home := os.Getenv("HOME")
		if home != "" {
			configDir = filepath.Join(home, ".config", "duster")
		}
	}
	if configDir == "" {
		configDir = filepath.Clean("./")
	}
	return configDir
}
