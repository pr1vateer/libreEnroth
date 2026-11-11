package world

import (
	"libre-enroth/internal/assets/tables"
	"libre-enroth/internal/game/items"
	"libre-enroth/internal/game/monsters"
	"libre-enroth/internal/game/party"
	"libre-enroth/internal/game/ui"
)

// MapStats columns the chests read.
const (
	mapStatTrap     = 9  // +0x2d: the chests' trap level
	mapStatTreasure = 11 // +0x2f: the random items' treasure level
)

// chests are the map's chest records (from its .dlv/.ddm).
func (w *World) chests() []items.Chest {
	if dl := w.delta(); dl != nil {
		return dl.Chests
	}
	return nil
}

// generateChests makes the chests' random items, as every map entry does.
//
// mm8: 0x45ff25 -> 0x44ec9d (Chests_GenerateItems)
func (w *World) generateChests() {
	c, ct := w.S.Ctx, w.tables.Chests
	if c == nil || c.Items == nil || ct == nil || c.Rand == nil {
		return
	}
	cs := w.chests()
	if len(cs) == 0 {
		return
	}
	items.GenerateChests(cs, c.Items, ct, w.tables.MapStat(w.name, mapStatTreasure), c.Rand, &w.S.Party.ArtifactsFound)
}

// OpenChest implements evt.Host: the chest's items are placed the first time, a trapped
// chest goes off unless the selected member disarms it (Disarm Trap at least twice the
// map's trap level; the member says 4) — a trap that goes off ends the event (the
// explosion and its damage are M9's) — and the chest screen opens.
//
// mm8: 0x420093 (Chest_Open)
func (w *World) OpenChest(id int) bool {
	cs := w.chests()
	c, ct := w.S.Ctx, w.tables.Chests
	if id < 0 || id >= len(cs) || c == nil || c.Items == nil || ct == nil {
		return false
	}
	ch := &cs[id]
	g := ct.GridOf(int(ch.Type))
	if ch.Flags&items.ChestPlaced == 0 {
		ch.PlaceItems(c.Items, g, c.Rand)
	}
	m := w.S.Party
	if sel := m.Selected; sel >= 1 && sel <= len(m.Players) {
		if ch.Flags&items.ChestTrapped != 0 && w.tables.MapIndex(w.name) != 0 {
			p := &m.Players[sel-1]
			if p.DisarmTrap(m.Env(c)) < 2*w.tables.MapStat(w.name, mapStatTrap) {
				w.chestTrap(int16(0x32b + c.Rand.Int()%4))
				ch.Flags &^= items.ChestTrapped
				return false
			}
			ch.Flags &^= items.ChestTrapped
			m.Speak(sel-1, 4, c)
		}
	}
	w.openChest = &ui.ChestView{Chest: ch, Grid: g, Picture: ct.Picture(int(ch.Type))}
	return true
}

// OpenedChest implements ui.ChestOpener: the chest an event opened, nil for none.
func (w *World) OpenedChest() *ui.ChestView { return w.openChest }

// CloseChest implements ui.ChestOpener.
func (w *World) CloseChest() { w.openChest = nil }

// chestTrap spawns one of objects 0x32b..0x32e at the party and, when the party is
// within the approximate distance 0x301, hurts each member: 5 plus the map's trap
// level rolls of 1+rand()%20. A member who can act and whose perception roll is at
// least 0x15 says 6 and is spared. The sound is 8 (M11).
//
// mm8: 0x420093 (the spawn), 0x43726d (Chest_TrapDamage)
func (w *World) chestTrap(kind int16) {
	g := w.group
	if g == nil || w.S == nil || w.S.Party == nil {
		return
	}
	w.spawnBlast(kind, g.X, g.Y, g.Z+g.EyeLevel)
	row := w.tables.MapIndex(w.name)
	trap := 0
	if mt := w.monsterTables(); mt != nil && row >= 0 && row < len(mt.MapStats) {
		trap = int(mt.MapStats[row].Trap)
	}
	w.trapDamage(kind, g.X, g.Y, g.Z+g.EyeLevel, trap)
}

// spawnBlast puts a trap or potion-mix object into the world.
func (w *World) spawnBlast(kind int16, x, y, z int32) {
	dl, mt := w.delta(), w.monsterTables()
	if dl == nil || mt == nil {
		return
	}
	o := monsters.Object{Type: kind, ObjList: int16(mt.ObjListOf(kind)), Pos: [3]int32{x, y, z}, Glow: 1}
	if w.indoor != nil && w.indoor.geo != nil {
		o.Sector = int16(w.indoor.geo.SectorAt(x, y, z))
	}
	dl.Objects, _ = monsters.Launch(dl.Objects, o, 0, 0, 0)
}

// trapDamage is Chest_TrapDamage for an explosion at x, y, z.
func (w *World) trapDamage(kind int16, x, y, z int32, trap int) {
	m, c := w.S.Party, w.S.Ctx
	g := w.group
	if m == nil || c == nil || c.Rand == nil || g == nil {
		return
	}
	dx := g.X - x
	dy := g.Y - y
	dz := g.Z + g.EyeLevel - z
	if dx < 0 {
		dx = -dx
	}
	if dy < 0 {
		dy = -dy
	}
	if dz < 0 {
		dz = -dz
	}
	a, b, d := dx, dy, dz
	if a < b {
		a, b = b, a
	}
	if a < d {
		a, d = d, a
	}
	if b < d {
		b, d = d, b
	}
	if a+d>>2+(b*11)>>5 >= 0x301 {
		return
	}
	amount := 5
	for range trap {
		amount += 1 + c.Rand.Int()%0x14
	}
	typ := party.DamageBody
	switch kind {
	case 0x32b:
		typ = party.DamageFire
	case 0x32c:
		typ = party.DamageAir
	case 0x32d:
		typ = party.DamageWater
	}
	e := m.Env(c)
	for i := range m.Players {
		p := &m.Players[i]
		if p.CanAct() && c.Rand.Int()%(p.Perception(e)+0x14) >= 0x15 {
			m.Speak(i, 6, c)
			continue
		}
		m.ReceiveDamage(i, int32(amount), typ, c)
	}
}

// PotionBlast is a mix that blew up: the same four objects as a chest trap.
//
// mm8: 0x415c6d (the explosion's object)
func (w *World) PotionBlast(kind int) { w.spawnBlast(int16(kind), w.group.X, w.group.Y, w.group.Z) }

// ChestGrid is chest type t's grid (for tools and tests).
func (w *World) ChestGrid(t int) tables.ChestGrid { return w.tables.Chests.GridOf(t) }

// GameTables are the game's text tables (the character screen's awards).
func (w *World) GameTables() *tables.All { return w.tables.Game }
