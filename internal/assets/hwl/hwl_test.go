package hwl

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"image/color"
	"testing"
)

func TestARGB1555(t *testing.T) {
	for _, c := range []struct {
		v    uint16
		want color.NRGBA
	}{
		{0x0000, color.NRGBA{0, 0, 0, 0}},
		{0x8000, color.NRGBA{0, 0, 0, 255}},
		{0xffff, color.NRGBA{255, 255, 255, 255}},
		{0xfc00, color.NRGBA{255, 0, 0, 255}},
		{0x83e0, color.NRGBA{0, 255, 0, 255}},
		{0x001f, color.NRGBA{0, 0, 255, 0}},
		{0x8421, color.NRGBA{8, 8, 8, 255}}, // 1 -> 0b00001000
	} {
		if got := ARGB1555(c.v); got != c.want {
			t.Errorf("ARGB1555(%#04x) = %v, want %v", c.v, got, c.want)
		}
	}
}

func TestFile(t *testing.T) {
	le := binary.LittleEndian
	pix := []uint16{0x8000, 0xffff, 0x7c00, 0x801f}
	raw := make([]byte, 8)
	for i, v := range pix {
		le.PutUint16(raw[2*i:], v)
	}
	var z bytes.Buffer
	w := zlib.NewWriter(&z)
	w.Write(raw)
	w.Close()

	entry := func(packed []byte, stored []byte) []byte {
		b := make([]byte, entryHeaderSize)
		le.PutUint32(b, uint32(len(packed)))
		for k, v := range []uint32{4, 2, 0, 0, 2, 2, 0, 0} {
			le.PutUint32(b[4+4*k:], v)
		}
		return append(b, stored...)
	}
	e0 := entry(nil, raw)             // stored
	e1 := entry(z.Bytes(), z.Bytes()) // zlib
	file := append([]byte("D3DT\x00\x00\x00\x00"), e0...)
	off0 := 8
	off1 := len(file)
	file = append(file, e1...)
	dir := len(file)
	le.PutUint32(file[4:], uint32(dir))
	d := make([]byte, 4+2*nameLen+8)
	le.PutUint32(d, 2)
	copy(d[4:], "Alpha")
	copy(d[4+nameLen:], "beta")
	le.PutUint32(d[4+2*nameLen:], uint32(off0))
	le.PutUint32(d[4+2*nameLen+4:], uint32(off1))
	file = append(file, d...)

	h, err := NewFile(bytes.NewReader(file), "synthetic")
	if err != nil {
		t.Fatal(err)
	}
	if got := h.Names(); len(got) != 2 || got[0] != "Alpha" || got[1] != "beta" {
		t.Fatalf("names %q", got)
	}
	for _, name := range []string{"alpha", "BETA"} {
		tex, err := h.Load(name)
		if err != nil {
			t.Fatal(err)
		}
		if tex.W != 2 || tex.H != 2 || tex.OrigW != 4 || tex.OrigH != 2 {
			t.Errorf("%s: %+v", name, tex)
		}
		for i, v := range pix {
			if tex.Pix[i] != v {
				t.Errorf("%s: pix[%d] = %#x, want %#x", name, i, tex.Pix[i], v)
			}
		}
		if a := tex.NRGBA().NRGBAAt(1, 1); a != (color.NRGBA{0, 0, 255, 255}) {
			t.Errorf("%s: NRGBA(1,1) = %v", name, a)
		}
	}
	if _, err := h.Load("gamma"); err == nil {
		t.Error("missing entry loaded")
	}
}
