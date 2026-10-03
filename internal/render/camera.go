package render

import "math"

// AngleUnits is a full turn in the game's angle units.
const AngleUnits = 2048

// Camera is a viewer in the game's world coordinates (x east, y north, z up). Yaw 0
// looks along +x and grows counter-clockwise (512 = north); pitch > 0 looks up. Both are
// in 2048ths of a turn. The projection is the original's:
//
//	depth = forward distance, sx = CX - Focal*left/depth, sy = CY - Focal*up/depth
//
// mm8: 0x482f93 (outdoor view transform), 0x48303c (projection)
type Camera struct {
	X, Y, Z    float64
	Yaw, Pitch float64
	Focal      float64 // pixels
	CX, CY     float64 // projection centre in frame pixels
	Near, Far  float64 // depth clip planes

	sinY, cosY, sinP, cosP float64
}

// Prepare computes the rotation; call it after changing the angles.
func (c *Camera) Prepare() {
	y := float64(c.Yaw * (2 * math.Pi / AngleUnits))
	p := float64(c.Pitch * (2 * math.Pi / AngleUnits))
	c.sinY, c.cosY = math.Sin(y), math.Cos(y)
	c.sinP, c.cosP = math.Sin(p), math.Cos(p)
}

// View transforms a world point to camera space: depth (forward), left, up.
func (c *Camera) View(x, y, z float64) (depth, left, up float64) {
	dx, dy, dz := x-c.X, y-c.Y, z-c.Z
	f := float64(c.cosY*dx) + float64(c.sinY*dy)
	left = float64(c.cosY*dy) - float64(c.sinY*dx)
	depth = float64(c.cosP*f) + float64(c.sinP*dz)
	up = float64(c.cosP*dz) - float64(c.sinP*f)
	return
}

// Depth is the forward distance of a world point.
func (c *Camera) Depth(x, y, z float64) float64 {
	d, _, _ := c.View(x, y, z)
	return d
}

// Project maps camera space to frame pixels.
func (c *Camera) Project(depth, left, up float64) (sx, sy float64) {
	k := c.Focal / depth
	return c.CX - float64(k*left), c.CY - float64(k*up)
}

// Ray returns the world direction through frame pixel (px, py), scaled so that its
// forward component is 1.
func (c *Camera) Ray(px, py float64) (dx, dy, dz float64) {
	l := (c.CX - px) / c.Focal
	u := (c.CY - py) / c.Focal
	// forward F = (cosY cosP, sinY cosP, sinP), left L = (-sinY, cosY, 0),
	// up U = (-cosY sinP, -sinY sinP, cosP)
	dx = float64(c.cosY*c.cosP) - float64(l*c.sinY) - float64(u*float64(c.cosY*c.sinP))
	dy = float64(c.sinY*c.cosP) + float64(l*c.cosY) - float64(u*float64(c.sinY*c.sinP))
	dz = c.sinP + float64(u*c.cosP)
	return
}
