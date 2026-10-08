package monsters

import (
	"fmt"
	"testing"

	"libre-enroth/internal/assets/desc"
	"libre-enroth/internal/assets/tables"
	"libre-enroth/internal/game/items"
	"libre-enroth/internal/game/physics"
)

// lcg is the MSVC rand() (party.Rand).
type lcg struct{ s uint32 }

func (r *lcg) Int() int {
	r.s = r.s*0x343fd + 0x269ec3
	return int(r.s >> 16 & 0x7fff)
}

// testTables: monsters 1..3 are "Foo A/B/C" (dmonlist 0..2), monster 4 "Bar A".
func testTables() *tables.Monsters {
	m := &tables.Monsters{Infos: make([]tables.MonsterInfo, 5)}
	for i, v := range []string{"Foo A", "Foo B", "Foo C", "Bar A"} {
		m.MonList = append(m.MonList, tables.MonListEntry{Name: v, Height: uint16(100 + i), Radius: uint16(30 + i), Speed: 200,
			Tint: 0x112233, Sprites: [10]string{"st", "wk", "at", "sh", "gh", "dy", "dd", "fd"}})
		m.Infos[i+1] = tables.MonsterInfo{Name: fmt.Sprint("Monster ", i+1), Picture: v, ID: uint16(i + 1), HP: int32(10 * (i + 1)), Speed: int32(300 + i)}
	}
	m.MapStats = []tables.MapStats{{}, {Monster: [3]string{"Foo", "Bar", ""}, Dif: [3]uint8{2, 0, 0}, Min: [3]uint8{2, 1, 1}, Max: [3]uint8{4, 1, 1}}}
	return m
}

func testSFT() *desc.SFT {
	s := &desc.SFT{}
	for _, g := range []string{"null", "st", "wk", "at", "sh", "gh", "dy", "dd", "fd"} {
		s.Frames = append(s.Frames, desc.Frame{Group: g})
	}
	// EIndex is sorted by group name: at dd dy fd gh null sh st wk.
	s.EIndex = []int{3, 7, 6, 8, 5, 0, 4, 1, 2}
	return s
}

// Spawn_Monsters on an outdoor point of MapStats slot 1 (difficulty 2: A < 70, B < 90):
// rand() % 3 + 2 monsters, each one rand() % 100 for its variant and one rand() % 0x800
// for the next monster's place 0x80 away; the first stands on the point.
//
// mm8: 0x44dfbe
func TestSpawnOutdoor(t *testing.T) {
	mt := testTables()
	// Seed 18 makes 3 monsters: B, C, A.
	e := &Env{Tables: mt, SFT: testSFT(), Rand: &lcg{18}}
	sp := &SpawnPoint{X: 1000, Y: 2000, Z: 50, Kind: SpawnMonsters, Index: 1, Group: 7}
	got, err := Spawn(e, nil, &mt.MapStats[1], sp, 0, 0, true)
	if err != nil {
		t.Fatal(err)
	}
	ref := &lcg{18}
	n := ref.Int()%3 + 2
	if len(got) != n {
		t.Fatalf("%d monsters, want %d", len(got), n)
	}
	if got[0].ID() != 2 || got[1].ID() != 3 || got[2].ID() != 1 {
		t.Errorf("variants %d %d %d, want B C A", got[0].ID(), got[1].ID(), got[2].ID())
	}
	x, y := int32(1000), int32(2000)
	for i, a := range got {
		v := ref.Int() % 100
		want := 3
		switch {
		case v < 70:
			want = 1
		case v < 90:
			want = 2
		}
		if a.ID() != want || int(a.MonList) != want || a.HP != int16(10*want) {
			t.Errorf("monster %d: id %d monlist %d hp %d, want variant %d", i, a.ID(), a.MonList, a.HP, want)
		}
		if a.Pos != [3]int16{int16(x), int16(y), 50} || a.Start != a.Pos || a.Group != 7 || a.Tether != 0x100 {
			t.Errorf("monster %d at %v start %v group %d, want (%d, %d, 50)", i, a.Pos, a.Start, a.Group, x, y)
		}
		if a.Flags != FlagHostile || a.Info.Hostility != 0 {
			t.Errorf("monster %d flags %#x", i, a.Flags)
		}
		// Actor_LoadSprites: sizes from dmonlist by the monsters.txt id, monsters.txt's speed.
		if a.Height != uint16(100+want-1) || a.Radius != uint16(30+want-1) || a.Speed != uint16(300+want-1) ||
			a.Sprites != [8]int16{1, 2, 3, 4, 5, 6, 7, 8} {
			t.Errorf("monster %d: h %d r %d speed %d sprites %v", i, a.Height, a.Radius, a.Speed, a.Sprites)
		}
		ang := int32(ref.Int() % 0x800)
		x = 1000 + int32(0x80*int64(physics.Cos(ang))>>16)
		y = 2000 + int32(0x80*int64(physics.Cos(ang-physics.QuarterTurn))>>16)
	}
	if r := e.Rand.Int(); r != ref.Int() {
		t.Error("random numbers out of step")
	}
}

// Indices 4..12 name the slot and its variant directly: one monster, no rand().
func TestSpawnFixedVariant(t *testing.T) {
	mt := testTables()
	e := &Env{Tables: mt, SFT: testSFT(), Rand: &lcg{1}}
	for _, c := range []struct {
		index uint16
		id    int
	}{{4, 1}, {7, 2}, {10, 3}, {5, 4}} {
		got, err := Spawn(e, nil, &mt.MapStats[1], &SpawnPoint{Kind: SpawnMonsters, Index: c.index}, 0, 0, false)
		if err != nil || len(got) != 1 || got[0].ID() != c.id {
			t.Errorf("index %d: %v %v", c.index, got, err)
		}
	}
	// The alert bit: an alert-only point spawns only when the map is alert.
	if got, _ := Spawn(e, nil, &mt.MapStats[1], &SpawnPoint{Index: 4, Attrib: SpawnAlertOnly}, 0, 0, false); len(got) != 0 {
		t.Error("alert-only point spawned on a calm map")
	}
	e.Alert = 1
	if got, _ := Spawn(e, nil, &mt.MapStats[1], &SpawnPoint{Index: 4}, 0, 0, false); len(got) != 0 {
		t.Error("calm point spawned on an alert map")
	}
	if got, _ := Spawn(e, nil, &mt.MapStats[1], &SpawnPoint{Index: 4, Attrib: SpawnAlertOnly}, 0, 0, false); len(got) != 1 {
		t.Error("alert-only point did not spawn on an alert map")
	}
	// A slot naming a monster dmonlist lacks is the original's fatal error.
	e.Alert = 0
	mt.MapStats[1].Monster[2] = "Nope"
	if _, err := Spawn(e, nil, &mt.MapStats[1], &SpawnPoint{Index: 6}, 0, 0, false); err == nil {
		t.Error("unknown monster spawned")
	}
}

// indoorGeo: one sector for x < 2000, floor 0 for y < 3000.
type indoorGeo struct{}

func (indoorGeo) SectorAt(x, y, z int32) int {
	if x < 2000 {
		return 1
	}
	return 2
}

func (indoorGeo) FloorZ(x, y, z int32, s int) (int32, int) {
	if y < 3000 {
		return 0, 1
	}
	return physics.NoFloor, 0
}

// Indoors the next place must be in the point's sector over a floor near the point's
// height; a rejected place still positions the next monster, which takes the slot.
func TestSpawnIndoor(t *testing.T) {
	mt := testTables()
	mt.MapStats[1].Dif[0] = 0
	mt.MapStats[1].Monster[0] = "Foo A"
	mt.MapStats[1].Min[0], mt.MapStats[1].Max[0] = 3, 3
	e := &Env{Tables: mt, SFT: testSFT(), Rand: &lcg{5}, Indoor: indoorGeo{}}
	sp := &SpawnPoint{X: 1960, Y: 2960, Z: 40, Kind: SpawnMonsters, Index: 1}
	got, err := Spawn(e, nil, &mt.MapStats[1], sp, 0, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	ref := &lcg{5}
	ref.Int() // the count
	x, y, z := int32(1960), int32(2960), int32(40)
	var want [][3]int16
	for range 3 {
		pos := [3]int16{int16(x), int16(y), int16(z)}
		ang := int32(ref.Int() % 0x800)
		x = 1960 + int32(0x40*int64(physics.Cos(ang))>>16)
		y = 2960 + int32(0x40*int64(physics.Cos(ang-physics.QuarterTurn))>>16)
		z = 40
		if x < 2000 && y < 3000 {
			z = 0
			want = append(want, pos)
		}
	}
	if len(got) != len(want) {
		t.Fatalf("%d kept, want %d", len(got), len(want))
	}
	for i := range got {
		if got[i].Pos != want[i] || got[i].Sector != 1 {
			t.Errorf("monster %d at %v sector %d, want %v", i, got[i].Pos, got[i].Sector, want[i])
		}
	}
}

type allLoaded bool

func (l allLoaded) Loaded(int) bool { return bool(l) }

// Actor_UpdateAnimation's table.
//
// mm8: 0x4584dd
func TestUpdateAnimation(t *testing.T) {
	for _, c := range []struct {
		state int16
		anim  uint16
		set   bool
	}{
		{Stand, tables.AnimStand, true}, {Tethered, tables.AnimWalk, false}, {Melee, tables.AnimAttack, true},
		{Ranged1, tables.AnimShoot, true}, {Ranged2, tables.AnimShoot, true}, {Ranged3, tables.AnimShoot, true},
		{Ranged4, tables.AnimShoot, true}, {Dying, tables.AnimDying, true}, {Resurrected, tables.AnimDying, true},
		{Dead, tables.AnimDead, false}, {Pursue, tables.AnimWalk, true}, {Flee, tables.AnimWalk, true},
		{Stunned, tables.AnimGotHit, true}, {Fidget, tables.AnimFidget, true}, {Interacting, tables.AnimStand, true},
		{Summoned, tables.AnimStand, true}, {Stoned, 99, false}, {Disabled, 99, false},
	} {
		a := Actor{AIState: c.state, Animation: 99, Flags: FlagAnimationSet}
		a.UpdateAnimation(allLoaded(true))
		if a.Animation != c.anim || (a.Flags&FlagAnimationSet != 0) != c.set {
			t.Errorf("state %#x: animation %d set %v, want %d %v", c.state, a.Animation, a.Flags&FlagAnimationSet != 0, c.anim, c.set)
		}
	}
	a := Actor{AIState: Dead}
	a.UpdateAnimation(allLoaded(false))
	if a.AIState != Removed {
		t.Error("a dead actor without a dead sprite stays")
	}
}

// Actor_GetRelation: same group 0, the party row of hostile.txt by class (id - 1) / 3 + 1,
// the hostile flag, the ally override and the buffs.
//
// mm8: 0x401051
func TestRelation(t *testing.T) {
	mt := &tables.Monsters{Hostile: make([]uint8, tables.HostileColumns*tables.HostileStride)}
	set := func(a, b int, v uint8) { mt.Hostile[a*tables.HostileStride+b] = v }
	set(0, 2, 3) // the party towards class 2
	set(2, 0, 0)
	set(2, 3, 4)
	lizard := &Actor{Info: tables.MonsterInfo{ID: 4}} // class 2
	pirate := &Actor{Info: tables.MonsterInfo{ID: 9}} // class 3
	other := &Actor{Info: tables.MonsterInfo{ID: 30}} // class 10
	if r := Relation(mt, lizard, nil); r != 0 {
		t.Errorf("lizard to party %d", r)
	}
	if r := Relation(mt, nil, lizard); r != 3 {
		t.Errorf("party to lizard %d", r)
	}
	if r := Relation(mt, lizard, pirate); r != 4 {
		t.Errorf("lizard to pirate %d", r)
	}
	lizard.Group, pirate.Group = 5, 5
	if r := Relation(mt, lizard, pirate); r != 0 {
		t.Errorf("same group %d", r)
	}
	lizard.Group = 0
	lizard.Flags = FlagHostile
	if r := Relation(mt, lizard, nil); r != 4 {
		t.Errorf("hostile lizard to party %d", r)
	}
	lizard.Flags = 0
	other.Ally = 2 // takes class 2's row
	if r := Relation(mt, other, pirate); r != 4 {
		t.Errorf("ally of class 2 to pirate %d", r)
	}
	other.Ally = 9999
	if r := Relation(mt, other, pirate); r != 0 {
		t.Errorf("ally 9999 (class 0) to pirate %d", r)
	}
	lizard.Buffs[BuffBerserk].Expires = 1
	if r := Relation(mt, lizard, nil); r != 4 {
		t.Errorf("berserk %d", r)
	}
	lizard.Buffs[BuffBerserk].Expires = 0
	lizard.Buffs[BuffCharm].Expires = 1
	if r := Relation(mt, lizard, pirate); r != 4 {
		t.Errorf("charmed lizard to a monster %d", r)
	}
	if r := Relation(mt, lizard, nil); r != 0 {
		t.Errorf("charmed lizard to party %d", r)
	}
}

// Actor_Init and the record codec.
func TestActorRecord(t *testing.T) {
	var b [ActorSize]byte
	for i := range b {
		b[i] = byte(i*7 + 3)
	}
	a := DecodeActor(b[:])
	var out [ActorSize]byte
	a.Encode(out[:])
	if out != b {
		t.Fatal("actor record does not encode back")
	}
	a.Init()
	if a.Tether != 0x100 || a.Radius != 0x20 || a.Height != 0x80 || a.Speed != 200 || a.Flags != 0 ||
		a.Buffs != [NumBuffs]Buff{} || a.Pos != [3]int16{} || a.NPC != 0 {
		t.Errorf("Init: %+v", a)
	}
	var ob [ObjectSize]byte
	for i := range ob {
		ob[i] = byte(i*5 + 1)
	}
	o := DecodeObject(ob[:])
	var oo [ObjectSize]byte
	o.Encode(oo[:])
	if oo != ob {
		t.Fatal("object record does not encode back")
	}
}

// Object_Create fills the first free slot; an object without a dobjlist entry is not made.
func TestCreateObject(t *testing.T) {
	objs := []Object{{ObjList: 3}, {}, {ObjList: 4}}
	objs, i := CreateObject(objs, Object{ObjList: 5, Pos: [3]int32{1, 2, 3}})
	if i != 1 || objs[1].Start != [3]int32{1, 2, 3} || len(objs) != 3 {
		t.Errorf("slot %d, %+v", i, objs[1])
	}
	objs, i = CreateObject(objs, Object{ObjList: 6})
	if i != 3 || len(objs) != 4 {
		t.Errorf("slot %d, %d objects", i, len(objs))
	}
	if _, i = CreateObject(objs, Object{}); i != -1 {
		t.Error("an object without a kind was made")
	}
}

// Spawn_Items: rand() % 100 (below 20 nothing, below 60 gold by the point's index, else
// an item), then the level from g_treasureLevels[index * 7 + map treasure]; the gold
// pile lies at the point as an identified object of its sprite's dobjlist kind.
//
// mm8: 0x44ea5f
func TestSpawnItemsGold(t *testing.T) {
	mt := testTables()
	mt.ObjList = []tables.ObjListEntry{{}, {ID: 50}, {ID: 51}}
	mt.Treasure[2*7+3] = [2]uint8{2, 2}
	it := &tables.Items{Items: make([]tables.ItemDef, 0x100)}
	it.Items[0xbb].Sprite = 51
	row := &tables.MapStats{Treasure: 3}
	sp := &SpawnPoint{X: 5, Y: 6, Z: 7, Kind: 1, Index: 2}
	for seed := uint32(1); seed < 40; seed++ {
		e := &Env{Tables: mt, Items: it, Rand: &lcg{seed}}
		ref := &lcg{seed}
		r := ref.Int() % 100
		ref.Int() // the level: always 2
		objs := SpawnItems(e, nil, row, sp)
		switch {
		case r < 20:
			if len(objs) != 0 {
				t.Errorf("seed %d: r %d made %v", seed, r, objs)
			}
		case r < 60:
			amount := int32(ref.Int()%101 + 100)
			if len(objs) != 1 || objs[0].Item.Number != 0xbb || objs[0].Item.Special != amount ||
				objs[0].Item.Flags != items.FlagIdentified || objs[0].ObjList != 2 || objs[0].Type != 51 ||
				objs[0].Pos != [3]int32{5, 6, 7} || objs[0].Glow != 1 {
				t.Errorf("seed %d: gold %+v, want %d", seed, objs, amount)
			}
		}
	}
}
