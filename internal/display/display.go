// Package display maps the original 640x480 UI onto the render resolution.
//
// The UI art is scaled uniformly to fit (s = min(W/640, H/480)) and centred, which gives
// a pillarbox on wide screens. The 3D view (M3) renders natively at the render resolution
// inside UIRectToScreen(viewport), so the world gets sharper and not only bigger.
package display

import (
	"fmt"
	"image"
	"math"
	"strconv"
	"strings"
)

// UI is the size of the original screen, in which every layout coordinate is given.
var UI = image.Pt(640, 480)

// Res is a requested render resolution.
type Res struct {
	W, H int
	Auto bool // follow the window size (times the device scale factor)
}

func (r Res) String() string {
	if r.Auto {
		return "auto"
	}
	return fmt.Sprintf("%dx%d", r.W, r.H)
}

// ParseRes parses "WxH" (e.g. "1024x768") or "auto".
func ParseRes(s string) (Res, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "auto" {
		return Res{Auto: true}, nil
	}
	ws, hs, ok := strings.Cut(s, "x")
	if !ok {
		return Res{}, fmt.Errorf("resolution %q: want WxH or auto", s)
	}
	w, err1 := strconv.Atoi(ws)
	h, err2 := strconv.Atoi(hs)
	if err1 != nil || err2 != nil || w < 320 || h < 240 || w > 16384 || h > 16384 {
		return Res{}, fmt.Errorf("resolution %q: want WxH with 320x240 <= WxH <= 16384x16384", s)
	}
	return Res{W: w, H: h}, nil
}

// Transform places the 640x480 UI in a W x H render target.
type Transform struct {
	W, H       int     // render target
	Scale      float64 // UI pixel -> render pixels
	OffX, OffY int     // render position of UI (0, 0)
}

// Fit returns the largest uniform scale of the UI that fits w x h, centred. Offsets are
// whole pixels so that integer scales stay pixel-exact.
func Fit(w, h int) Transform {
	s := math.Min(float64(w)/float64(UI.X), float64(h)/float64(UI.Y))
	return Transform{
		W: w, H: h, Scale: s,
		// +1e-6: 640*1.6 is 1024.0000000000001 in float64.
		OffX: int(math.Floor((float64(w)-float64(UI.X)*s)/2 + 1e-6)),
		OffY: int(math.Floor((float64(h)-float64(UI.Y)*s)/2 + 1e-6)),
	}
}

// ToUI maps a render-target point (e.g. the mouse) to UI pixels. Points in the borders
// map outside 0..639 x 0..479.
func (t Transform) ToUI(x, y float64) (int, int) {
	return int(math.Floor((x - float64(t.OffX)) / t.Scale)), int(math.Floor((y - float64(t.OffY)) / t.Scale))
}

// ToScreen maps a UI point to render pixels.
func (t Transform) ToScreen(x, y float64) (float64, float64) {
	return float64(t.OffX) + x*t.Scale, float64(t.OffY) + y*t.Scale
}

// UIRectToScreen maps a UI rectangle to the render pixels it covers (edges rounded to
// the nearest pixel, so adjacent rectangles stay adjacent).
func (t Transform) UIRectToScreen(r image.Rectangle) image.Rectangle {
	x0, y0 := t.ToScreen(float64(r.Min.X), float64(r.Min.Y))
	x1, y1 := t.ToScreen(float64(r.Max.X), float64(r.Max.Y))
	return image.Rect(int(math.Round(x0)), int(math.Round(y0)), int(math.Round(x1)), int(math.Round(y1)))
}

// UIRect is the area the whole UI covers in render pixels.
func (t Transform) UIRect() image.Rectangle { return t.UIRectToScreen(image.Rectangle{Max: UI}) }

// IntegerScale reports whether the scale is a whole number (nearest filtering is exact).
func (t Transform) IntegerScale() bool { return t.Scale == math.Trunc(t.Scale) }
