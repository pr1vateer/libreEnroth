package world

import (
	"libre-enroth/internal/game/items"
	"libre-enroth/internal/game/monsters"
	"libre-enroth/internal/game/physics"
	"libre-enroth/internal/game/ui"
)

// Ground objects (re/notes/monsters.md, "Objects"): items the party throws or drops,
// that monsters drop as they die, or that events summon are SpriteObjects in the map's
// delta; they move each frame before the party.

// Throwing an item (Party_DropMouseItem 0x4211c8, Evt_Click 0x421a65).
const (
	throwPitch = 0xb8
	throwSpeed = 200
)

// throwItem throws an item from the party's eyes, yawOffset off its heading. It is a
// temporary object: the next load of the map removes it.
//
// mm8: 0x4211c8 (Party_DropMouseItem: no pack has room), 0x421a65 (Evt_Click)
func (w *World) throwItem(it items.Item, yawOffset int32) {
	dl, mt := w.delta(), w.monsterTables()
	if dl == nil || mt == nil {
		return
	}
	p := w.group
	e := w.monsterEnv()
	o := e.ItemObject(it, p.X, p.Y, p.Z+p.EyeLevel)
	o.Caster = monsters.KindParty
	o.Attrib = monsters.ObjTemporary
	dl.Objects, _ = monsters.Launch(dl.Objects, o, p.Dir+yawOffset, throwPitch, throwSpeed)
}

// throwMouseItem is a click in the view that nothing took while an item is on the
// mouse: the item is thrown towards the click.
//
// mm8: 0x421a65 (Evt_Click: 0x469aba turns the mouse's x into a yaw)
func (w *World) throwMouseItem(x int) {
	m := w.S.Party
	if m.MouseItem.Number == 0 {
		return
	}
	focal := int32(FocalOutdoor)
	if w.indoor != nil {
		f := FocalIndoor
		focal = int32(f) // ftol
	}
	v := ui.Viewport
	cx := int32(v.Min.X+v.Max.X-1) >> 1
	it := m.MouseItem
	m.MouseItem = items.Item{}
	w.throwItem(it, physics.Atan2(int32(x)-cx, focal)-physics.QuarterTurn)
}

// DropItem implements evt.Host: SummonItem's objects.
//
// mm8: 0x4446bd (case 0x22), 0x42ecee (Object_DropItem)
func (w *World) DropItem(it items.Item, x, y, z, speed int32, count int, randomYaw bool) {
	dl, mt := w.delta(), w.monsterTables()
	if dl == nil || mt == nil {
		return
	}
	sprite := w.tables.Game.Items.Item(it.Number).Sprite
	dl.Objects = w.monsterEnv().DropItem(dl.Objects, &it, sprite, x, y, z, speed, count, randomYaw, 0)
}

// moveObjects moves the ground objects (before the party moves).
//
// mm8: 0x46bafa, 0x46bb13 (World_TickIndoor/Outdoor: Objects_Update 0x472b60 first)
func (w *World) moveObjects(ticks int32) {
	if b := w.brain(); b != nil {
		b.MoveObjects(ticks)
	}
}
