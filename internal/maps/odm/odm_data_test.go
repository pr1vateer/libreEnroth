package odm_test

import (
	"strings"
	"testing"

	"libre-enroth/internal/assets/assettest"
	"libre-enroth/internal/assets/lod"
	"libre-enroth/internal/maps/odm"
)

func openGames(t *testing.T) *lod.Archive {
	t.Helper()
	a, err := lod.Open(assettest.File(t, "games.lod"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { a.Close() })
	return a
}

// TestAllODM parses every outdoor map and checks that every index is in range.
func TestAllODM(t *testing.T) {
	games := openGames(t)
	n := 0
	for _, e := range games.Entries {
		if !strings.HasSuffix(strings.ToLower(e.Name), ".odm") {
			continue
		}
		n++
		raw, err := games.Raw(e.Name)
		if err != nil {
			t.Fatal(err)
		}
		b, err := lod.UnpackMap(raw)
		if err != nil {
			t.Fatalf("%s: %v", e.Name, err)
		}
		m, err := odm.Parse(b)
		if err != nil {
			t.Fatalf("%s: %v", e.Name, err)
		}
		for i, mod := range m.Models {
			for j, f := range mod.Faces {
				if len(f.Verts) < 3 && f.Attr&odm.FaceInvisible == 0 {
					t.Errorf("%s: model %d face %d: %d vertices", e.Name, i, j, len(f.Verts))
				}
				if f.Texture == "" && f.Attr&odm.FaceInvisible == 0 {
					t.Errorf("%s: model %d face %d: no texture", e.Name, i, j)
				}
			}
		}
		for i, c := range m.CellFaces {
			if int(c) > len(m.FaceIDs) {
				t.Errorf("%s: cell %d face list offset %d > %d", e.Name, i, c, len(m.FaceIDs))
			}
		}
		for i, d := range m.Decorations {
			if d.Name == "" {
				t.Errorf("%s: decoration %d has no name", e.Name, i)
			}
		}
	}
	if n != 14 {
		t.Errorf("parsed %d .odm, want 14", n)
	}
}

func TestOut01(t *testing.T) {
	games := openGames(t)
	raw, err := games.Raw("out01.odm")
	if err != nil {
		t.Fatal(err)
	}
	b, err := lod.UnpackMap(raw)
	if err != nil {
		t.Fatal(err)
	}
	m, err := odm.Parse(b)
	if err != nil {
		t.Fatal(err)
	}
	faces := 0
	for _, mod := range m.Models {
		faces += len(mod.Faces)
	}
	got := []int{len(m.Normals), len(m.Models), faces, len(m.Decorations), len(m.FaceIDs), len(m.Spawns)}
	want := []int{5178, 128, 6740, 800, 26490, 50}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("counts (normals, models, faces, decorations, face ids, spawns) = %v, want %v", got, want)
			break
		}
	}
	if m.TileMode != 0 || m.Tilesets[0].Group != 3 || m.Tilesets[3].Group != 10 {
		t.Errorf("header: mode %d tilesets %v", m.TileMode, m.Tilesets)
	}
	// The stored normals match the triangles they belong to.
	for _, c := range [][2]int{{40, 40}, {64, 70}, {90, 30}} {
		gx, gy := c[0], c[1]
		for k := range 2 {
			n, ok := m.TriangleNormal(gx, gy, k)
			if !ok {
				t.Fatalf("cell %v: no normal %d", c, k)
			}
			p := func(x, y int) [3]float64 {
				wx, wy := odm.CellCorner(x, y)
				return [3]float64{float64(wx), float64(wy), float64(m.Height(x, y))}
			}
			var a, b2, c2 [3]float64
			if k == 1 {
				a, b2, c2 = p(gx, gy+1), p(gx+1, gy+1), p(gx, gy)
			} else {
				a, b2, c2 = p(gx+1, gy), p(gx, gy), p(gx+1, gy+1)
			}
			u := [3]float64{b2[0] - a[0], b2[1] - a[1], b2[2] - a[2]}
			v := [3]float64{c2[0] - a[0], c2[1] - a[1], c2[2] - a[2]}
			cr := [3]float64{u[1]*v[2] - u[2]*v[1], u[2]*v[0] - u[0]*v[2], u[0]*v[1] - u[1]*v[0]}
			if cr[2] < 0 {
				cr = [3]float64{-cr[0], -cr[1], -cr[2]}
			}
			l := cr[0]*cr[0] + cr[1]*cr[1] + cr[2]*cr[2]
			dot := (cr[0]*float64(n[0]) + cr[1]*float64(n[1]) + cr[2]*float64(n[2]))
			if dot*dot < 0.9999*l {
				t.Errorf("cell %v triangle %d: stored normal %v vs computed %v", c, k, n, cr)
			}
		}
	}
}

// TestCollisionFields sweeps every BModel face of the 14 maps for the fields the
// collision code reads: the closing vertex, face and model boxes around the vertices,
// and the zCalc plane of sloped floors and ceilings (modulo 2^16, see below).
//
// mm8: 0x46d6bb (Outdoor_FloorZ), 0x46ead9 (Collide_Models)
func TestCollisionFields(t *testing.T) {
	games := openGames(t)
	n, full, sloped := 0, 0, 0
	for _, e := range games.Entries {
		if !strings.HasSuffix(strings.ToLower(e.Name), ".odm") {
			continue
		}
		n++
		raw, err := games.Raw(e.Name)
		if err != nil {
			t.Fatal(err)
		}
		b, err := lod.UnpackMap(raw)
		if err != nil {
			t.Fatal(err)
		}
		m, err := odm.Parse(b)
		if err != nil {
			t.Fatal(err)
		}
		for mi, mod := range m.Models {
			for fi, f := range mod.Faces {
				nv := len(f.Verts)
				if cap(f.Verts) <= nv || cap(f.XDisp) <= nv || cap(f.YDisp) <= nv || cap(f.ZDisp) <= nv {
					t.Fatalf("%s: model %d face %d: no closing entry", e.Name, mi, fi)
				}
				if nv == odm.MaxFaceVerts {
					full++
				} else if f.Verts[:nv+1][nv] != f.Verts[0] {
					t.Errorf("%s: model %d face %d: closing vertex %d != %d", e.Name, mi, fi, f.Verts[:nv+1][nv], f.Verts[0])
				}
				for _, vi := range f.Verts {
					v := mod.Vertices[vi]
					in := func(c, lo, hi int32) bool { return lo <= c && c <= hi }
					if !in(v.X, int32(f.BBox[0]), int32(f.BBox[1])) || !in(v.Y, int32(f.BBox[2]), int32(f.BBox[3])) ||
						!in(v.Z, int32(f.BBox[4]), int32(f.BBox[5])) {
						t.Errorf("%s: model %d face %d: vertex %v outside the face box %v", e.Name, mi, fi, v, f.BBox)
					}
					if !in(v.X, mod.BBoxMin.X, mod.BBoxMax.X) || !in(v.Y, mod.BBoxMin.Y, mod.BBoxMax.Y) ||
						!in(v.Z, mod.BBoxMin.Z, mod.BBoxMax.Z) {
						t.Errorf("%s: model %d: vertex %v outside the model box %v..%v", e.Name, mi, v, mod.BBoxMin, mod.BBoxMax)
					}
					if f.PolyType == 4 || f.PolyType == 6 {
						z := int32(int64(v.X)*int64(f.ZCalc[0])>>16) + int32(int64(v.Y)*int64(f.ZCalc[1])>>16) + f.ZCalc[2]>>16
						// zCalc[2] = -dist/nz overflows on steep faces in the shipped files, and
						// the game reads the same wrapped value: compare modulo 2^16.
						// Three truncating shifts lose up to 3 units.
						if d := int16(z - v.Z); d < -4 || d > 2 {
							t.Errorf("%s: model %d face %d: zCalc gives %d at vertex %v", e.Name, mi, fi, z, v)
						}
					}
				}
				if f.PolyType == 4 || f.PolyType == 6 {
					sloped++
				}
			}
		}
	}
	if n != 14 {
		t.Errorf("parsed %d .odm, want 14", n)
	}
	t.Logf("%d faces with 20 vertices, %d sloped", full, sloped)
}
