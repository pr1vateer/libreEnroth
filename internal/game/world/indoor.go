package world

import (
	"fmt"
	"math"
	"strings"

	"libre-enroth/internal/assets/desc"
	"libre-enroth/internal/game/physics"
	"libre-enroth/internal/maps/blv"
	"libre-enroth/internal/maps/delta"
	"libre-enroth/internal/render"
)

// Indoor is a loaded .blv with its .dlv state and what drawing it needs.
type Indoor struct {
	Map   *blv.Map
	Delta *delta.Delta

	t      *Tables
	tex    *TextureCache
	decIdx []int // ddeclist.bin index of each decoration (0 = unknown)
	// Per-face texture coordinates and offsets as the D3D renderer uses them; door faces
	// get theirs recomputed when their door moves (Door_UpdateAll).
	faceU, faceV   [][]int16
	faceDU, faceDV []int16
	decLights      []render.PointLight // lights of decorations (Level_Load)
	geo            *physics.IndoorGeo  // floors and collision for the party

	// per-frame scratch
	secRect  []rect
	secSeen  []bool
	queue    []portalEntry
	drawn    []bool
	lightBuf []render.PointLight
	vbuf     []render.Vertex
}

// rect is a screen rectangle in frame pixels, [x0, x1) x [y0, y1).
type rect struct{ x0, y0, x1, y1 float64 }

func (r rect) empty() bool { return r.x0 >= r.x1 || r.y0 >= r.y1 }

func (r rect) intersect(o rect) rect {
	return rect{max(r.x0, o.x0), max(r.y0, o.y0), min(r.x1, o.x1), min(r.y1, o.y1)}
}

func (r rect) union(o rect) rect {
	if r.empty() {
		return o
	}
	return rect{min(r.x0, o.x0), min(r.y0, o.y0), max(r.x1, o.x1), max(r.y1, o.y1)}
}

func (r rect) contains(o rect) bool {
	return !r.empty() && o.x0 >= r.x0 && o.y0 >= r.y0 && o.x1 <= r.x1 && o.y1 <= r.y1
}

// portalEntry is a sector seen through a portal, clipped to rect.
type portalEntry struct {
	sector int
	r      rect
	via    int // the portal face it was entered through, -1 for the camera's sector
}

// Limits of the portal walk (Indoor_AddFaceHW 0x4b137c).
const maxPortalEntries = 150

// NewIndoor parses an unpacked .blv and its .dlv (nil for none) and applies the delta.
//
// mm8: 0x498050 (Blv_Load), 0x45f895 (Level_Load: door reset, decoration lights)
func NewIndoor(t *Tables, tex *TextureCache, blvBlob, dlvBlob []byte) (*Indoor, error) {
	m, err := blv.Parse(blvBlob)
	if err != nil {
		return nil, err
	}
	in := &Indoor{Map: m, t: t, tex: tex}
	if dlvBlob != nil {
		d, err := delta.Parse(dlvBlob, delta.DLV, len(m.Faces), len(m.Decorations), m.DDataSize)
		if err != nil {
			return nil, err
		}
		in.Delta = d
		for i := range m.Faces {
			m.Faces[i].Attr = d.FaceAttrs[i]
		}
		for i := range m.Decorations {
			m.Decorations[i].Flags = d.DecFlags[i]
		}
		for i := range m.Outlines {
			if d.Revealed[i/8]&(0x80>>(i%8)) != 0 {
				m.Outlines[i].Flags |= 1
			}
		}
	}
	in.faceU = make([][]int16, len(m.Faces))
	in.faceV = make([][]int16, len(m.Faces))
	in.faceDU = make([]int16, len(m.Faces))
	in.faceDV = make([]int16, len(m.Faces))
	for i := range m.Faces {
		f := &m.Faces[i]
		in.faceU[i], in.faceV[i] = f.U, f.V
		x := &m.FaceExtras[f.Extra]
		in.faceDU[i], in.faceDV[i] = x.TexDU, x.TexDV
	}
	in.decIdx = make([]int, len(m.Decorations))
	for i, d := range m.Decorations {
		in.decIdx[i] = t.Decs.Find(d.Name)
		if di := in.decIdx[i]; di > 0 && d.Flags&decHidden == 0 {
			dec := &t.Decs[di]
			if dec.Flags&decNoDraw == 0 && dec.LightRadius != 0 {
				in.decLights = append(in.decLights, pointLight(
					float64(d.Pos[0]), float64(d.Pos[1]), float64(int(d.Pos[2])+dec.Height/2),
					dec.LightRadius, dec.R, dec.G, dec.B))
			}
		}
	}
	in.geo = &physics.IndoorGeo{Map: m, Decorations: collisionDecorations(t.Decs, in.decIdx, len(m.Decorations),
		func(i int) (uint16, [3]int32) { return m.Decorations[i].Flags, m.Decorations[i].Pos })}
	in.settleDoors()
	in.secRect = make([]rect, len(m.Sectors))
	in.secSeen = make([]bool, len(m.Sectors))
	in.drawn = make([]bool, len(m.Faces))
	return in, nil
}

// settleDoors restarts the doors so the next update puts them in place, as Level_Load
// does on every map load.
//
// mm8: 0x45f895 (Level_Load: door reset)
func (in *Indoor) settleDoors() {
	if in.Delta == nil {
		return
	}
	for i := range in.Delta.Doors {
		d := &in.Delta.Doors[i]
		if len(d.Verts) == 0 {
			continue
		}
		d.Settle()
	}
	in.UpdateDoors(0)
}

// Decoration flags (LevelDecoration +0x02 and DecorationDesc +0x4a).
const (
	decHidden = 0x20 // level decoration: not drawn, no light
	decNoDraw = 0x2  // descriptor: a marker without sprite or light
)

// pointLight converts a light record: colour bytes 0..255, all zero meaning white
// (LightPoly_Build 0x45affd substitutes 0xffffff for a zero colour).
func pointLight(x, y, z float64, radius int, r, g, b uint8) render.PointLight {
	if r == 0 && g == 0 && b == 0 {
		r, g, b = 255, 255, 255
	}
	return render.PointLight{X: x, Y: y, Z: z, Radius: float64(radius),
		R: float64(r) / 255, G: float64(g) / 255, B: float64(b) / 255}
}

// UpdateDoors advances moving doors by ticks and moves their vertices: distance =
// speed*time/128 clamped to MoveLength, vertex = offset + dir*distance. Their faces get
// new planes and, as on the D3D path, texture coordinates projected on the face's
// texture axes.
//
// mm8: 0x46f475 (Door_UpdateAll)
func (in *Indoor) UpdateDoors(ticks int) {
	if in.Delta == nil {
		return
	}
	m := in.Map
	for i := range in.Delta.Doors {
		d := &in.Delta.Doors[i]
		if d.State == delta.DoorClosed || d.State == delta.DoorOpen {
			d.Attr &^= delta.DoorMoving
			continue
		}
		if d.Attr&delta.DoorStopped != 0 || len(d.Verts) == 0 {
			continue
		}
		d.Time += int32(ticks)
		dist, done := d.Distance()
		if done {
			if d.State == delta.DoorClosing {
				d.State = delta.DoorClosed
			} else {
				d.State = delta.DoorOpen
			}
		}
		for j, vi := range d.Verts {
			if j >= len(d.XOffsets) {
				break
			}
			m.Vertices[vi] = blv.Vec3s{
				X: d.XOffsets[j] + int16(dist*d.Dir[0]>>16),
				Y: d.YOffsets[j] + int16(dist*d.Dir[1]>>16),
				Z: d.ZOffsets[j] + int16(int64(dist)*int64(d.Dir[2])>>16),
			}
		}
		for j, fi := range d.Faces {
			in.doorFace(int(fi), d, j, dist)
		}
	}
}

// doorFace recomputes a door face after its vertices moved.
//
// mm8: 0x46f475 (plane, then the g_hwRender branch: u/v = vertex . axis >> 16, aligned
// by attr 0x1000/0x8000/0x8/0x20000, or offset with the door for attr 0x40000)
func (in *Indoor) doorFace(fi int, d *delta.Door, j int, dist int32) {
	m := in.Map
	f := &m.Faces[fi]
	if len(f.Verts) == 0 {
		return
	}
	v0 := m.Vertices[f.Verts[0]]
	f.Dist = -(f.Normal[0]*int32(v0.X) + f.Normal[1]*int32(v0.Y) + f.Normal[2]*int32(v0.Z))
	f.DistF = -(float32(v0.X)*f.NormalF[0] + float32(v0.Y)*f.NormalF[1] + float32(v0.Z)*f.NormalF[2])
	if f.Normal[2] != 0 {
		f.ZCalc[2] = -int32((int64(f.Dist) << 16) / int64(f.Normal[2]))
	}
	ua, va := faceAxes(f)
	u, v := make([]int16, len(f.Verts)), make([]int16, len(f.Verts))
	minU, minV, maxU, maxV := int32(math.MaxInt32), int32(math.MaxInt32), int32(math.MinInt32), int32(math.MinInt32)
	for k, vi := range f.Verts {
		p := m.Vertices[vi]
		pu := (int32(p.X)*ua[0] + int32(p.Y)*ua[1] + int32(p.Z)*ua[2]) >> 16
		pv := (int32(p.X)*va[0] + int32(p.Y)*va[1] + int32(p.Z)*va[2]) >> 16
		u[k], v[k] = int16(pu), int16(pv)
		minU, maxU = min(minU, pu), max(maxU, pu)
		minV, maxV = min(minV, pv), max(maxV, pv)
	}
	in.faceU[fi], in.faceV[fi] = u, v
	var du, dv int32
	tex := in.tex.Bitmap(f.Texture)
	switch {
	case f.Attr&blv.FaceAlignLeft != 0:
		du -= minU
	case f.Attr&blv.FaceAlignRight != 0 && tex != nil:
		du -= int32(tex.OrigW) + maxU
	}
	switch {
	case f.Attr&blv.FaceAlignTop != 0:
		dv -= minV
	case f.Attr&blv.FaceAlignBottom != 0 && tex != nil:
		dv -= int32(tex.OrigH) + maxV
	}
	if f.Attr&blv.FaceDoorTexture != 0 && j < len(d.DeltaU) {
		dot := func(a [3]int32) int32 {
			return int32(int64(a[0])*int64(d.Dir[0])>>16 + int64(a[1])*int64(d.Dir[1])>>16 + int64(a[2])*int64(d.Dir[2])>>16)
		}
		du = -int32(int64(dist)*int64(dot(ua))>>16) + int32(d.DeltaU[j])
		dv = int32(d.DeltaV[j]) - int32(int64(dist)*int64(dot(va))>>16)
	}
	in.faceDU[fi], in.faceDV[fi] = int16(du), int16(dv)
}

// faceAxes is the texture u and v direction of a face (16.16): walls run u along the
// wall and v down, floors and ceilings u east and v south; attr bits flip them.
//
// mm8: 0x497c5a (Face_TextureAxes)
func faceAxes(f *blv.Face) (u, v [3]int32) {
	switch f.PolyType {
	case blv.PolyWall:
		u, v = [3]int32{-f.Normal[1], f.Normal[0], 0}, [3]int32{0, 0, -0x10000}
	case blv.PolySlopedFloor, blv.PolySlopedCeil:
		if abs32(f.Normal[2]) < 0xb569 {
			x, y := float64(-f.Normal[1]), float64(f.Normal[0])
			l := math.Hypot(x, y)
			if l == 0 {
				l = 1
			}
			u = [3]int32{int32(x / l * 65536), int32(y / l * 65536), 0}
			v = [3]int32{0, 0, -0x10000}
			break
		}
		fallthrough
	default:
		u, v = [3]int32{0x10000, 0, 0}, [3]int32{0, -0x10000, 0}
	}
	if f.Attr&blv.FaceFlipU != 0 {
		u = [3]int32{-u[0], -u[1], -u[2]}
	}
	if f.Attr&blv.FaceFlipV != 0 {
		v = [3]int32{-v[0], -v[1], -v[2]}
	}
	return
}

// ToggleDoors starts every closed door opening and every open one closing (debug key).
func (in *Indoor) ToggleDoors() {
	if in.Delta == nil {
		return
	}
	for i := range in.Delta.Doors {
		d := &in.Delta.Doors[i]
		switch d.State {
		case delta.DoorClosed:
			d.State, d.Time = delta.DoorOpening, 0
		case delta.DoorOpen:
			d.State, d.Time = delta.DoorClosing, 0
		default:
			continue
		}
		d.Attr |= delta.DoorMoving
	}
}

// SectorAt is the sector containing a point, 0 for none (physics.IndoorGeo.SectorAt
// at the point rounded down).
//
// mm8: 0x499f5f (Indoor_GetSector)
func (in *Indoor) SectorAt(x, y, z float64) int {
	return in.Geo().SectorAt(int32(math.Floor(x)), int32(math.Floor(y)), int32(math.Floor(z)))
}

// Geo is the map's collision and floor geometry (without decorations for an Indoor
// built by hand).
func (in *Indoor) Geo() *physics.IndoorGeo {
	if in.geo == nil {
		in.geo = &physics.IndoorGeo{Map: in.Map}
	}
	return in.geo
}

// DefaultCamera stands on the map's "Party Start" marker decoration, facing its yaw;
// without one, in the middle of the first floor of sector 1, looking east.
func (in *Indoor) DefaultCamera() FreeCam {
	m := in.Map
	for _, d := range m.Decorations {
		if strings.EqualFold(d.Name, "Party Start") {
			return FreeCam{X: float64(d.Pos[0]), Y: float64(d.Pos[1]), Z: float64(d.Pos[2]), Yaw: float64(d.Yaw & 2047)}
		}
	}
	if len(m.Sectors) > 1 {
		for _, fi := range m.Sectors[1].Floors {
			f := &m.Faces[fi]
			if len(f.Verts) < 3 {
				continue
			}
			var x, y float64
			minZ := math.Inf(1)
			for _, vi := range f.Verts {
				v := m.Vertices[vi]
				x += float64(v.X)
				y += float64(v.Y)
				minZ = min(minZ, float64(v.Z))
			}
			n := float64(len(f.Verts))
			return FreeCam{X: math.Round(x / n), Y: math.Round(y / n), Z: minZ}
		}
	}
	return FreeCam{}
}

// visibleSectors walks the portals from the camera's sector: a portal facing the camera
// whose projection overlaps the rectangle its sector was seen through opens the sector
// behind it, clipped to the overlap. Unlike the original, which clips every face to the
// frustum of its portal, faces are drawn whole and the z-buffer sorts them out; a sector
// is only revisited when it is seen through a part of the screen not covered before.
// Returns false when the camera is outside every sector.
//
// mm8: 0x43d61a (Indoor_PortalVisibility), 0x43e3aa, 0x4b137c (Indoor_AddFaceHW)
func (in *Indoor) visibleSectors(cam *render.Camera, full rect) bool {
	m := in.Map
	clear(in.secSeen)
	clear(in.secRect)
	start := in.SectorAt(cam.X, cam.Y, cam.Z)
	if start == 0 {
		return false
	}
	in.queue = append(in.queue[:0], portalEntry{start, full, -1})
	in.secSeen[start], in.secRect[start] = true, full
	for qi := 0; qi < len(in.queue) && len(in.queue) < maxPortalEntries; qi++ {
		e := in.queue[qi]
		for _, fi := range m.Sectors[e.sector].Portals {
			if int(fi) == e.via {
				continue
			}
			f := &m.Faces[fi]
			next := int(f.Sector)
			if next == e.sector {
				next = int(f.Back)
			}
			if next <= 0 {
				continue
			}
			var r rect
			if qi == 0 && in.standingIn(f, cam) {
				r = e.r
			} else {
				v0 := m.Vertices[f.Verts[0]]
				d := float64(f.Normal[0])*(float64(v0.X)-cam.X) + float64(f.Normal[1])*(float64(v0.Y)-cam.Y) +
					float64(f.Normal[2])*(float64(v0.Z)-cam.Z)
				if int(f.Sector) != e.sector {
					d = -d
				}
				if d >= 0 {
					continue
				}
				pr, ok := in.project(f, cam)
				if !ok {
					continue
				}
				r = pr.intersect(e.r)
				if r.empty() {
					continue
				}
			}
			if in.secRect[next].contains(r) {
				continue
			}
			in.secSeen[next] = true
			in.secRect[next] = in.secRect[next].union(r)
			if len(in.queue) < maxPortalEntries {
				in.queue = append(in.queue, portalEntry{next, r, int(fi)})
			}
		}
	}
	return true
}

// standingIn reports the camera inside a portal's box (16 units of slack) and within 9
// units of its plane: the sector behind is then seen through the whole view.
//
// mm8: 0x4b137c (first-entry case)
func (in *Indoor) standingIn(f *blv.Face, cam *render.Camera) bool {
	b := f.BBox
	if cam.X < float64(b[0])-16 || cam.X > float64(b[1])+16 || cam.Y < float64(b[2])-16 ||
		cam.Y > float64(b[3])+16 || cam.Z < float64(b[4])-16 || cam.Z > float64(b[5])+16 {
		return false
	}
	d := float64(f.Normal[0])*cam.X + float64(f.Normal[1])*cam.Y + float64(f.Normal[2])*cam.Z + float64(f.Dist)
	return math.Abs(d) <= 0x90000
}

// project returns the screen bounding box of a face clipped to the near plane.
func (in *Indoor) project(f *blv.Face, cam *render.Camera) (rect, bool) {
	m := in.Map
	type cp struct{ d, l, u float64 }
	pts := make([]cp, 0, len(f.Verts)+4)
	for _, vi := range f.Verts {
		v := m.Vertices[vi]
		d, l, u := cam.View(float64(v.X), float64(v.Y), float64(v.Z))
		pts = append(pts, cp{d, l, u})
	}
	r := rect{math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)}
	add := func(p cp) {
		x, y := cam.Project(p.d, p.l, p.u)
		r = rect{min(r.x0, x), min(r.y0, y), max(r.x1, x), max(r.y1, y)}
	}
	any := false
	for i, a := range pts {
		b := pts[(i+1)%len(pts)]
		if a.d >= cam.Near {
			add(a)
			any = true
		}
		if (a.d >= cam.Near) != (b.d >= cam.Near) {
			t := (cam.Near - a.d) / (b.d - a.d)
			add(cp{cam.Near, a.l + t*(b.l-a.l), a.u + t*(b.u-a.u)})
			any = true
		}
	}
	if !any {
		return rect{}, false
	}
	r.x0, r.y0 = math.Floor(r.x0), math.Floor(r.y0)
	r.x1, r.y1 = math.Ceil(r.x1)+1, math.Ceil(r.y1)+1
	return r, true
}

// Torch is the party's light indoors: radius 800 (times the Torch Light power, 1
// without the spell), white.
//
// mm8: 0x43d049 (Lights_AddMobile at the camera, radius 800 * power)
const torchRadius = 800

// Draw queues the faces of the visible sectors and their decorations. Outside every
// sector (free camera) all sectors are drawn.
//
// mm8: 0x43d049 (Indoor_DrawWorld), 0x4b0ef5 (Indoor_DrawFaceHW)
func (in *Indoor) Draw(r *render.Renderer, cam *render.Camera, f *render.Frame, clk Clock, billboards func(torch *render.PointLight)) {
	m := in.Map
	full := rect{0, 0, float64(f.W), float64(f.H)}
	if !in.visibleSectors(cam, full) {
		for i := range in.secSeen {
			in.secSeen[i] = i > 0
		}
	}
	clear(in.drawn)
	in.lightBuf = in.lightBuf[:0]
	torch := render.PointLight{X: cam.X, Y: cam.Y, Z: cam.Z, Radius: torchRadius, R: 1, G: 1, B: 1}
	ms := clk.Ticks * 1000 / TicksPerSecond
	var sky *Tex
	for si, seen := range in.secSeen {
		if !seen {
			continue
		}
		for _, fi := range m.Sectors[si].Faces {
			if in.drawn[fi] {
				continue
			}
			in.drawn[fi] = true
			if t := in.drawFace(r, cam, int(fi), &torch, ms, clk); t != nil {
				sky = t
			}
		}
	}
	if sky != nil {
		k := float64(sky.W) / float64(sky.OrigW)
		drift := float64(clk.Ticks) * 0xe0 / 65536
		r.Sky(sky.Texture, 512, k/8, drift*k, drift*k, 1.0/32, 1)
	}
	if billboards != nil {
		billboards(&torch)
	}
	in.drawDecorations(r, cam, &torch, clk)
}

// drawFace queues one face. Sky faces are not drawn; their texture is returned so the
// sky plane shows through.
func (in *Indoor) drawFace(r *render.Renderer, cam *render.Camera, fi int, torch *render.PointLight, ms int, clk Clock) *Tex {
	m := in.Map
	f := &m.Faces[fi]
	if len(f.Verts) < 3 || f.Attr&(blv.FaceInvisible|blv.FacePortal) != 0 || f.Texture == "" {
		return nil
	}
	v0 := m.Vertices[f.Verts[0]]
	if float64(f.Normal[0])*(float64(v0.X)-cam.X)+float64(f.Normal[1])*(float64(v0.Y)-cam.Y)+
		float64(f.Normal[2])*(float64(v0.Z)-cam.Z) >= 0 {
		return nil // Face_IsBackfacing 0x4347d8
	}
	name := f.Texture
	if f.Attr&blv.FaceAnimated != 0 {
		if seq := in.t.TFT.Find(name); seq >= 0 {
			name = in.t.TFT[in.t.TFT.At(seq, clk.Ticks)].Name
		}
	}
	tex := in.tex.Bitmap(name)
	if tex == nil {
		return nil
	}
	if f.Attr&blv.FaceSky != 0 {
		return tex
	}
	// Texture offset and scrolling (Indoor_FaceTexOffset 0x4b12db: GetTickCount >> 3
	// masked by the texture size).
	du, dv := int(in.faceDU[fi]), int(in.faceDV[fi])
	scroll := ms >> 3
	switch {
	case f.Attr&blv.FaceScrollDown != 0:
		dv -= scroll & (tex.OrigH - 1)
	case f.Attr&blv.FaceScrollUp != 0:
		dv += scroll & (tex.OrigH - 1)
	}
	switch {
	case f.Attr&blv.FaceScrollLeft != 0:
		du -= scroll & (tex.OrigW - 1)
	case f.Attr&blv.FaceScrollRight != 0:
		du += scroll & (tex.OrigW - 1)
	}
	if f.Attr&blv.FaceFluid != 0 {
		if strings.EqualFold(name, "wtrtyl") {
			if t := in.tex.Bitmap(fmt.Sprintf("HDWTR%03d", waterFrame(clk.Ticks))); t != nil {
				tex = t
			}
		} else {
			// Lava and the like bob along v (0x4b0ef5).
			dv += (tex.OrigH - 1) & (cos2048((ms>>2)-512) >> 8)
		}
	}
	// Light: the sector's ambient dim gives the base grey, the lights add in colour.
	dim := int(m.Sectors[f.Sector].MinAmbient)
	base := float64(max(0xf8-8*dim, 0)) / 255
	lights := in.faceLights(f, torch)
	us, vs := in.faceU[fi], in.faceV[fi]
	verts := in.vbuf[:0]
	for k, vi := range f.Verts {
		p := m.Vertices[vi]
		verts = append(verts, render.Vertex{
			X: float64(p.X), Y: float64(p.Y), Z: float64(p.Z),
			U: float64(int(us[k])+du) / float64(tex.OrigW),
			V: float64(int(vs[k])+dv) / float64(tex.OrigH),
			L: base,
		})
	}
	in.vbuf = verts
	var flags render.Flags
	if tex.Alpha {
		flags = render.AlphaTest
	}
	r.SetID(facePID(fi))
	r.PolygonLit(verts, tex.Texture, flags, lights)
	r.SetID(0)
	return nil
}

// faceLights gathers up to 20 lights that reach a face: the torch, then the sector's
// static lights that are on, then decoration lights.
//
// mm8: 0x45b962 (Indoor_GatherFaceLights), 0x45ba81 (Light_AddIfTouches)
func (in *Indoor) faceLights(f *blv.Face, torch *render.PointLight) []render.PointLight {
	const maxLights = 20
	start := len(in.lightBuf)
	add := func(l *render.PointLight, mobile bool) {
		if len(in.lightBuf)-start >= maxLights || l.Radius <= 0 {
			return
		}
		b := f.BBox
		if l.X <= float64(b[0])-l.Radius || l.X >= float64(b[1])+l.Radius ||
			l.Y <= float64(b[2])-l.Radius || l.Y >= float64(b[3])+l.Radius ||
			l.Z <= float64(b[4])-l.Radius || l.Z >= float64(b[5])+l.Radius {
			return
		}
		d := float64(f.NormalF[0])*l.X + float64(f.NormalF[1])*l.Y + float64(f.NormalF[2])*l.Z + float64(f.DistF)
		if (!mobile && d < 0) || d > l.Radius {
			return
		}
		in.lightBuf = append(in.lightBuf, *l)
	}
	add(torch, true)
	m := in.Map
	for _, li := range m.Sectors[f.Sector].Lights {
		sl := &m.Lights[li]
		if sl.Flags&blv.LightOff != 0 {
			continue
		}
		l := pointLight(float64(sl.Pos.X), float64(sl.Pos.Y), float64(sl.Pos.Z), int(sl.Radius), sl.R, sl.G, sl.B)
		add(&l, false)
	}
	for i := range in.decLights {
		add(&in.decLights[i], false)
	}
	if len(in.lightBuf) == start {
		return nil
	}
	return in.lightBuf[start:len(in.lightBuf):len(in.lightBuf)]
}

// drawDecorations queues the billboards of the decorations in visible sectors, lit by
// their sector's ambient and the torch.
//
// mm8: 0x43d6fa (Indoor_AddDecorationBillboard)
func (in *Indoor) drawDecorations(r *render.Renderer, cam *render.Camera, torch *render.PointLight, clk Clock) {
	m := in.Map
	for si, seen := range in.secSeen {
		if !seen {
			continue
		}
		s := &m.Sectors[si]
		for _, di := range s.Decorations {
			d := &m.Decorations[di]
			idx := in.decIdx[di]
			if d.Flags&decHidden != 0 || idx <= 0 {
				continue
			}
			dec := &in.t.Decs[idx]
			if dec.Flags&(decNoSprite|decNoDraw) != 0 {
				continue
			}
			x, y, z := float64(d.Pos[0]), float64(d.Pos[1]), float64(d.Pos[2])
			depth := cam.Depth(x, y, z)
			if depth < 4 {
				continue
			}
			fi := in.t.SFT.At(dec.SFT, clk.Ticks+abs(int(d.Pos[0]+d.Pos[1])))
			if fi < 0 || fi >= len(in.t.SFT.Frames) {
				continue
			}
			fr := &in.t.SFT.Frames[fi]
			ang := int(math.Round(math.Atan2(y-cam.Y, x-cam.X) * 2048 / (2 * math.Pi)))
			view := ((int(d.Yaw) + 128) - ang + 1024) >> 8 & 7
			names, mirror := fr.ViewNames()
			t := in.tex.Sprite(names[view])
			if t == nil {
				continue
			}
			bb := billboard(t, fr, mirror[view])
			bb.X, bb.Y, bb.Z = x, y, z
			bb.L = 1
			if fr.Flags&desc.FrameLuminous == 0 {
				bb.L = in.billboardLight(si, x, y, z, torch)
			}
			r.SetID(decorationPID(int(di)))
			r.Billboard(&bb)
			r.SetID(0)
		}
	}
}

// billboardLight is a sprite's light in sector si: the sector's ambient plus the party's
// torch.
//
// mm8: 0x43d6fa (Indoor_AddDecorationBillboard)
func (in *Indoor) billboardLight(si int, x, y, z float64, torch *render.PointLight) float64 {
	l := 0.0
	if si > 0 && si < len(in.Map.Sectors) {
		l = float64(max(0xf8-8*int(in.Map.Sectors[si].MinAmbient), 0)) / 255
	}
	if dist := math.Sqrt((x-torch.X)*(x-torch.X) + (y-torch.Y)*(y-torch.Y) + (z-torch.Z)*(z-torch.Z)); dist < torch.Radius {
		l += 1 - dist/torch.Radius
	}
	return min(l, 1)
}

// MapMarkers reports nothing yet: the indoor automap draws outlines (M6).
func (in *Indoor) MapMarkers(fn func(x, y float64)) {}
