package evt

import (
	"libre-enroth/internal/game/items"
	"libre-enroth/internal/game/party"
)

// The member variables of Compare, Add, Subtract and Set: the stats, resistances,
// skills, HP, spell points and the rest of a Player (re/notes/evt.md#variables).
const (
	VarSex          Var = 0x01
	VarClass        Var = 0x02
	VarHP           Var = 0x03
	VarHPFull       Var = 0x04 // Compare: HP >= max; Add/Set: HP = max
	VarSP           Var = 0x05
	VarSPFull       Var = 0x06
	VarAC           Var = 0x07
	VarACBonus      Var = 0x08
	VarLevel        Var = 0x09
	VarLevelBonus   Var = 0x0a
	VarAge          Var = 0x0b // Compare: the age; Add/Sub/Set: the age bonus
	VarAwards       Var = 0x0c
	VarExp          Var = 0x0d
	VarBonusFirst   Var = 0x19 // stat bonuses in struct order (Speed before Accuracy)
	VarBaseFirst    Var = 0x20 // base stats, struct order
	VarActualFirst  Var = 0x27 // actual stats (Add/Sub/Set change the bonus)
	VarResistFirst  Var = 0x2e // 0x2e..0x38 base resistances
	VarResBonFirst  Var = 0x39 // 0x39..0x43 resistance bonuses
	VarSkillFirst   Var = 0x44 // skills 0..38
	VarSkillLast    Var = 0x6a
	VarFullFirst    Var = 0xe2 // actual stat >= base + items, struct order
	VarPlayerBits   Var = 0xe9
	VarSpecialItems Var = 0xf4 // the party's items 0x1d6/0x1d7/0x1dd weighted 1/3/5
	VarSkillPoints  Var = 0xf5
)

// structStat maps the struct order of the stats (the variables' order) to stat codes.
var structStat = [7]party.Stat{party.StatMight, party.StatIntellect, party.StatPersonality,
	party.StatEndurance, party.StatSpeed, party.StatAccuracy, party.StatLuck}

// resistSlot maps the resistance variables (from 0x2e, and from 0x39 for the bonuses)
// to the damage type slots; -1 has no case.
var resistSlot = [11]int{party.DamageFire, party.DamageAir, party.DamageWater, party.DamageEarth,
	party.DamageSpirit, party.DamageMind, party.DamageBody, party.DamageLight, party.DamageDark,
	-1, party.DamageMagic}

// skillVar reports a skill variable with a case: 0x61..0x66 (Perception..ID Monster)
// have none in any of the four functions.
func skillVar(v Var) bool {
	return v >= VarSkillFirst && v <= VarSkillLast && (v < 0x61 || v > 0x66)
}

// playerVar reports a member variable handled here.
func playerVar(v Var) bool {
	switch {
	case v >= VarSex && v <= VarExp, v == VarItem, v >= VarBonusFirst && v < VarSkillFirst,
		skillVar(v), v >= VarFullFirst && v <= VarPlayerBits, v == VarSpecialItems,
		v == VarSkillPoints, v == VarEquipped:
		return true
	}
	return false
}

// env is the stat environment, nil when the tables are not loaded.
func (r *run) env() *party.Env {
	c := r.h.Ctx()
	if c.Items == nil || c.Classes == nil {
		return nil
	}
	return r.m.Env(c)
}

// resist returns the resistance (bonus) slot of v, nil without a case.
func resist(p *party.Player, v Var) *uint16 {
	switch {
	case v >= VarResistFirst && v < VarResBonFirst:
		if s := resistSlot[v-VarResistFirst]; s >= 0 {
			return &p.Resists[s]
		}
	case v >= VarResBonFirst && v < VarSkillFirst:
		if s := resistSlot[v-VarResBonFirst]; s >= 0 {
			return &p.ResistBonus[s]
		}
	}
	return nil
}

// comparePlayer is Compare of member variable v for member p: "current >= value"
// (signed) unless noted; -1 for a variable without a case.
//
// mm8: 0x4483d3 (Evt_Compare)
func (r *run) comparePlayer(p int, v Var, value uint32) bool {
	e := r.env()
	if p < 0 || e == nil {
		if e == nil {
			r.h.Stub(OpCompare, nil, v)
		}
		return false
	}
	m := r.m
	pl := &m.Players[p]
	val := int32(value)
	cur := int32(-1)
	switch {
	case v == VarSex:
		return boolU32(party.IsFemale(pl.Face)) == value
	case v == VarClass:
		return uint32(pl.Class) == value
	case v == VarHP:
		cur = pl.HP
	case v == VarHPFull:
		return pl.HP >= pl.MaxHP(e)
	case v == VarSP:
		cur = pl.SP
	case v == VarSPFull:
		return pl.SP >= pl.MaxSP(e)
	case v == VarAC:
		cur = int32(pl.AC(e))
	case v == VarACBonus:
		cur = int32(pl.ACBonus)
	case v == VarLevel:
		cur = int32(pl.LevelBase)
	case v == VarLevelBonus:
		cur = int32(pl.LevelBonus)
	case v == VarAge:
		cur = int32(pl.Age(e))
	case v == VarAwards:
		if pl.Awards.Get(int(int16(value))) {
			cur = val
		}
	case v == VarExp:
		cur = int32(pl.Exp)
	case v == VarItem:
		return m.HasItem(p, int32(value))
	case v >= VarBonusFirst && v < VarBaseFirst:
		cur = int32(pl.Stats[v-VarBonusFirst].Bonus)
	case v >= VarBaseFirst && v < VarActualFirst:
		cur = int32(pl.Stats[v-VarBaseFirst].Base)
	case v >= VarActualFirst && v < VarResistFirst:
		cur = int32(pl.ActualStat(e, structStat[v-VarActualFirst]))
	case v >= VarResistFirst && v < VarSkillFirst:
		if s := resist(pl, v); s != nil {
			cur = int32(*s)
		}
	case skillVar(v):
		sk := pl.Skills[v-VarSkillFirst]
		if val < 0x40 {
			cur = int32(sk & party.SkillLevel)
		} else {
			cur = int32(uint32(sk) & value)
		}
	case v >= VarFullFirst && v < VarPlayerBits:
		s := structStat[v-VarFullFirst]
		return pl.ActualStat(e, s) >= pl.BaseStat(e, s)
	case v == VarPlayerBits:
		if pl.Bits.Get(int(int16(value))) {
			cur = val
		}
	case v == VarSpecialItems:
		n := int32(0)
		for i := range m.Players {
			for _, it := range m.Players[i].Items {
				switch it.Number {
				case 0x1d6:
					n++
				case 0x1d7:
					n += 3
				case 0x1dd:
					n += 5
				}
			}
		}
		cur = n
	case v == VarSkillPoints:
		cur = pl.SkillPoints
	case v == VarEquipped:
		for s := 0; s < party.NumSlots; s++ {
			if it := pl.Equipped(s); it != nil && uint32(it.Number) == value {
				return true
			}
		}
		return false
	}
	return cur >= val
}

func boolU32(b bool) uint32 {
	if b {
		return 1
	}
	return 0
}

// addSkill is Add's skill rule: below 0x40 the value is or-ed into the level (the
// mastery bits drop); from 0x40 its low byte adds to the level, at most 60, the mastery
// kept.
//
// mm8: 0x449726 (Evt_Add, 0x449c35)
func addSkill(sk uint16, value uint32) uint16 {
	if value > 0x3f {
		lvl := min(uint32(sk&party.SkillLevel)+value&0xff, 0x3c)
		return sk&0xffc0 | uint16(lvl)
	}
	return uint16(uint8(sk)&0x3f) | uint16(uint8(value))
}

// setSkill is Set's skill rule: below 0x40 the value is or-ed into the level (the
// mastery bits drop); from 0x40 it is or-ed into the expert and master bits of the
// low byte, the level and the grandmaster bit dropped.
//
// mm8: 0x448d4b (Evt_Set, 0x4492df)
func setSkill(sk uint16, value uint32) uint16 {
	if value > 0x3f {
		return uint16(uint8(sk)&0xc0 | uint8(value))
	}
	return uint16(uint8(sk)&0x3f) | uint16(uint8(value))
}

// capByte caps a signed short at 255.
func capByte(v *int16) {
	if *v > 0xff {
		*v = 0xff
	}
}

// statPtr is the stat field of a stat variable: the bonus for 0x19..0x1f and
// 0x27..0x2d (Add, Subtract and Set change the bonus of the "actual" stats), the base
// for 0x20..0x26.
func statPtr(pl *party.Player, v Var) (f *int16, base bool) {
	switch {
	case v >= VarBonusFirst && v < VarBaseFirst:
		return &pl.Stats[v-VarBonusFirst].Bonus, false
	case v >= VarBaseFirst && v < VarActualFirst:
		return &pl.Stats[v-VarBaseFirst].Base, true
	case v >= VarActualFirst && v < VarResistFirst:
		return &pl.Stats[v-VarActualFirst].Bonus, false
	}
	return nil, false
}

// Reactions to stat changes: 0x5c for a base value, 0x5b for a bonus.
const (
	speechBase  = 0x5c
	speechBonus = 0x5b
)

// addPlayer is Add of member variable v for member p (in the party). The portrait's
// sparkle (0x97) and the sound are M9's and M11's.
//
// mm8: 0x449726 (Evt_Add)
func (r *run) addPlayer(p int, v Var, value uint32) {
	e := r.env()
	if e == nil {
		r.h.Stub(OpAdd, nil, v)
		return
	}
	m, ctx := r.m, r.h.Ctx()
	pl := &m.Players[p]
	switch {
	case v == VarClass:
		pl.Class = int(uint8(value))
	case v == VarHP:
		pl.HP += int32(value)
		pl.HP = min(pl.HP, pl.MaxHP(e))
	case v == VarHPFull:
		pl.HP = pl.MaxHP(e)
	case v == VarSP:
		pl.SP += int32(value)
		pl.SP = min(pl.SP, pl.MaxSP(e))
	case v == VarSPFull:
		pl.SP = pl.MaxSP(e)
	case v == VarACBonus, v == VarLevel, v == VarLevelBonus:
		f := map[Var]*int16{VarACBonus: &pl.ACBonus, VarLevel: &pl.LevelBase, VarLevelBonus: &pl.LevelBonus}[v]
		*f += int16(value)
		capByte(f)
	case v == VarAge:
		pl.AgeBonus += int16(value)
	case v == VarAwards:
		n := int(int16(value))
		if !pl.Awards.Get(n) && r.h.AwardText(int(value)) {
			m.Speak(p, 0x60, ctx)
		}
		pl.Awards.Set(n, true)
	case v == VarExp:
		pl.Exp += int64(int32(value))
		if pl.Exp > 4000000000 {
			pl.Exp = 4000000000
		}
	case v == VarItem:
		m.GiveItem(int32(value), true, ctx)
	case v >= VarBonusFirst && v < VarResistFirst:
		f, base := statPtr(pl, v)
		*f += int16(value)
		capByte(f)
		r.statSpeech(p, base)
	case v >= VarResistFirst && v < VarSkillFirst:
		if s := resist(pl, v); s != nil {
			*s += uint16(value)
			if *s > 0xff {
				*s = 0xff
			}
			r.statSpeech(p, v < VarResBonFirst)
		}
	case skillVar(v):
		pl.Skills[v-VarSkillFirst] = addSkill(pl.Skills[v-VarSkillFirst], value)
	case v == VarPlayerBits:
		pl.Bits.Set(int(int16(value)), true)
	case v == VarSkillPoints:
		pl.SkillPoints += int32(value)
	}
}

func (r *run) statSpeech(p int, base bool) {
	s := speechBonus
	if base {
		s = speechBase
	}
	r.m.Speak(p, s, r.h.Ctx())
}

// subSlot is the member Subtract makes speak: p when in slot 1..3, else the first.
func subSlot(p int) int {
	if p >= 1 && p <= 3 {
		return p
	}
	return 0
}

// subPlayer is Subtract of member variable v for member p (-1: nobody, nothing).
// HP goes through ReceiveDamage (physical).
//
// mm8: 0x44a0fe (Evt_Sub)
func (r *run) subPlayer(p int, v Var, value uint32) {
	e := r.env()
	if e == nil {
		r.h.Stub(OpSubtract, nil, v)
		return
	}
	if p < 0 {
		return
	}
	m, ctx := r.m, r.h.Ctx()
	pl := &m.Players[p]
	speak := func(base bool) {
		s := speechBonus
		if base {
			s = speechBase
		}
		if k := subSlot(p); k < len(m.Players) {
			m.Speak(k, s, ctx)
		}
	}
	switch {
	case v == VarHP:
		m.ReceiveDamage(p, int32(value), party.DamagePhysical, ctx)
	case v == VarSP:
		pl.SP = max(pl.SP-int32(value), 0)
	case v == VarACBonus:
		pl.ACBonus -= int16(uint8(value))
	case v == VarLevel:
		pl.LevelBase -= int16(uint8(value))
	case v == VarLevelBonus:
		pl.LevelBonus -= int16(uint8(value))
	case v == VarAge:
		pl.AgeBonus -= int16(value)
	case v == VarAwards:
		pl.Awards.Set(int(int16(value)), false)
	case v == VarExp:
		pl.Exp -= int64(int32(value))
	case v == VarItem:
		// the pack's top-left cells, then the mouse item (the cells covered by an
		// item read stray memory in the original; they are skipped here)
		if t := ctx.Items; t != nil {
			for c := range pl.Grid {
				if n := pl.Grid[c]; n > 0 && uint32(pl.Items[n-1].Number) == value {
					pl.RemoveItemAt(t, c)
					return
				}
			}
		}
		if uint32(m.MouseItem.Number) == value {
			m.MouseItem = items.Item{}
		}
	case v >= VarBonusFirst && v < VarResistFirst:
		f, base := statPtr(pl, v)
		*f -= int16(value)
		speak(base)
	case v >= VarResistFirst && v < VarSkillFirst:
		if s := resist(pl, v); s != nil {
			*s -= uint16(value)
			// the fire and air bonuses take the base values' reaction
			speak(v < VarResBonFirst+2)
		}
	case skillVar(v):
		pl.Skills[v-VarSkillFirst] -= uint16(uint8(value))
	case v == VarPlayerBits:
		pl.Bits.Set(int(int16(value)), false)
	case v == VarSkillPoints:
		if value <= uint32(pl.SkillPoints) {
			pl.SkillPoints -= int32(value)
		} else {
			pl.SkillPoints = 0
		}
	}
}

// Lich resistances: Set class 1 raises fire, air, water and earth to 20 and makes mind
// and body immune.
const lichMinResist = 0x14

// setPlayer is Set of member variable v for member p (in the party).
//
// mm8: 0x448d4b (Evt_Set)
func (r *run) setPlayer(p int, v Var, value uint32) {
	e := r.env()
	if e == nil {
		r.h.Stub(OpSet, nil, v)
		return
	}
	m, ctx := r.m, r.h.Ctx()
	pl := &m.Players[p]
	switch {
	case v == VarClass:
		pl.Class = int(uint8(value))
		if uint8(value) == 1 {
			// The lich: resistances, then the portrait of the lich (0x1a male, 0x1b
			// female) in face and voice; the old ones are kept at +0x1be8/+0x1bec.
			for k := party.DamageFire; k <= party.DamageEarth; k++ {
				pl.Resists[k] = max(pl.Resists[k], lichMinResist)
			}
			pl.Resists[party.DamageMind], pl.Resists[party.DamageBody] = party.ImmuneResist, party.ImmuneResist
			pl.OldVoice, pl.OldFace = pl.Voice, pl.Face
			f := 0x1a
			if party.IsFemale(pl.Face) {
				f = 0x1b
			}
			pl.Face, pl.Voice = f, f // the portrait panel reloads the face (portraits.sync)
		}
	case v == VarHP:
		pl.HP = int32(value)
	case v == VarHPFull:
		pl.HP = pl.MaxHP(e)
	case v == VarSP:
		pl.SP = int32(value)
	case v == VarSPFull:
		pl.SP = pl.MaxSP(e)
	case v == VarACBonus:
		pl.ACBonus = int16(uint8(value))
	case v == VarLevel:
		pl.LevelBase = int16(uint8(value))
	case v == VarLevelBonus:
		pl.LevelBonus = int16(uint8(value))
	case v == VarAge:
		pl.AgeBonus = int16(value)
	case v == VarAwards:
		n := int(int16(value))
		if !pl.Awards.Get(n) && r.h.AwardText(int(value)) {
			m.Speak(p, 0x60, ctx)
		}
		pl.Awards.Set(n, true)
	case v == VarExp:
		pl.Exp = int64(int32(value))
	case v == VarItem:
		m.GiveItem(int32(value), false, ctx)
	case v >= VarBonusFirst && v < VarResistFirst:
		f, base := statPtr(pl, v)
		*f = int16(uint8(value))
		r.statSpeech(p, base)
	case v >= VarResistFirst && v < VarSkillFirst:
		if s := resist(pl, v); s != nil {
			*s = uint16(uint8(value))
			r.statSpeech(p, v < VarResBonFirst)
		}
	case skillVar(v):
		pl.Skills[v-VarSkillFirst] = setSkill(pl.Skills[v-VarSkillFirst], value)
	case v == VarPlayerBits:
		pl.Bits.Set(int(int16(value)), true)
	case v == VarSkillPoints:
		pl.SkillPoints = int32(value)
	}
}
