//go:build !windows

package platform

// Linux panels/docks vary too much (and Wayland forbids client-side window
// positioning), so the window manager places the popover there.
func screenAndWorkArea() (screen, work Rect, ok bool) { return Rect{}, Rect{}, false }
