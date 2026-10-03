package pcx

import (
	"encoding/binary"
	"image"
	"image/color"
	"testing"
)

func header(w, h, planes, bpl int) []byte {
	b := make([]byte, headerSize)
	b[0], b[1], b[2], b[3] = 0x0a, 5, 1, 8
	binary.LittleEndian.PutUint16(b[8:], uint16(w-1))
	binary.LittleEndian.PutUint16(b[10:], uint16(h-1))
	b[65] = byte(planes)
	binary.LittleEndian.PutUint16(b[66:], uint16(bpl))
	return b
}

func TestDecode24(t *testing.T) {
	// 3x2 image, 4 bytes per line (one pad byte per plane).
	// Row 0: R plane run of 4 x 10, G plane 20 21 22 + pad, B plane literal 0xc1 via run(1).
	// Row 1: a single run of 12 x 7 covering all three planes.
	data := header(3, 2, 3, 4)
	data = append(data,
		0xc4, 10,
		20, 21, 22, 0,
		0xc1, 0xc1, 0xc3, 30,
		0xcc, 7)
	img, err := Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	rgba := img.(*image.NRGBA)
	if rgba.Bounds() != image.Rect(0, 0, 3, 2) {
		t.Fatalf("bounds %v", rgba.Bounds())
	}
	want := []color.NRGBA{{10, 20, 0xc1, 255}, {10, 21, 30, 255}, {10, 22, 30, 255}}
	for x, w := range want {
		if got := rgba.NRGBAAt(x, 0); got != w {
			t.Errorf("(%d,0) = %v, want %v", x, got, w)
		}
	}
	if got := rgba.NRGBAAt(2, 1); got != (color.NRGBA{7, 7, 7, 255}) {
		t.Errorf("(2,1) = %v", got)
	}
}

func TestDecode8(t *testing.T) {
	data := header(2, 1, 1, 2)
	data = append(data, 0xc2, 5)
	pal := make([]byte, 769)
	pal[0] = 0x0c
	pal[1+3*5], pal[2+3*5], pal[3+3*5] = 1, 2, 3
	img, err := Decode(append(data, pal...))
	if err != nil {
		t.Fatal(err)
	}
	p := img.(*image.Paletted)
	if p.ColorIndexAt(1, 0) != 5 || p.At(0, 0) != (color.NRGBA{1, 2, 3, 255}) {
		t.Errorf("pixel %d %v", p.ColorIndexAt(1, 0), p.At(0, 0))
	}
}

func TestDecodeErrors(t *testing.T) {
	if _, err := Decode(header(2, 2, 3, 2)); err == nil {
		t.Error("missing pixel data accepted")
	}
	if _, err := Decode(header(2, 2, 4, 2)); err == nil {
		t.Error("4 planes accepted")
	}
	if _, err := Decode([]byte{1, 2, 3}); err == nil {
		t.Error("short header accepted")
	}
}
