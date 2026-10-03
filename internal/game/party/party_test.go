package party

import (
	"testing"

	"libre-enroth/internal/game/physics"
	"libre-enroth/internal/game/physics/physicstest"
	"libre-enroth/internal/maps/blv"
	"libre-enroth/internal/maps/odm"
)

const one = 0x10000

// room is a 4096 x 4096 indoor room, floor at 0, ceiling at 2048.
func room() *physics.IndoorGeo {
	b := physicstest.New()
	s := b.Sector(0, 4096, 0, 4096, 0, 2048)
	b.Box(s, 0, 4096, 0, 4096, 0, 2048, "")
	return &physics.IndoorGeo{Map: &b.M}
}

func frames(n int, p *Party, act Action, move func()) {
	for range n {
		if act != 0 {
			p.Actions.Push(act)
		}
		move()
	}
}

// mm8: 0x472e86 (Party_MoveIndoor), 0x47078b (Collide_Begin). Walking east at 0x180:
// vx = 384, speed 385, dir.x = 170 * 384 = 65280; a 2-tick frame moves
// 65280 * (385 * 1024 >> 16 = 6) >> 16 = 5 units, so a second of 2-tick frames covers
// 64 * 5 = 320; in 32-tick frames 65280 * 96 >> 16 = 95 each, 380 a second.
func TestWalkFlat(t *testing.T) {
	g := room()
	p := New(1000, 2000, 1, 0)
	frames(64, p, Forward, func() { p.MoveIndoor(g, 2) })
	if p.X != 1320 || p.Y != 2000 || p.Z != 1 {
		t.Errorf("after 64 2-tick frames: (%d, %d, %d), want (1320, 2000, 1)", p.X, p.Y, p.Z)
	}
	p = New(1000, 2000, 1, 0)
	frames(4, p, Forward, func() { p.MoveIndoor(g, 32) })
	if p.X != 1380 {
		t.Errorf("after 4 32-tick frames: x %d, want 1380", p.X)
	}
	// Running doubles the speed: vx = 768, speed 769, dir 85*768 = 65280, moveDist
	// 769*1024 >> 16 = 12, step 65280*12 >> 16 = 11.
	p = New(1000, 2000, 1, 0)
	frames(1, p, RunForward, func() { p.MoveIndoor(g, 2) })
	if p.X != 1011 {
		t.Errorf("run: x %d, want 1011", p.X)
	}
	// Turning: 90 deg/s = (0x400 * 0x5a / 180) * dtFixed >> 16 = 512 * 1024 >> 16 = 8 per
	// 2-tick frame.
	p = New(1000, 2000, 1, 0)
	frames(3, p, TurnLeft, func() { p.MoveIndoor(g, 2) })
	if p.Dir != 24 {
		t.Errorf("turn: dir %d, want 24", p.Dir)
	}
	p.TurnDelta = 0x40
	frames(1, p, TurnRight, func() { p.MoveIndoor(g, 2) })
	if p.Dir != 24-0x40+2048 {
		t.Errorf("turn delta: dir %d", p.Dir)
	}
}

// mm8: 0x472e86. The jump sets vz = (5 << 6) * 1.5 = 480; gravity then takes 2 * 5 * dt
// a frame while airborne. Frame 1: vz 460, speed 461, dir.z 142 * 460 = 65320,
// moveDist 461 * 1024 >> 16 = 7, rise 65320 * 7 >> 16 = 6. Frame 2: vz 440, dir 148 *
// 440 = 65120, moveDist 6, rise 5. Frame 3: vz 420, dir 155 * 420 = 65100, rise 5.
func TestJumpArc(t *testing.T) {
	g := room()
	p := New(1000, 2000, 1, 0)
	want := [][2]int32{{7, 460}, {12, 440}, {17, 420}}
	for i, w := range want {
		if i == 0 {
			p.Actions.Push(Jump)
		}
		p.MoveIndoor(g, 2)
		if p.Z != w[0] || p.VZ != w[1] {
			t.Errorf("frame %d: z %d vz %d, want %v", i+1, p.Z, p.VZ, w)
		}
	}
	top := p.Z
	for i := 0; i < 200 && (p.Z > 1 || p.VZ != 0); i++ {
		p.MoveIndoor(g, 2)
		top = max(top, p.Z)
	}
	if p.Z != 1 || p.VZ != 0 {
		t.Errorf("landed at z %d vz %d, want 1 and 0", p.Z, p.VZ)
	}
	// Frozen: about 23 rising frames of vz/64 each, minus a unit of truncation per frame.
	if top != 56 {
		t.Errorf("peak %d, want 56", top)
	}
	if p.Flags&FlagAirborne != 0 {
		t.Error("still flagged airborne")
	}
}

// step builds a room whose floor rises by h at x 2000, with the riser facing west.
func step(h int16) *physics.IndoorGeo {
	b := physicstest.New()
	s := b.Sector(0, 4096, 0, 4096, 0, 2048)
	b.Face(s, blv.PolyFloor, 0, [3]int32{0, 0, one}, [3]int16{0, 0, 0}, [3]int16{2000, 0, 0}, [3]int16{2000, 4096, 0}, [3]int16{0, 4096, 0})
	b.Face(s, blv.PolyFloor, 0, [3]int32{0, 0, one}, [3]int16{2000, 0, h}, [3]int16{4096, 0, h}, [3]int16{4096, 4096, h}, [3]int16{2000, 4096, h})
	b.Face(s, blv.PolyWall, 0, [3]int32{-one, 0, 0}, [3]int16{2000, 0, 0}, [3]int16{2000, 0, h}, [3]int16{2000, 4096, h}, [3]int16{2000, 4096, 0})
	b.Box(s, 0, 4096, 0, 4096, 0, 2048, "")
	b.M.Faces[3].Attr |= physics.AttrEthereal // the box's own floor: the two above replace it
	return &physics.IndoorGeo{Map: &b.M}
}

// mm8: 0x472e86, 0x46d0d0. A riser is climbed when the lower sphere's contact point
// (its centre, radius + 1 = 0x26 above the feet) passes over it: the floor lookup at
// z + 0x28 then finds the higher floor and the next frame stands on it. Higher risers
// stop the party at its radius.
func TestStepUp(t *testing.T) {
	for _, c := range []struct {
		h     int16
		climb bool
	}{{0x10, true}, {0x20, true}, {0x25, true}, {0x27, false}, {0x40, false}} {
		g := step(c.h)
		p := New(1800, 2000, 1, 0)
		frames(120, p, Forward, func() { p.MoveIndoor(g, 2) })
		if c.climb {
			if p.X <= 2000 || p.Z != int32(c.h)+1 {
				t.Errorf("riser %#x: at (%d, z %d), want past x 2000 on z %d", c.h, p.X, p.Z, c.h+1)
			}
		} else if p.X > 2000-int32(p.Radius) || p.Z != 1 {
			t.Errorf("riser %#x: at (%d, z %d), want stopped before x %d", c.h, p.X, p.Z, 2000-p.Radius)
		}
	}
}

// mm8: 0x472e86 (the wall branch of the face hit: push the velocity out along the
// normal, then the party out to its radius). Walking north-east into a wall along y
// 3000 slides east along it.
func TestWallSlide(t *testing.T) {
	g := room()
	b := physicstest.Builder{M: *g.Map}
	b.Face(1, blv.PolyWall, 0, [3]int32{0, -one, 0}, [3]int16{0, 3000, 0}, [3]int16{4096, 3000, 0}, [3]int16{4096, 3000, 2048}, [3]int16{0, 3000, 2048})
	g = &physics.IndoorGeo{Map: &b.M}
	p := New(1000, 2900, 1, 256)
	frames(40, p, Forward, func() { p.MoveIndoor(g, 2) })
	x0 := p.X
	if p.Y > 3000-p.Radius || p.Y < 3000-p.Radius-8 {
		t.Errorf("y %d, want held against the wall at %d", p.Y, 3000-p.Radius)
	}
	frames(20, p, Forward, func() { p.MoveIndoor(g, 2) })
	if p.X-x0 < 20*2 {
		t.Errorf("slid %d east in 20 frames, want at least 40", p.X-x0)
	}
	if p.Y > 3000-p.Radius {
		t.Errorf("went through the wall: y %d", p.Y)
	}
}

// mm8: 0x46f29b (Collide_Portals), 0x46e663. Two rooms joined by a portal at x 2000:
// walking east crosses it and the party's sector changes.
func TestPortalCrossing(t *testing.T) {
	b := physicstest.New()
	a := b.Sector(0, 2000, 0, 4096, 0, 1024)
	c := b.Sector(2000, 4096, 0, 4096, 0, 1024)
	b.Box(a, 0, 2000, 0, 4096, 0, 1024, "e")
	b.Box(c, 2000, 4096, 0, 4096, 0, 1024, "w")
	b.Portal(a, c, [3]int32{-one, 0, 0}, [3]int16{2000, 0, 0}, [3]int16{2000, 0, 1024}, [3]int16{2000, 4096, 1024}, [3]int16{2000, 4096, 0})
	g := &physics.IndoorGeo{Map: &b.M}
	p := New(1900, 2000, 1, 0)
	p.MoveIndoor(g, 2)
	if p.Sector != a {
		t.Fatalf("start sector %d, want %d", p.Sector, a)
	}
	frames(60, p, Forward, func() { p.MoveIndoor(g, 2) })
	if p.X <= 2100 || p.Sector != c || p.Z != 1 {
		t.Errorf("at x %d sector %d z %d, want past 2100 in sector %d", p.X, p.Sector, p.Z, c)
	}
}

// flatOutdoor is an all-zero terrain without models or water.
func flatOutdoor(planeOfWater bool) *physics.OutdoorGeo {
	g := physics.NewOutdoorGeo(&odm.Map{}, func(int, int) uint16 { return 0 }, nil)
	g.PlaneOfWater = planeOfWater
	return g
}

// mm8: 0x473fee, 0x46bab0 (Physics_Gravity: 5, or 1 on the Plane of Water). Falling
// from 500: one 2-tick frame takes vz to -2 * 5 * 2 = -20, or -4 on the Plane of Water.
func TestOutdoorGravity(t *testing.T) {
	for _, c := range []struct {
		water bool
		vz    int32
	}{{false, -20}, {true, -4}} {
		p := New(0, 0, 500, 0)
		p.MoveOutdoor(flatOutdoor(c.water), 2)
		if p.VZ != c.vz {
			t.Errorf("plane of water %v: vz %d, want %d", c.water, p.VZ, c.vz)
		}
	}
	// It lands on the terrain: sinking below the floor puts the party at floor + 1.
	g := flatOutdoor(false)
	p := New(0, 0, 500, 0)
	for range 200 {
		p.MoveOutdoor(g, 2)
	}
	if p.Z != 1 || p.VZ != 0 {
		t.Errorf("landed at z %d vz %d, want 1 0", p.Z, p.VZ)
	}
}

// mm8: 0x473fee. On flat terrain the walk matches indoors: 5 units per 2-tick frame.
// Flying (the fly buff) rises 0x1e per fly-up action and runs at 4x.
func TestOutdoorWalkFly(t *testing.T) {
	g := flatOutdoor(false)
	p := New(0, 0, 0, 0)
	frames(64, p, Forward, func() { p.MoveOutdoor(g, 2) })
	if p.X != 320 || p.Z != 0 {
		t.Errorf("walk: x %d z %d, want 320 0", p.X, p.Z)
	}
	p.Actions.Push(FlyUp)
	p.MoveOutdoor(g, 2)
	if p.Z != 0 {
		t.Errorf("fly up without the buff: z %d", p.Z)
	}
	p.Fly = true
	for range 3 {
		p.Actions.Push(FlyUp)
		p.MoveOutdoor(g, 2)
	}
	if !p.Flying || p.FlyBaseZ != 3*flyStep {
		t.Errorf("flying %v base %d, want %d", p.Flying, p.FlyBaseZ, 3*flyStep)
	}
	p.Actions.Push(Land)
	p.MoveOutdoor(g, 2)
	for range 100 {
		p.MoveOutdoor(g, 2)
	}
	if p.Flying || p.Z != 1 {
		t.Errorf("after landing: flying %v z %d, want on the ground at 1", p.Flying, p.Z)
	}
}

// mm8: 0x42efd9 (Input_GameKeys): Shift xor AlwaysRun runs, Ctrl turns into strafing.
func TestKeys(t *testing.T) {
	for _, c := range []struct {
		held             []Binding
		shift, ctrl, run bool
		want             []Action
	}{
		{[]Binding{BindForward}, false, false, false, []Action{Forward}},
		{[]Binding{BindForward}, true, false, false, []Action{RunForward}},
		{[]Binding{BindForward}, true, false, true, []Action{Forward}},
		{[]Binding{BindBackward, BindLeft}, false, false, true, []Action{RunBack, RunTurnLeft}},
		{[]Binding{BindLeft, BindRight}, false, true, false, []Action{StrafeLeft, StrafeRight}},
		{[]Binding{BindJump, BindFlyUp, BindLookUp}, false, false, false, []Action{Jump, LookUp, FlyUp}},
	} {
		p := New(0, 0, 0, 0)
		p.Keys(fakeKeys{c.held, c.shift, c.ctrl}, c.run)
		var got []Action
		for p.Actions.Len() > 0 {
			got = append(got, p.Actions.Pop())
		}
		if len(got) != len(c.want) {
			t.Errorf("%v: %v, want %v", c.held, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("%v: %v, want %v", c.held, got, c.want)
				break
			}
		}
	}
}

type fakeKeys struct {
	held        []Binding
	shift, ctrl bool
}

func (k fakeKeys) Active(b Binding) bool {
	for _, h := range k.held {
		if h == b {
			return true
		}
	}
	return false
}
func (k fakeKeys) Shift() bool { return k.shift }
func (k fakeKeys) Ctrl() bool  { return k.ctrl }
