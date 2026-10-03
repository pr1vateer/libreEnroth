package ui

import (
	"image"
	"strings"

	"libre-enroth/internal/gfx"
)

// Minimap geometry: FUN_0043f7f4(0x1f2, 0x175, 0x27b, 0x1de, zoom, ...).
var minimapRect = image.Rect(498, 373, 635, 478)

// Minimap zoom: 16.16 screen pixels per 128 world units; the in-game buttons double or
// halve it within 0x200..0x800 outdoors (messages 0x60 / 0x61).
const (
	minimapZoomMin     = 0x200
	minimapZoomMax     = 0x800
	minimapZoomDefault = 0x400
)

type minimap struct {
	img    *gfx.Sprite // the map's picture in icons.lod, named after the map
	arrows [8]*gfx.Sprite
	zoom   int
}

// newMinimap loads the outdoor map picture and the party arrows.
//
// mm8: 0x47df28 (Odm_Load: icons.lod texture named after the map), 0x41bbaa (MAPDIR8,
// MAPDIR1..7)
func newMinimap(l *loader, mapName string) *minimap {
	base := mapName
	if i := strings.LastIndexByte(base, '.'); i >= 0 {
		base = base[:i]
	}
	m := &minimap{zoom: minimapZoomDefault}
	m.img, _ = l.r.Cache.Icon(base, false) // indoor maps have none
	for i, n := range []string{"MAPDIR8", "MAPDIR1", "MAPDIR2", "MAPDIR3", "MAPDIR4", "MAPDIR5", "MAPDIR6", "MAPDIR7"} {
		m.arrows[i] = l.icon(n, false)
	}
	return m
}

// draw renders the map around the party: the picture covers the 65536-unit square
// of the map and is nearest-sampled at (W << 16) / zoom texels per screen pixel;
// markers and the arrow go on top.
//
// mm8: 0x43f7f4
func (m *minimap) draw(c *gfx.Canvas, w World) {
	if m.img == nil || w == nil {
		return
	}
	px, py, yaw := w.Party()
	r := minimapRect
	cx, cy := (r.Min.X+r.Max.X)>>1, (r.Min.Y+r.Max.Y)>>1
	img := m.img
	step := (img.W << 16) / m.zoom           // 16.16 texels per pixel
	texel := float64(1<<16) / float64(img.W) // world units per texel
	u0 := (px+0x8000)/texel - float64(r.Dx())/2*float64(step)/65536
	v0 := (0x8000-py)/texel - float64(r.Dy())/2*float64(step)/65536
	ui := int64(u0 * 65536)
	vi := int64(v0 * 65536)
	for y := 0; y < r.Dy(); y++ {
		ty := int((vi + int64(y*step)) >> 16)
		for x := 0; x < r.Dx(); x++ {
			tx := int((ui + int64(x*step)) >> 16)
			o := c.Img.PixOffset(r.Min.X+x, r.Min.Y+y)
			if tx < 0 || ty < 0 || tx >= img.W || ty >= img.H {
				copy(c.Img.Pix[o:o+4], []byte{0, 0, 0, 0xff})
				continue
			}
			copy(c.Img.Pix[o:o+4], img.Pix[4*(ty*img.W+tx):][:4])
		}
	}
	white := gfx.RGB16(0xff, 0xff, 0xff).RGBA()
	w.MapMarkers(func(x, y float64) {
		sx := int((x-px)*float64(m.zoom)/65536) + cx
		sy := cy - int((y-py)*float64(m.zoom)/65536)
		if !image.Pt(sx, sy).In(r) {
			return
		}
		if m.zoom <= 0x200 {
			c.Set(sx, sy, white)
			return
		}
		for dx := -1; dx <= 1; dx++ {
			for dy := -1; dy <= 1; dy++ {
				c.Set(sx+dx, sy+dy, white)
			}
		}
	})
	c.BlitKeyed(m.arrows[arrowIndex(yaw)], cx-3, cy-3)
}

// arrowIndex picks the party arrow for a yaw (0 = east, 512 = north).
//
// mm8: 0x43f7f4 (thresholds 0x80, 0x181, 0x280, 0x381, 0x480, 0x581, 0x680, 0x781)
func arrowIndex(yaw int) int {
	y := yaw & 2047
	for i, t := range []int{0x80, 0x181, 0x280, 0x381, 0x480, 0x581, 0x680, 0x781} {
		if y < t {
			return i
		}
	}
	return 0
}
