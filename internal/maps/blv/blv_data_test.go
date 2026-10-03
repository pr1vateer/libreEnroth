package blv_test

import (
	"strings"
	"testing"

	"libre-enroth/internal/assets/assettest"
	"libre-enroth/internal/assets/lod"
	"libre-enroth/internal/maps/blv"
)

func parse(t *testing.T, games *lod.Archive, name string) *blv.Map {
	t.Helper()
	raw, err := games.Raw(name)
	if err != nil {
		t.Fatal(err)
	}
	b, err := lod.UnpackMap(raw)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	m, err := blv.Parse(b)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return m
}

// TestAllBLV parses every indoor map (Parse checks every cross-reference) and checks
// a few invariants the renderer relies on.
func TestAllBLV(t *testing.T) {
	games, err := lod.Open(assettest.File(t, "games.lod"))
	if err != nil {
		t.Fatal(err)
	}
	defer games.Close()
	n, untextured := 0, 0
	for _, e := range games.Entries {
		if !strings.HasSuffix(strings.ToLower(e.Name), ".blv") {
			continue
		}
		n++
		m := parse(t, games, e.Name)
		for i, f := range m.Faces {
			if f.Attr&(blv.FaceInvisible|blv.FacePortal) == 0 && len(f.Verts) >= 3 && f.Texture == "" {
				untextured++ // 14 in d19; Indoor_DrawFaceHW skips faces without a texture
			}
			if f.Attr&blv.FacePortal != 0 && f.Back == f.Sector {
				t.Errorf("%s: portal face %d leads to its own sector %d", e.Name, i, f.Sector)
			}
		}
		for i, s := range m.Sectors[1:] {
			for _, p := range s.Portals {
				if m.Faces[p].Attr&blv.FacePortal == 0 {
					t.Errorf("%s: sector %d portal list has non-portal face %d", e.Name, i+1, p)
				}
			}
		}
	}
	if n != 47 || untextured != 14 {
		t.Errorf("parsed %d .blv with %d untextured faces, want 47 and 14", n, untextured)
	}
}

func TestD05(t *testing.T) {
	games, err := lod.Open(assettest.File(t, "games.lod"))
	if err != nil {
		t.Fatal(err)
	}
	defer games.Close()
	m := parse(t, games, "d05.blv")
	got := []int{len(m.Vertices), len(m.Faces), len(m.FaceExtras), len(m.Sectors), m.NumDoors,
		len(m.Decorations), len(m.Lights), len(m.Nodes), len(m.Spawns), len(m.Outlines)}
	want := []int{4616, 3917, 1108, 33, 200, 1, 60, 1874, 52, 2994}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("counts = %v, want %v", got, want)
		}
	}
	// Stored integer and float planes agree, and every vertex lies on its face plane.
	for i, f := range m.Faces {
		if len(f.Verts) < 3 {
			continue
		}
		for k := range 3 {
			if d := float64(f.Normal[k])/65536 - float64(f.NormalF[k]); d > 1e-3 || d < -1e-3 {
				t.Fatalf("face %d: normal %v vs %v", i, f.Normal, f.NormalF)
			}
		}
		for _, vi := range f.Verts {
			v := m.Vertices[vi]
			dist := float64(f.NormalF[0])*float64(v.X) + float64(f.NormalF[1])*float64(v.Y) +
				float64(f.NormalF[2])*float64(v.Z) + float64(f.DistF)
			if dist > 2 || dist < -2 {
				t.Fatalf("face %d: vertex %d is %.2f off the plane", i, vi, dist)
			}
		}
	}
}
