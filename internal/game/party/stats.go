package party

import (
	"libre-enroth/internal/assets/tables"
	"libre-enroth/internal/game/clock"
	"libre-enroth/internal/game/items"
)

// Stat is a "character attribute" code: what the item, buff and skill bonus functions
// are asked for. 0..6 are the seven stats (Accuracy before Speed here, unlike the
// struct), then HP, SP, AC, the resistances, the skills that items can raise, the
// level and the attack and damage values.
type Stat int

const (
	StatMight       Stat = 0
	StatIntellect   Stat = 1
	StatPersonality Stat = 2
	StatEndurance   Stat = 3
	StatAccuracy    Stat = 4
	StatSpeed       Stat = 5
	StatLuck        Stat = 6
	StatHP          Stat = 7
	StatSP          Stat = 8
	StatAC          Stat = 9
	StatFireRes     Stat = 0xa
	StatAirRes      Stat = 0xb
	StatWaterRes    Stat = 0xc
	StatEarthRes    Stat = 0xd
	StatMindRes     Stat = 0xe
	StatBodyRes     Stat = 0xf
	StatAlchemy     Stat = 0x10
	StatStealing    Stat = 0x11
	StatDisarm      Stat = 0x12
	StatIDItem      Stat = 0x13
	StatIDMonster   Stat = 0x14
	StatArmsmaster  Stat = 0x15
	StatDodge       Stat = 0x16
	StatUnarmed     Stat = 0x17
	StatLevel       Stat = 0x18
	StatAttack      Stat = 0x19 // melee attack bonus
	StatDamage      Stat = 0x1a // melee damage bonus
	StatDamageMin   Stat = 0x1b
	StatDamageMax   Stat = 0x1c
	StatRanged      Stat = 0x1d // ranged attack bonus
	StatRangedDmg   Stat = 0x1e
	StatRangedMin   Stat = 0x1f
	StatRangedMax   Stat = 0x20
	StatSpiritRes   Stat = 0x21
	StatFireMagic   Stat = 0x22 // .. 0x2a: the magic skills fire, air, water, earth, spirit, mind, body, light, dark
	StatMeditation  Stat = 0x2b
	StatBow         Stat = 0x2c
	StatShield      Stat = 0x2d
	StatLearning    Stat = 0x2e
	StatDarkElf     Stat = 0x2f
	StatVampire     Stat = 0x30
	StatDragon      Stat = 0x31
)

// Skills (Player +0x378, 39 of them): the low 6 bits are the level, 0x40 expert, 0x80
// master, 0x100 grandmaster.
const (
	SkillStaff        = 0
	SkillSword        = 1
	SkillDagger       = 2
	SkillAxe          = 3
	SkillSpear        = 4
	SkillBow          = 5
	SkillMace         = 6
	SkillBlaster      = 7
	SkillShield       = 8
	SkillLeather      = 9
	SkillChain        = 10
	SkillPlate        = 11
	SkillFire         = 12 // .. 20: fire, air, water, earth, spirit, mind, body, light, dark
	SkillDarkElf      = 21
	SkillVampire      = 22
	SkillDragon       = 23
	SkillIDItem       = 24
	SkillMerchant     = 25
	SkillRepair       = 26
	SkillBodybuilding = 27
	SkillMeditation   = 28
	SkillPerception   = 29
	SkillRegeneration = 30
	SkillDisarm       = 31
	SkillDodge        = 32
	SkillUnarmed      = 33
	SkillIDMonster    = 34
	SkillArmsmaster   = 35
	SkillStealing     = 36
	SkillAlchemy      = 37
	SkillLearning     = 38
	NumSkills         = 39

	SkillLevel  = 0x3f
	SkillExpert = 0x40
	SkillMaster = 0x80
	SkillGM     = 0x100
)

// Mastery is a skill value's rank: 1 normal, 2 expert, 3 master, 4 grandmaster.
//
// mm8: 0x456f57
func Mastery(skill uint16) int {
	switch {
	case skill&SkillGM != 0:
		return 4
	case skill&SkillMaster != 0:
		return 3
	case skill&SkillExpert != 0:
		return 2
	}
	return 1
}

// MasteryMult is the multiplier of a rank: 1, 2, 3, 5.
//
// mm8: 0x491e52
func MasteryMult(skill uint16) int {
	switch {
	case skill&SkillGM != 0:
		return 5
	case skill&SkillMaster != 0:
		return 3
	case skill&SkillExpert != 0:
		return 2
	}
	return 1
}

// Equipment slots (Player +0x1c04: the 1-based item slot in each, 0 = empty).
const (
	SlotOffhand  = 0
	SlotMainHand = 1
	SlotBow      = 2
	SlotArmor    = 3
	SlotHelm     = 4
	SlotBelt     = 5
	SlotCloak    = 6
	SlotGauntlet = 7
	SlotBoots    = 8
	SlotAmulet   = 9
	SlotRing     = 10 // .. 15
	NumSlots     = 16
)

// Damage types of ReceiveDamage and the resistances (Player +0x1a08 base, +0x1a1e
// bonus, both [11]).
const (
	DamageFire     = 0
	DamageAir      = 1
	DamageWater    = 2
	DamageEarth    = 3
	DamagePhysical = 4
	DamageMagic    = 5
	DamageSpirit   = 6
	DamageMind     = 7
	DamageBody     = 8
	DamageLight    = 9
	DamageDark     = 10
	NumResists     = 11
)

// Buff is a SpellBuff (0x10 bytes): a spell's effect until it expires. M9 casts them;
// until then they stay zero and the stat formulas read their powers as 0.
type Buff struct {
	Expires clock.Time // +0x00
	Power   uint16     // +0x08
	Skill   uint16     // +0x0a
	Overlay uint16     // +0x0c
	Caster  uint8      // +0x0e
	Flags   uint8      // +0x0f
}

// Active reports a buff that has not expired (expire time > 0, signed).
func (b *Buff) Active() bool { return b.Expires > 0 }

// Buff counts: the party's (Party +0x8a8) and each player's (Player +0x1a34).
const (
	NumPartyBuffs  = 20
	NumPlayerBuffs = 27
)

// Player buffs the formulas read.
const (
	BuffPreservation = 11 // HP at 0 or below leaves the player unconscious, not dead
	BuffRegeneration = 12
	BuffMerchant     = 24 // replaces the merchant skill level with its power
	BuffPainReflect  = 26 // physical damage does nothing
)

// Env is what the stat formulas read besides the player: the tables, the party's buffs
// and the game time (for the age).
type Env struct {
	Items   *tables.Items
	Classes *tables.Classes
	Buffs   *[NumPartyBuffs]Buff
	Time    clock.Time
}

// Env returns the stat environment of the party's members.
func (m *Members) Env(c *Ctx) *Env {
	return &Env{Items: c.Items, Classes: c.Classes, Buffs: &m.Buffs, Time: m.Time}
}

func (e *Env) partyBuff(i int) uint16 {
	if e.Buffs == nil {
		return 0
	}
	return e.Buffs[i].Power
}

// statOffsets are the struct order of the seven stats (+0x354.. {base, bonus}): Might,
// Intellect, Personality, Endurance, Speed, Accuracy, Luck. Stat codes put Accuracy
// before Speed, so stat code s lives at Stats[statSlot[s]].
var statSlot = [7]int{0, 1, 2, 3, 5, 4, 6}

// StatPair is a stat's base value and its permanent bonus.
type StatPair struct{ Base, Bonus int16 }

// Base is a stat's base value (struct order handled).
func (p *Player) Base(s Stat) int { return int(p.Stats[statSlot[s]].Base) }

// BasePtr and BonusPtr address a stat's base value and bonus by stat code.
func (p *Player) BasePtr(s Stat) *int16  { return &p.Stats[statSlot[s]].Base }
func (p *Player) BonusPtr(s Stat) *int16 { return &p.Stats[statSlot[s]].Bonus }

// SkillAt returns the raw skill value (level and mastery bits), 0 outside 0..38.
func (p *Player) SkillAt(skill int) uint16 {
	if skill < 0 || skill >= NumSkills {
		return 0
	}
	return p.Skills[skill]
}

// Item returns the item in 1-based slot n (nil for 0 or out of range).
func (p *Player) Item(n int32) *items.Item {
	if n < 1 || int(n) > len(p.Items) {
		return nil
	}
	return &p.Items[n-1]
}

// Equipped returns the item worn in equipment slot s when it is not broken, else nil.
//
// mm8: 0x48eb25
func (p *Player) Equipped(s int) *items.Item {
	n := p.Equip[s]
	if n == 0 {
		return nil
	}
	it := p.Item(n)
	if it == nil || it.Broken() {
		return nil
	}
	return it
}

// equipDef is the items.txt row of the item in slot s (worn or broken; the original's
// 0x48eaa7/0x48eacc read it either way, an empty slot reading item 0).
func (p *Player) equipDef(e *Env, s int) *tables.ItemDef {
	if it := p.Item(p.Equip[s]); it != nil {
		return it.Def(e.Items)
	}
	return e.Items.Item(0)
}

// HasSpecial reports a worn item with special bonus sp.
//
// mm8: 0x48eb4b
func (p *Player) HasSpecial(sp int32) bool {
	for s := 0; s < NumSlots; s++ {
		if it := p.Equipped(s); it != nil && it.Special == sp {
			return true
		}
	}
	return false
}

// Wears reports item id worn in slot (or in any slot for slot >= NumSlots).
//
// mm8: 0x48eb8c
func (p *Player) Wears(id int32, slot int) bool {
	if slot < NumSlots {
		it := p.Equipped(slot)
		return it != nil && it.Number == id
	}
	for s := 0; s < NumSlots; s++ {
		if it := p.Equipped(s); it != nil && it.Number == id {
			return true
		}
	}
	return false
}

// Unarmed reports no weapon in hand: nothing worn in the main hand and nothing but a
// shield in the off hand.
//
// mm8: 0x48eaf1
func (p *Player) Unarmed(e *Env) bool {
	if p.Equipped(SlotMainHand) != nil {
		return false
	}
	if p.Equipped(SlotOffhand) != nil && p.equipDef(e, SlotOffhand).EquipType != tables.EquipShield {
		return false
	}
	return true
}

// isDragon reports the dragon classes (14, 15).
func (p *Player) isDragon() bool { return p.Class == 14 || p.Class == 15 }

// skillOfStat is the skill an item bonus to a skill stat needs; the bonus is 0
// without it. Stats without an entry (-1) need nothing.
//
// mm8: 0x48fe4d (the first switch)
func skillOfStat(s Stat) int {
	switch s {
	case StatAlchemy:
		return SkillAlchemy
	case StatDisarm:
		return SkillDisarm
	case StatIDItem:
		return SkillIDItem
	case StatIDMonster:
		return SkillIDMonster
	case StatArmsmaster:
		return SkillArmsmaster
	case StatDodge:
		return SkillDodge
	case StatUnarmed:
		return SkillUnarmed
	case StatMeditation:
		return SkillMeditation
	case StatBow:
		return SkillBow
	case StatShield:
		return SkillShield
	case StatLearning:
		return SkillLearning
	case StatDarkElf:
		return SkillDarkElf
	case StatVampire:
		return SkillVampire
	case StatDragon:
		return SkillDragon
	}
	if s >= StatFireMagic && s <= StatFireMagic+8 {
		return SkillFire + int(s-StatFireMagic)
	}
	return -1
}

// halfSkill is half a skill's level (the low byte >> 1 & 0x1f), the bonus the "of
// <school> Magic" specials and some artifacts give.
func (p *Player) halfSkill(skill int) int { return int(uint8(p.Skills[skill])>>1) & 0x1f }

// ItemBonus is what the worn items add to stat s: the standard bonuses (summed for
// stats and resistances, the highest for skills), the special bonuses and the
// artifacts', the weapons' dice and damage modifiers for the attack values, and the
// armour's AC. A dragon's AC also counts its level and five times its dragon skill.
// noOffhand leaves the off hand out of the melee values.
//
// mm8: 0x48fe4d
func (p *Player) ItemBonus(e *Env, s Stat, noOffhand bool) int {
	if sk := skillOfStat(s); sk >= 0 && p.Skills[sk] == 0 {
		return 0
	}
	v, half, arms := 0, 0, 0
	if p.isDragon() && s == StatAC {
		v = p.Level(e) + int(p.Skill(e, SkillDragon)&SkillLevel)*5
	}
	dice := func(d *tables.ItemDef) int { return int(d.DiceCount) + int(d.Mod2) }
	isWeapon := func(d *tables.ItemDef) bool { return d.EquipType <= tables.EquipMissile }
	switch {
	case s < 0:
	case s <= StatUnarmed || s >= StatFireMagic && s <= StatDragon:
		for slot := 0; slot < NumSlots; slot++ {
			it := p.Equipped(slot)
			if it == nil {
				continue
			}
			d := it.Def(e.Items)
			if s == StatAC {
				if t := p.equipDef(e, slot).EquipType; t > tables.EquipMissile && t < tables.EquipWand {
					v += dice(d)
				}
			}
			if !d.IsRare() || d.Material == tables.MaterialSpecial {
				if it.Bonus == int32(s)+1 {
					switch {
					case s < StatAlchemy:
						v += int(it.Strength)
					case s <= StatUnarmed:
						v = max(v, int(it.Strength))
					}
					continue
				}
				v = specialBonus(p, s, it.Special, v, &half)
				continue
			}
			v = artifactBonus(p, s, it.Number, v, &half, &arms)
		}
	case s == StatLevel:
		if p.HasSpecial(0x19) {
			v += 5
		}
	case s == StatAttack || s == StatDamage:
		if p.Unarmed(e) {
			v = 0
			break
		}
		if p.Equipped(SlotMainHand) != nil {
			if d := p.equipDef(e, SlotMainHand); isWeapon(d) {
				v += int(d.Mod2)
			}
		}
		if !noOffhand && p.Equipped(SlotOffhand) != nil {
			if d := p.equipDef(e, SlotOffhand); isWeapon(d) {
				v += int(d.Mod2)
			}
		}
	case s == StatDamageMin:
		if p.Unarmed(e) {
			v++
			break
		}
		if p.Equipped(SlotMainHand) != nil {
			if d := p.equipDef(e, SlotMainHand); isWeapon(d) {
				v += dice(d)
				// a spear held in both hands
				if p.Equip[SlotOffhand] == 0 && d.Skill == SkillSpear {
					v++
				}
			}
		}
		if !noOffhand && p.Equipped(SlotOffhand) != nil {
			if d := p.equipDef(e, SlotOffhand); isWeapon(d) {
				v += dice(d)
			}
		}
	case s == StatDamageMax:
		if p.Unarmed(e) {
			v += 3
			break
		}
		if p.Equipped(SlotMainHand) != nil {
			if d := p.equipDef(e, SlotMainHand); isWeapon(d) {
				n := int(d.DiceCount)
				if p.Equip[SlotOffhand] == 0 && d.Skill == SkillSpear {
					n++
				}
				v += int(d.Mod2) + n*int(d.DiceSides)
			}
		}
		if !noOffhand && p.Equipped(SlotOffhand) != nil {
			if d := p.equipDef(e, SlotOffhand); isWeapon(d) {
				v += int(d.Mod2) + int(d.DiceCount)*int(d.DiceSides)
			}
		}
	case s == StatRanged || s == StatRangedDmg:
		if p.Equipped(SlotBow) != nil {
			v += int(p.equipDef(e, SlotBow).Mod2)
		}
	case s == StatRangedMin:
		if p.Equipped(SlotBow) != nil {
			v += dice(p.equipDef(e, SlotBow))
		}
	case s == StatRangedMax:
		if p.Equipped(SlotBow) != nil {
			d := p.equipDef(e, SlotBow)
			v += int(d.Mod2) + int(d.DiceCount)*int(d.DiceSides)
		}
	}
	return arms + half + v
}

// specialBonus adds special bonus sp's share of stat s to v (or sets half, the "of
// <school> Magic" half skill levels; the last one counts).
//
// mm8: 0x48fe4d (the special bonus switch)
func specialBonus(p *Player, s Stat, sp int32, v int, half *int) int {
	in := func(set ...Stat) bool {
		for _, x := range set {
			if s == x {
				return true
			}
		}
		return false
	}
	atLeast := func(n int) int { return max(v, n) }
	switch sp {
	case 1: // of Protection: the six resistances
		if s >= StatFireRes && s <= StatBodyRes {
			v += 10
		}
	case 2: // of the Gods: the seven stats
		if s >= StatMight && s <= StatLuck {
			v += 10
		}
	case 0x1a:
		if s == StatFireMagic+1 {
			*half = p.halfSkill(SkillFire + 1)
		}
	case 0x1b:
		if s == StatFireMagic+6 {
			*half = p.halfSkill(SkillFire + 6)
		}
	case 0x1c:
		if s == StatFireMagic+8 {
			*half = p.halfSkill(SkillFire + 8)
		}
	case 0x1d:
		if s == StatFireMagic+3 {
			*half = p.halfSkill(SkillFire + 3)
		}
	case 0x1e:
		if s == StatFireMagic {
			*half = p.halfSkill(SkillFire)
		}
	case 0x1f:
		if s == StatFireMagic+7 {
			*half = p.halfSkill(SkillFire + 7)
		}
	case 0x20:
		if s == StatFireMagic+5 {
			*half = p.halfSkill(SkillFire + 5)
		}
	case 0x21:
		if s == StatFireMagic+4 {
			*half = p.halfSkill(SkillFire + 4)
		}
	case 0x22:
		if s == StatFireMagic+2 {
			*half = p.halfSkill(SkillFire + 2)
		}
	case 0x2a: // of Doom: stats and resistances +1
		if s >= StatMight && s <= StatBodyRes {
			v++
		}
	case 0x2b:
		if in(StatEndurance, StatHP, StatAC) {
			v += 10
		}
	case 0x2c:
		if s == StatHP {
			v += 10
		}
	case 0x2d:
		if in(StatSpeed, StatAccuracy) {
			v += 5
		}
	case 0x2e:
		if s == StatMight {
			v += 25
		}
	case 0x2f:
		if s == StatSP {
			v += 10
		}
	case 0x30:
		if s == StatAC {
			v += 5
		}
		if s == StatEndurance {
			v += 15
		}
	case 0x31:
		if in(StatLuck, StatIntellect) {
			v += 10
		}
	case 0x32:
		if s == StatFireRes {
			v += 30
		}
	case 0x33:
		if in(StatSpeed, StatIntellect, StatSP) {
			v += 10
		}
	case 0x34:
		if in(StatEndurance, StatAccuracy) {
			v += 10
		}
	case 0x35:
		if in(StatMight, StatPersonality) {
			v += 10
		}
	case 0x36:
		if s == StatEndurance {
			v += 15
		}
	case 0x37:
		if s == StatLuck {
			v += 15
		}
	case 0x38:
		if in(StatMight, StatEndurance) {
			v += 5
		}
	case 0x39:
		if in(StatIntellect, StatPersonality) {
			v += 5
		}
	case 0x3c:
		if in(StatUnarmed, StatDodge) {
			v = atLeast(3)
		}
	case 0x3d:
		if in(StatStealing, StatDisarm) {
			v = atLeast(3)
		}
	case 0x3e:
		if in(StatIDItem, StatIDMonster) {
			v = atLeast(3)
		}
	case 0x43:
		if s == StatDisarm {
			v = atLeast(2)
		}
	case 0x44:
		if s == StatAC {
			v += 5
		}
	case 0x45:
		if s == StatAirRes {
			v += 20
		}
	case 0x46:
		if s == StatWaterRes {
			v += 10
		}
		if s == StatAlchemy {
			v = atLeast(2)
		}
	}
	return v
}

// artifactBonus adds artifact id's share of stat s to v; the half skill levels go to
// half and the Armsmaster bonus of 0x1f6 to arms.
//
// mm8: 0x48fe4d (the artifact switch)
func artifactBonus(p *Player, s Stat, id int32, v int, half, arms *int) int {
	in := func(set ...Stat) bool {
		for _, x := range set {
			if s == x {
				return true
			}
		}
		return false
	}
	add := func(n int, set ...Stat) {
		if in(set...) {
			v += n
		}
	}
	halfOf := func(stat Stat) {
		if s == stat {
			*half = p.halfSkill(SkillFire + int(stat-StatFireMagic))
		}
	}
	stats := s >= StatMight && s <= StatLuck
	resists := s >= StatFireRes && s <= StatBodyRes
	switch id {
	case 500:
		add(40, StatAccuracy)
	case 0x1f5:
		add(40, StatMight)
	case 0x1f6:
		if s == StatArmsmaster {
			*arms += 7
		} else {
			add(30, StatAirRes)
		}
	case 0x1f7:
		add(40, StatEndurance, StatLuck)
	case 0x1f8:
		add(20, StatMight)
	case 0x1f9:
		add(40, StatFireRes)
	case 0x1fa:
		add(20, StatEndurance)
	case 0x1fb:
		if stats {
			v += 10
		}
	case 0x1fd:
		add(40, StatPersonality)
	case 0x1fe:
		add(20, StatMight, StatEndurance)
	case 0x200:
		add(50, StatAccuracy)
		add(4, StatBow)
	case 0x201:
		add(30, StatEndurance)
	case 0x202:
		if stats || resists {
			v += 10
		}
	case 0x203:
		add(15, StatSpeed, StatAccuracy)
	case 0x204:
		halfOf(StatFireMagic + 4)
		halfOf(StatFireMagic + 6)
		halfOf(StatFireMagic + 5)
	case 0x205:
		add(8, StatDisarm, StatBow, StatArmsmaster)
	case 0x206:
		add(30, StatSpeed)
	case 0x207:
		if s >= StatFireRes && s <= StatEarthRes {
			v += 40
		}
	case 0x208:
		add(15, StatIntellect, StatPersonality)
	case 0x209:
		halfOf(StatFireMagic + 8)
		add(50, StatIntellect)
	case 0x20a:
		add(30, StatIntellect)
		if resists {
			v += 10
		}
	case 0x20b:
		add(-15, StatPersonality)
		add(-50, StatWaterRes)
	case 0x20c:
		add(70, StatSpeed, StatAccuracy)
		add(-20, StatAC)
	case 0x20d:
		add(-20, StatSpeed)
	case 0x20e:
		add(70, StatMight, StatAccuracy)
		add(-50, StatIntellect, StatPersonality)
	case 0x20f:
		add(50, StatMight)
		add(-40, StatLuck)
	case 0x210:
		add(70, StatWaterRes)
		add(-70, StatFireRes)
	case 0x211:
		add(40, StatMight)
		add(-40, StatAccuracy)
	case 0x212:
		halfOf(StatFireMagic + 1)
		halfOf(StatFireMagic)
		halfOf(StatFireMagic + 2)
		halfOf(StatFireMagic + 3)
		add(-40, StatAC)
	case 0x213:
		add(100, StatAccuracy)
		add(5, StatBow)
		add(-20, StatAC)
	case 0x214:
		add(-50, StatAccuracy)
	case 0x215:
		add(80, StatPersonality)
		add(70, StatIntellect)
		add(-30, StatMindRes, StatSpiritRes)
	case 0x216:
		add(-15, StatPersonality, StatLuck)
	case 0x217:
		halfOf(StatFireMagic + 2)
		add(5, StatAlchemy)
		add(40, StatIntellect)
		add(-20, StatEndurance)
	case 0x218:
		add(90, StatLuck)
		add(-50, StatPersonality)
	case 0x219:
		add(100, StatMight)
		add(-30, StatAccuracy)
		add(-15, StatAC)
	}
	return v
}

// BuffBonus is what the player's and the party's buffs add to stat s (M9 casts them).
//
// mm8: 0x490a44
func (p *Player) BuffBonus(e *Env, s Stat) int {
	pb := func(i int) int { return int(p.Buffs[i].Power) }
	party := func(i int) int { return int(e.partyBuff(i)) }
	switch s {
	case StatMight:
		return pb(19) + party(2)
	case StatIntellect:
		return pb(17) + party(2)
	case StatPersonality:
		return pb(20) + party(2)
	case StatEndurance:
		return pb(16) + party(2)
	case StatAccuracy:
		return pb(15) + party(2)
	case StatSpeed:
		return pb(21) + party(2)
	case StatLuck:
		return pb(18) + party(2)
	case StatAC:
		return pb(14) + party(15)
	case StatFireRes:
		return pb(5) + party(6)
	case StatAirRes:
		return pb(0) + party(0)
	case StatWaterRes:
		return pb(22) + party(17)
	case StatEarthRes:
		return pb(3) + party(4)
	case StatMindRes:
		return pb(9) + party(12)
	case StatBodyRes:
		return pb(2) + party(1)
	case StatAttack, StatRanged:
		return pb(1)
	case StatDamage:
		return pb(8) + party(9)
	}
	return 0
}

// skillStat is the item bonus stat of a skill (-1: items do not raise it).
//
// mm8: 0x490b8c (Player_GetSkill's switch)
func skillStat(skill int) Stat {
	switch {
	case skill >= SkillFire && skill <= SkillFire+8:
		return StatFireMagic + Stat(skill-SkillFire)
	}
	switch skill {
	case SkillBow:
		return StatBow
	case SkillShield:
		return StatShield
	case SkillIDItem:
		return StatIDItem
	case SkillMeditation:
		return StatMeditation
	case SkillDisarm:
		return StatDisarm
	case SkillDodge:
		return StatDodge
	case SkillUnarmed:
		return StatUnarmed
	case SkillIDMonster:
		return StatIDMonster
	case SkillArmsmaster:
		return StatArmsmaster
	case SkillStealing:
		return StatStealing
	case SkillAlchemy:
		return StatAlchemy
	case SkillLearning:
		return StatLearning
	}
	return -1
}

// Skill is a skill's value with the items' bonus to its level (the merchant buff's
// power for Merchant), the level kept within 1..60 (a negative total leaves level 1).
//
// mm8: 0x490b8c (Player_GetSkill)
func (p *Player) Skill(e *Env, skill int) uint16 {
	if skill < 0 || skill >= NumSkills {
		return 0
	}
	bonus := 0
	if skill == SkillMerchant && p.Buffs[BuffMerchant].Active() {
		bonus = int(p.Buffs[BuffMerchant].Power)
	}
	if s := skillStat(skill); s >= 0 {
		bonus += p.ItemBonus(e, s, false)
	}
	raw := p.Skills[skill]
	lvl := int(raw&SkillLevel) + bonus
	switch {
	case lvl >= 0x3c:
		return raw&^SkillLevel + 0x3c
	case lvl < 0:
		return raw&^SkillLevel + 1
	}
	return uint16(int(raw) + bonus)
}

// SkillBonus is what the skills add to stat s: HP and SP from Bodybuilding and
// Meditation times the class's per level gain, AC from the armour and shield skills
// and Dodging, and the attack and damage of the weapon skills, Unarmed, Armsmaster and
// the bow and blaster.
//
// mm8: 0x490cc3
func (p *Player) SkillBonus(e *Env, s Stat) int {
	arms := 0
	if a := p.Skill(e, SkillArmsmaster); a != 0 {
		m, k := Mastery(a), 0
		switch s {
		case StatDamage:
			switch {
			case m >= 4:
				k = 2
			case m >= 3:
				k = 1
			}
		case StatAttack:
			switch {
			case m >= 4:
				k = 2
			case m >= 2:
				k = 1
			}
		}
		arms = int(a&SkillLevel) * k
	}
	cls := p.classIndex()
	switch s {
	case StatHP:
		b := p.Skill(e, SkillBodybuilding)
		return MasteryMult(b) * int(b&SkillLevel) * int(e.Classes.HPPerLevel[cls])
	case StatSP:
		b := p.Skill(e, SkillMeditation)
		return MasteryMult(b) * int(b&SkillLevel) * int(e.Classes.SPPerLevel[cls])
	case StatAC:
		return p.armorSkillAC(e)
	case StatAttack:
		v := 0
		if p.Unarmed(e) {
			u := p.Skill(e, SkillUnarmed)
			if u == 0 {
				return 0
			}
			k := 1
			if Mastery(u) > 2 {
				k = 2
			}
			v = int(u&SkillLevel) * k
		} else {
			it, d := p.firstWeapon(e)
			if it == nil {
				return 0
			}
			sk := p.Skill(e, int(d.Skill))
			k, extra := 0, 0
			switch d.Skill {
			case SkillStaff:
				k = 1
				if Mastery(sk) > 3 {
					if u := p.Skill(e, SkillUnarmed); u != 0 {
						m := 1
						if Mastery(u) > 2 {
							m = 2
						}
						extra = int(u&SkillLevel) * m
					}
				}
			case SkillSword, SkillDagger, SkillAxe, SkillSpear, SkillMace:
				k = 1
			case SkillBlaster:
				return int(sk&SkillLevel) * MasteryMult(sk)
			}
			v = int(sk&SkillLevel)*k + extra
		}
		return v + arms
	case StatRanged:
		for slot := 0; slot < NumSlots; slot++ {
			if p.Equipped(slot) == nil {
				continue
			}
			d := p.equipDef(e, slot)
			sk := p.Skill(e, int(d.Skill))
			switch d.Skill {
			case SkillBow:
				return int(sk & SkillLevel)
			case SkillBlaster:
				return int(sk&SkillLevel) * MasteryMult(sk)
			}
		}
		return 0
	case StatDamage, StatDamageMin, StatDamageMax:
		if p.Unarmed(e) {
			u := p.Skill(e, SkillUnarmed)
			if u == 0 {
				return 0
			}
			k := 0
			switch m := Mastery(u); {
			case m >= 3:
				k = 2
			case m >= 2:
				k = 1
			}
			return int(u&SkillLevel) * k
		}
		it, d := p.firstWeapon(e)
		if it == nil {
			return 0
		}
		sk := p.Skill(e, int(d.Skill))
		m, k := Mastery(sk), 0
		switch d.Skill {
		case SkillStaff, SkillDagger:
			if m >= 4 {
				k = 1
			}
		case SkillAxe:
			if m >= 3 {
				k = 1
			}
		case SkillSpear, SkillMace:
			if m >= 2 {
				k = 1
			}
		}
		if s != StatDamage {
			arms = 0
		}
		return int(sk&SkillLevel)*k + arms
	}
	return 0
}

// firstWeapon is the first worn item (by slot) of a hand-held type (weapon or
// two-handed weapon).
func (p *Player) firstWeapon(e *Env) (*items.Item, *tables.ItemDef) {
	for slot := 0; slot < NumSlots; slot++ {
		it := p.Equipped(slot)
		if it == nil {
			continue
		}
		if d := it.Def(e.Items); d.EquipType <= tables.EquipWeapon2 {
			return it, d
		}
	}
	return nil, nil
}

// armorSkillAC is the AC the armour skills give: each worn item's skill level counts
// once per rank up to its mastery, at the ranks its kind lists; Dodging adds the same
// way when nothing worn forbids it (a shield, or armour beyond leather, which needs a
// grandmaster).
//
// mm8: 0x490cc3 (param 9)
func (p *Player) armorSkillAC(e *Env) int {
	total := 0
	noDodge, leather := false, false
	for slot := 0; slot < NumSlots; slot++ {
		if p.Equipped(slot) == nil {
			continue
		}
		d := p.equipDef(e, slot)
		var ranks [4]bool
		switch d.Skill {
		case SkillStaff:
			ranks = [4]bool{false, true, false, false}
		case SkillSword, SkillSpear:
			ranks = [4]bool{false, false, false, true}
		case SkillShield:
			noDodge = true
			ranks = [4]bool{true, false, true, false}
		case SkillLeather:
			leather = true
			ranks = [4]bool{true, false, true, false}
		case SkillChain, SkillPlate:
			noDodge = true
			ranks = [4]bool{true, false, false, false}
		default:
			continue
		}
		sk := p.Skill(e, int(d.Skill))
		for r := 0; r < Mastery(sk); r++ {
			if ranks[r] {
				total += int(sk & SkillLevel)
			}
		}
	}
	dodge := p.Skill(e, SkillDodge)
	m := Mastery(dodge)
	if noDodge || leather && m != 4 {
		return total
	}
	for r, ok := range [4]bool{true, true, true, false} {
		if r < m && ok {
			total += int(dodge & SkillLevel)
		}
	}
	return total
}

// classIndex is the class id kept inside the tables.
func (p *Player) classIndex() int {
	if p.Class < 0 || p.Class >= tables.NumClasses {
		return 0
	}
	return p.Class
}

// Level is the level with its bonus and the items' and buffs'.
//
// mm8: 0x48df95
func (p *Player) Level(e *Env) int {
	return int(p.LevelBase) + p.ItemBonus(e, StatLevel, false) + p.BuffBonus(e, StatLevel) + int(p.LevelBonus)
}

// BaseAge is the age from the calendar: game years since the start + 1172 - birth year.
//
// mm8: 0x48f744
func (p *Player) BaseAge(e *Env) int {
	hours := e.Time.Seconds() / 60 / 60
	days := int(uint32(hours) / 24)
	return days/7/4/12 - int(p.BirthYear) + clock.BaseYear
}

// Age adds the age bonus (magical ageing).
//
// mm8: 0x48f794
func (p *Player) Age(e *Env) int { return p.BaseAge(e) + int(p.AgeBonus) }

// CondPercent is the % of stat s the main condition leaves.
//
// mm8: 0x48fd62
func (p *Player) CondPercent(e *Env, s Stat) int {
	return int(e.Classes.CondPct[s][p.MainCondition()])
}

// BaseStat is a stat's base value and the items' bonus (the Base* getters, also used
// as the "full" value against which drained stats show).
//
// mm8: 0x48dedd..0x48df67 (0x48df7e is BaseStat(StatLevel) over the level)
func (p *Player) BaseStat(e *Env, s Stat) int {
	return p.ItemBonus(e, s, false) + p.Base(s)
}

// ActualStat is a stat as the formulas use it: the base value scaled by the age % and
// then the condition %, plus the bonus, the items' and the buffs'.
//
// mm8: 0x48dfc4 (Might), 0x48e020, 0x48e07d, 0x48e0da (Endurance), 0x48e137
// (Accuracy), 0x48e194 (Speed), 0x48e1f1 (Luck)
func (p *Player) ActualStat(e *Env, s Stat) int {
	age := e.Classes.AgePercent(p.Age(e), int(s))
	cond := p.CondPercent(e, s)
	sp := p.Stats[statSlot[s]]
	return int(sp.Bonus) + cond*(age*int(sp.Base)/100)/100 + p.ItemBonus(e, s, false) + p.BuffBonus(e, s)
}

// MaxHP is the class's base HP, plus the skills', the items' and (StatToBonus(End) +
// level) times the class's per level gain; at least 1.
//
// mm8: 0x48f5b9
func (p *Player) MaxHP(e *Env) int32 {
	c := p.classIndex()
	end := e.Classes.StatToBonus(p.ActualStat(e, StatEndurance))
	v := int(e.Classes.HPBase[c]) + p.SkillBonus(e, StatHP) + p.ItemBonus(e, StatHP, false) +
		(end+p.Level(e))*int(e.Classes.HPPerLevel[c])
	return int32(max(v, 1))
}

// MaxSP is the spell points of the casting classes (by Intellect for the
// necromancers, dark elves and dragons, by Personality for the clerics, minotaurs and
// vampires); 0 for knights and trolls, else at least 0.
//
// mm8: 0x48f61d
func (p *Player) MaxSP(e *Env) int32 {
	var stat Stat
	switch p.Class {
	case 0, 1, 10, 11, 14, 15:
		stat = StatIntellect
	case 2, 3, 8, 9, 12, 13:
		stat = StatPersonality
	default:
		return 0
	}
	c := p.classIndex()
	b := e.Classes.StatToBonus(p.ActualStat(e, stat))
	v := (b+p.Level(e))*int(e.Classes.SPPerLevel[c]) + int(e.Classes.SPBase[c]) +
		p.ItemBonus(e, StatSP, false) + p.SkillBonus(e, StatSP)
	return int32(max(v, 0))
}

// BaseAC is the AC without the AC bonus and the buffs.
//
// mm8: 0x48f6be
func (p *Player) BaseAC(e *Env) int {
	v := e.Classes.StatToBonus(p.ActualStat(e, StatSpeed)) + p.ItemBonus(e, StatAC, false) + p.SkillBonus(e, StatAC)
	return max(v, 0)
}

// AC is the armour class: StatToBonus(Speed), the items', the skills', the buffs' and
// the AC bonus; at least 0.
//
// mm8: 0x48f6f7
func (p *Player) AC(e *Env) int {
	v := e.Classes.StatToBonus(p.ActualStat(e, StatSpeed)) + p.ItemBonus(e, StatAC, false) +
		p.SkillBonus(e, StatAC) + p.BuffBonus(e, StatAC) + int(p.ACBonus)
	return max(v, 0)
}

// resistIndex maps a resistance stat to its damage type slot (-1: none).
func resistIndex(s Stat) int {
	switch s {
	case StatFireRes:
		return DamageFire
	case StatAirRes:
		return DamageAir
	case StatWaterRes:
		return DamageWater
	case StatEarthRes:
		return DamageEarth
	case StatMindRes:
		return DamageMind
	case StatBodyRes:
		return DamageBody
	case StatSpiritRes:
		return DamageSpirit
	}
	return -1
}

// ImmuneResist is the resistance of an immunity (vampires to mind, liches to mind and
// body).
const ImmuneResist = 65000

// racialResist is the class's bonus to a resistance and whether it is immune.
//
// mm8: 0x48f7a7, 0x48f96f
func (p *Player) racialResist(s Stat) (int, bool) {
	c := p.Class
	is := func(a, b int) bool { return c == a || c == b }
	v := 0
	switch s {
	case StatFireRes, StatAirRes, StatWaterRes:
		if is(10, 11) {
			v = 5
		}
	case StatEarthRes:
		if is(10, 11) {
			v = 5
		}
		if is(6, 7) {
			v += 5
		}
	case StatMindRes:
		if is(8, 9) {
			v = 5
		}
		if is(12, 13) || c == 1 {
			return v, true
		}
	case StatBodyRes:
		if is(8, 9) {
			v = 5
		}
		if is(6, 7) {
			v += 5
		}
		if c == 1 {
			return v, true
		}
	case StatSpiritRes:
		if is(4, 5) {
			v = 5
		}
		if is(2, 3) {
			v += 5
		}
		if is(0, 1) {
			v += 5
		}
		if is(8, 9) {
			v += 5
		}
	}
	return v, false
}

// BaseResist is a resistance without the buffs: its base value, the race's and the
// items'.
//
// mm8: 0x48f7a7
func (p *Player) BaseResist(e *Env, s Stat) int {
	r, immune := p.racialResist(s)
	if immune {
		return ImmuneResist
	}
	i := resistIndex(s)
	base := 0
	if i >= 0 {
		base = int(p.Resists[i])
	} else {
		base = int(p.Resists[0]) // other codes read the fire slot (iVar3 = 0)
	}
	return base + r + p.ItemBonus(e, s, false)
}

// Resist is a resistance: BaseResist plus the buffs, and the leather grandmaster's
// skill level to fire, air, water and earth while wearing leather.
//
// mm8: 0x48f96f
func (p *Player) Resist(e *Env, s Stat) int {
	extra := 0
	if s >= StatFireRes && s <= StatEarthRes && Mastery(p.Skills[SkillLeather]) == 4 &&
		p.Equipped(SlotArmor) != nil && p.equipDef(e, SlotArmor).Skill == SkillLeather {
		extra = int(p.Skills[SkillLeather] & SkillLevel)
	}
	r, immune := p.racialResist(s)
	if immune {
		return ImmuneResist
	}
	i := resistIndex(s)
	base := 0
	if i >= 0 {
		base = int(p.Resists[i])
	} else {
		base = int(p.Resists[0])
	}
	return base + p.BuffBonus(e, s) + extra + r + p.ItemBonus(e, s, false)
}

// MeleeAttack is the melee attack bonus: StatToBonus(Accuracy) and the skills', items'
// and buffs'.
//
// mm8: 0x48e24e
func (p *Player) MeleeAttack(e *Env, noOffhand bool) int {
	acc := e.Classes.StatToBonus(p.ActualStat(e, StatAccuracy))
	return p.BuffBonus(e, StatAttack) + p.ItemBonus(e, StatAttack, noOffhand) + p.SkillBonus(e, StatAttack) + acc
}

// MeleeDamageMin and MeleeDamageMax are the melee damage range: StatToBonus(Might),
// the weapons' dice, the skills' and the buffs'; at least 1.
//
// mm8: 0x48e293, 0x48e2d5
func (p *Player) MeleeDamageMin(e *Env) int { return p.meleeDamage(e, StatDamageMin) }
func (p *Player) MeleeDamageMax(e *Env) int { return p.meleeDamage(e, StatDamageMax) }

func (p *Player) meleeDamage(e *Env, s Stat) int {
	v := e.Classes.StatToBonus(p.ActualStat(e, StatMight)) + p.ItemBonus(e, s, false) +
		p.SkillBonus(e, StatDamage) + p.BuffBonus(e, StatDamage)
	return max(v, 1)
}

// RangedAttack is the ranged attack bonus.
//
// mm8: 0x48e5ba
func (p *Player) RangedAttack(e *Env) int {
	acc := e.Classes.StatToBonus(p.ActualStat(e, StatAccuracy))
	return p.BuffBonus(e, StatRanged) + acc + p.ItemBonus(e, StatRanged, false) + p.SkillBonus(e, StatRanged)
}

// RangedDamageMin and RangedDamageMax are the bow's damage range; a grandmaster of the
// bow adds the skill level. At least 0 (0 without a bow).
//
// mm8: 0x48e5f3, 0x48e65b
func (p *Player) RangedDamageMin(e *Env) int { return p.rangedDamage(e, StatRangedMin) }
func (p *Player) RangedDamageMax(e *Env) int { return p.rangedDamage(e, StatRangedMax) }

func (p *Player) rangedDamage(e *Env, s Stat) int {
	v := p.ItemBonus(e, s, false) + p.SkillBonus(e, StatRangedDmg) + p.BuffBonus(e, StatRangedDmg)
	if bow := p.Skills[SkillBow]; bow != 0 && Mastery(bow) > 3 && p.Equipped(SlotBow) != nil {
		v += int(bow & SkillLevel)
	}
	return max(v, 0)
}

// Global.txt text of the damage of a blaster in hand ("Variable").
const txtVariable = 0x253

// MeleeDamageText and RangedDamageText are the character screen's damage texts: "min -
// max", one number when equal, global.txt 0x253 when a blaster (items 150..176) is in
// the main hand, "N/A" for no ranged damage. Dragons show their breath, 10 + level to
// (5 * level + 5) * 2 of their skill.
//
// mm8: 0x48e7ba, 0x48e857
func (p *Player) MeleeDamageText(e *Env, global func(int) string) string {
	lo, hi := p.MeleeDamageMin(e), p.MeleeDamageMax(e)
	if p.isDragon() {
		lo, hi = p.dragonDamage(e)
	}
	return p.damageText(lo, hi, false, global)
}

func (p *Player) RangedDamageText(e *Env, global func(int) string) string {
	lo, hi := p.RangedDamageMin(e), p.RangedDamageMax(e)
	if p.isDragon() {
		lo, hi = p.dragonDamage(e)
	}
	return p.damageText(lo, hi, true, global)
}

func (p *Player) dragonDamage(e *Env) (int, int) {
	l := int(p.Skill(e, SkillDragon) & SkillLevel)
	return l + 10, (l*5 + 5) * 2
}

func (p *Player) damageText(lo, hi int, ranged bool, global func(int) string) string {
	var s string
	switch {
	case ranged && hi == 0:
		s = "N/A"
	case lo == hi:
		s = itoa(lo)
	default:
		s = itoa(lo) + " - " + itoa(hi)
	}
	if n := p.Equip[SlotMainHand]; n != 0 {
		if it := p.Item(n); it != nil && it.Number > 0x97 && it.Number < 0xb1 {
			s = global(txtVariable)
		}
	}
	return s
}

func itoa(v int) string {
	if v < 0 {
		return "-" + itoa(-v)
	}
	if v < 10 {
		return string(rune('0' + v))
	}
	return itoa(v/10) + string(rune('0'+v%10))
}

// MerchantDiscount is the Merchant skill's share in prices: 10000 for a grandmaster,
// else (rank - 1) * level + 7 + the level with items, less the map's reputation; a
// player without the skill gets only -reputation.
//
// mm8: 0x491f5e (Player_MerchantDiscount)
func (p *Player) MerchantDiscount(e *Env, reputation int) int {
	sk := p.Skill(e, SkillMerchant)
	raw := p.Skills[SkillMerchant]
	if Mastery(sk) >= 4 {
		return 10000
	}
	if sk&SkillLevel == 0 {
		return -reputation
	}
	return (MasteryMult(raw)-1)*int(raw&SkillLevel) - reputation + 7 + int(sk&SkillLevel)
}

// Perception is the trap-noticing value: 10000 for a grandmaster, else (rank - 1) *
// level + the level with items.
//
// mm8: 0x491fb0
func (p *Player) Perception(e *Env) int {
	sk, raw := p.Skill(e, SkillPerception), p.Skills[SkillPerception]
	if Mastery(sk) >= 4 {
		return 10000
	}
	return (MasteryMult(raw)-1)*int(raw&SkillLevel) + int(sk&SkillLevel)
}

// DisarmTrap is the trap-disarming value: as Perception, with the level doubled by an
// item of special 0x23.
//
// mm8: 0x491fed
func (p *Player) DisarmTrap(e *Env) int {
	sk, raw := p.Skill(e, SkillDisarm), p.Skills[SkillDisarm]
	if Mastery(sk) >= 4 {
		return 10000
	}
	lvl := int(sk & SkillLevel)
	if p.HasSpecial(0x23) {
		lvl *= 2
	}
	return (MasteryMult(raw)-1)*int(raw&SkillLevel) + lvl
}

// LearningBonus is the % of experience Learning adds: (rank - 1) * level + 9 + the level
// with items, 0 without the skill.
//
// mm8: 0x49203f
func (p *Player) LearningBonus(e *Env) int {
	sk, raw := p.Skill(e, SkillLearning), p.Skills[SkillLearning]
	if sk == 0 {
		return 0
	}
	return (MasteryMult(raw)-1)*int(raw&SkillLevel) + 9 + int(sk&SkillLevel)
}

// CanTrain reports enough experience for the next level: level * (level + 1) / 2 *
// 1000.
//
// mm8: 0x48e90c
func (p *Player) CanTrain() bool {
	n := int64(0)
	for i := int64(0); i < int64(p.LevelBase); i++ {
		n += i + 1
	}
	return int64(int32(n*1000)) <= p.Exp
}
