// Package font decodes .fnt bitmap fonts (language LODs and icons.lod).
//
// Layout (FontHeader in re/symbols/types.h): first char @0, last char @1, height @5;
// 256 x {left, width, right int32} @0x20; 256 x u32 glyph offsets @0xc20; glyph pixels
// @0x1020, width*height bytes each: 0 transparent, 1 shadow, 255 ink.
package font

import (
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
)

const (
	offMetrics = 0x20
	offOffsets = 0xc20
	offPixels  = 0x1020
)

// Pixel values.
const (
	Transparent = 0
	Shadow      = 1
	Ink         = 255
)

// Metrics are the horizontal metrics of one glyph.
type Metrics struct {
	Left, Width, Right int32
}

// Font is a decoded font.
type Font struct {
	First, Last byte
	Height      int
	Metrics     [256]Metrics
	Offsets     [256]uint32
	Pixels      []byte
}

// Decode parses a .fnt payload (already unpacked from its LOD entry).
//
// mm8: 0x44ad1d (Font_Load)
func Decode(raw []byte) (*Font, error) {
	if len(raw) < offPixels {
		return nil, fmt.Errorf("font: %d bytes, need at least %d", len(raw), offPixels)
	}
	le := binary.LittleEndian
	f := &Font{First: raw[0], Last: raw[1], Height: int(raw[5]), Pixels: raw[offPixels:]}
	for i := 0; i < 256; i++ {
		m := raw[offMetrics+12*i:]
		f.Metrics[i] = Metrics{int32(le.Uint32(m)), int32(le.Uint32(m[4:])), int32(le.Uint32(m[8:]))}
		f.Offsets[i] = le.Uint32(raw[offOffsets+4*i:])
	}
	if f.First > f.Last {
		return nil, fmt.Errorf("font: first %d > last %d", f.First, f.Last)
	}
	for c := int(f.First); c <= int(f.Last); c++ {
		w := f.Metrics[c].Width
		if w < 0 || int64(f.Offsets[c])+int64(w)*int64(f.Height) > int64(len(f.Pixels)) {
			return nil, fmt.Errorf("font: glyph %d (%d px wide @%d) out of range", c, w, f.Offsets[c])
		}
	}
	return f, nil
}

// Glyph returns the width and the row-major width*Height pixels of c, or 0, nil if c is
// outside First..Last.
func (f *Font) Glyph(c byte) (int, []byte) {
	if c < f.First || c > f.Last {
		return 0, nil
	}
	w := int(f.Metrics[c].Width)
	o := int(f.Offsets[c])
	return w, f.Pixels[o : o+w*f.Height]
}

// IsPrintable reports whether c has a glyph or is one of the control characters the text
// code interprets (\t \n \f \r).
//
// mm8: 0x44adb8 (Font_IsPrintable)
func (f *Font) IsPrintable(c byte) bool {
	return (c >= f.First && c <= f.Last) || c == '\f' || c == '\r' || c == '\t' || c == '\n'
}

// TextWidth returns the pixel width of the first line of s. s is in the game's 8-bit
// encoding (Windows-1252), not UTF-8. "\f" starts a 5-digit colour code ("\fNNNNN"),
// \t \n \r end the measurement.
//
// mm8: 0x44adf8 (Font_TextWidth)
func (f *Font) TextWidth(s string) int {
	w := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !f.IsPrintable(c) {
			continue
		}
		switch c {
		case '\t', '\n', '\r':
			return w
		case '\f':
			i += 5
			continue
		}
		m := f.Metrics[c]
		if i > 0 {
			w += int(m.Left)
		}
		w += int(m.Width)
		// The original tests i < strlen here, which always holds inside the loop, so the
		// right bearing of the last character is counted too.
		w += int(m.Right)
	}
	return w
}

// Atlas renders First..Last in a 16-column grid: transparent background, shadow black,
// ink white.
func (f *Font) Atlas() *image.Paletted {
	const cols = 16
	cw := 1
	for c := int(f.First); c <= int(f.Last); c++ {
		cw = max(cw, int(f.Metrics[c].Width))
	}
	cw += 2
	ch := f.Height + 2
	n := int(f.Last) - int(f.First) + 1
	rows := (n + cols - 1) / cols
	pal := make(color.Palette, 256)
	for i := range pal {
		pal[i] = color.NRGBA{uint8(i), uint8(i), uint8(i), 0xff}
	}
	pal[Transparent] = color.NRGBA{}
	pal[Shadow] = color.NRGBA{0, 0, 0, 0xff}
	pal[Ink] = color.NRGBA{0xff, 0xff, 0xff, 0xff}
	img := image.NewPaletted(image.Rect(0, 0, cols*cw, rows*ch), pal)
	for k := 0; k < n; k++ {
		w, pix := f.Glyph(byte(int(f.First) + k))
		x0, y0 := (k%cols)*cw+1, (k/cols)*ch+1
		for y := 0; y < f.Height; y++ {
			copy(img.Pix[(y0+y)*img.Stride+x0:][:w], pix[y*w:])
		}
	}
	return img
}
