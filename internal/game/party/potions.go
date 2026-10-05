package party

import (
	"libre-enroth/internal/assets/tables"
	"libre-enroth/internal/game/items"
)

// Using items (dropped on the paper doll or right-clicked on a portrait) and mixing
// potions (an item on the cursor right-clicked on one in the pack); re/notes/inventory.md
// #potions.

// Item ids the potion code tests.
const (
	ItemBottle      = 0xdc // the empty potion bottle
	ItemCatalyst    = 0xdd
	ItemCureWounds  = 0xde
	ItemMagicPotion = 0xdf
	ItemCureWeak    = 0xe0
	ItemRecharge    = 0xe9
	ItemHarden      = 0xec
	ItemFlaming     = 0xf6 // .. 0xfa: the weapon enchanting potions
	ItemSlaying     = 0x107
	ItemRejuvenate  = 0x10f
	firstReagent    = 200
	lastReagent     = 0xdb
	firstPotion     = ItemCatalyst
	lastPotion      = ItemRejuvenate
)

// namer gives item names the members' names and global.txt.
type namer struct {
	m *Members
	n Notes
}

func (x namer) Global(i int) string { return x.n.Global(i) }
func (x namer) MemberName(i int) string {
	if i < 0 || i >= len(x.m.Players) {
		return ""
	}
	return x.m.Players[i].Name
}

// Namer is an items.Namer over the members and n's global.txt.
func (m *Members) Namer(n Notes) items.Namer { return namer{m, n} }

// UseResult is what using the cursor's item leaves to the screen.
type UseResult struct {
	// Close: the character screen closes (a potion or reagent was taken).
	Close bool
	// Error: nothing happened; the error sound (0x1b, M11).
	Error bool
	// Scroll is a message scroll to show (its items.txt id), 0 for none.
	Scroll int32
}

// Potion buffs: the player buff each buff potion casts and whether its power is the
// potion's × 3 (else 5 with rank 3), as Player_UseItem calls SpellBuff_Apply. The
// durations' x87 factors and the buffs themselves are M9's.
var potionBuff = map[int32]struct {
	buff  int
	power bool
}{
	0xe4: {7, false}, 0xe5: {8, false}, 0xe6: {1, false}, 0xe7: {11, true}, 0xe8: {13, true},
	0xea: {14, false}, 0xeb: {23, false}, 0xf0: {19, true}, 0xf1: {17, true}, 0xf2: {20, true},
	0xf3: {16, true}, 0xf4: {21, true}, 0xf5: {15, true}, 0xff: {18, true}, 0x100: {5, true},
	0x101: {0, true}, 0x102: {22, true}, 0x103: {3, true}, 0x104: {9, true}, 0x105: {2, true},
}

// purePotions are the one-time +50 potions: the stat each raises, by item id from
// 0x108 (Pure Luck, Speed, Intellect, Endurance, Personality, Accuracy, Might).
var purePotions = [7]Stat{StatLuck, StatSpeed, StatIntellect, StatEndurance, StatPersonality, StatAccuracy, StatMight}

// UseItem is member i using the item on the cursor: reagents (three can be eaten),
// potions (healing, mana, cures, the one-time stat potions and Rejuvenation; the buff
// potions are M9's), spell books (learnt when the school's skill ranks high enough)
// and message scrolls; spell scrolls and the special items are M9's. A used potion or
// reagent costs the member a recovery of 213 ticks (100 in turn-based mode) and closes
// the character screen; a learnt book or read scroll does not.
//
// mm8: 0x467aa9 (Player_UseItem; the sounds are M11's, the item's sparkle on the
// portrait 0x4a906d M9's)
func (m *Members) UseItem(i int, c *Ctx, n Notes) UseResult {
	p := &m.Players[i]
	t := c.Items
	it := m.MouseItem
	d := it.Def(t)
	e := m.Env(c)
	cantUse := func() UseResult {
		n.Status(cSprintf(n.Global(0x24), it.Name(t, m.Namer(n))), 2)
		return UseResult{Error: true}
	}
	cantAct := func() UseResult {
		n.Status(cSprintf(n.Global(0x17e), n.Global(conditionNameGlobal[p.MainCondition()])), 2)
		return UseResult{Error: true}
	}
	speak := true
	switch d.EquipType {
	case tables.EquipReagent:
		switch it.Number {
		case firstReagent:
			p.Heal(e, 2)
		case 0xcd:
			p.SP = min(p.SP+2, p.MaxSP(e))
		case 0xd2:
			m.SetCondition(i, CondPoison1, true, c)
			speak = false
		default:
			return cantUse()
		}
	case tables.EquipPotion:
		var ok bool
		if speak, ok = m.drink(i, it, c, n); !ok {
			return cantUse()
		}
	case tables.EquipSpellScroll:
		if !p.CanAct() {
			return cantAct()
		}
		n.Stub("scroll", "reading a spell scroll casts its spell (M9)")
		return UseResult{Error: true}
	case tables.EquipBook:
		return m.readBook(i, c, n)
	case tables.EquipMessageScroll:
		if !p.CanAct() {
			return cantAct()
		}
		m.Speak(i, 0x25, c)
		if it.Number > 699 && it.Number < 0x30f {
			return UseResult{Scroll: it.Number}
		}
		return UseResult{}
	default:
		switch it.Number {
		case 0x27a, 0x28d, 0x28f, 0x290, 0x291:
			n.Stub("special item", "the special items (horn, lamp, ...) are M9's")
			return UseResult{Error: true}
		}
		return cantUse()
	}
	if speak {
		m.Speak(i, 0x24, c)
	}
	m.MouseItem = items.Item{}
	if m.TurnBased {
		m.SetRecovery(i, 100) // and the turn-based queue's 100 (M9)
	} else {
		rec := float64(recMod1) * useRecovery
		m.SetRecovery(i, int(rec))
	}
	return UseResult{Close: true}
}

// useRecovery is the recovery after taking a potion, × the ini's recmod1 (0x4ea548).
const useRecovery = 213.33333333333334

// conditionNameGlobal are the global.txt names of the conditions (g_conditionNames,
// Txt_LoadGlobal 0x45181f).
var conditionNameGlobal = [...]int{0x34, 0xf1, 0xe, 4, 0x45, 0x75, 0xa6, 0x41, 0xa6, 0x41, 0xa6, 0x41,
	0xa2, 0xe7, 0x3a, 0xdc, 0x4c, 0x259, 0x62}

// drink is a potion's effect. ok false: it cannot be drunk (the bottle, the potions
// for items); speak false: the member says nothing (the catalyst, Haste while weak).
//
// mm8: 0x467aa9 (the potion switch)
func (m *Members) drink(i int, it items.Item, c *Ctx, n Notes) (speak, ok bool) {
	p := &m.Players[i]
	e := m.Env(c)
	clear := func(cs ...Condition) {
		for _, cd := range cs {
			p.Conditions[cd] = 0
		}
	}
	switch id := it.Number; {
	case id == ItemCatalyst:
		m.SetCondition(i, CondPoison1, true, c)
		return false, true
	case id == ItemCureWounds:
		p.Heal(e, it.Bonus+10)
	case id == ItemMagicPotion:
		p.SP = min(p.SP+it.Bonus+10, p.MaxSP(e))
	case id == ItemCureWeak:
		clear(CondWeak)
	case id == 0xe1:
		clear(CondDisease3, CondDisease2, CondDisease1)
	case id == 0xe2:
		clear(CondPoison3, CondPoison2, CondPoison1)
	case id == 0xe3:
		clear(CondAsleep)
	case id == 0xe4 && p.Conditions[CondWeak] != 0:
		return false, true // Haste does nothing while weak
	case id == 0xed:
		clear(CondAfraid)
	case id == 0xee:
		clear(CondCursed)
	case id == 0xef:
		clear(CondInsane)
	case id == 0xfb:
		clear(CondParalyzed)
	case id == 0xfc:
		// Divine Restoration: every condition but dead, stoned and eradicated.
		keep := [3]int64{p.Conditions[CondDead], p.Conditions[CondStoned], p.Conditions[CondEradicated]}
		p.Conditions = [numConditions]int64{}
		p.Conditions[CondDead], p.Conditions[CondStoned], p.Conditions[CondEradicated] = keep[0], keep[1], keep[2]
	case id == 0xfd:
		p.Heal(e, it.Bonus*5)
	case id == 0xfe:
		p.SP = min(p.SP+it.Bonus*5, p.MaxSP(e))
	case id == 0x106:
		clear(CondStoned)
	case id >= 0x108 && id < 0x108+7:
		k := id - 0x108
		if !p.PurePotions[k] {
			*p.BasePtr(purePotions[k]) += 50
			p.PurePotions[k] = true
		}
	case id == ItemRejuvenate:
		p.AgeBonus = 0
	default:
		if _, buff := potionBuff[id]; buff {
			n.Stub("potion buff", "the buff potions' spell buffs are M9's")
			return true, true
		}
		return false, false
	}
	return true, true
}

// readBook learns the spell of the book on the cursor: the school's skill must reach
// the spell (normal the first 4, expert 7, master 10, grandmaster all 11).
//
// mm8: 0x467aa9 (type 0x10)
func (m *Members) readBook(i int, c *Ctx, n Notes) UseResult {
	p := &m.Players[i]
	t := c.Items
	it := m.MouseItem
	k := int(it.Number) - 400
	if k < 0 || k >= NumSpells {
		return UseResult{Error: true}
	}
	if p.Spells[k] {
		n.Status(cSprintf(n.Global(0x17c), it.Name(t, m.Namer(n))), 2)
		return UseResult{Error: true}
	}
	if !p.CanAct() {
		n.Status(cSprintf(n.Global(0x17e), n.Global(conditionNameGlobal[p.MainCondition()])), 2)
		return UseResult{Error: true}
	}
	skill := p.Skills[SkillFire+k/spellsPerSchool]
	limit := i // the original leaves the member number there for a skill of no rank
	switch Mastery(skill) {
	case 1:
		limit = 4
	case 2:
		limit = 7
	case 3:
		limit = 10
	case 4:
		limit = 11
	}
	if limit < k%spellsPerSchool+1 || skill == 0 {
		n.Status(cSprintf(n.Global(0x17d), it.Name(t, m.Namer(n))), 2)
		m.Speak(i, 0x14, c)
		return UseResult{}
	}
	p.Spells[k] = true
	m.Speak(i, 0x15, c)
	m.MouseItem = items.Item{}
	return UseResult{}
}

// Mix is the result of a right click with an item on the cursor on one in the pack.
type Mix int

const (
	MixNone    Mix = iota // nothing to mix: the item's pop-up shows
	MixDone               // mixed (or the potion worked on the item)
	MixExplode            // an explosion: the character screen closes
	MixFailed             // the potion does nothing to that item
	MixError              // the error sound
)

// MixPotion is member i right-clicking with the cursor's item on item slot slot of the
// pack: a reagent in an empty bottle makes the basic potion of its colour (power: the
// reagent's dice count + the alchemy level); two potions mix by potion.txt into a new
// one (power: their mean) and leave a bottle, give the catalyst's power or blow up
// (explosion 1..4: damage 10..20, 30..100 and one broken item, 50..250 and five, or
// eradication and everything broken) when the alchemy rank is too low for the result;
// the potions for items recharge a wand, harden or temporarily enchant a weapon. The
// first success makes the member speak (0x10), the first explosion 0x11.
//
// mm8: 0x415c6d (Inventory_RightClick; the explosion's object and sound are M9/M11's)
func (m *Members) MixPotion(i int, slot int32, first bool, c *Ctx, n Notes) Mix {
	p := &m.Players[i]
	t := c.Items
	e := m.Env(c)
	target := p.Item(slot)
	mouse := m.MouseItem
	if target == nil || mouse.Number == ItemBottle {
		return MixNone
	}
	alch := p.Skill(e, SkillAlchemy)
	level, mastery := int(alch&SkillLevel), Mastery(alch)
	switch {
	case mouse.Number >= firstReagent && mouse.Number <= lastReagent && target.Number == ItemBottle:
		target.Bonus = int32(t.Item(mouse.Number).DiceCount) + int32(level)
		switch (mouse.Number - firstReagent) / 5 {
		case 0:
			target.Number = ItemCureWounds
		case 1:
			target.Number = ItemMagicPotion
		case 2:
			target.Number = ItemCureWeak
		default:
			target.Number = ItemCatalyst
		}
		m.MouseItem = items.Item{}
		if first {
			m.Speak(i, 0x10, c)
		}
		return MixDone
	case mouse.Number >= firstPotion && mouse.Number <= lastPotion && target.Number >= firstPotion && target.Number <= lastPotion:
		return m.mix(i, slot, level, mastery, first, c, n)
	case mouse.Number == ItemRecharge || mouse.Number == ItemHarden || mouse.Number >= ItemFlaming && mouse.Number <= ItemFlaming+4 || mouse.Number == ItemSlaying:
		return m.potionOnItem(target, mouse, c)
	}
	return MixNone
}

// mix is two potions together.
func (m *Members) mix(i int, slot int32, level, mastery int, first bool, c *Ctx, n Notes) Mix {
	p := &m.Players[i]
	t := c.Items
	target := p.Item(slot)
	mouse := m.MouseItem
	var r int32
	if mouse.Number == ItemCatalyst || target.Number == ItemCatalyst {
		r = 5
	} else {
		r = int32(t.Potion.At(mouse.Number, target.Number))
	}
	// The alchemy rank a result needs: normal for 0xe1..0xe3, expert 0xe4..0xef,
	// master 0xf0..0x105, grandmaster above; short of it, explosion 1..4.
	if level == 0 {
		switch {
		case r >= 0xe1 && r <= 0xe3:
			r = 1
		case r >= 0xe4 && r <= 0xef:
			r = 2
		case r >= 0xf0 && r <= 0x105:
			r = 3
		case r > 0x105:
			r = 4
		}
	} else {
		switch {
		case r >= 0xe4 && r <= 0xef:
			if mastery == 1 {
				r = 2
			}
		case r >= 0xf0 && r <= 0x105:
			if mastery < 3 {
				r = 3
			}
		case r > 0x105:
			if mastery != 4 {
				r = 4
			}
		}
	}
	top := p.topOf(slot)
	if top < 0 {
		top = 0
	}
	switch r {
	case 0:
		return MixNone
	case 1, 2, 3, 4:
		p.RemoveItemAt(t, top)
		switch r {
		case 1:
			m.ReceiveDamage(i, int32(c.Rand.Int()%0xb+10), 0, c)
		case 2:
			m.ReceiveDamage(i, int32(c.Rand.Int()%0x47+0x1e), 0, c)
			p.BreakItems(1, c.Rand)
		case 3:
			m.ReceiveDamage(i, int32(c.Rand.Int()%0xc9+0x32), 0, c)
			p.BreakItems(5, c.Rand)
		case 4:
			m.SetCondition(i, CondEradicated, false, c)
			p.BreakItems(0, c.Rand)
		}
		m.MouseItem = items.Item{}
		if first {
			if p.CanAct() {
				m.Speak(i, 0x11, c)
			}
			n.Status(n.Global(0x1bc), 2)
		}
		return MixExplode
	case 5:
		if level == 0 {
			return MixNone
		}
		if target.Number == ItemCatalyst {
			target.Number = mouse.Number
		} else {
			target.Bonus = mouse.Bonus
		}
		m.addBottle(p, t, c)
	default:
		m.addBottle(p, t, c)
		note := int(t.PotNotes.At(mouse.Number, target.Number))
		target.Number = r
		target.Bonus = (target.Bonus + mouse.Bonus) / 2
		// Evt_Set(member, 0xe1, note): a new autonote with text makes the member speak.
		if !m.Autonotes.Get(note) && n.AutonoteText(note) {
			m.Speak(i, 0x60, c)
		}
		m.Autonotes.Set(note, true)
	}
	if t.Item(target.Number).IDRepair == 0 {
		target.Flags |= items.FlagIdentified
	}
	m.MouseItem = items.Item{}
	if first {
		m.Speak(i, 0x10, c)
	}
	return MixDone
}

// addBottle puts an empty (identified) bottle in the pack, if it fits.
func (m *Members) addBottle(p *Player, t *tables.Items, c *Ctx) {
	s, full := p.AddItemNumber(t, -1, ItemBottle)
	if s != 0 {
		p.Item(s).Flags = items.FlagIdentified
	}
	m.sayFull(full, c)
}

// potionOnItem is a potion for items used on one: Recharge refills a wand's charges
// less (70 - power) % of them, Harden hardens an unbroken item that is not an artifact,
// the enchanting potions give a weapon without a bonus a special one for power × 1800
// × 128 / 30 game ticks.
//
// mm8: 0x415c6d (0x416665..)
func (m *Members) potionOnItem(target *items.Item, mouse items.Item, c *Ctx) Mix {
	t := c.Items
	typ := t.Item(target.Number).EquipType
	switch {
	case mouse.Number == ItemRecharge:
		if typ != tables.EquipWand {
			return MixError
		}
		s := float64(70-mouse.Bonus) * 0.01
		if s < 0 {
			s = 0
		}
		mx := float64(target.MaxCharges)
		target.MaxCharges = uint8(int64(mx - mx*s))
		target.Charges = int32(target.MaxCharges)
	case mouse.Number == ItemHarden:
		if target.Flags&items.FlagBroken == 0 && typ < tables.EquipReagent && target.Number < items.FirstArtifact {
			target.Flags |= items.FlagHardened | 0x10 // 0x10: the sparkle
		}
	default:
		target.ExpireBonus(int64(m.Time))
		weapon := typ == tables.EquipWeapon || typ == tables.EquipWeapon2 || typ == tables.EquipMissile
		if target.Flags&items.FlagBroken != 0 || target.Special != 0 || target.Bonus != 0 || !weapon || target.Number >= items.FirstArtifact {
			break
		}
		if mouse.Number == ItemSlaying {
			target.Special = 0x28
		} else {
			target.Special = c.Classes.EnchantSpecial[mouse.Number-ItemFlaming]
		}
		d := int64(float64(mouse.Bonus*0x708<<7) * float64(float32(1.0/30)))
		target.Expires = int64(m.Time) + d
		target.Flags |= items.FlagTempBonus | 0x10
	}
	m.MouseItem = items.Item{}
	return MixDone
}
