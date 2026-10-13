//go:build windows

package api

import (
	"golang.org/x/sys/windows"
	"unsafe"
)

// PROCESS_MEMORY_COUNTERS follows the native pointer width for all SIZE_Ts.
// https://learn.microsoft.com/en-us/windows/win32/api/psapi/ns-psapi-process_memory_counters
func preparationPeakWorkingSet() *uint64 {
	var counters struct {
		Size             uint32
		PageFaults       uint32
		PeakWorkingSet   uintptr
		WorkingSet       uintptr
		PeakPagedPool    uintptr
		PagedPool        uintptr
		PeakNonPagedPool uintptr
		NonPagedPool     uintptr
		Pagefile         uintptr
		PeakPagefile     uintptr
	}
	counters.Size = uint32(unsafe.Sizeof(counters))
	proc := windows.NewLazySystemDLL("psapi.dll").NewProc("GetProcessMemoryInfo")
	if err := proc.Find(); err != nil {
		return nil
	}
	ok, _, _ := proc.Call(uintptr(windows.CurrentProcess()), uintptr(unsafe.Pointer(&counters)), uintptr(counters.Size))
	if ok == 0 {
		return nil
	}
	value := uint64(counters.PeakWorkingSet)
	return &value
}
