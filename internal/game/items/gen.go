package items

import "libre-enroth/internal/assets/tables"

// Kinds the generator takes besides an equip type + 1 (1..20): a whole skill or one of
// the coarser groups.
//
// mm8: 0x4552ca (ItemGen_Generate's switch on the kind)
const (
	KindAny = 0 // by the rnditems weights of the level
)

// kindTarget maps a kind to what the candidates must match: an equip type or a skill.
//
// mm8: 0x4552ca
func kindTarget(kind int) (equip int, skill int) {
	equip, skill = -1, -1
	switch kind {
	case 0x14:
		equip = tables.EquipWeapon
	case 0x15:
		equip = tables.EquipArmor
	case 0x16:
		skill = tables.SkillMisc
	case 0x17, 0x18, 0x19, 0x1a, 0x1b, 0x1c:
		skill = kind - 0x16 // sword .. mace
	case 0x1e:
		skill = 0 // staff
	case 0x1f, 0x20, 0x21:
		skill = kind - 0x16 // leather, chain, plate
	case 0x22, 0x23, 0x24, 0x25, 0x26, 0x27, 0x28, 0x29, 0x2a:
		equip = kind - 0x1e // shield .. wand
	case 0x2b:
		equip = tables.EquipSpellScroll
	case 0x2c:
		equip = tables.EquipPotion
	case 0x2d:
		equip = tables.EquipReagent
	case 0x2e, 0x2f:
		equip = tables.EquipGem
	default:
		equip = kind - 1
	}
	return equip, skill
}

// Generate makes a random item of treasure level 1..6 and kind (KindAny, an equip type
// + 1, or a kind of kindTarget). With KindAny the level's rnditems weights pick it and
// at level 6 one time in 20 it is an artifact not yet found (found records them). force
// makes an armour or weapon get a bonus whenever the level allows one. The rand()
// calls are the original's, in its order.
//
// mm8: 0x4552ca (ItemGen_Generate)
func Generate(t *tables.Items, level, kind int, force bool, rng Rand, found *[NumArtifacts]bool) Item {
	if found == nil {
		found = new([NumArtifacts]bool)
	}
	var it Item
	lvl := level - 1
	chance := func(id int32) int { return int(t.Item(id).Chance[lvl]) }
	if kind == KindAny {
		missing := 0
		for i := range found {
			if !found[i] {
				missing++
			}
		}
		r := rng.Int()
		if lvl == 5 && r%100 < 5 && missing > 0 {
			k := rng.Int() % missing
			i := 0
			for ; i < NumArtifacts; i++ {
				if !found[i] {
					if k == 0 {
						break
					}
					k--
				}
			}
			found[i] = true
			it.Number = int32(FirstArtifact + i)
			it.ApplySpecial(t)
			return it
		}
		r = rng.Int()
		if n := int(t.LevelTotals[lvl]); n != 0 {
			r %= n
		} else {
			r = 0 // the original divides by zero; the tables never have an empty level
		}
		sum := 0
		for sum < r {
			sum += chance(it.Number + 1)
			it.Number++
		}
		if it.Number == 0 {
			it.Number = 1
		}
	} else {
		equip, skill := kindTarget(kind)
		var cands []int32
		total := 0
		for id := int32(1); id < 500; id++ {
			d := t.Item(id)
			if skill >= 0 && int(d.Skill) == skill || skill < 0 && int(d.EquipType) == equip {
				cands = append(cands, id)
				total += chance(id)
			}
		}
		r := 0
		if total != 0 {
			r = rng.Int() % total
		}
		if len(cands) > 0 {
			it.Number = cands[0]
		}
		if it.Number == 0 {
			it.Number = 1
		}
		// The walk goes on past the candidates into the zeroed list (id 0) when the
		// weights do not reach r, as the original's does.
		sum := chance(it.Number)
		for k := 1; sum < r; k++ {
			it.Number = 0
			if k < len(cands) {
				it.Number = cands[k]
			}
			sum += chance(it.Number)
		}
	}
	d := t.Item(it.Number)
	if d.EquipType == tables.EquipPotion && it.Number != EmptyBottle {
		it.Bonus = 0
		for range 2 {
			it.Bonus += int32(rng.Int()%4 + 1)
		}
		it.Bonus *= int32(level)
	}
	if d.IDRepair == 0 {
		it.Flags = FlagIdentified
	}
	if d.EquipType != tables.EquipPotion {
		it.Special, it.Bonus = 0, 0
	}
	switch typ := d.EquipType; {
	case typ < tables.EquipArmor:
		c := int(t.BonusChance[2][lvl])
		if c == 0 {
			return it
		}
		if r := rng.Int(); c <= r%100 && !force {
			return it
		}
		it.Special = special(t, lvl, typ, rng)
	case typ > tables.EquipAmulet:
		if typ == tables.EquipWand {
			it.Charges = max(int32(rng.Int()%6+1+int(d.Mod2)), 1)
			it.MaxCharges = uint8(it.Charges)
		}
	default:
		stdC, spcC := int(t.BonusChance[0][lvl]), int(t.BonusChance[1][lvl])
		if stdC == 0 {
			return it
		}
		r := rng.Int() % 100
		if !force {
			if r >= stdC && spcC == 0 {
				return it
			}
		} else {
			r = rng.Int() % (spcC + stdC)
		}
		switch {
		case r < stdC:
			k := rng.Int() % int(t.StdTotals[typ-tables.EquipArmor])
			b := 0
			sum := int(t.Std[0].Chance[typ-tables.EquipArmor])
			for sum < k {
				b++
				sum += int(t.Std[b].Chance[typ-tables.EquipArmor])
			}
			it.Bonus = int32(b + 1)
			lo, hi := t.StdRange[lvl][0], t.StdRange[lvl][1]
			it.Strength = int32(rng.Int())%(hi-lo+1) + lo
			if b := it.Bonus - 1; b == 0x15 || b == 0x16 || b == 0x17 {
				it.Strength >>= 1 // armsmaster, dodging, unarmed
			}
			if it.Strength <= 0 {
				it.Strength = 1
			}
		case r < stdC+spcC:
			it.Special = special(t, lvl, typ, rng)
		}
	}
	return it
}

// special draws a special bonus for an item of equip type typ at level index lvl from
// the first SpcCount bonuses whose letter suits the level: A-B at level 3, A-C at 4,
// B-D at 5, D at 6 (none at 1 and 2, where the tables never ask for one).
//
// mm8: 0x4552ca (ItemGen_Generate, the end)
func special(t *tables.Items, lvl int, typ uint8, rng Rand) int32 {
	var cands []int
	total := 0
	for i := 0; i < t.SpcCount; i++ {
		s := &t.Spc[i]
		ok := false
		switch lvl {
		case 2:
			ok = s.Level == 0 || s.Level == 1
		case 3:
			ok = s.Level <= 2
		case 4:
			ok = s.Level >= 1 && s.Level <= 3
		case 5:
			ok = s.Level == 3
		}
		if !ok {
			continue
		}
		total += int(s.Chance[typ])
		if s.Chance[typ] != 0 {
			cands = append(cands, i)
		}
	}
	if total == 0 {
		return 0 // the original divides by zero here; the tables never get here
	}
	r := rng.Int()%total + 1
	pick := func(k int) int {
		if k < len(cands) {
			return cands[k]
		}
		return 0 // the zeroed rest of the list
	}
	n := pick(0)
	sum := int(t.Spc[n].Chance[typ])
	for k := 1; sum < r; k++ {
		n = pick(k)
		sum += int(t.Spc[n].Chance[typ])
	}
	return int32(n + 1)
}
