package monsters

import (
	"testing"

	"libre-enroth/internal/assets/tables"
	"libre-enroth/internal/game/items"
	"libre-enroth/internal/game/party"
	"libre-enroth/internal/game/physics"
)

// dropTables: monster 4 is id 0x1f (a reagent dropper: item 0x28d, sprite 70, dobjlist
// kind 2); dobjlist kind 1 is an item of sprite 60 (radius 10, height 16).
func dropTables(b *Brain) *tables.Items {
	t := &tables.Items{Items: make([]tables.ItemDef, 0x300)}
	t.Items[0x28d].Sprite = 70
	t.Items[0x28d].EquipType = tables.EquipReagent
	t.Items[5].Sprite = 60
	b.Tables.ObjList = []tables.ObjListEntry{{}, {ID: 60, Radius: 10, Height: 16}, {ID: 70, Radius: 10, Height: 16}}
	b.Party.Ctx.Items = t
	return t
}

// Launch: speed along yaw and pitch (16.16 cosines, the products truncated as the
// original's); straight up is (0, 0, speed).
//
// mm8: 0x42eaea
func TestLaunch(t *testing.T) {
	for _, c := range []struct {
		yaw, pitch, speed int32
		want              [3]int16
	}{
		{0, 0, 200, [3]int16{200, 0, 0}},
		{physics.QuarterTurn, 0, 200, [3]int16{0, 200, 0}},
		{0, physics.QuarterTurn, 200, [3]int16{0, 0, 200}},
		{0, 0xb8, 200, [3]int16{
			int16(200 * physics.Mul16(physics.Cos(0xb8), 0x10000) >> 16), 0,
			int16(200 * physics.Cos(0xb8-physics.QuarterTurn) >> 16)}},
	} {
		objs, i := Launch(nil, Object{ObjList: 1, Pos: [3]int32{1, 2, 3}}, c.yaw, c.pitch, c.speed)
		if i != 0 || objs[0].Vel != c.want || objs[0].Start != [3]int32{1, 2, 3} {
			t.Errorf("yaw %d pitch %d: slot %d vel %v, want %v", c.yaw, c.pitch, i, objs[0].Vel, c.want)
		}
	}
}

// Actor_Die: the dying animation, no HP or buffs; the hunts of the monster's id are
// marked; each reagent drops on rand() % 100 < 20, thrown at rand() % 200 + 200 in a
// random direction (rand() % 2048) 45..90 degrees up ((rand() % 512 >> 1) + 256), 16
// above the feet.
//
// mm8: 0x402ec9, 0x42ecee
func TestDie(t *testing.T) {
	drops := 0
	for seed := uint32(1); seed < 60; seed++ {
		mt := aiTables()
		a := monster(mt, 4, 100, 200)
		a.Info.ID = 0x1f
		a.Buffs[BuffHaste].Expires = 5
		b := testBrain(1, 0, 0, a)
		b.Rand = &lcg{seed}
		dropTables(b)
		var objs []Object
		b.Objects = &objs
		m := b.Party.Members
		m.Hunts[0].Monster, m.Hunts[1].Monster = 0x1f, 5
		b.Die(0)
		d := &b.Actors[0]
		if d.AIState != Dying || d.HP != 0 || d.Animation != tables.AnimDying || d.Buffs[BuffHaste].Active() {
			t.Fatalf("dying: state %d hp %d anim %d", d.AIState, d.HP, d.Animation)
		}
		if m.Hunts[0].Killed != 1 || m.Hunts[1].Killed != 0 {
			t.Fatalf("hunts %+v", m.Hunts)
		}
		ref := &lcg{seed}
		if ref.Int()%100 >= 20 {
			if len(objs) != 0 {
				t.Errorf("seed %d: dropped %+v", seed, objs)
			}
			continue
		}
		drops++
		speed := int32(ref.Int()%200 + 200)
		yaw := int32(ref.Int() % 2048)
		pitch := int32(ref.Int()%512>>1) + 256
		_, want := Launch(nil, Object{ObjList: 2}, yaw, pitch, speed)
		w, _ := Launch(nil, Object{ObjList: 2}, yaw, pitch, speed)
		if len(objs) != 1 || objs[0].Item.Number != 0x28d || objs[0].Type != 70 || objs[0].ObjList != 2 ||
			objs[0].Pos != [3]int32{100, 200, 16} || objs[0].Vel != w[0].Vel || objs[0].Yaw != uint16(yaw) || want != 0 {
			t.Errorf("seed %d: drop %+v", seed, objs)
		}
	}
	if drops == 0 {
		t.Error("no seed dropped the reagent")
	}
}

// Actor_GenerateLoot: the gold dice into the gold slot (0xbb), by the treasure chance
// an item into the loot slot; the corpse is marked looted.
//
// mm8: 0x40920b
func TestGenerateLoot(t *testing.T) {
	mt := aiTables()
	a := monster(mt, 1, 0, 0)
	a.Info.GoldDice, a.Info.GoldSides, a.Info.TreasureChance = 3, 6, 0
	r, ref := &lcg{7}, &lcg{7}
	want := int32(0)
	for range 3 {
		want += int32(1 + ref.Int()%6)
	}
	a.GenerateLoot(&tables.Items{}, r, nil)
	if g := a.Items[ItemGold]; g.Number != items.GoldSmall || g.Special != want || a.Flags&FlagLooted == 0 {
		t.Errorf("gold %+v, want %d; flags %#x", g, want, a.Flags)
	}
}

// The kill checks count the actors whose alert-only bit equals Map_IsAlert (none on an
// alert map); mode 0 all, 1 a group, 2 a monster id - 1, 3 one actor, 4 a placemon name;
// count 0 asks for all gone, else at least that many.
//
// mm8: 0x444473, 0x4094d8..0x40968c
func TestIsKilled(t *testing.T) {
	mt := aiTables()
	acts := []Actor{monster(mt, 1, 0, 0), monster(mt, 2, 0, 0), monster(mt, 2, 0, 0), monster(mt, 3, 0, 0)}
	acts[0].Group, acts[1].Group, acts[2].Group = 7, 7, 8
	acts[1].UniqueName = 4
	acts[3].Flags |= FlagAlertOnly
	acts[0].AIState = Dead
	acts[2].AIState = Disabled
	for _, c := range []struct {
		mode  int
		param int32
		n     int
		dis   bool
		alert uint32
		want  bool
	}{
		{KilledAll, 0, 0, false, 0, false},
		{KilledAll, 0, 1, false, 0, true},
		{KilledAll, 0, 2, true, 0, true},
		{KilledAll, 0, 0, false, 1, true}, // nobody counts on an alert map
		{KilledGroup, 7, 0, false, 0, false},
		{KilledGroup, 8, 0, true, 0, true},
		{KilledGroup, 8, 0, false, 0, false},
		{KilledMonster, 0, 0, false, 0, true},
		{KilledMonster, 1, 0, true, 0, false},
		{KilledMonster, 2, 0, false, 0, true}, // monster 3 is alert-only: none count
		{KilledActor, 0, 0, false, 0, true},
		{KilledActor, 9, 0, false, 0, true}, // no such actor: none count
		{KilledUnique, 4, 1, false, 0, false},
		{5, 0, 0, false, 0, false},
	} {
		if got := IsKilled(acts, c.alert, c.mode, c.param, c.n, c.dis); got != c.want {
			t.Errorf("%+v: %v", c, got)
		}
	}
}

// The event opcodes on actors: flags (clearing exactly the disabled bit stands one up),
// group flags (setting it disables the group; clearing it stands up the living),
// groups, allies, items.
//
// mm8: 0x446e7d, 0x446edd, 0x446bd6, 0x446bf6, 0x44690e, 0x446854
func TestActorEvents(t *testing.T) {
	mt := aiTables()
	acts := []Actor{monster(mt, 1, 0, 0), monster(mt, 2, 0, 0), monster(mt, 3, 0, 0)}
	acts[0].Group, acts[1].Group, acts[2].Group = 3, 3, 4
	acts[2].NPC = 77
	acts[0].AIState, acts[0].Flags = Disabled, FlagDisabled
	SetActorFlag(acts, 0, FlagDisabled, false)
	if acts[0].AIState != Stand || acts[0].Flags&FlagDisabled != 0 {
		t.Errorf("actor 0: state %d flags %#x", acts[0].AIState, acts[0].Flags)
	}
	SetActorFlag(acts, 9, FlagDisabled, true) // no such actor
	SetGroupFlag(acts, 3, FlagDisabled, true, allLoaded(true))
	if acts[0].AIState != Disabled || acts[1].AIState != Disabled || acts[2].AIState == Disabled {
		t.Errorf("group 3 disabled: %d %d %d", acts[0].AIState, acts[1].AIState, acts[2].AIState)
	}
	acts[1].HP = 0
	SetGroupFlag(acts, 3, FlagDisabled, false, allLoaded(true))
	if acts[0].AIState != Stand || acts[1].AIState != Disabled || acts[1].Flags&FlagDisabled != 0 {
		t.Errorf("group 3 enabled: %d %d", acts[0].AIState, acts[1].AIState)
	}
	ChangeGroup(acts, 3, 5)
	ChangeGroupAlly(acts, 5, 9999)
	SetActorGroup(acts, 2, 6)
	if acts[0].Group != 5 || acts[1].Ally != 9999 || acts[2].Group != 6 || acts[2].Ally != 0 {
		t.Errorf("groups %d %d %d allies %d %d", acts[0].Group, acts[1].Group, acts[2].Group, acts[1].Ally, acts[2].Ally)
	}
	for _, id := range []int32{10, 11, 12, 13} {
		SetActorItem(acts, 0, id, true)
	}
	a := &acts[0]
	if a.CarriedItem != 10 || a.Items[ItemEvent0].Number != 11 || a.Items[ItemEvent1].Number != 12 {
		t.Errorf("given: %d %d %d", a.CarriedItem, a.Items[0].Number, a.Items[1].Number)
	}
	SetActorItem(acts, 0, 12, false)
	SetActorItem(acts, 0, 10, false)
	if a.CarriedItem != 0 || a.Items[ItemEvent1].Number != 0 || a.Items[ItemEvent0].Number != 11 {
		t.Errorf("taken: %d %d %d", a.CarriedItem, a.Items[0].Number, a.Items[1].Number)
	}
	NPCSetItem(acts, 77, 20, true)
	NPCSetItem(acts, 78, 21, true)
	if acts[2].CarriedItem != 20 {
		t.Errorf("npc item %d", acts[2].CarriedItem)
	}
}

// Actors_ResetAfterRest: the living go home with full HP and stand for their recovery
// facing their target; the disabled go home too but keep their state; all calm down,
// peasants forget their hostility, corpses lose their rolled loot. A scheduled actor
// moves to the job of the hour.
//
// mm8: 0x40928c, 0x403446
func TestResetAfterRest(t *testing.T) {
	mt := aiTables()
	acts := []Actor{monster(mt, 1, 100, 100), monster(mt, 2, 100, 100), monster(mt, 3, 100, 100), monster(mt, 4, 100, 100)}
	for i := range acts {
		a := &acts[i]
		a.Pos = [3]int16{500, 600, 0}
		a.HP = 1
		a.Info.Hostility = 4
	}
	acts[1].AIState, acts[1].Flags = Disabled, FlagDisabled
	acts[2].AIState, acts[2].Flags = Dead, FlagLooted
	acts[3].Ally = 1 // a peasant class
	acts[3].Flags |= FlagHostile | FlagSchedule
	acts[3].Jobs[2] = Job{X: 300, Y: 400, Z: 5, Attrib: jobActive, Action: tables.MoveFree, Hour: 8}
	acts[3].Jobs[5] = Job{X: 700, Y: 800, Z: 5, Attrib: jobActive, Action: tables.MoveFree, Hour: 20}
	b := testBrain(1, 0, 0, acts...)
	b.ResetAfterRest(10)
	a := b.Actors
	if a[0].Pos != a[0].Start || int32(a[0].HP) != a[0].Info.HP || a[0].AIState != Stand ||
		int32(a[0].ActionLength) != a[0].Info.Recovery || a[0].Info.Hostility != 0 {
		t.Errorf("living: %+v", a[0].Pos)
	}
	if a[1].Pos != a[1].Start || a[1].AIState != Disabled || int32(a[1].HP) != a[1].Info.HP {
		t.Errorf("disabled: pos %v state %d hp %d", a[1].Pos, a[1].AIState, a[1].HP)
	}
	if a[2].Pos == a[2].Start || a[2].HP != 1 || a[2].Flags&FlagLooted != 0 {
		t.Errorf("corpse: pos %v hp %d flags %#x", a[2].Pos, a[2].HP, a[2].Flags)
	}
	if a[3].Flags&FlagHostile != 0 || a[3].Start != [3]int16{300, 400, 5} || a[3].Pos != a[3].Start || a[3].Info.Move != tables.MoveFree {
		t.Errorf("peasant: flags %#x start %v pos %v", a[3].Flags, a[3].Start, a[3].Pos)
	}
}

// Objects_Update and Objects_MoveIndoor: an item dropped in the air falls (gravity 5 per
// tick), lands on the floor and slows to rest; one sliding along the floor stops at a
// wall, its radius away (bouncing back a little); one with a
// lifetime kind goes when it is over.
//
// mm8: 0x472b60, 0x4716af
func TestMoveObjectsIndoor(t *testing.T) {
	g := rooms()
	b := indoorBrain(g, 1800, 900)
	dropTables(b)
	objs := []Object{
		{ObjList: 1, Type: 60, Pos: [3]int32{500, 500, 300}, Sector: 1, Item: items.Item{Number: 5}},
		{ObjList: 1, Type: 60, Pos: [3]int32{40, 200, 1}, Sector: 1, Vel: [3]int16{-400, 0, 0}},
	}
	b.Objects = &objs
	b.MoveObjects(2)
	if objs[0].Vel[2] != -10 || objs[0].Time != 2 {
		t.Errorf("first frame: vel %v time %d", objs[0].Vel, objs[0].Time)
	}
	for range 300 {
		b.MoveObjects(2)
	}
	if o := &objs[0]; o.Pos[2] < 0 || o.Pos[2] > 2 || o.Vel != [3]int16{} || o.Pos[0] != 500 {
		t.Errorf("dropped: pos %v vel %v", o.Pos, o.Vel)
	}
	if o := &objs[1]; o.Pos[0] < 9 || o.Pos[0] > 20 || o.Vel != [3]int16{} {
		t.Errorf("thrown: pos %v vel %v", o.Pos, o.Vel)
	}
	b.Tables.ObjList[1].Flags |= tables.ObjLifetime
	b.Tables.ObjList[1].Lifetime = 10
	b.MoveObjects(2)
	if objs[0].ObjList != 0 || objs[0].Attrib&ObjRemoved == 0 {
		t.Errorf("lifetime over: %+v", objs[0])
	}
}

// Party.Members.MarkKilled flags the hunts of a monster id.
func TestMarkKilled(t *testing.T) {
	var m party.Members
	m.Hunts[2].Monster = 9
	m.MarkKilled(9)
	m.MarkKilled(3)
	if m.Hunts[2].Killed != 1 || m.Hunts[0].Killed != 0 {
		t.Errorf("%+v", m.Hunts)
	}
}
