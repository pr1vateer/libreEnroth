// Package bitmap decodes 8-bit palettised textures (bitmaps.lod, and the same format in
// icons.lod): a packed-file header, w*h index bytes (plus 3 smaller mip levels when
// flags & 2), then a 0x300-byte RGB palette.
package bitmap

import (
	"fmt"
	"image"
	"image/color"

	"libre-enroth/internal/assets/lod"
	"libre-enroth/internal/assets/palette"
)

// Texture is a decoded bitmap.
type Texture struct {
	Name      string
	W, H      int
	PaletteID int
	Flags     int32
	Levels    [][]byte // mip levels; level i is (W>>i) x (H>>i)
	Palette   color.Palette
}

// Decode builds a Texture from a packed-file header, its inflated payload and the bytes
// that followed it (the palette).
//
// mm8: 0x4113e0 (Texture_Load: flags & 2 -> mips at +w*h, +w*h/4, +w*h/16)
func Decode(hdr lod.FileHeader, data, tail []byte) (*Texture, error) {
	if hdr.W <= 0 || hdr.H <= 0 {
		return nil, fmt.Errorf("bitmap %q: not an image (%dx%d)", hdr.Name, hdr.W, hdr.H)
	}
	t := &Texture{Name: hdr.Name, W: hdr.W, H: hdr.H, PaletteID: hdr.PaletteID, Flags: hdr.Flags}
	n := 1
	if hdr.Flags&lod.FileFlagMips != 0 {
		n = 4
	}
	off := 0
	for i := 0; i < n; i++ {
		size := (t.W >> i) * (t.H >> i)
		if off+size > len(data) {
			return nil, fmt.Errorf("bitmap %q: level %d needs %d bytes, have %d", hdr.Name, i, off+size, len(data))
		}
		t.Levels = append(t.Levels, data[off:off+size])
		off += size
	}
	if len(tail) >= palette.Size {
		p, err := palette.FromRGB(tail)
		if err != nil {
			return nil, err
		}
		t.Palette = p
	}
	return t, nil
}

// Load reads and decodes a bitmap from an archive.
func Load(a *lod.Archive, name string) (*Texture, error) {
	h, data, tail, err := a.ReadPacked(name)
	if err != nil {
		return nil, err
	}
	return Decode(h, data, tail)
}

// Image returns mip level as a paletted image. Without an embedded palette a grey ramp
// is used.
func (t *Texture) Image(level int) *image.Paletted {
	w, h := t.W>>level, t.H>>level
	pal := t.Palette
	if pal == nil {
		pal = Grey()
	}
	img := image.NewPaletted(image.Rect(0, 0, w, h), pal)
	copy(img.Pix, t.Levels[level])
	return img
}

// Grey returns a 256-entry grey ramp, for data without a palette of its own.
func Grey() color.Palette {
	p := make(color.Palette, 256)
	for i := range p {
		p[i] = color.Gray{uint8(i)}
	}
	return p
}
