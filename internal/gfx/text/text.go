// Package text draws the game's bitmap fonts into a gfx.Canvas: a port of Font.cpp.
//
// Strings are in the game's 8-bit encoding (Windows-1252 bytes in a Go string), not
// UTF-8. Control codes, as in the original:
//
//	\fNNNNN  ink colour, 5 decimal digits of an RGB565 value; 00000 = default
//	\tNNN    move to x = NNN from the left of the rect
//	\rNNN    right-align the rest of the line, NNN pixels from the right edge
//	\n       new line, height-3 pixels lower
//	""       a literal "
//
// See re/notes/ui.md#fonts-fontcpp-0x44ad1d0x44d1f9.
package text

import (
	"image/color"
	"strconv"
	"strings"

	"libre-enroth/internal/assets/font"
	"libre-enroth/internal/gfx"
)

// Font is a decoded font bound to the palette its uncoloured glyphs are drawn with
// (FONTPAL for every font the game loads).
type Font struct {
	*font.Font
	pal [256]color.RGBA
}

// New binds f to pal.
//
// mm8: 0x44ad1d (Font_Load binds icons palettes to FontHeader.palettes)
func New(f *font.Font, pal color.Palette) *Font {
	t := &Font{Font: f}
	for i := range t.pal {
		if i < len(pal) {
			r, g, b, _ := pal[i].RGBA()
			t.pal[i] = color.RGBA{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8), 0xff}
		}
	}
	return t
}

// Load reads name from the language LODs and binds FONTPAL.
func Load(c *gfx.Cache, name string) (*Font, error) {
	f, err := c.Font(name)
	if err != nil {
		return nil, err
	}
	pal, err := c.FontPalette()
	if err != nil {
		return nil, err
	}
	return New(f, pal), nil
}

// LineHeight is the distance between lines.
func (f *Font) LineHeight() int { return f.Height - 3 }

// Rect is the text box the draw routines take: x, y, width, height. The original struct
// also carries the inclusive right/bottom edges; Right derives them.
type Rect struct{ X, Y, W, H int }

// Right is the inclusive right edge (used by \r).
func (r Rect) Right() int { return r.X + r.W - 1 }

// num parses the n digits after position i (like atoi on a strncpy'd buffer: leading
// digits only).
func num(s string, i, n int) int {
	end := min(i+n, len(s))
	d := s[min(i, len(s)):end]
	j := 0
	for j < len(d) && d[j] >= '0' && d[j] <= '9' {
		j++
	}
	v, _ := strconv.Atoi(d[:j])
	return v
}

// CenterOffset returns the x offset that centres the first line of s in width.
//
// mm8: 0x44addc (Font_CenterOffset)
func (f *Font) CenterOffset(width int, s string) int {
	return max(0, (width-f.TextWidth(s))>>1)
}

// Wrap breaks s into lines no wider than width, starting at x offset xoff, by turning the
// last space (or the start of the line) before an overflowing glyph into '\n'. Text that
// contains \r is returned unchanged unless keepR.
//
// mm8: 0x44b06b (Font_WrapText)
func (f *Font) Wrap(s string, width, xoff int, keepR bool) string {
	b := []byte(s)
	x, lineX, lastBreak := xoff, xoff, 0
	for i := 0; i < len(b); i++ {
		c := b[i]
		if !f.IsPrintable(c) {
			continue
		}
		m := f.Metrics[c]
		switch {
		case c == '\t':
			lineX = xoff + num(s, i+1, 3)
			x = lineX
			i += 3
		case c == '\n':
			x = lineX
			lastBreak = i
		case c == '\f':
			i += 5
		case c == '\r':
			if !keepR {
				return s
			}
		case c == ' ':
			x += int(m.Width)
			lastBreak = i
		case x+int(m.Left+m.Width+m.Right) < width:
			if lastBreak < i {
				x += int(m.Left)
			}
			x += int(m.Width)
			if i < len(b) {
				x += int(m.Right)
			}
		default:
			b[lastBreak] = '\n'
			x = lineX
			for j := lastBreak; j <= i; j++ {
				cj := b[j]
				if !f.IsPrintable(cj) {
					continue
				}
				mj := f.Metrics[cj]
				if j > lastBreak {
					x += int(mj.Left)
				}
				x += int(mj.Width)
				if j < i {
					x += int(mj.Right)
				}
			}
		}
	}
	return string(b)
}

// Draw draws s left-aligned in r at offset (x, y): wrapped to r.W unless maxY != 0, in
// which case it is drawn as is and stops at the first line whose bottom passes maxY.
// ink 0 draws glyphs through the font palette; otherwise glyph value 1 is drawn in shadow
// and the rest in ink.
//
// mm8: 0x44b786 (Font_DrawText)
func (f *Font) Draw(c *gfx.Canvas, r Rect, x, y int, ink, shadow gfx.Color16, s string, maxY int) {
	if s == "" || s == "null" {
		return
	}
	t := s
	if maxY == 0 {
		t = f.Wrap(s, r.W, x, false)
	}
	cx := x + r.X
	cy := r.Y + y
	if maxY != 0 && f.Height+cy > maxY {
		return
	}
	tab := 0
	for i := 0; i < len(t); i++ {
		ch := t[i]
		if !f.IsPrintable(ch) {
			continue
		}
		switch ch {
		case '\t':
			tab = num(t, i+1, 3)
			cx = x + tab + r.X
			i += 3
		case '\n':
			y += f.Height - 3
			cx = x + r.X + tab
			cy = r.Y + y
			if maxY != 0 && (f.Height-3)+cy > maxY {
				return
			}
		case '\f':
			ink = gfx.Color16(num(t, i+1, 5))
			i += 5
		case '\r':
			n := num(t, i+1, 3)
			i += 3
			// The original measures from the last digit of NNN on, so that digit's
			// width is part of the offset.
			cx = r.Right() - f.TextWidth(t[i:]) - n
			cy = r.Y + y
			if maxY != 0 && (f.Height-3)+cy > maxY {
				return
			}
		default:
			if ch == '"' && i+1 < len(t) && t[i+1] == '"' {
				i++
			}
			ch = t[i]
			m := f.Metrics[ch]
			if i > 0 {
				cx += int(m.Left)
			}
			f.glyph(c, cx, cy, ch, ink, shadow)
			cx += int(m.Width)
			if i < len(s) {
				cx += int(m.Right)
			}
		}
	}
}

// DrawCentered wraps s to r.W and draws every line centred, lineGap pixels tighter than
// the font height. y == 0 centres the block vertically in r.
//
// mm8: 0x44bd6a (Font_DrawTextCentered)
func (f *Font) DrawCentered(c *gfx.Canvas, r Rect, x, y int, ink gfx.Color16, s string, lineGap int) {
	t := f.Wrap(s, r.W, x, false)
	if y == 0 {
		free := r.H - f.Height*(strings.Count(t, "\n")+1)
		if free < 1 {
			y = r.Y
		} else {
			y = free/2 + r.Y
		}
	} else {
		y += r.Y
	}
	for _, line := range strings.Split(t, "\n") {
		if line == "" { // strtok skips empty tokens
			continue
		}
		f.DrawLine(c, x+f.CenterOffset(r.W-x, line)+r.X, y, ink, line)
		y += f.Height - lineGap
	}
}

// DrawLine draws one line at x, y. ink 0 means white; the shadow is black.
//
// mm8: 0x44bb6c (Font_DrawLine)
func (f *Font) DrawLine(c *gfx.Canvas, x, y int, ink gfx.Color16, s string) {
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if !f.IsPrintable(ch) || ch == '\t' {
			continue
		}
		switch ch {
		case '\n':
			return
		case '\f':
			ink = gfx.Color16(num(s, i+1, 5))
			i += 5
		case '\r':
		default:
			m := f.Metrics[ch]
			if m.Width == 0 {
				continue
			}
			if i > 0 {
				x += int(m.Left)
			}
			col := ink
			if col == 0 {
				col = 0xffff
			}
			f.glyph(c, x, y, ch, col, 0)
			x += int(m.Width)
			if i < len(s) {
				x += int(m.Right)
			}
		}
	}
}

// glyph draws one character with its top-left corner at x, y.
//
// mm8: 0x4a6dc0 (Screen_DrawGlyphPalette, ink == 0), 0x4a6f56 (Screen_DrawGlyphColor)
func (f *Font) glyph(c *gfx.Canvas, x, y int, ch byte, ink, shadow gfx.Color16) {
	w, pix := f.Glyph(ch)
	inkC, shadowC := ink.RGBA(), shadow.RGBA()
	for gy := 0; gy < f.Height; gy++ {
		for gx := 0; gx < w; gx++ {
			v := pix[gy*w+gx]
			switch {
			case v == font.Transparent:
			case ink == 0:
				c.Set(x+gx, y+gy, f.pal[v])
			case v == font.Shadow:
				c.Set(x+gx, y+gy, shadowC)
			default:
				c.Set(x+gx, y+gy, inkC)
			}
		}
	}
}

// GlyphWithShadowPal draws ch in ink with shadow pixels taken from the font palette, as
// the credits scroller does.
//
// mm8: 0x410fad (scroller glyph blit)
func (f *Font) GlyphWithShadowPal(c *gfx.Canvas, x, y int, ch byte, ink gfx.Color16) {
	w, pix := f.Glyph(ch)
	inkC := ink.RGBA()
	for gy := 0; gy < f.Height; gy++ {
		for gx := 0; gx < w; gx++ {
			switch v := pix[gy*w+gx]; v {
			case font.Transparent:
			case font.Shadow:
				c.Set(x+gx, y+gy, f.pal[1])
			default:
				c.Set(x+gx, y+gy, inkC)
			}
		}
	}
}

// DrawClipped draws s at offset (x, y) in r on one line no wider than maxWidth: text
// narrower than that is drawn as Draw does; longer text is cut before the character
// at which the running width reached maxWidth (colour codes and \r take no width) and
// drawn unwrapped. It returns the width drawn.
//
// mm8: 0x44b450 (Font_DrawTextClipped, without its reversed mode)
func (f *Font) DrawClipped(c *gfx.Canvas, r Rect, x, y int, ink gfx.Color16, s string, maxWidth int) int {
	if w := f.TextWidth(s); w < maxWidth {
		f.Draw(c, r, x, y, ink, 0, s, 0)
		return w
	}
	w, i := 0, 0
	for ; i < len(s) && w < maxWidth; i++ {
		ch := s[i]
		if !f.IsPrintable(ch) {
			continue
		}
		switch {
		case ch == 9 || ch == 10 || ch == '\r':
		case ch == '\f':
			i += 5
		default:
			m := f.Metrics[ch]
			if i > 0 {
				w += int(m.Left)
			}
			w += int(m.Width) + int(m.Right)
		}
	}
	t := s[:max(i-1, 0)]
	f.Draw(c, r, x, y, ink, 0, t, 1<<30)
	return f.TextWidth(t)
}
