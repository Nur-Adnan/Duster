//go:build windows

package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows/registry"
)

type startupEntry struct {
	Name     string
	Command  string
	Location string
	Enabled  bool
	IsAdmin  bool
}

func getStartupEntries() ([]startupEntry, error) {
	var entries []startupEntry

	entries = append(entries, readRunKey(registry.CURRENT_USER,
		`SOFTWARE\Microsoft\Windows\CurrentVersion\Run`, "HKCU\\Run", false)...)

	entries = append(entries, readRunKey(registry.LOCAL_MACHINE,
		`SOFTWARE\Microsoft\Windows\CurrentVersion\Run`, "HKLM\\Run", true)...)

	startupDir := os.Getenv("APPDATA")
	if startupDir != "" {
		folder := filepath.Join(startupDir, `Microsoft\Windows\Start Menu\Programs\Startup`)
		if folderEntries, err := readStartupFolder(folder); err == nil {
			entries = append(entries, folderEntries...)
		}
	}

	return entries, nil
}

func readRunKey(hive registry.Key, path, label string, isAdmin bool) []startupEntry {
	var entries []startupEntry
	k, err := registry.OpenKey(hive, path, registry.QUERY_VALUE)
	if err != nil {
		return nil
	}
	defer k.Close()

	names, err := k.ReadValueNames(-1)
	if err != nil {
		return nil
	}

	for _, name := range names {
		val, _, err := k.GetStringValue(name)
		if err != nil {
			continue
		}
		entries = append(entries, startupEntry{
			Name:     name,
			Command:  val,
			Location: label,
			Enabled:  isStartupApproved(hive, name),
			IsAdmin:  isAdmin,
		})
	}
	return entries
}

func isStartupApproved(hive registry.Key, name string) bool {
	return startupApprovedIn(hive, `SOFTWARE\Microsoft\Windows\CurrentVersion\Explorer\StartupApproved\Run`, name)
}

// isStartupFolderApproved reports the Task Manager enable/disable state of a
// Startup-folder item; the value name is the shortcut's file name (with
// extension) under the per-user StartupFolder approval key.
func isStartupFolderApproved(fileName string) bool {
	return startupApprovedIn(registry.CURRENT_USER,
		`SOFTWARE\Microsoft\Windows\CurrentVersion\Explorer\StartupApproved\StartupFolder`, fileName)
}

func startupApprovedIn(hive registry.Key, keyPath, name string) bool {
	k, err := registry.OpenKey(hive, keyPath, registry.QUERY_VALUE)
	if err != nil {
		return true
	}
	defer k.Close()

	val, _, err := k.GetBinaryValue(name)
	if err != nil || len(val) < 1 {
		return true
	}

	return val[0] != 0x03 && val[0] != 0x07
}

func readStartupFolder(dir string) ([]startupEntry, error) {
	var entries []startupEntry
	files, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	for _, f := range files {
		if f.IsDir() {
			continue
		}
		name := f.Name()
		lower := strings.ToLower(name)
		if strings.HasSuffix(lower, ".lnk") || strings.HasSuffix(lower, ".url") {
			clean := strings.TrimSuffix(strings.TrimSuffix(name, ".lnk"), ".url")
			clean = strings.TrimSuffix(strings.TrimSuffix(clean, ".LNK"), ".URL")
			entries = append(entries, startupEntry{
				Name:     clean,
				Command:  filepath.Join(dir, name),
				Location: "Startup Folder",
				Enabled:  isStartupFolderApproved(name),
				IsAdmin:  false,
			})
		}
	}
	return entries, nil
}

func toggleStartupApproval(entry startupEntry) error {
	if entry.Location == "Startup Folder" {
		return fmt.Errorf("startup folder entries cannot be toggled, only removed")
	}

	var hive registry.Key
	if strings.HasPrefix(entry.Location, "HKCU") {
		hive = registry.CURRENT_USER
	} else {
		hive = registry.LOCAL_MACHINE
	}

	k, _, err := registry.CreateKey(hive,
		`SOFTWARE\Microsoft\Windows\CurrentVersion\Explorer\StartupApproved\Run`,
		registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("cannot access startup approval key: %v", err)
	}
	defer k.Close()

	val := make([]byte, 12)
	if entry.Enabled {
		val[0] = 0x03
	} else {
		val[0] = 0x02
	}

	return k.SetBinaryValue(entry.Name, val)
}

func removeStartupEntry(entry startupEntry) error {
	if entry.Location == "Startup Folder" {
		return os.Remove(entry.Command)
	}

	var hive registry.Key
	if strings.HasPrefix(entry.Location, "HKCU") {
		hive = registry.CURRENT_USER
	} else {
		hive = registry.LOCAL_MACHINE
	}

	k, err := registry.OpenKey(hive, `SOFTWARE\Microsoft\Windows\CurrentVersion\Run`, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("cannot open Run key: %v", err)
	}
	defer k.Close()

	if err := k.DeleteValue(entry.Name); err != nil {
		return err
	}

	ak, err := registry.OpenKey(hive,
		`SOFTWARE\Microsoft\Windows\CurrentVersion\Explorer\StartupApproved\Run`,
		registry.SET_VALUE)
	if err == nil {
		defer ak.Close()
		_ = ak.DeleteValue(entry.Name)
	}

	return nil
}
