//go:build windows

package platform

import (
	"syscall"
	"unsafe"
)

var (
	user32           = syscall.NewLazyDLL("user32.dll")
	procGetMetrics   = user32.NewProc("GetSystemMetrics")
	procSysParamInfo = user32.NewProc("SystemParametersInfoW")
)

const (
	smCxScreen     = 0
	smCyScreen     = 1
	spiGetWorkArea = 0x0030
)

// screenAndWorkArea reads the primary monitor size and its work area (the part
// not covered by the taskbar). ok=false if either call fails.
func screenAndWorkArea() (screen, work Rect, ok bool) {
	cx, _, _ := procGetMetrics.Call(smCxScreen)
	cy, _, _ := procGetMetrics.Call(smCyScreen)
	var r struct{ L, T, R, B int32 }
	ret, _, _ := procSysParamInfo.Call(spiGetWorkArea, 0, uintptr(unsafe.Pointer(&r)), 0)
	if ret == 0 || cx == 0 || cy == 0 {
		return Rect{}, Rect{}, false
	}
	return Rect{0, 0, int(cx), int(cy)}, Rect{int(r.L), int(r.T), int(r.R), int(r.B)}, true
}
