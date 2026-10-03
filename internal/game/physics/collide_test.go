package physics_test

import (
	"testing"

	"libre-enroth/internal/game/physics"
	"libre-enroth/internal/game/physics/physicstest"
	"libre-enroth/internal/maps/blv"
)

const one = 0x10000

// floorGeo is a 1000x1000 room: floor at z 0, ceiling at 1000.
func floorGeo() (*physics.IndoorGeo, int) {
	b := physicstest.New()
	s := b.Sector(0, 1000, 0, 1000, 0, 1000)
	b.Box(s, 0, 1000, 0, 1000, 0, 1000, "")
	return &physics.IndoorGeo{Map: &b.M}, s
}

// mm8: 0x475e0b (Collide_SphereFace). A sphere of radius 37 at z 100 moving straight
// down: d = (37 - 100) << 16, the plane is reached after t = d / (n.dir) = 63.0, and
// the contact point (500, 500, 100 - 63 - 37) = (500, 500, 0) is inside the floor.
func TestSphereFace(t *testing.T) {
	g, _ := floorGeo()
	floor := g.Poly(0)
	dist, ok := physics.SphereFace(37, [3]int32{500, 500, 100}, [3]int32{0, 0, -one}, &floor, false, false)
	if !ok || dist != 63 {
		t.Errorf("sphere down onto the floor: %d %v, want 63", dist, ok)
	}
	// Within 2r of the plane the sphere counts as touching: distance 0 at once.
	if dist, ok := physics.SphereFace(37, [3]int32{500, 500, 50}, [3]int32{0, 0, -one}, &floor, false, false); !ok || dist != 0 {
		t.Errorf("touching: %d %v, want 0", dist, ok)
	}
	// Outside the face's outline: no hit.
	if _, ok := physics.SphereFace(37, [3]int32{1500, 500, 100}, [3]int32{0, 0, -one}, &floor, false, false); ok {
		t.Error("hit outside the floor")
	}
	// Moving parallel to the plane from far away: |d| >> 14 > |n.dir| rejects it.
	if _, ok := physics.SphereFace(37, [3]int32{500, 500, 100}, [3]int32{one, 0, 0}, &floor, false, false); ok {
		t.Error("parallel move hit the floor")
	}
	floor.Attr |= physics.AttrEthereal
	if _, ok := physics.SphereFace(37, [3]int32{500, 500, 100}, [3]int32{0, 0, -one}, &floor, true, false); ok {
		t.Error("ethereal floor hit")
	}
}

// mm8: 0x4768a0 (Collide_RayFace). From (500, 500, 100) straight down the floor is 100
// away: s = -(n.p + d) = -100 << 16, t = s / (n.dir) = 100.0; the rounded contact
// (500, 500, 0) is inside. Rays from behind a solid face miss.
func TestRayFace(t *testing.T) {
	g, _ := floorGeo()
	floor := g.Poly(0)
	if d, ok := physics.RayFace([3]int32{500, 500, 100}, [3]int32{0, 0, -one}, 100, &floor); !ok || d != 100 {
		t.Errorf("ray: %d %v, want 100", d, ok)
	}
	if _, ok := physics.RayFace([3]int32{500, 500, 100}, [3]int32{0, 0, -one}, 99, &floor); ok {
		t.Error("ray beyond its length hit")
	}
	if _, ok := physics.RayFace([3]int32{500, 500, -100}, [3]int32{0, 0, -one}, 1000, &floor); ok {
		t.Error("ray from behind hit")
	}
	if _, ok := physics.RayFace([3]int32{500, 500, -100}, [3]int32{0, 0, one}, 1000, &floor); ok {
		t.Error("ray from behind towards the back side hit a solid face")
	}
	floor.Attr |= physics.AttrPortal
	if d, ok := physics.RayFace([3]int32{500, 500, -100}, [3]int32{0, 0, one}, 1000, &floor); !ok || d != 100 {
		t.Errorf("portal from behind: %d %v, want 100", d, ok)
	}
}

// mm8: 0x476178 (Face_ContainsPoint: one crossing of the +u ray = inside)
func TestContainsPoint(t *testing.T) {
	g, _ := floorGeo()
	floor := g.Poly(0) // x/y plane
	for _, c := range []struct {
		x, y int16
		in   bool
	}{{500, 500, true}, {1, 999, true}, {-1, 500, false}, {1001, 500, false}, {500, 1001, false}, {500, -5, false}} {
		if got := floor.ContainsPoint(c.x, c.y, 0); got != c.in {
			t.Errorf("floor contains (%d, %d) = %v", c.x, c.y, got)
		}
	}
	west := g.Poly(2) // the x = 0 wall: y/z plane
	if !west.ContainsPoint(0, 500, 500) || west.ContainsPoint(0, 500, 1500) {
		t.Error("west wall containment")
	}
	south := g.Poly(4) // the y = 0 wall: x/z plane
	if !south.ContainsPoint(500, 0, 500) || south.ContainsPoint(1500, 0, 500) {
		t.Error("south wall containment")
	}
}

// mm8: 0x47078b (Collide_Begin). 384 units/s along x for 2 ticks: speed 384 | 1 = 385,
// dir.x = (0x10000 / 385) * 384 = 65280, moveDist = 385 * 1024 >> 16 = 6, and the
// end point is 65280 * 6 >> 16 = 5 further.
func TestBegin(t *testing.T) {
	s := physics.State{RadiusLo: 37, RadiusHi: 18, Velocity: [3]int32{384, 0, 0},
		PosLo: [3]int32{100, 100, 38}, PosHi: [3]int32{100, 100, 161}}
	if !s.Begin(1024) {
		t.Fatal("Begin: nothing to move")
	}
	if s.Speed != 385 || s.Dir != [3]int32{65280, 0, 0} || s.MoveDist != 6 || s.NewPosLo[0] != 105 || s.NewPosHi[2] != 161 {
		t.Errorf("speed %d dir %v moveDist %d newLo %v newHi %v", s.Speed, s.Dir, s.MoveDist, s.NewPosLo, s.NewPosHi)
	}
	if s.BBox != [6]int32{63, 142, 63, 137, 1, 179} || s.AdjustedDist != physics.NoHit || s.LastPortal != -1 {
		t.Errorf("bbox %v adjusted %d lastPortal %d", s.BBox, s.AdjustedDist, s.LastPortal)
	}
	s.TotalMoved = 6
	if s.Begin(1024) {
		t.Error("Begin moved past the frame's distance")
	}
}

// mm8: 0x46d0d0 (Indoor_FloorZ): the highest floor at most 5 above z, else the lowest;
// none = -30000.
func TestIndoorFloorZ(t *testing.T) {
	b := physicstest.New()
	s := b.Sector(0, 1000, 0, 1000, 0, 1000)
	b.Box(s, 0, 1000, 0, 1000, 0, 1000, "")
	// a raised platform at z 200 over x 400..600
	b.Face(s, blv.PolyFloor, 0, [3]int32{0, 0, one}, [3]int16{400, 0, 200}, [3]int16{600, 0, 200}, [3]int16{600, 1000, 200}, [3]int16{400, 1000, 200})
	g := &physics.IndoorGeo{Map: &b.M}
	for _, c := range []struct{ x, z, want int32 }{
		{100, 50, 0},    // only the floor
		{500, 300, 200}, // above the platform: it
		{500, 204, 200}, // within 5 above it
		{500, 100, 0},   // under it: the floor
	} {
		if got, _ := g.FloorZ(c.x, 500, c.z, s); got != c.want {
			t.Errorf("FloorZ(%d, 500, %d) = %d, want %d", c.x, c.z, got, c.want)
		}
	}
	if got, _ := g.FloorZ(2000, 500, 0, s); got != physics.NoFloor {
		t.Errorf("outside: %d", got)
	}
	if got := g.SectorAt(500, 500, 10); got != s {
		t.Errorf("SectorAt = %d", got)
	}
}

// mm8: 0x46e2dc (Collide_IndoorDecorations): a decoration of radius 20 at x 600 in the
// path of a sphere of radius 37 moving along +x from x 500 at 1000 units/s (speed 1001,
// dir.x = (0x10000 / 1001) * 1000 = 65000): the axis is 65000 * 100 >> 16 = 99 ahead,
// reached when the centres are 57 apart: 99 - isqrt(57^2) = 42.
func TestSweepCylinder(t *testing.T) {
	s := physics.State{RadiusLo: 37, RadiusHi: 18, Velocity: [3]int32{1000, 0, 0},
		PosLo: [3]int32{500, 500, 38}, PosHi: [3]int32{500, 500, 161}}
	s.Begin(0x10000)
	d, ok := s.SweepCylinder(physics.Cylinder{X: 600, Y: 500, Z: 0, Radius: 20, Height: 100})
	if !ok || d != 42 {
		t.Errorf("cylinder: %d %v, want 42", d, ok)
	}
	if _, ok := s.SweepCylinder(physics.Cylinder{X: 600, Y: 600, Z: 0, Radius: 20, Height: 100}); ok {
		t.Error("cylinder 100 to the side hit")
	}
	if _, ok := s.SweepCylinder(physics.Cylinder{X: 600, Y: 500, Z: 50, Radius: 20, Height: 100}); ok {
		t.Error("cylinder above the sphere centre hit") // the path's z (38) is below its base
	}
}
