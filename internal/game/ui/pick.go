package ui

import (
	"image"

	"libre-enroth/internal/gfx"
)

// pickBuffer is the screen's id buffer (Screen +0x40034, g_pickBuffer 0xf018b4): 640 ×
// 480 ids that the inventory, paper doll and chest draws write under their items, and
// clicks read back at the mouse (the item slot, or the chest cell + 1). As in the
// original, a click reads what the last frame drew.
type pickBuffer struct {
	ids [gfx.ScreenW * gfx.ScreenH]int32
}

func (b *pickBuffer) clear() { clear(b.ids[:]) }

// at is the id under x, y (0 outside the screen).
func (b *pickBuffer) at(x, y int) int32 {
	if x < 0 || y < 0 || x >= gfx.ScreenW || y >= gfx.ScreenH {
		return 0
	}
	return b.ids[y*gfx.ScreenW+x]
}

// keyed writes id under the opaque pixels of s at x, y (turned like gfx.BlitEx when
// rotated), within clip.
//
// mm8: 0x4a602e (Screen_PickKeyed), 0x411092 (the chest's)
func (b *pickBuffer) keyed(s *gfx.Sprite, x, y int, id int32, rotated bool, clip image.Rectangle) {
	if s == nil {
		return
	}
	w, h := s.W, s.H
	if rotated {
		w, h = s.H, s.W
	}
	dst := image.Rect(x, y, x+w, y+h).Intersect(clip).Intersect(image.Rect(0, 0, gfx.ScreenW, gfx.ScreenH))
	for dy := dst.Min.Y; dy < dst.Max.Y; dy++ {
		for dx := dst.Min.X; dx < dst.Max.X; dx++ {
			sx, sy := dx-x, dy-y
			if rotated {
				sx, sy = s.W-1-(dy-y), dx-x
			}
			if s.Key != nil && s.Key[sy*s.W+sx] {
				continue
			}
			b.ids[dy*gfx.ScreenW+dx] = id
		}
	}
}

// rect writes id over the whole rectangle of s at x, y, within clip.
//
// mm8: 0x4a5f22 (Screen_PickRect)
func (b *pickBuffer) rect(s *gfx.Sprite, x, y int, id int32, clip image.Rectangle) {
	if s == nil {
		return
	}
	dst := image.Rect(x, y, x+s.W, y+s.H).Intersect(clip).Intersect(image.Rect(0, 0, gfx.ScreenW, gfx.ScreenH))
	for dy := dst.Min.Y; dy < dst.Max.Y; dy++ {
		for dx := dst.Min.X; dx < dst.Max.X; dx++ {
			b.ids[dy*gfx.ScreenW+dx] = id
		}
	}
}
