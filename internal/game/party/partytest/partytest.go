// Package partytest has small, made-up item and class tables for the stat tests: the
// shapes of the game's tables with values that keep hand-traced results simple.
package partytest

import "libre-enroth/internal/assets/tables"

// Item ids of Items.
const (
	Sword     = 1 // weapon, sword, 2d4 + 1, 1x3 cells
	Chain     = 2 // armour, chain, AC 10 + 2, 2x3
	Ring      = 3 // ring, 1x1
	Spear     = 4 // weapon, spear, 1d6, 1x4
	Plate     = 5 // armour, plate, AC 20
	Leather   = 6 // armour, leather, AC 4
	Bow       = 7 // missile, bow, 3d2 + 1
	Shield    = 8 // shield, AC 6
	Potion    = 9 // a potion bottle
	Wand      = 10
	Gem       = 11 // misc, 2x2
	NumItems  = 12
	OfMight   = 1 // standard bonus "of Might"
	OfHealth  = 8 // standard bonus "of Health" (HP)
	OfArms    = 0x16
	OfGods    = 2 // special bonus "of the Gods" (+10 to the seven stats)
	OfDoom    = 0x2a
	OfAirMagi = 0x1a
)

// Items is a 12-item table with the standard and special bonuses the tests use, item
// weights for levels 1..6 and the bonus chances.
func Items() *tables.Items {
	t := &tables.Items{Items: make([]tables.ItemDef, NumItems), Std: make([]tables.StdBonus, tables.NumStd),
		Spc: make([]tables.SpcBonus, tables.NumSpc), SpcCount: 3}
	def := func(id int, name string, typ, skill, n, s, mod2 uint8, value int32, w, h int) {
		t.Items[id] = tables.ItemDef{Name: name, UnidentifiedName: "?" + name, EquipType: typ, Skill: skill,
			DiceCount: n, DiceSides: s, Mod2: mod2, Value: value, W: w, H: h, IDRepair: 1}
	}
	def(Sword, "Sword", tables.EquipWeapon, 1, 2, 4, 1, 100, 1, 3)
	def(Chain, "Chain", tables.EquipArmor, 10, 10, 1, 2, 300, 2, 3)
	def(Ring, "Ring", tables.EquipRing, tables.SkillMisc, 0, 0, 0, 50, 1, 1)
	def(Spear, "Spear", tables.EquipWeapon, 4, 1, 6, 0, 80, 1, 4)
	def(Plate, "Plate", tables.EquipArmor, 11, 20, 1, 0, 500, 2, 3)
	def(Leather, "Leather", tables.EquipArmor, 9, 4, 1, 0, 60, 2, 3)
	def(Bow, "Bow", tables.EquipMissile, 5, 3, 2, 1, 150, 1, 4)
	def(Shield, "Shield", tables.EquipShield, 8, 6, 1, 0, 90, 2, 2)
	def(Potion, "Potion", tables.EquipPotion, tables.SkillMisc, 0, 0, 0, 10, 1, 1)
	def(Wand, "Wand", tables.EquipWand, tables.SkillMisc, 0, 0, 2, 200, 1, 2)
	def(Gem, "Gem", tables.EquipGem, tables.SkillMisc, 0, 0, 0, 30, 2, 2)
	t.Items[Ring].IDRepair = 0
	t.Items[Potion].IDRepair = 0
	// rnditems weights: level 1 items 1..3 (10, 20, 30); level 3 the sword and chain
	t.Items[Sword].Chance = [6]uint8{10, 0, 5, 0, 0, 0}
	t.Items[Chain].Chance = [6]uint8{20, 0, 5, 0, 0, 0}
	t.Items[Ring].Chance = [6]uint8{30, 0, 0, 0, 0, 0}
	for _, it := range t.Items {
		for l, c := range it.Chance {
			t.LevelTotals[l] += int32(c)
		}
	}
	t.Std[OfMight-1] = tables.StdBonus{OfName: "of Might", Stat: "Might", Chance: [9]uint8{5, 0, 0, 0, 0, 0, 0, 10, 0}}
	t.Std[OfHealth-1] = tables.StdBonus{OfName: "of Health", Stat: "Hit Points", Chance: [9]uint8{10, 0, 0, 0, 0, 0, 0, 0, 0}}
	t.Std[OfArms-1] = tables.StdBonus{OfName: "of Arms", Stat: "Armsmaster skill", Chance: [9]uint8{0, 0, 0, 0, 0, 0, 0, 10, 0}}
	for _, s := range t.Std {
		for k, c := range s.Chance {
			t.StdTotals[k] += int32(c)
		}
	}
	t.StdRange = [6][2]int32{{0, 0}, {1, 5}, {3, 8}, {6, 12}, {10, 17}, {15, 25}}
	t.Spc[0] = tables.SpcBonus{Name: "of Protection", Value: 1000, Level: 1, Chance: [12]uint8{0, 0, 0, 10, 0, 0, 0, 0, 0, 0, 10, 0}}
	t.Spc[OfGods-1] = tables.SpcBonus{Name: "of The Gods", Value: 3, Level: 0, Chance: [12]uint8{5, 0, 0, 10, 0, 0, 0, 0, 0, 0, 0, 0}}
	t.Spc[2] = tables.SpcBonus{Name: "of Carnage", Value: 5000, Level: 3, Chance: [12]uint8{10, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}}
	t.Spc[OfDoom-1] = tables.SpcBonus{Name: "of Doom", Value: 500, Level: 1}
	t.Spc[OfAirMagi-1] = tables.SpcBonus{Name: "of Air Magic", Value: 2, Level: 1}
	for i := 0; i < t.SpcCount; i++ {
		for k, c := range t.Spc[i].Chance {
			t.SpcTotals[k] += int32(c)
		}
	}
	t.BonusChance = [3][6]int32{{0, 40, 40, 40, 40, 75}, {0, 0, 10, 15, 20, 25}, {0, 0, 10, 20, 30, 50}}
	return t
}

// Classes are class tables where the knight (4) has 35 HP + 5 a level and no spell
// points, the cleric (2) 30 HP + 2 and 15 SP + 3 a level by Personality, the necromancer
// (0) 20 HP + 3 and 25 SP + 4 by Intellect. StatToBonus is 2 from 20, 1 from 10, else 0.
// Weak leaves half of Might; from 50 years Might counts 75 %.
func Classes() *tables.Classes {
	c := &tables.Classes{
		BonusLimits: []int16{20, 10, 0},
		Bonus:       []int8{2, 1, 0},
		AgeLimits:   [4]int32{50, 100, 150, 65535},
		EquipSlot:   [13]uint8{1, 1, 2, 3, 0, 4, 5, 6, 7, 8, 10, 9, 1},
	}
	c.HPBase[4], c.HPPerLevel[4] = 35, 5
	c.HPBase[2], c.HPPerLevel[2], c.SPBase[2], c.SPPerLevel[2] = 30, 2, 15, 3
	c.HPBase[0], c.HPPerLevel[0], c.SPBase[0], c.SPPerLevel[0] = 20, 3, 25, 4
	for cl := range c.Stats {
		for s := range c.Stats[cl] {
			c.Stats[cl][s] = tables.StatRange{Base: 11, Max: 25, UpCost: 1, UpStep: 1}
		}
	}
	c.Stats[4][0] = tables.StatRange{Base: 14, Max: 35, UpCost: 1, UpStep: 2} // cheap Might
	c.Stats[4][1] = tables.StatRange{Base: 7, Max: 15, UpCost: 2, UpStep: 1}  // dear Intellect
	for s := range c.CondPct {
		for k := range c.CondPct[s] {
			c.CondPct[s][k] = 100
		}
		c.AgePct[s] = [4]uint8{100, 100, 100, 100}
	}
	c.CondPct[0][1] = 50
	c.AgePct[0] = [4]uint8{100, 75, 40, 10}
	// knights: sword and leather to start, spear, chain, plate, shield, merchant choosable
	for s, v := range map[int]uint8{1: 2, 9: 2, 4: 1, 10: 1, 11: 1, 8: 1, 25: 1} {
		c.Skills[2][s] = v
	}
	return c
}
