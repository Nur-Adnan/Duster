package sysinfo

type DiskInfo struct {
	Drive string
	Total uint64
	Free  uint64
	Used  uint64
}

type ProcessInfo struct {
	Name   string
	PID    int32
	CPU    float64
	Memory float64
	Status string
}

type SystemStats struct {
	// Hardware identity (for the Duster status header line)
	HostName  string
	CPUModel  string
	OSVersion string

	// CPU metrics
	CPUPercent float64
	CPUCores   []float64

	// Memory metrics
	RAMTotal   uint64
	RAMUsed    uint64
	RAMAvail   uint64
	RAMPercent float64

	// Disk metrics
	Disks        []DiskInfo
	DiskReadSec  uint64
	DiskWriteSec uint64

	// Network metrics
	NetDownSec uint64
	NetUpSec   uint64

	// Power metrics
	BatteryLevel  int
	BatteryStatus string
	BatteryHealth string

	// System health
	UptimeSeconds uint64
	TopProcesses  []ProcessInfo
	HealthScore   int
}
