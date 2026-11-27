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

// ScreenAndWorkArea reads the primary monitor size and its work area (the part
// not covered by the taskbar). ok=false if either call fails.
func ScreenAndWorkArea() (screen, work Rect, ok bool) {
	cx, _, _ := procGetMetrics.Call(smCxScreen)
	cy, _, _ := procGetMetrics.Call(smCyScreen)
	var r struct{ L, T, R, B int32 }
	ret, _, _ := procSysParamInfo.Call(spiGetWorkArea, 0, uintptr(unsafe.Pointer(&r)), 0)
	if ret == 0 || cx == 0 || cy == 0 {
		return Rect{}, Rect{}, false
	}
	return Rect{0, 0, int(cx), int(cy)}, Rect{int(r.L), int(r.T), int(r.R), int(r.B)}, true
}

// EnableDPIAwareness opts the process in to per-monitor DPI scaling. Without it Windows
// bitmap-stretches our window on high-resolution displays (blurry), and the screen
// geometry above would be reported in scaled units. Call once, before creating windows.
func EnableDPIAwareness() {
	p := user32.NewProc("SetProcessDpiAwarenessContext")
	if p.Find() == nil {
		_, _, _ = p.Call(^uintptr(3)) // DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2 (-4)
	}
}

// DPIScale returns the display scale for a window (1.0 = 96 dpi, 1.5 = 144 dpi...).
func DPIScale(hwnd uintptr) float64 {
	p := user32.NewProc("GetDpiForWindow")
	if p.Find() != nil {
		return 1
	}
	if dpi, _, _ := p.Call(hwnd); dpi >= 96 {
		return float64(dpi) / 96
	}
	return 1
}
