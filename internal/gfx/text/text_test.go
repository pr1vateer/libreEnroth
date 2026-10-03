package text

import (
	"image/color"
	"testing"

	"libre-enroth/internal/assets/font"
	"libre-enroth/internal/gfx"
)

// synth builds a font where every printable char is w px wide with 1 px bearings on
// each side and h px tall; glyphs are solid ink with a shadow bottom row.
func synth(w, h int) *Font {
	f := &font.Font{First: 32, Last: 126, Height: h}
	for c := 32; c <= 126; c++ {
		f.Metrics[c] = font.Metrics{Left: 1, Width: int32(w), Right: 1}
		f.Offsets[c] = uint32(len(f.Pixels))
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				v := byte(font.Ink)
				if y == h-1 {
					v = font.Shadow
				}
				f.Pixels = append(f.Pixels, v)
			}
		}
	}
	pal := make(color.Palette, 256)
	for i := range pal {
		pal[i] = color.RGBA{uint8(i), 0, 0, 0xff}
	}
	return New(f, pal)
}

func TestTextWidthAndCenter(t *testing.T) {
	f := synth(4, 6)
	// "ab": a = 4+1, b = 1+4+1 (the first char has no left bearing, the last keeps its right).
	if got := f.TextWidth("ab"); got != 11 {
		t.Errorf("TextWidth(ab) = %d, want 11", got)
	}
	if got := f.TextWidth("ab\ncd"); got != 11 {
		t.Errorf("TextWidth stops at newline: %d", got)
	}
	// After a colour code 'a' is no longer at index 0, so (as in the original) it gets
	// its left bearing too.
	if got := f.TextWidth("\f00000ab"); got != 12 {
		t.Errorf("TextWidth skips colour codes: %d", got)
	}
	if got := f.CenterOffset(31, "ab"); got != 10 {
		t.Errorf("CenterOffset(31, ab) = %d, want 10", got)
	}
	if got := f.CenterOffset(5, "abcdef"); got != 0 {
		t.Errorf("CenterOffset clamps at 0, got %d", got)
	}
}

func TestWrap(t *testing.T) {
	f := synth(4, 6) // each letter advances 6 px (1+4+1), a space 4 px
	for _, c := range []struct {
		in    string
		width int
		want  string
	}{
		{"aaa bbb", 100, "aaa bbb"},
		{"aaa bbb", 30, "aaa\nbbb"},
		{"aaa bbb ccc", 50, "aaa bbb\nccc"},
		{"aaa\nbbb ccc", 30, "aaa\nbbb\nccc"},
		{"\f00255aaa bbb", 30, "\f00255aaa\nbbb"},
		{"aaa \r010bbb", 10, "aaa \r010bbb"}, // \r: unchanged
	} {
		if got := f.Wrap(c.in, c.width, 0, false); got != c.want {
			t.Errorf("Wrap(%q, %d) = %q, want %q", c.in, c.width, got, c.want)
		}
	}
}

func TestDrawColours(t *testing.T) {
	f := synth(2, 3)
	c := gfx.NewCanvas()
	red := gfx.RGB16(0xff, 0, 0)
	f.Draw(c, Rect{X: 10, Y: 10, W: 200, H: 50}, 0, 0, red, 0, "a\f02016b", 0)
	// 'a' at x=10: ink rows 0-1, shadow row 2.
	if got := c.Img.RGBAAt(10, 10); got != red.RGBA() {
		t.Errorf("ink pixel %v", got)
	}
	if got := c.Img.RGBAAt(10, 12); got != (color.RGBA{0, 0, 0, 0xff}) {
		t.Errorf("shadow pixel %v", got)
	}
	// 'b' after the \f code: x = 10 + 2 + 1 (right) + 1 (left) = 14, colour 2016 = pure green.
	if got, want := c.Img.RGBAAt(14, 10), gfx.Color16(2016).RGBA(); got != want || want.G != 0xff {
		t.Errorf("colour-coded pixel %v, want %v", got, want)
	}
	// Uncoloured text maps glyph values through the font palette.
	f.Draw(c, Rect{X: 100, Y: 100, W: 50, H: 10}, 0, 0, 0, 0, "a", 0)
	if got := c.Img.RGBAAt(100, 100); got != (color.RGBA{255, 0, 0, 0xff}) {
		t.Errorf("palette ink %v", got)
	}
	if got := c.Img.RGBAAt(100, 102); got != (color.RGBA{1, 0, 0, 0xff}) {
		t.Errorf("palette shadow %v", got)
	}
	// Transparent stays transparent.
	if got := c.Img.RGBAAt(102, 100); got.A != 0 {
		t.Errorf("gap pixel %v", got)
	}
}

func TestDrawCenteredLines(t *testing.T) {
	f := synth(4, 6)
	c := gfx.NewCanvas()
	// Two lines in a 40-wide box: each centred; second line height-3 = 3 px lower... with
	// lineGap 3 the advance is Height-3.
	f.DrawCentered(c, Rect{X: 0, Y: 0, W: 40, H: 100}, 0, 1, gfx.RGB16(0xff, 0xff, 0xff), "aaa bbbbb", 3)
	// "aaa" is 6+6+5 = 17 wide -> offset 11; "bbbbb" 29 -> offset 5.
	if c.Img.RGBAAt(11, 1).A == 0 || c.Img.RGBAAt(10, 1).A != 0 {
		t.Error("first line not at x=11")
	}
	if c.Img.RGBAAt(5, 4).A == 0 || c.Img.RGBAAt(4, 4).A != 0 {
		t.Error("second line not at x=5, y=4")
	}
}
