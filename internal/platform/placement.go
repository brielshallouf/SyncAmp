package platform

// Rect is a screen rectangle in pixels (Right/Bottom exclusive).
type Rect struct{ L, T, R, B int }

func (r Rect) W() int { return r.R - r.L }
func (r Rect) H() int { return r.B - r.T }

// PlacePanel positions a w×h flyout the way OS tray flyouts behave: in the
// corner of the usable area next to the taskbar. The taskbar edge is inferred
// from how the work area (screen minus taskbar) differs from the full screen,
// so it works for a taskbar docked top, bottom, left or right. With an
// auto-hidden taskbar (work == screen) it defaults to bottom-right.
//
// It returns the top-left position, the (possibly reduced) height so the
// panel never overflows a small screen, and the detected edge.
func PlacePanel(screen, work Rect, w, h, margin int) (x, y, height int, edge string) {
	switch {
	case work.T > screen.T:
		edge = "top"
	case work.L > screen.L:
		edge = "left"
	case work.R < screen.R:
		edge = "right"
	case work.B < screen.B:
		edge = "bottom"
	default:
		edge = "none"
	}
	height = h
	if max := work.H() - 2*margin; height > max && max > 200 {
		height = max
	}
	switch edge {
	case "top":
		x, y = work.R-w-margin, work.T+margin
	case "left":
		x, y = work.L+margin, work.B-height-margin
	default: // bottom, right, none
		x, y = work.R-w-margin, work.B-height-margin
	}
	if x < work.L {
		x = work.L
	}
	if y < work.T {
		y = work.T
	}
	return
}
