//go:build windows

package sysinfo

import (
	"math"
	"runtime"
	"sort"
	"sync"
	"syscall"
	"time"
	"unicode/utf16"
	"unsafe"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/net"
	"github.com/shirou/gopsutil/v3/process"
)

// Dynamic API loaders
var (
	kernel32                 = syscall.NewLazyDLL("kernel32.dll")
	procGlobalMemoryStatusEx = kernel32.NewProc("GlobalMemoryStatusEx")
	procGetSystemPowerStatus = kernel32.NewProc("GetSystemPowerStatus")
	procGetTickCount64       = kernel32.NewProc("GetTickCount64")
	procGetDiskFreeSpaceExW  = kernel32.NewProc("GetDiskFreeSpaceExW")
	procGetLogicalDrives     = kernel32.NewProc("GetLogicalDrives")
	procGetComputerNameExW   = kernel32.NewProc("GetComputerNameExW")

	advapi32             = syscall.NewLazyDLL("advapi32.dll")
	procRegOpenKeyExW    = advapi32.NewProc("RegOpenKeyExW")
	procRegQueryValueExW = advapi32.NewProc("RegQueryValueExW")
	procRegCloseKey      = advapi32.NewProc("RegCloseKey")
)

type MEMORYSTATUSEX struct {
	DwLength                uint32
	DwMemoryLoad            uint32
	UllTotalPhys            uint64
	UllAvailPhys            uint64
	UllTotalPageFile        uint64
	UllAvailPageFile        uint64
	UllTotalVirtual         uint64
	UllAvailVirtual         uint64
	UllAvailExtendedVirtual uint64
}

type SYSTEM_POWER_STATUS struct {
	ACLineStatus        byte
	BatteryFlag         byte
	BatteryLifePercent  byte
	SystemStatusFlag    byte
	BatteryLifeTime     uint32
	BatteryFullLifeTime uint32
}

var lastNetDown, lastNetUp uint64
var lastDiskRead, lastDiskWrite uint64
var lastTime time.Time

func init() {
	lastTime = time.Now()
	// Fetch initial values to establish baseline delta rates
	if ioNets, err := net.IOCounters(false); err == nil && len(ioNets) > 0 {
		lastNetDown = ioNets[0].BytesRecv
		lastNetUp = ioNets[0].BytesSent
	}
	if ioDisks, err := disk.IOCounters(); err == nil {
		for _, io := range ioDisks {
			lastDiskRead += io.ReadBytes
			lastDiskWrite += io.WriteBytes
		}
	}
}

// GetSystemStats returns the populated native Windows system metrics.
func GetSystemStats() (SystemStats, error) {
	stats := SystemStats{
		HealthScore: 100,
	}

	// 1. Uptime via native GetTickCount64 (extremely lightweight)
	ret, _, _ := procGetTickCount64.Call()
	stats.UptimeSeconds = uint64(ret) / 1000

	// 2. RAM metrics via native GlobalMemoryStatusEx
	var memInfo MEMORYSTATUSEX
	memInfo.DwLength = uint32(unsafe.Sizeof(memInfo))
	ret, _, _ = procGlobalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&memInfo)))
	if ret != 0 {
		stats.RAMTotal = memInfo.UllTotalPhys
		stats.RAMAvail = memInfo.UllAvailPhys
		stats.RAMUsed = stats.RAMTotal - stats.RAMAvail
		stats.RAMPercent = float64(memInfo.DwMemoryLoad)
	}

	// 3. CPU Usage using gopsutil (PDH-backed implementation)
	if percents, err := cpu.Percent(0, false); err == nil && len(percents) > 0 {
		stats.CPUPercent = percents[0]
	}
	if corePercents, err := cpu.Percent(0, true); err == nil {
		stats.CPUCores = corePercents
	}

	// 4. Power & Battery via native GetSystemPowerStatus
	var powerInfo SYSTEM_POWER_STATUS
	ret, _, _ = procGetSystemPowerStatus.Call(uintptr(unsafe.Pointer(&powerInfo)))
	if ret != 0 {
		stats.BatteryLevel = int(powerInfo.BatteryLifePercent)
		if stats.BatteryLevel == 255 {
			stats.BatteryLevel = 100 // Default fallback when no battery is found (e.g. Desktops)
		}

		switch powerInfo.ACLineStatus {
		case 0:
			stats.BatteryStatus = "Discharging"
		case 1:
			stats.BatteryStatus = "Charging"
		default:
			stats.BatteryStatus = "Plugged In"
		}
		stats.BatteryHealth = "Normal"
	}

	// 5. Native Windows Disk scanning via GetLogicalDrives & GetDiskFreeSpaceExW
	stats.Disks = getWindowsDisks()

	// 6. Network speeds (I/O counters rates)
	now := time.Now()
	deltaSec := now.Sub(lastTime).Seconds()
	if deltaSec <= 0 {
		deltaSec = 1.0
	}
	lastTime = now

	if ioNets, err := net.IOCounters(false); err == nil && len(ioNets) > 0 {
		currentDown := ioNets[0].BytesRecv
		currentUp := ioNets[0].BytesSent

		stats.NetDownSec = uint64(math.Max(0, float64(currentDown-lastNetDown)/deltaSec))
		stats.NetUpSec = uint64(math.Max(0, float64(currentUp-lastNetUp)/deltaSec))

		lastNetDown = currentDown
		lastNetUp = currentUp
	}

	// 7. Disk speeds
	if ioDisks, err := disk.IOCounters(); err == nil {
		var currentRead, currentWrite uint64
		for _, io := range ioDisks {
			currentRead += io.ReadBytes
			currentWrite += io.WriteBytes
		}
		stats.DiskReadSec = uint64(math.Max(0, float64(currentRead-lastDiskRead)/deltaSec))
		stats.DiskWriteSec = uint64(math.Max(0, float64(currentWrite-lastDiskWrite)/deltaSec))

		lastDiskRead = currentRead
		lastDiskWrite = currentWrite
	}

	// 8. Process metrics (Top 5 CPU-hungry)
	stats.TopProcesses = getTopProcesses()

	// 9. Health Score (Weighted: CPU 30%, RAM 30%, Disk 25%, Temp/Uptime 15%)
	cpuPenalty := stats.CPUPercent * 0.3
	ramPenalty := stats.RAMPercent * 0.3

	var totalDiskUsedPercent float64
	if len(stats.Disks) > 0 {
		var sumPercent float64
		for _, d := range stats.Disks {
			if d.Total > 0 {
				sumPercent += (float64(d.Used) / float64(d.Total)) * 100
			}
		}
		totalDiskUsedPercent = sumPercent / float64(len(stats.Disks))
	}
	diskPenalty := totalDiskUsedPercent * 0.25

	stats.HealthScore = 100 - int(cpuPenalty+ramPenalty+diskPenalty)
	if stats.HealthScore < 10 {
		stats.HealthScore = 10 // Floor bounds
	}

	// 10. Hardware identity fields for status header
	stats.HostName = getWindowsHostName()
	stats.CPUModel = getWindowsCPUModel()
	stats.OSVersion = getWindowsOSVersion()

	return stats, nil
}

// getWindowsHostName queries the DNS hostname via kernel32.
func getWindowsHostName() string {
	const ComputerNameDnsHostname = 1
	buf := make([]uint16, 256)
	size := uint32(len(buf))
	ret, _, _ := procGetComputerNameExW.Call(
		uintptr(ComputerNameDnsHostname),
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(unsafe.Pointer(&size)),
	)
	if ret != 0 {
		return syscall.UTF16ToString(buf[:size])
	}
	return "WINDOWS-PC"
}

// registryQueryString reads a REG_SZ value from HKLM via dynamic advapi32.
func registryQueryString(keyPath, valueName string) string {
	const HKEY_LOCAL_MACHINE uintptr = 0x80000002
	const KEY_READ = 0x20019

	keyPtr, err := syscall.UTF16PtrFromString(keyPath)
	if err != nil {
		return ""
	}
	var hKey uintptr
	ret, _, _ := procRegOpenKeyExW.Call(
		HKEY_LOCAL_MACHINE,
		uintptr(unsafe.Pointer(keyPtr)),
		0,
		KEY_READ,
		uintptr(unsafe.Pointer(&hKey)),
	)
	if ret != 0 {
		return ""
	}
	defer procRegCloseKey.Call(hKey)

	valPtr, err := syscall.UTF16PtrFromString(valueName)
	if err != nil {
		return ""
	}
	var dataType uint32
	buf := make([]uint16, 256)
	bufSize := uint32(len(buf) * 2)
	ret, _, _ = procRegQueryValueExW.Call(
		hKey,
		uintptr(unsafe.Pointer(valPtr)),
		0,
		uintptr(unsafe.Pointer(&dataType)),
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(unsafe.Pointer(&bufSize)),
	)
	if ret != 0 {
		return ""
	}
	// REG_SZ = 1
	if dataType != 1 {
		return ""
	}
	nChars := int(bufSize) / 2
	if nChars > 0 && buf[nChars-1] == 0 {
		nChars--
	}
	return string(utf16.Decode(buf[:nChars]))
}

// getWindowsCPUModel reads ProcessorNameString from registry.
func getWindowsCPUModel() string {
	v := registryQueryString(
		`HARDWARE\DESCRIPTION\System\CentralProcessor\0`,
		`ProcessorNameString`,
	)
	if v == "" {
		return "CPU"
	}
	// Trim extra spaces that Intel puts in the name
	result := ""
	prevSpace := false
	for _, ch := range v {
		if ch == ' ' {
			if prevSpace {
				continue
			}
			prevSpace = true
		} else {
			prevSpace = false
		}
		result += string(ch)
	}
	// Shorten for display: take up to first 24 chars after trim
	if len(result) > 24 {
		result = result[:24]
	}
	return result
}

// getWindowsOSVersion reads the ProductName from the CurrentVersion registry key.
func getWindowsOSVersion() string {
	v := registryQueryString(
		`SOFTWARE\Microsoft\Windows NT\CurrentVersion`,
		`ProductName`,
	)
	if v == "" {
		return "Windows"
	}
	// Also try to get DisplayVersion (e.g. "23H2")
	display := registryQueryString(
		`SOFTWARE\Microsoft\Windows NT\CurrentVersion`,
		`DisplayVersion`,
	)
	if display != "" {
		return v + " " + display
	}
	return v
}

func getWindowsDisks() []DiskInfo {
	var disks []DiskInfo

	// Fetch active bitmask representing drive letters (e.g. bit 0 = A:, bit 2 = C:)
	ret, _, _ := procGetLogicalDrives.Call()
	driveMask := uint32(ret)

	for i := 0; i < 26; i++ {
		if (driveMask & (1 << i)) != 0 {
			driveLetter := string(rune('A'+i)) + `:\`

			// We only want typical primary drives (C, D, etc.), let's ignore older floppy boundaries (A, B)
			if i < 2 {
				continue
			}

			// Call native GetDiskFreeSpaceExW
			drivePtr, _ := syscall.UTF16PtrFromString(driveLetter)
			var freeAvail, totalBytes, totalFree uint64

			callRet, _, _ := procGetDiskFreeSpaceExW.Call(
				uintptr(unsafe.Pointer(drivePtr)),
				uintptr(unsafe.Pointer(&freeAvail)),
				uintptr(unsafe.Pointer(&totalBytes)),
				uintptr(unsafe.Pointer(&totalFree)),
			)

			if callRet != 0 && totalBytes > 0 {
				disks = append(disks, DiskInfo{
					Drive: driveLetter,
					Total: totalBytes,
					Free:  freeAvail,
					Used:  totalBytes - freeAvail,
				})
			}
		}
	}

	return disks
}

type procCacheEntry struct {
	TotalTime float64
	Timestamp time.Time
}

var (
	procCacheMutex sync.Mutex
	procCache      = make(map[int32]procCacheEntry)
)

func getTopProcesses() []ProcessInfo {
	var results []ProcessInfo
	procs, err := process.Processes()
	if err != nil {
		return results
	}

	type procVal struct {
		name string
		pid  int32
		cpu  float64
	}
	var vals []procVal
	seenPIDs := make(map[int32]bool)
	numCPUs := float64(runtime.NumCPU())
	if numCPUs <= 0 {
		numCPUs = 1
	}

	now := time.Now()

	for _, p := range procs {
		pid := p.Pid
		seenPIDs[pid] = true

		// Query process CPU times quickly (direct syscall, no sleep)
		times, err := p.Times()
		if err != nil {
			continue
		}
		totalTime := times.User + times.System

		name, err := p.Name()
		if err != nil {
			continue
		}

		procCacheMutex.Lock()
		entry, exists := procCache[pid]
		procCache[pid] = procCacheEntry{
			TotalTime: totalTime,
			Timestamp: now,
		}
		procCacheMutex.Unlock()

		if !exists {
			// First observation: establish baseline, CPU usage is 0 for this frame
			continue
		}

		deltaSec := now.Sub(entry.Timestamp).Seconds()
		if deltaSec <= 0.05 {
			continue // Prevent division by zero or extremely small ticks
		}

		deltaTime := totalTime - entry.TotalTime
		if deltaTime < 0 {
			deltaTime = 0 // Handle counter wraps or anomalies
		}

		// Calculate normalized CPU percentage (normalized to total CPU cores like Task Manager)
		cpuPercent := (deltaTime / deltaSec) * 100.0 / numCPUs
		if cpuPercent > 100.0 {
			cpuPercent = 100.0
		}

		if cpuPercent > 0.01 {
			vals = append(vals, procVal{
				name: name,
				pid:  pid,
				cpu:  cpuPercent,
			})
		}
	}

	// Clean up process cache to prevent leaks of exited PIDs
	procCacheMutex.Lock()
	for pid := range procCache {
		if !seenPIDs[pid] {
			delete(procCache, pid)
		}
	}
	procCacheMutex.Unlock()

	// Sort values by CPU usage descending — O(n·log n) instead of O(n²)
	sort.Slice(vals, func(i, j int) bool {
		return vals[i].cpu > vals[j].cpu
	})

	// Take top 5
	limit := 5
	if len(vals) < limit {
		limit = len(vals)
	}

	for i := 0; i < limit; i++ {
		results = append(results, ProcessInfo{
			Name: vals[i].name,
			PID:  vals[i].pid,
			CPU:  vals[i].cpu,
		})
	}

	return results
}
