package world

import (
	"math"

	"libre-enroth/internal/game/ui"
)

// EyeHeight is the camera height above the party's feet.
//
// mm8: 0x465936 (mm6.ini [party] eyelevel, default 0xa0)
const EyeHeight = 0xa0

// FreeCam is the M3 debug camera: it flies without collision. Position is the party's
// feet; the eye is EyeHeight above.
type FreeCam struct {
	X, Y, Z    float64
	Yaw, Pitch float64 // 2048ths of a turn
	dragX      int
	dragY      int
	dragging   bool
}

// Free-camera speeds per 60 Hz tick.
const (
	camMove   = 1024.0 / 60 // units
	camFast   = 4.0
	camTurn   = 512.0 / 60 // 90 degrees per second
	camPitch  = 256.0 / 60
	camClimb  = 512.0 / 60
	maxPitch  = 500.0
	mouseLook = 4.0 // angle units per UI pixel of drag
)

// Update moves the camera: W/S or Up/Down move, A/D strafe, Left/Right turn, PgUp/PgDn
// pitch, Space/C up and down, Shift fast, right-drag looks around.
func (c *FreeCam) Update(in *ui.Input) {
	speed := 1.0
	if in.Down(ui.KeyShift) {
		speed = camFast
	}
	fwd, side := 0.0, 0.0
	if in.Down('W') || in.Down(ui.KeyUp) {
		fwd++
	}
	if in.Down('S') || in.Down(ui.KeyDown) {
		fwd--
	}
	if in.Down('A') {
		side++
	}
	if in.Down('D') {
		side--
	}
	if in.Down(ui.KeyLeft) {
		c.Yaw += camTurn * speed
	}
	if in.Down(ui.KeyRight) {
		c.Yaw -= camTurn * speed
	}
	if in.Down(ui.KeyPageUp) {
		c.Pitch += camPitch * speed
	}
	if in.Down(ui.KeyPageDown) {
		c.Pitch -= camPitch * speed
	}
	if in.Down(ui.KeySpace) {
		c.Z += camClimb * speed
	}
	if in.Down('C') {
		c.Z -= camClimb * speed
	}
	if in.Right {
		if c.dragging {
			c.Yaw -= float64(in.X-c.dragX) * mouseLook
			c.Pitch -= float64(in.Y-c.dragY) * mouseLook
		}
		c.dragX, c.dragY, c.dragging = in.X, in.Y, true
	} else {
		c.dragging = false
	}
	c.Yaw = math.Mod(c.Yaw+2048, 2048)
	c.Pitch = max(-maxPitch, min(maxPitch, c.Pitch))
	a := c.Yaw * (2 * math.Pi / 2048)
	sin, cos := math.Sin(a), math.Cos(a)
	c.X += (fwd*cos - side*sin) * camMove * speed
	c.Y += (fwd*sin + side*cos) * camMove * speed
}
