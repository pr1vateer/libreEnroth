package monsters

import (
	"math"

	"libre-enroth/internal/game/physics"
)

// Line of sight (re/notes/monsters.md, "Line of sight").

// losReach is how far an object sees another.
const losReach = 0x1400

// maxPortalSteps bounds CanSee's walk through the portals.
const maxPortalSteps = 0x1e

// seePos is where CanSee looks from or to on object pid, and its sector indoors: an
// actor at 7/10 of its height (rounded), the party at its eyes; ok is false for kinds
// it does not know (the party only as the second object).
func (b *Brain) seePos(pid uint32, second bool) (x, y, z int32, sector int, ok bool) {
	n := int(pid >> 3)
	switch pid & 7 {
	case KindObject:
		if n >= len(b.Objects) {
			return
		}
		o := &b.Objects[n]
		return o.Pos[0], o.Pos[1], o.Pos[2], int(o.Sector), true
	case KindActor:
		if n >= len(b.Actors) {
			return
		}
		a := &b.Actors[n]
		z := int32(a.Pos[2]) + int32(math.RoundToEven(float64(int16(a.Height))*0.7))
		return int32(a.Pos[0]), int32(a.Pos[1]), z, int(a.Sector), true
	case KindParty:
		if !second {
			return
		}
		p := &b.Party
		x, y, z = p.X, p.Y, p.EyeLevel+p.Z
	case KindDecoration:
		x, y, z = b.decorationPos(n)
	default:
		return
	}
	if b.Indoor != nil {
		sector = b.Indoor.SectorAt(x, y, z)
	}
	return x, y, z, sector, true
}

// ray is a segment cast for line of sight: its start, its 16.16 direction scaled by the
// inverse length, and its bounding box.
type ray struct {
	x, y, z    int32
	dx, dy, dz int32 // unit * length: (0x10000 / length) * delta
	box        [6]int32
}

func newRay(x1, y1, z1, x2, y2, z2 int32) (ray, uint32) {
	dx, dy, dz := x2-x1, y2-y1, z2-z1
	d := physics.Isqrt(uint32(dz*dz + dy*dy + dx*dx))
	inv := int32(0x10000)
	if d != 0 {
		inv = int32(0x10000 / int64(int32(d)))
	}
	r := ray{x: x1, y: y1, z: z1, dx: inv * dx, dy: inv * dy, dz: inv * dz}
	r.box = [6]int32{min(x1, x2), max(x1, x2), min(y1, y2), max(y1, y2), min(z1, z2), max(z1, z2)}
	return r, d
}

func (r *ray) overlaps(b [6]int16) bool {
	return r.box[0] <= int32(b[1]) && int32(b[0]) <= r.box[1] && r.box[2] <= int32(b[3]) &&
		int32(b[2]) <= r.box[3] && r.box[4] <= int32(b[5]) && int32(b[4]) <= r.box[5]
}

// hits intersects the ray with a face plane and tests the point in it: the ray must
// not run along the plane, must start on its side the right way round, and reach it
// at a distance >= 0.
func (r *ray) hits(f *physics.Poly) bool {
	dot := physics.Mul16(f.Normal[0], r.dx) + physics.Mul16(f.Normal[1], r.dy) + physics.Mul16(f.Normal[2], r.dz)
	if dot == 0 {
		return false
	}
	s := f.Normal[1]*r.y + f.Normal[0]*r.x + r.z*f.Normal[2] + f.Dist
	ns := -s
	if dot >= 1 && ns < 0 || dot < 1 && ns >= 1 {
		return false
	}
	if abs32(ns)>>14 > abs32(dot) {
		return false
	}
	t := int32((int64(ns) << 16) / int64(dot))
	if t < 0 {
		return false
	}
	at := func(d, o int32) int32 { return (int32(int64(d)*int64(t)>>16)+0x8000)>>16 + o }
	return f.ContainsVertexPoint(at(r.dx, r.x), at(r.dy, r.y), at(r.dz, r.z))
}

// CanSee reports whether object from sees object to: within 0x1400, and indoors along
// the portals from from's sector to to's (at most 30 steps): each step takes the
// first portal of the sector whose plane the segment crosses inside it, from the
// right side.
//
// mm8: 0x407be3 (Actor_CanSee)
func (b *Brain) CanSee(from, to uint32) bool {
	x1, y1, z1, s1, ok1 := b.seePos(from, false)
	x2, y2, z2, s2, ok2 := b.seePos(to, true)
	if !ok1 || !ok2 {
		return false
	}
	r, d := newRay(x1, y1, z1, x2, y2, z2)
	if int32(d) > losReach {
		return false
	}
	if b.Indoor == nil {
		return true
	}
	m := b.Indoor.Map
	for steps := 0; ; steps++ {
		if s1 == s2 {
			return true
		}
		if steps >= maxPortalSteps || s1 < 0 || s1 >= len(m.Sectors) {
			return false
		}
		next := -1
		for _, pf := range m.Sectors[s1].Portals {
			f := &m.Faces[pf]
			v0 := m.Vertices[f.Verts[0]]
			side := (int32(v0.X)-x1)*f.Normal[0] + (int32(v0.Y)-y1)*f.Normal[1] + (int32(v0.Z)-z1)*f.Normal[2]
			if s1 != int(f.Sector) {
				side = -side
			}
			if side >= 0 && int32(v0.X) != x1 && int32(v0.Y) != y1 && int32(v0.Z) != z1 {
				continue
			}
			if !r.overlaps(f.BBox) {
				continue
			}
			p := b.Indoor.Poly(int(pf))
			if r.hits(&p) {
				next = int(pf)
				break
			}
		}
		if next < 0 {
			return false
		}
		f := &m.Faces[next]
		ns := int(f.Sector)
		if ns == s1 {
			ns = int(f.Back)
		}
		if ns == s1 {
			return false
		}
		s1 = ns
	}
}

// lineDist is the distance of (cx, cy) from the line through (x1, y1) and (x2, y2), in
// 32-bit arithmetic; 0 for a point line.
//
// mm8: 0x40938f
func lineDist(x1, y1, x2, y2, cx, cy int32) int32 {
	dx, dy := abs32(x2-x1), abs32(y2-y1)
	l := int32(physics.Isqrt(uint32(dy*dy + dx*dx)))
	if l == 0 {
		return 0
	}
	return abs32(((y1-cy)*(x2-x1) - (x1-cx)*(y2-y1)) / l)
}

// CanSeePoints reports whether nothing blocks the way between two points: two rays,
// 0x20 to either side of the line between them, are cast against the BModels (outdoors)
// or the walls of both points' sectors (indoors); only both blocked block.
//
// mm8: 0x408520 (Map_CanSeePoints)
func (b *Brain) CanSeePoints(x1, y1, z1, x2, y2, z2 int32) bool {
	ang := physics.Atan2(x2-x1, y2-y1)
	var blocked [2]bool
	for k, side := range [2]int32{physics.QuarterTurn, -physics.QuarterTurn} {
		ax, ay, az := offsetPoint(0x20, ang+side, 0, x1, y1, z1)
		bx, by, bz := offsetPoint(0x20, ang+side, 0, x2, y2, z2)
		r, _ := newRay(ax, ay, az, bx, by, bz)
		if b.Outdoor != nil {
			blocked[k] = b.rayBlockedOutdoor(&r, bx, by)
		} else if b.Indoor != nil {
			blocked[k] = b.rayBlockedIndoor(&r, bx, by, bz)
		}
	}
	return !blocked[0] || !blocked[1]
}

// rayBlockedOutdoor tests the faces of the models whose position lies within their
// radius + 0x80 of the ray's line.
func (b *Brain) rayBlockedOutdoor(r *ray, x2, y2 int32) bool {
	g := b.Outdoor
	for mi := range g.Map.Models {
		m := &g.Map.Models[mi]
		if lineDist(r.x, r.y, x2, y2, m.Position.X, m.Position.Y) > m.Radius+0x80 {
			continue
		}
		for fi := range m.Faces {
			if !r.overlaps(m.Faces[fi].BBox) {
				continue
			}
			p := g.Poly(mi, fi)
			if r.hits(&p) {
				return true
			}
		}
	}
	return false
}

// rayBlockedIndoor tests the walls of the end point's sector, then the start's: the
// original walks numWalls + 2 * numFloors entries from the wall list, which runs on into
// the ceilings, fluids, portals and the face list in L.RData; portals do not block.
func (b *Brain) rayBlockedIndoor(r *ray, x2, y2, z2 int32) bool {
	g := b.Indoor
	m := g.Map
	for k := range 2 {
		x, y, z := r.x, r.y, r.z
		if k == 0 {
			x, y, z = x2, y2, z2
		}
		si := g.SectorAt(x, y, z)
		if si < 0 || si >= len(m.Sectors) {
			continue
		}
		sec := &m.Sectors[si]
		n := len(sec.Walls) + 2*len(sec.Floors)
		for _, fi := range runOn(n, sec.Walls, sec.Ceilings, sec.Fluids, sec.Portals, sec.Faces, sec.Cogs, sec.Decorations, sec.Markers) {
			if fi < 0 || int(fi) >= len(m.Faces) {
				continue
			}
			f := &m.Faces[fi]
			if f.Attr&physics.AttrPortal != 0 || !r.overlaps(f.BBox) {
				continue
			}
			p := g.Poly(int(fi))
			if r.hits(&p) {
				return true
			}
		}
	}
	return false
}

// runOn is the first n entries of the lists laid end to end.
func runOn(n int, lists ...[]int16) []int16 {
	out := make([]int16, 0, n)
	for _, l := range lists {
		for _, v := range l {
			if len(out) == n {
				return out
			}
			out = append(out, v)
		}
	}
	return out
}
