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
	"libre-enroth/internal/assets/tables"
	"libre-enroth/internal/assets/txt"
	"libre-enroth/internal/evt"
	"libre-enroth/internal/game/dialog"
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
	Global   *evt.Script // global.evt
	// Game are the house, NPC and dialogue tables (tables.Load).
	Game *tables.All
	// Chests are the chest grids, pictures and treasure levels (tables.LoadChests).
	Chests *tables.Chests
}

// HouseName is the name of 2DEvents house id, "" if none.
//
// mm8: 0x4414e8 (Txt_Load2DEvents: row id at 0x5a5670 + id*0x34, name at +4)
func (t *Tables) HouseName(id int) string {
	if t.Game == nil {
		return ""
	}
	if h := t.Game.House(id); h != nil {
		return h.Name
	}
	return ""
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
	// mm8: 0x441a6f (Evt_LoadGlobal)
	parse("global.evt", func(b []byte) (e error) { t.Global, e = evt.Parse(b, evt.GlobalMaxBytes); return })
	if err == nil {
		t.Game, err = tables.Load(d)
	}
	if err == nil {
		t.Chests, err = tables.LoadChests(d)
	}
	return t, err
}

// MapStat is column col of the map's MapStats.txt row as a number (0 when absent): 9
// the trap level (+0x2d), 11 the treasure level (+0x2f).
//
// mm8: 0x452a82 (Txt_LoadMapStats)
func (t *Tables) MapStat(name string, col int) int {
	if t.MapStats == nil {
		return 0
	}
	for _, row := range t.MapStats.Rows {
		if len(row) > col && len(row) > 2 && strings.EqualFold(row[2], name) {
			return int(txt.Atoi(row[col]))
		}
	}
	return 0
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
	// Clock is the time of day the view is lit for and the animation clock; Update
	// sets the hour and minute from the session's calendar.
	Clock Clock
	// S is the session: the party members, their clock and the RNG.
	S *Session
	// PartyDead is set when the clock finds nobody able to act (g_partyCreateResult 8;
	// what follows is M7/M8).
	PartyDead bool

	group  *party.Party
	name   string
	vm     *evt.VM
	strs   []string // the map's .str
	timers *evt.Timers
	travel *Arrival // a MoveToMap to another map, done after the frame
	// transition is the open transition dialogue (a MoveToMap asking, a map edge).
	transition *transition
	frame      *render.Frame // the last rendered view, for picking
	// MessageText and ReplyText are the map's message (g_evtMessage 0x5c678c) and the
	// dialogue reply (0xffd350).
	MessageText, ReplyText string
	dialog                 *dialog.Dialog
	inn                    int // the Adventurer's Inn's house shown, 0 none
	message                *message
	tex                    *TextureCache
	tables                 *Tables
	outdoor                *Outdoor
	indoor                 *Indoor
	r                      render.Renderer
	cam                    render.Camera
	subTicks               int
	// openChest is the chest an OpenChest event opened, until the screen closes.
	openChest *ui.ChestView
}

var _ ui.World = (*World)(nil)

// Load opens a map from games.lod, with its .ddm/.dlv template, into session s (nil:
// a new session with a one-member party), and runs its map-load events. A map visited
// before in the session comes back as it was left. After a MoveToMap the party arrives
// where it said.
func Load(d *assets.Data, tables *Tables, tex *TextureCache, name string, s *Session) (*World, error) {
	if s == nil {
		s = NewSession()
	}
	key := strings.ToLower(name)
	w, ok := s.worlds[key]
	if !ok {
		var err error
		if w, err = load(d, tables, tex, name, s); err != nil {
			return nil, err
		}
		if s.worlds == nil {
			s.worlds = map[string]*World{}
		}
		s.worlds[key] = w
	} else {
		w.group = party.New(w.group.X, w.group.Y, w.group.Z, w.group.Dir)
		if w.indoor != nil {
			w.indoor.settleDoors()
		}
	}
	w.group.Hooks = eventHooks{w: w}
	if m := s.Party; m.Roster == nil && tables.Game != nil {
		m.Roster = party.NewRoster(tables.Game.Roster, s.Ctx, &m.ArtifactsFound)
	}
	if c := s.carry; c != nil {
		w.group.Fly, w.group.WaterWalk, w.group.FeatherFall, w.group.Levitate = c.Fly, c.WaterWalk, c.FeatherFall, c.Levitate
		w.group.TurnDelta = c.TurnDelta
		s.carry = nil
	}
	if a := s.arrival; a != nil {
		if !w.placeAtStart(a) {
			w.placeDefault()
		}
		if a.Ground && w.outdoor != nil {
			// mm8: 0x42f877 (msg 0x5a: z = Terrain_HeightAt(1, x, y))
			p := w.group
			z, _, _ := w.outdoor.geo.TerrainZ(p.X, p.Y, p.Levitate, false, false)
			p.Teleport(p.X, p.Y, z, p.Dir)
		} else {
			w.dropParty()
		}
		s.arrival = nil
	} else if ok {
		w.placeDefault()
	}
	w.travel, w.transition, w.PartyDead = nil, nil, false
	w.syncClock()
	w.enter()
	return w, nil
}

// placeDefault puts the party where the map's default camera stands.
func (w *World) placeDefault() {
	w.Cam = w.defaultCam()
	w.group.Teleport(int32(w.Cam.X), int32(w.Cam.Y), int32(w.Cam.Z), int32(w.Cam.Yaw))
	w.dropParty()
}

func (w *World) defaultCam() FreeCam {
	if w.outdoor != nil {
		return w.outdoor.DefaultCamera()
	}
	return w.indoor.DefaultCamera()
}

func load(d *assets.Data, tables *Tables, tex *TextureCache, name string, s *Session) (*World, error) {
	raw, err := d.Games.Raw(name)
	if err != nil {
		return nil, err
	}
	blob, err := lod.UnpackMap(raw)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	w := &World{name: name, tex: tex, tables: tables, S: s}
	w.syncClock()
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
	if err := w.loadScripts(d); err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
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

// Update implements ui.World: one 60 Hz tick of the game loop: the clock (stopped in
// turn-based mode), input, party movement and doors. Debug keys: F2 opens/closes
// every door indoors, F3 toggles the free camera (leaving it puts the party where the
// camera is), F4 the fly buff, F5 water walking.
//
// mm8: 0x46261d (Game_Loop: Timer_Update, Party_UpdateTime unless the timer is paused
// or stopped, then World_Tick)
func (w *World) Update(in *ui.Input) {
	w.subTicks += TicksPerSecond
	ticks := w.subTicks / 60
	w.Clock.Ticks += ticks
	w.subTicks %= 60
	m := w.S.Party
	w.timers.Scan(m.Time, &w.S.timerScan, func(id, step int) { w.vm.Run(evt.Source{Kind: evt.SourceMap}, id, step, true) })
	if !m.TurnBased && m.UpdateTime(ticks, w.S.Ctx) {
		w.PartyDead = true
	}
	w.syncClock()
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
		w.proximity()
	}
	if w.indoor != nil {
		if in.Pressed(ui.KeyF2) {
			w.indoor.ToggleDoors()
		}
		w.indoor.UpdateDoors(ticks)
	}
}

// syncClock lights the view for the calendar's time of day.
func (w *World) syncClock() {
	c := w.S.Party.Calendar
	w.Clock.Hour, w.Clock.Minute = c.Hour, c.Minute
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
		w.tickEdge()
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
	f.EnablePick()
	w.frame = f
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
