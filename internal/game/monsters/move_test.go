package monsters

import (
	"testing"

	"libre-enroth/internal/assets/tables"
	"libre-enroth/internal/game/party"
	"libre-enroth/internal/game/physics"
	"libre-enroth/internal/game/physics/physicstest"
)

const one = 0x10000

// rooms: sector 1 (0..1000 x 0..1000) opens east into sector 2 (1000..2000) through a
// portal; sector 3 (0..1000 x 1000..2000) is walled off from sector 1.
func rooms() *physics.IndoorGeo {
	b := physicstest.New()
	s1 := b.Sector(0, 1000, 0, 1000, 0, 500)
	s2 := b.Sector(1000, 2000, 0, 1000, 0, 500)
	s3 := b.Sector(0, 1000, 1000, 2000, 0, 500)
	b.Box(s1, 0, 1000, 0, 1000, 0, 500, "e")
	b.Box(s2, 1000, 2000, 0, 1000, 0, 500, "w")
	b.Box(s3, 0, 1000, 1000, 2000, 0, 500, "")
	b.Portal(s1, s2, [3]int32{-one, 0, 0}, [3]int16{1000, 0, 0}, [3]int16{1000, 0, 500}, [3]int16{1000, 1000, 500}, [3]int16{1000, 1000, 0})
	return &physics.IndoorGeo{Map: &b.M}
}

// indoorBrain is testBrain on an indoor map.
func indoorBrain(g *physics.IndoorGeo, px, py int32, actors ...Actor) *Brain {
	b := testBrain(1, px, py, actors...)
	b.Outdoor, b.Indoor = nil, g
	for i := range b.Actors {
		a := &b.Actors[i]
		a.Sector = int16(g.SectorAt(int32(a.Pos[0]), int32(a.Pos[1]), int32(a.Pos[2])))
	}
	return b
}

// Actor_CanSee walks the portals from the actor's sector to the party's; Map_CanSeePoints
// casts its two rays against the walls of both points' sectors.
//
// mm8: 0x407be3, 0x408520
func TestLineOfSight(t *testing.T) {
	g := rooms()
	mt := aiTables()
	b := indoorBrain(g, 1500, 500, monster(mt, 1, 500, 500))
	if !b.CanSee(PID(0), partyPID) {
		t.Error("through the portal")
	}
	b.Party.X, b.Party.Y = 500, 1500
	if b.CanSee(PID(0), partyPID) {
		t.Error("through the wall")
	}
	if !b.CanSeePoints(500, 500, 50, 1500, 500, 50) {
		t.Error("points through the portal")
	}
	if b.CanSeePoints(500, 500, 50, 500, 1500, 50) {
		t.Error("points through the wall")
	}
	b.Party.X, b.Party.Y = 500, 600
	if b.CanSee(PID(0), partyPID) != true {
		t.Error("same sector")
	}
	b.Party.Y = 500 + 0x1401
	b.Party.X = 500
	if b.CanSee(PID(0), partyPID) {
		t.Error("beyond 0x1400")
	}
}

// walker is a monster walking at speed 200 toward yaw.
func walker(mt *tables.Monsters, id int, x, y, z int16, yaw uint16) Actor {
	a := monster(mt, id, x, y)
	a.Pos[2] = z
	a.Speed, a.Yaw = 200, yaw
	a.AIState, a.Animation = Tethered, tables.AnimWalk
	return a
}

// Actors_MoveIndoor: a walker's velocity is speed along its yaw; one 2-tick frame moves
// it Mul16(65200, Mul16(201, 1024) = 3) = 2 (as the party's sweep, physics).
//
// mm8: 0x46fb6d, 0x47078b
func TestMoveIndoorWalk(t *testing.T) {
	g := rooms()
	mt := aiTables()
	b := indoorBrain(g, 1800, 900, walker(mt, 4, 200, 500, 0, 0))
	b.MoveIndoor(2)
	a := &b.Actors[0]
	if a.Pos != [3]int16{202, 500, 0} || a.Vel != [3]int16{200, 0, 0} || a.Sector != 1 {
		t.Errorf("after a frame: pos %v vel %v sector %d", a.Pos, a.Vel, a.Sector)
	}
	// It walks on through the portal into sector 2.
	for range 400 {
		b.MoveIndoor(2)
	}
	if a.Pos[0] <= 1000 || a.Sector != 2 {
		t.Errorf("through the portal: pos %v sector %d", a.Pos, a.Sector)
	}
}

// A standing monster in the air falls (5 * dt * 8 a frame) and lands on the floor.
func TestMoveIndoorFall(t *testing.T) {
	g := rooms()
	mt := aiTables()
	a := monster(mt, 4, 500, 500)
	a.Pos[2] = 300
	b := indoorBrain(g, 1800, 900, a)
	b.MoveIndoor(2)
	if v := b.Actors[0].Vel[2]; v != -80 {
		t.Errorf("first frame vz %d, want -80", v)
	}
	for range 100 {
		b.MoveIndoor(2)
	}
	if p, v := b.Actors[0].Pos, b.Actors[0].Vel; p[2] < 0 || p[2] > 1 || v[2] != 0 {
		t.Errorf("landed at %v vel %v", p, v)
	}
}

// A walker stops at a wall, its radius away.
func TestMoveIndoorWall(t *testing.T) {
	g := rooms()
	mt := aiTables()
	b := indoorBrain(g, 1800, 900, walker(mt, 4, 500, 500, 0, 1024)) // west
	for range 200 {
		b.MoveIndoor(2)
	}
	if x := b.Actors[0].Pos[0]; x < 30 || x > 40 {
		t.Errorf("at the west wall: x %d", x)
	}
}

// A monster hostile to the party that walks into it stops; a friendly one turns to it
// (Actor_FaceObject: state 10). The party's cylinder is twice its radius wide.
//
// mm8: 0x46fb6d (pid & 7 == 4), 0x46f14e, 0x404323
func TestMoveIntoParty(t *testing.T) {
	g := rooms()
	mt := aiTables()
	b := indoorBrain(g, 700, 500, walker(mt, 1, 300, 500, 0, 0))
	for range 300 {
		b.MoveIndoor(2)
	}
	if a := &b.Actors[0]; a.Pos[0] < 590 || a.Pos[0] > 597 || a.Vel[0] != 0 {
		t.Errorf("hostile: pos %v vel %v", a.Pos, a.Vel)
	}
	b = indoorBrain(g, 700, 500, walker(mt, 4, 300, 500, 0, 0))
	for range 300 {
		b.MoveIndoor(2)
		if b.Actors[0].AIState != Tethered {
			break
		}
	}
	if a := &b.Actors[0]; a.AIState != Interacting && a.AIState != Fidget && a.AIState != Stand {
		t.Errorf("friendly: state %d at %v", a.AIState, a.Pos)
	}
}

// obstacles lets the party's sweep meet the actors.
type obstacles struct{ actors []Actor }

func (o obstacles) Collide(s *physics.State) { CollideActors(s, o.actors) }
func (o obstacles) Bumped()                  {}

// The party walking into a monster stops at its radius; a peasant does not stop it
// unless it is hostile.
//
// mm8: 0x472e86 (the actor loop: not Actor_IsPeasantKind, or hostile), 0x46e143
func TestPartyBlocked(t *testing.T) {
	g := rooms()
	mt := aiTables()
	for _, c := range []struct {
		peasant, hostile bool
		stop             bool
	}{{false, false, true}, {true, false, false}, {true, true, true}} {
		a := monster(mt, 4, 500, 500) // class 2
		if c.peasant {
			a.Ally = 1 // class 1: a peasant kind
		}
		if c.hostile {
			a.Flags |= FlagHostile
		}
		p := party.New(200, 500, 1, 0)
		p.Obstacles = obstacles{[]Actor{a}}
		for range 200 {
			p.Actions.Push(party.Forward)
			p.MoveIndoor(g, 2)
		}
		if stopped := p.X < 500-30-0x25+5; stopped != c.stop {
			t.Errorf("peasant %v hostile %v: party at x %d", c.peasant, c.hostile, p.X)
		}
	}
}
