//go:build windows

package sysinfo

import (
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

// pdh.dll is not a KnownDLL: load it from System32 only, never from beside du.exe.
var (
	pdh                              = windows.NewLazySystemDLL("pdh.dll")
	procPdhOpenQueryW                = pdh.NewProc("PdhOpenQueryW")
	procPdhAddEnglishCounterW        = pdh.NewProc("PdhAddEnglishCounterW")
	procPdhCollectQueryData          = pdh.NewProc("PdhCollectQueryData")
	procPdhGetFormattedCounterArrayW = pdh.NewProc("PdhGetFormattedCounterArrayW")
	procPdhCloseQuery                = pdh.NewProc("PdhCloseQuery")
)

const (
	pdhFmtDouble = 0x00000200
	pdhMoreData  = 0x800007D2
)

// pdhFmtCounterValueItem mirrors PDH_FMT_COUNTERVALUE_ITEM_W read as a double
// (24 bytes on 64-bit Windows; openEnglishCounter refuses any other layout).
type pdhFmtCounterValueItem struct {
	name        uintptr
	cStatus     uint32
	doubleValue float64
}

var (
	thermalOnce    sync.Once
	thermalMu      sync.Mutex
	thermalQuery   uintptr
	thermalCounter uintptr // 0: no thermal zone counter, report nothing
)

// openEnglishCounter opens a query holding one counter named by its English
// path, so it also works on localized Windows (wildcards only in the instance).
func openEnglishCounter(path string) (query, counter uintptr, ok bool) {
	if unsafe.Sizeof(pdhFmtCounterValueItem{}) != 24 || pdh.Load() != nil {
		return 0, 0, false
	}
	for _, p := range []*windows.LazyProc{procPdhOpenQueryW, procPdhAddEnglishCounterW, procPdhCollectQueryData, procPdhGetFormattedCounterArrayW, procPdhCloseQuery} {
		if p.Find() != nil {
			return 0, 0, false
		}
	}
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, 0, false
	}
	if r, _, _ := procPdhOpenQueryW.Call(0, 0, uintptr(unsafe.Pointer(&query))); r != 0 {
		return 0, 0, false
	}
	if r, _, _ := procPdhAddEnglishCounterW.Call(query, uintptr(unsafe.Pointer(p)), 0, uintptr(unsafe.Pointer(&counter))); r != 0 {
		procPdhCloseQuery.Call(query)
		return 0, 0, false
	}
	return query, counter, true
}

// sampleCounterArray collects one sample and returns the value of every
// instance PDH marks valid, or nil when the sample failed.
func sampleCounterArray(query, counter uintptr) []float64 {
	if r, _, _ := procPdhCollectQueryData.Call(query); r != 0 {
		return nil
	}
	var size, count uint32
	r, _, _ := procPdhGetFormattedCounterArrayW.Call(counter, pdhFmtDouble,
		uintptr(unsafe.Pointer(&size)), uintptr(unsafe.Pointer(&count)), 0)
	if r != pdhMoreData || size == 0 {
		return nil
	}
	// The buffer holds the items followed by their instance names; a slice of
	// items keeps it aligned for reading.
	itemSize := uint32(unsafe.Sizeof(pdhFmtCounterValueItem{}))
	buf := make([]pdhFmtCounterValueItem, (size+itemSize-1)/itemSize)
	r, _, _ = procPdhGetFormattedCounterArrayW.Call(counter, pdhFmtDouble,
		uintptr(unsafe.Pointer(&size)), uintptr(unsafe.Pointer(&count)), uintptr(unsafe.Pointer(&buf[0])))
	if r != 0 || int(count) > len(buf) {
		return nil
	}
	values := make([]float64, 0, count)
	for _, item := range buf[:count] {
		if item.cStatus <= 1 { // PDH_CSTATUS_VALID_DATA or PDH_CSTATUS_NEW_DATA
			values = append(values, item.doubleValue)
		}
	}
	return values
}

// initThermal opens the thermal zone counter once for the life of the process;
// it needs no admin rights. Where the counter set is missing, thermalCounter
// stays 0 and later ticks skip PDH entirely; where it exists with no zones
// (common on VMs), a tick costs one cheap collect that yields nothing.
func initThermal() {
	for _, path := range []string{
		`\Thermal Zone Information(*)\High Precision Temperature`,
		`\Thermal Zone Information(*)\Temperature`, // builds without the high-precision counter
	} {
		if query, counter, ok := openEnglishCounter(path); ok {
			thermalQuery, thermalCounter = query, counter
			return
		}
	}
}

// cpuTemperature returns the hottest thermal zone in °C, or 0 when the machine
// exposes none or this sample failed.
func cpuTemperature() float64 {
	thermalOnce.Do(initThermal)
	if thermalCounter == 0 {
		return 0
	}
	thermalMu.Lock()
	defer thermalMu.Unlock()
	return hottestCelsius(sampleCounterArray(thermalQuery, thermalCounter))
}
