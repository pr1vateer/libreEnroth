// Package world turns a loaded map into render primitives: terrain, buildings,
// decorations and sky outdoors, sectors and portals indoors (re/notes/render.md). It
// implements ui.World for the in-game screen.
package world

import (
	"fmt"
	"math"
	"strings"

	"libre-enroth/internal/assets"
	"libre-enroth/internal/assets/desc"
	"libre-enroth/internal/assets/lod"
	"libre-enroth/internal/assets/txt"
	"libre-enroth/internal/game/ui"
	"libre-enroth/internal/render"
)

// Tables are the descriptor tables shared by every map.
type Tables struct {
	Tiles    [3]desc.Tiles // dtile.bin, dtile2.bin, dtile3.bin
	Decs     desc.DecList
	SFT      *desc.SFT
	TFT      desc.TFT
	MapStats *txt.Table
}

// LoadTables reads the descriptor tables from the language LODs.
//
// mm8: 0x464974 (Game_Init)
func LoadTables(d *assets.Data) (*Tables, error) {
	t := &Tables{}
	var err error
	get := func(name string) []byte {
		if err != nil {
			return nil
		}
		var b []byte
		_, b, err = d.LangFile(name)
		return b
	}
	parse := func(name string, fn func([]byte) error) {
		if b := get(name); err == nil {
			err = fn(b)
		}
	}
	for i, n := range []string{"dtile.bin", "dtile2.bin", "dtile3.bin"} {
		parse(n, func(b []byte) (e error) { t.Tiles[i], e = desc.ParseTiles(b); return })
	}
	parse("ddeclist.bin", func(b []byte) (e error) { t.Decs, e = desc.ParseDecList(b); return })
	parse("dsft.bin", func(b []byte) (e error) { t.SFT, e = desc.ParseSFT(b); return })
	parse("dtft.bin", func(b []byte) (e error) { t.TFT, e = desc.ParseTFT(b); return })
	parse("mapstats.txt", func(b []byte) error { t.MapStats = txt.Parse(b); return nil })
	return t, err
}

// MapIndex is the row number of a map in MapStats.txt (0 when absent).
//
// mm8: 0x4532c5 (MapStats find by file name)
func (t *Tables) MapIndex(name string) int {
	for _, row := range t.MapStats.Rows {
		if len(row) > 2 && strings.EqualFold(row[2], name) {
			var n int
			if _, err := fmt.Sscan(row[0], &n); err == nil {
				return n
			}
		}
	}
	return 0
}

// World is a loaded map, the camera looking at it, and the clock.
type World struct {
	Cam   FreeCam
	Clock Clock

	name     string
	tex      *TextureCache
	tables   *Tables
	outdoor  *Outdoor
	r        render.Renderer
	cam      render.Camera
	subTicks int
}

var _ ui.World = (*World)(nil)

// Load opens a map from games.lod.
func Load(d *assets.Data, tables *Tables, tex *TextureCache, name string) (*World, error) {
	raw, err := d.Games.Raw(name)
	if err != nil {
		return nil, err
	}
	blob, err := lod.UnpackMap(raw)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	w := &World{name: name, tex: tex, tables: tables, Clock: Clock{Hour: 9}}
	switch {
	case strings.HasSuffix(strings.ToLower(name), ".odm"):
		o, err := NewOutdoor(tables, tex, name, blob)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		w.outdoor = o
		w.Cam = o.DefaultCamera()
	default:
		return nil, fmt.Errorf("%s: not an outdoor map", name)
	}
	return w, nil
}

// MapName implements ui.World.
func (w *World) MapName() string { return w.name }

// Outdoor implements ui.World.
func (w *World) Outdoor() bool { return w.outdoor != nil }

// Party implements ui.World.
func (w *World) Party() (x, y float64, yaw int) { return w.Cam.X, w.Cam.Y, int(w.Cam.Yaw) & 2047 }

// MapMarkers implements ui.World.
func (w *World) MapMarkers(fn func(x, y float64)) {
	if w.outdoor != nil {
		w.outdoor.MapMarkers(fn)
	}
}

// Update implements ui.World: one 60 Hz tick.
func (w *World) Update(in *ui.Input) {
	w.Cam.Update(in)
	w.subTicks += TicksPerSecond
	w.Clock.Ticks += w.subTicks / 60
	w.subTicks %= 60
}

// FocalOutdoor is the outdoor projection distance at the 640-pixel-wide view. The
// game computes it from the mm6.ini view width (vx1..vx2, default 8..468, never
// shipped) and a 65 degree field of view, (461/2) / tan(32 deg) + 0.5, truncated.
//
// mm8: 0x422584 (field of view), 0x46402e (its arguments), 0x47b04f (copy to 0x6f2f24)
var FocalOutdoor = math.Trunc(461*0.5/math.Tan(32*0.01745329) + 0.5)

// Clip distances: near 8 (0x47893a and 0x4815f0 clip vertices nearer than 8) and the
// mist distance (polygons are cut there; the sky shows beyond).
const (
	nearClip = 8
)

// Render implements ui.World. f is the viewport at render resolution; the projection
// scales with its width so the view matches the original at any resolution.
func (w *World) Render(f *render.Frame) {
	s := float64(f.W) / 640
	w.cam = render.Camera{
		X: w.Cam.X, Y: w.Cam.Y, Z: w.Cam.Z + EyeHeight,
		Yaw: w.Cam.Yaw, Pitch: w.Cam.Pitch,
		CX: float64(f.W) / 2, CY: float64(f.H) / 2,
		Near: nearClip,
	}
	f.Clear(render.RGB(0, 0, 0))
	if w.outdoor != nil {
		w.cam.Focal = FocalOutdoor * s
		w.cam.Far = distMist
		w.cam.Prepare()
		w.r.Begin(f, &w.cam)
		w.outdoor.Draw(&w.r, &w.cam, w.Clock)
		w.r.End()
	}
}
