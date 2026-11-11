package monsters

import (
	"libre-enroth/internal/game/party"
	"libre-enroth/internal/game/physics"
)

// A projectile's end (re/notes/spells.md). Impact reports 1 when the object keeps
// flying and 0 when it stops. A party projectile that meets the party keeps flying,
// and an actor's projectile that meets an actor of relation 0 keeps flying.

// Impact is Object_Impact.
//
// mm8: 0x46bd3a
func (b *Brain) Impact(i int, pid uint32) int {
	objs := b.objects()
	if i < 0 || i >= len(objs) || objs[i].ObjList == 0 {
		return 0
	}
	o := &objs[i]
	if pid&7 == KindActor && o.Caster&7 == KindActor {
		ai, ti := int(o.Caster>>3), int(pid>>3)
		if ai >= 0 && ai < len(b.Actors) && ti >= 0 && ti < len(b.Actors) &&
			Relation(b.Tables, &b.Actors[ai], &b.Actors[ti]) == 0 {
			return 1
		}
	}
	if pid&7 == KindParty && o.Caster&7 == KindParty {
		return 1
	}
	if b.Party.TurnBased && o.Attrib&objTurnBased != 0 {
		o.Attrib &^= objTurnBased
	}
	switch kind := o.Type; {
	case kind == 545 || kind == 550:
		return b.arrowImpact(i, pid)
	case missileBolt(kind):
		b.directHit(i, pid)
		b.advanceSprite(i)
		return 0
	case kind == 600 || kind == 1050 || kind == 9080 || kind == 12010:
		b.areaBurst(i)
		return 0
	case kind == 3090:
		b.iceBlast(i)
		return 0
	case statusSpell(kind):
		b.statusImpact(i, pid)
		return 0
	default:
		return 0
	}
}

func missileBolt(t int16) bool {
	switch t {
	case 500, 505, 510, 515, 520, 525, 530, 535, 540, 555,
		1010, 1020, 1100, 2060, 4000, 8000, 11000:
		return true
	default:
		return false
	}
}

func statusSpell(t int16) bool {
	switch t {
	case 0x2724, 0x236e, 12000, 0x2b0c:
		return true
	default:
		return false
	}
}

// arrowImpact is a direct hit, unless the arrow is item 0x195 or special 3, which
// becomes the explosion (object 600) and waits for that object's impact.
func (b *Brain) arrowImpact(i int, pid uint32) int {
	o := &b.objects()[i]
	if o.Item.Number == 0x195 || o.Item.Special == 3 {
		o.Type = 600
		b.rebind(o)
		o.Vel = [3]int16{}
		o.Time = 0
		return 0
	}
	b.directHit(i, pid)
	removeObject(o)
	return 0
}

// directHit is Object_HitDirect: the caster's kind picks the damage function.
//
// mm8: 0x438f98
func (b *Brain) directHit(i int, pid uint32) {
	o := &b.objects()[i]
	v := normalize(int32(o.Vel[0]), int32(o.Vel[1]), int32(o.Vel[2]))
	switch pid & 7 {
	case KindActor:
		ti := int(pid >> 3)
		switch o.Caster & 7 {
		case KindActor:
			b.HitActor(uint32(o.Caster), ti, v, int(o.CasterAbility))
		case KindParty:
			b.hitFromObject(i, ti, v)
		}
	case KindParty:
		b.hitParty(i, -1)
	}
}

func (b *Brain) advanceSprite(i int) {
	o := &b.objects()[i]
	if o.Type != 555 {
		// Energy bolt 555 hits and does not play the usual end; every other bolt does.
	}
	o.Type++
	b.rebind(o)
	o.Vel = [3]int16{}
	o.Time = 0
}

func (b *Brain) rebind(o *Object) {
	if b.Tables == nil {
		o.ObjList = 0
	} else {
		o.ObjList = int16(b.Tables.ObjListOf(o.Type))
	}
	if o.ObjList == 0 {
		removeObject(o)
	}
}

// areaBurst stops the projectile, shows the next sprite and hits everyone inside
// 0x200. There is no further falloff.
//
// mm8: 0x46bd3a (object 600, fireball 1050, breath 9080, flame blast 12010)
func (b *Brain) areaBurst(i int) {
	o := &b.objects()[i]
	o.Type++
	b.rebind(o)
	if o.ObjList == 0 {
		return
	}
	o.Vel = [3]int16{}
	o.Time = 0
	b.attacks.add(uint32(i)<<3|KindObject, 0x200, o.Pos[0], o.Pos[1], o.Pos[2], 0, 0)
}

// iceBlast throws 7 shards, or 9 at grandmaster, an eighth of a turn apart, then
// removes the parent.
//
// mm8: 0x46bd3a (object 3090)
func (b *Brain) iceBlast(i int) {
	objs := b.objects()
	parent := objs[i]
	parent.Type += 2
	if b.Tables != nil {
		parent.ObjList = int16(b.Tables.ObjListOf(parent.Type))
	}
	n := 7
	if parent.SpellSkill == 4 {
		n = 9
	}
	yaw := int32(int16(parent.Yaw)) - physics.FullTurn
	for range n {
		yaw += physics.QuarterTurn >> 1
		shard := parent
		objs, _ = Launch(objs, shard, yaw, 0, 1000)
	}
	if b.Objects != nil {
		*b.Objects = objs
	}
	if i < len(b.objects()) {
		removeObject(&b.objects()[i])
	}
}

// statusImpact is Blind, Dark Grasp and Charm: an actor who fails the save, for
// the duration in that function. Anyone else just removes the object.
func (b *Brain) statusImpact(i int, pid uint32) {
	o := &b.objects()[i]
	if pid&7 == KindActor {
		ti := int(pid >> 3)
		if ti >= 0 && ti < len(b.Actors) {
			a := &b.Actors[ti]
			dur, buff, save, power := statusDur(o)
			if b.SavingThrow(a, save) && b.Party.Members != nil {
				a.Buffs[buff].SetBuff(int64(b.Party.Members.Time)+dur, power, int16(o.SpellSkill), 0, 0)
			}
		}
	}
	removeObject(o)
}

func statusDur(o *Object) (dur int64, buff, save int, power int16) {
	level := int64(o.SpellLevel)
	gm := o.SpellSkill == 4
	switch o.Type {
	case 0x2724: // level*300, *600 at grandmaster
		dur = level * 300
		if gm {
			dur = level * 600
		}
		return dur, 0x16, 7, 0
	case 0x236e:
		dur = level * 300
		if gm {
			dur = level * 600
		}
		return dur, 7, 10, 4
	case 12000:
		switch o.SpellSkill {
		case 1:
			dur = level * 0xb4
		case 2:
			dur = level * 300
		}
		return dur, 4, 7, 0
	default: // charm, 0x2b0c; grandmaster lasts 0x1baf800
		dur = level * 600
		if o.SpellSkill == 3 {
			dur = level * 0xe10
		}
		if gm {
			dur = 0x1baf800
		}
		return dur, BuffCharm, 7, 0
	}
}

// hitFromObject is the object branch of Damage_ActorFromParty.
//
// mm8: 0x4378ec
func (b *Brain) hitFromObject(obj, ti int, v [3]int32) {
	m, c := b.Party.Members, b.Party.Ctx
	objs := b.objects()
	if m == nil || c == nil || ti < 0 || ti >= len(b.Actors) || obj < 0 || obj >= len(objs) {
		return
	}
	o := &objs[obj]
	roster := int(o.Caster >> 3)
	member := -1
	if m != nil {
		for i := range m.Players {
			if m.Players[i].RosterID == roster {
				member = i
				break
			}
		}
	}
	if member < 0 || member >= len(m.Players) {
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
	ac := 1
	if o.Spell != 99 {
		d := approxDist3(b.Party.X-o.Pos[0], b.Party.Y-o.Pos[1], b.Party.Z-o.Pos[2])
		if d >= 0xa00 {
			if d >= 0x1400 && a.Flags&0x400 == 0 {
				return
			}
			ac = 2
		}
	}
	e := m.Env(c)
	typ := b.spellType(o, a)
	dmg := int32(0)
	auto := true
	element := false
	switch o.Spell {
	case 0x85:
		dmg = int32(p.RollRangedDamage(e, int(a.MonList), IsKind, c.Rand))
		auto = o.Item.Number != 0 && o.Item.Special == 3
		if auto {
			typ = party.DamageFire
		} else {
			typ = party.DamagePhysical
		}
		element = true
	case 0x86:
		dmg = int32(p.RollRangedDamage(e, 0, IsKind, c.Rand))
		typ = party.DamageFire
		auto = false
		element = true
	case 0x88:
		dmg = int32(p.RollRangedDamage(e, int(a.MonList), IsKind, c.Rand))
		typ = party.DamagePhysical
		auto = false
		element = true
		if a.Active(BuffShield) {
			dmg >>= 1
		}
	case 0x87:
		dmg = int32(p.RollMeleeDamage(e, int(int16(a.Info.ID)), IsKind, true, true, c.Rand))
		typ = party.DamagePhysical
		auto = false
		ac = 1
	default:
		dmg = b.SpellDamage(int(o.Spell), int(o.SpellLevel), int(o.SpellSkill), int32(a.HP))
		auto = true
	}
	if !auto && !b.HitsFromParty(p, a, ac, 0) {
		m.Speak(member, 0x34, c)
		return
	}
	if p.Conditions[party.CondWeak] != 0 {
		dmg >>= 1
	}
	if a.Active(BuffStoned) {
		dmg = 0
	}
	dealt := b.ResistDamage(a, typ, dmg)
	if element && o.Item.Number != 0 {
		et, ed, vamp := party.ElementDamage(&o.Item, c.Rand)
		if vamp && a.HP > 0 {
			heal := min(int32(a.HP)/5, dealt/5)
			if dealt != 0 && heal == 0 {
				heal = 1
			}
			p.HP = min(p.HP+heal, p.MaxHP(e))
		}
		res := b.ResistDamage(a, et, int32(ed))
		dealt += res
		if ed != 0 {
			typ = et
		}
	}
	if o.Spell == 0x6f && dealt != 0 {
		heal := min(int32(a.HP)/3, dealt/3)
		if heal == 0 {
			heal = 1
		}
		p.HP = min(p.HP+heal, p.MaxHP(e))
	}
	a.HP -= int16(dealt)
	if dealt == 0 {
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
		b.Stun(ti, uint32(obj)<<3|KindObject, false)
		b.Aggro(ti, true)
	}
	b.announce(p.Name, a, killed, int(dealt))
	if a.Active(BuffPainReflection) {
		m.ReceiveDamage(member, dealt, typ, c)
	}
	knock := int32(0)
	if a.Info.HP != 0 {
		knock = min(dealt*0x14/a.Info.HP, 10)
	}
	if !IsKind(int(int16(a.Info.ID)), 4) {
		for n := range 3 {
			a.Vel[n] = int16(physics.Mul16(v[n], knock)) * 0x32
		}
	}
	b.HitEffect(ti, 1, dealt)
}

func (b *Brain) spellType(o *Object, a *Actor) int {
	switch o.Spell {
	case 0x89:
		return 0x32
	case 0x67:
		if a.Info.Resist[0] < a.Info.Resist[8] {
			return party.DamageFire
		}
		return party.DamageDark
	default:
		return int(b.Spells.At(int(o.Spell)).School)
	}
}

// hitParty is a monster missile meeting the party: the actor's blow on the member
// it prefers, with an arrow's dodge and the halves a shield and some specials give.
//
// mm8: 0x438625 (the object branch)
func (b *Brain) hitParty(obj, member int) {
	o := &b.objects()[obj]
	if o.Caster&7 != KindActor {
		b.spellOnParty(obj, member)
		return
	}
	ai := int(o.Caster >> 3)
	m, c := b.Party.Members, b.Party.Ctx
	if m == nil || c == nil || ai < 0 || ai >= len(b.Actors) {
		return
	}
	if c.Rand != nil {
		b.Rand = c.Rand
	}
	a := &b.Actors[ai]
	if member < 0 {
		member = b.ChooseMember(a)
	}
	if member < 0 || member >= len(m.Players) {
		return
	}
	p := &m.Players[member]
	if o.Type == 545 {
		sk := p.Skill(m.Env(c), party.SkillUnarmed)
		if party.Mastery(sk) > 3 && b.rand()%100 < int(sk&party.SkillLevel) {
			m.Speak(member, 6, c)
			return
		}
	}
	if !b.HitsMember(a, p) {
		return
	}
	dmg := b.BlowDamage(a, int(o.CasterAbility))
	if p.Buffs[13].Active() {
		dmg >>= 1
	}
	e := m.Env(c)
	if p.HasSpecial(0x24) {
		dmg >>= 1
	}
	if p.HasSpecial(0x45) {
		dmg >>= 1
	}
	if sh := p.Equipped(party.SlotOffhand); sh != nil && e.Items != nil && sh.Def(e.Items).EquipType == 4 &&
		party.Mastery(p.Skills[party.SkillShield]) == 4 {
		dmg >>= 1
	}
	typ := b.damageType(a, a, int(o.CasterAbility))
	m.ReceiveDamage(member, dmg, typ, c)
}

// spellOnParty is a party projectile that was not caught by the friendly-fire
// early-out (an area blow's object, aimed at one member).
func (b *Brain) spellOnParty(obj, member int) {
	o := &b.objects()[obj]
	m, c := b.Party.Members, b.Party.Ctx
	if m == nil || c == nil {
		return
	}
	if c.Rand != nil {
		b.Rand = c.Rand
	}
	if member < 0 {
		var live []int
		for i := range m.Players {
			if m.Players[i].CanAct() {
				live = append(live, i)
			}
		}
		if len(live) == 0 {
			return
		}
		member = live[b.rand()%len(live)]
	}
	if member < 0 || member >= len(m.Players) {
		return
	}
	p := &m.Players[member]
	var dmg int32
	typ := int(b.Spells.At(int(o.Spell)).School)
	if o.Caster&7 == KindParty && o.Spell == 0x85 {
		roster := int(o.Caster >> 3)
		for i := range m.Players {
			if m.Players[i].RosterID == roster {
				dmg = int32(m.Players[i].RollRangedDamage(m.Env(c), 0, IsKind, c.Rand))
				break
			}
		}
		typ = party.DamageFire
	} else {
		dmg = b.SpellDamage(int(o.Spell), int(o.SpellLevel), int(o.SpellSkill), p.MaxHP(m.Env(c)))
	}
	m.ReceiveDamage(member, dmg, typ, c)
}

func approxDist3(dx, dy, dz int32) int32 {
	a, b, c := abs32(dx), abs32(dy), abs32(dz)
	if a < b {
		a, b = b, a
	}
	if a < c {
		a, c = c, a
	}
	if b < c {
		b, c = c, b
	}
	return a + c>>2 + (b*11)>>5
}
