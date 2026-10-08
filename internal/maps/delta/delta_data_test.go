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

// TestActorsObjects decodes the actor and object records of every template and encodes
// them back byte for byte. out01.ddm has 62 actors (monsters 1, 2, 3, 5 and 12); d05.dlv
// has none and 52 spawn points.
func TestActorsObjects(t *testing.T) {
	games, err := lod.Open(assettest.File(t, "games.lod"))
	if err != nil {
		t.Fatal(err)
	}
	defer games.Close()
	n := 0
	for _, e := range games.Entries {
		name := strings.ToLower(e.Name)
		var kind delta.Kind
		var numFaces, numDecs int
		var ddata int32
		base := name[:max(len(name)-4, 0)]
		switch {
		case strings.HasSuffix(name, ".blv"):
			m, err := blv.Parse(unpack(t, games, name))
			if err != nil {
				t.Fatal(err)
			}
			kind, numFaces, numDecs, ddata = delta.DLV, len(m.Faces), len(m.Decorations), m.DDataSize
			base += ".dlv"
			if name == "d05.blv" && len(m.Spawns) != 52 {
				t.Errorf("d05: %d spawn points", len(m.Spawns))
			}
		case strings.HasSuffix(name, ".odm"):
			m, err := odm.Parse(unpack(t, games, name))
			if err != nil {
				t.Fatal(err)
			}
			for _, mod := range m.Models {
				numFaces += len(mod.Faces)
			}
			kind, numDecs = delta.DDM, len(m.Decorations)
			base += ".ddm"
		default:
			continue
		}
		raw := unpack(t, games, base)
		d, err := delta.Parse(raw, kind, numFaces, numDecs, ddata)
		if err != nil {
			t.Fatalf("%s: %v", base, err)
		}
		n++
		off := 0x28 + len(d.Revealed) + 4*numFaces + 2*numDecs + 4
		buf := make([]byte, delta.ActorSize)
		for i := range d.Actors {
			d.Actors[i].Encode(buf)
			if want := raw[off+i*delta.ActorSize : off+(i+1)*delta.ActorSize]; string(buf) != string(want) {
				t.Errorf("%s actor %d does not encode back", base, i)
			}
		}
		off += len(d.Actors)*delta.ActorSize + 4
		ob := make([]byte, delta.ObjectSize)
		for i := range d.Objects {
			d.Objects[i].Encode(ob)
			if want := raw[off+i*delta.ObjectSize : off+(i+1)*delta.ObjectSize]; string(ob) != string(want) {
				t.Errorf("%s object %d does not encode back", base, i)
			}
		}
		switch base {
		case "out01.ddm":
			ids := map[int]int{}
			for i := range d.Actors {
				ids[d.Actors[i].ID()]++
			}
			if len(d.Actors) != 62 || len(ids) != 5 || ids[1] == 0 || ids[2] == 0 || ids[3] == 0 || ids[5] == 0 || ids[12] == 0 {
				t.Errorf("out01: %d actors, ids %v", len(d.Actors), ids)
			}
		case "d05.dlv":
			if len(d.Actors) != 0 {
				t.Errorf("d05: %d actors", len(d.Actors))
			}
		}
	}
	if n != 61 {
		t.Errorf("%d delta files", n)
	}
}
