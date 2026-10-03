package bitmap

import (
	"bytes"
	"testing"

	"libre-enroth/internal/assets/lod"
)

func TestDecodeMips(t *testing.T) {
	data := make([]byte, 8*4+4*2+2*1+1*0) // 8x4 with mips: 32 + 8 + 2 + 0
	for i := range data {
		data[i] = byte(i)
	}
	pal := bytes.Repeat([]byte{0, 0x80, 0xff}, 256)
	h := lod.FileHeader{Name: "t", W: 8, H: 4, Flags: lod.FileFlagMips}
	tex, err := Decode(h, data, pal)
	if err != nil {
		t.Fatal(err)
	}
	if len(tex.Levels) != 4 || len(tex.Levels[1]) != 8 || tex.Levels[1][0] != 32 || len(tex.Levels[3]) != 0 {
		t.Fatalf("levels %v", tex.Levels)
	}
	img := tex.Image(1)
	if img.Bounds().Dx() != 4 || img.Bounds().Dy() != 2 || img.Pix[0] != 32 {
		t.Errorf("mip1 image %v %v", img.Bounds(), img.Pix)
	}
	if r, g, b, _ := tex.Palette[3].RGBA(); r != 0 || g>>8 != 0x80 || b>>8 != 0xff {
		t.Errorf("palette %v", tex.Palette[3])
	}

	h.Flags = 0
	if tex, err := Decode(h, data, nil); err != nil || len(tex.Levels) != 1 || tex.Palette != nil {
		t.Errorf("no mips: %v", err)
	}
	if _, err := Decode(lod.FileHeader{W: 8, H: 8, Flags: lod.FileFlagMips}, data, nil); err == nil {
		t.Error("short data accepted")
	}
	if _, err := Decode(lod.FileHeader{}, nil, pal); err == nil {
		t.Error("0x0 accepted")
	}
}
