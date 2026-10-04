package world

import (
	"testing"

	"libre-enroth/internal/game/party"
	"libre-enroth/internal/game/physics"
)

// walk pushes one action per 2-tick frame for n frames (0 = none).
func walk(w *World, n int, acts ...party.Action) {
	for range n {
		for _, a := range acts {
			w.group.Actions.Push(a)
		}
		w.moveParty(2)
	}
}

// TestPartySpawn: the party starts on the floor under the map's Party Start (out01:
// the new-game position) and stays there.
//
// mm8: 0x472e86, 0x473fee, 0x46d0d0 (Indoor_FloorZ), 0x46d6bb (Outdoor_FloorZ)
func TestPartySpawn(t *testing.T) {
	e := newEnv(t)
	for _, c := range []struct {
		name    string
		x, y, z int32
		sector  int
	}{
		{"d05.blv", -3008, -1696, 2465, 15},
		{"d16.blv", -1216, 1888, 1, 6},
		{"d28.blv", 0, -448, 1, 1},
		{"out01.odm", 3766, 7649, 544, 0},
	} {
		w, err := Load(e.d, e.tables, e.tex, c.name, nil)
		if err != nil {
			t.Fatal(err)
		}
		walk(w, 60)
		p := w.group
		if p.X != c.x || p.Y != c.y || p.Z != c.z || p.VZ != 0 || p.Sector != c.sector || p.Flags != 0 {
			t.Errorf("%s: at (%d, %d, %d) vz %d sector %d flags %#x, want (%d, %d, %d) sector %d",
				c.name, p.X, p.Y, p.Z, p.VZ, p.Sector, p.Flags, c.x, c.y, c.z, c.sector)
		}
	}
}

// TestD28Walks: from the start of d28 the party walks into the walls on all four sides
// (stopping at its radius; going west or south moves 6 units a frame instead of 5, as
// the arithmetic shifts round down), and up the stairs to the north-east.
//
// mm8: 0x472e86 (Party_MoveIndoor), 0x46e663 (Collide_IndoorFaces)
func TestD28Walks(t *testing.T) {
	e := newEnv(t)
	for _, c := range []struct {
		dir     int32
		x, y, z int32
	}{
		{0, 509, -442, 1},
		{512, 0, 537, 1},
		{1024, -54, -448, 1},
		{1536, 0, -1432, 1},
		{128, 762, -345, 73}, // the stairs
	} {
		w, err := Load(e.d, e.tables, e.tex, "d28.blv", nil)
		if err != nil {
			t.Fatal(err)
		}
		w.group.Dir = c.dir
		lastZ := w.group.Z
		for range 800 {
			walk(w, 1, party.Forward)
			if w.group.Z < lastZ {
				t.Errorf("dir %d: went down from %d to %d", c.dir, lastZ, w.group.Z)
			}
			lastZ = w.group.Z
		}
		if p := w.group; p.X != c.x || p.Y != c.y || p.Z != c.z {
			t.Errorf("dir %d: ended at (%d, %d, %d), want (%d, %d, %d)", c.dir, p.X, p.Y, p.Z, c.x, c.y, c.z)
		}
	}
}

// TestOut01Water: walking west from the beach of Dagger Wound. On foot the water gate
// stops the party on the last dry cell; with water walking it walks on at the terrain
// height (flag 0x80); levitating it floats 20 above; jumping in sinks it to 60 under the
// surface (z = floor + 1 = -59) with the water flag.
//
// mm8: 0x476c22 (Outdoor_WaterGate), 0x483842 (Terrain_HeightAt: -0x3c, levitating +0x14)
func TestOut01Water(t *testing.T) {
	e := newEnv(t)
	for _, c := range []struct {
		mode    string
		x, z    int32
		flags   uint32
		jumpAt  int
		inWater bool
	}{
		{"foot", -508, 0, 0, -1, false},
		{"water walk", -1612, 0, party.FlagWaterWalk, -1, true},
		{"levitate", -1612, 20, 0, -1, true},
		{"jump", -1606, -59, party.FlagWater, 10, true},
	} {
		w, err := Load(e.d, e.tables, e.tex, "out01.odm", nil)
		if err != nil {
			t.Fatal(err)
		}
		p := w.group
		shore := int32(physics.GridX(p.X)-8-64) * 512 // the last dry cell's west edge
		p.Teleport(shore+100, p.Y, p.Z, 1024)
		w.dropParty()
		p.WaterWalk = c.mode == "water walk"
		p.Levitate = c.mode == "levitate"
		for i := range 200 {
			if i == c.jumpAt {
				p.Actions.Push(party.Jump)
			}
			walk(w, 1, party.Forward)
		}
		_, water, _ := w.outdoor.geo.TerrainZ(p.X, p.Y, false, true, true)
		if p.X != c.x || p.Y != 7649 || p.Z != c.z || p.Flags != c.flags || water != c.inWater {
			t.Errorf("%s: at (%d, %d, %d) flags %#x over water %v, want x %d z %d flags %#x water %v",
				c.mode, p.X, p.Y, p.Z, p.Flags, water, c.x, c.z, c.flags, c.inWater)
		}
	}
}
