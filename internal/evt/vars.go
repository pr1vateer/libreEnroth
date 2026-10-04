package evt

import (
	"fmt"
	"strings"

	"libre-enroth/internal/game/clock"
	"libre-enroth/internal/game/party"
)

// Var is an event variable number (Compare, Add, Subtract, Set).
type Var int

// The variables libre-enroth has so far (re/notes/evt.md lists them all).
const (
	VarQBits        Var = 0x10
	VarItem         Var = 0x11
	VarHour         Var = 0x12
	VarDayOfYear    Var = 0x13
	VarDayOfWeek    Var = 0x14
	VarGold         Var = 0x15
	VarRandomGold   Var = 0x16
	VarFood         Var = 0x17
	VarRandomFood   Var = 0x18
	VarCondFirst    Var = 0x6b // conditions 0..16
	VarCondLast     Var = 0x7b
	VarMainCond     Var = 0x7c
	VarMapFirst     Var = 0x7d // map variables 0..99
	VarMapLast      Var = 0xe0
	VarAutonotes    Var = 0xe1
	VarFlying       Var = 0xf2
	VarMonth        Var = 0xf6
	VarCounterFirst Var = 0xf7 // counters 0..9
	VarCounterLast  Var = 0x100
	VarStampFirst   Var = 0x101
	VarStampLast    Var = 0x114
	VarReputation   Var = 0x115
	VarHistoryFirst Var = 0x116
	VarHistoryLast  Var = 0x132
	VarLocation0C   Var = 0x133
	VarBank         Var = 0x134
	VarDeaths       Var = 0x135
	VarBounty       Var = 0x136
	VarPrison       Var = 0x137
	VarArenaFirst   Var = 0x138
	VarArenaLast    Var = 0x13b
	VarInvisible    Var = 0x13c
	VarEquipped     Var = 0x13d
	VarInParty      Var = 0x13e
)

// Global.txt messages of the money and food variables.
const (
	txtFoundGold  = 0x1d3 // "You found %lu gold!"
	txtFoodGained = 0x1f6
	txtGoldLost   = 0x1f7
	txtFoodLost   = 0x1f8
	txtFoodSet    = 0x1f5
	txtGoldSet    = 500
)

// partyVar reports a variable that is not about one member (the stats of members are
// M7's: libre-enroth's players do not have them yet).
func partyVar(v Var) bool {
	switch {
	case v == VarQBits, v >= VarHour && v <= VarRandomFood, v >= VarCondFirst && v <= VarAutonotes,
		v == VarFlying, v >= VarMonth && v <= VarInvisible, v == VarInParty:
		return true
	}
	return false
}

// cfmt formats a C printf format with ints: %lu, %u and %i become %d.
func cfmt(f string, args ...any) string {
	f = strings.NewReplacer("%lu", "%d", "%u", "%d", "%i", "%d", "%ld", "%d").Replace(f)
	return fmt.Sprintf(f, args...)
}

// counterDue reports that value hours have passed since stamp: stamp +
// ftol(int32(value * 0x70800) * (float)(1/30)) <= now (x87, exact; the multiply wraps
// in 32 bits as the original's imul does).
func counterDue(stamp, now clock.Time, value uint32) bool {
	i := int64(int32(value * 0x70800))
	// float(1/30) = 0x888889 * 2^-28
	prod := i * 0x888889
	var d int64
	if prod >= 0 {
		d = prod >> 28
	} else {
		d = -((-prod) >> 28)
	}
	return stamp+clock.Time(d) <= now
}

// compare is Compare's test of variable v against value for member p (-1 none):
// mostly "variable >= value" (signed); bits, conditions and the calendar test their own
// way. Unknown variables read as -1.
//
// mm8: 0x4483d3 (Evt_Compare)
func (r *run) compare(p int, v Var, value uint32) bool {
	m, h := r.m, r.h
	val := int32(value)
	cur := int32(-1)
	switch {
	case v == VarQBits:
		if m.QBits.Get(int(int16(value))) {
			return true
		}
	case v == VarHour:
		return uint32(m.Calendar.Hour) == value
	case v == VarDayOfYear:
		return uint32(m.Calendar.DayOfYear()) == value
	case v == VarDayOfWeek:
		return uint32(m.Calendar.Weekday()) == value
	case v == VarGold:
		cur = m.Gold
	case v == VarFood:
		cur = m.Food
	case v >= VarCondFirst && v <= VarCondLast:
		// the low dword of the condition's time: set or not
		return p >= 0 && int32(m.Players[p].Conditions[v-VarCondFirst]) != 0
	case v == VarMainCond:
		if p < 0 {
			return false
		}
		c := m.Players[p].MainCondition()
		if c == party.CondGood {
			return true
		}
		cur = int32(c)
	case v >= VarMapFirst && v <= VarMapLast:
		if mv := h.MapVars(); mv != nil {
			cur = int32(mv[v-VarMapFirst])
		}
	case v == VarAutonotes:
		// The test is one bit off from what Add and Set set (value - 1, as written).
		return m.Autonotes.Get(int(int16(value) - 1))
	case v == VarFlying:
		return h.Flying() && h.PartyBuff(7)
	case v == VarMonth:
		return uint32(m.Calendar.Month) == value
	case v >= VarCounterFirst && v <= VarCounterLast:
		ts := m.Counters[v-VarCounterFirst]
		return ts != 0 && counterDue(ts, m.Time, value)
	case v == VarReputation:
		if loc := h.Location(); loc != nil {
			return val <= le32(loc[8:])
		}
	case v == VarLocation0C:
		if loc := h.Location(); loc != nil {
			return uint32(le32(loc[0xc:])) == value
		}
	case v == VarBank:
		cur = m.Bank
	case v == VarDeaths:
		cur = m.Deaths
	case v == VarBounty:
		cur = m.Bounty
	case v == VarPrison:
		cur = m.Prison
	case v >= VarArenaFirst && v <= VarArenaLast:
		cur = int32(m.ArenaWins[v-VarArenaFirst])
	case v == VarInvisible:
		return h.PartyBuff(11)
	case v == VarInParty:
		return m.InParty != nil && m.InParty(int(value))
	case !partyVar(v):
		h.Stub(OpCompare, nil, v)
	}
	return cur >= val
}

func le32(b []byte) int32 {
	return int32(uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16 | uint32(b[3])<<24)
}

func put32(b []byte, v int32) {
	b[0], b[1], b[2], b[3] = byte(v), byte(v>>8), byte(v>>16), byte(v>>24)
}

// randomAmount is rand() % n + 1 (n 0 counts as 1).
func (r *run) randomAmount(n uint32) int32 {
	if n == 0 {
		n = 1
	}
	return int32(uint32(r.h.Ctx().Rand.Int())%n) + 1
}

// stampHistory sets history entry i to now unless it is set.
func (r *run) stampHistory(i int) {
	if r.m.History[i] == 0 {
		r.m.History[i] = r.m.Time
	}
}

// add is Add for member p. Like the original it does nothing for a member outside the
// party; a newly set quest bit or autonote with text makes the member speak.
//
// mm8: 0x449726 (Evt_Add)
func (r *run) add(p int, v Var, value uint32) {
	if p < 0 {
		return
	}
	m, h := r.m, r.h
	ctx := h.Ctx()
	switch {
	case v == VarQBits:
		n := int(int16(value))
		if !m.QBits.Get(n) && h.QuestText(int(value)) {
			m.Speak(p, 0x5d, ctx)
		}
		m.QBits.Set(n, true)
	case v == VarGold:
		r.addGold(int32(value))
	case v == VarRandomGold:
		r.addGold(r.randomAmount(value))
	case v == VarFood:
		m.Food += int32(value)
		h.Status(cfmt(r.global(txtFoodGained), int32(value)), 2)
		if uint32(m.Food) > 0xffff {
			m.Food = 0xffff
		}
	case v == VarRandomFood:
		n := r.randomAmount(value)
		m.Food += n
		h.Status(cfmt(r.global(txtFoodGained), n), 2)
	case v >= VarCondFirst && v <= VarCondLast:
		m.SetCondition(p, party.Condition(v-VarCondFirst), false, ctx)
	case v == VarMainCond:
		m.Players[p].Conditions = [len(m.Players[p].Conditions)]int64{}
	case v >= VarMapFirst && v <= VarMapLast:
		if mv := h.MapVars(); mv != nil {
			mv[v-VarMapFirst] = byte(min(int(mv[v-VarMapFirst])+int(value&0xff), 0xff))
		}
	case v == VarAutonotes:
		n := int(int16(value))
		if !m.Autonotes.Get(n) && h.AutonoteText(int(value)) {
			m.Speak(p, 0x60, ctx)
		}
		m.Autonotes.Set(n, true)
	case v >= VarCounterFirst && v <= VarCounterLast:
		m.Counters[v-VarCounterFirst] = m.Time
	case v >= VarStampFirst && v <= VarStampLast:
		m.Stamps[v-VarStampFirst] = m.Time
	case v == VarReputation:
		if loc := h.Location(); loc != nil {
			put32(loc[8:], min(le32(loc[8:])+int32(value), 10000))
		}
	case v >= VarHistoryFirst && v <= VarHistoryLast:
		r.stampHistory(int(v - VarHistoryFirst))
	case v == VarBank:
		m.Bank += int32(value)
	case v == VarDeaths:
		m.Deaths += int32(value)
	case v == VarBounty:
		m.Bounty += int32(value)
	case v == VarPrison:
		m.Prison += int32(value)
	case v >= VarArenaFirst && v <= VarArenaLast:
		m.ArenaWins[v-VarArenaFirst] += byte(value)
	case v == VarInParty:
		// Joining the party is M6's (0x48dc48); the quest bit id+400 is set either way.
		h.Stub(OpAdd, nil, v)
		m.QBits.Set(int(int16(value)+400), true)
	case v == VarHour, v == VarDayOfYear, v == VarDayOfWeek, v == VarFlying, v == VarMonth,
		v == VarLocation0C, v == VarInvisible:
	default:
		h.Stub(OpAdd, nil, v)
	}
}

// addGold adds gold with "You found %lu gold!".
//
// mm8: 0x420921 (Party_AddGold, mode 1)
func (r *run) addGold(n int32) {
	r.m.Gold += n
	r.h.Status(cfmt(r.global(txtFoundGold), n), 2)
}

// sub is Subtract for member p. Gold and bank gold the party does not have abort the
// event. The speech for a cleared quest bit goes to the member in party slot 1..3 of p
// (slot 0 otherwise), as the original finds it.
//
// mm8: 0x44a0fe (Evt_Sub)
func (r *run) sub(p int, v Var, value uint32) {
	m, h := r.m, r.h
	switch {
	case v == VarQBits:
		m.QBits.Set(int(int16(value)), false)
		slot := 0
		if p >= 1 && p <= 3 {
			slot = p
		}
		if slot < len(m.Players) {
			m.Speak(slot, 0x60, h.Ctx())
		}
	case v == VarGold:
		if uint32(m.Gold) < value {
			r.abort = true
			return
		}
		r.takeGold(value)
	case v == VarRandomGold:
		n := uint32(r.randomAmount(value))
		n = min(n, uint32(m.Gold))
		r.takeGold(n)
		h.Status(cfmt(r.global(txtGoldLost), n), 2)
	case v == VarFood:
		m.EatFood(int32(value))
	case v == VarRandomFood:
		n := r.randomAmount(value)
		n = min(n, m.Food)
		m.EatFood(n)
		h.Status(cfmt(r.global(txtFoodLost), n), 2)
	case v >= VarCondFirst && v <= VarCondLast:
		if p >= 0 {
			m.Players[p].Conditions[v-VarCondFirst] = 0
		}
	case v >= VarMapFirst && v <= VarMapLast:
		if mv := h.MapVars(); mv != nil {
			mv[v-VarMapFirst] -= byte(value)
		}
	case v == VarAutonotes:
		m.Autonotes.Set(int(int16(value)-1), false) // one off, as Compare
	case v == VarReputation:
		if loc := h.Location(); loc != nil {
			put32(loc[8:], max(le32(loc[8:])-int32(value), -10000))
		}
	case v == VarBank:
		if uint32(m.Bank) < value {
			r.abort = true
			return
		}
		m.Bank -= int32(value)
	case v == VarDeaths:
		m.Deaths -= int32(value)
	case v == VarBounty:
		m.Bounty -= int32(value)
	case v == VarPrison:
		m.Prison -= int32(value)
	case v >= VarArenaFirst && v <= VarArenaLast:
		m.ArenaWins[v-VarArenaFirst] -= byte(value)
	case v == VarInParty:
		h.Stub(OpSubtract, nil, v) // leaving the party (0x48dbc2) is M6's
	case partyVar(v):
		// the rest of the party variables cannot be subtracted
	default:
		h.Stub(OpSubtract, nil, v)
	}
}

// takeGold takes up to n gold.
//
// mm8: 0x4931e5 (Party_TakeGold)
func (r *run) takeGold(n uint32) {
	if uint32(r.m.Gold) < n {
		r.m.Gold = 0
		return
	}
	r.m.Gold -= int32(n)
}

// set is Set for member p (nothing outside the party). Conditions set this way can be
// blocked.
//
// mm8: 0x448d4b (Evt_Set)
func (r *run) set(p int, v Var, value uint32) {
	if p < 0 {
		return
	}
	m, h := r.m, r.h
	ctx := h.Ctx()
	switch {
	case v == VarQBits:
		n := int(int16(value))
		if !m.QBits.Get(n) && h.QuestText(int(value)) {
			m.Speak(p, 0x5d, ctx)
		}
		m.QBits.Set(n, true)
	case v == VarGold:
		m.Gold = int32(value)
	case v == VarRandomGold:
		n := r.randomAmount(value)
		m.Gold = n
		h.Status(cfmt(r.global(txtGoldSet), n), 2)
	case v == VarFood:
		m.Food = int32(value)
	case v == VarRandomFood:
		n := r.randomAmount(value)
		m.Food = n
		h.Status(cfmt(r.global(txtFoodSet), n), 2)
	case v >= VarCondFirst && v <= VarCondLast:
		m.SetCondition(p, party.Condition(v-VarCondFirst), true, ctx)
	case v == VarMainCond:
		m.Players[p].Conditions = [len(m.Players[p].Conditions)]int64{}
	case v >= VarMapFirst && v <= VarMapLast:
		if mv := h.MapVars(); mv != nil {
			mv[v-VarMapFirst] = byte(value)
		}
	case v == VarAutonotes:
		n := int(int16(value))
		if !m.Autonotes.Get(n) && h.AutonoteText(int(value)) {
			m.Speak(p, 0x60, ctx)
		}
		m.Autonotes.Set(n, true)
	case v >= VarCounterFirst && v <= VarCounterLast:
		m.Counters[v-VarCounterFirst] = m.Time
	case v >= VarStampFirst && v <= VarStampLast:
		m.Stamps[v-VarStampFirst] = m.Time
	case v == VarReputation:
		if loc := h.Location(); loc != nil {
			put32(loc[8:], min(int32(value), 10000))
		}
	case v >= VarHistoryFirst && v <= VarHistoryLast:
		r.stampHistory(int(v - VarHistoryFirst))
	case v == VarBank:
		m.Bank = int32(value)
	case v == VarDeaths:
		m.Deaths = int32(value)
	case v == VarBounty:
		m.Bounty = int32(value)
	case v == VarPrison:
		m.Prison = int32(value)
	case v >= VarArenaFirst && v <= VarArenaLast:
		m.ArenaWins[v-VarArenaFirst] = byte(value)
	case partyVar(v):
	default:
		h.Stub(OpSet, nil, v)
	}
}

func (r *run) global(i int) string { return r.h.Global(i) }
