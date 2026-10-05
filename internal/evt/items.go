package evt

import (
	"libre-enroth/internal/assets/tables"
	"libre-enroth/internal/game/items"
	"libre-enroth/internal/game/party"
)

// checkSkill is CheckSkill (0x2b): +5 skill, +6 the rank's bit to test (0 any, 1 0x40,
// 2 0x80, 3 0x100), +7 i32 minimum level, +0xb the step to jump to. The selector picks
// the member (an empty slot reads the hero, roster character 0); everyone (5) passes
// when one member does.
//
// mm8: 0x4446bd (case 0x2b)
func (r *run) checkSkill(rec Record) bool {
	skill, rank, level := rec.U8(5), rec.U8(6), rec.I32(7)
	pass := func(pl *party.Player) bool {
		sk := pl.SkillAt(skill)
		bits := [4]uint16{1, sk & party.SkillExpert, sk & party.SkillMaster, sk & party.SkillGM}
		return int32(sk&party.SkillLevel) >= level && rank < 4 && bits[rank] != 0
	}
	m := r.m
	switch {
	case r.sel < 5:
		pl := m.RosterPlayer(0)
		if r.sel < len(m.Players) {
			pl = &m.Players[r.sel]
		}
		return pl != nil && pass(pl)
	case r.sel == selAll:
		for i := range m.Players {
			if pass(&m.Players[i]) {
				return true
			}
		}
		return false
	}
	p := r.player(r.sel)
	return p >= 0 && pass(&m.Players[p])
}

// giveItem is GiveItem (0x29): an item of +5 treasure level and +6 kind is made (no
// bonus forced) and, with a +7 id, takes that id; it goes on the mouse.
//
// mm8: 0x4446bd (case 0x29)
func (r *run) giveItem(rec Record) {
	c := r.h.Ctx()
	if c.Items == nil {
		r.h.Stub(OpGiveItem, rec, -1)
		return
	}
	level := max(min(rec.U8(5), 6), 1) // the original indexes its level tables with it
	it := items.Generate(c.Items, level, rec.U8(6), false, c.Rand, &r.m.ArtifactsFound)
	if id := rec.U32(7); id != 0 {
		it.Number = int32(id)
	}
	r.m.SetMouseItem(c.Items, it)
}

// summonItem is SummonItem (0x22): +5 id, above 1000 a random enchantment of level
// id/1000 for item id%1000 (a bonus forced); identified. The original drops it at
// +9 xyz as an object (M8); here it goes on the mouse.
//
// mm8: 0x4446bd (case 0x22), 0x42ecee (the object)
func (r *run) summonItem(rec Record) {
	c := r.h.Ctx()
	if c.Items == nil {
		r.h.Stub(OpSummonItem, rec, -1)
		return
	}
	v := int32(rec.U32(5))
	id := v % 1000
	var it items.Item
	if v > 1000 {
		it = items.Generate(c.Items, max(min(int(v/1000), 6), 1), int(c.Items.Item(id).EquipType)+1, true, c.Rand, &r.m.ArtifactsFound)
	}
	it.Flags |= items.FlagIdentified
	it.Number = id
	r.h.Stub(OpSummonItem, rec, -1) // notes once that the object is M8's
	r.m.SetMouseItem(c.Items, it)
}

// CheckItemsCount reports at least n items with ids min..max on the mouse, in the
// packs and worn. A worn item counts only when its id is min (the original tests
// "== min && <= max"). The original also reads the pack's empty and covered cells as
// item slots; only the items' top-left cells count here.
//
// mm8: 0x44a994 (Evt_CheckItemsCount)
func CheckItemsCount(m *party.Members, lo, hi, n int32) bool {
	in := func(id int32) bool { return lo <= id && id <= hi }
	count := int32(0)
	if in(m.MouseItem.Number) {
		count = 1
	}
	for i := range m.Players {
		pl := &m.Players[i]
		for _, c := range pl.Grid {
			if c > 0 && in(pl.Items[c-1].Number) {
				count++
			}
		}
		for s := 0; s < party.NumSlots; s++ {
			if it := pl.Equipped(s); it != nil && it.Number == lo && it.Number <= hi {
				count++
			}
		}
	}
	return n <= count
}

// RemoveItems takes up to n items with ids min..max: the mouse item first, then each
// member's pack and worn items.
//
// mm8: 0x44aa52 (Evt_RemoveItems)
func RemoveItems(m *party.Members, t *tables.Items, lo, hi, n int32) {
	if n < 1 {
		return
	}
	in := func(id int32) bool { return lo <= id && id <= hi }
	done := int32(0)
	if in(m.MouseItem.Number) {
		m.MouseItem = items.Item{}
		if done++; done == n {
			return
		}
	}
	for i := range m.Players {
		pl := &m.Players[i]
		for c := range pl.Grid {
			if k := pl.Grid[c]; k > 0 && in(pl.Items[k-1].Number) {
				pl.RemoveItemAt(t, c)
				if done++; done == n {
					return
				}
			}
		}
		for s := 0; s < party.NumSlots; s++ {
			if it := pl.Equipped(s); it != nil && in(it.Number) {
				*it = items.Item{}
				pl.Equip[s] = 0
				if done++; done == n {
					return
				}
			}
		}
	}
}
