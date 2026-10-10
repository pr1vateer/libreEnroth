package party

import (
	"testing"

	"libre-enroth/internal/game/items"
	"libre-enroth/internal/game/party/partytest"
)

// seedWhere is the first seed whose generator satisfies ok.
func seedWhere(t *testing.T, ok func(r *Rand) bool) uint32 {
	t.Helper()
	for s := uint32(1); s < 1<<22; s++ {
		if ok(NewRand(s)) {
			return s
		}
	}
	t.Fatal("no seed")
	return 0
}

// Player_MonsterBonus: the luck roll rand() % (StatToBonus(Luck) + 30 + strength) must be
// under 30; then the condition is set, as the original maps it (Poison2 and Poison3
// swapped, Disease1 gives disease 3, Disease2 and 3 nothing).
//
// mm8: 0x48ed43
func TestMonsterBonusConditions(t *testing.T) {
	// The range is 31, or 32 where a stat bonus of 1 is the strength.
	pass := seedWhere(t, func(r *Rand) bool { v := r.Int(); return v%31 <= 29 && v%32 <= 29 })
	fail := seedWhere(t, func(r *Rand) bool { v := r.Int(); return v%31 > 29 && v%32 > 29 })
	for _, c := range []struct {
		bonus int
		cond  Condition
		ok    bool
	}{
		{BonusCurse, CondCursed, true}, {BonusWeak, CondWeak, true}, {BonusAsleep, CondAsleep, true},
		{BonusDrunk, CondDrunk, true}, {BonusAfraid, CondAfraid, true}, {BonusInsane, CondInsane, true},
		{BonusParalyze, CondParalyzed, true}, {BonusUncon, CondUnconscious, true}, {BonusDead, CondDead, true},
		{BonusStone, CondStoned, true}, {BonusErad, CondEradicated, true}, {BonusPoison1, CondPoison1, true},
		{BonusPoison2, CondPoison3, true}, {BonusPoison3, CondPoison2, true}, {BonusDisease1, CondDisease3, true},
		{BonusDisease2, CondGood, false}, {BonusDisease3, CondGood, false},
	} {
		// Stats of 11: StatToBonus 1 (Luck), the resistances 0: rand() % 31.
		m := &Members{Players: []Player{{Class: 4}}, Time: 1000}
		for k := range m.Players[0].Stats {
			m.Players[0].Stats[k].Base = 11
		}
		ctx := testCtx()
		ctx.Rand = NewRand(pass)
		got := m.MonsterBonus(0, c.bonus, ctx, nil)
		set := c.cond != CondGood && m.Players[0].Conditions[c.cond] != 0
		if got != c.ok || c.ok != set {
			t.Errorf("bonus %#x: %v, condition %d set %v", c.bonus, got, c.cond, set)
		}
		m.Players[0].Conditions = [numConditions]int64{}
		ctx.Rand = NewRand(fail)
		if m.MonsterBonus(0, c.bonus, ctx, nil) || c.cond != CondGood && m.Players[0].Conditions[c.cond] != 0 {
			t.Errorf("bonus %#x: not shaken off", c.bonus)
		}
	}
}

// The item bonuses: a worn armour breaks unless hardened (Mod2 + Material) * 3 adds to
// the roll's range); Steal moves a pack item into the monster's free item slot; Age adds
// a year; DrainSP empties the spell points.
//
// mm8: 0x48ed43
func TestMonsterBonusItems(t *testing.T) {
	newM := func() (*Members, *Ctx) {
		m := &Members{Players: []Player{{Class: 4, SP: 20}}}
		for k := range m.Players[0].Stats {
			m.Players[0].Stats[k].Base = 11
		}
		return m, testCtx()
	}
	// one candidate: rand() % 1, then the roll (chain: Mod2 2 * 3 = 6; % 37)
	seed := seedWhere(t, func(r *Rand) bool { r.Int(); return r.Int()%37 <= 29 })
	for _, hard := range []bool{false, true} {
		m, ctx := newM()
		p := &m.Players[0]
		it := items.Item{Number: partytest.Chain}
		if hard {
			it.Flags |= items.FlagHardened
		}
		p.wear(1, SlotArmor, it)
		ctx.Rand = NewRand(seed)
		if !m.MonsterBonus(0, BonusBrkArmor, ctx, nil) || p.Items[0].Broken() == hard {
			t.Errorf("hardened %v: broken %v", hard, p.Items[0].Broken())
		}
	}
	m, ctx := newM()
	p := &m.Players[0]
	n := p.AddItem(ctx.Items, -1, items.Item{Number: partytest.Ring})
	var stolen []items.Item
	ctx.Rand = NewRand(seedWhere(t, func(r *Rand) bool { r.Int(); return r.Int()%31 <= 29 }))
	if !m.MonsterBonus(0, BonusSteal, ctx, func(it items.Item) bool { stolen = append(stolen, it); return true }) ||
		len(stolen) != 1 || stolen[0].Number != partytest.Ring || p.Items[n-1].Number != 0 {
		t.Errorf("steal: %v, slot %d holds %d", stolen, n, p.Items[n-1].Number)
	}
	m, ctx = newM()
	ctx.Rand = NewRand(seedWhere(t, func(r *Rand) bool { return r.Int()%31 <= 29 }))
	if !m.MonsterBonus(0, BonusAge, ctx, nil) || m.Players[0].AgeBonus != 1 {
		t.Errorf("age: %d", m.Players[0].AgeBonus)
	}
	m, ctx = newM()
	ctx.Rand = NewRand(seedWhere(t, func(r *Rand) bool { return r.Int()%31 <= 29 }))
	if !m.MonsterBonus(0, BonusDrainSP, ctx, nil) || m.Players[0].SP != 0 {
		t.Errorf("drain: SP %d", m.Players[0].SP)
	}
}
