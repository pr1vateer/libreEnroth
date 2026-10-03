package blv

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"
)

// blob builds a minimal .blv: a dummy sector 0 and sector 1 with one 4-vertex floor
// face, one light, one BSP node, one decoration and one outline.
func blob(t *testing.T, faceSector int16) []byte {
	t.Helper()
	var b bytes.Buffer
	le := binary.LittleEndian
	w := func(v any) { binary.Write(&b, le, v) }
	hdr := make([]byte, 0x88)
	le.PutUint32(hdr[0x68:], 6*5*2)   // FData: 6 arrays of 4+1
	le.PutUint32(hdr[0x6c:], (1+1)*2) // RData: sector 1 lists one floor and one face
	le.PutUint32(hdr[0x70:], 1*2)     // RLData: one light
	le.PutUint32(hdr[0x74:], 0x1234)  // DData lives in the .dlv
	b.Write(hdr)
	w(int32(4))
	w([4][3]int16{{0, 0, 0}, {512, 0, 0}, {512, 512, 0}, {0, 512, 0}})
	w(int32(1))
	f := make([]byte, 0x60)
	binary.LittleEndian.PutUint32(f[8:], 0x3f800000) // normalF z = 1
	le.PutUint32(f[0x18:], 0x10000)
	le.PutUint32(f[0x2c:], FaceScrollDown)
	le.PutUint16(f[0x48:], 0)
	le.PutUint16(f[0x4c:], uint16(faceSector))
	f[0x5c] = PolyFloor
	f[0x5d] = 4
	b.Write(f)
	for k := range 6 { // verts, x/y/z displacements, u, v
		for _, v := range []int16{0, 1, 2, 3, 0} {
			w(v * int16(k*10+1))
		}
	}
	b.Write([]byte("floor1\x00\x00\x00\x00"))
	w(int32(1))
	x := make([]byte, 0x24)
	le.PutUint16(x[0x14:], 0xfff8) // -8
	le.PutUint16(x[0x1a:], 42)
	b.Write(x)
	b.Write(make([]byte, 10))
	w(int32(2))
	b.Write(make([]byte, 0x78)) // sector 0
	s := make([]byte, 0x78)
	le.PutUint32(s[0:], SectorUsesBSP)
	le.PutUint16(s[0x08:], 1) // floors
	le.PutUint16(s[0x30:], 1) // faces
	le.PutUint16(s[0x32:], 1)
	le.PutUint16(s[0x58:], 1) // lights
	le.PutUint16(s[0x66:], 7)
	le.PutUint16(s[0x6e:], 512)
	b.Write(s)
	w([2]int16{0, 0}) // RData: floors[0], faces[0]
	w(int16(0))       // RLData
	w(int32(200))     // doors
	w(int32(1))       // decorations
	d := make([]byte, 0x20)
	le.PutUint32(d[4:], 100)
	b.Write(d)
	b.Write(append([]byte("torch"), make([]byte, 27)...))
	w(int32(1))
	w([10]int16{10, 20, 30, 256, 0x00ff, 0x0580, 0, 0, 0, 0})
	w(int32(1))
	w([4]int16{-1, -1, 0, 1})
	w(int32(0)) // spawns
	w(int32(1))
	w([6]int16{0, 1, 0, 0, 0, 1})
	return b.Bytes()
}

func TestParse(t *testing.T) {
	m, err := Parse(blob(t, 1))
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Vertices) != 4 || len(m.Faces) != 1 || len(m.Sectors) != 2 || m.NumDoors != 200 || m.DDataSize != 0x1234 {
		t.Fatalf("counts: %d verts %d faces %d sectors %d doors", len(m.Vertices), len(m.Faces), len(m.Sectors), m.NumDoors)
	}
	f := m.Faces[0]
	if f.Texture != "floor1" || f.PolyType != PolyFloor || f.Attr != FaceScrollDown || f.NormalF[2] != 1 || f.Normal[2] != 0x10000 {
		t.Errorf("face: %+v", f)
	}
	if len(f.Verts) != 4 || f.Verts[3] != 3 || f.U[2] != 2*41 || f.V[3] != 3*51 {
		t.Errorf("face arrays: verts %v u %v v %v", f.Verts, f.U, f.V)
	}
	if x := m.FaceExtras[0]; x.TexDU != -8 || x.Event != 42 {
		t.Errorf("extra: %+v", x)
	}
	s := m.Sectors[1]
	if s.Flags != SectorUsesBSP || len(s.Floors) != 1 || len(s.Faces) != 1 || s.NumNonBSPFaces != 1 || len(s.Lights) != 1 || s.MinAmbient != 7 || s.BBox[1] != 512 {
		t.Errorf("sector: %+v", s)
	}
	if l := m.Lights[0]; l.Pos != (Vec3s{10, 20, 30}) || l.Radius != 256 || l.R != 0xff || l.G != 0 || l.B != 0x80 || l.Type != 5 {
		t.Errorf("light: %+v", l)
	}
	if n := m.Nodes[0]; n.Front != -1 || n.NumFaces != 1 {
		t.Errorf("node: %+v", n)
	}
	if d := m.Decorations[0]; d.Name != "torch" || d.Pos[0] != 100 {
		t.Errorf("decoration: %+v", d)
	}
	if o := m.Outlines[0]; o.V2 != 1 || o.Flags != 1 {
		t.Errorf("outline: %+v", o)
	}
}

func TestParseErrors(t *testing.T) {
	good := blob(t, 1)
	for _, c := range []struct {
		name string
		b    []byte
		want string
	}{
		{"truncated", good[:len(good)-3], "exceed"},
		{"trailing", append(append([]byte{}, good...), 0), "trailing"},
		{"bad sector", blob(t, 5), "sector 5 out of range"},
		{"huge count", func() []byte { b := append([]byte{}, good...); b[0x88] = 0xff; b[0x89] = 0xff; return b }(), "vertices count"},
	} {
		_, err := Parse(c.b)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err = %v, want %q", c.name, err, c.want)
		}
	}
}
