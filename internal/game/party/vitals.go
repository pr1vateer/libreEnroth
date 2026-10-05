package party

import "libre-enroth/internal/game/items"

// Armour damage factors of physical damage (floats in .rdata).
const (
	chainGMFactor     = float32(0.6667) // 0x4ea898
	plateMasterFactor = float32(0.5)    // 0x4ea508
)

// resistOfDamage is the resistance that guards against a damage type (-1: none).
//
// mm8: 0x48e967 (its switch)
func resistOfDamage(typ int) Stat {
	switch typ {
	case DamageFire:
		return StatFireRes
	case DamageAir:
		return StatAirRes
	case DamageWater:
		return StatWaterRes
	case DamageEarth:
		return StatEarthRes
	case DamageSpirit:
		return StatSpiritRes
	case DamageMind:
		return StatMindRes
	case DamageBody:
		return StatBodyRes
	}
	return -1
}

// IncomingDamage is how much of amount gets through: an immune lich takes none; a
// resistance halves it up to four times, each time rand() % (StatToBonus(Luck) + resist
// + 30) is 30 or more; physical damage through worn, unbroken chain (grandmaster) or
// plate (master) shrinks to 2/3 or 1/2.
//
// mm8: 0x48e967
func (p *Player) IncomingDamage(e *Env, typ int, amount int32, rng *Rand) int32 {
	res := 0
	if s := resistOfDamage(typ); s >= 0 {
		res = p.Resist(e, s)
	}
	if p.Class == 1 && res >= ImmuneResist {
		return 0
	}
	if res != 0 {
		d := max(e.Classes.StatToBonus(p.ActualStat(e, StatLuck))+res+30, 1)
		for range 4 {
			if rng.Int()%d < 30 {
				break
			}
			amount >>= 1
		}
	}
	if typ == DamagePhysical && p.Equip[SlotArmor] != 0 {
		if it := p.Item(p.Equip[SlotArmor]); it != nil && !it.Broken() {
			switch it.Def(e.Items).Skill {
			case SkillChain:
				if Mastery(p.Skills[SkillChain]) >= 4 {
					amount = int32(float64(amount) * float64(chainGMFactor))
				}
			case SkillPlate:
				if Mastery(p.Skills[SkillPlate]) >= 3 {
					amount = int32(float64(amount) * float64(plateMasterFactor))
				}
			}
		}
	}
	return amount
}

// ReceiveDamage hurts member i: it wakes them, then (unless pain reflection stops
// physical damage) takes IncomingDamage off their HP. At 0 HP or below they fall
// unconscious, or die when HP + Endurance (base and items) is below 1 without
// Preservation. Going to -10 HP or below breaks the worn armour unless it is hardened;
// on dying, the original tests the low dword of the game time instead (its register
// is reused), which this keeps. A member who can still act cries out (0x18). Returns
// the damage done.
//
// mm8: 0x48ec51 (Player_ReceiveDamage)
func (m *Members) ReceiveDamage(i int, amount int32, typ int, c *Ctx) int32 {
	p := &m.Players[i]
	p.Conditions[CondAsleep] = 0
	if typ == DamagePhysical && p.Buffs[BuffPainReflect].Active() {
		return 0
	}
	e := m.Env(c)
	dmg := p.IncomingDamage(e, typ, amount, c.Rand)
	p.HP -= dmg
	if p.HP < 1 {
		breakArmor := p.HP <= -10
		if p.BaseStat(e, StatEndurance)+int(p.HP) < 1 && !p.Buffs[BuffPreservation].Active() {
			m.SetCondition(i, CondDead, false, c)
			breakArmor = uint32(m.Time) != 0
			p.HP = min(p.HP, 0)
		} else {
			m.SetCondition(i, CondUnconscious, false, c)
		}
		if n := p.Equip[SlotArmor]; breakArmor && n != 0 {
			if it := p.Item(n); it != nil && it.Flags&items.FlagHardened == 0 {
				it.Flags |= items.FlagBroken
			}
		}
	}
	if dmg != 0 && p.CanAct() {
		m.Speak(i, 0x18, c)
	}
	return dmg
}

// Heal gives a member who is not dead or eradicated amount HP, up to their maximum; one
// back above 0 HP wakes from unconsciousness.
//
// mm8: 0x48ebf7
func (p *Player) Heal(e *Env, amount int32) {
	if p.Conditions[CondDead] != 0 || p.Conditions[CondEradicated] != 0 {
		return
	}
	p.HP = min(p.HP+amount, p.MaxHP(e))
	if p.Conditions[CondUnconscious] != 0 && p.HP > 0 {
		p.Conditions[CondUnconscious] = 0
	}
}

// SetRecovery makes member i wait at least ticks before acting; the selected member
// gives way to the next one (unless a selection is parked in 0x51d764, as while a
// spell picks its target; M9).
//
// mm8: 0x48fbfd
func (m *Members) SetRecovery(i int, ticks int) {
	p := &m.Players[i]
	ticks = max(ticks, 0)
	if ticks > p.Recovery {
		p.Recovery = ticks
	}
	if m.Selected == i+1 {
		m.Selected = m.NextSelectable()
	}
}

// Fall damage constants: a tenth of the maximum HP per 256 units fallen; the recovery
// is (20 - StatToBonus(Endurance)) * recmod1 * 2.1333 ticks, recmod1 being the ini's
// [debug] recmod1, "1.0" unless set (0x6f38fc, read at 0x46450a).
const (
	fallHPFactor = 0.1                // 0x4ea490 (double)
	fallRecovery = 2.1333333333333333 // 0x4ea438 (double)
	recMod1      = float32(1.0)
)

// FallDamage hurts every member after a fall of drop units (the caller checks the
// height): a member wearing an item of special 0x48 (feather falling) or artifact
// 0x20a is spared; the others take ftol(MaxHP * 0.1) * drop / 256 physical damage and
// a recovery. The rand() picks the landing cry (M11).
//
// mm8: 0x472e86, 0x473fee (Party_MoveIndoor/Outdoor, the landing)
func (m *Members) FallDamage(drop int32, c *Ctx) {
	e := m.Env(c)
	for i := range m.Players {
		p := &m.Players[i]
		if p.HasSpecial(0x48) || p.Wears(0x20a, NumSlots) {
			continue
		}
		c.Rand.Int() // sound 0x6c, 0x6d, 0x6e or 0x2c
		dmg := int32(float64(p.MaxHP(e))*fallHPFactor) * drop / 0x100
		m.ReceiveDamage(i, dmg, DamagePhysical, c)
		b := e.Classes.StatToBonus(p.ActualStat(e, StatEndurance))
		m.SetRecovery(i, int(float64(float64(20-b)*float64(recMod1))*fallRecovery))
	}
}

// HPRegen and SPRegen: the worn items that regenerate (special bonuses and artifacts).
//
// mm8: 0x493a34 (Party_Regen)
func (p *Player) regenItems(e *Env) (hp, sp bool) {
	for s := 0; s < NumSlots; s++ {
		it := p.Equipped(s)
		if it == nil {
			continue
		}
		if it.Number < 0x98 {
			switch it.Special {
			case 0x25, 0x2c, 0x32, 0x36:
				hp = true
			case 0x26, 0x2f, 0x37:
				sp = true
			case 0x42:
				hp, sp = true, true
			}
			continue
		}
		switch it.Number {
		case 0x1fd, 0x208:
			hp = true
		case 0x201:
			sp = true
		}
	}
	return hp, sp
}

// regenHP adds n HP to a member who is not dead or eradicated, up to the maximum, and
// wakes them from unconsciousness above 0 HP.
func (p *Player) regenHP(e *Env, n int32) {
	if p.Conditions[CondDead] != 0 || p.Conditions[CondEradicated] != 0 {
		return
	}
	p.HP += n
	if mx := p.MaxHP(e); mx < p.HP {
		p.HP = mx
	}
	if p.Conditions[CondUnconscious] != 0 && p.HP > 0 {
		p.Conditions[CondUnconscious] = 0
	}
}

// RegenMembers is Party_Regen's work on the members (once per call, however many
// five-minute periods passed): regenerating items give 1 HP and 1 spell point, the
// Regeneration buff its power in HP, the Regeneration skill its rank in HP. The fly and
// water walk upkeep and Immolation are M9's.
//
// mm8: 0x493a34 (Party_Regen)
func (m *Members) RegenMembers(c *Ctx) {
	e := m.Env(c)
	for i := range m.Players {
		p := &m.Players[i]
		hp, sp := p.regenItems(e)
		if hp {
			p.regenHP(e, 1)
		}
		if b := &p.Buffs[BuffRegeneration]; b.Active() {
			p.regenHP(e, int32(b.Power))
		}
		if r := p.Skill(e, SkillRegeneration); r != 0 {
			p.regenHP(e, int32(Mastery(r)))
		}
		if sp {
			p.SP++
			if mx := p.MaxSP(e); mx < p.SP {
				p.SP = mx
			}
		}
	}
}

// checkVitals is the per-member HP test of Party_UpdateTime: dead when HP + Endurance
// (base and items) is below 1 without Preservation, else unconscious at 0 HP or below.
//
// mm8: 0x494007 (Party_UpdateTime)
func (m *Members) checkVitals(i int, c *Ctx) {
	p := &m.Players[i]
	e := m.Env(c)
	switch {
	case p.BaseStat(e, StatEndurance)+int(p.HP) < 1 && !p.Buffs[BuffPreservation].Active():
		m.SetCondition(i, CondDead, false, c)
	case p.HP < 1:
		m.SetCondition(i, CondUnconscious, false, c)
	}
}

// StatHooks are the TimeHooks the stats make real: the maximum HP and spell points,
// starving, the HP checks and regeneration. The buffs, hazards and the daily reset
// stay NoTimeHooks' until M8/M9.
type StatHooks struct {
	NoTimeHooks
	M *Members
	C *Ctx
}

func (h StatHooks) MaxHP(p *Player) int32 { return p.MaxHP(h.M.Env(h.C)) }
func (h StatHooks) MaxSP(p *Player) int32 { return p.MaxSP(h.M.Env(h.C)) }

// Starve leaves a member without food HP / (days + 1) + 1.
//
// mm8: 0x494007 (Party_UpdateTime, the 3 AM rollover)
func (h StatHooks) Starve(p *Player, days int) { p.HP = p.HP/int32(days+1) + 1 }

func (h StatHooks) Regen(m *Members, n int) { m.RegenMembers(h.C) }

func (h StatHooks) Vitals(m *Members, i int) { m.checkVitals(i, h.C) }
