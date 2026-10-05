// Package gfx is the software compositor for the 640x480 UI layer: sprites decoded from
// the game's icons and PCX images are blitted into an RGBA canvas, which the engine then
// scales to the window. Pixels never drawn stay transparent, so the 3D view can show
// through (the in-game viewport).
//
// The original draws into a 16-bit (RGB565) DirectDraw surface; colours here stay 8 bits
// per channel. Colours that the game handles as 16-bit values (text colour codes) are
// Color16.
package gfx

import (
	"image"
	"image/color"
)

// Screen size of the original UI.
const (
	ScreenW = 640
	ScreenH = 480
)

// Canvas is the 640x480 UI framebuffer.
type Canvas struct {
	Img  *image.RGBA
	clip image.Rectangle
}

// NewCanvas returns a transparent canvas.
func NewCanvas() *Canvas {
	return &Canvas{Img: image.NewRGBA(image.Rect(0, 0, ScreenW, ScreenH)), clip: image.Rect(0, 0, ScreenW, ScreenH)}
}

// Clear makes the whole canvas transparent and resets the clip rectangle.
func (c *Canvas) Clear() {
	clear(c.Img.Pix)
	c.ResetClip()
}

// SetClip limits drawing to r (intersected with the canvas).
//
// mm8: 0x4a5a90 (Screen_SetClip, which takes inclusive corners)
func (c *Canvas) SetClip(r image.Rectangle) { c.clip = r.Intersect(c.Img.Rect) }

// ResetClip removes the clip rectangle.
//
// mm8: 0x4a5ac5 (Screen_ResetClip)
func (c *Canvas) ResetClip() { c.clip = c.Img.Rect }

// Clip returns the current clip rectangle.
func (c *Canvas) Clip() image.Rectangle { return c.clip }

// Fill paints r with col (clipped).
func (c *Canvas) Fill(r image.Rectangle, col color.RGBA) {
	r = r.Intersect(c.clip)
	for y := r.Min.Y; y < r.Max.Y; y++ {
		row := c.Img.Pix[c.Img.PixOffset(r.Min.X, y):]
		for x := 0; x < r.Dx(); x++ {
			row[4*x], row[4*x+1], row[4*x+2], row[4*x+3] = col.R, col.G, col.B, col.A
		}
	}
}

// Set paints one pixel (clipped).
func (c *Canvas) Set(x, y int, col color.RGBA) {
	if image.Pt(x, y).In(c.clip) {
		c.Img.SetRGBA(x, y, col)
	}
}

// Blit draws every pixel of s with its top-left corner at x, y.
//
// mm8: 0x4a5db8 (Screen_DrawTexture)
func (c *Canvas) Blit(s *Sprite, x, y int) { c.blit(s, x, y, false) }

// BlitKeyed draws s skipping its colour-keyed pixels (palette index 0 of a palettised
// image, whatever colour that index holds).
//
// mm8: 0x4a6272 (Screen_DrawTextureKeyed)
func (c *Canvas) BlitKeyed(s *Sprite, x, y int) { c.blit(s, x, y, true) }

func (c *Canvas) blit(s *Sprite, x, y int, keyed bool) {
	if s == nil {
		return
	}
	dst := image.Rect(x, y, x+s.W, y+s.H).Intersect(c.clip)
	if dst.Empty() {
		return
	}
	keyed = keyed && s.Key != nil
	for dy := dst.Min.Y; dy < dst.Max.Y; dy++ {
		sy := dy - y
		so := sy*s.W + (dst.Min.X - x)
		do := c.Img.PixOffset(dst.Min.X, dy)
		n := dst.Dx()
		if !keyed {
			copy(c.Img.Pix[do:do+4*n], s.Pix[4*so:4*(so+n)])
			continue
		}
		for i := 0; i < n; i++ {
			if !s.Key[so+i] {
				copy(c.Img.Pix[do+4*i:do+4*i+4], s.Pix[4*(so+i):4*(so+i)+4])
			}
		}
	}
}

// Color16 is a colour in the game's 16-bit screen format (RGB565), as used by the text
// colour codes ("\fNNNNN") and the GUI colour fields.
type Color16 uint16

// RGB16 packs r, g, b into a Color16 the way the game does for an RGB565 surface
// (truncating the low bits).
//
// mm8: 0x40f732 (Color16)
func RGB16(r, g, b uint8) Color16 {
	return Color16(uint16(r>>3)<<11 | uint16(g>>2)<<5 | uint16(b>>3))
}

// RGBA expands c to 8 bits per channel (bit replication, so 0xffff becomes white).
func (c Color16) RGBA() color.RGBA {
	r := uint8(c>>11) & 0x1f
	g := uint8(c>>5) & 0x3f
	b := uint8(c) & 0x1f
	return color.RGBA{r<<3 | r>>2, g<<2 | g>>4, b<<3 | b>>2, 0xff}
}

// Mask selects the colour channels a masked blit keeps: the original ANDs each pixel
// with a surface channel mask to draw broken items red and unidentified ones green.
type Mask uint8

const (
	MaskNone  Mask = iota
	MaskRed        // only the red channel (Screen_DrawTextureRed: the surface's red mask)
	MaskGreen      // only the green channel (Screen_DrawTextureGreen)
)

// BlitEx draws s keyed with its top-left corner at x, y, keeping only the channels of
// mask; rotated draws it turned 90° anticlockwise (s.H wide, s.W high: the source's
// right column becomes the top row), as the off hand's swords and daggers are.
//
// mm8: 0x4a6272 (Screen_DrawTextureKeyed), 0x4a684f (Screen_DrawTextureRed),
// 0x4a6ae3 (Screen_DrawTextureGreen); their rotate argument
func (c *Canvas) BlitEx(s *Sprite, x, y int, mask Mask, rotated bool) {
	if s == nil {
		return
	}
	w, h := s.W, s.H
	if rotated {
		w, h = s.H, s.W
	}
	dst := image.Rect(x, y, x+w, y+h).Intersect(c.clip)
	for dy := dst.Min.Y; dy < dst.Max.Y; dy++ {
		for dx := dst.Min.X; dx < dst.Max.X; dx++ {
			sx, sy := dx-x, dy-y
			if rotated {
				sx, sy = s.W-1-(dy-y), dx-x
			}
			i := sy*s.W + sx
			if s.Key != nil && s.Key[i] {
				continue
			}
			p := s.Pix[4*i : 4*i+4]
			o := c.Img.PixOffset(dx, dy)
			r, g, b := p[0], p[1], p[2]
			switch mask {
			case MaskRed:
				g, b = 0, 0
			case MaskGreen:
				r, b = 0, 0
			}
			c.Img.Pix[o], c.Img.Pix[o+1], c.Img.Pix[o+2], c.Img.Pix[o+3] = r, g, b, 0xff
		}
	}
}
