// Package sprite decodes sprites.lod entries (chapter "sprites08"): a 0x20-byte header,
// one 8-byte span per line, then the pixel blob (zlib when unpackedSize != 0). Pixels are
// palette indices into pal%03i of bitmaps.lod; index 0 is transparent.
package sprite

import (
	"encoding/binary"
	"fmt"
	"image"
	"image/color"

	"libre-enroth/internal/assets/lod"
)

const (
	headerSize = 0x20
	lineSize   = 8
)

// Line is the opaque span of one row: pixels Begin..End (inclusive) are stored at
// Pixels[Offset:]. Begin < 0 marks an empty row.
type Line struct {
	Begin, End int16
	Offset     uint32
}

// HeaderSize is the size of the fixed sprite header.
const HeaderSize = headerSize

// Header is the fixed 0x20-byte sprite header (SpriteHeader in re/symbols/types.h).
type Header struct {
	Name         string
	DataSize     int64 // pixel blob size on disk
	W, H         int
	PaletteID    int
	YSkip        int
	UnpackedSize int64 // 0 = raw pixels, else zlib
}

// ParseHeader decodes the fixed sprite header.
func ParseHeader(raw []byte) (Header, error) {
	if len(raw) < headerSize {
		return Header{}, fmt.Errorf("sprite: %d bytes, need %d", len(raw), headerSize)
	}
	le := binary.LittleEndian
	nameEnd := 0
	for nameEnd < 12 && raw[nameEnd] != 0 {
		nameEnd++
	}
	h := Header{
		Name:         string(raw[:nameEnd]),
		DataSize:     int64(int32(le.Uint32(raw[0x0c:]))),
		W:            int(int16(le.Uint16(raw[0x10:]))),
		H:            int(int16(le.Uint16(raw[0x12:]))),
		PaletteID:    int(int16(le.Uint16(raw[0x14:]))),
		YSkip:        int(int16(le.Uint16(raw[0x18:]))),
		UnpackedSize: int64(int32(le.Uint32(raw[0x1c:]))),
	}
	if h.W < 0 || h.H < 0 || h.DataSize < 0 || h.UnpackedSize < 0 {
		return h, fmt.Errorf("sprite %q: bad header (%dx%d, data %d, unpacked %d)", h.Name, h.W, h.H, h.DataSize, h.UnpackedSize)
	}
	return h, nil
}

// Sprite is a decoded sprite.
type Sprite struct {
	Header
	Lines  []Line
	Pixels []byte
}

// Decode parses a raw sprites.lod entry.
//
// mm8: 0x4acbca (Sprite_LoadFromLod)
func Decode(raw []byte) (*Sprite, error) {
	h, err := ParseHeader(raw)
	if err != nil {
		return nil, err
	}
	s := &Sprite{Header: h}
	le := binary.LittleEndian
	linesEnd := int64(headerSize + s.H*lineSize)
	if linesEnd+s.DataSize > int64(len(raw)) {
		return nil, fmt.Errorf("sprite %q: needs %d bytes, have %d", s.Name, linesEnd+s.DataSize, len(raw))
	}
	s.Lines = make([]Line, s.H)
	for y := range s.Lines {
		l := raw[headerSize+y*lineSize:]
		s.Lines[y] = Line{
			Begin:  int16(le.Uint16(l[0:])),
			End:    int16(le.Uint16(l[2:])),
			Offset: le.Uint32(l[4:]),
		}
	}
	s.Pixels = raw[linesEnd : linesEnd+s.DataSize]
	if s.UnpackedSize != 0 {
		p, err := lod.Inflate(s.Pixels, int(s.UnpackedSize))
		if err != nil {
			return nil, fmt.Errorf("sprite %q: %w", s.Name, err)
		}
		s.Pixels = p
	}
	for y, l := range s.Lines {
		if l.Begin < 0 {
			continue
		}
		if l.End < l.Begin || int(l.End) >= s.W ||
			int64(l.Offset)+int64(l.End-l.Begin)+1 > int64(len(s.Pixels)) {
			return nil, fmt.Errorf("sprite %q: line %d span %d..%d@%d out of range", s.Name, y, l.Begin, l.End, l.Offset)
		}
	}
	return s, nil
}

// Load reads and decodes a sprite from sprites.lod.
func Load(sprites *lod.Archive, name string) (*Sprite, error) {
	raw, err := sprites.Raw(name)
	if err != nil {
		return nil, err
	}
	return Decode(raw)
}

// Image renders the sprite with pal. Index 0 (and every pixel outside a span) is
// transparent.
func (s *Sprite) Image(pal color.Palette) *image.Paletted {
	p := make(color.Palette, len(pal))
	copy(p, pal)
	if len(p) > 0 {
		p[0] = color.NRGBA{}
	}
	img := image.NewPaletted(image.Rect(0, 0, s.W, s.H), p)
	for y, l := range s.Lines {
		if l.Begin < 0 {
			continue
		}
		n := int(l.End-l.Begin) + 1
		copy(img.Pix[y*img.Stride+int(l.Begin):][:n], s.Pixels[l.Offset:])
	}
	return img
}
