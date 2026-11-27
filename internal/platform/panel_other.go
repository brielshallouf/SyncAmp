//go:build !windows

package platform

// ScreenAndWorkArea is only meaningful on Windows. Linux panels/docks vary too much (and Wayland forbids client-side window
// positioning), so the window manager places the popover there.
func ScreenAndWorkArea() (screen, work Rect, ok bool) { return Rect{}, Rect{}, false }

// EnableDPIAwareness is a no-op off Windows.
func EnableDPIAwareness() {}
