package physics

import "libre-enroth/internal/maps/blv"

// NoFloor is what the indoor floor finders return when there is no floor under a point.
const NoFloor = -30000

// portalFloorZ is the height Indoor_FloorZ reports for a floor-type portal under the
// point (sectors with flag 8).
const portalFloorZ = -29000

// sectorPortalFloors marks sectors whose floor-type portals count as floors.
const sectorPortalFloors = 0x8

// Decoration is a level decoration as the collision code sees it.
type Decoration struct {
	Cylinder
	Solid bool // not hidden (level flag 0x20) and blocking (descriptor flag 1 clear)
}

// IndoorGeo answers floor and collision queries on an indoor map. Door animation moves
// Map's vertices and face planes in place (world.Indoor.UpdateDoors).
type IndoorGeo struct {
	Map         *blv.Map
	Decorations []Decoration // per level decoration
}

// Poly is the collision view of face fi.
func (g *IndoorGeo) Poly(fi int) Poly {
	f := &g.Map.Faces[fi]
	return Poly{
		Normal: f.Normal, Dist: f.Dist, ZCalc: f.ZCalc, Attr: f.Attr, PolyType: f.PolyType, BBox: f.BBox,
		Verts: closed(f.Verts), XDisp: f.XDisp, YDisp: f.YDisp, ZDisp: f.ZDisp, Vtx: g.Map.Vertices,
	}
}

// SectorAt is the sector containing a point, 0 for none: among the sectors whose
// bounding box holds it (z within 0x40), the floor or portal face under the point
// (crossing test in x/y) that is nearest below it.
//
// mm8: 0x499f5f (Indoor_GetSector)
func (g *IndoorGeo) SectorAt(px, py, pz int32) int {
	m := g.Map
	var cands []int
	for si := 1; si < len(m.Sectors); si++ {
		s := &m.Sectors[si]
		b := s.BBox
		if px < int32(b[0]) || px > int32(b[1]) || py < int32(b[2]) || py > int32(b[3]) ||
			pz < int32(b[4])-0x40 || pz > int32(b[5])+0x40 {
			continue
		}
		for _, list := range [2][]int16{s.Floors, s.Portals} {
			for _, fi := range list {
				f := &m.Faces[fi]
				if f.PolyType != blv.PolyFloor && f.PolyType != blv.PolySlopedFloor {
					continue
				}
				if g.sectorCrossings(f, px, py) == 1 {
					cands = append(cands, int(fi))
				}
			}
		}
	}
	switch len(cands) {
	case 0:
		return 0
	case 1:
		return int(m.Faces[cands[0]].Sector)
	}
	best, bestD := 0, int32(0xffffff)
	for _, fi := range cands {
		f := &m.Faces[fi]
		var fz int32
		if f.PolyType == blv.PolyFloor {
			fz = int32(m.Vertices[f.Verts[0]].Z)
		} else {
			fz = int32((int64(px)<<16*int64(f.ZCalc[0])>>16 + int64(py)<<16*int64(f.ZCalc[1])>>16 + int64(f.ZCalc[2]) + 0x8000) >> 16)
		}
		if d := pz - fz; d >= 0 && d < bestD {
			best, bestD = int(f.Sector), d
		}
	}
	return best
}

// sectorCrossings counts the edges of a face's x/y outline crossed by a ray from
// (px, py), stopping at 2 like the original.
//
// mm8: 0x499f5f (inline)
func (g *IndoorGeo) sectorCrossings(f *blv.Face, px, py int32) int {
	m := g.Map
	n := len(f.Verts)
	cross := 0
	for k := 0; k < n && cross < 2; k++ {
		a, b := m.Vertices[f.Verts[k]], m.Vertices[f.Verts[(k+1)%n]]
		if (py <= int32(a.Y)) == (py <= int32(b.Y)) {
			continue
		}
		left := 0
		if int32(a.X) < px {
			left |= 1
		}
		if int32(b.X) < px {
			left |= 2
		}
		if left == 3 {
			continue
		}
		if left != 0 {
			// x of the edge at py, 16.16 slope as in the original
			var ix int32
			if a.X < b.X {
				slope := (int64(b.X-a.X) << 16) / int64(b.Y-a.Y)
				ix = int32(a.X) + int32(int64(py-int32(a.Y))*slope>>16)
			} else {
				slope := (int64(a.X-b.X) << 16) / int64(a.Y-b.Y)
				ix = int32(b.X) + int32(int64(py-int32(b.Y))*slope>>16)
			}
			if ix <= px {
				continue
			}
		}
		cross++
	}
	return cross
}

// maxFloors is the size of the floor finders' candidate lists.
const maxFloors = 50

// pickFloor chooses among candidate heights: the highest at most 5 above z, else the
// lowest. It returns the index of the choice.
func pickFloor(zs []int32, z int32) int {
	best := 0
	for i := 1; i < len(zs); i++ {
		c, cur := zs[i], zs[best]
		if z+5 < cur {
			if c < cur {
				best = i
			}
		} else if cur < c && c <= z+5 {
			best = i
		}
	}
	return best
}

// FloorZ is the floor height under (x, y) in a sector and the floor face, or NoFloor.
// Sloped floors use zCalc; in sectors with flag 8 floor-type portals count as floors
// at -29000.
//
// mm8: 0x46d0d0 (Indoor_FloorZ)
func (g *IndoorGeo) FloorZ(x, y, z int32, sector int) (int32, int) {
	m := g.Map
	if sector <= 0 || sector >= len(m.Sectors) {
		return NoFloor, 0
	}
	s := &m.Sectors[sector]
	var zs [maxFloors]int32
	var fs [maxFloors]int
	n := 0
	inBox := func(b [6]int16) bool {
		return x <= int32(b[1]) && int32(b[0]) <= x && y <= int32(b[3]) && int32(b[2]) <= y
	}
	for _, fi := range s.Floors {
		f := &m.Faces[fi]
		if f.Attr&AttrEthereal != 0 || !inBox(f.BBox) {
			continue
		}
		p := g.Poly(int(fi))
		if !p.containsXY(x, y, false) {
			continue
		}
		if n >= maxFloors {
			break
		}
		if f.PolyType == blv.PolyFloor || f.PolyType == blv.PolyCeiling {
			zs[n] = int32(m.Vertices[f.Verts[0]].Z)
		} else {
			zs[n] = f.ZCalc[2]>>16 + Mul16(y, f.ZCalc[1]) + Mul16(x, f.ZCalc[0])
		}
		fs[n] = int(fi)
		n++
	}
	if s.Flags&sectorPortalFloors != 0 {
		for _, fi := range s.Portals {
			f := &m.Faces[fi]
			if f.PolyType != blv.PolyFloor || !inBox(f.BBox) {
				continue
			}
			p := g.Poly(int(fi))
			if !p.containsXY(x, y, true) {
				continue
			}
			if n >= maxFloors {
				break
			}
			zs[n], fs[n] = portalFloorZ, int(fi)
			n++
		}
	}
	if n == 0 {
		return NoFloor, 0
	}
	i := pickFloor(zs[:n], z)
	return zs[i], fs[i]
}

// FloorZSector is FloorZ in *sector; when that finds no floor or one more than 50
// above z, it looks the sector up again and retries there.
//
// mm8: 0x46ef67 (Indoor_FloorZSector)
func (g *IndoorGeo) FloorZSector(x, y, z int32, sector *int) (int32, int) {
	fz, face := g.FloorZ(x, y, z, *sector)
	if fz != NoFloor && fz <= z+0x32 {
		return fz, face
	}
	*sector = g.SectorAt(x, y, z)
	if *sector == 0 {
		return NoFloor, face
	}
	return g.FloorZ(x, y, z, *sector)
}

// FloorZNudge retries FloorZSector 2 units west, east, south and north (each from a
// fresh sector lookup at z + 0x28), then 0x8c higher.
//
// mm8: 0x472d4c (Indoor_FloorZNudge)
func (g *IndoorGeo) FloorZNudge(x, y, z int32, sector *int) (int32, int) {
	var fz int32
	var face int
	for _, d := range [4][2]int32{{-2, 0}, {2, 0}, {0, -2}, {0, 2}} {
		*sector = g.SectorAt(x+d[0], y+d[1], z+0x28)
		fz, face = g.FloorZSector(x+d[0], y+d[1], z+0x28, sector)
		if fz != NoFloor && *sector != 0 {
			return fz, face
		}
	}
	*sector = g.SectorAt(x, y, z+0x8c)
	return g.FloorZSector(x, y, z+0x8c, sector)
}

// sweepFace tests the lower sphere, and with CheckHi the upper one, against a face
// already known to overlap the sweep box, recording a hit as pid.
//
// mm8: 0x46e663, 0x46ead9 (inline, the same for both)
func (s *State) sweepFace(p *Poly, pid int32, ethereal, notParty bool) {
	test := func(pos, newPos [3]int32, r int32) {
		d1 := p.planeDist(pos)
		d2 := p.planeDist(newPos)
		if d1 <= 0 || d1 > s.RadiusLo && d2 > s.RadiusLo || d2 > d1 {
			return
		}
		dist, ok := SphereFace(r, pos, s.Dir, p, ethereal, notParty)
		if !ok {
			// The original subtracts the lower radius for both spheres.
			if dist, ok = RayFace(pos, s.Dir, r+s.MoveDist, p); !ok {
				return
			}
			dist -= s.RadiusLo
		}
		s.record(dist, pid)
	}
	test(s.PosLo, s.NewPosLo, s.RadiusLo)
	if s.CheckHi {
		test(s.PosHi, s.NewPosHi, s.RadiusHi)
	}
}

// maxCollideSectors bounds the sector and its portal neighbours CollideFaces visits.
const maxCollideSectors = 10

// CollideFaces sweeps against the floors, walls and ceilings of the current sector and
// of the sectors behind its portals near the path. ethereal skips ethereal faces (the
// party always passes true; with false only walls are solid when ethereal), notParty
// faces with AttrNotForParty.
//
// mm8: 0x46e663 (Collide_IndoorFaces)
func (g *IndoorGeo) CollideFaces(s *State, ethereal, notParty bool) {
	m := g.Map
	sectors := [maxCollideSectors]int{int(s.Sector)}
	n := 1
	cur := &m.Sectors[s.Sector]
	for _, pi := range cur.Portals {
		f := &m.Faces[pi]
		if !s.boxOverlaps(f.BBox) {
			continue
		}
		p := g.Poly(int(pi))
		if abs32(p.planeDist(s.PosLo)) > s.MoveDist+16 {
			continue
		}
		next := f.Sector
		if int32(next) == s.Sector {
			next = f.Back
		}
		sectors[n] = int(next)
		n++
		if n >= maxCollideSectors {
			break
		}
	}
	for _, si := range sectors[:n] {
		sec := &m.Sectors[si]
		// The original walks numFloors+numWalls+numCeilings entries from the floor list,
		// which runs on into the walls and ceilings in L.RData.
		for _, list := range [3][]int16{sec.Floors, sec.Walls, sec.Ceilings} {
			for _, fi := range list {
				f := &m.Faces[fi]
				if f.Attr&AttrPortal != 0 {
					continue
				}
				eth := ethereal || f.PolyType != blv.PolyWall
				if !s.boxOverlaps(f.BBox) || int32(fi) == s.IgnorePID {
					continue
				}
				p := g.Poly(int(fi))
				s.sweepFace(&p, int32(fi)<<3|KindFace, eth, notParty)
			}
		}
	}
}

// CollidePortals finds the first portal of the current sector the lower sphere's path
// crosses. When it comes before every hit so far and within this frame's move, the
// sweep moves into the sector behind it (AdjustedDist is reset for the new sector's
// tests) and it reports true: run the collide functions again.
//
// mm8: 0x46f29b (Collide_Portals)
func (g *IndoorGeo) CollidePortals(s *State) bool {
	m := g.Map
	best, bestFace := int32(NoHit), -1
	for _, pi := range m.Sectors[s.Sector].Portals {
		if int32(pi) == s.LastPortal {
			continue
		}
		f := &m.Faces[pi]
		if !s.boxOverlaps(f.BBox) {
			continue
		}
		p := g.Poly(int(pi))
		d1, d2 := p.planeDist(s.PosLo), p.planeDist(s.NewPosLo)
		r := s.RadiusLo
		if (d1 < r || d2 < r) && (-r < d1 || -r < d2) {
			if t, ok := RayFace(s.PosLo, s.Dir, s.MoveDist, &p); ok && t < best {
				best, bestFace = t, int(pi)
			}
		}
	}
	if s.AdjustedDist < best || s.MoveDist < best || bestFace < 0 {
		return false
	}
	s.LastPortal = int32(bestFace)
	f := &m.Faces[bestFace]
	if int32(f.Sector) == s.Sector {
		s.Sector = int32(f.Back)
	} else {
		s.Sector = int32(f.Sector)
	}
	s.AdjustedDist = 0xfffffff
	return true
}

// CollideDecorations sweeps against the solid decorations of the current sector.
//
// mm8: 0x46e2dc (Collide_IndoorDecorations)
func (g *IndoorGeo) CollideDecorations(s *State) {
	for _, di := range g.Map.Sectors[s.Sector].Decorations {
		if int(di) >= len(g.Decorations) || !g.Decorations[di].Solid {
			continue
		}
		if d, ok := s.SweepCylinder(g.Decorations[di].Cylinder); ok {
			s.record(d, int32(di)<<3|KindDecoration)
		}
	}
}
