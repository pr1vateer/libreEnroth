// Package physicstest builds small synthetic indoor maps for the movement tests: axis
// aligned rooms with 16.16 face planes, the way the .blv stores them.
package physicstest

import (
	"libre-enroth/internal/maps/blv"
)

// Builder accumulates vertices, faces and sectors. Sector 0 is the dummy one.
type Builder struct {
	M blv.Map
}

// New starts a map with the dummy sector 0.
func New() *Builder {
	b := &Builder{}
	b.M.Sectors = []blv.Sector{{}}
	b.M.FaceExtras = []blv.FaceExtra{{}}
	return b
}

// Sector adds an empty sector with a bounding box and returns its index.
func (b *Builder) Sector(x1, x2, y1, y2, z1, z2 int16) int {
	b.M.Sectors = append(b.M.Sectors, blv.Sector{BBox: [6]int16{x1, x2, y1, y2, z1, z2}})
	return len(b.M.Sectors) - 1
}

// Projection flags of the point-in-polygon test (Face_ContainsPoint 0x476178).
const (
	attrXY = 0x100
	attrXZ = 0x200
)

// Face adds a polygon with the given 16.16 normal to a sector's list for its type and
// returns its index. The plane distance comes from the first point; the projection
// plane from the normal's largest component; Verts and the displacements carry the
// stored closing entry.
func (b *Builder) Face(sector int, poly uint8, attr uint32, normal [3]int32, pts ...[3]int16) int {
	m := &b.M
	n := len(pts)
	f := blv.Face{Normal: normal, PolyType: poly, Attr: attr, Sector: int16(sector), Back: int16(sector)}
	f.Verts = make([]uint16, n+1)
	f.XDisp, f.YDisp, f.ZDisp = make([]int16, n+1)[:n], make([]int16, n+1)[:n], make([]int16, n+1)[:n]
	f.BBox = [6]int16{pts[0][0], pts[0][0], pts[0][1], pts[0][1], pts[0][2], pts[0][2]}
	for i, p := range pts {
		m.Vertices = append(m.Vertices, blv.Vec3s{X: p[0], Y: p[1], Z: p[2]})
		f.Verts[i] = uint16(len(m.Vertices) - 1)
		for k := range 3 {
			f.BBox[2*k] = min(f.BBox[2*k], p[k])
			f.BBox[2*k+1] = max(f.BBox[2*k+1], p[k])
		}
	}
	f.Verts[n] = f.Verts[0]
	f.Verts = f.Verts[:n]
	p0 := pts[0]
	f.Dist = -(normal[0]*int32(p0[0]) + normal[1]*int32(p0[1]) + normal[2]*int32(p0[2]))
	ax, ay, az := abs(normal[0]), abs(normal[1]), abs(normal[2])
	switch {
	case az >= ax && az >= ay:
		f.Attr |= attrXY
	case ay >= ax:
		f.Attr |= attrXZ
	}
	if poly == blv.PolySlopedFloor && normal[2] != 0 {
		// z = (x*a >> 16) + (y*b >> 16) + (c >> 16) with a = -nx/nz, b = -ny/nz, c = -dist/nz
		f.ZCalc = [3]int32{div16(-normal[0], normal[2]), div16(-normal[1], normal[2]), div16(-f.Dist, normal[2])}
	}
	m.Faces = append(m.Faces, f)
	fi := len(m.Faces) - 1
	s := &m.Sectors[sector]
	switch {
	case attr&1 != 0:
		s.Portals = append(s.Portals, int16(fi))
	case poly == blv.PolyFloor || poly == blv.PolySlopedFloor:
		s.Floors = append(s.Floors, int16(fi))
	case poly == blv.PolyCeiling || poly == blv.PolySlopedCeil:
		s.Ceilings = append(s.Ceilings, int16(fi))
	default:
		s.Walls = append(s.Walls, int16(fi))
	}
	return fi
}

// Portal adds a portal face between two sectors (listed in both).
func (b *Builder) Portal(front, back int, normal [3]int32, pts ...[3]int16) int {
	fi := b.Face(front, blv.PolyWall, 1, normal, pts...)
	b.M.Faces[fi].Back = int16(back)
	b.M.Sectors[back].Portals = append(b.M.Sectors[back].Portals, int16(fi))
	return fi
}

// Box adds the floor (at z1), ceiling (z2) and four walls of an axis-aligned room to a
// sector, all facing inwards. open lists walls to leave out: 'w', 'e', 's', 'n'.
func (b *Builder) Box(sector int, x1, x2, y1, y2, z1, z2 int16, open string) {
	const one = 0x10000
	b.Face(sector, blv.PolyFloor, 0, [3]int32{0, 0, one}, [3]int16{x1, y1, z1}, [3]int16{x2, y1, z1}, [3]int16{x2, y2, z1}, [3]int16{x1, y2, z1})
	b.Face(sector, blv.PolyCeiling, 0, [3]int32{0, 0, -one}, [3]int16{x1, y1, z2}, [3]int16{x1, y2, z2}, [3]int16{x2, y2, z2}, [3]int16{x2, y1, z2})
	has := func(c rune) bool {
		for _, o := range open {
			if o == c {
				return false
			}
		}
		return true
	}
	if has('w') {
		b.Face(sector, blv.PolyWall, 0, [3]int32{one, 0, 0}, [3]int16{x1, y1, z1}, [3]int16{x1, y2, z1}, [3]int16{x1, y2, z2}, [3]int16{x1, y1, z2})
	}
	if has('e') {
		b.Face(sector, blv.PolyWall, 0, [3]int32{-one, 0, 0}, [3]int16{x2, y1, z1}, [3]int16{x2, y1, z2}, [3]int16{x2, y2, z2}, [3]int16{x2, y2, z1})
	}
	if has('s') {
		b.Face(sector, blv.PolyWall, 0, [3]int32{0, one, 0}, [3]int16{x1, y1, z1}, [3]int16{x1, y1, z2}, [3]int16{x2, y1, z2}, [3]int16{x2, y1, z1})
	}
	if has('n') {
		b.Face(sector, blv.PolyWall, 0, [3]int32{0, -one, 0}, [3]int16{x1, y2, z1}, [3]int16{x2, y2, z1}, [3]int16{x2, y2, z2}, [3]int16{x1, y2, z2})
	}
}

func abs(v int32) int32 {
	if v < 0 {
		return -v
	}
	return v
}

func div16(a, b int32) int32 { return int32((int64(a) << 16) / int64(b)) }
