package sprite

import (
	"encoding/binary"
	"image/color"
	"testing"
)

func TestDecodeAndImage(t *testing.T) {
	le := binary.LittleEndian
	// 4x3 sprite: row 0 x=1..2, row 1 empty, row 2 x=0..3. Pixels stored raw.
	pix := []byte{5, 6, 1, 2, 3, 4}
	b := make([]byte, headerSize+3*lineSize)
	copy(b, "test")
	le.PutUint32(b[0x0c:], uint32(len(pix)))
	le.PutUint16(b[0x10:], 4)
	le.PutUint16(b[0x12:], 3)
	le.PutUint16(b[0x14:], 42)
	line := func(y int, begin, end int16, off uint32) {
		l := b[headerSize+y*lineSize:]
		le.PutUint16(l, uint16(begin))
		le.PutUint16(l[2:], uint16(end))
		le.PutUint32(l[4:], off)
	}
	line(0, 1, 2, 0)
	line(1, -1, -1, 0)
	line(2, 0, 3, 2)
	s, err := Decode(append(b, pix...))
	if err != nil {
		t.Fatal(err)
	}
	if s.Name != "test" || s.W != 4 || s.H != 3 || s.PaletteID != 42 {
		t.Errorf("%+v", s.Header)
	}
	pal := make(color.Palette, 256)
	for i := range pal {
		pal[i] = color.NRGBA{uint8(i), 0, 0, 255}
	}
	img := s.Image(pal)
	want := []byte{0, 5, 6, 0, 0, 0, 0, 0, 1, 2, 3, 4}
	for i, w := range want {
		if img.Pix[i] != w {
			t.Fatalf("pix = %v, want %v", img.Pix, want)
		}
	}
	if _, _, _, a := img.At(0, 0).RGBA(); a != 0 {
		t.Error("index 0 not transparent")
	}
	if pal[0] == img.Palette[0] {
		t.Error("caller's palette modified")
	}

	line(2, 0, 3, 4) // span past the pixel blob
	if _, err := Decode(append(b, pix...)); err == nil {
		t.Error("out-of-range span accepted")
	}
}
