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
	indoor   *Indoor
	r        render.Renderer
	cam      render.Camera
	subTicks int
}

var _ ui.World = (*World)(nil)

// Load opens a map from games.lod, with its .ddm/.dlv template for indoor maps.
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
	// The map's state file: .ddm outdoors, .dlv indoors (games.lod has the new-game
	// templates; saves are M10).
	state := func(ext string) ([]byte, error) {
		sname := name[:len(name)-4] + ext
		if _, ok := d.Games.Find(sname); !ok {
			return nil, nil
		}
		raw, err := d.Games.Raw(sname)
		if err != nil {
			return nil, err
		}
		b, err := lod.UnpackMap(raw)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", sname, err)
		}
		return b, nil
	}
	switch {
	case strings.HasSuffix(strings.ToLower(name), ".odm"):
		ddm, err := state(".ddm")
		if err != nil {
			return nil, err
		}
		o, err := NewOutdoor(tables, tex, name, blob, ddm)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		w.outdoor = o
		w.Cam = o.DefaultCamera()
	case strings.HasSuffix(strings.ToLower(name), ".blv"):
		dlv, err := state(".dlv")
		if err != nil {
			return nil, err
		}
		in, err := NewIndoor(tables, tex, blob, dlv)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		w.indoor = in
		w.Cam = in.DefaultCamera()
	default:
		return nil, fmt.Errorf("%s: not a map", name)
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
	switch {
	case w.outdoor != nil:
		w.outdoor.MapMarkers(fn)
	case w.indoor != nil:
		w.indoor.MapMarkers(fn)
	}
}

// Indoor is the loaded indoor map, nil outdoors.
func (w *World) Indoor() *Indoor { return w.indoor }

// Update implements ui.World: one 60 Hz tick. F2 opens/closes every door indoors.
func (w *World) Update(in *ui.Input) {
	w.Cam.Update(in)
	w.subTicks += TicksPerSecond
	ticks := w.subTicks / 60
	w.Clock.Ticks += ticks
	w.subTicks %= 60
	if w.indoor != nil {
		if in.Pressed(ui.KeyF2) {
			w.indoor.ToggleDoors()
		}
		w.indoor.UpdateDoors(ticks)
	}
}

// FocalOutdoor is the outdoor projection distance at the 640-pixel-wide view. The
// game computes it from the mm6.ini view width (vx1..vx2, default 8..468, never
// shipped) and a 65 degree field of view, (461/2) / tan(32 deg) + 0.5, truncated.
//
// mm8: 0x422584 (field of view), 0x46402e (its arguments), 0x47b04f (copy to 0x6f2f24)
var FocalOutdoor = math.Trunc(461*0.5/math.Tan(32*0.01745329) + 0.5)

// FocalIndoor is the indoor projection distance at the 640-pixel-wide view: the larger
// viewport side times 0.8814736 (a 65 degree frustum).
//
// mm8: 0x435ace (Camera_SetupIndoor)
const FocalIndoor = 640 * 0.8814736

// indoorFar is the indoor far plane: there is no mist indoors.
const indoorFar = 0x10000

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
	if w.indoor != nil {
		w.cam.Focal = FocalIndoor * s
		w.cam.Far = indoorFar
		w.cam.Prepare()
		w.r.Begin(f, &w.cam)
		w.indoor.Draw(&w.r, &w.cam, f, w.Clock)
		w.r.End()
	}
}
