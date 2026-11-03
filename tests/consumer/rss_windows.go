package main

import (
	"syscall"
	"unsafe"
)

// PROCESS_MEMORY_COUNTERS uses SIZE_T for memory values; WorkingSetSize is bytes.
func rss() int64 {
	var c struct {
		CB, PageFaultCount                 uint32
		PeakWorkingSetSize, WorkingSetSize uintptr
		Other                              [6]uintptr
	}
	c.CB = uint32(unsafe.Sizeof(c))
	p := syscall.NewLazyDLL("psapi.dll").NewProc("GetProcessMemoryInfo")
	ok, _, _ := p.Call(^uintptr(0), uintptr(unsafe.Pointer(&c)), uintptr(c.CB))
	if ok == 0 {
		return -1
	}
	return int64(c.WorkingSetSize)
}
