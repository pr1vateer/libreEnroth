package world

import (
	"encoding/binary"
	"log"
	"math"
	"sort"
	"strings"

	"libre-enroth/internal/assets"
	"libre-enroth/internal/evt"
	"libre-enroth/internal/game/clock"
	"libre-enroth/internal/game/party"
	"libre-enroth/internal/game/physics"
	"libre-enroth/internal/game/ui"
	"libre-enroth/internal/maps/blv"
	"libre-enroth/internal/maps/delta"
	"libre-enroth/internal/maps/odm"
)

// Event triggers on the map: faces, decorations and the rest of what the map's script
// addresses. The world is the VM's host (evt.Host).
var _ evt.Host = (*World)(nil)

// Face attributes the event code reads (indoor and BModel faces share them).
const (
	attrHintOnly   = 0x100000  // the event only gives a hint (FaceExtra_HasHint 0x44a90d)
	attrClickable  = 0x2000000 // clicking runs the event
	attrPressPlate = 0x4000000 // stepping on it runs the event
)

// Distances of the event triggers (Mouse_UpdateHover 0x420aab, Evt_Click 0x421a65,
// Evt_Interact 0x469b64).
const (
	clickDepth   = 0x200 // faces and decorations further away do not react to a click
	interactHalf = 100   // Space looks at the view's centre +-100 UI pixels
)

// PIDs: what the pick buffer holds (index << 3 | kind, outdoor faces model << 6 | face).
const (
	pidDecoration = 5
	pidFace       = 6
)

func facePID(i int) uint32               { return uint32(i)<<3 | pidFace }
func modelFacePID(m, f int) uint32       { return uint32(m<<6|f)<<3 | pidFace }
func decorationPID(i int) uint32         { return uint32(i)<<3 | pidDecoration }
func pidKind(pid uint32) uint32          { return pid & 7 }
func pidIndex(pid uint32) int            { return int(pid >> 3) }
func pidModelFace(pid uint32) (m, f int) { return int(pid >> 9), int(pid>>3) & 0x3f }

// loadScripts reads the map's .evt and .str (either may be missing: out09 and out10
// have no .evt) and builds the VM with global.evt.
//
// mm8: 0x442119 (Evt_LoadMapEvents), 0x441bcb (Evt_BuildMapIndex)
func (w *World) loadScripts(d *assets.Data) error {
	base := w.name[:len(w.name)-4]
	w.vm = &evt.VM{Global: w.tables.Global, Host: w}
	if _, b, err := d.LangFile(base + ".evt"); err == nil {
		s, err := evt.Parse(b, evt.MapMaxBytes)
		if err != nil {
			return err
		}
		w.vm.Map = s
	}
	if _, b, err := d.LangFile(base + ".str"); err == nil {
		s, err := evt.ParseStrings(b)
		if err != nil {
			return err
		}
		w.strs = s
	}
	return nil
}

// enter runs the map-load part of the events: OnMapReload with messages off, then the
// timers start (the long ones count from the last visit).
//
// mm8: 0x46411c (map load: Level_Load, then Evt_InitTimers 0x441caf)
func (w *World) enter() {
	w.S.deferredNPC = 0
	w.generateChests()
	w.vm.MapReload()
	var last clock.Time
	if dl := w.delta(); dl != nil {
		last = clock.Time(binary.LittleEndian.Uint64(dl.Time[0:8]))
	}
	w.timers = evt.InitTimers(w.vm.Map, w.S.Party.Time, last)
}

// leave stores the time of this visit (the delta's location time, which long timers
// count from).
func (w *World) leave() {
	if dl := w.delta(); dl != nil {
		binary.LittleEndian.PutUint64(dl.Time[0:8], uint64(w.S.Party.Time))
	}
}

func (w *World) delta() *delta.Delta {
	switch {
	case w.indoor != nil:
		return w.indoor.Delta
	case w.outdoor != nil:
		return w.outdoor.Delta
	}
	return nil
}

// RunEvent runs map event id (from step 0). targetPID is what triggered it (the original
// keeps it for traps; unused here).
func (w *World) RunEvent(id int, canShow bool) evt.Result {
	return w.vm.Run(evt.Source{Kind: evt.SourceMap}, id, 0, canShow)
}

// eventHooks are the party's move hooks: a pressure plate runs its map event.
type eventHooks struct {
	party.NoHooks
	w *World
}

// FaceEvent runs the event of a pressure plate the party stepped on.
//
// mm8: 0x472e86, 0x473fee (Party_MoveIndoor/Outdoor: Evt_Process(event, 0, 1))
func (h eventHooks) FaceEvent(event int) { h.w.RunEvent(event, true) }

// FallDamage hurts the members after a hard landing (the stats' tables loaded).
//
// mm8: 0x472e86, 0x473fee (Party_MoveIndoor/Outdoor)
func (h eventHooks) FallDamage(height int32) {
	if c := h.w.S.Ctx; c != nil && c.Items != nil && c.Classes != nil {
		h.w.S.Party.FallDamage(height, c)
	}
}

// ---- decorations ----------------------------------------------------------------------

// decView is a level decoration of either map kind.
type decView struct {
	flags                             *uint16
	pos                               [3]int32
	yaw                               int32
	cog, event, radius, degrees, evar int16
	name                              string
	idx                               *int // ddeclist.bin index
}

func (w *World) numDecorations() int {
	switch {
	case w.indoor != nil:
		return len(w.indoor.Map.Decorations)
	case w.outdoor != nil:
		return len(w.outdoor.Map.Decorations)
	}
	return 0
}

func (w *World) decoration(i int) decView {
	if w.indoor != nil {
		d := &w.indoor.Map.Decorations[i]
		return decView{&d.Flags, d.Pos, d.Yaw, d.Cog, d.Event, d.Radius, d.Degrees, d.EventVar, d.Name, &w.indoor.decIdx[i]}
	}
	d := &w.outdoor.Map.Decorations[i]
	return decView{&d.Flags, [3]int32{d.Pos.X, d.Pos.Y, d.Pos.Z}, d.Yaw, d.Cog, d.Event, d.Radius, d.Degrees, d.EventVar, d.Name, &w.outdoor.decIdx[i]}
}

// interactive reports a decoration whose event lives in a map variable and runs from
// global.evt (barrels, cauldrons, fountains...): ddeclist index + 1 in 0xc1..0xd9 or
// 0xe4.
//
// mm8: 0x44a961
func interactive(decIdx int) bool {
	i := decIdx + 1
	return i > 0xc0 && (i < 0xda || i == 0xe4)
}

// decodeEvent and encodeEvent convert an interactive decoration's global event to and
// from its map variable: 0..0x15 <-> 268..289, 0x17..0x3e <-> 531..570.
//
// mm8: 0x44f614 (decode), 0x44f5b2 (encode)
func decodeEvent(v int) int {
	switch {
	case v >= 0 && v <= 0x15:
		return v + 0x10c
	case v >= 0x17 && v <= 0x3e:
		return v + 0x1fc
	}
	return 0 // "Error in decode event"
}

func encodeEvent(e int) int {
	switch {
	case e >= 0x10c && e <= 0x121:
		return e - 0x10c
	case e >= 0x213 && e <= 0x23a:
		return e - 0x1fc
	}
	return 0 // "Error in encode event"
}

// runDecoration runs a decoration's event: its map event, or for an interactive one
// the global event its map variable holds. Reports whether there was one.
func (w *World) runDecoration(i int) bool {
	d := w.decoration(i)
	if d.event != 0 {
		w.RunEvent(int(d.event), true)
		return true
	}
	if !interactive(*d.idx) {
		return false
	}
	mv := w.MapVars()
	if mv == nil || int(d.evar) < 0 || int(d.evar) >= len(mv) {
		return false
	}
	w.vm.Run(evt.Source{Kind: evt.SourceDecoration, Dec: i}, decodeEvent(int(mv[d.evar])), 0, true)
	return true
}

// proximity runs the events of decorations that trigger on the party: every tick it is
// within their radius (max + mid*11/32 + min/4 of the axis distances). Actor and
// object triggers are M8/M9's.
//
// mm8: 0x46ce50 (Evt_Proximity, from World_Tick)
func (w *World) proximity() {
	p := w.group
	for i := range w.numDecorations() {
		d := w.decoration(i)
		if *d.flags&odm.DecTriggerParty == 0 || d.event == 0 {
			continue
		}
		a := [3]int32{abs32(d.pos[0] - p.X), abs32(d.pos[1] - p.Y), abs32(d.pos[2] - p.Z)}
		sort.Slice(a[:], func(i, j int) bool { return a[i] > a[j] })
		if a[0]+a[2]>>2+a[1]*11>>5 < int32(d.radius) {
			w.RunEvent(int(d.event), true)
		}
	}
}

// ---- picking ---------------------------------------------------------------------------

// pick reads the last rendered frame's pick buffer under UI point (x, y).
func (w *World) pick(x, y int) (pid uint32, depth float64) {
	f := w.frame
	if f == nil {
		return 0, 0
	}
	v := ui.Viewport
	fx := (float64(x-v.Min.X) + 0.5) * float64(f.W) / float64(v.Dx())
	fy := (float64(y-v.Min.Y) + 0.5) * float64(f.H) / float64(v.Dy())
	return f.PickAt(int(math.Floor(fx)), int(math.Floor(fy)))
}

// faceEvent is the event and attributes of a picked face.
func (w *World) faceEvent(pid uint32) (event int, attr uint32, ok bool) {
	switch {
	case w.indoor != nil:
		i := pidIndex(pid)
		m := w.indoor.Map
		if i >= len(m.Faces) {
			return 0, 0, false
		}
		f := &m.Faces[i]
		return int(m.FaceExtras[f.Extra].Event), f.Attr, true
	case w.outdoor != nil:
		mi, fi := pidModelFace(pid)
		if mi >= len(w.outdoor.Map.Models) || fi >= len(w.outdoor.Map.Models[mi].Faces) {
			return 0, 0, false
		}
		f := &w.outdoor.Map.Models[mi].Faces[fi]
		return int(f.Event), f.Attr, true
	}
	return 0, 0, false
}

// decRadius is a decoration's ddeclist radius.
func (w *World) decRadius(i int) float64 {
	if di := *w.decoration(i).idx; di > 0 && di < len(w.tables.Decs) {
		return float64(w.tables.Decs[di].Radius)
	}
	return 0
}

// Click handles a left click in the view at UI point (x, y): a decoration less than
// 512 units (beyond its radius) away runs its event, a clickable face less than 512
// away its event; another face says "Nothing here". Items and monsters are M7/M8's.
//
// mm8: 0x421a65 (Evt_Click)
func (w *World) Click(x, y int) {
	pid, depth := w.pick(x, y)
	switch pidKind(pid) {
	case pidDecoration:
		if i := pidIndex(pid); i < w.numDecorations() && depth-w.decRadius(i) < clickDepth {
			w.runDecoration(i)
		}
	case pidFace:
		if depth >= clickDepth {
			return
		}
		event, attr, ok := w.faceEvent(pid)
		if !ok {
			return
		}
		if attr&attrClickable != 0 {
			w.RunEvent(event, true)
			return
		}
		w.NothingHere()
	}
}

// Interact is the interact key (Space): the faces and decorations within 512 units in
// the middle of the view, nearest first, are tried until one runs an event. A
// decoration remembers that it was used (flag 8).
//
// mm8: 0x469b64 (Evt_Interact: the software path's scan of the view's centre), 0x469d53
// (Evt_InteractObject)
func (w *World) Interact() {
	f := w.frame
	if f == nil {
		return
	}
	type cand struct {
		pid   uint32
		depth float64
	}
	var cands []cand
	seen := map[uint32]bool{}
	v := ui.Viewport
	cx := (v.Min.X + v.Max.X - 1) >> 1
	for y := v.Min.Y; y < v.Max.Y; y += 2 {
		for x := cx - interactHalf; x < cx+interactHalf; x += 2 {
			pid, depth := w.pick(x, y)
			if pid == 0 || seen[pid] || len(cands) >= 100 {
				continue
			}
			switch pidKind(pid) {
			case pidDecoration:
				if i := pidIndex(pid); i >= w.numDecorations() || depth-w.decRadius(i) > clickDepth {
					continue
				}
			default:
				if depth >= clickDepth {
					continue
				}
			}
			seen[pid] = true
			cands = append(cands, cand{pid, depth})
		}
	}
	sort.SliceStable(cands, func(i, j int) bool {
		if int(cands[i].depth) != int(cands[j].depth) {
			return cands[i].depth < cands[j].depth
		}
		return cands[i].pid < cands[j].pid
	})
	for _, c := range cands {
		if w.interactObject(c.pid) {
			return
		}
	}
}

// interactObject tries one picked object for Interact; it reports that it ran an event.
//
// mm8: 0x469d53
func (w *World) interactObject(pid uint32) bool {
	switch pidKind(pid) {
	case pidDecoration:
		i := pidIndex(pid)
		d := w.decoration(i)
		if d.event == 0 {
			return w.runDecoration(i)
		}
		w.RunEvent(int(d.event), true)
		*d.flags |= odm.DecVisibleOnMap
		return true
	case pidFace:
		event, attr, ok := w.faceEvent(pid)
		if !ok {
			return false
		}
		if w.indoor != nil && attr&attrClickable == 0 {
			w.NothingHere()
			return false
		}
		if attr&attrHintOnly != 0 || event == 0 {
			return false
		}
		w.RunEvent(event, true)
		return true
	}
	return false // items and monsters: M7/M8
}

// Hover is the status line text for UI point (x, y) in the view: the hint of a face's
// or decoration's event, an interactive decoration's topic name (npctopic.txt of its
// event) or a decoration's name. "" clears the hover text.
//
// mm8: 0x420aab (Mouse_UpdateHover, viewport part), 0x4424e6 (Evt_HoverText)
func (w *World) Hover(x, y int) string {
	pid, depth := w.pick(x, y)
	switch pidKind(pid) {
	case pidFace:
		if depth >= clickDepth {
			return ""
		}
		event, attr, ok := w.faceEvent(pid)
		if !ok || w.indoor != nil && attr&(attrClickable|attrPressPlate) == 0 || event == 0 {
			return ""
		}
		return w.hoverText(event)
	case pidDecoration:
		i := pidIndex(pid)
		if i >= w.numDecorations() {
			return ""
		}
		d := w.decoration(i)
		switch {
		case d.event != 0:
			return w.hoverText(int(d.event))
		case interactive(*d.idx):
			if mv := w.MapVars(); mv != nil && int(d.evar) >= 0 && int(d.evar) < len(mv) {
				return w.interactiveName(decodeEvent(int(mv[d.evar])))
			}
			return ""
		case *d.idx > 0 && *d.idx < len(w.tables.Decs):
			return w.tables.Decs[*d.idx].GameName
		}
	}
	return ""
}

func (w *World) hoverText(event int) string {
	if w.vm.Map == nil {
		return ""
	}
	return w.vm.Map.HoverText(event, w.strs, w.tables.HouseName)
}

// ---- evt.Host ----------------------------------------------------------------------------

// Members implements evt.Host.
func (w *World) Members() *party.Members { return w.S.Party }

// Ctx implements evt.Host.
func (w *World) Ctx() *party.Ctx { return w.S.Ctx }

// MapVars implements evt.Host.
func (w *World) MapVars() *[200]byte {
	if dl := w.delta(); dl != nil {
		return &dl.Vars
	}
	return nil
}

// Location implements evt.Host.
func (w *World) Location() []byte {
	if dl := w.delta(); dl != nil {
		return dl.Header[:]
	}
	return nil
}

// Str implements evt.Host.
func (w *World) Str(i int) string {
	if i < 0 || i >= len(w.strs) {
		return ""
	}
	return w.strs[i]
}

// Global implements evt.Host.
func (w *World) Global(i int) string { return w.S.global(i) }

// NothingHere implements evt.Host.
//
// mm8: 0x44a8a2 (Status_NothingHere)
func (w *World) NothingHere() {
	if st := w.S.status(); st.Timed() == "" {
		st.Show(w.S.global(0x209), 2)
	}
}

// Status implements evt.Host.
func (w *World) Status(text string, seconds int) { w.S.status().Show(text, seconds) }

// Message implements evt.Host: the map's message text (drawn by M6's dialogue box).
func (w *World) Message(text string) { w.MessageText = text }

// Teleport implements evt.Host: the party moves within the map, at rest.
//
// mm8: 0x4446bd (case 6, map name "0")
func (w *World) Teleport(x, y, z, dir, look, vz int32) {
	p := w.group
	if dir == -1 {
		dir = p.Dir
	}
	p.Teleport(x, y, z, dir)
	p.Look, p.VZ = look, vz
}

// MoveToMap implements evt.Host: the move happens after the frame (the UI loads the
// map), the party arriving at its Party Start with the non-zero values replacing it.
//
// mm8: 0x447f80 (Evt_Travel), 0x44808d (Level_PlaceParty)
func (w *World) MoveToMap(name string, x, y, z, dir, look, vz int32) {
	w.travel = &Arrival{Map: name, X: x, Y: y, Z: z, Dir: dir, Look: look, VZ: vz}
}

// Travel implements ui.Traveler: the map a MoveToMap asked for, once.
func (w *World) Travel() (string, bool) {
	if w.travel == nil {
		return "", false
	}
	w.leave()
	w.S.arrival, w.S.carry = w.travel, w.group
	w.travel = nil
	return w.S.arrival.Map, true
}

// SetDoor implements evt.Host.
func (w *World) SetDoor(id, action int) {
	if w.indoor != nil {
		w.indoor.SetDoorState(uint32(id), action)
	}
}

// StopDoor implements evt.Host: the door stops where it is (attr 8; the sound is M11).
//
// mm8: 0x448201 (Door_Stop)
func (w *World) StopDoor(id int) {
	if d := w.indoor.door(uint32(id)); d != nil {
		d.Attr |= delta.DoorStopped
	}
}

// SetTexture implements evt.Host: every face with the cog gets the bitmap (nothing if
// it does not load); an animated face stays animated if the name is a dtft.bin
// sequence, else it turns static.
//
// mm8: 0x4469b1 (Evt_SetTexture)
func (w *World) SetTexture(cog int32, name string) {
	if cog == 0 || w.tex.Bitmap(name) == nil {
		return
	}
	set := func(tex *string, attr *uint32, animated uint32) {
		*tex = name
		if *attr&animated != 0 && w.tables.TFT.Find(name) < 0 {
			*attr &^= animated
		}
	}
	switch {
	case w.indoor != nil:
		m := w.indoor.Map
		for i := 1; i < len(m.FaceExtras); i++ {
			if x := &m.FaceExtras[i]; int32(x.Cog) == cog {
				f := &m.Faces[x.Face]
				set(&f.Texture, &f.Attr, blv.FaceAnimated)
			}
		}
	case w.outdoor != nil:
		for mi := range w.outdoor.Map.Models {
			for fi := range w.outdoor.Map.Models[mi].Faces {
				if f := &w.outdoor.Map.Models[mi].Faces[fi]; int32(f.Cog) == cog {
					set(&f.Texture, &f.Attr, odm.FaceAnimated)
				}
			}
		}
	}
}

// SetSprite implements evt.Host: decorations with the cog show or hide (and block or
// not), and change to the named ddeclist entry unless the name is "0".
//
// mm8: 0x446c17 (Evt_SetSprite)
func (w *World) SetSprite(cog int32, visible bool, name string) {
	for i := range w.numDecorations() {
		d := w.decoration(i)
		if int32(d.cog) != cog {
			continue
		}
		if name != "" && !strings.EqualFold(name, "0") {
			*d.idx = w.tables.Decs.Find(name)
		}
		if visible {
			*d.flags &^= decHidden
		} else {
			*d.flags |= decHidden
		}
		w.updateDecorationCollision(i)
	}
}

// updateDecorationCollision refreshes one decoration's collision cylinder.
func (w *World) updateDecorationCollision(i int) {
	d := w.decoration(i)
	var list []physics.Decoration
	switch {
	case w.indoor != nil:
		list = w.indoor.geo.Decorations
	case w.outdoor != nil:
		list = w.outdoor.geo.Decorations
	}
	if i >= len(list) {
		return
	}
	one := collisionDecorations(w.tables.Decs, []int{*d.idx}, 1, func(int) (uint16, [3]int32) { return *d.flags, d.pos })
	list[i] = one[0]
}

// SetFacesBit implements evt.Host: sets or clears attribute bits of every face with
// the cog.
//
// mm8: 0x446d4a (Evt_SetFacesBit)
func (w *World) SetFacesBit(cog int32, bits uint32, on bool) {
	if cog == 0 {
		return
	}
	set := func(a *uint32) {
		if on {
			*a |= bits
		} else {
			*a &^= bits
		}
	}
	switch {
	case w.indoor != nil:
		m := w.indoor.Map
		for i := 1; i < len(m.FaceExtras); i++ {
			if x := &m.FaceExtras[i]; int32(x.Cog) == cog {
				set(&m.Faces[x.Face].Attr)
			}
		}
	case w.outdoor != nil:
		for mi := range w.outdoor.Map.Models {
			for fi := range w.outdoor.Map.Models[mi].Faces {
				if f := &w.outdoor.Map.Models[mi].Faces[fi]; int32(f.Cog) == cog {
					set(&f.Attr)
				}
			}
		}
	}
}

// SetLight implements evt.Host: indoor lights with the id turn on or off.
//
// mm8: 0x446cdf (Evt_ToggleIndoorLight)
func (w *World) SetLight(id int32, on bool) {
	if w.indoor == nil || id < 0 {
		return
	}
	for i := range w.indoor.Map.Lights {
		l := &w.indoor.Map.Lights[i]
		if l.ID != id {
			continue
		}
		if on {
			l.Flags &^= blv.LightOff
		} else {
			l.Flags |= blv.LightOff
		}
	}
}

// ChangeEvent implements evt.Host: an interactive decoration gets another global
// event; 0 empties and hides it.
//
// mm8: 0x4446bd (case 0x2a)
func (w *World) ChangeEvent(dec int, event int32) {
	if dec < 0 || dec >= w.numDecorations() {
		return
	}
	d := w.decoration(dec)
	mv := w.MapVars()
	if mv == nil || int(d.evar) < 0 || int(d.evar) >= len(mv) {
		return
	}
	if event == 0 {
		mv[d.evar] = 0
		*d.flags |= decHidden
		w.updateDecorationCollision(dec)
		return
	}
	mv[d.evar] = byte(encodeEvent(int(event)))
}

// ToggleChestFlag implements evt.Host: the chest's flags (+2) in the delta.
//
// mm8: 0x446e44
func (w *World) ToggleChestFlag(chest int32, bit uint16, on bool) {
	dl := w.delta()
	if dl == nil || chest < 0 || chest >= 20 || int(chest) >= len(dl.Chests) {
		return
	}
	c := &dl.Chests[chest]
	if on {
		c.Flags |= bit
	} else {
		c.Flags &^= bit
	}
}

// PartyBuff implements evt.Host: fly (7) is the debug toggle; the rest is M9's.
func (w *World) PartyBuff(i int) bool { return i == 7 && w.group.Fly }

// Flying implements evt.Host.
func (w *World) Flying() bool { return w.group.Flying }

// Stub implements evt.Host: what later milestones do is logged once per opcode or
// variable.
func (w *World) Stub(op evt.Op, r evt.Record, v evt.Var) {
	key := [2]int{int(op), int(v)}
	if w.S.stubbed == nil {
		w.S.stubbed = map[[2]int]bool{}
	}
	if w.S.stubbed[key] {
		return
	}
	w.S.stubbed[key] = true
	logf := w.S.Log
	if logf == nil {
		logf = log.Printf
	}
	if v >= 0 {
		logf("evt: %v of variable %#x: the player stats are M7's", op, int(v))
		return
	}
	logf("evt: %v (%s) not done yet", op, op.Milestone())
}

// ---- arrival ---------------------------------------------------------------------------

// Arrival is where a MoveToMap puts the party on the new map: its Party Start, with
// the non-zero values replacing the position, heading, pitch and vertical speed.
type Arrival struct {
	Map                    string
	X, Y, Z, Dir, Look, VZ int32
	// Start picks the marker: 0 "Party Start", 1..4 the North/South/East/West Start a
	// map edge arrives at (edgeStarts).
	Start int
	// Ground puts the party on the terrain (a map edge) rather than on the floor under
	// it.
	Ground bool
}

// placeAtStart puts the party on the arrival's marker ("Party Start" unless it says
// another; heading from its degrees, or its yaw when set), then applies the arrival's
// non-zero values. It reports whether the map has the marker.
//
// mm8: 0x44808d (Level_PlaceParty)
func (w *World) placeAtStart(a *Arrival) bool {
	p := w.group
	found := false
	marker := edgeStarts[0]
	if a != nil && a.Start > 0 && a.Start < len(edgeStarts) {
		marker = edgeStarts[a.Start]
	}
	for i := range w.numDecorations() {
		d := w.decoration(i)
		if !strings.EqualFold(d.name, marker) {
			continue
		}
		dir := int32(d.degrees) * 512 / 90
		if d.yaw != 0 {
			dir = d.yaw
		}
		p.Teleport(d.pos[0], d.pos[1], d.pos[2], dir)
		found = true
		break
	}
	if a == nil {
		return found
	}
	x, y, z, dir := p.X, p.Y, p.Z, p.Dir
	if a.X != 0 {
		x = a.X
	}
	if a.Y != 0 {
		y = a.Y
	}
	if a.Z != 0 {
		z = a.Z
	}
	if a.Dir != 0 {
		dir = a.Dir
	}
	p.Teleport(x, y, z, dir)
	if a.Look != 0 {
		p.Look = a.Look
	}
	if a.VZ != 0 {
		p.VZ = a.VZ
	}
	return true
}

// ---- doors -------------------------------------------------------------------------------

// door is the first door with the id, nil for none.
func (in *Indoor) door(id uint32) *delta.Door {
	if in == nil || in.Delta == nil {
		return nil
	}
	for i := range in.Delta.Doors {
		if in.Delta.Doors[i].ID == id {
			return &in.Delta.Doors[i]
		}
	}
	return nil
}

// SetDoorState starts door id moving: action 0 opens it, 1 closes it, 2 toggles a door
// at rest. A door moving the other way turns round where it is (its time is set to
// what the new direction would have taken to get there); one already moving that way
// or at rest there stays. A stopped door is released.
//
// mm8: 0x448263 (Door_SetState)
func (in *Indoor) SetDoorState(id uint32, action int) {
	d := in.door(id)
	if d == nil {
		return
	}
	d.Attr &^= delta.DoorStopped
	reverse := func(state uint16, speed, other int32) {
		t := int32(0x3c00)
		if d.Time != 0x3c00 && speed != 0 {
			t = (d.MoveLength<<7)/speed - ((other*d.Time/0x80)<<7)/speed
		}
		d.State, d.Time = state, t
	}
	closeDoor := func() {
		switch d.State {
		case delta.DoorOpen:
			d.State, d.Time = delta.DoorClosing, 0
		case delta.DoorOpening:
			reverse(delta.DoorClosing, d.CloseSpeed, d.OpenSpeed)
		}
	}
	openDoor := func() {
		switch d.State {
		case delta.DoorClosed:
			d.State, d.Time = delta.DoorOpening, 0
		case delta.DoorClosing:
			reverse(delta.DoorOpening, d.OpenSpeed, d.CloseSpeed)
		}
	}
	switch action {
	case 0:
		openDoor()
	case 1:
		closeDoor()
	case 2:
		switch d.State {
		case delta.DoorOpen:
			closeDoor()
		case delta.DoorClosed:
			openDoor()
		}
	}
}
