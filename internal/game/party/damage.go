package party

import "libre-enroth/internal/game/items"

// Monster attack bonuses (monsters.txt "Bonus", MonsterInfo +0x13).
const (
	BonusCurse     = 1
	BonusWeak      = 2
	BonusAsleep    = 3
	BonusDrunk     = 4
	BonusInsane    = 5
	BonusPoison1   = 6
	BonusPoison2   = 7
	BonusPoison3   = 8
	BonusDisease1  = 9
	BonusDisease2  = 0xa
	BonusDisease3  = 0xb
	BonusParalyze  = 0xc
	BonusUncon     = 0xd
	BonusDead      = 0xe
	BonusStone     = 0xf
	BonusErad      = 0x10
	BonusBrkItem   = 0x11
	BonusBrkArmor  = 0x12
	BonusBrkWeapon = 0x13
	BonusSteal     = 0x14
	BonusAge       = 0x15
	BonusDrainSP   = 0x16
	BonusAfraid    = 0x17
)

// Speech of a member a bonus hits.
const (
	speechItemLost = 0x28
	speechDrained  = 0x29
	speechAged     = 0x2a
)

// MonsterBonus is a monster's blow's special effect on member i, which the member may
// shake off: rand() % (StatToBonus(Luck) + 30 + strength) must be under 30 for it to
// work, the strength being a stat bonus or a resistance by the effect (or the item's
// (Mod2 + Material) * 3 for the breaking ones). Curses and conditions are set (as the
// original maps them: Poison2 gives poison 3 and Poison3 poison 2, Disease1 disease 3,
// Disease2 and 3 nothing); an item breaks (unless hardened); Steal takes a pack item
// into the first free one of the monster's two item slots (steal; false when both are
// full); Age adds a year, DrainSP empties the spell points. It reports whether the
// effect was not shaken off (sounds and the portrait spark are M11's).
//
// mm8: 0x48ed43 (Player_MonsterBonus)
func (m *Members) MonsterBonus(i, bonus int, c *Ctx, steal func(items.Item) bool) bool {
	p := &m.Players[i]
	e := m.Env(c)
	toBonus := func(s Stat) int { return e.Classes.StatToBonus(p.ActualStat(e, s)) }
	var cand []int
	var item *items.Item
	cell := 0
	strength := 0
	itemStrength := func() bool {
		if len(cand) == 0 {
			return false
		}
		item = &p.Items[cand[c.Rand.Int()%len(cand)]]
		d := item.Def(e.Items)
		strength = (int(d.Mod2) + int(d.Material)) * 3
		return true
	}
	switch bonus {
	case BonusCurse:
		strength = toBonus(StatPersonality)
	case BonusWeak, BonusAsleep, BonusDrunk, BonusDisease1, BonusDisease2, BonusDisease3, BonusUncon, BonusAge:
		strength = toBonus(StatEndurance)
	case BonusInsane, BonusParalyze, BonusAfraid:
		strength = p.Resist(e, StatMindRes)
	case BonusPoison1, BonusPoison2, BonusPoison3, BonusDead, BonusErad:
		strength = p.Resist(e, StatBodyRes)
	case BonusStone:
		strength = p.Resist(e, StatEarthRes)
	case BonusBrkItem:
		for k := range items.Slots {
			if it := &p.Items[k]; it.Number > 0 && it.Number < 0x98 && !it.Broken() {
				cand = append(cand, k)
			}
		}
		if !itemStrength() {
			return false
		}
	case BonusBrkArmor, BonusBrkWeapon:
		for s := range NumSlots {
			if p.Equipped(s) == nil {
				continue
			}
			typ := p.Item(p.Equip[s]).Def(e.Items).EquipType
			switch {
			case bonus == BonusBrkArmor && s == SlotArmor, bonus == BonusBrkWeapon && s == SlotBow:
				cand = append(cand, int(p.Equip[s]-1))
			}
			if s <= SlotMainHand {
				if bonus == BonusBrkArmor && typ == 4 || bonus == BonusBrkWeapon && (typ == 0 || typ == 1) {
					cand = append(cand, int(p.Equip[s]-1))
				}
			}
		}
		if !itemStrength() {
			return false
		}
	case BonusSteal:
		var cells []int
		for k := range items.PackSlots {
			if n := p.Grid[k]; n > 0 {
				if num := p.Items[n-1].Number; num > 0 && num < 0x98 {
					cells = append(cells, k)
				}
			}
		}
		if len(cells) == 0 {
			return false
		}
		cell = cells[c.Rand.Int()%len(cells)]
		strength = toBonus(StatAccuracy)
	case BonusDrainSP:
		strength = (toBonus(StatPersonality) + toBonus(StatIntellect)) >> 1
	}
	if c.Rand.Int()%(toBonus(StatLuck)+30+strength) > 29 {
		return false
	}
	switch bonus {
	case BonusCurse, BonusWeak, BonusAsleep, BonusDrunk, BonusAfraid:
		cond := map[int]Condition{BonusCurse: CondCursed, BonusWeak: CondWeak, BonusAsleep: CondAsleep,
			BonusDrunk: CondDrunk, BonusAfraid: CondAfraid}[bonus]
		m.SetCondition(i, cond, true, c)
	case BonusInsane, BonusParalyze, BonusUncon, BonusDead, BonusStone, BonusErad:
		cond := map[int]Condition{BonusInsane: CondInsane, BonusParalyze: CondParalyzed, BonusUncon: CondUnconscious,
			BonusDead: CondDead, BonusStone: CondStoned, BonusErad: CondEradicated}[bonus]
		m.SetCondition(i, cond, true, c)
	case BonusPoison1:
		m.SetCondition(i, CondPoison1, true, c)
	case BonusPoison2:
		m.SetCondition(i, CondPoison3, true, c)
	case BonusPoison3:
		m.SetCondition(i, CondPoison2, true, c)
	case BonusDisease1:
		m.SetCondition(i, CondDisease3, true, c)
	case BonusBrkItem, BonusBrkArmor, BonusBrkWeapon:
		if item.Flags&items.FlagHardened == 0 {
			m.Speak(i, speechItemLost, c)
			item.Flags |= items.FlagBroken
		}
	case BonusSteal:
		m.Speak(i, speechItemLost, c)
		if steal(*p.Item(p.Grid[cell])) {
			p.RemoveItemAt(e.Items, cell)
		}
	case BonusAge:
		m.Speak(i, speechAged, c)
		p.AgeBonus++
	case BonusDrainSP:
		m.Speak(i, speechDrained, c)
		p.SP = 0
	default:
		return false
	}
	return true
}

// MonsterHitRecovery is the recovery a monster's blow gives the member it hits, as
// a fall's: (20 - StatToBonus(Endurance)) * recmod1 * 2.1333 ticks.
//
// mm8: 0x438625
func (m *Members) MonsterHitRecovery(i int, c *Ctx) {
	e := m.Env(c)
	b := e.Classes.StatToBonus(m.Players[i].ActualStat(e, StatEndurance))
	m.SetRecovery(i, int(float64(float64(20-b)*float64(recMod1))*fallRecovery))
}
