package party

import (
	"libre-enroth/internal/assets/tables"
	"libre-enroth/internal/game/items"
)

// The character screen's item handling: clicks on the backpack and the paper doll, and
// the checks of the item pop-up (re/notes/inventory.md).

// Notes is where the item code puts what it shows besides the party: global.txt
// strings, the timed status line, and notes about what later milestones do.
type Notes interface {
	Global(i int) string
	Status(text string, seconds int)
	// Stub notes something a later milestone does (logged once per key).
	Stub(key, what string)
	// AutonoteText reports autonote n having a text (a new one makes the member speak).
	AutonoteText(n int) bool
}

// ItemAtCell is the item slot (1-based) covering pack cell, 0 for none or outside the
// pack; top is then the item's top-left cell.
//
// mm8: 0x421733 (Player_ItemAtCell)
func (p *Player) ItemAtCell(cell int) (n int32, top int) {
	if cell < 0 || cell >= items.PackSlots {
		return 0, cell
	}
	v := p.Grid[cell]
	if v < 0 {
		top = int(-1 - v)
		return p.Grid[top], top
	}
	return v, cell
}

// topOf is the top-left cell of item slot n in the pack, -1 when it is not there.
func (p *Player) topOf(n int32) int {
	for c := range p.Grid {
		if p.Grid[c] == n {
			return c
		}
	}
	return -1
}

// ClickPack is a left click on member i's backpack. hit is the item slot whose picture
// is under the mouse (the pick buffer; 0 for none) and cell the grid cell under it,
// counted from the grid's corner (6, 32) in 32-pixel steps. With nothing on the
// cursor the item there is picked up; an item on the cursor goes to the cell (or
// anywhere it fits), or swaps with the item there (which stays when the cursor's item
// fits nowhere).
//
// mm8: 0x421764 (CharScreen_ClickInventory; the enchanting spells' item target is M9)
func (m *Members) ClickPack(i int, t *tables.Items, hit int32, cell int, c *Ctx) {
	p := &m.Players[i]
	var n int32
	top := cell
	if hit == 0 {
		// The original lets cell 126 through and finds nothing there.
		if cell < 0 || cell > items.PackSlots {
			return
		}
		n, top = p.ItemAtCell(cell)
	} else {
		if top = p.topOf(hit); top < 0 {
			return
		}
		n = hit
	}
	switch {
	case m.MouseItem.Number == 0:
		if n == 0 {
			return
		}
		m.MouseItem = *p.Item(n)
		p.RemoveItemAt(t, top)
	case n == 0:
		// Each failed try without a free item slot makes the member say so.
		s, full := p.AddItemNumber(t, top, m.MouseItem.Number)
		if s == 0 {
			m.sayFull(full, c)
			if s, full = p.AddItemNumber(t, -1, m.MouseItem.Number); s == 0 {
				m.sayFull(full, c)
				return
			}
		}
		*p.Item(s) = m.MouseItem
		m.MouseItem = items.Item{}
	default:
		old := *p.Item(n)
		p.RemoveItemAt(t, top)
		if p.AddItem(t, top, m.MouseItem) == 0 && p.AddItem(t, -1, m.MouseItem) == 0 {
			w, h := size(t, old.Number)
			p.Grid.Place(top, w, h, n)
			*p.Item(n) = old
			return
		}
		m.MouseItem = old
	}
}

// sayFull is Player_PlaceItemNumber's reaction when the item fit but no item slot was
// free: the selected member says so (0xf).
func (m *Members) sayFull(full bool, c *Ctx) {
	if full && m.Selected >= 1 {
		m.Speak(m.Selected-1, 0xf, c)
	}
}

// HasSkillOrSay reports that member i has the skill (or the skill is "misc", 39 and
// up); without it the status line says so for 2 s (global.txt 0x43).
//
// mm8: 0x492cfc (Player_HasSkillOrSay)
func (p *Player) HasSkillOrSay(skill int, n Notes) bool {
	if skill < NumSkills && p.SkillAt(skill) == 0 {
		n.Status(cSprintf(n.Global(0x43), p.Name), 2)
		return false
	}
	return true
}

// cSprintf fills the first "%s" of a C format.
func cSprintf(f, s string) string {
	for i := 0; i+1 < len(f); i++ {
		if f[i] == '%' && f[i+1] == 's' {
			return f[:i] + s + f[i+2:]
		}
	}
	return f
}

// DollResult is what a click on the paper doll leaves to the screen.
type DollResult int

const (
	DollDone  DollResult = iota
	DollUse              // the cursor's item is not worn: Player_UseItem it
	DollError            // the error sound (0x1b; M11)
)

// putMouse wears the cursor's item in a free item slot as equipment slot slot. It
// reports false when no item slot is free.
func (m *Members) putMouse(p *Player, slot int) bool {
	s := p.FreeSlot()
	if s < 0 {
		return false
	}
	it := m.MouseItem
	it.Slot = uint8(slot + 1)
	p.Items[s] = it
	p.Equip[slot] = int32(s + 1)
	m.MouseItem = items.Item{}
	return true
}

// swapMouse puts the cursor's item in item slot n as equipment slot slot and the item
// that was there on the cursor.
func (m *Members) swapMouse(t *tables.Items, p *Player, n int32, slot int) {
	in := m.MouseItem
	p.Items[n-1].Slot = 0
	m.MouseItem = items.Item{}
	m.SetMouseItem(t, p.Items[n-1])
	in.Slot = uint8(slot + 1)
	p.Items[n-1] = in
	p.Equip[slot] = n
}

// takeMouse puts the cursor's item in a new item slot s as equipment slot slot and the
// item of item slot n on the cursor. Item slot n keeps its copy, unreferenced (the
// original leaks the slot).
func (m *Members) takeMouse(t *tables.Items, p *Player, n int32, s, slot int) {
	in := m.MouseItem
	p.Items[n-1].Slot = 0
	m.MouseItem = items.Item{}
	m.SetMouseItem(t, p.Items[n-1])
	in.Slot = uint8(slot + 1)
	p.Items[s] = in
}

// ClickDoll is a left click on member i's paper doll. hit is the worn item under the
// mouse (its item slot, from the pick buffer; 0 for none), x the mouse's x. With
// nothing on the cursor the item clicked is taken off (a click on no item takes the
// bow; not in a shop, mode 0x17). An item on the cursor is worn by its equip type,
// swapping with what was worn: one-handed weapons and wands go to the main hand, or to
// the off hand right of x 0x22f for an expert with a dagger or a master with a sword;
// a spear with something in the off hand, or a shield, sword or dagger with a spear in
// hand, needs a master of the spear; rings take the first free ring finger (else
// replace the last). Types that are not worn are used instead (DollUse). A member who
// cannot wear it says 0x27.
//
// mm8: 0x468a4b (PaperDoll_Click; the enchanting spells' item target is M9, the wand
// sound M11)
func (m *Members) ClickDoll(i int, hit int32, x int, shop bool, c *Ctx, n Notes) DollResult {
	t := c.Items
	p := &m.Players[i]
	e := m.Env(c)
	main, off := p.Equip[SlotMainHand], p.Equip[SlotOffhand]
	var twoHanded int32
	if main != 0 && p.Item(main).Def(t).EquipType == tables.EquipWeapon2 {
		twoHanded = main
	}
	if m.MouseItem.Number == 0 {
		if hit != 0 {
			it := p.Item(hit)
			if it == nil {
				return DollDone
			}
			m.SetMouseItem(t, *it)
			if it.Slot >= 1 {
				p.Equip[it.Slot-1] = 0
			}
			*it = items.Item{}
			return DollDone
		}
		if shop || p.Equip[SlotBow] == 0 {
			return DollDone
		}
		bow := p.Item(p.Equip[SlotBow])
		m.SetMouseItem(t, *bow)
		*bow = items.Item{}
		p.Equip[SlotBow] = 0
		return DollDone
	}
	d := m.MouseItem.Def(t)
	typ, skill := int(d.EquipType), int(d.Skill)
	spearMaster := func() bool { return Mastery(p.Skill(e, SkillSpear)) >= 3 }
	switch {
	case skill == SkillSpear:
		if off != 0 && !spearMaster() {
			m.Speak(i, 0x27, c)
			return DollDone
		}
	case (skill == SkillShield || skill == SkillSword || skill == SkillDagger) && main != 0 && int(p.Item(main).Def(t).Skill) == SkillSpear:
		if !spearMaster() {
			m.Speak(i, 0x27, c)
			return DollDone
		}
	}
	if !p.CanWear(t, m.MouseItem.Number) {
		m.Speak(i, 0x27, c)
		return DollDone
	}
	switch typ {
	case tables.EquipWeapon, tables.EquipWand:
		if !p.HasSkillOrSay(skill, n) {
			break
		}
		offOK := skill == SkillDagger && p.Skills[SkillDagger]&0xffc0 != 0 ||
			skill == SkillSword && Mastery(p.Skills[SkillSword]) > 2
		if offOK && x > 0x22f && twoHanded == 0 {
			if off != 0 {
				m.swapMouse(t, p, off, SlotOffhand)
				return DollDone
			}
			m.putMouse(p, SlotOffhand)
			return DollDone
		}
		if main != 0 {
			m.swapMouse(t, p, main, SlotMainHand)
			if twoHanded != 0 {
				p.Equip[SlotOffhand] = 0
			}
			return DollDone
		}
		m.putMouse(p, SlotMainHand)
		return DollDone
	case tables.EquipWeapon2:
		if !p.HasSkillOrSay(skill, n) {
			break
		}
		if main != 0 {
			if off != 0 {
				return DollError
			}
			m.swapMouse(t, p, main, SlotMainHand)
			return DollDone
		}
		s := p.FreeSlot()
		if s < 0 {
			return DollDone
		}
		if off != 0 {
			m.takeMouse(t, p, off, s, SlotMainHand)
			p.Equip[SlotOffhand] = 0
			p.Equip[SlotMainHand] = int32(s + 1)
			return DollDone
		}
		m.putMouse(p, SlotMainHand)
		return DollDone
	case tables.EquipMissile, tables.EquipArmor, tables.EquipHelm, tables.EquipBelt, tables.EquipCloak,
		tables.EquipGauntlets, tables.EquipBoots, tables.EquipAmulet:
		if !p.HasSkillOrSay(skill, n) {
			break
		}
		m.equipInSlot(t, p, int(c.Classes.EquipSlot[typ]))
		return DollDone
	case tables.EquipShield:
		if !p.HasSkillOrSay(skill, n) {
			break
		}
		if off != 0 {
			m.swapMouse(t, p, off, SlotOffhand)
			if twoHanded != 0 {
				p.Equip[SlotMainHand] = 0
			}
			return DollDone
		}
		s := p.FreeSlot()
		if s < 0 {
			return DollDone
		}
		if twoHanded == 0 {
			m.putMouse(p, SlotOffhand)
			return DollDone
		}
		m.takeMouse(t, p, main, s, SlotOffhand)
		p.Equip[SlotOffhand] = int32(s + 1)
		p.Equip[SlotMainHand] = 0
		return DollDone
	case tables.EquipRing:
		for s := SlotRing; s < NumSlots; s++ {
			if p.Equip[s] == 0 && m.putMouse(p, s) {
				return DollDone
			}
		}
		if p.Equip[NumSlots-1] != 0 { // the original reads before the items otherwise
			m.swapMouse(t, p, p.Equip[NumSlots-1], NumSlots-1)
		}
		return DollDone
	default:
		return DollUse
	}
	m.Speak(i, 0x27, c)
	return DollDone
}

// equipInSlot wears the cursor's item in equipment slot slot, swapping with the item
// worn there.
//
// mm8: 0x467840 (PaperDoll_EquipInSlot)
func (m *Members) equipInSlot(t *tables.Items, p *Player, slot int) {
	if n := p.Equip[slot]; n != 0 {
		m.swapMouse(t, p, n, slot)
		return
	}
	m.putMouse(p, slot)
}

// BreakItems breaks the member's weapons, armour and other worn kinds of item (ids
// 1..0x97) after a potion explosion: n = 0 every one that is not hardened; else n times
// the k-th of them is checked for hardening and, when it is not hardened, a random one
// of them breaks (hardened or not). Past the list the check reads item slot 0.
//
// mm8: 0x415b98 (Player_BreakItems)
func (p *Player) BreakItems(n int, rng *Rand) {
	var list [items.Slots]int
	count := 0
	for s := range p.Items {
		if id := p.Items[s].Number; id > 0 && id < 0x98 {
			list[count] = s
			count++
		}
	}
	if count == 0 {
		return
	}
	if n == 0 {
		for _, s := range list[:count] {
			if p.Items[s].Flags&items.FlagHardened == 0 {
				p.Items[s].Flags |= items.FlagBroken
			}
		}
		return
	}
	for k := 0; k < n; k++ {
		if p.Items[list[k]].Flags&items.FlagHardened == 0 {
			p.Items[list[rng.Int()%count]].Flags |= items.FlagBroken
		}
	}
}

// CanIdentify reports that the member's ID Item skill (with item bonuses) identifies
// the item: mastery multiplier × level reaches its difficulty, or grandmaster.
//
// mm8: 0x491ea1 (Player_CanIdentify, of the selected member)
func (p *Player) CanIdentify(e *Env, it *items.Item) bool {
	s := p.Skill(e, SkillIDItem)
	return int(it.Def(e.Items).IDRepair) <= MasteryMult(s)*int(s&SkillLevel) || Mastery(s) > 3
}

// CanRepair is CanIdentify for the Repair skill.
//
// mm8: 0x491f09 (Player_CanRepair)
func (p *Player) CanRepair(e *Env, it *items.Item) bool {
	s := p.Skill(e, SkillRepair)
	return int(it.Def(e.Items).IDRepair) <= MasteryMult(s)*int(s&SkillLevel) || Mastery(s) > 3
}

// SpendSkillPoint raises member i's skill by a level for level + 1 skill points (the
// new level), which the member says (0xe). It returns the global.txt message when it
// cannot: 0x1e8 too few points, 0x1e7 the skill is at 60.
//
// mm8: 0x42f877 (msg 0x79; the sound 0xcd is M11)
func (m *Members) SpendSkillPoint(i, skill int, c *Ctx) (refusal int) {
	p := &m.Players[i]
	v := p.Skills[skill]
	if int(p.SkillPoints) < int(v&SkillLevel)+1 {
		return 0x1e8
	}
	if v&SkillLevel > 0x3b {
		return 0x1e7
	}
	p.Skills[skill] = v + 1
	p.SkillPoints -= int32(p.Skills[skill] & SkillLevel)
	m.Speak(i, 0xe, c)
	return 0
}

// AddGold gives the party gold found, which the status line announces ("You found %lu
// gold!", global.txt 0x1d3, for 2 s).
//
// mm8: 0x420921 (Party_AddGold, mode 0; the sound 0x85 is M11's)
func (m *Members) AddGold(n int32, notes Notes) {
	m.Gold += n
	notes.Status(cLu(notes.Global(0x1d3), uint32(n)), 2)
}

// cLu fills the first "%lu" (or "%d") of a C format.
func cLu(f string, v uint32) string {
	for _, verb := range []string{"%lu", "%d", "%u"} {
		for i := 0; i+len(verb) <= len(f); i++ {
			if f[i:i+len(verb)] == verb {
				return f[:i] + itoa(int(v)) + f[i+len(verb):]
			}
		}
	}
	return f
}
