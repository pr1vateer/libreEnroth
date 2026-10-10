package monsters

import (
	"math"
	"testing"

	"libre-enroth/internal/assets/desc"
	"libre-enroth/internal/assets/tables"
	"libre-enroth/internal/game/party"
	"libre-enroth/internal/game/party/partytest"
	"libre-enroth/internal/game/physics"
	"libre-enroth/internal/maps/odm"
)

// aiTables: monsters 1..3 (class 1) hate the party (hostile.txt 4), monster 4 (class 2)
// does not; class 1 and 2 hate each other at 2.
func aiTables() *tables.Monsters {
	m := testTables()
	m.Hostile = make([]uint8, 4*tables.HostileStride)
	m.Hostile[1*tables.HostileStride+0] = 4
	m.Hostile[1*tables.HostileStride+2] = 2
	m.Hostile[2*tables.HostileStride+1] = 2
	for id := 1; id <= 4; id++ {
		in := &m.Infos[id]
		in.AI, in.Move, in.Level, in.AC = tables.AINormal, tables.MoveShort, 4, 5
		in.Attack1 = tables.Attack{Type: party.DamagePhysical, Dice: 2, Sides: 6, Add: 3}
		in.Recovery = 50
		in.Hostility = 2
		in.Speed = 300
	}
	return m
}

// aiSFT gives each animation a sequence of length 4 (32 ticks).
func aiSFT() *desc.SFT {
	s := testSFT()
	for i := range s.Frames {
		s.Frames[i].Length = 4
	}
	return s
}

// testBrain is an outdoor brain over an empty map with a one-member party (a knight
// with 35 HP) at (px, py, 0).
func testBrain(seed uint32, px, py int32, actors ...Actor) *Brain {
	m := &party.Members{Players: []party.Player{{Class: 4, Face: 0}}}
	c := &party.Ctx{Rand: party.NewRand(seed), Items: partytest.Items(), Classes: partytest.Classes()}
	m.DefaultMember(0, c)
	b := &Brain{
		Tables: aiTables(), SFT: aiSFT(), Rand: c.Rand,
		Outdoor: &physics.OutdoorGeo{Map: &odm.Map{}},
		Party:   Party{X: px, Y: py, Height: 0xc0, EyeLevel: 0xa0, Radius: 0x25, Members: m, Ctx: c},
		Actors:  actors,
	}
	return b
}

// monster is a standing actor of monster id at (x, y, 0).
func monster(t *tables.Monsters, id int, x, y int16) Actor {
	var a Actor
	a.Init()
	a.Info = t.Infos[id]
	a.Info.Hostility = 0
	a.HP = int16(a.Info.HP)
	a.Pos, a.Start = [3]int16{x, y, 0}, [3]int16{x, y, 0}
	a.Radius, a.Height, a.Speed = 30, 100, 300
	for k := range a.Sprites {
		a.Sprites[k] = int16(k + 1)
	}
	a.Flags |= FlagAnimationSet
	return a
}

// mm8: 0x46155d: max + mid * 11/32 + min / 4.
func TestApproxDist(t *testing.T) {
	if d := approxDist(300, -400, 32); d != 400+300*11/32+8 {
		t.Errorf("approxDist = %d", d)
	}
}

// Actor_GetDirectionInfo from an actor 100 high at the origin (aimed at z 75) to the
// party at (300, 400, 0) at its eyes (0xa0): dx 300, dy 400, dz 85, length
// sqrt(257225) = 507.17.
//
// mm8: 0x4043dc
func TestDirection(t *testing.T) {
	mt := aiTables()
	b := testBrain(1, 300, 400, monster(mt, 1, 0, 0))
	d := b.Direction(PID(0), partyPID, 0)
	l := math.Sqrt(257225)
	want := Dir{V: [3]int32{int32(300 / l * 65536), int32(400 / l * 65536), int32(85 / l * 65536)},
		Dist: 507, DistXZ: 500, Yaw: physics.Atan2(300, 400), Pitch: physics.Atan2(500, 85), From: PID(0), To: partyPID}
	if d != want {
		t.Errorf("direction %+v, want %+v", d, want)
	}
	// A preferred height replaces the eye level; the same point gives the unit x.
	if d := b.Direction(PID(0), partyPID, 75-0+0); d.V[2] != 0 {
		t.Errorf("prefZ 75: %+v", d)
	}
	b.Party.X, b.Party.Y = 0, 0
	if d := b.Direction(PID(0), partyPID, 75); d.V != [3]int32{0x10000, 0, 0} || d.Dist != 1 || d.DistXZ != 1 || d.Yaw != 0 {
		t.Errorf("same point: %+v", d)
	}
}

// Actor_FindTarget: the nearest foe in sight within the radius of the relation (class 1
// against class 2: 2 -> 0xa00), the party when nearer (its relation 4 -> 0x2800); an
// invisible party is no target; a monster already hostile looks as far as its
// monsters.txt hostility (2 -> 0xa00).
//
// mm8: 0x401237
func TestFindTarget(t *testing.T) {
	mt := aiTables()
	b := testBrain(1, 5000, 0, monster(mt, 1, 0, 0), monster(mt, 4, 1000, 0), monster(mt, 4, 3000, 0))
	if got := b.FindTarget(0, true); got != PID(1) {
		t.Errorf("target %#x, want actor 1", got)
	}
	b.Party.X = 500
	if got := b.FindTarget(0, true); got != partyPID {
		t.Errorf("target %#x, want the party", got)
	}
	b.Party.Members.Buffs[party.PartyBuffInvisibility].Expires = 1
	if got := b.FindTarget(0, true); got != PID(1) {
		t.Errorf("invisible party: target %#x, want actor 1", got)
	}
	b.Party.Members.Buffs[party.PartyBuffInvisibility].Expires = 0
	b.Actors[1].Pos[0] = 2600 // beyond 0xa00
	b.Party.X = 20000
	if got := b.FindTarget(0, true); got != 0 {
		t.Errorf("out of reach: target %#x", got)
	}
	// The actor that hit it last is a foe at 4 (0x2800) unless of its kind.
	b.Actors[0].LastHitBy = int32(PID(2))
	if got := b.FindTarget(0, false); got != PID(2) {
		t.Errorf("last hit: target %#x, want actor 2", got)
	}
}

// think runs actor 0's turn of the AI.
func think(b *Brain) {
	b.listOutdoor()
	b.think(b.list[0], 2)
}

// Actors_UpdateAI's decisions: a monster that hates the party turns hostile within
// 0x1400 and pursues it from afar (45 degrees off, at most 128 ticks), closes in
// straight within 0x400, and attacks within 307.2 (the melee animation's 32 ticks,
// recovery 50 * 2.1333 = 106); afraid and wimpy monsters flee; a weak normal one flees
// below 20 % of its HP.
//
// mm8: 0x401ad8, 0x4027d6, 0x40296a, 0x403f4e, 0x402ab6
func TestDecisions(t *testing.T) {
	mt := aiTables()
	b := testBrain(1, 2000, 0, monster(mt, 1, 0, 0))
	a := &b.Actors[0]
	think(b)
	if a.Info.Hostility != 4 || a.AIState != Pursue || a.ActionLength != 0x80 || a.Yaw != 0x100 && a.Yaw != 0xff00 {
		t.Errorf("far: hostility %d state %d length %d yaw %#x", a.Info.Hostility, a.AIState, a.ActionLength, a.Yaw)
	}
	b = testBrain(1, 600, 0, monster(mt, 1, 0, 0))
	a = &b.Actors[0]
	think(b)
	// d = 609 - 30 = 579 < 0x400: straight on, distXZ 600 * 128 / 300 = 256 -> 32.
	if a.AIState != Pursue || a.ActionLength != 0x20 || a.Yaw != 0 {
		t.Errorf("near: state %d length %d yaw %#x", a.AIState, a.ActionLength, a.Yaw)
	}
	b = testBrain(1, 200, 0, monster(mt, 1, 0, 0))
	a = &b.Actors[0]
	a.Info.Recovery = 0 // spawned monsters keep monsters.txt's Rec: they wait it out first
	think(b)
	if a.AIState != Melee || a.ActionLength != 32 || a.Info.Recovery != 106 || a.Animation != tables.AnimAttack {
		t.Errorf("melee: state %d length %d recovery %d anim %d", a.AIState, a.ActionLength, a.Info.Recovery, a.Animation)
	}
	// The blow is queued when the animation is over.
	a.ActionTime = 32
	a.Info.Recovery = 0
	b.think(0, 2)
	if b.Pending() != 1 || b.attacks.a[0].pid != PID(0) || b.attacks.a[0].reach != 0x1400 || b.attacks.a[0].z != 50 {
		t.Errorf("blow: %d queued %+v", b.Pending(), b.attacks.a[0])
	}

	for _, c := range []struct {
		name string
		set  func(a *Actor)
	}{
		{"afraid", func(a *Actor) { a.Buffs[BuffAfraid].Expires = 1 << 40 }},
		{"wimp", func(a *Actor) { a.Info.AI = tables.AIWimp }},
		{"hurt", func(a *Actor) { a.HP = 1 }},
	} {
		b = testBrain(1, 1500, 0, monster(mt, 1, 0, 0))
		a = &b.Actors[0]
		c.set(a)
		think(b)
		if a.AIState != Flee {
			t.Errorf("%s: state %d, want flee", c.name, a.AIState)
		}
	}
	// A hurt aggressive monster fights on above 10 %.
	b = testBrain(1, 1500, 0, monster(mt, 1, 0, 0))
	a = &b.Actors[0]
	a.Info.AI, a.HP = tables.AIAggressive, 2
	think(b)
	if a.AIState != Pursue {
		t.Errorf("aggressive: state %d", a.AIState)
	}
}

// Actor_RandomMove: a friendly monster without a foe walks toward its start, give or
// take 0x80, for (the way home + radius * (rand & 15) / 16) * 32 / speed ticks; a
// quarter of the time it stands 128..255 ticks instead.
//
// mm8: 0x403537, 0x404153
func TestTether(t *testing.T) {
	mt := aiTables()
	for seed := uint32(1); seed < 40; seed++ {
		b := testBrain(seed, 3000, 0, monster(mt, 4, 1000, 0))
		a := &b.Actors[0]
		a.Start = [3]int16{0, 0, 0}
		ref := party.NewRand(seed)
		think(b)
		r := ref.Int() & 0xf
		dist := int32(1000) + physics.Mul16(0x400, int32(r)<<12)
		if ref.Int()%100 < 25 {
			if a.AIState != Stand || a.ActionLength != uint16(ref.Int()%0x80+0x80) {
				t.Errorf("seed %d: short stand: state %d length %d", seed, a.AIState, a.ActionLength)
			}
			continue
		}
		yaw := physics.Atan2(-1000, 0) - 0x80 + int32(ref.Int()%0x100)
		if a.AIState != Tethered || a.Yaw != uint16(yaw) || a.ActionLength != uint16((dist<<5)/300) {
			t.Errorf("seed %d: state %d yaw %d length %d, want yaw %d length %d", seed, a.AIState, a.Yaw, a.ActionLength, yaw, (dist<<5)/300)
		}
	}
}

// Actor_ChooseAttack: a healing spell is only usable when hurt; its Use% roll comes
// first, then the second attack's Att% roll.
//
// mm8: 0x425cd8, 0x425db4
func TestChooseAttack(t *testing.T) {
	mt := aiTables()
	for seed := uint32(1); seed < 30; seed++ {
		b := testBrain(seed, 0, 0, monster(mt, 1, 0, 0))
		a := &b.Actors[0]
		a.Info.Spell1, a.Info.Spell1Chance, a.Info.Attack2Chance = 0x44, 50, 30
		ref := party.NewRand(seed)
		want := 0
		if ref.Int()%100 < 30 {
			want = 1
		}
		if got := b.ChooseAttack(0, 100); got != want {
			t.Errorf("seed %d, full HP: %d, want %d", seed, got, want)
		}
		a.HP--
		ref = party.NewRand(seed + 1000)
		b.Rand = party.NewRand(seed + 1000)
		switch {
		case ref.Int()%100 < 50:
			want = 2
		case ref.Int()%100 < 30:
			want = 1
		default:
			want = 0
		}
		if got := b.ChooseAttack(0, 100); got != want {
			t.Errorf("seed %d, hurt: %d, want %d", seed, got, want)
		}
	}
	b := testBrain(1, 0, 0, monster(mt, 1, 0, 0))
	if !b.SpellUsable(&b.Actors[0], 0x34, 0x180) || b.SpellUsable(&b.Actors[0], 0x34, 0x181) {
		t.Error("Spirit Lash reaches 0x180")
	}
	b.Actors[0].Buffs[BuffHaste].Expires = 5
	if b.SpellUsable(&b.Actors[0], 5, 0) {
		t.Error("Haste while hasted")
	}
}

// Actor_HitMember: the to-hit roll (AC + 5 < rand() % (AC + 10 + level * 2) + 1), the
// hit sound's roll, 2d6 + 3 physical damage through ReceiveDamage, the recovery
// (20 - StatToBonus(Endurance 11) = 19) * 2.1333 = 40 ticks.
//
// mm8: 0x438625, 0x4260f3, 0x439b44
func TestHitMember(t *testing.T) {
	mt := aiTables()
	hits, misses := 0, 0
	for seed := uint32(1); seed < 40; seed++ {
		b := testBrain(seed, 0, 0, monster(mt, 1, 0, 0))
		m := b.Party.Members
		p := &m.Players[0]
		ac := int32(p.AC(m.Env(b.Party.Ctx)))
		hp := p.HP
		ref := party.NewRand(seed)
		b.HitMember(0, AttackFirst, 0)
		if ac+5 >= int32(ref.Int())%(ac+10+8)+1 {
			misses++
			if p.HP != hp || p.Recovery != 0 {
				t.Errorf("seed %d: missed but HP %d recovery %d", seed, p.HP, p.Recovery)
			}
			continue
		}
		hits++
		ref.Int()
		dmg := int32(3 + 2 + ref.Int()%6 + ref.Int()%6)
		if p.HP != hp-dmg || p.Recovery != 40 {
			t.Errorf("seed %d: HP %d -> %d recovery %d, want -%d, 40", seed, hp, p.HP, p.Recovery, dmg)
		}
	}
	if hits == 0 || misses == 0 {
		t.Errorf("%d hits, %d misses", hits, misses)
	}
}

// Actor_ResistDamage: 30 + resistance; each roll of 30 or more halves the damage, up to
// four times; immune takes nothing.
//
// mm8: 0x4261b5
func TestResistDamage(t *testing.T) {
	mt := aiTables()
	b := testBrain(7, 0, 0, monster(mt, 1, 0, 0))
	a := &b.Actors[0]
	a.Info.Resist[0] = 20
	ref := party.NewRand(7)
	want := int32(64)
	for k := 1; k <= 4; k++ {
		if ref.Int()%50 <= 29 {
			break
		}
		want = 64 >> k
	}
	if got := b.ResistDamage(a, 0, 64); got != want {
		t.Errorf("fire 64 -> %d, want %d", got, want)
	}
	a.Info.Resist[9] = tables.Immune
	if got := b.ResistDamage(a, party.DamagePhysical, 64); got != 0 {
		t.Errorf("immune: %d", got)
	}
}

// Actor_HitActor: a blow that gets through stuns the target, turns its kind hostile
// and pushes it back; a target's HP at 0 starts its death.
//
// mm8: 0x43990b, 0x40332f, 0x439278, 0x402ec9
func TestHitActor(t *testing.T) {
	mt := aiTables()
	for seed := uint32(1); seed < 30; seed++ {
		b := testBrain(seed, 20000, 0, monster(mt, 1, 0, 0), monster(mt, 4, 100, 0), monster(mt, 4, 200, 0))
		tgt := &b.Actors[1]
		hp := tgt.HP
		b.HitActor(PID(0), 1, normalize(100, 0, 0), AttackFirst)
		switch {
		case tgt.HP == hp:
			if tgt.LastHitBy != int32(PID(0)) || tgt.AIState == Stunned {
				t.Errorf("seed %d: miss: last hit %#x state %d", seed, tgt.LastHitBy, tgt.AIState)
			}
		case tgt.HP > 0:
			if tgt.AIState != Stunned || tgt.Info.Hostility != 4 || b.Actors[2].Info.Hostility != 4 || tgt.Vel[0] <= 0 {
				t.Errorf("seed %d: hit: state %d hostility %d ally %d vel %v", seed, tgt.AIState, tgt.Info.Hostility,
					b.Actors[2].Info.Hostility, tgt.Vel)
			}
		default:
			if tgt.AIState != Dying || tgt.HP != 0 {
				t.Errorf("seed %d: killed: state %d HP %d", seed, tgt.AIState, tgt.HP)
			}
		}
	}
}

// Actors_HostileNear: an actor alive within 0x1400 (0xa00 indoors) that hates the
// party keeps it from resting.
//
// mm8: 0x42e9d7
func TestHostileNear(t *testing.T) {
	mt := aiTables()
	acts := []Actor{monster(mt, 4, 1000, 0), monster(mt, 1, 7000, 0)}
	if HostileNear(mt, acts, 0, 0, 0, false) {
		t.Error("a friend is no threat")
	}
	if !HostileNear(mt, acts, 4000, 0, 0, false) || HostileNear(mt, acts, 4000, 0, 0, true) {
		t.Error("the foe 3000 away")
	}
	acts[1].AIState = Dead
	if HostileNear(mt, acts, 4000, 0, 0, false) {
		t.Error("a dead foe")
	}
}
