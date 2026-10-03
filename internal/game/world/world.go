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
	"libre-enroth/internal/game/party"
	"libre-enroth/internal/game/physics"
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

// World is a loaded map, the party in it, and the clock. The M3 free camera stays as a
// debug view (F3).
type World struct {
	Cam FreeCam
	// FreeCamOn shows and moves the free camera instead of the party.
	FreeCamOn bool
	// AlwaysRun is the KEY_ALWAYSRUN toggle (U).
	AlwaysRun bool
	Clock     Clock

	group    *party.Party
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
		w.group = party.New(int32(w.Cam.X), int32(w.Cam.Y), int32(w.Cam.Z), int32(w.Cam.Yaw))
		w.dropParty()
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
		w.group = party.New(int32(w.Cam.X), int32(w.Cam.Y), int32(w.Cam.Z), int32(w.Cam.Yaw))
		w.dropParty()
	default:
		return nil, fmt.Errorf("%s: not a map", name)
	}
	return w, nil
}

// MapName implements ui.World.
func (w *World) MapName() string { return w.name }

// Outdoor implements ui.World.
func (w *World) Outdoor() bool { return w.outdoor != nil }

// Party implements ui.World: the free camera while it is on, else the party.
func (w *World) Party() (x, y float64, yaw int) {
	if w.FreeCamOn {
		return w.Cam.X, w.Cam.Y, int(w.Cam.Yaw) & 2047
	}
	return float64(w.group.X), float64(w.group.Y), int(w.group.Dir)
}

// MapMarkers implements ui.World.
func (w *World) MapMarkers(fn func(x, y float64)) {
	switch {
	case w.outdoor != nil:
		w.outdoor.MapMarkers(fn)
	case w.indoor != nil:
		w.indoor.MapMarkers(fn)
	}
}

// PartyState is the moving party.
func (w *World) PartyState() *party.Party { return w.group }

// Indoor is the loaded indoor map, nil outdoors.
func (w *World) Indoor() *Indoor { return w.indoor }

// Update implements ui.World: one 60 Hz tick of input, party movement, doors and the
// clock. Debug keys: F2 opens/closes every door indoors, F3 toggles the free camera
// (leaving it puts the party where the camera is), F4 the fly buff, F5 water walking.
func (w *World) Update(in *ui.Input) {
	w.subTicks += TicksPerSecond
	ticks := w.subTicks / 60
	w.Clock.Ticks += ticks
	w.subTicks %= 60
	if in.Pressed(ui.KeyF3) {
		w.FreeCamOn = !w.FreeCamOn
		if w.FreeCamOn {
			p := w.group
			w.Cam = FreeCam{X: float64(p.X), Y: float64(p.Y), Z: float64(p.Z), Yaw: float64(p.Dir), Pitch: float64(p.Look)}
		} else {
			w.SetPartyFromCam()
		}
	}
	if in.Pressed(ui.KeyF4) {
		w.group.Fly = !w.group.Fly
	}
	if in.Pressed(ui.KeyF5) {
		w.group.WaterWalk = !w.group.WaterWalk
	}
	if w.FreeCamOn {
		w.Cam.Update(in)
	} else {
		w.movePartyKeys(in)
		w.moveParty(int32(ticks))
	}
	if w.indoor != nil {
		if in.Pressed(ui.KeyF2) {
			w.indoor.ToggleDoors()
		}
		w.indoor.UpdateDoors(ticks)
	}
}

// keyBindings are the original's default keys (KeyConfig_Load 0x458da9 with no KEY_*
// registry values); held marks the bindings polled as held (g_keyOnce 0), the rest
// fire when pressed.
var keyBindings = map[party.Binding]struct {
	key  ui.Key
	held bool
}{
	party.BindForward:     {ui.KeyUp, true},
	party.BindBackward:    {ui.KeyDown, true},
	party.BindLeft:        {ui.KeyLeft, true},
	party.BindRight:       {ui.KeyRight, true},
	party.BindJump:        {'X', false},
	party.BindAlwaysRun:   {'U', false},
	party.BindLookUp:      {ui.KeyPageDown, false},
	party.BindLookDown:    {ui.KeyDelete, false},
	party.BindCenterView:  {ui.KeyEnd, false},
	party.BindFlyUp:       {ui.KeyPageUp, true},
	party.BindFlyDown:     {ui.KeyInsert, true},
	party.BindLand:        {ui.KeyHome, false},
	party.BindStrafeLeft:  {ui.KeyBracketLeft, true},
	party.BindStrafeRight: {ui.KeyBracketRight, true},
}

type keyState struct{ in *ui.Input }

func (k keyState) Active(b party.Binding) bool {
	kb, ok := keyBindings[b]
	switch {
	case !ok:
		return false
	case kb.held:
		return k.in.Down(kb.key)
	}
	return k.in.Pressed(kb.key)
}

func (k keyState) Shift() bool { return k.in.Down(ui.KeyShift) }
func (k keyState) Ctrl() bool  { return k.in.Down(ui.KeyControl) }

// movePartyKeys queues the frame's movement actions; the AlwaysRun key toggles for the
// next frame, as the original's key loop does.
//
// mm8: 0x42efd9 (Input_GameKeys)
func (w *World) movePartyKeys(in *ui.Input) {
	k := keyState{in}
	w.group.Keys(k, w.AlwaysRun)
	if k.Active(party.BindAlwaysRun) {
		w.AlwaysRun = !w.AlwaysRun
	}
}

// moveParty runs the party's move for a frame of ticks.
//
// mm8: 0x46bac9 (World_Tick: 0x46bafa indoors, 0x46bb13 outdoors)
func (w *World) moveParty(ticks int32) {
	if ticks <= 0 {
		return
	}
	switch {
	case w.indoor != nil:
		w.group.MoveIndoor(w.indoor.geo, ticks)
	case w.outdoor != nil:
		w.group.MoveOutdoor(w.outdoor.geo, ticks)
		w.group.ClampToMap()
	}
}

// SetPartyFromCam puts the party at the free camera's position and heading, dropped
// onto the floor under it.
func (w *World) SetPartyFromCam() {
	c := w.Cam
	look := max(-0x80, min(0x80, int32(math.Round(c.Pitch))))
	w.group.Teleport(int32(math.Round(c.X)), int32(math.Round(c.Y)), int32(math.Round(c.Z)), int32(math.Round(c.Yaw)))
	w.group.Look = look
	w.dropParty()
}

// dropParty puts the party on the floor under it, if there is one.
func (w *World) dropParty() {
	p := w.group
	switch {
	case w.indoor != nil:
		g := w.indoor.geo
		sector := g.SectorAt(p.X, p.Y, p.Z)
		if z, _ := g.FloorZSector(p.X, p.Y, p.Z+0x28, &sector); z != physics.NoFloor {
			p.Teleport(p.X, p.Y, z+1, p.Dir)
			p.Sector = sector
		}
	case w.outdoor != nil:
		z, _, _, _ := w.outdoor.geo.FloorZ(p.X, p.Y, p.Z, p.Levitate, p.WaterWalk)
		p.Teleport(p.X, p.Y, z, p.Dir)
	}
}

// eye is the camera the party sees through: 0x19 units behind its position
// (Party+0x18), at its eye level, turned by Dir and pitched by Look.
//
// mm8: 0x47b25b (Outdoor_Render), 0x43ed51 (Render_Frame, indoors), 0x43e540 (IndoorView_Setup)
func (w *World) eye() (x, y, z, yaw, pitch float64) {
	p := w.group
	ex := p.X - physics.Mul16(cameraBack, physics.Cos(p.Dir))
	ey := p.Y - physics.Mul16(cameraBack, physics.Sin(p.Dir))
	ez := p.EyeLevel + p.Z
	if w.indoor != nil && w.indoor.geo.SectorAt(ex, ey, ez) == 0 {
		ex, ey = p.X, p.Y // IndoorView_Setup: outside every sector, look from the party
	}
	return float64(ex), float64(ey), float64(ez), float64(p.Dir), float64(p.Look)
}

// cameraBack is how far behind the party the eye sits.
//
// mm8: 0x492487 (Party_Ctor: +0x18 = 0x19)
const cameraBack = 0x19

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
	if !w.FreeCamOn {
		w.cam.X, w.cam.Y, w.cam.Z, w.cam.Yaw, w.cam.Pitch = w.eye()
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
