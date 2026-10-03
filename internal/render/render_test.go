package render

import (
	"crypto/sha256"
	"encoding/hex"
	"math"
	"math/rand"
	"testing"
)

func solid(c uint32) *Texture { return FromRGBA(1, 1, []uint32{c}) }

func checker(n int, a, b uint32) *Texture {
	pix := make([]uint32, n*n)
	for y := range n {
		for x := range n {
			if (x/(n/2)+y/(n/2))%2 == 0 {
				pix[y*n+x] = a
			} else {
				pix[y*n+x] = b
			}
		}
	}
	return FromRGBA(n, n, pix)
}

func testCam(w, h int) *Camera {
	c := &Camera{Focal: float64(w) / 2, CX: float64(w) / 2, CY: float64(h) / 2, Near: 8, Far: 100000}
	c.Prepare()
	return c
}

// wall is a square facing the camera (which looks along +x) at distance d.
func wall(d, half float64, l float64) []Vertex {
	return []Vertex{
		{X: d, Y: half, Z: half, U: 0, V: 0, L: l},
		{X: d, Y: -half, Z: half, U: 1, V: 0, L: l},
		{X: d, Y: -half, Z: -half, U: 1, V: 1, L: l},
		{X: d, Y: half, Z: -half, U: 0, V: 1, L: l},
	}
}

func TestZOrder(t *testing.T) {
	red, green := RGB(255, 0, 0), RGB(0, 255, 0)
	for _, order := range []bool{false, true} {
		f := NewFrame(64, 48)
		f.Clear(0)
		var r Renderer
		r.Begin(f, testCam(64, 48))
		near, far := wall(100, 30, 1), wall(200, 1000, 1)
		if order {
			r.Polygon(near, solid(red), 0)
			r.Polygon(far, solid(green), 0)
		} else {
			r.Polygon(far, solid(green), 0)
			r.Polygon(near, solid(red), 0)
		}
		r.End()
		if got := f.Pix[24*64+32]; got != red {
			t.Errorf("near-first=%v: centre = %#x, want red", order, got)
		}
		if got := f.Pix[2*64+2]; got != green {
			t.Errorf("near-first=%v: corner = %#x, want green", order, got)
		}
		if z := f.Z[24*64+32]; math.Abs(float64(z)-1.0/100) > 1e-7 {
			t.Errorf("depth = %v, want 1/100", z)
		}
	}
}

// TestPerspective draws a checkered floor and checks every pixel away from a cell
// border against the analytic ray-plane intersection.
func TestPerspective(t *testing.T) {
	const w, h = 160, 120
	a, b := RGB(255, 255, 255), RGB(0, 0, 0)
	f := NewFrame(w, h)
	f.Clear(RGB(1, 2, 3))
	cam := testCam(w, h)
	cam.Pitch = -100
	cam.Prepare()
	var r Renderer
	r.Begin(f, cam)
	const size = 256.0 // world units per texture repeat
	z := -150.0
	floor := []Vertex{
		{X: 10, Y: -2000, Z: z, U: 10 / size, V: -2000 / size, L: 1},
		{X: 10, Y: 2000, Z: z, U: 10 / size, V: 2000 / size, L: 1},
		{X: 3000, Y: 2000, Z: z, U: 3000 / size, V: 2000 / size, L: 1},
		{X: 3000, Y: -2000, Z: z, U: 3000 / size, V: -2000 / size, L: 1},
	}
	r.Polygon(floor, checker(64, a, b), 0)
	r.End()
	checked, bad := 0, 0
	for y := range h {
		for x := range w {
			dx, dy, dz := cam.Ray(float64(x)+0.5, float64(y)+0.5)
			if dz >= 0 {
				continue
			}
			k := (z - cam.Z) / dz
			wx, wy := cam.X+dx*k, cam.Y+dy*k
			if wx < 40 || wx > 2800 || math.Abs(wy) > 1900 {
				continue
			}
			u, v := wx/size, wy/size
			fu, fv := u*2-math.Floor(u*2), v*2-math.Floor(v*2)
			// Skip pixels whose footprint touches a cell border (bilinear blending).
			du := 2 * k / cam.Focal / size * 2 * 1.5
			if fu < du || fu > 1-du || fv < du || fv > 1-du {
				continue
			}
			want := a
			if (int(math.Floor(u*2))+int(math.Floor(v*2)))%2 != 0 {
				want = b
			}
			checked++
			if f.Pix[y*w+x] != want {
				bad++
			}
		}
	}
	if checked < 2000 || bad > 0 {
		t.Errorf("%d of %d checked pixels wrong", bad, checked)
	}
}

func TestNearClip(t *testing.T) {
	f := NewFrame(64, 48)
	f.Clear(0)
	cam := testCam(64, 48)
	var r Renderer
	r.Begin(f, cam)
	// A floor that starts behind the camera: everything below the horizon is floor.
	floor := []Vertex{
		{X: -500, Y: -5000, Z: -50, L: 1}, {X: -500, Y: 5000, Z: -50, L: 1},
		{X: 50000, Y: 5000, Z: -50, L: 1}, {X: 50000, Y: -5000, Z: -50, L: 1},
	}
	r.Polygon(floor, solid(RGB(9, 9, 9)), 0)
	if r.Triangles() == 0 {
		t.Fatal("clipped away")
	}
	r.End()
	for y := 25; y < 48; y++ {
		for x := range 64 {
			if f.Pix[y*64+x] != RGB(9, 9, 9) {
				t.Fatalf("pixel %d,%d = %#x, want floor", x, y, f.Pix[y*64+x])
			}
			if z := f.Z[y*64+x]; z <= 0 || z > 1.0/8 || math.IsNaN(float64(z)) {
				t.Fatalf("pixel %d,%d depth %v", x, y, z)
			}
		}
	}
	for x := range 64 {
		if f.Pix[x] != 0 {
			t.Fatalf("sky row drawn at %d", x)
		}
	}
	// Entirely behind the camera.
	r.Begin(f, cam)
	r.Polygon(wall(-100, 50, 1), solid(1), 0)
	if r.Triangles() != 0 {
		t.Error("polygon behind the camera was queued")
	}
}

// scene queues a deterministic pile of textured, lit, fogged and alpha-tested
// primitives plus a sky.
func scene(r *Renderer, f *Frame) {
	cam := &Camera{X: 0, Y: 0, Z: 200, Yaw: 100, Pitch: -40, Focal: float64(f.W) * 0.6, CX: float64(f.W) / 2, CY: float64(f.H) / 2, Near: 8, Far: 8192}
	cam.Prepare()
	r.Begin(f, cam)
	r.FogColor = RGB(128, 128, 140)
	rng := rand.New(rand.NewSource(1))
	texs := []*Texture{checker(64, RGB(200, 180, 40), RGB(30, 90, 160)), checker(32, RGB(250, 250, 250), RGB(90, 10, 10))}
	cut := checker(32, RGB(10, 200, 10), 0) // half transparent
	r.Sky(texs[1], 512, 1.0/8, 3, 3, 1.0/32, 1)
	for i := 0; i < 1500; i++ {
		x, y := rng.Float64()*6000-1000, rng.Float64()*6000-3000
		s := 50 + rng.Float64()*300
		var vs []Vertex
		for k := range 3 + rng.Intn(3) {
			a := float64(k) / 5 * 2 * math.Pi
			vs = append(vs, Vertex{X: x + s*math.Cos(a), Y: y + s*math.Sin(a), Z: rng.Float64() * 400,
				U: rng.Float64() * 4, V: rng.Float64() * 4, L: 0.4 + rng.Float64()*0.6, F: rng.Float64() * 0.5})
		}
		r.Polygon(vs, texs[i%2], 0)
	}
	for i := 0; i < 200; i++ {
		r.Billboard(&Billboard{X: rng.Float64() * 5000, Y: rng.Float64()*5000 - 2500, Z: 0,
			Left: -60, Right: 60, Top: 200, U0: 0, V0: 0, U1: 1, V1: 1, L: 1, Tex: cut, Flags: AlphaTest | ClampUV})
	}
}

func frameHash(f *Frame) string {
	h := sha256.New()
	h.Write(f.Bytes())
	for _, z := range f.Z {
		b := math.Float32bits(z)
		h.Write([]byte{byte(b), byte(b >> 8), byte(b >> 16), byte(b >> 24)})
	}
	return hex.EncodeToString(h.Sum(nil))
}

func TestBandsIdentical(t *testing.T) {
	var hashes []string
	for _, n := range []int{1, 2, 6, 13} {
		f := NewFrame(320, 200)
		f.Clear(0)
		r := Renderer{Workers: n}
		scene(&r, f)
		r.End()
		hashes = append(hashes, frameHash(f))
	}
	for i := 1; i < len(hashes); i++ {
		if hashes[i] != hashes[0] {
			t.Fatalf("band counts give different frames: %v", hashes)
		}
	}
}

// TestGolden freezes the synthetic scene, so rasteriser changes are deliberate.
func TestGolden(t *testing.T) {
	f := NewFrame(320, 200)
	f.Clear(0)
	var r Renderer
	scene(&r, f)
	r.End()
	const want = "9d5a59ddcdb625dd0eb510a0fa13cab001899ea666e0449ba2b2af6e675e6e70"
	if got := frameHash(f); want != "" && got != want {
		t.Errorf("hash %s, want %s", got, want)
	} else if want == "" {
		t.Logf("hash %s", got)
	}
}

func BenchmarkFrame(b *testing.B) {
	for _, sz := range [][2]int{{640, 338}, {1024, 540}, {1280, 676}} {
		b.Run(fmtSize(sz), func(b *testing.B) {
			f := NewFrame(sz[0], sz[1])
			var r Renderer
			for b.Loop() {
				f.Clear(0)
				scene(&r, f)
				r.End()
			}
		})
	}
}

func fmtSize(s [2]int) string {
	return itoa(s[0]) + "x" + itoa(s[1])
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	var b []byte
	for ; v > 0; v /= 10 {
		b = append([]byte{byte('0' + v%10)}, b...)
	}
	return string(b)
}

// TestPointLight lights a grey wall at distance 100 with a red light 20 units in front of
// its centre: the centre gets base + (1 - 20/radius) red, the corners only the base.
func TestPointLight(t *testing.T) {
	const w, h = 64, 48
	for _, workers := range []int{1, 5} {
		f := NewFrame(w, h)
		f.Clear(0)
		r := Renderer{Workers: workers}
		r.Begin(f, testCam(w, h))
		lights := []PointLight{{X: 80, Y: 0, Z: 0, Radius: 40, R: 1, G: 0.5, B: 0}}
		r.PolygonLit(wall(100, 100, 0.25), solid(RGB(200, 200, 200)), 0, lights)
		r.End()
		c := f.Pix[(h/2)*w+w/2]
		// d ~ 20 at the centre pixel: red 0.25 + 0.5, green 0.25 + 0.25, blue 0.25
		cr, cg, cb := c&0xff, c>>8&0xff, c>>16&0xff
		if cr < 145 || cr > 152 || cg < 95 || cg > 102 || cb != 50 {
			t.Errorf("workers %d: centre = %d,%d,%d, want ~150,100,50", workers, cr, cg, cb)
		}
		if c := f.Pix[2*w+2]; c != RGB(50, 50, 50) {
			t.Errorf("workers %d: corner = %#x, want only the base light", workers, c)
		}
	}
	// Unlit and lit with no lights in range give the same pixels.
	a, b := NewFrame(w, h), NewFrame(w, h)
	var r Renderer
	r.Begin(a, testCam(w, h))
	r.Polygon(wall(100, 30, 0.6), checker(8, RGB(255, 0, 0), RGB(0, 0, 255)), 0)
	r.End()
	r.Begin(b, testCam(w, h))
	r.PolygonLit(wall(100, 30, 0.6), checker(8, RGB(255, 0, 0), RGB(0, 0, 255)), 0,
		[]PointLight{{X: -1000, Radius: 10, R: 1, G: 1, B: 1}})
	r.End()
	if frameHash(a) != frameHash(b) {
		t.Error("an out-of-range light changed the image")
	}
}
