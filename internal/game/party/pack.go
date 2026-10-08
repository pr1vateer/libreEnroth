package party

import (
	"libre-enroth/internal/assets/tables"
	"libre-enroth/internal/game/items"
)

// FreeSlot is the first empty item slot of the pack (0-based), -1 when all are taken.
//
// mm8: 0x492bef
func (p *Player) FreeSlot() int {
	for i := 0; i < items.PackSlots; i++ {
		if p.Items[i].Number == 0 {
			return i
		}
	}
	return -1
}

// size is an item's size in pack cells.
func size(t *tables.Items, id int32) (w, h int) {
	d := t.Item(id)
	return d.W, d.H
}

// AddItem puts it in the pack at cell (-1: the first cell it fits at, down the columns
// from the left), first giving an item of special material its fixed bonus. Returns
// the 1-based item slot, 0 when it does not fit or no slot is free.
//
// mm8: 0x492e59, 0x492ecf
func (p *Player) AddItem(t *tables.Items, cell int, it items.Item) int32 {
	it.ApplySpecial(t)
	w, h := size(t, it.Number)
	if cell == -1 {
		if cell = p.Grid.Free(w, h); cell < 0 {
			return 0
		}
	} else if !p.Grid.Fits(cell, w, h) {
		return 0
	}
	n := p.FreeSlot()
	if n < 0 {
		return 0
	}
	p.Grid.Place(cell, w, h, int32(n+1))
	p.Items[n] = it
	return int32(n + 1)
}

// AddItemNumber puts a bare item id in the pack at cell (-1: the first that fits).
// Returns the 1-based slot (whose other fields the caller fills), 0 when it does not
// fit; full reports that it fit but no item slot was free (the original has the
// selected member say so, 0xf).
//
// mm8: 0x492ddb, 0x492c09
func (p *Player) AddItemNumber(t *tables.Items, cell int, id int32) (n int32, full bool) {
	w, h := size(t, id)
	if cell == -1 {
		if cell = p.Grid.Free(w, h); cell < 0 {
			return 0, false
		}
	} else if !p.Grid.Fits(cell, w, h) {
		return 0, false // the error sound 0x1b (M11)
	}
	s := p.FreeSlot()
	if s < 0 {
		return 0, true
	}
	p.Grid.Place(cell, w, h, int32(s+1))
	p.Items[s] = items.Item{Number: id}
	return int32(s + 1), false
}

// RemoveItemAt takes the item whose top-left cell is cell out of the pack.
//
// mm8: 0x49305c
func (p *Player) RemoveItemAt(t *tables.Items, cell int) {
	n := p.Grid[cell]
	it := p.Item(n)
	if it == nil {
		return
	}
	w, h := size(t, it.Number)
	*it = items.Item{}
	p.Grid.Clear(cell, w, h)
}

// CanWear reports whether the player's race and class allow item id: minotaurs wear no
// helms or boots, dragons only rings, amulets and the like (nothing of types 0..9 or
// wands), and some artifacts are for one class only.
//
// mm8: 0x49326f
func (p *Player) CanWear(t *tables.Items, id int32) bool {
	typ := t.Item(id).EquipType
	switch f := p.Face + 1; {
	case f < 0x15:
	case f < 0x17:
		if typ == tables.EquipHelm || typ == tables.EquipBoots {
			return false
		}
	case f < 0x19 || f > 0x1a:
	default:
		if typ <= tables.EquipBoots || typ == tables.EquipWand {
			return false
		}
	}
	c := p.Class
	is := func(a, b int) bool { return c == a || c == b }
	switch id {
	case 0x204:
		return is(2, 3) // clerics
	case 0x1f8:
		return is(8, 9) // minotaurs
	case 0x1fc:
		return is(12, 13) // vampires
	case 0x202, 0x214:
		return is(10, 11) // dark elves
	case 0x203:
		return is(4, 5) // knights
	case 0x209:
		return c == 1 // the lich
	case 0x211:
		return is(0, 1) // necromancers
	}
	return true
}

// EquipItem wears it in the slot of its equip type (a ring in the first free of the
// six ring slots), when that slot is free, an item slot is free and the player may
// wear it. Reports success.
//
// mm8: 0x492d41
func (p *Player) EquipItem(t *tables.Items, cls *tables.Classes, it items.Item) bool {
	n := p.FreeSlot()
	if n < 0 || !p.CanWear(t, it.Number) {
		return false
	}
	typ := t.Item(it.Number).EquipType
	if typ >= tables.EquipReagent {
		return false
	}
	slot := int(cls.EquipSlot[typ])
	if typ == tables.EquipRing {
		slot = -1
		for s := SlotRing; s < NumSlots; s++ {
			if p.Equip[s] == 0 {
				slot = s
				break
			}
		}
		if slot < 0 {
			return false
		}
	} else if p.Equip[slot] != 0 {
		return false
	}
	it.Slot = uint8(slot + 1)
	p.Items[n] = it
	p.Equip[slot] = int32(n + 1)
	return true
}

// AddItem gives the party an item: the selected member first, then the others in turn
// from the next slot; the one who takes it says so (0x3c). Reports whether anyone had
// room.
//
// mm8: 0x48dd30 (Party_AddItem; the pick-up sound 0x85 is M11's)
func (m *Members) AddItem(t *tables.Items, it items.Item, c *Ctx) bool {
	if t.Item(it.Number).IDRepair == 0 {
		it.Flags |= items.FlagIdentified
	}
	order := make([]int, len(m.Players))
	for i := range order {
		order[i] = i
	}
	if m.Selected != 0 {
		for k := range order {
			order[k] = (m.Selected - 1 + k) % len(m.Players)
		}
	}
	for _, i := range order {
		p := &m.Players[i]
		if n, _ := p.AddItemNumber(t, -1, it.Number); n != 0 {
			*p.Item(n) = it
			m.Speak(i, 0x3c, c)
			return true
		}
	}
	return false
}

// DropMouseItem puts the item on the cursor into a pack: the selected member's, else
// the first member's with room (every member's when none is selected). With no room
// anywhere the original throws it in front of the party (an object, M8); here it stays
// on the cursor. Reports whether the cursor is free.
//
// mm8: 0x4211c8
func (m *Members) DropMouseItem(t *tables.Items) bool {
	if m.MouseItem.Number == 0 {
		return true
	}
	try := func(i int) bool {
		p := &m.Players[i]
		n, _ := p.AddItemNumber(t, -1, m.MouseItem.Number)
		if n == 0 {
			return false
		}
		*p.Item(n) = m.MouseItem
		m.MouseItem = items.Item{}
		return true
	}
	if s := m.Selected; s >= 1 && s <= len(m.Players) && try(s-1) {
		return true
	}
	for i := range m.Players {
		if try(i) {
			return true
		}
	}
	return false
}

// SetMouseItem puts it on the cursor, the item there before going to a pack first (when
// no pack has room the original throws that one on the ground, M8; here it is lost).
//
// mm8: 0x493779 (Party_SetMouseItem)
func (m *Members) SetMouseItem(t *tables.Items, it items.Item) {
	m.DropMouseItem(t)
	m.MouseItem = it
}

// HasItem reports member i holding item id in any of its slots (the worn ones too),
// or id on the mouse: Compare of the item variable.
//
// mm8: 0x44a0fe.. (Evt_Compare case 0x11)
func (m *Members) HasItem(i int, id int32) bool {
	p := &m.Players[i]
	for k := range p.Items {
		if uint32(p.Items[k].Number) == uint32(id) {
			return true
		}
	}
	return uint32(m.MouseItem.Number) == uint32(id)
}

// GiveItem puts item id on the mouse as Add and Set of the item variable do:
// identified, a handed-out artifact recorded; charge (Add) also charges a wand.
//
// mm8: 0x449726 (Evt_Add case 0x11), 0x448d4b (Evt_Set case 0x11)
func (m *Members) GiveItem(id int32, charge bool, c *Ctx) {
	t := c.Items
	if t == nil {
		return
	}
	it := items.Item{Number: id, Flags: items.FlagIdentified}
	if uint32(id) >= items.FirstArtifact && uint32(id) < items.FirstArtifact+items.NumArtifacts {
		m.ArtifactsFound[id-items.FirstArtifact] = true
	}
	if charge && it.Number >= items.FirstWand && it.Number <= items.LastWand {
		it.Charges = max(int32(c.Rand.Int()%6+1+int(t.Item(it.Number).Mod2)), 1)
		it.MaxCharges = uint8(it.Charges)
	}
	m.SetMouseItem(t, it)
}
