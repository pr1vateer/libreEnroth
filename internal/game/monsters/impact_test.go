package monsters

import (
	"testing"

	"libre-enroth/internal/assets/tables"
	"libre-enroth/internal/game/party"
)

// LaunchFor steps a member's projectile aside: slots 0 and 1 left by 0x18, slot 2
// ahead by 8, slots 3 and 4 right by 0x18. member 0 does not move.
//
// mm8: 0x42eaea
func TestLaunchMemberOffset(t *testing.T) {
	base := Object{ObjList: 1, Pos: [3]int32{1000, 2000, 300}, Yaw: 0}
	roster := []int{10, 11, 12, 13, 14}
	got, _ := LaunchFor(nil, base, 0, 0, 0, 0, roster)
	if got[0].Pos != base.Pos {
		t.Fatalf("member 0 moved to %v", got[0].Pos)
	}
	for slot, id := range roster {
		o := base
		objs, _ := LaunchFor(nil, o, 0, 0, 0, int32(id)+1, roster)
		var dist, yaw int32
		switch slot {
		case 0, 1:
			dist, yaw = 0x18, int32(int16(base.Yaw))+512
		case 2:
			dist, yaw = 8, int32(int16(base.Yaw))
		default:
			dist, yaw = 0x18, int32(int16(base.Yaw))-512
		}
		x, y, z := offsetPoint(dist, yaw, 0, base.Pos[0], base.Pos[1], base.Pos[2])
		if objs[0].Pos != [3]int32{x, y, z} {
			t.Errorf("slot %d at %v, want %d %d %d", slot, objs[0].Pos, x, y, z)
		}
	}
}

func impactBrain() *Brain {
	mt := &tables.Monsters{ObjList: []tables.ObjListEntry{
		{},
		{ID: 1020, Flags: tables.ObjFlagOnIntercept | tables.ObjLifetime, Lifetime: 100, Speed: 400},
		{ID: 1021, Flags: tables.ObjLifetime, Lifetime: 20},
		{ID: 1050, Flags: tables.ObjFlagOnIntercept | tables.ObjLifetime, Lifetime: 100},
		{ID: 1051, Flags: tables.ObjLifetime, Lifetime: 30},
		{ID: 3090, Flags: tables.ObjFlagOnIntercept},
		{ID: 3092, Speed: 1000},
		{ID: 545, Flags: tables.ObjFlagOnIntercept | tables.ObjNoPickup, Speed: 500, Radius: 8, Height: 8},
	}}
	return &Brain{Tables: mt, Spells: tables.Spells{{}, {School: 0, Object: 1020}, {School: 0, Add: 6, Sides: 1, Object: 1050}}}
}

// A party projectile that meets the party keeps flying. One that meets an actor
// stops and hurts him. A wall (pid 0) stops a fireball into its burst. Outside
// 0x200 an area blow misses; inside it hits, including the caster.
//
// mm8: 0x46bd3a, 0x4373e5
func TestImpactAndArea(t *testing.T) {
	b := impactBrain()
	b.Party = Party{X: 0, Y: 0, Z: 0, Height: 192, EyeLevel: 160, Radius: 40}
	m := &party.Members{Players: []party.Player{{RosterID: 0, HP: 40, SP: 10, Name: "A"}}}
	m.Players[0].Stats[party.StatEndurance] = party.StatPair{Base: 10}
	c := &party.Ctx{Rand: party.NewRand(1), Items: &tables.Items{}, Classes: &tables.Classes{
		BonusLimits: []int16{1, 0}, Bonus: []int8{0, 0},
	}}
	b.Party.Members = m
	b.Party.Ctx = c
	b.Rand = c.Rand
	objs := []Object{{
		Type: 1020, ObjList: 1, Pos: [3]int32{0, 0, 0}, Caster: KindParty, Spell: 2, SpellLevel: 5, SpellSkill: 1,
	}}
	b.Objects = &objs
	if b.Impact(0, KindParty) != 1 || objs[0].ObjList == 0 {
		t.Fatal("a party bolt should keep flying through the party")
	}
	a := Actor{HP: 30, Info: tables.MonsterInfo{ID: 1, HP: 30, AC: -100}}
	b.Actors = []Actor{a}
	if b.Impact(0, KindActor) != 0 {
		t.Fatal("hit on an actor should stop the bolt")
	}
	if b.Actors[0].HP >= 30 {
		t.Fatalf("actor HP %d, want damage", b.Actors[0].HP)
	}

	objs = []Object{{Type: 1050, ObjList: 3, Pos: [3]int32{0, 0, 0}, Caster: KindParty, Spell: 2, SpellLevel: 4, SpellSkill: 1}}
	b.Objects = &objs
	b.Actors[0].HP = 30
	b.Actors[0].Pos = [3]int16{0x100, 0, 0}
	b.Actors[0].Radius = 0
	if b.Impact(0, 0) != 0 || objs[0].Type != 1051 {
		t.Fatalf("wall burst type %d obj %d", objs[0].Type, objs[0].ObjList)
	}
	if c.Classes == nil || len(c.Classes.BonusLimits) == 0 {
		t.Fatalf("classes %+v", c.Classes)
	}
	// Inside 0x200 (reach) + radius 0: the actor at 0x100 is hit. One at 0x300 is not.
	b.Strike()
	if b.Actors[0].HP >= 30 {
		t.Fatalf("inside the burst HP %d", b.Actors[0].HP)
	}
	objs = []Object{{Type: 1050, ObjList: 3, Pos: [3]int32{0, 0, 0}, Caster: KindParty, Spell: 2, SpellLevel: 1, SpellSkill: 1}}
	b.Objects = &objs
	b.Actors[0].HP = 30
	b.Actors[0].Pos = [3]int16{0x300, 0, 0}
	b.Impact(0, 0)
	b.Strike()
	if b.Actors[0].HP != 30 {
		t.Fatalf("outside the burst HP %d", b.Actors[0].HP)
	}
}

// An ice blast of normal mastery throws 7 shards; grandmaster throws 9. The parent
// is gone.
//
// mm8: 0x46bd3a (object 3090)
func TestIceBlastShards(t *testing.T) {
	b := impactBrain()
	for _, tc := range []struct{ skill, want int }{{1, 7}, {4, 9}} {
		objs := []Object{{Type: 3090, ObjList: 5, SpellSkill: int32(tc.skill), Yaw: 0}}
		b.Objects = &objs
		b.Impact(0, 0)
		n := 0
		for _, o := range *b.Objects {
			if o.ObjList != 0 && o.Type == 3092 {
				n++
			}
		}
		if n != tc.want {
			t.Errorf("skill %d shards %d, want %d", tc.skill, n, tc.want)
		}
	}
}

// A missile's lifetime calls impact with the object's own pid, and the object stops.
//
// mm8: 0x472b60
func TestLifetimeImpact(t *testing.T) {
	b := impactBrain()
	b.Party = Party{Height: 100, EyeLevel: 80}
	objs := []Object{{Type: 1020, ObjList: 1, Time: 100, Spell: 2, Caster: KindParty}}
	b.Objects = &objs
	b.MoveObjects(1)
	if objs[0].Type != 1021 && objs[0].ObjList != 0 {
		t.Fatalf("expired bolt type %d list %d", objs[0].Type, objs[0].ObjList)
	}
}

// Actor_LaunchMissile maps missile 1 to object 545 and tags the actor as caster.
//
// mm8: 0x404bc6
func TestLaunchMissile(t *testing.T) {
	b := impactBrain()
	b.Actors = []Actor{{Pos: [3]int16{10, 20, 30}, Height: 80, Info: tables.MonsterInfo{HP: 10}}}
	b.Objects = &[]Object{}
	b.LaunchMissile(0, Dir{Yaw: 100, Pitch: 0, Dist: 100}, 1, 0)
	objs := b.objects()
	if len(objs) != 1 || objs[0].Type != 545 || objs[0].Caster != int32(KindActor) || objs[0].ObjList == 0 {
		t.Fatalf("missile %+v", objs)
	}
}
