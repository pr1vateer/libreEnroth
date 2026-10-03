package font

import (
	"encoding/binary"
	"testing"
)

// synthFont has glyphs 'A'..'C' with {left, width, right} = {1,3,2}, {0,4,0}, {2,2,1},
// height 2.
func synthFont(t *testing.T) *Font {
	t.Helper()
	raw := make([]byte, offPixels)
	raw[0], raw[1], raw[5] = 'A', 'C', 2
	le := binary.LittleEndian
	metrics := map[byte][3]int32{'A': {1, 3, 2}, 'B': {0, 4, 0}, 'C': {2, 2, 1}}
	off := uint32(0)
	var pix []byte
	for c := byte('A'); c <= 'C'; c++ {
		m := metrics[c]
		for k, v := range m {
			le.PutUint32(raw[offMetrics+12*int(c)+4*k:], uint32(v))
		}
		le.PutUint32(raw[offOffsets+4*int(c):], off)
		for i := 0; i < int(m[1])*2; i++ {
			pix = append(pix, c)
		}
		off += uint32(m[1]) * 2
	}
	f, err := Decode(append(raw, pix...))
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func TestTextWidth(t *testing.T) {
	f := synthFont(t)
	for _, c := range []struct {
		s    string
		want int
	}{
		{"", 0},
		{"A", 3 + 2},                 // no left bearing on the first char, right always
		{"AB", 3 + 2 + 0 + 4 + 0},    // left of B (0) added
		{"CA", 2 + 1 + 1 + 3 + 2},    // left of A (1) added
		{"AxB", 3 + 2 + 4},           // x not printable: skipped, B still gets its left (0)
		{"A\nB", 5},                  // \n stops
		{"A\tB", 5},                  // \t stops
		{"A\rB", 5},                  // \r stops
		{"\f00255A", 1 + 3 + 2},      // colour code skipped; A is not at index 0 -> left
		{"B\f12345C", 4 + 2 + 2 + 1}, // C after colour code
	} {
		if got := f.TextWidth(c.s); got != c.want {
			t.Errorf("TextWidth(%q) = %d, want %d", c.s, got, c.want)
		}
	}
}

func TestGlyphAndAtlas(t *testing.T) {
	f := synthFont(t)
	w, pix := f.Glyph('B')
	if w != 4 || len(pix) != 8 || pix[0] != 'B' {
		t.Errorf("Glyph('B') = %d, %v", w, pix)
	}
	if w, pix := f.Glyph('Z'); w != 0 || pix != nil {
		t.Error("glyph outside range")
	}
	a := f.Atlas()
	if a.Bounds().Dx() != 16*(4+2) || a.Bounds().Dy() != 2+2 {
		t.Errorf("atlas %v", a.Bounds())
	}
}

func TestDecodeRejectsTruncated(t *testing.T) {
	if _, err := Decode(make([]byte, 100)); err == nil {
		t.Error("short font accepted")
	}
	raw := make([]byte, offPixels)
	raw[0], raw[1], raw[5] = 'A', 'A', 10
	binary.LittleEndian.PutUint32(raw[offMetrics+12*'A'+4:], 8)
	if _, err := Decode(raw); err == nil {
		t.Error("glyph beyond pixel data accepted")
	}
}
