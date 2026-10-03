package delta_test

import (
	"strings"
	"testing"

	"libre-enroth/internal/assets/assettest"
	"libre-enroth/internal/assets/lod"
	"libre-enroth/internal/maps/blv"
	"libre-enroth/internal/maps/delta"
	"libre-enroth/internal/maps/odm"
)

func unpack(t *testing.T, a *lod.Archive, name string) []byte {
	t.Helper()
	raw, err := a.Raw(name)
	if err != nil {
		t.Fatal(err)
	}
	b, err := lod.UnpackMap(raw)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return b
}

// TestTemplates parses the .dlv/.ddm template of every map in games.lod against its map.
func TestTemplates(t *testing.T) {
	games, err := lod.Open(assettest.File(t, "games.lod"))
	if err != nil {
		t.Fatal(err)
	}
	defer games.Close()
	var nd, nm int
	for _, e := range games.Entries {
		name := strings.ToLower(e.Name)
		base := strings.TrimSuffix(strings.TrimSuffix(name, ".blv"), ".odm")
		switch {
		case strings.HasSuffix(name, ".blv"):
			m, err := blv.Parse(unpack(t, games, name))
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			d, err := delta.Parse(unpack(t, games, base+".dlv"), delta.DLV, len(m.Faces), len(m.Decorations), m.DDataSize)
			if err != nil {
				t.Fatalf("%s.dlv: %v", base, err)
			}
			nd++
			for i, door := range d.Doors {
				if len(door.XOffsets) != len(door.Verts) {
					t.Errorf("%s door %d: %d offsets for %d vertices", base, i, len(door.XOffsets), len(door.Verts))
				}
				for _, v := range door.Verts {
					if int(v) < 0 || int(v) >= len(m.Vertices) {
						t.Errorf("%s door %d: vertex %d out of range", base, i, v)
					}
				}
				for _, f := range door.Faces {
					if int(f) < 0 || int(f) >= len(m.Faces) {
						t.Errorf("%s door %d: face %d out of range", base, i, f)
					}
				}
				if door.State > delta.DoorOpening {
					t.Errorf("%s door %d: state %d", base, i, door.State)
				}
			}
			if len(d.Notes) != delta.NotesSize-4 {
				t.Errorf("%s.dlv: %d bytes of notes", base, len(d.Notes))
			}
		case strings.HasSuffix(name, ".odm"):
			m, err := odm.Parse(unpack(t, games, name))
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			faces := 0
			for _, mod := range m.Models {
				faces += len(mod.Faces)
			}
			if _, ok := games.Find(base + ".ddm"); !ok {
				t.Errorf("%s: no .ddm", name)
				continue
			}
			if _, err := delta.Parse(unpack(t, games, base+".ddm"), delta.DDM, faces, len(m.Decorations), 0); err != nil {
				t.Fatalf("%s.ddm: %v", base, err)
			}
			nm++
		}
	}
	if nd != 47 || nm != 14 {
		t.Errorf("parsed %d .dlv and %d .ddm, want 47 and 14", nd, nm)
	}
}
