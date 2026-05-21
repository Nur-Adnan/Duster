//go:build !windows

package sysinfo

// GetSystemStats stub returning simulated data for development and cross-compilation environments.
func GetSystemStats() (SystemStats, error) {
	return SystemStats{
		// Hardware identity (stub values for CI/non-Windows)
		HostName:  "DESKTOP-DEV",
		CPUModel:  "Intel Core i7-12700K",
		OSVersion: "Windows 11 23H2",

		// CPU
		CPUPercent: 42.5,
		CPUCores:   []float64{38.2, 45.1, 48.0, 39.4, 52.1, 29.7, 61.3, 34.8},

		// Memory
		RAMTotal:   16 * 1024 * 1024 * 1024,
		RAMUsed:    9 * 1024 * 1024 * 1024,
		RAMAvail:   7 * 1024 * 1024 * 1024,
		RAMPercent: 56.2,

		// Disks
		Disks: []DiskInfo{
			{
				Drive: `C:\`,
				Total: 512 * 1024 * 1024 * 1024,
				Free:  256 * 1024 * 1024 * 1024,
				Used:  256 * 1024 * 1024 * 1024,
			},
			{
				Drive: `D:\`,
				Total: 1024 * 1024 * 1024 * 1024,
				Free:  600 * 1024 * 1024 * 1024,
				Used:  424 * 1024 * 1024 * 1024,
			},
		},
		DiskReadSec:  15 * 1024 * 1024,
		DiskWriteSec: 4 * 1024 * 1024,

		// Network
		NetDownSec: 5 * 1024 * 1024,
		NetUpSec:   200 * 1024,

		// Power
		BatteryLevel:  85,
		BatteryStatus: "Charging",
		BatteryHealth: "Normal",

		// System
		UptimeSeconds: 124500,
		TopProcesses: []ProcessInfo{
			{Name: "dusterd", PID: 1024, CPU: 3.1, Memory: 12.4, Status: "Running"},
			{Name: "rustc", PID: 1088, CPU: 15.2, Memory: 32.1, Status: "Idle"},
			{Name: "code-server", PID: 1140, CPU: 0.8, Memory: 14.5, Status: "Active"},
			{Name: "zsh", PID: 1201, CPU: 0.1, Memory: 0.4, Status: "Idle"},
			{Name: "top", PID: 1312, CPU: 0.4, Memory: 0.2, Status: "Active"},
		},
		HealthScore: 82,
	}, nil
}
