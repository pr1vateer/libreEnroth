package monsters

import (
	"libre-enroth/internal/game/party"
	"libre-enroth/internal/game/physics"
)

// A member's blow (re/notes/combat.md). Bows, wands and a dragon's breath are queued
// as projectiles (re/notes/spells.md); this is the melee blow itself.

// MeleeReach is the distance a blow still connects (0x42e27f).
const MeleeReach = 407.2

// HitsFromParty is the to-hit roll of a member's blow or shot. acClass is 0 for a
// blow and a close shot (the roll must reach AC + 15), 2 for a shot from 0xa00 on
// (AC + (AC+15)/2 + 15) and 3 for a harder one (AC*2 + 30). bonus is added to the
// roll (a blaster's skill). A hit is a roll that is not under the threshold.
//
// The armour class is halved while buff 21 is up. Hour of Power (buff 13) adds
// Shield's power (buff 14) — the bytes at 0x425f69 read 0x1c4, not Hour of Power's
// own power, which a monster's to-hit does read. Stoneskin then replaces that when
// it is higher. Quirk kept (re/notes/combat.md).
//
// mm8: 0x425f2b
func (b *Brain) HitsFromParty(p *party.Player, a *Actor, acClass, bonus int) bool {
	if p == nil || a == nil || b.Party.Ctx == nil {
		return false
	}
	ac := a.Info.AC
	if a.Active(BuffACHalved) {
		ac /= 2
	}
	extra := int32(0)
	if a.Active(BuffHourOfPower) {
		extra = int32(a.Buffs[BuffShield].Power)
	}
	if a.Active(BuffStoneskin) && int32(a.Buffs[BuffStoneskin].Power) > extra {
		extra = int32(a.Buffs[BuffStoneskin].Power)
	}
	ac += extra
	e := b.Party.Members.Env(b.Party.Ctx)
	attack := p.RangedAttack(e)
	if acClass == 0 {
		attack = p.MeleeAttack(e, false)
	}
	mod := int(ac + 30 + int32(attack)*2)
	if mod < 1 {
		mod = 1 // a non-positive modulus crashes; the original divides by it
	}
	roll := b.rand() % mod
	roll += bonus
	threshold := int(ac + 15)
	switch acClass {
	case 2:
		threshold = int(ac + (ac+15)/2 + 15)
	case 3:
		threshold = int(ac*2 + 30)
	}
	return roll >= threshold
}

// SavingThrow reports whether an effect of damage type typ gets past an actor's
// resistance: rand() % (level/4 + 30 + resistance) is under 30. Immune resistance
// (65000) always saves. A type with no resistance (magic, and anything else) never
// saves.
//
// mm8: 0x4262b2
func (b *Brain) SavingThrow(a *Actor, typ int) bool {
	res, ok := savingResist(a, typ)
	if !ok {
		return true
	}
	if res >= 65000 {
		return false
	}
	n := int(a.Info.Level>>2) + 30 + res
	if n < 1 {
		n = 1
	}
	return b.rand()%n < 30
}

// savingResist is the monsters.txt resistance a saving throw of this damage type
// reads (0x4262b2's switch). The bool is false when the type has no case.
func savingResist(a *Actor, typ int) (int, bool) {
	idx := -1
	switch typ {
	case 0:
		idx = 0
	case 1:
		idx = 1
	case 2:
		idx = 2
	case 3:
		idx = 3
	case 4:
		idx = 9
	case 6:
		idx = 5
	case 7:
		idx = 4
	case 8:
		idx = 6
	case 9:
		idx = 7
	case 10:
		idx = 8
	default:
		return 0, false
	}
	if idx < 0 || idx >= len(a.Info.Resist) {
		return 0, false
	}
	return int(a.Info.Resist[idx]), true
}

// SetBuff sets an actor's spell buff when the new expiry is later than the one it
// has (or it has none). power, skill and overlay are the buff's fields.
//
// mm8: 0x4571f2 (SpellBuff_Set)
func (bf *Buff) SetBuff(expires int64, power, skill, overlay int16, caster uint8) bool {
	if bf.Expires != 0 && expires <= bf.Expires {
		return false
	}
	bf.Expires = expires
	bf.Power = power
	bf.Skill = skill
	bf.Overlay = overlay
	bf.Caster = caster
	return true
}

// HitFromParty is a member's blow on actor ti, along v (a 16.16 unit vector, toward
// the actor). A miss, a swing that Pain Reflection turns aside, or a blow of no
// damage that neither a staff nor a mace nor an item of special 0x18 stunned, does
// nothing but a grunt. Otherwise the actor takes the damage (its resistances, a
// weapon's element, hammerhands), may die or flinch, and is knocked back.
//
// mm8: 0x4378ec (Damage_ActorFromParty, the melee path)
func (b *Brain) HitFromParty(member, ti int, v [3]int32) {
	m := b.Party.Members
	c := b.Party.Ctx
	if m == nil || c == nil || member < 0 || member >= len(m.Players) || ti < 0 || ti >= len(b.Actors) {
		return
	}
	p := &m.Players[member]
	a := &b.Actors[ti]
	if a.Gone(true) {
		return
	}
	a.Flags |= FlagNearParty | FlagActive
	if a.AIState == Flee {
		a.Flags |= FlagFledHit
	}
	if c.Rand != nil {
		b.Rand = c.Rand
	}
	e := m.Env(c)
	// The procs and the damage dice are rolled before the to-hit check, and discarded
	// when the blow misses (0x4378ec).
	stun, paralyse := weaponProcs(p, e, c.Rand)
	dmg := int32(p.RollMeleeDamage(e, int(int16(a.Info.ID)), IsKind, false, false, c.Rand))
	if p.Buffs[party.BuffPainReflect].Active() {
		m.Speak(member, 0x34, c)
		return
	}
	if !b.HitsFromParty(p, a, 0, 0) {
		m.Speak(member, 0x34, c)
		return
	}
	if p.Conditions[party.CondWeak] != 0 {
		dmg >>= 1
	}
	if a.Active(BuffStoned) {
		dmg = 0
	}
	phys := b.ResistDamage(a, party.DamagePhysical, dmg)
	if p.Unarmed(e) && p.Buffs[party.BuffHammerhands].Active() {
		phys += b.ResistDamage(a, party.DamageBody, int32(p.Buffs[party.BuffHammerhands].Power))
	}
	dealt := phys
	typ := party.DamagePhysical
	if !p.Unarmed(e) {
		for _, slot := range []int{party.SlotOffhand, party.SlotMainHand} {
			it := p.Equipped(slot)
			if it == nil {
				continue
			}
			if it.Number == 0x20b && b.SavingThrow(a, 3) {
				a.Buffs[BuffSlowed].SetBuff(int64(m.Time)+0x500, 2, 2, 0, 0)
				a.Flags |= FlagHostile
				// the sparkle overlay (0x4a8471) is drawn with the other effects
			}
			et, ed, vamp := party.ElementDamage(it, c.Rand)
			if vamp && a.HP > 0 {
				heal := min(int32(a.HP)/5, phys/5)
				if phys != 0 && heal == 0 {
					heal = 1
				}
				p.HP = min(p.HP+heal, p.MaxHP(e))
			}
			resisted := b.ResistDamage(a, et, int32(ed))
			dealt += resisted
			if ed != 0 {
				typ = et
			}
		}
	}
	a.HP -= int16(dealt)
	if dealt == 0 && !stun && !p.HasItemSpecial(0x18) {
		m.Speak(member, 0x34, c)
		return
	}
	killed := a.HP < 1
	if killed {
		b.Die(ti)
		b.Aggro(ti, true)
		if a.Info.Exp != 0 {
			m.AwardExp(int(a.Info.Exp), c)
		}
		speech := 0x33
		if b.rand()%100 < 20 {
			speech = 1
			if a.Info.HP > 99 {
				speech = 2
			}
		}
		m.Speak(member, speech, c)
	} else {
		b.Stun(ti, uint32(member)<<3|KindParty, false)
		b.Aggro(ti, true)
	}
	b.announce(p.Name, a, killed, int(dealt)) // damage line: 0xa4 or 0xaf
	if a.Active(BuffPainReflection) && dealt != 0 {
		m.ReceiveDamage(member, dealt, typ, c)
	}
	knock := int32(0)
	if a.Info.HP != 0 {
		knock = min(phys*0x14/a.Info.HP, 10)
	}
	if (stun || p.HasItemSpecial(0x18)) && b.SavingThrow(a, 3) {
		add := 0x14
		knock = 10
		if !b.Party.TurnBased {
			add = 42 // ftol(1 * 42.6666...) at 0x4ea5e0; turn-based keeps the 20
		}
		a.Info.Recovery += int32(add)
		b.announceLine(p.Name, 0x27b, a, 0)
	}
	if paralyse && a.CanAct() && b.SavingThrow(a, 3) {
		sk := p.Skill(e, party.SkillMace)
		level := int(sk & party.SkillLevel)
		// ftol(level * 0x1e00 * float32(1/30)); 0x1e00/30 = 256
		dur := int64(level * 0x1e00)
		dur = int64(float32(dur) * float32(1.0/30))
		a.Buffs[BuffParalysed].SetBuff(int64(m.Time)+dur, 0, int16(party.Mastery(sk)), 0, 0)
		b.announceLine(p.Name, 0x27c, a, 0)
	}
	if !IsKind(int(int16(a.Info.ID)), 4) {
		for n := range 3 {
			a.Vel[n] = int16(physics.Mul16(v[n], knock)) * 0x32
		}
	}
	b.HitEffect(ti, 1, phys)
}

// weaponProcs is a master staff's or mace's chance to stun (the skill level in 100)
// and a grandmaster mace's second chance to paralyse. Each chance draws one rand.
func weaponProcs(p *party.Player, e *party.Env, rng interface{ Int() int }) (stun, paralyse bool) {
	if rng == nil {
		return false, false
	}
	try := func(skill int, mastery int) bool {
		if party.Mastery(p.Skills[skill]) <= mastery {
			return false
		}
		return rng.Int()%100 < int(p.Skill(e, skill)&party.SkillLevel)
	}
	it := p.Equipped(party.SlotMainHand)
	if it == nil || e.Items == nil {
		return false, false
	}
	switch it.Def(e.Items).Skill {
	case party.SkillStaff:
		stun = try(party.SkillStaff, 2)
	case party.SkillMace:
		stun = try(party.SkillMace, 2)
		paralyse = try(party.SkillMace, 3)
	}
	return stun, paralyse
}

// HitEffect is the blow's spark. The overlay is drawn with the other effects (M9g);
// until then the call only records that the original would show one.
//
// mm8: 0x4394c9 (Actor_HitEffect)
func (b *Brain) HitEffect(i, kind int, amount int32) {}

// announce shows the hit line when something is listening (ShowDamage defaults on).
// 0xa4 is "%s hits %s for %lu damage"; a kill is 0xaf, "%s inflicts %lu points killing %s".
func (b *Brain) announce(attacker string, a *Actor, killed bool, dmg int) {
	id := 0xa4
	if killed {
		id = 0xaf
	}
	b.announceLine(attacker, id, a, dmg)
}

func (b *Brain) announceLine(attacker string, id int, a *Actor, dmg int) {
	h, ok := b.Host.(interface {
		Announce(id int, attacker, target string, dmg int)
	})
	if !ok {
		return
	}
	h.Announce(id, attacker, actorName(b, a), dmg)
}

func actorName(b *Brain, a *Actor) string {
	if a.UniqueName != 0 && int(a.UniqueName) < len(b.Tables.PlaceMon) {
		return b.Tables.PlaceMon[a.UniqueName]
	}
	if in := b.Tables.Info(a.ID()); in != nil {
		return in.Name
	}
	return ""
}

// MeleeDistance is how far the actor's surface is from the party (never below 0).
func MeleeDistance(px, py, pz int32, a *Actor) int32 {
	dx := int32(a.Pos[0]) - px
	dy := int32(a.Pos[1]) - py
	dz := int32(a.Pos[2]) - pz
	d := int32(physics.Isqrt(uint32(dx*dx+dy*dy+dz*dz))) - int32(int16(a.Radius))
	if d < 0 {
		return 0
	}
	return d
}

// BlowDirection is the 16.16 unit vector from the party to the actor.
func BlowDirection(px, py, pz int32, a *Actor) [3]int32 {
	return normalize(int32(a.Pos[0])-px, int32(a.Pos[1])-py, int32(a.Pos[2])-pz)
}
