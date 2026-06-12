//go:build windows

package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

type driverInfo struct {
	Name         string `json:"Name"`
	Version      string `json:"Version"`
	Manufacturer string `json:"Manufacturer"`
	Signed       bool   `json:"Signed"`
	Class        string `json:"Class"`
}

func scanInstalledDrivers() ([]driverInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	script := `Get-CimInstance Win32_PnPSignedDriver | ` +
		`Where-Object { $_.DeviceName -ne $null -and $_.DeviceName.Trim() -ne '' } | ` +
		`Select-Object @{N='Name';E={$_.DeviceName}},` +
		`@{N='Version';E={if($_.DriverVersion){"$($_.DriverVersion)"}else{"N/A"}}},` +
		`@{N='Manufacturer';E={if($_.Manufacturer){"$($_.Manufacturer)"}else{"Unknown"}}},` +
		`@{N='Signed';E={[bool]$_.IsSigned}},` +
		`@{N='Class';E={if($_.DeviceClass){"$($_.DeviceClass)"}else{"Other"}}} | ` +
		`ConvertTo-Json -Compress`

	exe := systemExecutable("WindowsPowerShell\\v1.0\\powershell.exe")
	cmd := exec.CommandContext(ctx, exe, "-NoProfile", "-NonInteractive", "-Command", script)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("driver scan failed: %v", err)
	}

	trimmed := strings.TrimSpace(string(out))
	if trimmed == "" || trimmed == "null" {
		return nil, nil
	}

	var drivers []driverInfo
	if trimmed[0] == '[' {
		if err := json.Unmarshal([]byte(trimmed), &drivers); err != nil {
			return nil, fmt.Errorf("failed to parse driver data: %v", err)
		}
	} else {
		var single driverInfo
		if err := json.Unmarshal([]byte(trimmed), &single); err != nil {
			return nil, fmt.Errorf("failed to parse driver data: %v", err)
		}
		drivers = append(drivers, single)
	}

	return drivers, nil
}
