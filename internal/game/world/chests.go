package world

import (
	"libre-enroth/internal/assets/tables"
	"libre-enroth/internal/game/items"
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
				c.Rand.Int() // which of the four trap explosions (objects 0x32b..0x32e, M9)
				w.S.note("chest trap", "a chest trap's explosion and its damage are M9's")
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

// ChestGrid is chest type t's grid (for tools and tests).
func (w *World) ChestGrid(t int) tables.ChestGrid { return w.tables.Chests.GridOf(t) }

// GameTables are the game's text tables (the character screen's awards).
func (w *World) GameTables() *tables.All { return w.tables.Game }
