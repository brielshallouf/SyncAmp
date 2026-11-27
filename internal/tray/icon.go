// Package tray renders the system-tray UI. Icons are generated at runtime so
// the repo carries no binary assets.
package tray

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"runtime"
)

var palette = map[string]color.RGBA{
	"ok":      {0x2e, 0xa0, 0x43, 255},
	"warn":    {0xd4, 0xa0, 0x17, 255},
	"error":   {0xe5, 0x39, 0x35, 255},
	"running": {0x1e, 0x88, 0xe5, 255},
	"unknown": {0x80, 0x88, 0x90, 255},
}

// iconFor draws a filled, anti-aliased-ish circle in the status colour.
// Windows requires ICO; other platforms take PNG directly. A PNG-in-ICO
// container is valid on Windows Vista and later.
func iconFor(status string) []byte {
	c, ok := palette[status]
	if !ok {
		c = palette["unknown"]
	}
	const n = 32
	img := image.NewRGBA(image.Rect(0, 0, n, n))
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			dx, dy := float64(x)+0.5-n/2, float64(y)+0.5-n/2
			d2 := dx*dx + dy*dy
			switch {
			case d2 <= 13*13:
				img.SetRGBA(x, y, c)
			case d2 <= 14.5*14.5: // soft edge
				cc := c
				cc.A = 128
				img.SetRGBA(x, y, cc)
			}
		}
	}
	var pb bytes.Buffer
	_ = png.Encode(&pb, img)
	if runtime.GOOS != "windows" {
		return pb.Bytes()
	}
	var ico bytes.Buffer
	_ = binary.Write(&ico, binary.LittleEndian, []uint16{0, 1, 1}) // reserved, type=icon, count
	ico.Write([]byte{n, n, 0, 0})                                  // w, h, palette, reserved
	_ = binary.Write(&ico, binary.LittleEndian, []uint16{1, 32})   // planes, bpp
	_ = binary.Write(&ico, binary.LittleEndian, uint32(pb.Len()))
	_ = binary.Write(&ico, binary.LittleEndian, uint32(22)) // data offset
	ico.Write(pb.Bytes())
	return ico.Bytes()
}
