package gfx

import (
	"fmt"
	"image"
	"image/color"

	"libre-enroth/internal/assets/bitmap"
)

// Sprite is an image converted once for blitting: opaque RGBA pixels plus, for
// palettised sources, the colour-key mask.
type Sprite struct {
	W, H int
	Pix  []byte // RGBA, row-major, alpha always 0xff
	Key  []bool // nil for images without a key; true where the source index was 0
}

// FromTexture converts mip level 0 of a palettised bitmap/icon. Index 0 becomes the
// colour key.
func FromTexture(t *bitmap.Texture) (*Sprite, error) {
	if t.Palette == nil {
		return nil, fmt.Errorf("gfx: %q has no palette", t.Name)
	}
	s := &Sprite{W: t.W, H: t.H, Pix: make([]byte, 4*t.W*t.H), Key: make([]bool, t.W*t.H)}
	var lut [256][4]byte
	for i, c := range t.Palette {
		r, g, b, _ := c.RGBA()
		lut[i] = [4]byte{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8), 0xff}
	}
	for i, p := range t.Levels[0] {
		copy(s.Pix[4*i:], lut[p][:])
		s.Key[i] = p == 0
	}
	return s, nil
}

// FromImage converts any image (24-bit PCX, ...) into an unkeyed sprite. A paletted
// image gets index 0 as its key, like a texture.
func FromImage(img image.Image) *Sprite {
	b := img.Bounds()
	s := &Sprite{W: b.Dx(), H: b.Dy(), Pix: make([]byte, 4*b.Dx()*b.Dy())}
	pal, paletted := img.(*image.Paletted)
	if paletted {
		s.Key = make([]bool, s.W*s.H)
	}
	for y := 0; y < s.H; y++ {
		for x := 0; x < s.W; x++ {
			c := color.RGBAModel.Convert(img.At(b.Min.X+x, b.Min.Y+y)).(color.RGBA)
			i := y*s.W + x
			s.Pix[4*i], s.Pix[4*i+1], s.Pix[4*i+2], s.Pix[4*i+3] = c.R, c.G, c.B, 0xff
			if paletted {
				s.Key[i] = pal.ColorIndexAt(b.Min.X+x, b.Min.Y+y) == 0
			}
		}
	}
	return s
}

// Bounds returns the sprite rectangle placed at x, y.
func (s *Sprite) Bounds(x, y int) image.Rectangle { return image.Rect(x, y, x+s.W, y+s.H) }
