package evt

import (
	"encoding/binary"
	"testing"

	"libre-enroth/internal/game/clock"
	"libre-enroth/internal/game/items"
	"libre-enroth/internal/game/party"
	"libre-enroth/internal/game/party/partytest"
)

// statHost is a three-knight party with the made-up tables.
func statHost() *fakeHost {
	h := newHost(3)
	h.ctx.Items, h.ctx.Classes = partytest.Items(), partytest.Classes()
	h.m.Time = clock.NewGame
	for i := range h.m.Players {
		p := &h.m.Players[i]
		p.Class, p.LevelBase, p.BirthYear = 4, 1, clock.BaseYear-20
		for k := range p.Stats {
			p.Stats[k].Base = 10
		}
		p.HP = 30
	}
	return h
}

// event runs records as event 1 (step 0 first) and returns the mapvar 0 flag a jump
// to step 9 sets.
func event(t *testing.T, h *fakeHost, recs ...[]byte) bool {
	t.Helper()
	recs = append(recs, rec(1, 8, OpExit), rec(1, 9, OpSet, varArgs(mapVar(0), 1)...))
	h.vars[0] = 0
	vm := &VM{Host: h, Map: script(t, recs...)}
	vm.Run(Source{}, 1, 0, true)
	return h.vars[0] == 1
}

// Compare of member variables with the selectors: a slot, everyone, the selected one.
//
// mm8: 0x4483d3 (Evt_Compare)
func TestComparePlayer(t *testing.T) {
	h := statHost()
	h.m.Players[1].HP = 50
	cmp := func(sel int, v Var, value uint32) bool {
		return event(t, h, rec(1, 0, OpForPartyMember, byte(sel)), rec(1, 1, OpCompare, varArgs(v, value, 9)...))
	}
	if cmp(0, VarHP, 40) || !cmp(1, VarHP, 40) || !cmp(selAll, VarHP, 40) {
		t.Error("HP >= 40 by slot/everyone")
	}
	h.m.Selected = 2
	if !cmp(selSelected, VarHP, 50) {
		t.Error("the selected member")
	}
	if cmp(0, VarHPFull, 0) { // knight max HP 35 + (1 + 1) * 5 = 45 > 30: no
		t.Error("HP full at 30 of 45")
	}
	if !cmp(1, VarHPFull, 0) {
		t.Error("HP 50 of 45 not full")
	}
	h.m.Players[0].Skills[party.SkillSword] = party.SkillExpert | 4
	if !cmp(0, VarSkillFirst+party.SkillSword, 4) || cmp(0, VarSkillFirst+party.SkillSword, 5) {
		t.Error("sword level")
	}
	if !cmp(0, VarSkillFirst+party.SkillSword, party.SkillExpert) || cmp(0, VarSkillFirst+party.SkillSword, party.SkillMaster) {
		t.Error("sword rank")
	}
	h.m.Players[0].Skills[party.SkillPerception] = 9
	if cmp(0, VarSkillFirst+party.SkillPerception, 1) {
		t.Error("perception (0x61) has no case: reads -1")
	}
	*h.m.Players[0].BasePtr(party.StatSpeed) = 17 // struct slot 4: var 0x24
	if !cmp(0, VarBaseFirst+4, 17) || !cmp(0, VarActualFirst+4, 17) {
		t.Error("speed is the fifth stat variable")
	}
	h.m.MouseItem = items.Item{Number: partytest.Gem}
	if !cmp(2, VarItem, partytest.Gem) || cmp(2, VarItem, partytest.Ring) {
		t.Error("the mouse item")
	}
	h.m.Players[2].Items[4] = items.Item{Number: partytest.Ring}
	if !cmp(2, VarItem, partytest.Ring) {
		t.Error("an item slot")
	}
	if cmp(2, VarEquipped, partytest.Ring) {
		t.Error("the ring is not worn")
	}
	h.m.Players[2].Equip[party.SlotRing] = 5
	if !cmp(2, VarEquipped, partytest.Ring) {
		t.Error("worn ring")
	}
	if !cmp(0, VarSex, 0) || cmp(0, VarClass, 3) || !cmp(0, VarClass, 4) {
		t.Error("sex/class")
	}
}

// Add, Subtract and Set of stats, resistances, skills, HP and items.
//
// mm8: 0x449726 (Evt_Add), 0x44a0fe (Evt_Sub), 0x448d4b (Evt_Set)
func TestAddSubSetPlayer(t *testing.T) {
	h := statHost()
	p := &h.m.Players[0]
	do := func(op Op, v Var, value uint32) {
		t.Helper()
		event(t, h, rec(1, 0, OpForPartyMember, 0), rec(1, 1, op, varArgs(v, value)...))
	}
	do(OpAdd, VarBaseFirst, 250)
	if p.Base(party.StatMight) != 0xff {
		t.Errorf("might capped at 255: %d", p.Base(party.StatMight))
	}
	do(OpSubtract, VarBaseFirst, 55)
	do(OpAdd, VarActualFirst+5, 3) // accuracy: its bonus
	if p.Base(party.StatMight) != 200 || p.Stats[5].Bonus != 3 {
		t.Errorf("might %d, accuracy bonus %d", p.Base(party.StatMight), p.Stats[5].Bonus)
	}
	do(OpSet, VarResistFirst+5, 0x123) // mind (0x33): the low byte
	if p.Resists[party.DamageMind] != 0x23 {
		t.Errorf("mind resistance %#x", p.Resists[party.DamageMind])
	}
	do(OpAdd, VarHP, 100) // up to the maximum: 35 + (1 + 1) * 5
	if p.HP != 45 {
		t.Errorf("HP %d", p.HP)
	}
	do(OpSubtract, VarHP, 15) // physical damage
	if p.HP != 30 {
		t.Errorf("HP after damage %d", p.HP)
	}
	sk := &p.Skills[party.SkillSword]
	*sk = party.SkillMaster | 5
	do(OpAdd, VarSkillFirst+party.SkillSword, 0x42) // +0x42 to the level, at most 60
	if *sk != party.SkillMaster|0x3c {
		t.Errorf("add 0x42: %#x", *sk)
	}
	*sk = party.SkillMaster | 5
	do(OpAdd, VarSkillFirst+party.SkillSword, 2) // or-ed in, the mastery dropped
	if *sk != 7 {
		t.Errorf("add 2: %#x", *sk)
	}
	*sk = party.SkillGM | party.SkillExpert | 5
	do(OpSet, VarSkillFirst+party.SkillSword, party.SkillMaster)
	if *sk != party.SkillExpert|party.SkillMaster {
		t.Errorf("set master: %#x", *sk)
	}
	do(OpSubtract, VarSkillFirst+party.SkillSword, 0x40)
	if *sk != party.SkillMaster {
		t.Errorf("sub 0x40: %#x", *sk)
	}
	// (only ids 152..176 get charges: not this made-up wand)
	do(OpAdd, VarItem, partytest.Wand)
	if it := h.m.MouseItem; it.Number != partytest.Wand || it.Charges != 0 || it.Flags != items.FlagIdentified {
		t.Errorf("mouse %+v", it)
	}
	do(OpSet, VarItem, partytest.Gem) // the wand goes to the pack
	if h.m.MouseItem.Number != partytest.Gem || p.Items[0].Number != partytest.Wand {
		t.Errorf("mouse %d, pack %d", h.m.MouseItem.Number, p.Items[0].Number)
	}
	do(OpSubtract, VarItem, partytest.Wand)
	if p.Items[0].Number != 0 || p.Grid[0] != 0 {
		t.Error("the wand is still packed")
	}
	do(OpAdd, VarExp, 1000)
	do(OpSubtract, VarSkillPoints, 5)
	if p.Exp != 1000 || p.SkillPoints != 0 {
		t.Errorf("exp %d, skill points %d", p.Exp, p.SkillPoints)
	}
	do(OpSet, VarClass, 1) // the lich
	if p.Class != 1 || p.Resists[party.DamageFire] != 20 || p.Resists[party.DamageBody] != party.ImmuneResist || p.Face != 0x1a {
		t.Errorf("lich: class %d fire %d body %d face %#x", p.Class, p.Resists[party.DamageFire], p.Resists[party.DamageBody], p.Face)
	}
}

// CheckSkill: level and rank bit, one member or anyone.
//
// mm8: 0x4446bd (case 0x2b)
func TestCheckSkill(t *testing.T) {
	h := statHost()
	h.m.Players[2].Skills[party.SkillMerchant] = party.SkillExpert | 6
	args := func(skill, rank byte, level int32) []byte {
		b := []byte{skill, rank}
		b = binary.LittleEndian.AppendUint32(b, uint32(level))
		return append(b, 9)
	}
	check := func(sel int, rank byte, level int32) bool {
		return event(t, h, rec(1, 0, OpForPartyMember, byte(sel)), rec(1, 1, OpCheckSkill, args(party.SkillMerchant, rank, level)...))
	}
	if !check(selAll, 0, 6) || check(selAll, 0, 7) || !check(selAll, 1, 1) || check(selAll, 2, 1) {
		t.Error("anyone")
	}
	if check(0, 0, 1) || !check(2, 1, 6) {
		t.Error("by slot")
	}
}

// GiveItem makes an item and puts it on the mouse; CheckItemsCount and RemoveItems
// count the mouse, the packs and (by the first id only) the worn items.
//
// mm8: 0x4446bd (case 0x29), 0x44a994, 0x44aa52
func TestItemOpcodes(t *testing.T) {
	h := statHost()
	give := []byte{2, 0x2c} // level 2, a potion
	give = binary.LittleEndian.AppendUint32(give, 0)
	event(t, h, rec(1, 0, OpGiveItem, give...))
	if it := h.m.MouseItem; it.Number != partytest.Potion || it.Bonus != 12 {
		t.Errorf("given %+v", it)
	}
	h.m.MouseItem = items.Item{}
	tb := h.ctx.Items
	h.m.Players[0].AddItem(tb, -1, items.Item{Number: partytest.Ring})
	h.m.Players[1].AddItem(tb, -1, items.Item{Number: partytest.Gem})
	h.m.Players[2].EquipItem(tb, h.ctx.Classes, items.Item{Number: partytest.Wand})
	count := func(lo, hi, n uint16) bool {
		a := binary.LittleEndian.AppendUint16(nil, lo)
		a = binary.LittleEndian.AppendUint16(a, hi)
		a = binary.LittleEndian.AppendUint16(a, n)
		return event(t, h, rec(1, 0, OpCheckItemsCount, append(a, 9)...))
	}
	if !count(partytest.Ring, partytest.Gem, 2) || count(partytest.Ring, partytest.Gem, 3) {
		t.Error("pack count")
	}
	if !count(partytest.Wand, partytest.Gem, 2) || count(partytest.Ring, partytest.Gem, 3) {
		t.Error("worn wand counts by its own id")
	}
	if !CheckItemsCount(h.m, partytest.Ring, partytest.Gem, 2) {
		t.Error("direct")
	}
	RemoveItems(h.m, tb, partytest.Ring, partytest.Gem, 2)
	if h.m.Players[0].Items[0].Number != 0 || h.m.Players[1].Items[0].Number != 0 || h.m.Players[2].Equip[party.SlotMainHand] == 0 {
		t.Error("removed the wrong items")
	}
	RemoveItems(h.m, tb, partytest.Wand, partytest.Wand, 1)
	if h.m.Players[2].Equip[party.SlotMainHand] != 0 {
		t.Error("the worn wand stayed")
	}
}
