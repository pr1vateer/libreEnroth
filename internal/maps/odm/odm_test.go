package odm

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"
)

// blob builds a minimal .odm: one 3-vertex model with one face, one decoration, one
// spawn point and 1 terrain normal.
func blob(t *testing.T, faceVerts int, trailing int) []byte {
	t.Helper()
	var b bytes.Buffer
	w := func(v any) { binary.Write(&b, binary.LittleEndian, v) }
	hdr := make([]byte, 0xb4)
	copy(hdr, "blank")
	hdr[0x5f] = 2
	binary.LittleEndian.PutUint16(hdr[0xa0:], 3)
	binary.LittleEndian.PutUint16(hdr[0xac:], 10)
	b.Write(hdr)
	heights := make([]byte, Grid*Grid)
	heights[5*Grid+7] = 3
	b.Write(heights)
	tiles := make([]byte, Grid*Grid)
	tiles[1] = 0x5a + 36 + 2 // tileset 1, tile 2
	tiles[2] = 0xc6 + 4      // tileset 3, tile 4
	b.Write(tiles)
	b.Write(make([]byte, Grid*Grid))
	w(int32(1))
	b.Write(make([]byte, 0x20000))
	b.Write(make([]byte, 0x10000)) // all normal indices 0
	w([3]float32{0, 0, 1})
	w(int32(1)) // models
	mh := make([]byte, 0xbc)
	copy(mh, "house")
	binary.LittleEndian.PutUint32(mh[0x44:], 3)
	binary.LittleEndian.PutUint32(mh[0x4c:], 1)
	binary.LittleEndian.PutUint32(mh[0x5c:], 1)
	binary.LittleEndian.PutUint32(mh[0xb8:], 77)
	b.Write(mh)
	w([3][3]int32{{0, 0, 0}, {100, 0, 0}, {0, 100, 50}})
	f := make([]byte, 0x134)
	binary.LittleEndian.PutUint32(f[8:], 0x10000)
	for k := range 3 {
		binary.LittleEndian.PutUint16(f[0x20+2*k:], uint16(k))
		binary.LittleEndian.PutUint16(f[0x48+2*k:], uint16(10*k))
		binary.LittleEndian.PutUint16(f[0x70+2*k:], uint16(20*k))
	}
	f[0x20+6] = 5 // a 4th vertex id, out of range
	f[0x12e] = byte(faceVerts)
	binary.LittleEndian.PutUint16(f[0x112:], 0xfff0) // -16
	b.Write(f)
	w(int16(0))              // order
	b.Write(make([]byte, 8)) // bsp
	b.Write([]byte("wall1\x00\x00\x00\x00\x00"))
	w(int32(1)) // decorations
	d := make([]byte, 0x20)
	binary.LittleEndian.PutUint16(d[2:], DecVisibleOnMap)
	binary.LittleEndian.PutUint32(d[4:], 1000)
	binary.LittleEndian.PutUint32(d[0x10:], 512)
	b.Write(d)
	dn := make([]byte, 0x20)
	copy(dn, "tree01")
	b.Write(dn)
	w(int32(2))
	w([2]uint16{0, 0})
	b.Write(make([]byte, 4*Grid*Grid))
	w(int32(1))
	s := make([]byte, 0x18)
	binary.LittleEndian.PutUint16(s[0xe:], 3)
	b.Write(s)
	b.Write(make([]byte, trailing))
	return b.Bytes()
}

func TestParse(t *testing.T) {
	m, err := Parse(blob(t, 3, 0))
	if err != nil {
		t.Fatal(err)
	}
	if m.Name != "blank" || m.TileMode != 2 || m.Tilesets[0].Group != 3 || m.Tilesets[3].Group != 10 {
		t.Errorf("header %+v", m.Header)
	}
	if m.Height(7, 5) != 96 || m.Height(-1, 0) != 0 {
		t.Errorf("Height = %d, %d", m.Height(7, 5), m.Height(-1, 0))
	}
	if len(m.Models) != 1 || m.Models[0].Name != "house" || m.Models[0].Radius != 77 {
		t.Fatalf("models %+v", m.Models)
	}
	f := m.Models[0].Faces[0]
	if f.Texture != "wall1" || len(f.Verts) != 3 || f.U[2] != 20 || f.V[2] != 40 || f.TexDU != -16 || f.Normal[2] != 0x10000 {
		t.Errorf("face %+v", f)
	}
	if v := m.Models[0].Vertices[2]; v != (Vec3{0, 100, 50}) {
		t.Errorf("vertex 2 = %v", v)
	}
	if len(m.Decorations) != 1 || m.Decorations[0].Name != "tree01" || m.Decorations[0].Pos.X != 1000 || m.Decorations[0].Yaw != 512 || m.Decorations[0].Flags != DecVisibleOnMap {
		t.Errorf("decorations %+v", m.Decorations)
	}
	if len(m.Spawns) != 1 || m.Spawns[0].Kind != 3 || len(m.FaceIDs) != 2 {
		t.Errorf("spawns %+v face ids %v", m.Spawns, m.FaceIDs)
	}
	if n, ok := m.TriangleNormal(3, 4, 1); !ok || n != [3]float32{0, 0, 1} {
		t.Errorf("TriangleNormal = %v %v", n, ok)
	}
	m.ResolveTilesets(func(g int) int { return 100 * g })
	if got := m.TileIndex(1, 0); got != 2+100*int(m.Tilesets[1].Group) {
		t.Errorf("TileIndex(1,0) = %d", got)
	}
	if got := m.TileIndex(2, 0); got != 4+1000 {
		t.Errorf("TileIndex(2,0) = %d", got)
	}
	if got := m.TileIndex(0, 0); got != 0 {
		t.Errorf("TileIndex(0,0) = %d", got)
	}
}

func TestParseErrors(t *testing.T) {
	for _, c := range []struct {
		name string
		b    []byte
		want string
	}{
		{"truncated", blob(t, 3, 0)[:0x5000], "need"},
		{"trailing", blob(t, 3, 4), "trailing"},
		{"too many face vertices", blob(t, 21, 0), "21 vertices"},
		{"vertex out of range", blob(t, 4, 0), "out of range"},
	} {
		_, err := Parse(c.b)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err = %v, want %q", c.name, err, c.want)
		}
	}
}

func TestCellCorner(t *testing.T) {
	if x, y := CellCorner(64, 64); x != 0 || y != 0 {
		t.Errorf("CellCorner(64,64) = %d,%d", x, y)
	}
	if x, y := CellCorner(0, 0); x != -0x8000 || y != 0x8000 {
		t.Errorf("CellCorner(0,0) = %d,%d", x, y)
	}
}
