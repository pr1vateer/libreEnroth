package party

import (
	"libre-enroth/internal/assets/tables"
	"libre-enroth/internal/game/items"
)

// Attack recovery (re/notes/combat.md). The weapon table is g_recoveryBase 0x4fff94:
// one u16 per item skill (staff 100, sword 90, dagger 60, axe 100, spear 80, bow 100,
// mace 80, blaster 30, shield 10, leather 10, chain 20, plate 30). An attack then
// waits at least 30 of those units, turned into ticks by recmod1 * 2.1333.

// recoveryBase is g_recoveryBase (0x4fff94), indexed by an item's skill.
var recoveryBase = [...]int{
	100, 90, 60, 100, 80, 100, 80, 30, 10, 10, 20, 30,
}

// meleeReach is how far a blow connects, in world units (0x42e27f: 407.2).
const meleeReach = 407.2

// swiftItems and the specials that take 20 off a weapon's recovery (0x48f237).
var swiftRecover = map[int32]bool{
	0x3b: true, 0x29: true,
}
var swiftNumbers = map[int32]bool{
	0x1f8: true, 0x1fa: true, 0x200: true, 0x203: true, 0x206: true, 0x214: true,
}

const slowNumber = 0x20d // adds 20 to recovery

// RecoveryUnits is how long a blow or a shot takes, in the weapon table's units:
// the slower weapon in hand (a wand reads nothing here; its spell's recovery is the
// cast queue's), plus armour and a shield, less speed, a skilled sword, axe or bow,
// armsmaster, haste and a swift enchantment. Never below 0.
//
// mm8: 0x48f237 (Player_GetRecovery)
func (p *Player) RecoveryUnits(e *Env, ranged bool) int {
	base := recoveryBase[0]
	var hand *items.Item
	blaster := false
	weapon := func(it *items.Item) (int, bool) {
		if it == nil || e.Items == nil || it.Def(e.Items).EquipType == tables.EquipWand {
			return 0, false
		}
		return recoveryBase[int(it.Def(e.Items).Skill)%len(recoveryBase)], true
	}
	if !ranged {
		if p.Unarmed(e) && p.Skill(e, SkillUnarmed) != 0 {
			base = recoveryBase[2]
		} else {
			if u, ok := weapon(p.Equipped(SlotMainHand)); ok {
				base = u
				hand = p.Equipped(SlotMainHand)
			}
			if it := p.Equipped(SlotOffhand); it != nil {
				if u, ok := weapon(it); ok && base < u {
					base = u
					hand = it
				}
			}
		}
	} else if it := p.Equipped(SlotBow); it != nil {
		if u, ok := weapon(it); ok {
			base = u
			hand = it
		}
	}
	armor := 0
	if it := p.Equipped(SlotArmor); it != nil && e.Items != nil {
		leather := Mastery(p.Skills[SkillLeather])
		if leather < 1 || leather >= len(recoveryBase) {
			leather = 1
		}
		raw := recoveryBase[leather]
		d := it.Def(e.Items)
		sk := p.Skill(e, int(d.Skill))
		factor := armorFactor(int(d.Skill), Mastery(sk))
		armor = int(float64(raw) * factor)
	}
	shield := 0
	if it := p.Equipped(SlotOffhand); it != nil && e.Items != nil && it.Def(e.Items).EquipType == tables.EquipShield {
		d := it.Def(e.Items)
		raw := recoveryBase[int(d.Skill)%len(recoveryBase)]
		sk := p.Skill(e, int(d.Skill))
		m := Mastery(sk)
		if m < 1 {
			m = 1
		}
		if m == 1 {
			shield = raw
		}
	}
	speed := 0
	if e.Classes != nil {
		speed = e.Classes.StatToBonus(p.ActualStat(e, StatSpeed))
	}
	trained := 0
	if hand != nil && e.Items != nil {
		skill := int(hand.Def(e.Items).Skill)
		blaster = skill == SkillBlaster
		if skill == SkillSword || skill == SkillAxe || skill == SkillBow {
			sk := p.Skill(e, skill)
			if Mastery(p.Skills[skill]) > 1 {
				trained = int(sk & SkillLevel)
			}
		}
	}
	arms := 0
	if !ranged && !blaster {
		if sk := p.Skill(e, SkillArmsmaster); sk != 0 {
			arms = int(sk & SkillLevel)
			if Mastery(sk) > 3 {
				arms *= 2
			}
		}
	}
	haste := 0
	if p.Buffs[BuffHaste].Active() {
		haste = 25
	}
	swift, slow := 0, 0
	if hand != nil {
		if swiftRecover[hand.Special] || swiftNumbers[hand.Number] {
			swift = 20
		}
		if hand.Number == slowNumber {
			slow = 20
		}
	}
	n := slow - arms - swift - haste - trained - speed + shield + armor + base
	if n < 0 {
		return 0
	}
	return n
}

// armorFactor is the share of the armour penalty a rank pays: leather is free from
// expert, chain from master (expert pays half), plate from grandmaster (expert and
// master pay half). Anything else pays it all. Rank is 1..4.
func armorFactor(skill, rank int) float64 {
	if rank < 1 {
		rank = 1
	}
	if rank > 4 {
		rank = 4
	}
	var f [4]float64
	switch skill {
	case SkillLeather:
		f = [4]float64{1, 0, 0, 0}
	case SkillChain:
		f = [4]float64{1, 0.5, 0, 0}
	case SkillPlate:
		f = [4]float64{1, 0.5, 0.5, 0}
	default:
		f = [4]float64{1, 1, 1, 1}
	}
	return f[rank-1]
}

// AttackDelay is the recovery an attack gives, in timer ticks: at least 30 table
// units, times recmod1 and 2.1333. Turn-based mode keeps the units for the queue
// (M9f) and waits here not at all.
//
// mm8: 0x42e27f (the recovery after a blow)
func AttackDelay(units int, turnBased bool) int {
	if turnBased {
		return 0
	}
	if units < 0x1e {
		units = 0x1e
	}
	return int(float64(float64(units)*float64(recMod1)) * fallRecovery)
}

// SpellDelay is the recovery a cast gives. The spell row's units (a bow or a breath
// passes Player.RecoveryUnits instead) are multiplied by recmod1 and 2.1333, with no
// floor of 30. Turn-based mode stores the units themselves (the queue is M9f).
//
// mm8: 0x42685f (the fmul of 0x4ea438 after the switch)
func SpellDelay(units int, turnBased bool) int {
	if units < 0 {
		units = 0
	}
	if turnBased {
		return units
	}
	return int(float64(float64(units)*float64(recMod1)) * fallRecovery)
}

// RollMeleeDamage is one melee blow's damage. Unarmed is 1 + rand()%3. A weapon is
// its dice (a spear in both hands rolls one more) plus its Mod2, doubled against the
// monster kind a slaying enchantment names, and tripled one time in ten for a master
// of the dagger. Might, the damage skills and the damage buffs are added unless
// noMight (a blaster's swing). mainOnly leaves the off hand out. At least 1.
// isKind reports Monster_IsKind (nil skips the slaying check).
//
// mm8: 0x48e317 (Player_RollMeleeDamage)
func (p *Player) RollMeleeDamage(e *Env, monsterID int, isKind func(id, kind int) bool, noMight, mainOnly bool, rng *Rand) int {
	total := 0
	if p.Unarmed(e) {
		total = 1
		if rng != nil {
			total = rng.Int()%3 + 1
		}
	} else {
		roll := func(it *items.Item) int {
			if it == nil || e.Items == nil {
				return 0
			}
			d := it.Def(e.Items)
			n := int(d.DiceCount)
			if d.Skill == SkillSpear && p.Equip[SlotOffhand] == 0 && it == p.Equipped(SlotMainHand) {
				n++
			}
			dmg := 0
			for range n {
				sides := int(d.DiceSides)
				if sides < 1 {
					sides = 1
				}
				dmg += 1 + rng.Int()%sides
			}
			dmg += int(d.Mod2)
			if isKind != nil && slays(it, d, monsterID, isKind) {
				dmg *= 2
			}
			if d.Skill == SkillDagger && Mastery(p.Skills[SkillDagger]) > 2 && rng.Int()%100 < 10 {
				dmg *= 3
			}
			return dmg
		}
		total = roll(p.Equipped(SlotMainHand))
		if !mainOnly {
			if it := p.Equipped(SlotOffhand); it != nil && e.Items != nil && it.Def(e.Items).EquipType != tables.EquipShield {
				total += roll(it)
			}
		}
	}
	if !noMight && e.Classes != nil {
		total += e.Classes.StatToBonus(p.ActualStat(e, StatMight))
		total += p.SkillBonus(e, StatDamage) + p.BuffBonus(e, StatDamage)
	}
	if total < 1 {
		return 1
	}
	return total
}

// RollRangedDamage is one shot from the bow: its dice plus Mod2, doubled when a
// slaying special matches the monster, plus the level of a grandmaster of the bow.
// monster 0 skips the slaying check. No bow in hand is 0. The caller passes the
// actor's monList, as 0x4378ec does.
//
// mm8: 0x48e6c3
func (p *Player) RollRangedDamage(e *Env, monster int, isKind func(id, kind int) bool, rng *Rand) int {
	it := p.Equipped(SlotBow)
	if it == nil || e == nil || e.Items == nil || rng == nil {
		return 0
	}
	d := it.Def(e.Items)
	total := 0
	sides := int(d.DiceSides)
	if sides < 1 {
		sides = 1
	}
	for range int(d.DiceCount) {
		total += 1 + rng.Int()%sides
	}
	total += int(d.Mod2)
	if monster != 0 && isKind != nil {
		kind := 0
		switch {
		case it.Special == 0x40:
			kind = 1
		case it.Special == 0x27 || it.Number == 0x1f7:
			kind = 7
		case it.Special == 0x28:
			kind = 2
		case it.Special == 0x3f:
			kind = 8
		}
		if kind != 0 && isKind(monster, kind) {
			total *= 2
		}
	}
	if p.Skills[SkillBow] != 0 && Mastery(p.Skills[SkillBow]) > 3 {
		total += int(p.Skills[SkillBow] & SkillLevel)
	}
	return total
}

// slays reports an enchantment that doubles the blow against monsterID.
func slays(it *items.Item, d *tables.ItemDef, monsterID int, isKind func(id, kind int) bool) bool {
	if monsterID == 0 {
		return false
	}
	kind := 0
	switch {
	case it.Special == 0x40:
		kind = 1
	case it.Special == 0x27 || it.Number == 0x1f7:
		kind = 7
	case it.Special == 0x28 || it.Number == 0x1fa:
		kind = 2
	case it.Special == 0x3f:
		kind = 8
	default:
		return false
	}
	return isKind(monsterID, kind)
}

// ElementDamage is the extra damage a weapon's enchantment adds, and whether it
// drinks blood (the caller heals). typ is a damage type; dmg 0 adds nothing.
//
// mm8: 0x4383da
func ElementDamage(it *items.Item, rng *Rand) (typ int, dmg int, vamp bool) {
	if it == nil || it.Number == 0 {
		return 0, 0, false
	}
	roll := func() int { return rng.Int() }
	dice := func(n, sides int) int {
		s := 0
		for range n {
			s += 1 + roll()%sides
		}
		return s
	}
	switch it.Number {
	case 500:
		return DamageLight, roll()%10 + 10, false
	case 0x1f5:
		return DamageDark, roll()%10 + 10, false
	case 0x1f8:
		return DamageFire, roll()%6 + 6, false
	case 0x1f9, 0x20d:
		return DamageFire, roll()%10 + 10, false
	case 0x1fb, 0x1fe:
		return DamageBody, roll()%10 + 10, false
	case 0x1fc:
		return DamageBody, 10, true
	case 0x211:
		return DamageAir, roll()%6 + 4, false
	}
	switch it.Special {
	case 0x2e:
		return DamageFire, roll()%11 + 10, false
	case 0xb:
		return DamageFire, dice(2, 6), false
	case 4:
		return DamageWater, roll()%2 + 3, false
	case 5:
		return DamageWater, roll()%3 + 6, false
	case 6:
		return DamageWater, roll()%4 + 9, false
	case 7:
		return DamageAir, roll()%4 + 2, false
	case 8:
		return DamageAir, roll()%7 + 4, false
	case 9:
		return DamageAir, roll()%10 + 6, false
	case 10:
		return DamageFire, dice(1, 6), false
	case 0xc:
		return DamageFire, dice(3, 6), false
	case 0xd:
		return DamageBody, 5, false
	case 0xe:
		return DamageBody, 8, false
	case 0xf:
		return DamageWater, 12, false
	case 0x10, 0x29, 0x20f:
		if it.Special == 0x20f {
			return DamageFire, 0, true
		}
		return DamageDark, 0, true
	}
	return 0, 0, false
}

// HasItemSpecial reports a worn item whose special bonus is sp. 0x18 stuns on a
// blow that lands; the function returns 5 for that one (0x48fddd).
//
// mm8: 0x48fddd
func (p *Player) HasItemSpecial(sp int32) bool {
	for s := range NumSlots {
		if it := p.Equipped(s); it != nil && it.Special == sp {
			return true
		}
	}
	return false
}

// AwardExp divides a kill's experience among the members who are not unconscious,
// dead, stoned or eradicated. Each one's Learning adds its percent. Nothing is given
// when the share rounds to 0, and a member stops at 4,000,000,000.
//
// mm8: 0x4255bb (Party_AwardExp)
func (m *Members) AwardExp(total int, c *Ctx) {
	if total <= 0 || c == nil {
		return
	}
	var living []int
	for i := range m.Players {
		p := &m.Players[i]
		if p.Conditions[CondUnconscious] != 0 || p.Conditions[CondDead] != 0 ||
			p.Conditions[CondStoned] != 0 || p.Conditions[CondEradicated] != 0 {
			continue
		}
		living = append(living, i)
	}
	if len(living) == 0 {
		return
	}
	share := total / len(living)
	if share == 0 {
		return
	}
	e := m.Env(c)
	for _, i := range living {
		p := &m.Players[i]
		got := int64(share + p.LearningBonus(e)*share/100)
		p.Exp += got
		if p.Exp > 4000000000 {
			p.Exp = 4000000000
		}
	}
}
