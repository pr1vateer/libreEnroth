package display

import (
	"image"
	"testing"
)

func TestParseRes(t *testing.T) {
	for _, c := range []struct {
		in   string
		want Res
		ok   bool
	}{
		{"640x480", Res{W: 640, H: 480}, true},
		{"1024X768", Res{W: 1024, H: 768}, true},
		{" 1920x1080 ", Res{W: 1920, H: 1080}, true},
		{"auto", Res{Auto: true}, true},
		{"1024", Res{}, false},
		{"axb", Res{}, false},
		{"100x100", Res{}, false},
		{"-640x480", Res{}, false},
	} {
		got, err := ParseRes(c.in)
		if (err == nil) != c.ok || got != c.want {
			t.Errorf("ParseRes(%q) = %v, %v; want %v ok=%v", c.in, got, err, c.want, c.ok)
		}
	}
}

func TestFit(t *testing.T) {
	for _, c := range []struct {
		w, h       int
		scale      float64
		offX, offY int
		ui         image.Rectangle
		integer    bool
	}{
		{640, 480, 1, 0, 0, image.Rect(0, 0, 640, 480), true},
		{1024, 768, 1.6, 0, 0, image.Rect(0, 0, 1024, 768), false},
		{1280, 720, 1.5, 160, 0, image.Rect(160, 0, 1120, 720), false},
		{1920, 1080, 2.25, 240, 0, image.Rect(240, 0, 1680, 1080), false},
		{1280, 960, 2, 0, 0, image.Rect(0, 0, 1280, 960), true},
		{640, 600, 1, 0, 60, image.Rect(0, 60, 640, 540), true}, // letterbox
	} {
		tr := Fit(c.w, c.h)
		if tr.Scale != c.scale || tr.OffX != c.offX || tr.OffY != c.offY {
			t.Errorf("Fit(%d,%d) = %+v; want scale %v off %d,%d", c.w, c.h, tr, c.scale, c.offX, c.offY)
		}
		if got := tr.UIRect(); got != c.ui {
			t.Errorf("Fit(%d,%d).UIRect() = %v; want %v", c.w, c.h, got, c.ui)
		}
		if tr.IntegerScale() != c.integer {
			t.Errorf("Fit(%d,%d).IntegerScale() = %v", c.w, c.h, !c.integer)
		}
	}
}

// Every UI pixel's centre maps to a render point that maps back to the same UI pixel.
func TestRoundTrip(t *testing.T) {
	for _, sz := range []image.Point{{640, 480}, {1024, 768}, {1280, 720}, {1920, 1080}, {800, 600}, {3840, 2160}} {
		tr := Fit(sz.X, sz.Y)
		for y := 0; y < 480; y += 7 {
			for x := 0; x < 640; x += 5 {
				sx, sy := tr.ToScreen(float64(x)+0.5, float64(y)+0.5)
				if ux, uy := tr.ToUI(sx, sy); ux != x || uy != y {
					t.Fatalf("%v: UI %d,%d -> %.2f,%.2f -> %d,%d", sz, x, y, sx, sy, ux, uy)
				}
			}
		}
		// Pillarbox / letterbox points fall outside the UI.
		if tr.OffX > 0 {
			if ux, _ := tr.ToUI(float64(tr.OffX)-1, 10); ux >= 0 {
				t.Errorf("%v: left border maps to UI x %d", sz, ux)
			}
		}
	}
}

func TestUIRectToScreen(t *testing.T) {
	viewport := image.Rect(0, 29, 640, 367) // re/notes/ui.md: Viewport_Set(0,29,639,366)
	for _, c := range []struct {
		w, h int
		want image.Rectangle
	}{
		{640, 480, image.Rect(0, 29, 640, 367)},
		{1024, 768, image.Rect(0, 46, 1024, 587)},
		{1280, 720, image.Rect(160, 44, 1120, 551)},
		{1920, 1080, image.Rect(240, 65, 1680, 826)},
	} {
		if got := Fit(c.w, c.h).UIRectToScreen(viewport); got != c.want {
			t.Errorf("%dx%d: viewport -> %v; want %v", c.w, c.h, got, c.want)
		}
	}
}
