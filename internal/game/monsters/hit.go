package monsters

import (
	"libre-enroth/internal/game/items"
	"libre-enroth/internal/game/party"
	"libre-enroth/internal/game/physics"
)

// Melee blows (re/notes/monsters.md, "Melee"): a melee attack whose animation ends
// queues a blow (AttackList_Add); at the end of the world tick each blow lands on its
// attacker's target when that is still within reach and in sight: an actor
// (Actor_HitActor) or a party member the monster prefers (Actor_HitMember).

// Attack types (the AttackList's and the blows'): the two attacks, the two spells,
// the explosion special.
const (
	AttackFirst   = 0
	AttackSecond  = 1
	AttackSpell1  = 2
	AttackSpell2  = 3
	AttackExplode = 4
)

const maxAttacks = 100

type attack struct {
	pid     uint32
	reach   int32
	x, y, z int32
	typ     int
	single  bool // flag 1: the attacker's target only; else everyone in reach (M9)
}

// attackList is g_attackList 0x5214c8: 100 pids, x, y, z, reaches, flags and types.
type attackList struct {
	n int
	a [maxAttacks]attack
}

// add queues a blow.
//
// mm8: 0x40276d (AttackList_Add)
func (l *attackList) add(pid uint32, reach, x, y, z int32, typ, flags int) {
	if l.n >= maxAttacks {
		return
	}
	l.a[l.n] = attack{pid: pid, reach: reach, x: int32(int16(x)), y: int32(int16(y)), z: int32(int16(z)), typ: typ, single: flags&1 != 0}
	l.n++
}

// Pending is the number of queued blows.
func (b *Brain) Pending() int { return b.attacks.n }

// Strike lands the queued blows. An actor target that can act (or is paralysed) is hit
// within reach + its radius of the blow's point, seen from 0x32 above its feet; the
// party within reach + 0x20 (its height, in the original's precedence slip, shifted
// by 1 - the blow's z), seen from its eyes. The list empties.
//
// mm8: 0x4373e5 (AttackList_Process)
func (b *Brain) Strike() {
	for k := range b.attacks.n {
		at := &b.attacks.a[k]
		if !at.single || at.pid&7 != KindActor {
			b.stub("area", "AttackList_Process 0x4373e5: area blows and object attackers (M9)")
			continue
		}
		ai := int(at.pid >> 3)
		if ai >= len(b.Actors) {
			continue
		}
		target := b.targets[ai]
		switch target & 7 {
		case KindActor:
			ti := int(target >> 3)
			t := &b.Actors[ti]
			if !t.Active(BuffParalysed) && !t.CanAct() {
				continue
			}
			r := at.reach + int32(int16(t.Radius))
			dx, dy := int32(t.Pos[0])-at.x, int32(t.Pos[1])-at.y
			dz := int32(int16(t.Height)>>1) - at.z + int32(t.Pos[2])
			if uint32(dz*dz+dy*dy+dx*dx) >= uint32(r*r) ||
				!b.CanSeePoints(int32(t.Pos[0]), int32(t.Pos[1]), int32(t.Pos[2])+0x32, at.x, at.y, at.z) {
				continue
			}
			b.HitActor(at.pid, ti, normalize(dx, dy, dz), at.typ)
		case KindParty:
			p := &b.Party
			dx, dy := p.X-at.x, p.Y-at.y
			dz := (p.Height + p.Z) >> (uint32(1-at.z) & 31)
			r := at.reach + 0x20
			if uint32(dz*dz+dy*dy+dx*dx) >= uint32(r*r) ||
				!b.CanSeePoints(p.X, p.Y, p.EyeLevel+p.Z, at.x, at.y, at.z) {
				continue
			}
			b.HitMember(ai, at.typ, b.ChooseMember(&b.Actors[ai]))
		}
	}
	b.attacks.n = 0
}

// normalize scales a vector to 16.16 unit length.
//
// mm8: 0x43917b
func normalize(x, y, z int32) [3]int32 {
	d := int32(physics.Isqrt(uint32(x*x+z*z+y*y)) | 1)
	inv := int32(0x10000 / int64(d))
	return [3]int32{x * inv, y * inv, z * inv}
}

// unconscious reports the conditions that keep a member from being picked: paralysed,
// unconscious, dead, stoned, eradicated.
func pickable(p *party.Player) bool {
	for _, c := range [...]party.Condition{party.CondParalyzed, party.CondUnconscious, party.CondDead,
		party.CondStoned, party.CondEradicated} {
		if p.Conditions[c] != 0 {
			return false
		}
	}
	return true
}

// prefClasses are the class pairs of the Pref letters' bits 1..0x80; 0x100 picks the men,
// 0x200 the women.
var prefClasses = [8][2]int{{0, 1}, {2, 3}, {4, 5}, {6, 7}, {8, 9}, {10, 11}, {12, 13}, {14, 15}}

// ChooseMember picks the member a monster's blow falls on: of those standing, one its
// Pref letters name (the last letter that names anyone decides), else any.
//
// mm8: 0x425ac2 (Actor_ChooseMember)
func (b *Brain) ChooseMember(a *Actor) int {
	m := b.Party.Members
	var list []int
	if pref := a.Info.Pref; pref != 0 {
		for k := range 15 {
			bit := pref & (1 << k)
			if bit == 0 {
				continue
			}
			c1, c2, sex := -1, -1, -1
			switch {
			case bit <= 0x80:
				for j := range prefClasses {
					if bit == 1<<j {
						c1, c2 = prefClasses[j][0], prefClasses[j][1]
					}
				}
			case bit == 0x100:
				sex = 0
			case bit == 0x200:
				sex = 1
			}
			list = list[:0]
			for i := range m.Players {
				p := &m.Players[i]
				female := 0
				if party.IsFemale(p.Face) {
					female = 1
				}
				if (p.Class == c1 || p.Class == c2 || female == sex) && pickable(p) {
					list = append(list, i)
				}
			}
		}
		if len(list) > 0 {
			return list[b.rand()%len(list)]
		}
	}
	list = list[:0]
	for i := range m.Players {
		if pickable(&m.Players[i]) {
			list = append(list, i)
		}
	}
	if len(list) == 0 {
		return 0
	}
	return list[b.rand()%len(list)]
}

// buffPower is buff k's power while it is on, else 0.
func (a *Actor) buffPower(k int) int32 {
	if a.Active(k) {
		return int32(uint16(a.Buffs[k].Power))
	}
	return 0
}

// toHitBonus is an attacker's to-hit bonus: Hour of Power or Bless, whichever is more,
// plus Fate, which is used up.
func (a *Actor) toHitBonus() int32 {
	bonus := a.buffPower(BuffHourOfPower)
	if p := a.buffPower(BuffBless); p > bonus {
		bonus = p
	}
	if a.Active(BuffFate) {
		bonus += a.buffPower(BuffFate)
		a.Buffs[BuffFate].Clear()
	}
	return bonus
}

// HitsMember is a monster's to-hit roll against a member's armour class.
//
// mm8: 0x4260f3 (Actor_HitsPlayer)
func (b *Brain) HitsMember(a *Actor, p *party.Player) bool {
	bonus := a.toHitBonus()
	e := b.Party.Members.Env(b.Party.Ctx)
	ac := int32(p.AC(e))
	return ac+5 < int32(b.rand())%(ac+10+int32(a.Info.Level)*2)+1+bonus
}

// HitsActor is a monster's to-hit roll against another: its armour class (halved by
// buff 21) plus Hour of Power or Stoneskin.
//
// mm8: 0x425ff7 (Actor_HitsActor)
func (b *Brain) HitsActor(a, t *Actor) bool {
	ac := t.Info.AC
	if t.Active(21) {
		ac /= 2
	}
	def := t.buffPower(BuffHourOfPower)
	if p := t.buffPower(BuffStoneskin); p > def {
		def = p
	}
	bonus := a.toHitBonus()
	return ac+def+5 < int32(b.rand())%(ac+def+10+int32(a.Info.Level)*2)+1+bonus
}

// Dice is n rolls of 1..sides (0 without sides).
//
// mm8: 0x451720 (Dice_Roll)
func (b *Brain) Dice(n, sides int) int32 {
	if sides == 0 {
		return 0
	}
	var s int32
	for range max(n, 0) {
		s += int32(1 + b.rand()%sides)
	}
	return s
}

// BlowDamage is the damage of an attack type: the first attack's dice (halved by buff
// 23) plus its bonus and the strongest of Hour of Power and Heroism plus Hammerhands;
// the second attack's dice plus its bonus; a spell's damage; the explosion's dice. A
// shrunk attacker's dice are divided by the shrink power; at least 1.
//
// mm8: 0x439b44 (Actor_BlowDamage)
func (b *Brain) BlowDamage(a *Actor, typ int) int32 {
	in := &a.Info
	var add, dmg int32
	shrink := func() {
		if p := a.buffPower(BuffShrink); p != 0 {
			dmg /= p
		}
		dmg = max(dmg, 1)
	}
	switch typ {
	case AttackFirst:
		bonus := a.buffPower(BuffHourOfPower)
		if p := a.buffPower(BuffHeroism); p > bonus {
			bonus = p
		}
		bonus += a.buffPower(BuffHammerhands)
		dmg = b.Dice(int(in.Attack1.Dice), int(in.Attack1.Sides))
		if a.Active(BuffHalfDamage) {
			dmg /= 2
		}
		shrink()
		return bonus + int32(in.Attack1.Add) + dmg
	case AttackSecond:
		dmg = b.Dice(int(in.Attack2.Dice), int(in.Attack2.Sides))
		add = int32(in.Attack2.Add)
	case AttackSpell1, AttackSpell2:
		spell, skill := in.Spell1, in.Spell1Skill
		if typ == AttackSpell2 {
			spell, skill = in.Spell2, in.Spell2Skill
		}
		return b.SpellDamage(int(spell), int(skill&0x3f), party.Mastery(skill), 0)
	case AttackExplode:
		dmg = b.Dice(int(in.SpecialA), int(in.SpecialB))
		add = int32(in.SpecialC)
	default:
		return 0
	}
	shrink()
	return add + dmg
}

// SpellDamage is a damage spell's roll at a skill level and mastery (maxHP for Mass
// Distortion's share of the target's).
//
// mm8: 0x43962c (Spell_Damage)
func (b *Brain) SpellDamage(spell, level, mastery int, maxHP int32) int32 {
	byMastery := func(sides [5]int, add bool) int32 {
		if mastery < 1 || mastery > 4 || sides[mastery] == 0 {
			return 0
		}
		d := b.Dice(level, sides[mastery])
		if add {
			d += int32(sides[mastery])
		}
		return d
	}
	switch spell {
	case 0x34: // Spirit Lash
		return b.Dice(2, 4)*int32(level) + 10
	case 7: // Fire Spike
		return byMastery([5]int{0, 8, 8, 8, 10}, false)
	case 10: // Inferno
		return int32(level*2 + 12)
	case 0x2b:
		return int32(level*2 + 20)
	case 0x2c: // Mass Distortion
		return (int32(b.Spells.At(0x2c).Add) + int32(level*2)) * maxHP / 100
	case 0x6f:
		return byMastery([5]int{0, 3, 3, 5, 7}, true)
	case 0x7b:
		return byMastery([5]int{0, 10, 10, 11, 12}, true)
	case 0x88:
		return byMastery([5]int{0, 4, 6, 8, 10}, false)
	case 0x89:
		return b.Dice(level, 10) + 10
	}
	s := b.Spells.At(spell)
	return b.Dice(level, int(s.Sides)) + int32(s.Add)
}

// damageType is the damage type of an attack: the attack's, the spell's school, the
// explosion's; physical otherwise. spells is whose spells the spell types read.
func (b *Brain) damageType(a, spells *Actor, typ int) int {
	switch typ {
	case AttackFirst:
		return int(a.Info.Attack1.Type)
	case AttackSecond:
		return int(a.Info.Attack2.Type)
	case AttackSpell1:
		return int(b.Spells.At(int(spells.Info.Spell1)).School)
	case AttackSpell2:
		return int(b.Spells.At(int(spells.Info.Spell2)).School)
	case AttackExplode:
		return int(int16(a.Info.SpecialD))
	}
	return party.DamagePhysical
}

// resistIndex is the monsters.txt resistance a damage type meets, and whether Hour of
// Power adds to it; -1: none.
var resistOf = map[int]struct {
	i     int
	power bool
}{
	0: {0, true}, 1: {1, true}, 2: {2, true}, 3: {3, true}, 4: {9, false}, 6: {5, false},
	7: {4, true}, 8: {6, true}, 9: {7, false}, 10: {8, false},
}

// ResistDamage is how much of dmg of a damage type gets through to an actor: immune
// (65000) takes none; otherwise it is halved up to four times, each time
// rand() % (resistance + 30 (+ Hour of Power)) is 30 or more.
//
// mm8: 0x4261b5 (Actor_ResistDamage)
func (b *Brain) ResistDamage(a *Actor, typ int, dmg int32) int32 {
	var res, bonus int32
	if r, ok := resistOf[typ]; ok {
		res = int32(a.Info.Resist[r.i])
		if r.power {
			bonus = a.buffPower(BuffHourOfPower)
		}
	}
	if res >= 65000 {
		return 0
	}
	n := max(bonus+30+res, 1)
	out := dmg
	for k := int32(1); k <= 4; k++ {
		if int32(b.rand())%n <= 29 {
			break
		}
		out = dmg >> k
	}
	return out
}

// Aggro turns actor i's kind within 4096 hostile (byParty: hostile to the party too,
// itself included, but not the unallied ones).
//
// mm8: 0x439278 (Actor_AggroAllies)
func (b *Brain) Aggro(i int, byParty bool) {
	a := &b.Actors[i]
	if byParty {
		a.Flags |= FlagHostile
	}
	for j := range b.Actors {
		t := &b.Actors[j]
		if j == i || !t.CanAct() || !IsAlly(a, t) || byParty && t.Ally == 9999 {
			continue
		}
		d := approxDist(int32(t.Pos[0])-int32(a.Pos[0]), int32(t.Pos[1])-int32(a.Pos[1]), int32(t.Pos[2])-int32(a.Pos[2]))
		if float32(d) < 4096 {
			t.Info.Hostility = 4
			if byParty {
				t.Flags |= FlagHostile
			}
		}
	}
}

// HitMember is a monster's blow on member i: the to-hit roll against the member's AC,
// the damage (of the attack type, divided by a shrink) through ReceiveDamage; a member
// with buff 10 deals what got through back (the monster's resistances apply; it may
// die or be stunned); the monster's bonus effect on Bonus% = Level * xN; the member's
// recovery; a cry at a quarter of the HP.
//
// mm8: 0x438625 (Actor_HitMember, the actor case)
func (b *Brain) HitMember(ai, typ, member int) {
	m, c := b.Party.Members, b.Party.Ctx
	if m == nil || c == nil || c.Classes == nil || member < 0 || member >= len(m.Players) {
		return
	}
	a := &b.Actors[ai]
	p := &m.Players[member]
	hpBefore := p.HP
	if !b.HitsMember(a, p) {
		return
	}
	b.rand() // the hit sound: 0x69..0x6b, 0x2d on plate or chain, else 0x6c..0x6e, 0x2c (M11)
	dmg := b.BlowDamage(a, typ)
	if p := a.buffPower(BuffShrink); p != 0 {
		dmg /= p
	}
	typ2 := b.damageType(a, a, typ)
	dealt := m.ReceiveDamage(member, dmg, typ2, c)
	if p.Buffs[party.BuffReflect].Active() && a.AIState != Dead && a.AIState != Dying {
		back := b.ResistDamage(a, typ2, dealt)
		a.HP -= int16(back)
		if back != 0 {
			if a.HP < 1 {
				b.Die(ai)
				b.Aggro(ai, true)
				if a.Info.Exp != 0 {
					b.stub("exp", "Party_AwardExp 0x4255bb: experience for a kill (M9)")
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
				b.Stun(ai, uint32(member)<<3|KindParty, false)
				b.Aggro(ai, true)
			}
		}
	}
	if a.Info.AttackBonus != 0 && b.rand()%100 < int(a.Info.AttackBonusMul)*int(a.Info.Level) {
		m.MonsterBonus(member, int(a.Info.AttackBonus), c, func(it items.Item) bool { return a.takeItem(it) })
	}
	if !b.Party.TurnBased {
		m.MonsterHitRecovery(member, c)
	}
	if dmg != 0 {
		quarter := float64(p.MaxHP(m.Env(c))) * 0.25
		if quarter < float64(hpBefore) && p.HP > 0 && float64(p.HP) <= quarter {
			m.Speak(member, 0x30, c)
		}
	}
}

// takeItem puts a stolen item into the first of the actor's two item slots that is
// empty; false when both are taken.
func (a *Actor) takeItem(it items.Item) bool {
	for k := range 2 {
		if a.Items[k].Number == 0 {
			a.Items[k] = it
			return true
		}
	}
	return false
}

// HitActor is a monster's blow on another: the to-hit roll; the damage (shrink
// divides it, a stoned target takes none, Shield halves the second attack) through
// the target's resistances. A target that takes none is stunned; otherwise it dies or
// is stunned, its kind turns hostile, and it is knocked back along v by up to 10
// (damage * 20 / its full HP) * 50, unless it never moves. A spell blow reads the
// target's spells for the damage type, as the original does.
//
// mm8: 0x43990b (Actor_HitActor)
func (b *Brain) HitActor(src uint32, ti int, v [3]int32, typ int) {
	if src&7 != KindActor {
		return
	}
	ai := int(src >> 3)
	a, t := &b.Actors[ai], &b.Actors[ti]
	if t.Gone(true) {
		return
	}
	t.LastHitBy = int32(src)
	if t.AIState == Flee {
		t.Flags |= FlagFledHit
	}
	if !b.HitsActor(a, t) {
		return
	}
	dmg := b.BlowDamage(a, typ)
	if p := a.buffPower(BuffShrink); p != 0 {
		dmg /= p
	}
	if t.Active(BuffStoned) {
		dmg = 0
	}
	if typ == AttackSecond && t.Active(BuffShield) {
		dmg >>= 1
	}
	got := b.ResistDamage(t, b.damageType(a, t, typ), dmg)
	t.HP -= int16(got)
	if got == 0 {
		b.Stun(ti, src, false)
		return
	}
	if t.HP < 1 {
		b.Die(ti)
	} else {
		b.Stun(ti, src, false)
	}
	b.Aggro(ti, false)
	k := int32(0)
	if t.Info.HP != 0 {
		k = min(got*0x14/t.Info.HP, 10)
	}
	if !IsKind(int(t.Info.ID), 4) {
		for n := range 3 {
			t.Vel[n] = int16(physics.Mul16(v[n], k)) * 0x32
		}
	}
	// 0x4394c9 shows the hit's spark (M9).
}
