// Package blv parses indoor maps (games.lod *.blv) after lod.UnpackMap. The layout is
// the games.lod branch of Blv_Load (0x498050); see re/notes/blv.md.
//
// World coordinates are the same as outdoors: x east, y north, z up, int16 vertices.
// Sector 0 is a dummy; faces, sectors and doors refer to each other by index.
package blv

import (
	"fmt"

	"libre-enroth/internal/maps/binread"
)

// Pool sizes of Blv_Alloc (0x497e8b); the loader does not check them, but a file that
// exceeds them would overflow the game's buffers.
const (
	MaxVertices    = 15000
	MaxFaces       = 10000
	MaxFaceExtras  = 5000
	MaxSectors     = 512
	MaxLights      = 400
	MaxDoors       = 200
	MaxNodes       = 5000
	MaxOutlines    = 7000
	MaxDecorations = 3000
	MaxFaceVerts   = 255 // numVerts is a byte
)

// Vec3s is an int16 world position.
type Vec3s struct{ X, Y, Z int16 }

// Header is the 0x88-byte map header; only the sizes of the variable-length blocks are
// known.
type Header struct {
	FDataSize  int32 // +0x68: 6 u16 arrays of numVerts+1 per face
	RDataSize  int32 // +0x6c: sector face/decoration lists
	RLDataSize int32 // +0x70: sector light lists
	DDataSize  int32 // +0x74: door arrays (stored in the .dlv)
}

// Face is one 0x60-byte face.
type Face struct {
	NormalF  [3]float32 // +0x00
	DistF    float32    // +0x0c
	Normal   [3]int32   // +0x10: 16.16
	Dist     int32      // +0x1c: 16.16, n.p + dist = 0 on the plane
	ZCalc    [3]int32   // +0x20: floors z = (a*x + b*y + c + 0x8000) >> 16
	Attr     uint32     // +0x2c: Face* bits
	Verts    []uint16   // L.FData; the stored arrays repeat the first vertex at the end
	XDisp    []int16
	YDisp    []int16
	ZDisp    []int16
	U, V     []int16  // texels
	Extra    int16    // +0x48: FaceExtras index
	Sector   int16    // +0x4c
	Back     int16    // +0x4e: the sector on the other side of a portal
	BBox     [6]int16 // +0x50: x1 x2 y1 y2 z1 z2
	PolyType uint8    // +0x5c: Poly* values
	Texture  string   // from the name list after FData
}

// Face attribute bits (Indoor_DrawFaceHW 0x4b0ef5, Indoor_FaceTexOffset 0x4b12db,
// Door_UpdateAll 0x46f475, Face_TextureAxes 0x497c5a).
const (
	FacePortal      = 0x1
	FaceScrollDown  = 0x4
	FaceAlignTop    = 0x8
	FaceFluid       = 0x10
	FaceScrollUp    = 0x20
	FaceScrollRight = 0x40
	FaceScrollLeft  = 0x800
	FaceAlignLeft   = 0x1000
	FaceInvisible   = 0x2000
	FaceAnimated    = 0x4000 // Texture names a dtft.bin sequence
	FaceAlignRight  = 0x8000
	FaceAlignBottom = 0x20000
	FaceDoorTexture = 0x40000  // the texture moves with its door
	FaceSky         = 0x400000 // the outdoor sky shows through
	FaceFlipU       = 0x800000
	FaceFlipV       = 0x1000000
)

// Face polygon types.
const (
	PolyWall        = 1
	PolyFloor       = 3
	PolySlopedFloor = 4
	PolyCeiling     = 5
	PolySlopedCeil  = 6
)

// FaceExtra is one 0x24-byte face extra.
type FaceExtra struct {
	Face    int16  // +0x0c
	TexDU   int16  // +0x14: texel offset added to the face u
	TexDV   int16  // +0x16
	Cog     int16  // +0x18
	Event   uint16 // +0x1a
	Texture string // from the second name list
	Raw     [0x24]byte
}

// Sector is one 0x78-byte sector. The lists index Map.Faces, except Decorations
// (Map.Decorations) and Lights (Map.Lights).
type Sector struct {
	Flags          uint32 // +0x00: SectorUsesBSP
	Floors         []int16
	Walls          []int16
	Ceilings       []int16
	Fluids         []int16
	Portals        []int16
	Faces          []int16 // every face of the sector
	NumNonBSPFaces int     // +0x32: Faces[:n] are drawn directly, the rest via the BSP
	Cogs           []int16
	Decorations    []int16
	Markers        []int16
	Lights         []int16  // L.RLData
	WaterLevel     int16    // +0x60
	MistLevel      int16    // +0x62
	LightDistMul   int16    // +0x64
	MinAmbient     int16    // +0x66: dim 0..31; base grey 0xf8 - 8*dim
	FirstBSPNode   int16    // +0x68
	ExitTag        int16    // +0x6a
	BBox           [6]int16 // +0x6c: x1 x2 y1 y2 z1 z2
}

// SectorUsesBSP marks sectors whose faces beyond NumNonBSPFaces are walked through the
// BSP from FirstBSPNode (Indoor_AddSectorFaces 0x43e3aa).
const SectorUsesBSP = 0x10

// Light is one 0x14-byte static light.
type Light struct {
	Pos        Vec3s // +0x00
	Radius     int16 // +0x06
	R, G, B    uint8 // +0x08
	Type       uint8 // +0x0b: set to 5 on load
	Flags      uint16
	Brightness int16
}

// LightOff marks a disabled light (Indoor_GatherFaceLights 0x45b962).
const LightOff = 0x8

// BSPNode is one 8-byte BSP node.
type BSPNode struct {
	Front, Back int16 // -1 = none
	FirstFace   int16 // index into the sector's Faces
	NumFaces    int16
}

// Decoration is one 0x20-byte level decoration (same layout as outdoors).
type Decoration struct {
	Name  string // from the name list; the game resolves it to a ddeclist.bin index
	Flags uint16 // +0x02: overwritten from the .dlv
	Pos   [3]int32
	Yaw   int32
	Raw   [0x20]byte
}

// Spawn is one 0x18-byte spawn point.
type Spawn struct {
	Pos  [3]int32
	Kind uint16 // +0x0e
	Raw  [0x18]byte
}

// Outline is one 0xc-byte automap line.
type Outline struct {
	V1, V2       uint16
	Face1, Face2 uint16
	Z            int16
	Flags        uint16 // 1 = revealed (set from the .dlv bits)
}

// Map is a parsed indoor map.
type Map struct {
	Header
	Vertices    []Vec3s
	Faces       []Face
	FaceExtras  []FaceExtra
	Sectors     []Sector
	NumDoors    int // the doors themselves are in the .dlv
	Decorations []Decoration
	Lights      []Light
	Nodes       []BSPNode
	Spawns      []Spawn
	Outlines    []Outline
}

// Parse decodes an unpacked .blv blob.
//
// mm8: 0x498050 (Blv_Load, games.lod branch)
func Parse(b []byte) (*Map, error) {
	r := binread.New(b)
	m := &Map{}
	h := r.Record(0x88)
	m.Header = Header{h.I32(0x68), h.I32(0x6c), h.I32(0x70), h.I32(0x74)}

	nv := r.Count("vertices", 6, MaxVertices)
	m.Vertices = make([]Vec3s, nv)
	for i := range m.Vertices {
		v := r.Record(6)
		m.Vertices[i] = Vec3s{v.I16(0), v.I16(2), v.I16(4)}
	}

	nf := r.Count("faces", 0x60, MaxFaces)
	recs := make([]binread.Rec, nf)
	for i := range recs {
		recs[i] = r.Record(0x60)
	}
	fdata := binread.New(r.Bytes(int(m.FDataSize)))
	m.Faces = make([]Face, nf)
	for i, f := range recs {
		face := &m.Faces[i]
		for k := range 3 {
			face.NormalF[k] = f.F32(4 * k)
			face.Normal[k] = f.I32(0x10 + 4*k)
			face.ZCalc[k] = f.I32(0x20 + 4*k)
		}
		face.DistF, face.Dist = f.F32(0xc), f.I32(0x1c)
		face.Attr = f.U32(0x2c)
		face.Extra, face.Sector, face.Back = f.I16(0x48), f.I16(0x4c), f.I16(0x4e)
		for k := range face.BBox {
			face.BBox[k] = f.I16(0x50 + 2*k)
		}
		face.PolyType = f.U8(0x5c)
		n := int(f.U8(0x5d))
		// mm8: 0x498050 face pointer fix-up: six arrays of n+1 u16 each
		arr := func() []int16 {
			a := binread.Rec(fdata.Bytes(2 * (n + 1)))
			out := make([]int16, n)
			for k := range out {
				out[k] = a.I16(2 * k)
			}
			return out
		}
		vs := arr()
		face.Verts = make([]uint16, n)
		for k, v := range vs {
			face.Verts[k] = uint16(v)
			if int(face.Verts[k]) >= nv {
				return nil, fmt.Errorf("blv: face %d: vertex %d out of range (%d)", i, face.Verts[k], nv)
			}
		}
		face.XDisp, face.YDisp, face.ZDisp = arr(), arr(), arr()
		face.U, face.V = arr(), arr()
	}
	if err := fdata.Err(); err != nil {
		return nil, fmt.Errorf("blv: FData: %w", err)
	}
	if fdata.Len() != 0 {
		return nil, fmt.Errorf("blv: FData: %d bytes unused", fdata.Len())
	}
	for i := range m.Faces {
		m.Faces[i].Texture = r.Record(10).Str(0, 10)
	}

	nx := r.Count("face extras", 0x24+10, MaxFaceExtras)
	m.FaceExtras = make([]FaceExtra, nx)
	for i := range m.FaceExtras {
		x := r.Record(0x24)
		fx := &m.FaceExtras[i]
		copy(fx.Raw[:], x)
		fx.Face, fx.TexDU, fx.TexDV = x.I16(0xc), x.I16(0x14), x.I16(0x16)
		fx.Cog, fx.Event = x.I16(0x18), x.U16(0x1a)
	}
	for i := range m.FaceExtras {
		m.FaceExtras[i].Texture = r.Record(10).Str(0, 10)
	}

	ns := r.Count("sectors", 0x78, MaxSectors)
	srecs := make([]binread.Rec, ns)
	for i := range srecs {
		srecs[i] = r.Record(0x78)
	}
	rdata := binread.New(r.Bytes(int(m.RDataSize)))
	rldata := binread.New(r.Bytes(int(m.RLDataSize)))
	list := func(rd *binread.Reader, n int16) []int16 {
		if n < 0 {
			rd.Fail("negative list length %d", n)
			return nil
		}
		a := binread.Rec(rd.Bytes(2 * int(n)))
		out := make([]int16, n)
		for k := range out {
			out[k] = a.I16(2 * k)
		}
		return out
	}
	m.Sectors = make([]Sector, ns)
	for i, s := range srecs {
		sec := &m.Sectors[i]
		sec.Flags = s.U32(0)
		// mm8: 0x498050 sector pointer fix-up, in this order
		sec.Floors = list(rdata, s.I16(0x08))
		sec.Walls = list(rdata, s.I16(0x10))
		sec.Ceilings = list(rdata, s.I16(0x18))
		sec.Fluids = list(rdata, s.I16(0x20))
		sec.Portals = list(rdata, s.I16(0x28))
		sec.Faces = list(rdata, s.I16(0x30))
		sec.NumNonBSPFaces = int(s.I16(0x32))
		sec.Cogs = list(rdata, s.I16(0x40))
		sec.Decorations = list(rdata, s.I16(0x48))
		sec.Markers = list(rdata, s.I16(0x50))
		sec.Lights = list(rldata, s.I16(0x58))
		sec.WaterLevel, sec.MistLevel = s.I16(0x60), s.I16(0x62)
		sec.LightDistMul, sec.MinAmbient = s.I16(0x64), s.I16(0x66)
		sec.FirstBSPNode, sec.ExitTag = s.I16(0x68), s.I16(0x6a)
		for k := range sec.BBox {
			sec.BBox[k] = s.I16(0x6c + 2*k)
		}
	}
	for what, rd := range map[string]*binread.Reader{"RData": rdata, "RLData": rldata} {
		if err := rd.Err(); err != nil {
			return nil, fmt.Errorf("blv: %s: %w", what, err)
		}
		if rd.Len() != 0 {
			return nil, fmt.Errorf("blv: %s: %d bytes unused", what, rd.Len())
		}
	}

	m.NumDoors = r.Count("doors", 0, MaxDoors)
	nd := r.Count("decorations", 0x40, MaxDecorations)
	m.Decorations = make([]Decoration, nd)
	for i := range m.Decorations {
		d := r.Record(0x20)
		dec := &m.Decorations[i]
		copy(dec.Raw[:], d)
		dec.Flags = d.U16(2)
		dec.Pos = [3]int32{d.I32(4), d.I32(8), d.I32(0xc)}
		dec.Yaw = d.I32(0x10)
	}
	for i := range m.Decorations {
		m.Decorations[i].Name = r.Record(0x20).Str(0, 0x20)
	}

	nl := r.Count("lights", 0x14, MaxLights)
	m.Lights = make([]Light, nl)
	for i := range m.Lights {
		l := r.Record(0x14)
		m.Lights[i] = Light{
			Pos: Vec3s{l.I16(0), l.I16(2), l.I16(4)}, Radius: l.I16(6),
			R: l.U8(8), G: l.U8(9), B: l.U8(0xa), Type: l.U8(0xb),
			Flags: l.U16(0xc), Brightness: l.I16(0xe),
		}
	}

	nn := r.Count("bsp nodes", 8, MaxNodes)
	m.Nodes = make([]BSPNode, nn)
	for i := range m.Nodes {
		n := r.Record(8)
		m.Nodes[i] = BSPNode{n.I16(0), n.I16(2), n.I16(4), n.I16(6)}
	}

	nsp := r.Count("spawns", 0x18, 1<<16)
	m.Spawns = make([]Spawn, nsp)
	for i := range m.Spawns {
		s := r.Record(0x18)
		copy(m.Spawns[i].Raw[:], s)
		m.Spawns[i].Pos = [3]int32{s.I32(0), s.I32(4), s.I32(8)}
		m.Spawns[i].Kind = s.U16(0xe)
	}

	no := r.Count("outlines", 0xc, MaxOutlines)
	m.Outlines = make([]Outline, no)
	for i := range m.Outlines {
		o := r.Record(0xc)
		m.Outlines[i] = Outline{o.U16(0), o.U16(2), o.U16(4), o.U16(6), o.I16(8), o.U16(0xa)}
	}

	if err := r.Err(); err != nil {
		return nil, fmt.Errorf("blv: %w", err)
	}
	if r.Len() != 0 {
		return nil, fmt.Errorf("blv: %d trailing bytes", r.Len())
	}
	return m, m.check()
}

// check validates every cross-reference so the renderer can index without guards.
func (m *Map) check() error {
	nf, ns, nx := len(m.Faces), len(m.Sectors), len(m.FaceExtras)
	for i := range m.Faces {
		f := &m.Faces[i]
		if int(f.Sector) < 0 || int(f.Sector) >= ns {
			return fmt.Errorf("blv: face %d: sector %d out of range (%d)", i, f.Sector, ns)
		}
		if f.Attr&FacePortal != 0 && (int(f.Back) < 0 || int(f.Back) >= ns) {
			return fmt.Errorf("blv: portal face %d: back sector %d out of range (%d)", i, f.Back, ns)
		}
		if int(f.Extra) < 0 || int(f.Extra) >= nx {
			return fmt.Errorf("blv: face %d: extra %d out of range (%d)", i, f.Extra, nx)
		}
	}
	faces := func(i int, what string, l []int16) error {
		for _, f := range l {
			if int(f) < 0 || int(f) >= nf {
				return fmt.Errorf("blv: sector %d: %s face %d out of range (%d)", i, what, f, nf)
			}
		}
		return nil
	}
	for i := range m.Sectors {
		s := &m.Sectors[i]
		for what, l := range map[string][]int16{
			"floor": s.Floors, "wall": s.Walls, "ceiling": s.Ceilings, "fluid": s.Fluids,
			"portal": s.Portals, "": s.Faces,
		} {
			if err := faces(i, what, l); err != nil {
				return err
			}
		}
		if s.NumNonBSPFaces < 0 || s.NumNonBSPFaces > len(s.Faces) {
			return fmt.Errorf("blv: sector %d: %d non-BSP faces of %d", i, s.NumNonBSPFaces, len(s.Faces))
		}
		for _, d := range s.Decorations {
			if int(d) < 0 || int(d) >= len(m.Decorations) {
				return fmt.Errorf("blv: sector %d: decoration %d out of range", i, d)
			}
		}
		for _, l := range s.Lights {
			if int(l) < 0 || int(l) >= len(m.Lights) {
				return fmt.Errorf("blv: sector %d: light %d out of range", i, l)
			}
		}
		if s.Flags&SectorUsesBSP != 0 {
			if int(s.FirstBSPNode) < 0 || int(s.FirstBSPNode) >= len(m.Nodes) {
				return fmt.Errorf("blv: sector %d: bsp node %d out of range", i, s.FirstBSPNode)
			}
		}
	}
	for i, n := range m.Nodes {
		for _, c := range []int16{n.Front, n.Back} {
			if c != -1 && (int(c) < 0 || int(c) >= len(m.Nodes)) {
				return fmt.Errorf("blv: bsp node %d: child %d out of range", i, c)
			}
		}
		if n.FirstFace < 0 || n.NumFaces < 0 {
			return fmt.Errorf("blv: bsp node %d: faces %d+%d", i, n.FirstFace, n.NumFaces)
		}
	}
	return nil
}
