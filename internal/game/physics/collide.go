package physics

import "libre-enroth/internal/maps/blv"

// Hit kinds in State.PID (id << 3 | kind).
const (
	KindActor      = 3
	KindDecoration = 5
	KindFace       = 6
)

// NoHit is AdjustedDist before anything was hit.
const NoHit = 0xffffff

// State is the swept-sphere query: a lower sphere at the feet and an upper one at head
// height move along Velocity for one frame; the collide functions shorten AdjustedDist
// to the nearest hit and record what it was in PID.
//
// mm8: 0x75e218 (g_collide, CollideState)
type State struct {
	CheckHi            bool  // +0x00: also test the upper sphere
	RadiusLo, RadiusHi int32 // +0x04, +0x08
	Height             int32 // +0x0c: upper sphere centre above the feet
	Velocity           [3]int32
	PosLo, PosHi       [3]int32 // +0x28, +0x34 sphere centres
	NewPosLo, NewPosHi [3]int32 // +0x40, +0x4c
	Dir                [3]int32 // +0x58: 16.16 unit velocity
	Speed              int32    // +0x64
	InvSpeed           int32    // +0x68
	MoveDist           int32    // +0x6c: distance left this frame
	TotalMoved         int32    // +0x70
	Sector             int32    // +0x74: indoors, updated at portals
	PID                int32    // +0x78: nearest hit, id << 3 | kind
	AdjustedDist       int32    // +0x7c: distance to it
	LastPortal         int32    // +0x80
	IgnorePID          int32    // +0x84
	BBox               [6]int32 // +0x8c: x1 x2 y1 y2 z1 z2 of the sweep
}

// Begin prepares the sweep for dt (16.16 seconds): direction, speed, the distance left
// this frame, the end positions and the box around the sweep. It reports false when
// there is nothing left to move.
//
// mm8: 0x47078b (Collide_Begin)
func (s *State) Begin(dt int32) bool {
	v := s.Velocity
	speed := int32(Isqrt(uint32(v[0]*v[0]+v[1]*v[1]+v[2]*v[2]))) | 1
	inv := int32(0x10000 / int64(speed))
	s.Speed, s.InvSpeed = speed, inv
	s.Dir = [3]int32{inv * v[0], inv * v[1], inv * v[2]}
	s.MoveDist = Mul16(speed, dt) - s.TotalMoved
	if s.MoveDist < 1 {
		return false
	}
	for k := range 2 {
		d := Mul16(s.Dir[k], s.MoveDist)
		s.NewPosLo[k] = s.PosLo[k] + d
		s.NewPosHi[k] = s.PosLo[k] + d
	}
	dz := Mul16(s.Dir[2], s.MoveDist)
	s.NewPosLo[2] = s.PosLo[2] + dz
	s.NewPosHi[2] = s.PosHi[2] + dz
	r := s.RadiusLo
	s.BBox[0] = min(s.PosLo[0], s.NewPosLo[0]) - r
	s.BBox[1] = max(s.PosLo[0], s.NewPosLo[0]) + r
	s.BBox[2] = min(s.PosLo[1], s.NewPosLo[1]) - r
	s.BBox[3] = max(s.PosLo[1], s.NewPosLo[1]) + r
	s.BBox[4] = min(s.PosLo[2], s.NewPosLo[2]) - r
	s.BBox[5] = max(s.PosHi[2], s.NewPosHi[2]) + s.RadiusHi
	s.PID = 0
	s.LastPortal = -1
	s.AdjustedDist = NoHit
	return true
}

// boxOverlaps tests a face bounding box (x1 x2 y1 y2 z1 z2) against the sweep box.
func (s *State) boxOverlaps(b [6]int16) bool {
	return s.BBox[0] <= int32(b[1]) && int32(b[0]) <= s.BBox[1] &&
		s.BBox[2] <= int32(b[3]) && int32(b[2]) <= s.BBox[3] &&
		s.BBox[4] <= int32(b[5]) && int32(b[4]) <= s.BBox[5]
}

// record keeps a hit if it is nearer than the current one.
func (s *State) record(dist, pid int32) {
	if dist < s.AdjustedDist {
		s.PID, s.AdjustedDist = pid, dist
	}
}

// Face attribute bits the collision code tests.
const (
	AttrPortal        = 0x1
	AttrXYPlane       = 0x100 // project on x/y for the point-in-polygon test
	AttrXZPlane       = 0x200 // on x/z; neither: y/z
	AttrFluid         = 0x10
	AttrNotForParty   = 0x200000   // skipped by the party's sweeps
	AttrPressurePlate = 0x4000000  // standing on it runs the face event
	AttrNotLevitating = 0x8000000  // ... unless someone levitates
	AttrEthereal      = 0x20000000 // no collision
	AttrLava          = 0x40000000 // burns (party flag 0x200)
)

// Poly is the face view the collision code works on: a BlvFace, or for a BModel face
// the BlvFace that ModelFace_AsBlvFace 0x46ef4e builds from it. Verts and the
// displacements hold numVerts + 1 entries (the stored closing one); Vtx is the vertex
// array they index, in 16 bits as the tests read it.
type Poly struct {
	Normal              [3]int32
	Dist                int32
	ZCalc               [3]int32
	Attr                uint32
	PolyType            uint8
	BBox                [6]int16
	Verts               []uint16
	XDisp, YDisp, ZDisp []int16
	Vtx                 []blv.Vec3s
}

// closed returns s with its stored closing entry (within the capacity), or with s[0]
// appended when there is none (synthetic faces).
func closed[T any](s []T) []T {
	if cap(s) > len(s) {
		return s[:len(s)+1]
	}
	if len(s) == 0 {
		return s
	}
	return append(s[:len(s):len(s)], s[0])
}

// vtx returns vertex i of the polygon; an index past the vertex array (junk in a closing
// entry) reads vertex Verts[0] instead of the memory the original would.
func (p *Poly) vtx(i int) blv.Vec3s {
	vi := int(p.Verts[i])
	if vi >= len(p.Vtx) {
		vi = int(p.Verts[0])
	}
	return p.Vtx[vi]
}

// NumVerts is the face's vertex count.
func (p *Poly) NumVerts() int { return len(p.Verts) - 1 }

// edges is the scratch outline of a point-in-polygon test: point pairs (start, end) per
// edge, 2n + 1 entries in all.
type edges struct{ u, v [2*256 + 1]int16 }

// crossings runs the original's edge loop over e (2n entries plus the copy of the first)
// for the point (pu, pv): it counts the edges whose v-range straddles pv and that are
// right of it, stopping at 2. round selects Face_ContainsPoint's rounded intersection;
// the floor finders truncate and multiply in the other order.
func (e *edges) crossings(n int, pu, pv int32, round bool) int {
	u, v := &e.u, &e.v
	u[2*n], v[2*n] = u[0], v[0]
	count := 0
	prev := int32(v[0]) >= pv
	for k := 0; k < 2*n && count < 2; k++ {
		v1 := int32(v[k+1])
		cur := v1 >= pv
		if cur != prev {
			side := 0
			if int32(u[k]) < pu {
				side |= 1
			}
			if int32(u[k+1]) < pu {
				side |= 2
			}
			switch {
			case side == 0:
				count++
			case side != 3:
				du, dv := int32(u[k+1])-int32(u[k]), v1-int32(v[k])
				var x int32
				if round {
					slope := Div16(du, dv)
					x = (Mul16((pv-int32(v[k]))<<16, slope)+0x8000)>>16 + int32(u[k])
				} else {
					t := Div16(pv-int32(v[k]), dv)
					x = int32(u[k]) + Mul16(t, du)
				}
				if x >= pu {
					count++
				}
			}
		}
		prev = cur
	}
	return count
}

// ContainsPoint reports whether a point on the face's plane lies inside it, testing on
// the plane its attributes name.
//
// mm8: 0x476178 (Face_ContainsPoint), 0x4764dc (ModelFace_ContainsPoint)
func (p *Poly) ContainsPoint(x, y, z int16) bool {
	n := p.NumVerts()
	if n <= 0 {
		return false
	}
	xd, yd, zd := closed(p.XDisp), closed(p.YDisp), closed(p.ZDisp)
	var e edges
	var pu, pv int16
	for i := range n {
		a, b := p.vtx(i), p.vtx(i+1)
		switch {
		case p.Attr&AttrXYPlane != 0:
			e.u[2*i], e.v[2*i] = a.X+xd[i], a.Y+yd[i]
			e.u[2*i+1], e.v[2*i+1] = b.X+xd[i+1], b.Y+yd[i+1]
		case p.Attr&AttrXZPlane != 0:
			e.u[2*i], e.v[2*i] = a.X+xd[i], a.Z+zd[i]
			e.u[2*i+1], e.v[2*i+1] = b.X+xd[i+1], b.Z+zd[i+1]
		default:
			e.u[2*i], e.v[2*i] = a.Y+yd[i], a.Z+zd[i]
			e.u[2*i+1], e.v[2*i+1] = b.Y+yd[i+1], b.Z+zd[i+1]
		}
	}
	switch {
	case p.Attr&AttrXYPlane != 0:
		pu, pv = x, y
	case p.Attr&AttrXZPlane != 0:
		pu, pv = x, z
	default:
		pu, pv = y, z
	}
	return e.crossings(n, int32(pu), int32(pv), true) == 1
}

// containsXY is the floor finders' test of the x/y outline: the second point of each
// edge takes the displacement of the first (as the original indexes it) unless
// nextDisp, which the portal loop of Indoor_FloorZ uses.
//
// mm8: 0x46d0d0, 0x46d6bb, 0x46db03 (inline)
func (p *Poly) containsXY(x, y int32, nextDisp bool) bool {
	n := p.NumVerts()
	if n <= 0 {
		return false
	}
	xd, yd := closed(p.XDisp), closed(p.YDisp)
	var e edges
	for i := range n {
		a, b := p.vtx(i), p.vtx(i+1)
		j := i
		if nextDisp {
			j = i + 1
		}
		e.u[2*i], e.v[2*i] = a.X+xd[i], a.Y+yd[i]
		e.u[2*i+1], e.v[2*i+1] = b.X+xd[j], b.Y+yd[j]
	}
	return e.crossings(n, x, y, false) == 1
}

// planeDist is the signed distance of a point to the face plane, (n.p + dist) >> 16 in
// 32-bit arithmetic.
func (p *Poly) planeDist(pt [3]int32) int32 {
	return (p.Normal[0]*pt[0] + p.Normal[1]*pt[1] + p.Normal[2]*pt[2] + p.Dist) >> 16
}

// SphereFace sweeps a sphere of the given radius from pos along dir (16.16) against the
// face plane: if the sphere already touches the plane the contact is where it stands,
// otherwise where its surface reaches the plane. The hit counts when that point is
// inside the face; dist is then the distance along dir (>= 0). ethereal skips
// AttrEthereal faces, notParty AttrNotForParty ones.
//
// mm8: 0x475e0b (Collide_SphereFace), 0x475fc0 (Collide_SphereModelFace)
func SphereFace(radius int32, pos, dir [3]int32, f *Poly, ethereal, notParty bool) (int32, bool) {
	if notParty && f.Attr&AttrNotForParty != 0 || ethereal && f.Attr&AttrEthereal != 0 {
		return 0, false
	}
	dot := Mul16(f.Normal[0], dir[0]) + Mul16(f.Normal[1], dir[1]) + Mul16(f.Normal[2], dir[2])
	d := radius<<16 - f.Normal[2]*pos[2] - f.Normal[0]*pos[0] - pos[1]*f.Normal[1] - f.Dist
	var t int32
	if abs32(d) < radius<<16 {
		radius = abs32(d) >> 16
	} else {
		if abs32(d)>>14 > abs32(dot) {
			return 0, false
		}
		t = Div16(d, dot)
	}
	x := int16(Mul16(dir[0], t)>>16+pos[0]) - int16(f.Normal[0]*radius>>16)
	y := int16(Mul16(dir[1], t)>>16+pos[1]) - int16(radius*f.Normal[1]>>16)
	z := int16(Mul16(dir[2], t)>>16+pos[2]) - int16(f.Normal[2]*radius>>16)
	if !f.ContainsPoint(x, y, z) {
		return 0, false
	}
	return max(t>>16, 0), true
}

// RayFace casts a ray from pos along dir (16.16) at the face from its front side (from
// either side for portals), up to maxDist; on a hit inside the face it returns the
// distance.
//
// mm8: 0x4768a0 (Collide_RayFace), 0x476a4d (Collide_RayModelFace)
func RayFace(pos, dir [3]int32, maxDist int32, f *Poly) (int32, bool) {
	if f.Attr&AttrEthereal != 0 {
		return 0, false
	}
	dot := Mul16(f.Normal[0], dir[0]) + Mul16(f.Normal[1], dir[1]) + Mul16(f.Normal[2], dir[2])
	if dot == 0 || dot > 0 && f.Attr&AttrPortal == 0 {
		return 0, false
	}
	s := -(f.Normal[0]*pos[0] + f.Normal[1]*pos[1] + pos[2]*f.Normal[2] + f.Dist)
	if dot > 0 && s < 0 || dot <= 0 && s > 0 {
		return 0, false
	}
	if abs32(s)>>14 > abs32(dot) {
		return 0, false
	}
	t := Div16(s, dot)
	if t > maxDist<<16 {
		return 0, false
	}
	x := int16((Mul16(dir[0], t)+0x8000)>>16) + int16(pos[0])
	y := int16((Mul16(dir[1], t)+0x8000)>>16) + int16(pos[1])
	z := int16((Mul16(dir[2], t)+0x8000)>>16) + int16(pos[2])
	if !f.ContainsPoint(x, y, z) {
		return 0, false
	}
	return t >> 16, true
}

// Cylinder is an upright obstacle: a decoration (or, in M8, an actor).
type Cylinder struct {
	X, Y, Z        int32 // base centre
	Radius, Height int32
}

// SweepCylinder tests the lower sphere's sweep against a cylinder and returns the
// distance to it (>= 0) when the path passes within radius + RadiusLo of its axis at a
// height inside it.
//
// mm8: 0x46e2dc, 0x46e493 (decorations), 0x46e143 (actors)
func (s *State) SweepCylinder(c Cylinder) (int32, bool) {
	r := c.Radius
	if s.BBox[0] > c.X+r || c.X-r > s.BBox[1] || s.BBox[2] > c.Y+r || c.Y-r > s.BBox[3] ||
		s.BBox[4] > c.Height+c.Z || c.Z > s.BBox[5] {
		return 0, false
	}
	dx, dy := c.X-s.PosLo[0], c.Y-s.PosLo[1]
	side := (s.Dir[1]*dx - s.Dir[0]*dy) >> 16
	rr := s.RadiusLo + r
	if abs32(side) > rr {
		return 0, false
	}
	along := (s.Dir[1]*dy + s.Dir[0]*dx) >> 16
	if along <= 0 {
		return 0, false
	}
	z := Mul16(along, s.Dir[2]) + s.PosLo[2]
	if z < c.Z || z > c.Z+c.Height {
		return 0, false
	}
	d := along - int32(Isqrt(uint32(rr*rr-side*side)))
	return max(d, 0), true
}

// ContainsVertexPoint is the line-of-sight tests' point-in-face check: the face's
// plain vertices (no displacements) projected by its attributes, the crossings counted
// with the rounded intersection.
//
// mm8: 0x4080d8 (Face_ContainsPointLOS), 0x4082f1 (ModelFace_ContainsPointLOS)
func (p *Poly) ContainsVertexPoint(x, y, z int32) bool {
	n := p.NumVerts()
	if n <= 0 {
		return false
	}
	u := make([]int32, n+1)
	v := make([]int32, n+1)
	var pu, pv int32
	for i := range n {
		a := p.vtx(i)
		switch {
		case p.Attr&AttrXYPlane != 0:
			u[i], v[i] = int32(a.X), int32(a.Y)
		case p.Attr&AttrXZPlane != 0:
			u[i], v[i] = int32(a.X), int32(a.Z)
		default:
			u[i], v[i] = int32(a.Y), int32(a.Z)
		}
	}
	switch {
	case p.Attr&AttrXYPlane != 0:
		pu, pv = x, y
	case p.Attr&AttrXZPlane != 0:
		pu, pv = x, z
	default:
		pu, pv = y, z
	}
	u[n], v[n] = u[0], v[0]
	count := 0
	prev := pv <= v[0]
	for k := 0; k < n && count < 2; k++ {
		cur := pv <= v[k+1]
		if cur != prev {
			side := 0
			if u[k+1] < pu {
				side |= 2
			}
			if u[k] < pu {
				side |= 1
			}
			if side == 0 {
				count++
			} else if side != 3 {
				slope := Div16(u[k+1]-u[k], v[k+1]-v[k])
				if pu <= (Mul16((pv-v[k])<<16, slope)+0x8000)>>16+u[k] {
					count++
				}
			}
		}
		prev = cur
	}
	return count == 1
}

// SweepActor tests the lower sphere's sweep against an actor's cylinder (base centre
// x, y, z, radius and height) and records the hit as pid when it is the nearest. Unlike
// the decorations' test, only the actor's base must be below the path's height at the
// closest point. It reports a touch whether or not it was the nearest hit.
//
// mm8: 0x46e143 (Collide_Actor)
func (s *State) SweepActor(x, y, z, radius, height, pid int32) bool {
	if s.BBox[0] > x+radius || x-radius > s.BBox[1] || s.BBox[2] > y+radius || y-radius > s.BBox[3] ||
		s.BBox[4] > height+z || z > s.BBox[5] {
		return false
	}
	dx, dy := x-s.PosLo[0], y-s.PosLo[1]
	rr := s.RadiusLo + radius
	side := (s.Dir[1]*dx - s.Dir[0]*dy) >> 16
	if abs32(side) > rr {
		return false
	}
	along := (s.Dir[1]*dy + s.Dir[0]*dx) >> 16
	if along <= 0 || z > s.PosLo[2]+Mul16(along, s.Dir[2]) {
		return false
	}
	d := max(along-int32(Isqrt(uint32(rr*rr-side*side))), 0)
	s.record(d, pid)
	return true
}

// SweepParty tests a monster's sweep against the party, a cylinder twice its radius:
// the path's height at the closest point must be within the party's (or anywhere
// above its feet when any).
//
// mm8: 0x46f14e (Collide_Party)
func (s *State) SweepParty(x, y, z, radius, height int32, any bool) {
	r := radius * 2
	if s.BBox[0] > x+r || x-r > s.BBox[1] || s.BBox[2] > y+r || y-r > s.BBox[3] ||
		s.BBox[4] > height+z || z > s.BBox[5] {
		return
	}
	dx, dy := x-s.PosLo[0], y-s.PosLo[1]
	rr := s.RadiusLo + r
	side := (s.Dir[1]*dx - s.Dir[0]*dy) >> 16
	if abs32(side) > rr {
		return
	}
	along := (s.Dir[0]*dx + s.Dir[1]*dy) >> 16
	if along <= 0 {
		return
	}
	pz := s.PosLo[2] + Mul16(along, s.Dir[2])
	if z > pz || pz > z+height && !any {
		return
	}
	d := max(along-int32(Isqrt(uint32(rr*rr-side*side))), 0)
	s.record(d, 4)
}
