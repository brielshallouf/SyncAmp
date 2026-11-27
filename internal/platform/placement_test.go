package platform

import "testing"

func TestPlacePanelFollowsTaskbarEdge(t *testing.T) {
	screen := Rect{0, 0, 1920, 1080}
	const w, h, m = 400, 680, 12
	cases := []struct {
		name     string
		work     Rect
		wantEdge string
		wantX    int
		wantY    int
	}{
		{"taskbar bottom", Rect{0, 0, 1920, 1040}, "bottom", 1920 - w - m, 1040 - h - m},
		{"taskbar top", Rect{0, 40, 1920, 1080}, "top", 1920 - w - m, 40 + m},
		{"taskbar left", Rect{60, 0, 1920, 1080}, "left", 60 + m, 1080 - h - m},
		{"taskbar right", Rect{0, 0, 1860, 1080}, "right", 1860 - w - m, 1080 - h - m},
		{"auto-hide taskbar", screen, "none", 1920 - w - m, 1080 - h - m},
	}
	for _, c := range cases {
		x, y, height, edge := PlacePanel(screen, c.work, w, h, m)
		if edge != c.wantEdge || x != c.wantX || y != c.wantY || height != h {
			t.Errorf("%s: got (%d,%d,h=%d,%s) want (%d,%d,h=%d,%s)", c.name, x, y, height, edge, c.wantX, c.wantY, h, c.wantEdge)
		}
	}
}

func TestPlacePanelNeverLeavesScreen(t *testing.T) {
	// Very short screen (small VM / scaled display): panel is shortened, never pushed off the top.
	screen := Rect{0, 0, 1366, 600}
	work := Rect{0, 0, 1366, 560}
	x, y, h, _ := PlacePanel(screen, work, 400, 680, 12)
	if y < work.T || y+h > work.B || x < work.L || x+400 > work.R {
		t.Errorf("panel (%d,%d %dx%d) outside work area %+v", x, y, 400, h, work)
	}
	if h >= 680 {
		t.Errorf("expected reduced height, got %d", h)
	}
	// Tiny work area: still clamped to the top-left, never negative.
	x, y, _, _ = PlacePanel(Rect{0, 0, 300, 300}, Rect{0, 0, 300, 300}, 400, 680, 12)
	if x < 0 || y < 0 {
		t.Errorf("negative position (%d,%d)", x, y)
	}
}
