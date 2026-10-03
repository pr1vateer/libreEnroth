package physics

import (
	"math"

	"libre-enroth/internal/maps/blv"
	"libre-enroth/internal/maps/odm"
)

// Tile attribute bits (dtile.bin +0x18).
const (
	TileBurn  = 0x1 // lava
	TileWater = 0x2
)

// Terrain heights in water and lava: the party sinks 60, or floats 20 above the
// surface while levitating.
//
// mm8: 0x483842 ((levitate & 0x50) - 0x3c)
const (
	waterSink  = -0x3c
	waterFloat = 0x50 - 0x3c
)

// NoCeiling is what CeilingZ returns when there is no BModel ceiling above.
const NoCeiling = 10000

// maxModelFloors is the size of the BModel floor and ceiling candidate lists.
const maxModelFloors = 20

// OutdoorGeo answers terrain, floor and collision queries on an outdoor map.
type OutdoorGeo struct {
	Map          *odm.Map
	PlaneOfWater bool                        // elemw.odm: low gravity, flying without the buff (Map_IsElemW 0x4640ef)
	FlyCeiling   int32                       // highest flying z (the .ddm's OutdoorLocation +0x51c), 0 = 4000
	TileAttr     [odm.Grid * odm.Grid]uint16 // per cell [gy*128+gx]
	Decorations  []Decoration                // per level decoration
	vtx          [][]blv.Vec3s               // per model, the vertices in 16 bits
}

// Gravity is the fall acceleration per timer tick: 5, or 1 on the Plane of Water.
//
// mm8: 0x46bab0 (Physics_Gravity)
func Gravity(planeOfWater bool) int32 {
	if planeOfWater {
		return 1
	}
	return 5
}

// NewOutdoorGeo prepares a map; attr is the tile attribute of a cell (Odm_TileAttr).
func NewOutdoorGeo(m *odm.Map, attr func(gx, gy int) uint16, decs []Decoration) *OutdoorGeo {
	g := &OutdoorGeo{Map: m, Decorations: decs}
	for gy := range odm.Grid {
		for gx := range odm.Grid {
			g.TileAttr[gy*odm.Grid+gx] = attr(gx, gy)
		}
	}
	g.vtx = make([][]blv.Vec3s, len(m.Models))
	for i, mod := range m.Models {
		v := make([]blv.Vec3s, len(mod.Vertices))
		for k, p := range mod.Vertices {
			v[k] = blv.Vec3s{X: int16(p.X), Y: int16(p.Y), Z: int16(p.Z)}
		}
		g.vtx[i] = v
	}
	return g
}

// Poly is the collision view of face fi of model mi.
//
// mm8: 0x46ef4e (ModelFace_AsBlvFace)
func (g *OutdoorGeo) Poly(mi, fi int) Poly {
	f := &g.Map.Models[mi].Faces[fi]
	return Poly{
		Normal: f.Normal, Dist: f.Dist, ZCalc: f.ZCalc, Attr: f.Attr, PolyType: f.PolyType, BBox: f.BBox,
		Verts: closed(f.Verts), XDisp: f.XDisp, YDisp: f.YDisp, ZDisp: f.ZDisp, Vtx: g.vtx[mi],
	}
}

// GridX and GridY are the cell of a world position; the terrain functions use rows
// GridY-1 (north edge) and GridY (south edge).
//
// mm8: 0x480754 (Odm_WorldToGridX), 0x480761 (Odm_WorldToGridY)
func GridX(x int32) int { return int(x>>9) + 64 }
func GridY(y int32) int { return 64 - int(y>>9) }

// Attr is the tile attribute of a cell, 0 outside the map.
//
// mm8: 0x47ff84 (Odm_TileAttr)
func (g *OutdoorGeo) Attr(gx, gy int) uint16 {
	if gx < 0 || gx >= odm.Grid || gy < 0 || gy >= odm.Grid {
		return 0
	}
	return g.TileAttr[gy*odm.Grid+gx]
}

// corners are the four heights of the cell under (x, y) and its edges.
type corners struct {
	x0, x1, yN, yS int32
	h              [4]int32 // NW, NE, SE, SW
}

func (g *OutdoorGeo) corners(x, y int32) corners {
	gx, gy := GridX(x), GridY(y)
	m := g.Map
	return corners{
		x0: int32(gx-64) * 512, x1: int32(gx+1-64) * 512,
		yN: int32(64-(gy-1)) * 512, yS: int32(64-gy) * 512,
		h: [4]int32{int32(m.Height(gx, gy-1)), int32(m.Height(gx+1, gy-1)), int32(m.Height(gx+1, gy)), int32(m.Height(gx, gy))},
	}
}

func (c *corners) flat() bool { return c.h[0] == c.h[1] && c.h[1] == c.h[2] && c.h[2] == c.h[3] }

// northEast reports whether (x, y) is on the cell's north-east triangle (NW, NE, SE).
func (c *corners) northEast(x, y int32) bool { return abs32(c.yN-y) < abs32(x-c.x0) }

// TerrainZ is the terrain height at (x, y) and the water and lava flags of its cell.
// The party sinks into water (unless noWaterSink, i.e. water walking) and lava (unless
// noBurnSink), or floats above it while levitating.
//
// mm8: 0x483842 (Terrain_HeightAt; the D3D path, tile attributes only)
func (g *OutdoorGeo) TerrainZ(x, y int32, levitate, noWaterSink, noBurnSink bool) (z int32, water, burn bool) {
	c := g.corners(x, y)
	attr := g.Attr(GridX(x), GridY(y)-1)
	burn, water = attr&TileBurn != 0, attr&TileWater != 0
	var adj int32
	if burn && !noBurnSink || water && !noWaterSink {
		adj = waterSink
		if levitate {
			adj = waterFloat
		}
	}
	if c.flat() {
		return c.h[0] + adj, water, burn
	}
	var dx, dy, a, b, base int32
	if c.northEast(x, y) {
		dx, dy, a, b, base = c.x1-x, c.yN-y, c.h[0], c.h[2], c.h[1]
	} else {
		dx, dy, a, b, base = x-c.x0, y-c.yS, c.h[2], c.h[0], c.h[3]
	}
	z = Mul16((a-base)*0x80, dx) + Mul16((b-base)*0x80, dy) + base
	return z + adj, water, burn
}

// TooSteep reports whether the terrain triangle under (x, y) rises more than 0x200.
//
// mm8: 0x4836bc (Terrain_TooSteep)
func (g *OutdoorGeo) TooSteep(x, y int32) bool {
	c := g.corners(x, y)
	if c.flat() {
		return false
	}
	a, b, d := c.h[0], c.h[3], c.h[2]
	if c.northEast(x, y) {
		a, b, d = c.h[2], c.h[1], c.h[0]
	}
	return max(a, b, d)-min(a, b, d) > 0x200
}

// TerrainNormal is the 16.16 unit normal of the terrain triangle under (x, y), computed
// in floating point and truncated.
//
// mm8: 0x46def7 (Terrain_Normal)
func (g *OutdoorGeo) TerrainNormal(x, y int32) [3]int32 {
	c := g.corners(x, y)
	// Triangle points (px, py, h): 0 = the corner the edges start from.
	x5, x7, xc := c.x1, c.x0, c.x0
	y6, y10, y8 := c.yS, c.yN, c.yS
	h18, h13, h10 := c.h[2], c.h[0], c.h[3]
	if c.northEast(x, y) {
		x5, x7, xc = c.x0, c.x1, c.x1
		y6, y10, y8 = c.yN, c.yS, c.yN
		h18, h13, h10 = c.h[0], c.h[2], c.h[1]
	}
	f2 := float32(x7 - xc)
	nx := float32(float64(h13-h10)*float64(y6-y8) - float64(y10-y8)*float64(h18-h10))
	ny := float32(float64(f2)*float64(h18-h10) - float64(h13-h10)*float64(x5-xc))
	nz := float32(float64(y10-y8)*float64(x5-xc) - float64(f2)*float64(y6-y8))
	l := math.Sqrt(float64(nx)*float64(nx) + float64(ny)*float64(ny) + float64(nz)*float64(nz))
	if l == 0 {
		return [3]int32{0, 0, 0x10000}
	}
	return [3]int32{int32(float64(nx) / l * 65536), int32(float64(ny) / l * 65536), int32(float64(nz) / l * 65536)}
}

// modelFloors collects the BModel floors (types 3, 4) or ceilings (5, 6) whose x/y
// outline holds (x, y), after zs[0]/ids[0] which the caller fills.
func (g *OutdoorGeo) modelFloors(x, y int32, ceiling bool, zs []int32, ids []int32) int {
	n := 1
	m := g.Map
	for mi := range m.Models {
		mod := &m.Models[mi]
		if x > mod.BBoxMax.X || mod.BBoxMin.X > x || y > mod.BBoxMax.Y || mod.BBoxMin.Y > y {
			continue
		}
		for fi := range mod.Faces {
			f := &mod.Faces[fi]
			flat, slope := f.PolyType == blv.PolyFloor, f.PolyType == blv.PolySlopedFloor
			if ceiling {
				flat, slope = f.PolyType == blv.PolyCeiling, f.PolyType == blv.PolySlopedCeil
			}
			if !flat && !slope || f.Attr&AttrEthereal != 0 ||
				x > int32(f.BBox[1]) || int32(f.BBox[0]) > x || y > int32(f.BBox[3]) || int32(f.BBox[2]) > y {
				continue
			}
			p := g.Poly(mi, fi)
			if !p.containsXY(x, y, false) {
				continue
			}
			if n >= maxModelFloors {
				break
			}
			if flat {
				zs[n] = mod.Vertices[f.Verts[0]].Z
			} else {
				zs[n] = f.ZCalc[2]>>16 + Mul16(y, f.ZCalc[1]) + Mul16(x, f.ZCalc[0])
			}
			ids[n] = int32(mi)<<6 | int32(fi)
			n++
		}
	}
	return n
}

// FloorZ is the floor under (x, y): the terrain, or the BModel floor the original's
// choice prefers (the highest at most 5 above z, else the lowest; later ones win ties),
// but never below the terrain. face is the BModel face id (model << 6 | face) or 0 for
// the terrain; water is the terrain's water flag or the chosen face's fluid flag.
//
// mm8: 0x46d6bb (Outdoor_FloorZ)
func (g *OutdoorGeo) FloorZ(x, y, z int32, levitate, noWaterSink bool) (fz int32, water, burn bool, face int32) {
	var zs [maxModelFloors]int32
	var ids [maxModelFloors]int32
	zs[0], water, burn = g.TerrainZ(x, y, levitate, noWaterSink, false)
	n := g.modelFloors(x, y, false, zs[:], ids[:])
	if n == 1 {
		return zs[0], water, burn, 0
	}
	best := 0
	for i := 1; i < n; i++ {
		c, cur := zs[i], zs[best]
		switch {
		case c == cur:
			best = i
		case z+5 < cur:
			if c < cur {
				best = i
			}
		case cur < c && c <= z+5:
			best = i
		}
	}
	if best != 0 {
		face = ids[best]
		f := &g.Map.Models[face>>6].Faces[face&0x3f]
		water = f.Attr&AttrFluid != 0
	}
	return max(zs[best], zs[0]), water, burn, face
}

// CeilingZ is the BModel ceiling over (x, y) for a head at z (the highest at most 15
// above z, else the lowest), or NoCeiling, and its face id.
//
// mm8: 0x46db03 (Outdoor_CeilingZ)
func (g *OutdoorGeo) CeilingZ(x, y, z int32) (int32, int32) {
	var zs [maxModelFloors]int32
	var ids [maxModelFloors]int32
	zs[0] = NoCeiling
	n := g.modelFloors(x, y, true, zs[:], ids[:])
	best := 0
	for i := 0; i < n; i++ {
		c, cur := zs[i], zs[best]
		switch {
		case c == cur:
			best = i
		case z+0xf < cur:
			if c < cur {
				best = i
			}
		case cur < c && c <= z+0xf:
			best = i
		}
	}
	if best == 0 {
		return zs[0], 0
	}
	return zs[best], ids[best]
}

// CollideModels sweeps against every BModel face near the path.
//
// mm8: 0x46ead9 (Collide_Models)
func (g *OutdoorGeo) CollideModels(s *State, ethereal, notParty bool) {
	m := g.Map
	for mi := range m.Models {
		mod := &m.Models[mi]
		if s.BBox[0] > mod.BBoxMax.X || mod.BBoxMin.X > s.BBox[1] || s.BBox[2] > mod.BBoxMax.Y ||
			mod.BBoxMin.Y > s.BBox[3] || s.BBox[4] > mod.BBoxMax.Z || mod.BBoxMin.Z > s.BBox[5] {
			continue
		}
		for fi := range mod.Faces {
			f := &mod.Faces[fi]
			if !s.boxOverlaps(f.BBox) {
				continue
			}
			if notParty && f.Attr&AttrNotForParty != 0 || f.Attr&(AttrEthereal|AttrPortal) != 0 {
				continue
			}
			p := g.Poly(mi, fi)
			s.sweepFace(&p, (int32(mi)<<6|int32(fi))<<3|KindFace, ethereal, notParty)
		}
	}
}

// CollideCellDecorations sweeps against the solid decorations listed in a cell's
// FaceIDs entries (kind 5).
//
// mm8: 0x46e493 (Collide_CellDecorations)
func (g *OutdoorGeo) CollideCellDecorations(s *State, gx, gy int) {
	m := g.Map
	if gx < 0 || gx >= odm.Grid || gy < 0 || gy >= odm.Grid {
		return
	}
	for i := int(m.CellFaces[gy*odm.Grid+gx]); i < len(m.FaceIDs); i++ {
		id := m.FaceIDs[i]
		if id&7 == KindDecoration {
			di := int(int16(id) >> 3)
			if di < len(g.Decorations) && g.Decorations[di].Solid {
				if d, ok := s.SweepCylinder(g.Decorations[di].Cylinder); ok {
					s.record(d, int32(int16(id)))
				}
			}
		}
		if id == 0 {
			break
		}
	}
}

// WaterGate decides whether the party may move into the cell (gx, gy) and whether it
// is then in water: on foot and on the ground, without water walking (or over lava
// unless levitating), it cannot step from land into water, and in water it stays in it.
//
// mm8: 0x476c22 (Outdoor_WaterGate)
func (g *OutdoorGeo) WaterGate(fromLand, toLand, flying, waterWalk, airborne, levitate bool, gx, gy int) (ok, inWater bool) {
	if !flying && !airborne && (!waterWalk || g.Attr(gx, gy)&TileBurn != 0 && !levitate) {
		if fromLand {
			return toLand, false
		}
		return true, true
	}
	return true, false
}
