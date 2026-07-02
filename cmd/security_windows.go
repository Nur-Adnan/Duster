//go:build windows

package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"golang.org/x/sys/windows/registry"
)

type securityCheckResult struct {
	Name    string
	Details string
	Status  string
}

func runSecurityAudit() ([]securityCheckResult, int, error) {
	var checks []securityCheckResult
	score := 100

	check, penalty := checkDefenderStatus()
	checks = append(checks, check)
	score -= penalty

	check, penalty = checkFirewallStatus()
	checks = append(checks, check)
	score -= penalty

	check, penalty = checkUACStatus()
	checks = append(checks, check)
	score -= penalty

	check, penalty = checkWindowsUpdateStatus()
	checks = append(checks, check)
	score -= penalty

	check, penalty = checkRDPStatus()
	checks = append(checks, check)
	score -= penalty

	if score < 0 {
		score = 0
	}

	return checks, score, nil
}

func checkDefenderStatus() (securityCheckResult, int) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	type mpStatus struct {
		RealTimeEnabled bool `json:"RealTimeProtectionEnabled"`
		AntivirusOn     bool `json:"AntivirusEnabled"`
	}

	script := `Get-MpComputerStatus | Select-Object RealTimeProtectionEnabled,AntivirusEnabled | ConvertTo-Json -Compress`
	exe := systemExecutable("WindowsPowerShell\\v1.0\\powershell.exe")
	cmd := exec.CommandContext(ctx, exe, "-NoProfile", "-NonInteractive", "-Command", script)
	out, err := cmd.Output()

	if err != nil {
		return securityCheckResult{
			Name:    "Windows Defender Antivirus",
			Details: "Could not query Defender status",
			Status:  "warning",
		}, 15
	}

	var status mpStatus
	if err := json.Unmarshal(out, &status); err != nil {
		return securityCheckResult{
			Name:    "Windows Defender Antivirus",
			Details: "Failed to parse Defender status",
			Status:  "warning",
		}, 15
	}

	if status.RealTimeEnabled && status.AntivirusOn {
		return securityCheckResult{
			Name:    "Windows Defender Antivirus",
			Details: "Real-time protection and antivirus are active",
			Status:  "secure",
		}, 0
	}

	var issues []string
	if !status.RealTimeEnabled {
		issues = append(issues, "real-time protection OFF")
	}
	if !status.AntivirusOn {
		issues = append(issues, "antivirus OFF")
	}

	return securityCheckResult{
		Name:    "Windows Defender Antivirus",
		Details: "Disabled: " + strings.Join(issues, ", "),
		Status:  "critical",
	}, 25
}

func checkFirewallStatus() (securityCheckResult, int) {
	// Read profile state from the registry instead of parsing `netsh` output:
	// netsh localizes ON/OFF ("EIN"/"AUS", ...), which made this check report
	// "0 profiles disabled" as critical on any non-English Windows.
	profiles := []struct {
		name string
		key  string
	}{
		{"Domain", `SYSTEM\CurrentControlSet\Services\SharedAccess\Parameters\FirewallPolicy\DomainProfile`},
		{"Private", `SYSTEM\CurrentControlSet\Services\SharedAccess\Parameters\FirewallPolicy\StandardProfile`},
		{"Public", `SYSTEM\CurrentControlSet\Services\SharedAccess\Parameters\FirewallPolicy\PublicProfile`},
	}

	var disabled []string
	queried := 0
	for _, p := range profiles {
		k, err := registry.OpenKey(registry.LOCAL_MACHINE, p.key, registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		val, _, valErr := k.GetIntegerValue("EnableFirewall")
		k.Close()
		if valErr != nil {
			continue
		}
		queried++
		if val == 0 {
			disabled = append(disabled, p.name)
		}
	}

	if queried == 0 {
		return securityCheckResult{
			Name:    "Windows Defender Firewall",
			Details: "Could not query firewall status",
			Status:  "warning",
		}, 10
	}

	if len(disabled) == 0 {
		return securityCheckResult{
			Name:    "Windows Defender Firewall",
			Details: fmt.Sprintf("All %d firewall profiles are active", queried),
			Status:  "secure",
		}, 0
	}

	return securityCheckResult{
		Name:    "Windows Defender Firewall",
		Details: "Firewall disabled for: " + strings.Join(disabled, ", "),
		Status:  "critical",
	}, 20
}

func checkUACStatus() (securityCheckResult, int) {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE,
		`SOFTWARE\Microsoft\Windows\CurrentVersion\Policies\System`, registry.QUERY_VALUE)
	if err != nil {
		return securityCheckResult{
			Name:    "User Account Control (UAC)",
			Details: "Could not query UAC status",
			Status:  "warning",
		}, 5
	}
	defer k.Close()

	val, _, err := k.GetIntegerValue("EnableLUA")
	if err != nil {
		return securityCheckResult{
			Name:    "User Account Control (UAC)",
			Details: "UAC registry value not found",
			Status:  "warning",
		}, 5
	}

	if val == 1 {
		return securityCheckResult{
			Name:    "User Account Control (UAC)",
			Details: "UAC is enabled and protecting system changes",
			Status:  "secure",
		}, 0
	}

	return securityCheckResult{
		Name:    "User Account Control (UAC)",
		Details: "UAC is disabled — system changes are unprotected",
		Status:  "critical",
	}, 25
}

func checkWindowsUpdateStatus() (securityCheckResult, int) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	type hotfix struct {
		InstalledOn string `json:"InstalledOn"`
	}

	script := `Get-HotFix | Sort-Object InstalledOn -Descending -ErrorAction SilentlyContinue | ` +
		`Select-Object -First 1 | ` +
		`Select-Object @{N='InstalledOn';E={$_.InstalledOn.ToString('yyyy-MM-dd')}} | ` +
		`ConvertTo-Json -Compress`

	exe := systemExecutable("WindowsPowerShell\\v1.0\\powershell.exe")
	cmd := exec.CommandContext(ctx, exe, "-NoProfile", "-NonInteractive", "-Command", script)
	out, err := cmd.Output()

	if err != nil {
		return securityCheckResult{
			Name:    "Windows Update Status",
			Details: "Could not query update history",
			Status:  "warning",
		}, 10
	}

	var hf hotfix
	if err := json.Unmarshal(out, &hf); err != nil || hf.InstalledOn == "" {
		return securityCheckResult{
			Name:    "Windows Update Status",
			Details: "No recent updates found in history",
			Status:  "warning",
		}, 15
	}

	lastUpdate, err := time.Parse("2006-01-02", hf.InstalledOn)
	if err != nil {
		return securityCheckResult{
			Name:    "Windows Update Status",
			Details: fmt.Sprintf("Last update: %s", hf.InstalledOn),
			Status:  "warning",
		}, 5
	}

	daysSince := int(time.Since(lastUpdate).Hours() / 24)

	if daysSince <= 30 {
		return securityCheckResult{
			Name:    "Windows Update Status",
			Details: fmt.Sprintf("Last update installed %d days ago (%s)", daysSince, hf.InstalledOn),
			Status:  "secure",
		}, 0
	}

	status := "warning"
	penalty := 10
	if daysSince > 90 {
		status = "critical"
		penalty = 20
	}

	return securityCheckResult{
		Name:    "Windows Update Status",
		Details: fmt.Sprintf("Last update installed %d days ago (%s)", daysSince, hf.InstalledOn),
		Status:  status,
	}, penalty
}

func checkRDPStatus() (securityCheckResult, int) {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE,
		`SYSTEM\CurrentControlSet\Control\Terminal Server`, registry.QUERY_VALUE)
	if err != nil {
		return securityCheckResult{
			Name:    "Remote Desktop (RDP)",
			Details: "Could not query RDP status",
			Status:  "warning",
		}, 5
	}
	defer k.Close()

	val, _, err := k.GetIntegerValue("fDenyTSConnections")
	if err != nil {
		return securityCheckResult{
			Name:    "Remote Desktop (RDP)",
			Details: "RDP configuration not found",
			Status:  "warning",
		}, 5
	}

	if val == 1 {
		return securityCheckResult{
			Name:    "Remote Desktop (RDP)",
			Details: "RDP is disabled (connections are denied)",
			Status:  "secure",
		}, 0
	}

	return securityCheckResult{
		Name:    "Remote Desktop (RDP)",
		Details: "RDP is enabled — remote connections are allowed",
		Status:  "warning",
	}, 10
}
