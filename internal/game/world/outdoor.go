package world

import (
	"fmt"
	"math"
	"strings"

	"libre-enroth/internal/assets/desc"
	"libre-enroth/internal/game/physics"
	"libre-enroth/internal/maps/delta"
	"libre-enroth/internal/maps/odm"
	"libre-enroth/internal/render"
)

// Outdoor is a loaded .odm with what drawing it needs.
type Outdoor struct {
	Map      *odm.Map
	Delta    *delta.Delta
	MapIndex int // MapStats.txt row
	SkyName  string

	t      *Tables
	tex    *TextureCache
	tiles  desc.Tiles
	decIdx []int // ddeclist.bin index of each decoration (0 = unknown)
	geo    *physics.OutdoorGeo
	vbuf   []render.Vertex
}

// NewOutdoor parses an unpacked .odm and its .ddm (nil for none), applies the delta's
// face attributes and decoration flags, and resolves its tables.
//
// mm8: 0x47df28 (Odm_Load)
func NewOutdoor(t *Tables, tex *TextureCache, name string, blob, ddm []byte) (*Outdoor, error) {
	m, err := odm.Parse(blob)
	if err != nil {
		return nil, err
	}
	o := &Outdoor{Map: m, t: t, tex: tex}
	if ddm != nil {
		faces := 0
		for _, mod := range m.Models {
			faces += len(mod.Faces)
		}
		d, err := delta.Parse(ddm, delta.DDM, faces, len(m.Decorations), 0)
		if err != nil {
			return nil, err
		}
		o.Delta = d
		i := 0
		for mi := range m.Models {
			for fi := range m.Models[mi].Faces {
				m.Models[mi].Faces[fi].Attr = d.FaceAttrs[i]
				i++
			}
		}
		for i := range m.Decorations {
			m.Decorations[i].Flags = d.DecFlags[i]
		}
	}
	// The header's tile mode picks the tile table (0x47df28: +0x5f == 2 -> dtile3.bin,
	// == 1 -> dtile2.bin, else dtile.bin).
	switch m.TileMode {
	case 2:
		o.tiles = t.Tiles[2]
	case 1:
		o.tiles = t.Tiles[1]
	default:
		o.tiles = t.Tiles[0]
	}
	m.ResolveTilesets(o.tiles.First)
	o.decIdx = make([]int, len(m.Decorations))
	for i, d := range m.Decorations {
		o.decIdx[i] = t.Decs.Find(d.Name)
	}
	o.MapIndex = t.MapIndex(name)
	o.SkyName = skyName(o.MapIndex)
	o.geo = physics.NewOutdoorGeo(m, o.TileAttr, collisionDecorations(t.Decs, o.decIdx, len(m.Decorations),
		func(i int) (uint16, [3]int32) {
			d := &m.Decorations[i]
			return d.Flags, [3]int32{d.Pos.X, d.Pos.Y, d.Pos.Z}
		}))
	o.geo.PlaneOfWater = strings.EqualFold(name, "elemw.odm")
	if o.Delta != nil {
		t := o.Delta.Time[0x20:]
		o.geo.FlyCeiling = int32(uint32(t[0]) | uint32(t[1])<<8 | uint32(t[2])<<16 | uint32(t[3])<<24)
	}
	return o, nil
}

// TileAttr is the dtile.bin attribute of the tile in cell (gx, gy), 0 outside.
//
// mm8: 0x47ff84 (Odm_TileAttr)
func (o *Outdoor) TileAttr(gx, gy int) uint16 {
	if gx < 0 || gx >= odm.Grid || gy < 0 || gy >= odm.Grid {
		return 0
	}
	if i := o.Map.TileIndex(gx, gy); i >= 0 && i < len(o.tiles) {
		return uint16(o.tiles[i].Attr)
	}
	return 0
}

// Geo is the map's terrain, floor and collision geometry.
func (o *Outdoor) Geo() *physics.OutdoorGeo { return o.geo }

// skyNames is the sky texture table at 0x4fe130.
var skyNames = [...]string{"plansky3", "sky6pm", "cloudsabove", "stormclds", "sunsetclouds",
	"Sky01", "sky02", "sky03", "sky04", "sky05", "sky06", "plansky1"}

// skyName picks the sky of a map (by MapStats index) for a fresh game: Dagger Wound
// (1) shows sunset clouds until quest bit 0xe4 is set, Shadowspire (6) storm clouds,
// Plane of Fire (11) sunset clouds, the rest plansky3. The game re-rolls the sky of
// most maps when a day passes between visits (20% overcast, else plansky3/plansky1);
// that needs the clock and saves (M5/M10).
//
// mm8: 0x47fd2e, 0x47fdc0
func skyName(mapIndex int) string {
	switch mapIndex {
	case 1, 11:
		return skyNames[4]
	case 6:
		return skyNames[3]
	}
	return skyNames[0]
}

// DefaultCamera is the party's position on a new game for out01, else the map centre
// at 1500 units up, looking north.
//
// mm8: 0x45f682 (Game_NewGame: x 0xeb6, y 0x1de1, z 0x221, yaw 0x200)
func (o *Outdoor) DefaultCamera() FreeCam {
	if o.MapIndex == 1 {
		return FreeCam{X: 0xeb6, Y: 0x1de1, Z: 0x221, Yaw: 0x200}
	}
	return FreeCam{X: 0, Y: 0, Z: float64(o.Map.Height(64, 64)) + 1500, Yaw: 512}
}

// MapMarkers reports the decorations the minimap shows.
//
// mm8: 0x43f7f4 (white dots for decorations with flag 8)
func (o *Outdoor) MapMarkers(fn func(x, y float64)) {
	for _, d := range o.Map.Decorations {
		if d.Flags&odm.DecVisibleOnMap != 0 {
			fn(float64(d.Pos.X), float64(d.Pos.Y))
		}
	}
}

// Draw queues the scene: sky, terrain, buildings, decorations.
//
// mm8: 0x47b25b (outdoor D3D: 0x479f2d sky, 0x47893a buildings, 0x4808b6 terrain,
// 0x47b61b decorations)
func (o *Outdoor) Draw(r *render.Renderer, cam *render.Camera, clk Clock) {
	sun := SunAt(clk.Hour, clk.Minute)
	o.drawSky(r, clk)
	o.drawTerrain(r, cam, &sun, clk)
	o.drawModels(r, cam, &sun, clk)
	o.drawDecorations(r, cam, &sun, clk)
}

// drawSky: a plane 512 units above the eye, 8 world units per texel, drifting 0x1c0/65536
// texels per tick along both axes; rays flatter than 1/32 are mirrored.
//
// mm8: 0x479f2d
func (o *Outdoor) drawSky(r *render.Renderer, clk Clock) {
	t := o.tex.Bitmap(o.SkyName)
	if t == nil {
		return
	}
	k := float64(t.W) / float64(t.OrigW) // hwl bitmaps are stored at half size
	drift := float64(clk.Ticks) * 0x1c0 / 65536
	r.Sky(t.Texture, 512, k/8, drift*k, drift*k, 1.0/32, 1)
}

// waterFrame is the frame of the 7-frame HDWTR/HDLAV/HWOIL animations.
//
// mm8: 0x43ed51 (Render_Frame: ((ticks >> 2) & 31) / 31 * 6, rounded)
func waterFrame(ticks int) int {
	return int(math.RoundToEven(float64(float32(float32((ticks>>2)&31)*0.032258064) * 6)))
}

// animatedSet picks the animated texture family of a water/lava cell from the tileset
// of the cell north of it and the map's tile mode.
//
// mm8: 0x48242c
func (o *Outdoor) animatedSet(gx, gy int) string {
	ts := -1
	if i := o.Map.TileIndex(gx, gy-1); i >= 0 && i < len(o.tiles) {
		ts = o.tiles[i].Tileset
	}
	switch o.Map.TileMode {
	case 2:
		if ts == 3 {
			return "HWOIL"
		}
		if ts == 2 {
			return "HDLAV"
		}
	case 1:
		return "HDLAV"
	default:
		if ts == 9 {
			return "HDLAV"
		}
	}
	return "HDWTR"
}

func isFluidTexture(name string) bool {
	switch strings.ToLower(name) {
	case "wtrtyl", "lavtyl", "tartyl":
		return true
	}
	return false
}

// Tile attribute bits (tile.def TTattr_*; FUN_0048940e).
const (
	tileWater  = 0x2
	tileShore  = 0x100 // TTattr_Water2: animated water under a transparent shore tile
	terrainFar = 17    // cells around the camera (the mist distance is 16 cells)
)

// drawTerrain queues the cells within the mist distance. A cell is one quad when its
// corners are level, else two triangles split along (gx,gy)-(gx+1,gy+1), each lit by
// its stored normal. Texture coordinates run 0..1 east (u) and south (v) per cell.
//
// mm8: 0x4808b6 (cell walk), 0x4815f0 (D3D cell draw)
func (o *Outdoor) drawTerrain(r *render.Renderer, cam *render.Camera, sun *Sun, clk Clock) {
	m := o.Map
	cgx := int(math.Floor(cam.X/odm.CellSize)) + 64
	cgy := 64 - int(math.Ceil(cam.Y/odm.CellSize))
	far2 := (cam.Far + odm.CellSize) * (cam.Far + odm.CellSize)
	frame := fmt.Sprintf("%03d", waterFrame(clk.Ticks))
	corner := func(gx, gy int, u, v float64) render.Vertex {
		x, y := odm.CellCorner(gx, gy)
		return render.Vertex{X: float64(x), Y: float64(y), Z: float64(m.Height(gx, gy)), U: u, V: v}
	}
	for gy := max(cgy-terrainFar, 0); gy < min(cgy+terrainFar, odm.Grid-1); gy++ {
		for gx := max(cgx-terrainFar, 0); gx < min(cgx+terrainFar, odm.Grid-1); gx++ {
			cx, cy := odm.CellCorner(gx, gy)
			dx, dy := float64(cx+odm.CellSize/2)-cam.X, float64(cy-odm.CellSize/2)-cam.Y
			if dx*dx+dy*dy > far2 {
				continue
			}
			ti := m.TileIndex(gx, gy)
			if ti < 0 || ti >= len(o.tiles) {
				continue
			}
			tile := &o.tiles[ti]
			tex := o.tex.Bitmap(tile.Name)
			var under *Tex
			switch {
			case tile.Attr&tileShore != 0:
				under = o.tex.Bitmap(o.animatedSet(gx, gy) + frame)
			case tile.Attr&tileWater != 0 && isFluidTexture(tile.Name):
				if t := o.tex.Bitmap(o.animatedSet(gx, gy) + frame); t != nil {
					tex = t
				}
			}
			if tex == nil {
				continue
			}
			p00, p01 := corner(gx, gy, 0, 0), corner(gx, gy+1, 0, 1)
			p11, p10 := corner(gx+1, gy+1, 1, 1), corner(gx+1, gy, 1, 0)
			if p00.Z == p01.Z && p01.Z == p11.Z && p11.Z == p10.Z {
				o.terrainPoly(r, cam, sun, []render.Vertex{p00, p01, p11, p10}, gx, gy, 1, tex, under)
				continue
			}
			o.terrainPoly(r, cam, sun, []render.Vertex{p01, p11, p00}, gx, gy, 1, tex, under)
			o.terrainPoly(r, cam, sun, []render.Vertex{p10, p00, p11}, gx, gy, 0, tex, under)
		}
	}
}

func (o *Outdoor) terrainPoly(r *render.Renderer, cam *render.Camera, sun *Sun, vs []render.Vertex, gx, gy, k int, tex, under *Tex) {
	n, ok := o.Map.TriangleNormal(gx, gy, k)
	dim := 20
	if ok {
		dim = sun.TerrainDim(n)
	} else {
		n = [3]float32{0, 0, 1}
	}
	// Only the side facing the camera is drawn (0x48327e).
	v := vs[0]
	if float64(n[0])*(cam.X-v.X)+float64(n[1])*(cam.Y-v.Y)+float64(n[2])*(cam.Z-v.Z) <= 0 {
		return
	}
	for i := range vs {
		vs[i].L = sun.OutdoorLight(dim, cam.Depth(vs[i].X, vs[i].Y, vs[i].Z))
	}
	if under != nil {
		r.Polygon(vs, under.Texture, 0)
		r.Polygon(vs, tex.Texture, render.AlphaTest)
		return
	}
	r.Polygon(vs, tex.Texture, 0)
}

// drawModels queues the visible faces of the buildings within reach.
//
// mm8: 0x47893a (RenderBuildings, D3D), 0x479a4c (model culling)
func (o *Outdoor) drawModels(r *render.Renderer, cam *render.Camera, sun *Sun, clk Clock) {
	scroll := clk.Ticks * 125 / 256 // GetTickCount() >> 4 at 128 ticks per second
	frame := fmt.Sprintf("%03d", waterFrame(clk.Ticks))
	for mi := range o.Map.Models {
		mod := &o.Map.Models[mi]
		dx, dy := float64(mod.Center.X)-cam.X, float64(mod.Center.Y)-cam.Y
		if math.Hypot(dx, dy)-float64(mod.Radius) > cam.Far+0x800 {
			continue
		}
		for fi := range mod.Faces {
			f := &mod.Faces[fi]
			if f.Attr&odm.FaceInvisible != 0 || len(f.Verts) < 3 {
				continue
			}
			v0 := mod.Vertices[f.Verts[0]]
			nx, ny, nz := float64(f.Normal[0])/65536, float64(f.Normal[1])/65536, float64(f.Normal[2])/65536
			if nx*(cam.X-float64(v0.X))+ny*(cam.Y-float64(v0.Y))+nz*(cam.Z-float64(v0.Z)) <= 0 {
				continue
			}
			name := f.Texture
			if f.Attr&odm.FaceAnimated != 0 {
				if seq := o.t.TFT.Find(name); seq >= 0 {
					name = o.t.TFT[o.t.TFT.At(seq, clk.Ticks)].Name
				}
			}
			tex := o.tex.Bitmap(name)
			if tex == nil {
				continue
			}
			du, dv := int(f.TexDU), int(f.TexDV)
			// Scrolling textures (0x47893a: poly flags 0x400/0x800 v, 0x1000/0x2000 u;
			// the v direction flips on nearly horizontal faces).
			um, vm := scroll&(tex.OrigW-1), scroll&(tex.OrigH-1)
			vdown := f.Attr&odm.FaceScrollDown != 0
			vup := f.Attr&odm.FaceScrollUp != 0 && !vdown
			if f.Normal[2] != 0 && abs32(f.Normal[2]) >= 0xe6ca {
				vdown, vup = vup, vdown
			}
			switch {
			case vdown:
				dv -= vm
			case vup:
				dv += vm
			}
			if f.Attr&odm.FaceScrollRight != 0 {
				du += um
			} else if f.Attr&odm.FaceScrollLeft != 0 {
				du -= um
			}
			if f.Attr&odm.FaceFluid != 0 && isFluidTexture(name) && f.Attr&(odm.FaceScrollDown|odm.FaceScrollUp|odm.FaceScrollLeft|odm.FaceScrollRight) == 0 {
				if t := o.tex.Bitmap("HDWTR" + frame); t != nil {
					tex = t
				}
			}
			dim := sun.FaceDim(f.Normal)
			vs := o.vbuf[:0]
			for k, vi := range f.Verts {
				p := mod.Vertices[vi]
				x, y, z := float64(p.X), float64(p.Y), float64(p.Z)
				vs = append(vs, render.Vertex{X: x, Y: y, Z: z,
					U: float64(int(f.U[k])+du) / float64(tex.OrigW),
					V: float64(int(f.V[k])+dv) / float64(tex.OrigH),
					L: sun.OutdoorLight(dim, cam.Depth(x, y, z))})
			}
			o.vbuf = vs
			var flags render.Flags
			if tex.Alpha {
				flags = render.AlphaTest
			}
			r.Polygon(vs, tex.Texture, flags)
		}
	}
}

func abs32(v int32) int32 {
	if v < 0 {
		return -v
	}
	return v
}

// Decoration list flags that suppress the sprite (0x47b61b: 0x80 and 0x400 emit
// particles instead, 0x22 are invisible markers).
const decNoSprite = 0x80 | 0x400 | 0x22

// drawDecorations queues the decoration billboards. The view (one of 8) depends on the
// decoration's yaw and the direction to the camera; the sprite stands on its anchor,
// sized by the frame scale, with the hwl crop placed inside the original frame.
//
// mm8: 0x47b61b (decoration billboards), 0x4a42b7 (D3D billboard quad)
func (o *Outdoor) drawDecorations(r *render.Renderer, cam *render.Camera, sun *Sun, clk Clock) {
	for i := range o.Map.Decorations {
		d := &o.Map.Decorations[i]
		di := o.decIdx[i]
		if d.Flags&odm.DecInvisible != 0 || di <= 0 {
			continue
		}
		dec := &o.t.Decs[di]
		if dec.Flags&decNoSprite != 0 {
			continue
		}
		x, y, z := float64(d.Pos.X), float64(d.Pos.Y), float64(d.Pos.Z)
		depth := cam.Depth(x, y, z)
		if depth < 4 || depth > cam.Far {
			continue
		}
		fi := o.t.SFT.At(dec.SFT, clk.Ticks+abs(int(d.Pos.X+d.Pos.Y)))
		if fi < 0 || fi >= len(o.t.SFT.Frames) {
			continue
		}
		fr := &o.t.SFT.Frames[fi]
		ang := int(math.Round(math.Atan2(y-cam.Y, x-cam.X) * 2048 / (2 * math.Pi)))
		view := ((int(d.Yaw) + 128) - ang + 1024) >> 8 & 7
		names, mirror := fr.ViewNames()
		t := o.tex.Sprite(names[view])
		if t == nil {
			continue
		}
		bb := billboard(t, fr, mirror[view])
		bb.X, bb.Y, bb.Z = x, y, z
		bb.L = 1
		if fr.Flags&desc.FrameLuminous == 0 {
			bb.L = sun.OutdoorLight(0, depth)
		}
		r.Billboard(&bb)
	}
}

// billboard sizes a sprite frame: scale is 16.16 world units per texel of the
// original sprite, anchored at the bottom centre of its OrigW x OrigH frame.
//
// mm8: 0x4a42b7
func billboard(t *Tex, fr *desc.Frame, mirror bool) render.Billboard {
	k := float64(fr.Scale) / 65536
	half := t.OrigW >> 1
	b := render.Billboard{
		Left:   -float64(half-t.Crop.Min.X) * k,
		Right:  float64(t.Crop.Max.X-half) * k,
		Top:    float64(t.OrigH-t.Crop.Min.Y) * k,
		Bottom: float64(t.OrigH-t.Crop.Max.Y) * k,
		U0:     0, V0: 0, U1: 1, V1: 1,
		Tex:   t.Texture,
		Flags: render.AlphaTest | render.ClampUV,
	}
	if mirror {
		b.Left, b.Right = -b.Right, -b.Left
		b.U0, b.U1 = 1, 0
	}
	if fr.Flags&desc.FrameBlend != 0 {
		b.Flags |= render.Blend
	}
	return b
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
