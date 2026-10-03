package gfx

import (
	"image"
	"image/color"
	"testing"

	"libre-enroth/internal/assets/bitmap"
)

func checker() *Sprite {
	// 2x2 palettised: index 0 (key, but coloured blue), 1 red, 2 green, 0.
	t := &bitmap.Texture{Name: "t", W: 2, H: 2, Levels: [][]byte{{0, 1, 2, 0}},
		Palette: color.Palette{color.RGBA{0, 0, 0xff, 0xff}, color.RGBA{0xff, 0, 0, 0xff}, color.RGBA{0, 0xff, 0, 0xff}}}
	s, err := FromTexture(t)
	if err != nil {
		panic(err)
	}
	return s
}

func TestBlit(t *testing.T) {
	s := checker()
	c := NewCanvas()
	c.Blit(s, 10, 10)
	if got := c.Img.RGBAAt(10, 10); got != (color.RGBA{0, 0, 0xff, 0xff}) {
		t.Errorf("opaque blit draws index 0 in its colour: %v", got)
	}
	c.Clear()
	c.BlitKeyed(s, 10, 10)
	if got := c.Img.RGBAAt(10, 10); got.A != 0 {
		t.Errorf("keyed blit skips index 0: %v", got)
	}
	if got := c.Img.RGBAAt(11, 10); got != (color.RGBA{0xff, 0, 0, 0xff}) {
		t.Errorf("keyed blit pixel 1: %v", got)
	}
	// Clipping, including partially off-canvas.
	c.Clear()
	c.SetClip(image.Rect(0, 0, 11, 11))
	c.Blit(s, 10, 10)
	if c.Img.RGBAAt(11, 10).A != 0 || c.Img.RGBAAt(10, 10).A == 0 {
		t.Error("clip rect not honoured")
	}
	c.ResetClip()
	c.Blit(s, -1, 479)
	if got := c.Img.RGBAAt(0, 479); got != (color.RGBA{0xff, 0, 0, 0xff}) {
		t.Errorf("off-canvas blit: %v", got)
	}
}

func TestColor16(t *testing.T) {
	if RGB16(0xff, 0xff, 0xff) != 0xffff || RGB16(0, 0xff, 0xff) != 0x07ff {
		t.Errorf("RGB16 white %#x cyan %#x", RGB16(0xff, 0xff, 0xff), RGB16(0, 0xff, 0xff))
	}
	if got := Color16(0xffff).RGBA(); got != (color.RGBA{0xff, 0xff, 0xff, 0xff}) {
		t.Errorf("0xffff -> %v", got)
	}
	if got := RGB16(0x70, 0x8f, 0xfe).RGBA(); got != (color.RGBA{0x73, 0x8e, 0xff, 0xff}) {
		t.Errorf("credits blue -> %v", got)
	}
}

func TestFromImagePaletted(t *testing.T) {
	img := image.NewPaletted(image.Rect(0, 0, 2, 1), color.Palette{color.Black, color.White})
	img.Pix[1] = 1
	s := FromImage(img)
	if !s.Key[0] || s.Key[1] || s.Pix[4] != 0xff {
		t.Errorf("FromImage: key %v pix %v", s.Key, s.Pix)
	}
}
