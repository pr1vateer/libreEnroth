package world

import (
	"testing"

	"libre-enroth/internal/maps/blv"
	"libre-enroth/internal/render"
)

// twoRooms is sector 1 (x 0..1000) and sector 2 (x 1000..2000), 1000 deep and 500 high,
// joined by a portal filling the wall at x = 1000; sector 3 lies beyond sector 2 with no
// portal to it.
func twoRooms() *Indoor {
	m := &blv.Map{}
	quad := func(sector int16, poly uint8, attr uint32, n [3]int32, dist int32, back int16, vs ...blv.Vec3s) int16 {
		f := blv.Face{Sector: sector, Back: back, PolyType: poly, Attr: attr, Normal: n, Dist: dist}
		f.NormalF = [3]float32{float32(n[0]) / 65536, float32(n[1]) / 65536, float32(n[2]) / 65536}
		f.DistF = float32(dist) / 65536
		f.BBox = [6]int16{32767, -32768, 32767, -32768, 32767, -32768}
		for _, v := range vs {
			f.Verts = append(f.Verts, uint16(len(m.Vertices)))
			m.Vertices = append(m.Vertices, v)
			f.BBox = [6]int16{min(f.BBox[0], v.X), max(f.BBox[1], v.X), min(f.BBox[2], v.Y), max(f.BBox[3], v.Y), min(f.BBox[4], v.Z), max(f.BBox[5], v.Z)}
		}
		m.Faces = append(m.Faces, f)
		return int16(len(m.Faces) - 1)
	}
	m.Sectors = make([]blv.Sector, 4)
	for s, x0 := range []int16{0, 1000, 2000} {
		x1 := x0 + 1000
		fl := quad(int16(s+1), blv.PolyFloor, 0, [3]int32{0, 0, 0x10000}, 0, 0,
			blv.Vec3s{X: x0, Y: 0}, blv.Vec3s{X: x1, Y: 0}, blv.Vec3s{X: x1, Y: 1000}, blv.Vec3s{X: x0, Y: 1000})
		sec := &m.Sectors[s+1]
		sec.Floors, sec.Faces = []int16{fl}, []int16{fl}
		sec.BBox = [6]int16{x0, x1, 0, 1000, 0, 500}
	}
	p := quad(1, blv.PolyWall, blv.FacePortal, [3]int32{-0x10000, 0, 0}, 1000*0x10000, 2,
		blv.Vec3s{X: 1000, Y: 0}, blv.Vec3s{X: 1000, Y: 1000}, blv.Vec3s{X: 1000, Y: 1000, Z: 500}, blv.Vec3s{X: 1000, Y: 0, Z: 500})
	for _, s := range []int{1, 2} {
		m.Sectors[s].Portals = []int16{p}
		m.Sectors[s].Faces = append(m.Sectors[s].Faces, p)
	}
	in := &Indoor{Map: m}
	in.secRect = make([]rect, len(m.Sectors))
	in.secSeen = make([]bool, len(m.Sectors))
	return in
}

func TestSectorAt(t *testing.T) {
	in := twoRooms()
	for _, c := range []struct {
		x, y, z float64
		want    int
	}{
		{500, 500, 160, 1}, {1500, 500, 160, 2}, {2500, 10, 0, 3}, {-10, 500, 160, 0}, {500, 500, 700, 0},
	} {
		if got := in.SectorAt(c.x, c.y, c.z); got != c.want {
			t.Errorf("SectorAt(%v, %v, %v) = %d, want %d", c.x, c.y, c.z, got, c.want)
		}
	}
}

func TestPortalVisibility(t *testing.T) {
	in := twoRooms()
	full := rect{0, 0, 640, 338}
	seen := func(yaw float64, x float64) []int {
		cam := render.Camera{X: x, Y: 500, Z: 160, Yaw: yaw, Focal: FocalIndoor, CX: 320, CY: 169, Near: 8, Far: indoorFar}
		cam.Prepare()
		if !in.visibleSectors(&cam, full) {
			return nil
		}
		var out []int
		for i, s := range in.secSeen {
			if s {
				out = append(out, i)
			}
		}
		return out
	}
	eq := func(a, b []int) bool {
		if len(a) != len(b) {
			return false
		}
		for i := range a {
			if a[i] != b[i] {
				return false
			}
		}
		return true
	}
	for _, c := range []struct {
		name   string
		yaw, x float64
		want   []int
	}{
		{"facing the portal", 0, 500, []int{1, 2}},
		{"facing away", 1024, 500, []int{1}},
		{"from the other side", 1024, 1500, []int{1, 2}},
		{"standing in the portal", 512, 1000, []int{1, 2}},
		{"outside", 0, -500, nil},
	} {
		if got := seen(c.yaw, c.x); !eq(got, c.want) {
			t.Errorf("%s: visible sectors %v, want %v", c.name, got, c.want)
		}
	}
	// The portal's rectangle limits sector 2 to the part of the screen it covers.
	seen(0, 0)
	if r := in.secRect[2]; r.x0 <= 0 && r.x1 >= 640 {
		t.Errorf("sector 2 seen through %+v, want a narrower rectangle", r)
	}
}
