package render

import (
	"math"
	"runtime"
	"sync"
)

// Flags select per-primitive raster state.
type Flags uint16

const (
	AlphaTest    Flags = 1 << iota // skip texels whose filtered alpha is below 1/2
	ClampUV                        // clamp texture coordinates instead of repeating
	NoDepthTest                    // always draw
	NoDepthWrite                   // leave the depth buffer alone
	Blend                          // average with the frame (half-transparent)
)

// Vertex is a world-space polygon corner.
type Vertex struct {
	X, Y, Z float64
	U, V    float64 // texture coordinates in whole textures (1 = one repeat)
	L       float64 // light: 0 black .. 1 full texture colour
	F       float64 // fog: 0 none .. 1 fog colour
}

// PointLight is a coloured light that adds to the vertex light of a lit polygon,
// falling off linearly to 0 at Radius: (1 - d/Radius) * colour, colour 0..1.
type PointLight struct {
	X, Y, Z, Radius float64
	R, G, B         float64
}

// plane is an attribute linear in frame pixel coordinates: a0 + ax*x + ay*y.
type plane struct{ a0, ax, ay float64 }

func (p *plane) at(x, y float64) float64 { return p.a0 + float64(p.ax*x) + float64(p.ay*y) }

// tri is a set-up screen-space triangle.
type tri struct {
	tex                *Texture
	flags              Flags
	x, y               [3]float64 // sorted by y (then x)
	ymin, ymax         int        // rows [ymin, ymax)
	iw, uw, vw, lw, fw plane      // 1/depth and attribute/depth
	lights             []PointLight
	xw, yw, zw         plane // world position/depth, for lights
	id                 uint32
	tint               uint32 // 0xRRGGBB light multiplier, 0 = none
}

// pv is a projected vertex.
type pv struct{ x, y, iw, u, v, l, f, wx, wy, wz float64 }

// sky is a textured plane above the camera, filling the frame behind everything.
type sky struct {
	tex              *Texture
	height, scale    float64
	scrollU, scrollV float64
	minDz            float64
	light            float64
}

// Renderer collects primitives for one frame and rasterises them on End.
type Renderer struct {
	Workers  int    // goroutines for End; 0 = GOMAXPROCS. The output does not depend on it.
	FogColor uint32 // colour that Vertex.F blends towards

	cam  *Camera
	f    *Frame
	tris []tri
	sky  *sky
	vbuf [2][]cv
	id   uint32
	tint uint32 // the billboard being queued
}

// SetID tags the primitives queued from now on: where they write depth they write id
// into the frame's ID buffer (if it has one). Begin resets it to 0.
func (r *Renderer) SetID(id uint32) { r.id = id }

// cv is a camera-space vertex during clipping; wx, wy, wz is the world position.
type cv struct{ d, l, u, tu, tv, li, fo, wx, wy, wz float64 }

// Begin starts a frame. The camera must be Prepared.
func (r *Renderer) Begin(f *Frame, cam *Camera) {
	r.f, r.cam = f, cam
	r.tris = r.tris[:0]
	r.sky = nil
	r.id = 0
}

// Triangles is the number of triangles queued so far.
func (r *Renderer) Triangles() int { return len(r.tris) }

// Sky queues a sky plane drawn before everything else: a horizontal plane height
// units above the eye, scale texels per world unit, shifted by scroll texels. Rays
// that climb less than minDz per unit of forward distance are mirrored about that
// angle (the original mirrors the sky below its horizon).
func (r *Renderer) Sky(tex *Texture, height, scale, scrollU, scrollV, minDz, light float64) {
	if tex != nil {
		r.sky = &sky{tex, height, scale, scrollU, scrollV, minDz, light}
	}
}

// Polygon queues a convex world-space polygon, clipped to the near and far planes.
// Polygons are not culled; callers decide which side is visible.
func (r *Renderer) Polygon(vs []Vertex, tex *Texture, flags Flags) {
	r.PolygonLit(vs, tex, flags, nil)
}

// PolygonLit is Polygon with point lights added to the vertex light per pixel (exact
// every 16 pixels, linear in between) in colour. lights must not change until End.
func (r *Renderer) PolygonLit(vs []Vertex, tex *Texture, flags Flags, lights []PointLight) {
	if len(vs) < 3 || tex == nil {
		return
	}
	in := r.vbuf[0][:0]
	for _, v := range vs {
		d, l, u := r.cam.View(v.X, v.Y, v.Z)
		in = append(in, cv{d, l, u, v.U * float64(tex.W), v.V * float64(tex.H), v.L, v.F, v.X, v.Y, v.Z})
	}
	out := clipDepth(in, r.vbuf[1][:0], r.cam.Near, false)
	in = clipDepth(out, in[:0], r.cam.Far, true)
	r.vbuf[0], r.vbuf[1] = in, out
	if len(in) < 3 {
		return
	}
	var p [3]pv
	proj := func(c cv) pv {
		iw := 1 / c.d
		x, y := r.cam.Project(c.d, c.l, c.u)
		return pv{x, y, iw, float64(c.tu * iw), float64(c.tv * iw), float64(c.li * iw), float64(c.fo * iw),
			float64(c.wx * iw), float64(c.wy * iw), float64(c.wz * iw)}
	}
	p[0] = proj(in[0])
	p[2] = proj(in[1])
	for i := 2; i < len(in); i++ {
		p[1] = p[2]
		p[2] = proj(in[i])
		r.addTri(p, tex, flags, lights)
	}
}

// clipDepth keeps the part of poly with d >= z (far = false) or d <= z (far = true).
func clipDepth(poly, out []cv, z float64, far bool) []cv {
	inside := func(c cv) bool { return (c.d >= z) != far || c.d == z }
	for i := range poly {
		a, b := poly[i], poly[(i+1)%len(poly)]
		ia, ib := inside(a), inside(b)
		if ia {
			out = append(out, a)
		}
		if ia != ib {
			t := (z - a.d) / (b.d - a.d)
			lerp := func(x, y float64) float64 { return x + float64(t*(y-x)) }
			out = append(out, cv{z, lerp(a.l, b.l), lerp(a.u, b.u), lerp(a.tu, b.tu), lerp(a.tv, b.tv), lerp(a.li, b.li), lerp(a.fo, b.fo),
				lerp(a.wx, b.wx), lerp(a.wy, b.wy), lerp(a.wz, b.wz)})
		}
	}
	return out
}

// Billboard is a camera-facing quad anchored at a world point, with its extent in world
// units: Left/Right along the screen x axis, Bottom/Top up from the anchor.
type Billboard struct {
	X, Y, Z                  float64
	Left, Right, Bottom, Top float64
	U0, V0, U1, V1           float64 // texture rectangle in whole textures; swap U for a mirror image
	L, F                     float64
	Tex                      *Texture
	Flags                    Flags
	// Tint, when not 0, multiplies the light per channel by 0xRRGGBB / 255: the
	// Direct3D vertex colour of a tinted monster (D3D_ModulateColor 0x4a147a).
	Tint uint32
}

// Billboard queues a billboard at the depth of its anchor.
func (r *Renderer) Billboard(b *Billboard) {
	if b.Tex == nil {
		return
	}
	d, l, u := r.cam.View(b.X, b.Y, b.Z)
	if d < r.cam.Near || d > r.cam.Far {
		return
	}
	sx, sy := r.cam.Project(d, l, u)
	k := r.cam.Focal / d
	x0, x1 := sx+float64(b.Left*k), sx+float64(b.Right*k)
	y0, y1 := sy-float64(b.Top*k), sy-float64(b.Bottom*k)
	iw := 1 / d
	w, h := float64(b.Tex.W), float64(b.Tex.H)
	corner := func(x, y, u, v float64) pv {
		return pv{x, y, iw, float64(u * w * iw), float64(v * h * iw), float64(b.L * iw), float64(b.F * iw), 0, 0, 0}
	}
	a := corner(x0, y0, b.U0, b.V0)
	c := corner(x1, y1, b.U1, b.V1)
	r.tint = b.Tint
	r.addTri([3]pv{a, corner(x1, y0, b.U1, b.V0), c}, b.Tex, b.Flags, nil)
	r.addTri([3]pv{a, c, corner(x0, y1, b.U0, b.V1)}, b.Tex, b.Flags, nil)
	r.tint = 0
}

// snap rounds to 1/16 pixel, so shared edges are walked identically.
func snap(v float64) float64 { return math.Round(float64(v*16)) / 16 }

func (r *Renderer) addTri(p [3]pv, tex *Texture, flags Flags, lights []PointLight) {
	for i := range p {
		p[i].x, p[i].y = snap(p[i].x), snap(p[i].y)
	}
	// Sort by y, then x.
	less := func(a, b pv) bool { return a.y < b.y || (a.y == b.y && a.x < b.x) }
	if less(p[1], p[0]) {
		p[0], p[1] = p[1], p[0]
	}
	if less(p[2], p[1]) {
		p[1], p[2] = p[2], p[1]
	}
	if less(p[1], p[0]) {
		p[0], p[1] = p[1], p[0]
	}
	ymin := int(math.Ceil(p[0].y - 0.5))
	ymax := int(math.Ceil(p[2].y - 0.5))
	ymin, ymax = max(ymin, 0), min(ymax, r.f.H)
	if ymin >= ymax {
		return
	}
	if max(p[0].x, p[1].x, p[2].x) < -0.5 || min(p[0].x, p[1].x, p[2].x) > float64(r.f.W)+0.5 {
		return
	}
	dx1, dy1 := p[1].x-p[0].x, p[1].y-p[0].y
	dx2, dy2 := p[2].x-p[0].x, p[2].y-p[0].y
	det := float64(dx1*dy2) - float64(dx2*dy1)
	if math.Abs(det) < 1e-9 {
		return
	}
	mk := func(a0, a1, a2 float64) plane {
		d1, d2 := a1-a0, a2-a0
		ax := (float64(d1*dy2) - float64(d2*dy1)) / det
		ay := (float64(d2*dx1) - float64(d1*dx2)) / det
		return plane{a0 - float64(ax*p[0].x) - float64(ay*p[0].y), ax, ay}
	}
	t := tri{
		tex: tex, flags: flags, id: r.id, tint: r.tint,
		x: [3]float64{p[0].x, p[1].x, p[2].x}, y: [3]float64{p[0].y, p[1].y, p[2].y},
		ymin: ymin, ymax: ymax,
		iw: mk(p[0].iw, p[1].iw, p[2].iw),
		uw: mk(p[0].u, p[1].u, p[2].u),
		vw: mk(p[0].v, p[1].v, p[2].v),
		lw: mk(p[0].l, p[1].l, p[2].l),
		fw: mk(p[0].f, p[1].f, p[2].f),
	}
	if len(lights) > 0 {
		t.lights = lights
		t.xw = mk(p[0].wx, p[1].wx, p[2].wx)
		t.yw = mk(p[0].wy, p[1].wy, p[2].wy)
		t.zw = mk(p[0].wz, p[1].wz, p[2].wz)
	}
	r.tris = append(r.tris, t)
}

// End rasterises everything queued since Begin: the frame is cut into horizontal bands,
// one goroutine each, and every band draws the primitives in submission order.
func (r *Renderer) End() {
	n := r.Workers
	if n <= 0 {
		n = runtime.GOMAXPROCS(0)
	}
	n = max(1, min(n, r.f.H))
	bandH := (r.f.H + n - 1) / n
	bins := make([][]int32, n)
	for i := range r.tris {
		t := &r.tris[i]
		for b := t.ymin / bandH; b <= (t.ymax-1)/bandH; b++ {
			bins[b] = append(bins[b], int32(i))
		}
	}
	var wg sync.WaitGroup
	for b := range n {
		y0, y1 := b*bandH, min(r.f.H, (b+1)*bandH)
		if y0 >= y1 {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			if r.sky != nil {
				r.drawSky(y0, y1)
			}
			for _, i := range bins[b] {
				r.raster(&r.tris[i], y0, y1)
			}
		}()
	}
	wg.Wait()
}

// edgeX is the x where edge a->b (ya < yb) crosses row centre yc.
func edgeX(xa, ya, xb, yb, yc float64) float64 {
	return xa + float64(float64(yc-ya)*float64(xb-xa))/(yb-ya)
}

func (r *Renderer) raster(t *tri, by0, by1 int) {
	y0, y1 := max(t.ymin, by0), min(t.ymax, by1)
	for y := y0; y < y1; y++ {
		yc := float64(y) + 0.5
		xl := edgeX(t.x[0], t.y[0], t.x[2], t.y[2], yc)
		var xs float64
		if yc < t.y[1] {
			xs = edgeX(t.x[0], t.y[0], t.x[1], t.y[1], yc)
		} else {
			xs = edgeX(t.x[1], t.y[1], t.x[2], t.y[2], yc)
		}
		if xs < xl {
			xl, xs = xs, xl
		}
		a := max(int(math.Ceil(xl-0.5)), 0)
		b := min(int(math.Ceil(xs-0.5)), r.f.W)
		if a < b {
			if t.lights != nil {
				r.spanLit(t, y, a, b)
			} else {
				r.span(t, y, a, b)
			}
		}
	}
}

// segment is the length of the affine runs between exact perspective divisions.
const segment = 16

func (r *Renderer) span(t *tri, y, xa, xb int) {
	f := r.f
	tex := t.tex
	py := float64(y) + 0.5
	row := y * f.W
	clampUV := t.flags&ClampUV != 0
	alpha := t.flags&AlphaTest != 0 && tex.Alpha
	depthTest := t.flags&NoDepthTest == 0
	depthWrite := t.flags&NoDepthWrite == 0
	blend := t.flags&Blend != 0
	fogC := r.FogColor
	ids := f.ID

	px := float64(xa) + 0.5
	iw0 := t.iw.at(px, py)
	at := func(p *plane, x, iw float64) int64 {
		return int64(math.Floor(float64(p.at(x, py) / iw * 65536)))
	}
	u, v := at(&t.uw, px, iw0), at(&t.vw, px, iw0)
	l, fg := at(&t.lw, px, iw0), at(&t.fw, px, iw0)
	for x := xa; x < xb; {
		n := min(segment, xb-x)
		pxe := float64(x+n) + 0.5
		iwe := t.iw.at(pxe, py)
		ue, ve := at(&t.uw, pxe, iwe), at(&t.vw, pxe, iwe)
		le, fe := at(&t.lw, pxe, iwe), at(&t.fw, pxe, iwe)
		du, dv := (ue-u)/int64(n), (ve-v)/int64(n)
		dl, df := (le-l)/int64(n), (fe-fg)/int64(n)
		pxs := float64(x) + 0.5
		for k := 0; k < n; k, u, v, l, fg = k+1, u+du, v+dv, l+dl, fg+df {
			i := row + x + k
			z := float32(t.iw.at(pxs+float64(k), py))
			if depthTest && z < f.Z[i] {
				continue
			}
			c := tex.sample(u, v, clampUV)
			if alpha {
				a := c >> 24
				if a < 128 {
					continue
				}
				if a < 255 {
					c = unpremul(c, a)
				}
			}
			if t.tint != 0 {
				c = shadeTint(c, l, fg, fogC, t.tint)
			} else {
				c = shade(c, l, fg, fogC)
			}
			if blend {
				c = (c>>1)&0x7f7f7f7f + (f.Pix[i]>>1)&0x7f7f7f7f
			}
			f.Pix[i] = c | 0xff000000
			if depthWrite {
				f.Z[i] = z
				if ids != nil {
					ids[i] = t.id
				}
			}
		}
		x += n
		u, v, l, fg = ue, ve, le, fe
	}
}

// spanLit is span with per-channel light: the vertex light plus the point lights,
// evaluated at the world position of every segment end.
func (r *Renderer) spanLit(t *tri, y, xa, xb int) {
	f := r.f
	tex := t.tex
	py := float64(y) + 0.5
	row := y * f.W
	clampUV := t.flags&ClampUV != 0
	alpha := t.flags&AlphaTest != 0 && tex.Alpha
	depthTest := t.flags&NoDepthTest == 0
	depthWrite := t.flags&NoDepthWrite == 0
	blend := t.flags&Blend != 0
	fogC := r.FogColor
	ids := f.ID

	at := func(p *plane, x, iw float64) int64 {
		return int64(math.Floor(float64(p.at(x, py) / iw * 65536)))
	}
	light := func(x, iw float64) (lr, lg, lb int64) {
		base := t.lw.at(x, py) / iw
		wx, wy, wz := t.xw.at(x, py)/iw, t.yw.at(x, py)/iw, t.zw.at(x, py)/iw
		cr, cg, cb := base, base, base
		for i := range t.lights {
			l := &t.lights[i]
			dx, dy, dz := wx-l.X, wy-l.Y, wz-l.Z
			d := math.Sqrt(float64(dx*dx) + float64(dy*dy) + float64(dz*dz))
			if d >= l.Radius {
				continue
			}
			k := 1 - d/l.Radius
			cr += float64(k * l.R)
			cg += float64(k * l.G)
			cb += float64(k * l.B)
		}
		fx := func(c float64) int64 { return int64(math.Floor(float64(min(c, 1) * 65536))) }
		return fx(cr), fx(cg), fx(cb)
	}
	px := float64(xa) + 0.5
	iw0 := t.iw.at(px, py)
	u, v := at(&t.uw, px, iw0), at(&t.vw, px, iw0)
	fg := at(&t.fw, px, iw0)
	lr, lg, lb := light(px, iw0)
	for x := xa; x < xb; {
		n := min(segment, xb-x)
		pxe := float64(x+n) + 0.5
		iwe := t.iw.at(pxe, py)
		ue, ve := at(&t.uw, pxe, iwe), at(&t.vw, pxe, iwe)
		fe := at(&t.fw, pxe, iwe)
		lre, lge, lbe := light(pxe, iwe)
		nn := int64(n)
		du, dv, df := (ue-u)/nn, (ve-v)/nn, (fe-fg)/nn
		dr, dg, db := (lre-lr)/nn, (lge-lg)/nn, (lbe-lb)/nn
		pxs := float64(x) + 0.5
		for k := 0; k < n; k, u, v, fg, lr, lg, lb = k+1, u+du, v+dv, fg+df, lr+dr, lg+dg, lb+db {
			i := row + x + k
			z := float32(t.iw.at(pxs+float64(k), py))
			if depthTest && z < f.Z[i] {
				continue
			}
			c := tex.sample(u, v, clampUV)
			if alpha {
				a := c >> 24
				if a < 128 {
					continue
				}
				if a < 255 {
					c = unpremul(c, a)
				}
			}
			c = shadeRGB(c, lr, lg, lb, fg, fogC)
			if blend {
				c = (c>>1)&0x7f7f7f7f + (f.Pix[i]>>1)&0x7f7f7f7f
			}
			f.Pix[i] = c | 0xff000000
			if depthWrite {
				f.Z[i] = z
				if ids != nil {
					ids[i] = t.id
				}
			}
		}
		x += n
		u, v, fg, lr, lg, lb = ue, ve, fe, lre, lge, lbe
	}
}

// shadeRGB modulates each channel by its light and blends towards the fog colour (all
// 16.16).
func shadeRGB(c uint32, lr, lg, lb, fg int64, fogC uint32) uint32 {
	ch := func(v uint32, l int64) uint32 { return v * uint32(clamp64(l>>8, 0, 256)) >> 8 }
	c = ch(c&0xff, lr) | ch(c>>8&0xff, lg)<<8 | ch(c>>16&0xff, lb)<<16 | c&0xff000000
	if fg > 0 {
		c = lerp2(c, fogC, uint32(clamp64(fg>>8, 0, 256)))
	}
	return c
}

// shadeTint is shade with the light multiplied per channel by tint (0xRRGGBB, each
// channel / 255).
func shadeTint(c uint32, l, fg int64, fogC uint32, tint uint32) uint32 {
	ch := func(t uint32) int64 { return l * int64(t) / 255 }
	return shadeRGB(c, ch(tint>>16&0xff), ch(tint>>8&0xff), ch(tint&0xff), fg, fogC)
}

// unpremul restores the colour of a partly covered texel.
func unpremul(c, a uint32) uint32 {
	r := (c & 0xff) * 255 / a
	g := (c >> 8 & 0xff) * 255 / a
	b := (c >> 16 & 0xff) * 255 / a
	return min(r, 255) | min(g, 255)<<8 | min(b, 255)<<16 | 0xff000000
}

// shade modulates by light l and blends towards fog colour by fg (both 16.16, 1.0 =
// 65536).
func shade(c uint32, l, fg int64, fogC uint32) uint32 {
	li := uint32(clamp64(l>>8, 0, 256))
	if li != 256 {
		c = lerp2(0, c, li)
	}
	if fg > 0 {
		c = lerp2(c, fogC, uint32(clamp64(fg>>8, 0, 256)))
	}
	return c
}

func clamp64(v, lo, hi int64) int64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func (r *Renderer) drawSky(y0, y1 int) {
	s, c, f := r.sky, r.cam, r.f
	li := int64(math.Round(s.light * 65536))
	for y := y0; y < y1; y++ {
		py := float64(y) + 0.5
		dx0, dy0, dz := c.Ray(0.5, py)
		dx1, dy1, _ := c.Ray(1.5, py)
		if dz < s.minDz {
			dz = 2*s.minDz - dz
		}
		k := s.height / dz * s.scale
		u := int64(math.Floor(float64((float64(dx0*k) + s.scrollU) * 65536)))
		v := int64(math.Floor(float64((float64(dy0*k) + s.scrollV) * 65536)))
		du := int64(math.Floor(float64(float64((dx1-dx0)*k) * 65536)))
		dv := int64(math.Floor(float64(float64((dy1-dy0)*k) * 65536)))
		row := y * f.W
		for x := 0; x < f.W; x, u, v = x+1, u+du, v+dv {
			f.Pix[row+x] = shade(s.tex.sample(u, v, false), li, 0, 0) | 0xff000000
		}
	}
}
