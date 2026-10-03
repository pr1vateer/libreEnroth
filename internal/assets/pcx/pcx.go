// Package pcx decodes the ZSoft PCX images stored in icons.lod and the language LODs
// (title.pcx, Loading.pcx, ...). They are wrapped in a packed-file header like any other
// entry.
//
// The game only handles 8 bits x 3 planes (24-bit RGB). A few unused entries (GryLite2,
// GryLite3, fontpal) are 8 bits x 1 plane with a 256-colour trailer; those are decoded
// too so that lodtool can show them.
package pcx

import (
	"encoding/binary"
	"fmt"
	"image"
	"image/color"

	"libre-enroth/internal/assets/lod"
)

const headerSize = 128

// Decode decodes a PCX file. 3-plane images become *image.NRGBA, 1-plane images with a
// palette trailer become *image.Paletted.
//
// mm8: 0x41080b (Pcx_Decode24)
func Decode(b []byte) (image.Image, error) {
	if len(b) < headerSize {
		return nil, fmt.Errorf("pcx: %d bytes, need a %d-byte header", len(b), headerSize)
	}
	le := binary.LittleEndian
	if b[0] != 0x0a || b[2] != 1 {
		return nil, fmt.Errorf("pcx: bad magic/encoding %#x/%d", b[0], b[2])
	}
	bpp := int(b[3])
	xmin, ymin := int(int16(le.Uint16(b[4:]))), int(int16(le.Uint16(b[6:])))
	xmax, ymax := int(int16(le.Uint16(b[8:]))), int(int16(le.Uint16(b[10:])))
	planes := int(b[65])
	bpl := int(le.Uint16(b[66:]))
	w, h := xmax-xmin+1, ymax-ymin+1
	if bpp != 8 {
		return nil, fmt.Errorf("pcx: %d bits per pixel, want 8", bpp)
	}
	if w <= 0 || h <= 0 || bpl < w {
		return nil, fmt.Errorf("pcx: bad geometry %dx%d, %d bytes per line", w, h, bpl)
	}
	var pal color.Palette
	switch planes {
	case 3:
	case 1:
		if len(b) < headerSize+769 || b[len(b)-769] != 0x0c {
			return nil, fmt.Errorf("pcx: 1 plane without a 256-colour palette")
		}
		pal = make(color.Palette, 256)
		p := b[len(b)-768:]
		for i := range pal {
			pal[i] = color.NRGBA{p[3*i], p[3*i+1], p[3*i+2], 0xff}
		}
	default:
		return nil, fmt.Errorf("pcx: %d planes unsupported", planes)
	}

	// Each scanline is planes*bpl bytes (R plane, G plane, B plane), RLE-coded: a byte
	// with the top two bits set is a run count (low 6 bits) for the next byte.
	line := make([]byte, planes*bpl)
	src := b[headerSize:]
	pos := 0
	var out image.Image
	var rgba *image.NRGBA
	var idx *image.Paletted
	if planes == 3 {
		rgba = image.NewNRGBA(image.Rect(0, 0, w, h))
		out = rgba
	} else {
		idx = image.NewPaletted(image.Rect(0, 0, w, h), pal)
		out = idx
	}
	for y := 0; y < h; y++ {
		for n := 0; n < len(line); {
			if pos >= len(src) {
				return nil, fmt.Errorf("pcx: data ends at line %d", y)
			}
			c := src[pos]
			pos++
			run := 1
			if c&0xc0 == 0xc0 {
				run = int(c & 0x3f)
				if pos >= len(src) {
					return nil, fmt.Errorf("pcx: data ends at line %d", y)
				}
				c = src[pos]
				pos++
			}
			for ; run > 0 && n < len(line); run-- {
				line[n] = c
				n++
			}
		}
		if planes == 3 {
			row := rgba.Pix[y*rgba.Stride:]
			for x := 0; x < w; x++ {
				row[4*x+0] = line[x]
				row[4*x+1] = line[bpl+x]
				row[4*x+2] = line[2*bpl+x]
				row[4*x+3] = 0xff
			}
		} else {
			copy(idx.Pix[y*idx.Stride:y*idx.Stride+w], line)
		}
	}
	return out, nil
}

// Load reads a PCX from an archive (icons.lod, or a language LOD).
//
// mm8: 0x410bf8 (Pcx_Load)
func Load(a *lod.Archive, name string) (image.Image, error) {
	_, data, _, err := a.ReadPacked(name)
	if err != nil {
		return nil, err
	}
	img, err := Decode(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	return img, nil
}
