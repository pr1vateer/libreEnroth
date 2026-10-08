package world

import (
	"encoding/binary"
	"fmt"
	"math"
	"strings"

	"libre-enroth/internal/assets/desc"
	"libre-enroth/internal/assets/tables"
	"libre-enroth/internal/evt"
	"libre-enroth/internal/game/items"
	"libre-enroth/internal/game/monsters"
	"libre-enroth/internal/game/physics"
	"libre-enroth/internal/game/ui"
	"libre-enroth/internal/maps/delta"
	"libre-enroth/internal/render"
)

// The map's monsters and ground objects live in its delta (the .ddm/.dlv records), so
// that a map visited again comes back as it was left (re/notes/monsters.md).

// Pick ids of actors and objects (Outdoor_AddActorBillboards 0x47c1d2,
// Outdoor_AddObjectBillboards 0x47bca5).
const (
	pidObject = 2
	pidActor  = 3
)

func actorPID(i int) uint32  { return uint32(i)<<3 | pidActor }
func objectPID(i int) uint32 { return uint32(i)<<3 | pidObject }

// Hover and click distances (Mouse_UpdateHover 0x420aab, Evt_Click 0x421a65).
const (
	actorNameDepth = 0x1400 // actor names show nearer than this
)

// Global.txt strings.
const (
	txtGetItem   = 0x1d6 // "Get %s"
	txtFoundItem = 0x1d7 // "You found %s!"
)

// Speech events of the member who greets an actor (Player_Speak).
const (
	speechGreetDay   = 0x16
	speechGreetNight = 0x17
)

// Frame flag 0x20: the object's sprite is centred on its position.
const frameCentered = 0x20

// Map kinds for the alert flag.
const (
	kindIndoor = iota
	kindOutdoor
)

func (w *World) kind() int {
	if w.indoor != nil {
		return kindIndoor
	}
	return kindOutdoor
}

// monsterTables are the monster tables, nil without them (tests without the game tables).
func (w *World) monsterTables() *tables.Monsters {
	if w.tables == nil || w.tables.Game == nil {
		return nil
	}
	return w.tables.Game.Monsters
}

// Actors are the map's actors (nil without a delta).
func (w *World) Actors() []monsters.Actor {
	if dl := w.delta(); dl != nil {
		return dl.Actors
	}
	return nil
}

// Objects are the map's ground objects.
func (w *World) Objects() []monsters.Object {
	if dl := w.delta(); dl != nil {
		return dl.Objects
	}
	return nil
}

// alert is Map_IsAlert: location header +0xc - of the last map of the other kind, as
// the original reads it (indoors the outdoor header, outdoors the indoor one).
//
// mm8: 0x44f967 (Map_IsAlert)
func (w *World) alert() uint32 {
	other := kindOutdoor
	if w.kind() == kindOutdoor {
		other = kindIndoor
	}
	d := w.S.lastDelta[other]
	if d == nil {
		return 0
	}
	return binary.LittleEndian.Uint32(d.Header[0xc:])
}

// mapStats is the map's MapStats.txt row, nil when it has none.
func (w *World) mapStats() *tables.MapStats {
	m := w.monsterTables()
	if m == nil {
		return nil
	}
	return m.MapStatsRow(w.tables.MapIndex(w.name))
}

// monsterEnv is what spawning reads.
func (w *World) monsterEnv() *monsters.Env {
	e := &monsters.Env{
		Tables: w.monsterTables(), Items: w.tables.Game.Items, SFT: w.tables.SFT,
		Rand: w.S.Ctx.Rand, Alert: w.alert(), Found: &w.S.Party.ArtifactsFound,
	}
	if w.indoor != nil {
		e.Indoor = w.indoor.geo
	}
	return e
}

// visitDay is the day number the map loaders get: game seconds / 86400 + 1.
//
// mm8: 0x45f9a9 (Level_Load), 0x47b04f (Outdoor_OnLoad)
func (w *World) visitDay() int32 { return int32(w.S.Party.Time.Seconds()/86400 + 1) }

// respawnDue reports that the map's template comes back and its spawn points run: on the
// first visit (the template's last spawn day is 0) and once the MapStats refill days have
// passed since the last spawn.
//
// mm8: 0x498050 (Blv_Load), 0x47df28 (Odm_Load)
func (w *World) respawnDue() bool {
	dl := w.delta()
	if dl == nil {
		return false
	}
	last := int32(binary.LittleEndian.Uint32(dl.Header[4:]))
	var refill int32
	if row := w.mapStats(); row != nil {
		refill = row.RefillDays
	}
	return uint32(refill) <= uint32(w.visitDay()-last) || last == 0
}

// markSpawned stores the spawn day in the location header.
func (w *World) markSpawned() {
	if dl := w.delta(); dl != nil {
		binary.LittleEndian.PutUint32(dl.Header[4:], uint32(w.visitDay()))
	}
}

// keepFrom carries the revealed map bits and the notes of the map's previous state into
// its reloaded template.
//
// mm8: 0x498050 (Blv_Load: memcpy of the 0x36b revealed bits and the notes)
func (w *World) keepFrom(old *World) {
	dn, do := w.delta(), old.delta()
	if dn == nil || do == nil {
		return
	}
	copy(dn.Revealed, do.Revealed)
	dn.Notes = append(dn.Notes[:0], do.Notes...)
}

// loadMonsters does the monster part of a map load: the temporary objects go, the spawn
// points run when spawn is set (a first visit or a respawn) and the interactive
// decorations get their events, the decorations' variables are numbered, the objects
// settle and the actors get their sprites and the alert/dead fix-ups.
//
// mm8: 0x45f895 (Level_Load), 0x47b04f (Outdoor_OnLoad)
func (w *World) loadMonsters(spawn bool) error {
	dl, mt := w.delta(), w.monsterTables()
	if dl == nil || mt == nil {
		return nil
	}
	w.S.lastDelta[w.kind()] = dl
	w.removeTemporaryObjects()
	row := w.mapStats()
	alert := w.alert() // Level_Load reads it before loading
	if spawn && row != nil {
		e := w.monsterEnv()
		for i := range w.spawns() {
			sp := w.spawns()[i]
			var err error
			if sp.Kind == monsters.SpawnMonsters {
				dl.Actors, err = monsters.Spawn(e, dl.Actors, row, &sp, 0, 0, false)
			} else {
				dl.Objects = monsters.SpawnItems(e, dl.Objects, row, &sp)
			}
			if err != nil {
				return fmt.Errorf("%s: %w", w.name, err)
			}
		}
		w.initInteractiveDecorations()
	}
	w.assignDecorationVars()
	w.settleObjects()
	if w.indoor != nil {
		w.fixActorsIndoor(row != nil, alert)
	} else {
		w.fixActorsOutdoor(row != nil)
	}
	return nil
}

// spawns are the map's spawn points.
func (w *World) spawns() []monsters.SpawnPoint {
	var out []monsters.SpawnPoint
	switch {
	case w.indoor != nil:
		for _, s := range w.indoor.Map.Spawns {
			out = append(out, monsters.SpawnPoint{X: s.Pos[0], Y: s.Pos[1], Z: s.Pos[2], Radius: s.Radius,
				Kind: s.Kind, Index: s.Index, Attrib: s.Attrib, Group: s.Group})
		}
	case w.outdoor != nil:
		for _, s := range w.outdoor.Map.Spawns {
			out = append(out, monsters.SpawnPoint{X: s.Pos.X, Y: s.Pos.Y, Z: s.Pos.Z, Radius: s.Radius,
				Kind: s.Kind, Index: s.Index, Attrib: s.Attrib, Group: s.Group})
		}
	}
	return out
}

// removeTemporaryObjects drops the objects marked temporary and those of a NoPickup kind
// (missiles, effects).
//
// mm8: 0x40933c (Objects_RemoveTemporary)
func (w *World) removeTemporaryObjects() {
	dl, mt := w.delta(), w.monsterTables()
	for i := range dl.Objects {
		o := &dl.Objects[i]
		if o.ObjList == 0 {
			continue
		}
		if o.Attrib&monsters.ObjTemporary != 0 || mt.ObjList[o.ObjList].Flags&tables.ObjNoPickup != 0 {
			removeObject(dl, i)
		}
	}
}

// removeObject frees object slot i (the record stays, marked removed; Object_Create
// reuses the slot).
//
// mm8: 0x42ee3b (Object_Remove; its turn-based bookkeeping is M9's)
func removeObject(dl *delta.Delta, i int) {
	dl.Objects[i].ObjList = 0
	dl.Objects[i].Attrib |= monsters.ObjRemoved
}

// settleObjects gives potions without a power one (rand() % 15 + 5) and applies the
// items' specials; outdoors the objects that are not temporary or NoPickup drop to the
// floor under them.
//
// mm8: 0x45f895 (Level_Load objects loop), 0x4804d4 (Outdoor_SettleObjects)
func (w *World) settleObjects() {
	dl, mt := w.delta(), w.monsterTables()
	t := w.tables.Game.Items
	for i := range dl.Objects {
		o := &dl.Objects[i]
		if o.ObjList == 0 {
			continue
		}
		if w.outdoor != nil && o.Attrib&monsters.ObjTemporary == 0 && mt.ObjList[o.ObjList].Flags&tables.ObjNoPickup == 0 {
			// Outdoor_FloorZ(0, x, y, z, height, ...): the height does not change the floor.
			o.Pos[2], _, _, _ = w.outdoor.geo.FloorZ(o.Pos[0], o.Pos[1], o.Pos[2], false, false)
		}
		it := &o.Item
		if it.Number == 0 {
			continue
		}
		if it.Number != items.EmptyBottle && t.Item(it.Number).EquipType == tables.EquipPotion && it.Bonus == 0 {
			it.Bonus = int32(w.S.Ctx.Rand.Int()%0xf + 5)
		}
		it.ApplySpecial(t)
	}
}

// spriteSet tells Actor_UpdateAnimation whether a frame's first view has a sprite.
type spriteSet struct{ w *World }

func (s spriteSet) Loaded(frame int) bool {
	sft := s.w.tables.SFT
	if frame < 0 || frame >= len(sft.Frames) {
		return false
	}
	fr := &sft.Frames[frame]
	names, _ := fr.ViewNames()
	return s.w.tex != nil && s.w.tex.SpritePal(names[0], fr.Palette) != nil
}

// fixActorsIndoor: actors flagged alert-only show only on an alert map (and only with a
// MapStats row), the others only on a calm one; the rest get their sprites, lose their
// hostility, and die when they have no HP.
//
// mm8: 0x45f895 (Level_Load: the actors loop)
func (w *World) fixActorsIndoor(hasRow bool, alert uint32) {
	dl, mt := w.delta(), w.monsterTables()
	if !hasRow {
		alert = 0
	}
	for i := range dl.Actors {
		a := &dl.Actors[i]
		var disable bool
		if a.Flags&monsters.FlagAlertOnly == 0 {
			disable = alert == 1
		} else {
			disable = !hasRow || alert == 0
		}
		if disable {
			a.Flags |= monsters.FlagDisabled
			a.AIState = monsters.Disabled
			continue
		}
		a.LoadSprites(mt, w.tables.SFT, false)
		a.Info.Hostility = 0
		if a.AIState != monsters.Removed && a.AIState != monsters.Disabled && (a.HP == 0 || a.Info.HP == 0) {
			a.AIState = monsters.Dead
			a.UpdateAnimation(spriteSet{w})
		}
	}
}

// fixActorsOutdoor is the outdoor version: the alert value is only read after the first
// alert-only actor (which is disabled regardless); action times and velocities reset;
// out07's placed monster 7 is marked as the dated appearer.
//
// mm8: 0x48059e (Outdoor_FixActors)
func (w *World) fixActorsOutdoor(hasRow bool) {
	dl, mt := w.delta(), w.monsterTables()
	out07 := strings.EqualFold(w.name, "out07.odm")
	var alert uint32
	for i := range dl.Actors {
		a := &dl.Actors[i]
		flags := a.Flags
		switch {
		case flags&monsters.FlagAlertOnly == 0 && alert == 1, flags&monsters.FlagAlertOnly != 0 && !hasRow:
			a.Flags |= monsters.FlagDisabled
			a.AIState = monsters.Disabled
			continue
		case flags&monsters.FlagAlertOnly != 0 && alert == 0:
			a.Flags |= monsters.FlagDisabled
			a.AIState = monsters.Disabled
			alert = w.alert()
			continue
		}
		if out07 && a.UniqueName == 7 {
			a.Flags = flags | monsters.FlagDatedAppearer
		}
		a.ActionTime, a.ActionLength = 0, 0
		if a.Flags&monsters.FlagDisabled != 0 {
			a.AIState = monsters.Disabled
		}
		if a.AIState != monsters.Removed && a.AIState != monsters.Disabled && (a.HP == 0 || a.Info.HP == 0) {
			a.AIState = monsters.Dead
		}
		a.Vel = [3]int16{}
		a.UpdateAnimation(spriteSet{w})
		a.Info.Hostility = 0
		a.LoadSprites(mt, w.tables.SFT, false)
	}
}

// datedAppearerVisible: out07's placed monster 7 shows only from 9 to 17 h on the 23rd
// day (week 3) of month 5; otherwise it hides and, unless gone, is disabled.
//
// mm8: 0x47b4f1 (Outdoor_DateActorVisible; its quest bit reads 0xba..0xc2 decide nothing)
func (w *World) datedAppearerVisible(a *monsters.Actor) bool {
	if a.AIState == monsters.Dead {
		return true
	}
	c := w.S.Party.Calendar
	if c.Hour < 9 || c.Hour > 0x11 || c.Month != 5 || c.Week != 3 || c.Day != 0x17 {
		a.Flags |= monsters.FlagDisabled
		if !a.Gone(false) {
			a.AIState = monsters.Disabled
		}
		return false
	}
	a.Flags &^= monsters.FlagDisabled
	if !a.Gone(false) && a.AIState == monsters.Disabled {
		a.AIState = monsters.Stand
	}
	return true
}

// actorFrame is the sprite frame an actor shows: walking cycles on the clock (offset per
// actor), everything else plays by its action time; stoned or paralysed actors freeze;
// a resurrecting one plays its dying backwards.
//
// mm8: 0x47c1d2, 0x43db2a
func (w *World) actorFrame(i int, a *monsters.Actor, clk Clock) int {
	sft := w.tables.SFT
	t := int(a.ActionTime)
	if a.Animation == tables.AnimWalk {
		t = clk.Ticks + i*32
	}
	if a.Active(monsters.BuffStoned) || a.Active(monsters.BuffParalysed) {
		t = 0
	}
	anim := int(a.Animation)
	if anim >= len(a.Sprites) {
		anim = 0
	}
	first := int(a.Sprites[anim])
	if a.AIState == monsters.Resurrected {
		return sft.AtReverse(first, t)
	}
	return sft.At(first, t)
}

// actorBillboard sizes actor i's sprite as seen from the camera; ok is false when there
// is nothing to draw.
func (w *World) actorBillboard(i int, a *monsters.Actor, cam *render.Camera, clk Clock) (bb render.Billboard, fr *desc.Frame, ok bool) {
	sft := w.tables.SFT
	fi := w.actorFrame(i, a, clk)
	if fi < 0 || fi >= len(sft.Frames) {
		return bb, nil, false
	}
	fr = &sft.Frames[fi]
	x, y := float64(a.Pos[0]), float64(a.Pos[1])
	ang := int(physics.Atan2(int32(a.Pos[0])-int32(math.Round(cam.X)), int32(a.Pos[1])-int32(math.Round(cam.Y))))
	view := ((int(a.Yaw) + 128) - ang + 1024) >> 8 & 7
	names, mirror := fr.ViewNames()
	tex := w.tex.SpritePal(names[view], fr.Palette)
	if tex == nil {
		return bb, nil, false
	}
	bb = billboard(tex, fr, mirror[view])
	bb.X, bb.Y, bb.Z = x, y, float64(a.Pos[2])
	if b := &a.Buffs[monsters.BuffShrink]; b.Active() && b.Power != 0 {
		k := 1 / float64(uint16(b.Power))
		bb.Left, bb.Right, bb.Top, bb.Bottom = bb.Left*k, bb.Right*k, bb.Top*k, bb.Bottom*k
	}
	if mt := w.monsterTables(); tex.HWL && mt != nil {
		if id := a.ID() - 1; id >= 0 && id < len(mt.MonList) {
			bb.Tint = mt.MonList[id].Tint & 0xffffff
		}
	}
	return bb, fr, true
}

// drawActors queues the visible actors (the removed and disabled ones are not); indoors
// only those in a sector seen this frame. light is the light at a point.
//
// mm8: 0x47c1d2 (Outdoor_AddActorBillboards), 0x43db2a (Indoor_AddActorBillboards)
func (w *World) drawActors(r *render.Renderer, cam *render.Camera, clk Clock, light func(x, y, z, depth float64, sector int) float64) {
	dl := w.delta()
	if dl == nil || w.monsterTables() == nil {
		return
	}
	for i := range dl.Actors {
		a := &dl.Actors[i]
		a.Flags &^= monsters.FlagDrawn
		if a.AIState == monsters.Removed || a.AIState == monsters.Disabled {
			continue
		}
		if w.indoor != nil {
			if s := int(a.Sector); s < 0 || s >= len(w.indoor.secSeen) || !w.indoor.secSeen[s] {
				continue
			}
		} else if a.Flags&monsters.FlagDatedAppearer != 0 && !w.datedAppearerVisible(a) {
			continue
		}
		bb, fr, ok := w.actorBillboard(i, a, cam, clk)
		if !ok {
			continue
		}
		depth := cam.Depth(bb.X, bb.Y, bb.Z)
		if depth < 4 || depth > cam.Far {
			continue
		}
		bb.L = 1
		if fr.Flags&desc.FrameLuminous == 0 {
			bb.L = light(bb.X, bb.Y, bb.Z, depth, int(a.Sector))
		}
		a.Flags |= monsters.FlagDrawn
		r.SetID(actorPID(i))
		r.Billboard(&bb)
		r.SetID(0)
	}
}

// drawObjects queues the ground objects: by their sft frame and time, the view from
// their yaw; a centred frame lowers the sprite by half its height. NoDraw kinds and the
// spell effects (types 500..599, 1000..12999, M9) are left out.
//
// mm8: 0x47bca5 (Outdoor_AddObjectBillboards), 0x43dff2 (Indoor_AddObjectBillboards)
func (w *World) drawObjects(r *render.Renderer, cam *render.Camera, light func(x, y, z, depth float64, sector int) float64) {
	dl, mt := w.delta(), w.monsterTables()
	if dl == nil || mt == nil {
		return
	}
	sft := w.tables.SFT
	for i := range dl.Objects {
		o := &dl.Objects[i]
		if o.ObjList <= 0 || int(o.ObjList) >= len(mt.ObjList) {
			continue
		}
		kind := &mt.ObjList[o.ObjList]
		if kind.Flags&tables.ObjNoDraw != 0 {
			continue
		}
		if t := o.Type; t >= 1000 && t <= 12999 || t >= 500 && t <= 599 || w.indoor != nil && t >= 0x32b && t <= 0x32e {
			continue
		}
		fi := sft.At(int(kind.SFT), int(o.Time))
		if fi < 0 || fi >= len(sft.Frames) {
			continue
		}
		fr := &sft.Frames[fi]
		ang := int(physics.Atan2(o.Pos[0]-int32(math.Round(cam.X)), o.Pos[1]-int32(math.Round(cam.Y))))
		view := ((int(o.Yaw) + 128) - ang + 1024) >> 8 & 7
		names, mirror := fr.ViewNames()
		tex := w.tex.SpritePal(names[view], fr.Palette)
		if tex == nil {
			continue
		}
		bb := billboard(tex, fr, mirror[view])
		z := float64(o.Pos[2])
		if fr.Flags&frameCentered != 0 {
			z -= float64(int32(int64(tex.OrigH)*int64(fr.Scale)>>16) >> 1)
		}
		bb.X, bb.Y, bb.Z = float64(o.Pos[0]), float64(o.Pos[1]), z
		depth := cam.Depth(bb.X, bb.Y, bb.Z)
		if depth < 4 || depth > cam.Far {
			continue
		}
		bb.L = 1
		if fr.Flags&desc.FrameLuminous == 0 {
			bb.L = light(bb.X, bb.Y, bb.Z, depth, int(o.Sector))
		}
		o.Attrib |= monsters.ObjDrawn
		r.SetID(objectPID(i))
		r.Billboard(&bb)
		r.SetID(0)
	}
}

// actorName is what hovering over an actor shows: its placemon.txt name, else its
// monsters.txt name.
//
// mm8: 0x420aab (Mouse_UpdateHover: uniqueName ? placeMon : g_monsters[id].name)
func (w *World) actorName(a *monsters.Actor) string {
	mt := w.monsterTables()
	if a.UniqueName != 0 {
		if int(a.UniqueName) < len(mt.PlaceMon) {
			return mt.PlaceMon[a.UniqueName]
		}
		return ""
	}
	if in := mt.Info(a.ID()); in != nil {
		return in.Name
	}
	return ""
}

// hoverActor and hoverObject are Mouse_UpdateHover's actor and item cases.
//
// mm8: 0x420aab
func (w *World) hoverActor(pid uint32, depth float64) string {
	i := pidIndex(pid)
	acts := w.Actors()
	if depth >= actorNameDepth || i >= len(acts) || w.monsterTables() == nil {
		return ""
	}
	return w.actorName(&acts[i])
}

func (w *World) hoverObject(pid uint32, depth float64) string {
	i := pidIndex(pid)
	objs, mt := w.Objects(), w.monsterTables()
	if i >= len(objs) || mt == nil || mt.ObjList[objs[i].ObjList].Flags&tables.ObjNoPickup != 0 {
		return ""
	}
	m := w.S.Party
	name := objs[i].Item.Name(w.tables.Game.Items, m.Namer(worldNotes{w}))
	if depth < clickDepth && m.MouseItem.Number == 0 {
		return cfmtS(w.Global(txtGetItem), name)
	}
	return name
}

// cfmtS fills the first %s of a C format.
func cfmtS(f, s string) string {
	if i := strings.Index(f, "%s"); i >= 0 {
		return f[:i] + s + f[i+2:]
	}
	return f
}

// worldNotes makes the world the party.Notes item names and gold need.
type worldNotes struct{ w *World }

func (n worldNotes) Global(i int) string             { return n.w.Global(i) }
func (n worldNotes) Status(text string, seconds int) { n.w.Status(text, seconds) }
func (n worldNotes) Stub(key, what string)           { n.w.S.note(key, what) }
func (n worldNotes) AutonoteText(i int) bool         { return n.w.AutonoteText(i) }

// pickUp takes ground object i: gold into the purse, anything else into a pack or onto
// the mouse ("You found %s!"); onlyEmptyMouse is the interact key's rule that an item
// needs a free mouse (it reports false when it is not). The object goes.
//
// mm8: 0x421a65 (Evt_Click), 0x469d53 (Evt_InteractObject)
func (w *World) pickUp(i int, onlyEmptyMouse bool) bool {
	dl := w.delta()
	o := &dl.Objects[i]
	m := w.S.Party
	t := w.tables.Game.Items
	it := o.Item
	if t.Item(it.Number).EquipType == tables.EquipGold {
		m.AddGold(it.Special, worldNotes{w})
	} else {
		if onlyEmptyMouse && m.MouseItem.Number != 0 {
			return false
		}
		w.Status(cfmtS(w.Global(txtFoundItem), t.Item(it.Number).UnidentifiedName), 2)
		if it.Number == 0x1fa {
			m.QBits.Set(0xb8, true)
		}
		if it.Number == 0x1c7 {
			m.QBits.Set(0xb9, true)
		}
		if !m.AddItem(t, it, w.S.Ctx) {
			m.SetMouseItem(t, it)
		}
	}
	removeObject(dl, i)
	return true
}

// clickObject and clickActor are Evt_Click's item and actor cases.
//
// mm8: 0x421a65 (Evt_Click)
func (w *World) clickObject(pid uint32, depth float64) {
	i := pidIndex(pid)
	objs, mt := w.Objects(), w.monsterTables()
	if i >= len(objs) || i >= monsters.MaxObjects || objs[i].ObjList == 0 || mt == nil ||
		mt.ObjList[objs[i].ObjList].Flags&tables.ObjNoPickup != 0 || depth >= clickDepth {
		return
	}
	w.pickUp(i, false)
}

func (w *World) clickActor(pid uint32, depth float64, shift bool) {
	i := pidIndex(pid)
	acts, mt := w.Actors(), w.monsterTables()
	if i >= len(acts) || mt == nil {
		return
	}
	a := &acts[i]
	if a.AIState == monsters.Dead {
		if depth < clickDepth {
			w.S.note("corpse", "Evt_Click: searching a corpse (M8c)")
		}
		return
	}
	if shift {
		w.S.note("cast", "Evt_Click with Shift: the quick spell at an actor (M9)")
		return
	}
	if monsters.Relation(mt, a, nil) != 0 || a.Flags&monsters.FlagHostile != 0 {
		w.S.note("attack", "Evt_Click on a hostile actor: msg 0x17, attack (M9)")
		return
	}
	if depth < clickDepth {
		w.talkToActor(a)
	}
}

// interactActor is Evt_InteractObject's actor case (Space); it stops the scan except at a
// dying or summoned actor.
//
// mm8: 0x469d53 (Evt_InteractObject)
func (w *World) interactActor(i int) bool {
	acts, mt := w.Actors(), w.monsterTables()
	if i >= len(acts) || mt == nil {
		return false
	}
	a := &acts[i]
	switch a.AIState {
	case monsters.Dying, monsters.Summoned:
		return false
	case monsters.Dead:
		w.S.note("corpse", "Evt_InteractObject: searching a corpse (M8c)")
		return true
	}
	if monsters.Relation(mt, a, nil) == 0 && a.Flags&monsters.FlagHostile == 0 {
		w.talkToActor(a)
	}
	return true
}

// interactItem is Evt_InteractObject's item case.
func (w *World) interactItem(i int) bool {
	objs, mt := w.Objects(), w.monsterTables()
	if i >= len(objs) || i >= monsters.MaxObjects || objs[i].ObjList == 0 || mt == nil ||
		mt.ObjList[objs[i].ObjList].Flags&tables.ObjNoPickup != 0 {
		return false
	}
	return w.pickUp(i, true)
}

// talkToActor: an actor able to act turns to the party (M8b) and talks when it is an
// NPC (msg 0xa1: Evt_SpeakNPC with the member's greeting); otherwise its group's news
// shows in the message box and the selected member greets.
//
// mm8: 0x421a65, 0x469d53, 0x42f97e (msg 0xa1: Evt_SpeakNPC(actor, 1))
func (w *World) talkToActor(a *monsters.Actor) {
	if !a.CanAct() {
		return
	}
	m := w.S.Party
	greet := func() {
		id := speechGreetDay
		if h := m.Calendar.Hour; h < 5 || h > 0x15 {
			id = speechGreetNight
		}
		m.Speak(m.Selected-1, id, w.S.Ctx)
	}
	if a.NPC != 0 {
		if m.Selected == 0 {
			return
		}
		w.SpeakNPC(int(a.NPC), true)
		greet()
		return
	}
	n := w.tables.Game.NPC
	if g := int(a.Group); g < 0 || g >= len(n.GroupNews) || n.GroupNews[g] == 0 {
		return
	} else if k := int(n.GroupNews[g]); k < len(n.News) && n.News[k] != "" {
		w.MessageText = n.News[k]
		w.Suspend(evt.Suspension{})
		greet()
	}
}

// debugKeys: F6 kills the nearest actor (M8c brings Actor_Die), F7 freezes the AI.
func (w *World) debugKeys(in *ui.Input) {
	if in.Pressed(ui.KeyF7) {
		w.AIFrozen = !w.AIFrozen
	}
	if !in.Pressed(ui.KeyF6) {
		return
	}
	dl := w.delta()
	if dl == nil {
		return
	}
	best, bestD := -1, int64(math.MaxInt64)
	for i := range dl.Actors {
		a := &dl.Actors[i]
		switch a.AIState {
		case monsters.Dead, monsters.Dying, monsters.Removed, monsters.Disabled:
			continue
		}
		dx, dy, dz := int64(a.Pos[0])-int64(w.group.X), int64(a.Pos[1])-int64(w.group.Y), int64(a.Pos[2])-int64(w.group.Z)
		if d := dx*dx + dy*dy + dz*dz; d < bestD {
			best, bestD = i, d
		}
	}
	if best < 0 {
		return
	}
	a := &dl.Actors[best]
	a.HP = 0
	a.AIState = monsters.Dead
	a.UpdateAnimation(spriteSet{w})
}
