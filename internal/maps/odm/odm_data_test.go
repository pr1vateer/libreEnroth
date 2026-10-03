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
