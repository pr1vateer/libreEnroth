// Package odm parses outdoor maps (games.lod *.odm) after lod.UnpackMap. The layout is
// the games.lod branch of Odm_Load (0x47df28); see re/notes/odm.md.
//
// World coordinates: x east, y north, z up; one terrain cell is 512 units and cell
// (gx, gy) of the 128 x 128 grid has its north-west corner at ((gx-64)*512, (64-gy)*512).
package odm

import (
	"fmt"
	"math"

	"libre-enroth/internal/maps/binread"
)

// Grid is the terrain size in cells per side.
const Grid = 128

// CellSize is the edge of a terrain cell in world units.
const CellSize = 512

// HeightScale converts a heightmap byte to world z.
//
// mm8: 0x47ffea (height = heightmap[gy*128+gx] << 5)
const HeightScale = 32

// Limits the loader enforces ("Can't load file!").
const (
	MaxDecorations = 3000
	maxModels      = 1000 // not checked by the original; a sanity bound
	maxFaces       = 1 << 16
	maxVertices    = 1 << 16
	MaxFaceVerts   = 20
)

// Vec3 is an integer world position.
type Vec3 struct{ X, Y, Z int32 }

// Tileset is one of the header's 4 terrain tilesets.
type Tileset struct {
	Group  int16 // TTtype_*: a tileset of the tile table
	Offset int16 // first tile index of that group (recomputed on load, 0x480728)
}

// Header is the 0xb4-byte map header.
type Header struct {
	Name     string // +0x00 char[32] ("blank")
	FileName string // +0x20 char[32] ("default.odm")
	Desc     string // +0x40 char[31]
	TileMode uint8  // +0x5f: 1 = dtile2.bin, 2 = dtile3.bin, else dtile.bin
	Sky      string // +0x60 char[32]: only used by the dev-tree loader
	Ground   string // +0x80 char[32]: overwritten with "grastyl"
	Tilesets [4]Tileset
}

// Face is one 0x134-byte BModel face.
type Face struct {
	Normal [3]int32 // +0x00: 16.16
	Dist   int32    // +0x0c: 16.16 plane distance
	// ZCalc gives the z of a sloped floor or ceiling: (x*a >> 16) + (y*b >> 16) + (c >> 16)
	// (Outdoor_FloorZ 0x46d6bb).
	ZCalc [3]int32 // +0x10
	Attr  uint32   // +0x1c: FaceInvisible, FaceAnimated, scroll bits, ...
	// Verts (+0x20 u16[20]) indexes BModel.Vertices. Verts and the displacements
	// (+0x98, +0xc0, +0xe8 i16[20]) have numVerts entries plus, within their capacity,
	// the entry [numVerts] that the collision code reads (internal/game/physics): the
	// stored closing vertex (== Verts[0] in every shipped face with fewer than 20), or for
	// a 20-vertex face whatever follows the array, as the original reads it.
	Verts               []uint16
	XDisp, YDisp, ZDisp []int16
	U, V                []int16  // +0x48, +0x70 i16[20]: texture coordinates in texels
	TexDU               int16    // +0x112: texture offset
	TexDV               int16    // +0x114
	BBox                [6]int16 // +0x116: x1 x2 y1 y2 z1 z2
	Cog                 int16    // +0x122
	Event               int16    // +0x124
	PolyType            uint8    // +0x12f: 1 wall, 3 floor, 4 sloped floor, 5 ceiling, 6 sloped ceiling
	Texture             string   // from the name list after the BSP nodes
}

// Face attribute bits.
const (
	FaceScrollDown  = 0x4    // texture v scrolls down (0x47893a: poly flag 0x400)
	FaceScrollUp    = 0x20   // v scrolls up (0x800)
	FaceScrollLeft  = 0x40   // u scrolls (0x1000)
	FaceScrollRight = 0x800  // u scrolls (0x2000)
	FaceFluid       = 0x10   // water/lava: animated HDWTR/HDLAV in D3D (poly flag 2)
	FaceInvisible   = 0x2000 // not drawn
	FaceAnimated    = 0x4000 // Texture names a dtft.bin sequence
)

// BSPNode is an 8-byte BModel BSP node (kept raw for now).
type BSPNode [8]byte

// BModel is one building model (0xbc-byte header, then its arrays).
type BModel struct {
	Name, Name2 string // +0x00, +0x20 char[32]
	Flags       uint32 // +0x40
	Position    Vec3   // +0x70: the line-of-sight test's model centre (0x408520)
	BBoxMin     Vec3   // +0x7c: bounding box (Outdoor_FloorZ 0x46d6bb, Collide_Models 0x46ead9)
	BBoxMax     Vec3   // +0x88
	Center      Vec3   // +0xac: bounding sphere (0x479a4c culls with it)
	Radius      int32  // +0xb8
	Vertices    []Vec3
	Faces       []Face
	Order       []int16 // +0x58 array, u16 per face: uninitialised junk in the shipped maps
	BSP         []BSPNode
}

// Decoration is one 0x20-byte level decoration.
type Decoration struct {
	Name     string // from the name list; the game resolves it to a ddeclist.bin index
	Flags    uint16 // +0x02; overwritten from the .ddm
	Pos      Vec3   // +0x04
	Yaw      int32  // +0x10: 2048 units
	Cog      int16  // +0x14: what SetSprite (evt 0x0d) addresses
	Event    int16  // +0x16: map event on click, Space or proximity
	Radius   int16  // +0x18: proximity trigger radius
	Degrees  int16  // +0x1a: start heading in degrees (Party Start markers)
	EventVar int16  // +0x1c: interactive decorations: the map variable holding the event
	Raw      [0x20]byte
}

// Decoration flags (FUN_0047b61b, the minimap FUN_0043f7f4).
const (
	DecTriggerParty  = 0x1 // proximity: the event runs while the party is within Radius
	DecTriggerActor  = 0x2 // ... an actor (M8)
	DecTriggerObject = 0x4 // ... an object (M9)
	DecVisibleOnMap  = 0x8 // also set when Space ran its event
	DecInvisible     = 0x20
)

// Spawn is one 0x18-byte spawn point.
type Spawn struct {
	Pos    Vec3   // +0x00
	Radius uint16 // +0x0c
	Kind   uint16 // +0x0e: 3 = monsters, anything else an item (Level_Load 0x45f895)
	Index  uint16 // +0x10: the MapStats monster slot / the item level
	Attrib uint16 // +0x12: 1 = only on an alert map
	Group  uint32 // +0x14: the monsters' group
}

// Map is a parsed outdoor map.
type Map struct {
	Header
	Heights, Tiles, Attrs [Grid * Grid]byte // row-major [gy*128+gx]
	// NormalIdx holds two TerNorm indices per cell for its two triangles. Cell (gx, gy)
	// uses entry (gx*128 + gy + 1)*2 + k: k = 1 for the triangle (gx,gy+1)-(gx+1,gy+1)-
	// (gx,gy), k = 0 for (gx+1,gy)-(gx,gy)-(gx+1,gy+1). See TriangleNormal.
	NormalIdx   [Grid * Grid * 2]uint16
	Normals     [][3]float32 // "TerNorm"
	Models      []BModel
	Decorations []Decoration
	FaceIDs     []uint16            // "IDLIST": BModel faces per cell, for collision (M4)
	CellFaces   [Grid * Grid]uint32 // offsets into FaceIDs
	Spawns      []Spawn
}

// Parse decodes an unpacked .odm blob.
//
// mm8: 0x47df28 (Odm_Load, games.lod branch)
func Parse(b []byte) (*Map, error) {
	r := binread.New(b)
	m := &Map{}
	h := r.Record(0xb4)
	m.Header = Header{
		Name: h.Str(0, 32), FileName: h.Str(0x20, 32), Desc: h.Str(0x40, 31), TileMode: h.U8(0x5f),
		Sky: h.Str(0x60, 32), Ground: h.Str(0x80, 32),
	}
	for i := range m.Tilesets {
		m.Tilesets[i] = Tileset{Group: h.I16(0xa0 + 4*i), Offset: h.I16(0xa2 + 4*i)}
	}
	copy(m.Heights[:], r.Bytes(Grid*Grid))
	copy(m.Tiles[:], r.Bytes(Grid*Grid))
	copy(m.Attrs[:], r.Bytes(Grid*Grid))
	nNorm := r.Count("normals", 0, 1<<20)
	r.Skip(0x20000) // a per-triangle grid the game reads but never uses
	if nb := r.Bytes(len(m.NormalIdx) * 2); nb != nil {
		for i := range m.NormalIdx {
			m.NormalIdx[i] = uint16(nb[2*i]) | uint16(nb[2*i+1])<<8
		}
	}
	if r.Len() < nNorm*12 && r.Err() == nil {
		r.Fail("normals: %d x 12 bytes exceed the %d bytes left", nNorm, r.Len())
	}
	m.Normals = make([][3]float32, 0, nNorm)
	for range nNorm {
		n := r.Record(12)
		m.Normals = append(m.Normals, [3]float32{n.F32(0), n.F32(4), n.F32(8)})
	}

	nModels := r.Count("bmodels", 0xbc, maxModels)
	hdrs := make([]binread.Rec, nModels)
	for i := range hdrs {
		hdrs[i] = r.Record(0xbc)
	}
	m.Models = make([]BModel, nModels)
	for i, mh := range hdrs {
		if err := parseModel(r, mh, &m.Models[i]); err != nil {
			return nil, fmt.Errorf("odm: bmodel %d: %w", i, err)
		}
	}

	nDec := r.Count("decorations", 0x40, MaxDecorations)
	m.Decorations = make([]Decoration, nDec)
	for i := range m.Decorations {
		d := r.Record(0x20)
		dec := &m.Decorations[i]
		copy(dec.Raw[:], d)
		dec.Flags = d.U16(2)
		dec.Pos = Vec3{d.I32(4), d.I32(8), d.I32(0xc)}
		dec.Yaw = d.I32(0x10)
		dec.Cog, dec.Event, dec.Radius = d.I16(0x14), d.I16(0x16), d.I16(0x18)
		dec.Degrees, dec.EventVar = d.I16(0x1a), d.I16(0x1c)
	}
	for i := range m.Decorations {
		m.Decorations[i].Name = r.Record(0x20).Str(0, 0x20)
	}

	nIDs := r.Count("face id list", 2, 1<<20)
	ids := r.Bytes(2 * nIDs)
	m.FaceIDs = make([]uint16, nIDs)
	for i := range m.FaceIDs {
		m.FaceIDs[i] = uint16(ids[2*i]) | uint16(ids[2*i+1])<<8
	}
	if cf := binread.Rec(r.Bytes(4 * Grid * Grid)); cf != nil {
		for i := range m.CellFaces {
			m.CellFaces[i] = cf.U32(4 * i)
		}
	}
	nSpawn := r.Count("spawns", 0x18, 1<<16)
	m.Spawns = make([]Spawn, nSpawn)
	for i := range m.Spawns {
		s := r.Record(0x18)
		m.Spawns[i] = Spawn{Pos: Vec3{s.I32(0), s.I32(4), s.I32(8)}, Radius: s.U16(0xc), Kind: s.U16(0xe),
			Index: s.U16(0x10), Attrib: s.U16(0x12), Group: s.U32(0x14)}
	}
	if err := r.Err(); err != nil {
		return nil, fmt.Errorf("odm: %w", err)
	}
	if r.Len() != 0 {
		return nil, fmt.Errorf("odm: %d trailing bytes", r.Len())
	}
	for i, n := range m.NormalIdx {
		if int(n) >= len(m.Normals) && i >= 2 {
			// The game guards lookups (index > count-1 -> no normal) but the shipped
			// maps never need it; report it so a bad file is noticed.
			return nil, fmt.Errorf("odm: normal index %d at %d >= %d normals", n, i, len(m.Normals))
		}
	}
	return m, nil
}

func parseModel(r *binread.Reader, h binread.Rec, m *BModel) error {
	m.Name, m.Name2 = h.Str(0, 32), h.Str(0x20, 32)
	m.Flags = h.U32(0x40)
	m.Position = Vec3{h.I32(0x70), h.I32(0x74), h.I32(0x78)}
	m.BBoxMin = Vec3{h.I32(0x7c), h.I32(0x80), h.I32(0x84)}
	m.BBoxMax = Vec3{h.I32(0x88), h.I32(0x8c), h.I32(0x90)}
	m.Center = Vec3{h.I32(0xac), h.I32(0xb0), h.I32(0xb4)}
	m.Radius = h.I32(0xb8)
	nv, nf, nn := int(h.I32(0x44)), int(h.I32(0x4c)), int(h.I32(0x5c))
	if nv < 0 || nv > maxVertices || nf < 0 || nf > maxFaces || nn < 0 || nn > maxFaces*4 {
		return fmt.Errorf("bad counts: %d vertices, %d faces, %d nodes", nv, nf, nn)
	}
	if need := nv*12 + nf*(0x134+2+10) + nn*8; need > r.Len() {
		return fmt.Errorf("arrays need %d bytes, %d left", need, r.Len())
	}
	m.Vertices = make([]Vec3, nv)
	for i := range m.Vertices {
		v := r.Record(12)
		m.Vertices[i] = Vec3{v.I32(0), v.I32(4), v.I32(8)}
	}
	m.Faces = make([]Face, nf)
	for i := range m.Faces {
		f := r.Record(0x134)
		face := &m.Faces[i]
		face.Normal = [3]int32{f.I32(0), f.I32(4), f.I32(8)}
		face.Dist = f.I32(0xc)
		face.Attr = f.U32(0x1c)
		n := int(f.U8(0x12e))
		if n > MaxFaceVerts {
			return fmt.Errorf("face %d: %d vertices", i, n)
		}
		face.ZCalc = [3]int32{f.I32(0x10), f.I32(0x14), f.I32(0x18)}
		face.Verts = make([]uint16, n+1)
		face.XDisp, face.YDisp, face.ZDisp = make([]int16, n+1), make([]int16, n+1), make([]int16, n+1)
		face.U = make([]int16, n)
		face.V = make([]int16, n)
		for k := range n + 1 { // [n] runs into the next field when n == 20
			face.Verts[k] = f.U16(0x20 + 2*k)
			face.XDisp[k], face.YDisp[k], face.ZDisp[k] = f.I16(0x98+2*k), f.I16(0xc0+2*k), f.I16(0xe8+2*k)
			if k == n {
				break
			}
			face.U[k] = f.I16(0x48 + 2*k)
			face.V[k] = f.I16(0x70 + 2*k)
			if int(face.Verts[k]) >= nv {
				return fmt.Errorf("face %d: vertex %d out of range (%d)", i, face.Verts[k], nv)
			}
		}
		face.Verts = face.Verts[:n]
		face.XDisp, face.YDisp, face.ZDisp = face.XDisp[:n], face.YDisp[:n], face.ZDisp[:n]
		face.TexDU, face.TexDV = f.I16(0x112), f.I16(0x114)
		for k := range face.BBox {
			face.BBox[k] = f.I16(0x116 + 2*k)
		}
		face.Cog = f.I16(0x122)
		face.Event = f.I16(0x124)
		face.PolyType = f.U8(0x12f)
	}
	m.Order = make([]int16, nf)
	for i := range m.Order {
		m.Order[i] = r.Record(2).I16(0)
	}
	m.BSP = make([]BSPNode, nn)
	for i := range m.BSP {
		copy(m.BSP[i][:], r.Bytes(8))
	}
	for i := range m.Faces {
		m.Faces[i].Texture = r.Record(10).Str(0, 10)
	}
	return r.Err()
}

// Height returns the terrain height at grid corner (gx, gy), 0 outside the grid.
//
// mm8: 0x47ffea
func (m *Map) Height(gx, gy int) int {
	if gx < 0 || gx >= Grid || gy < 0 || gy >= Grid {
		return 0
	}
	return int(m.Heights[gy*Grid+gx]) * HeightScale
}

// CellCorner returns the world x, y of grid corner (gx, gy).
//
// mm8: 0x480754 / 0x480761 are the inverse: gx = (x >> 9) + 64, gy = 64 - (y >> 9)
func CellCorner(gx, gy int) (x, y int) { return (gx - 64) * CellSize, (64 - gy) * CellSize }

// TriangleNormal returns the stored normal of triangle k of cell (gx, gy) (see
// NormalIdx), and false if the cell has none.
//
// mm8: 0x4815f0 (D3D terrain: 0x79b25c + (gx*128 + gy+1)*4 [+2])
func (m *Map) TriangleNormal(gx, gy, k int) ([3]float32, bool) {
	i := (gx*Grid+gy+1)*2 + k
	if i < 0 || i >= len(m.NormalIdx) || int(m.NormalIdx[i]) >= len(m.Normals) {
		return [3]float32{}, false
	}
	return m.Normals[m.NormalIdx[i]], true
}

// ResolveTilesets recomputes the tileset offsets from the tile table as the loader
// does (first is desc.Tiles.First).
//
// mm8: 0x480728, 0x4806ed
func (m *Map) ResolveTilesets(first func(group int) int) {
	for i := range m.Tilesets {
		m.Tilesets[i].Offset = int16(first(int(m.Tilesets[i].Group)))
	}
}

// TileIndex maps the tilemap value of cell (gx, gy) to an index into the tile table:
// values below 90 are direct, 90..197 select one of the first three header tilesets in
// groups of 36, and 198 and above the fourth (roads).
//
// mm8: 0x47fe7d
func (m *Map) TileIndex(gx, gy int) int {
	if gx < 0 || gx >= Grid || gy < 0 || gy >= Grid {
		return 0
	}
	t := int(m.Tiles[gy*Grid+gx])
	switch {
	case t >= 0xc6:
		return t - 0xc6 + int(m.Tilesets[3].Offset)
	case t >= 0x5a:
		g := (t - 0x5a) / 36
		return t - 0x5a - 36*g + int(m.Tilesets[g].Offset)
	}
	return t
}

// GroundZ is the terrain height at world x, y, interpolated on the cell's two
// triangles (0 outside the grid).
func (m *Map) GroundZ(x, y float64) float64 {
	fx := x/CellSize + 64
	fy := 64 - y/CellSize
	gx, gy := int(math.Floor(fx)), int(math.Floor(fy))
	if gx < 0 || gy < 0 || gx >= Grid-1 || gy >= Grid-1 {
		return 0
	}
	u, v := fx-float64(gx), fy-float64(gy) // u east, v south within the cell
	h00, h10 := float64(m.Height(gx, gy)), float64(m.Height(gx+1, gy))
	h01, h11 := float64(m.Height(gx, gy+1)), float64(m.Height(gx+1, gy+1))
	if v >= u { // triangle (gx,gy+1)-(gx+1,gy+1)-(gx,gy)
		return h00 + (h11-h01)*u + (h01-h00)*v
	}
	return h00 + (h10-h00)*u + (h11-h10)*v // triangle (gx+1,gy)-(gx,gy)-(gx+1,gy+1)
}
