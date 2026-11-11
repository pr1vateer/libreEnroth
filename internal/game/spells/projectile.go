package spells

import "libre-enroth/internal/game/party"

// launch pays for a projectile spell and builds its object. The shared cases sit
// a third of the party's height up; a blaster (0x87) and a breath (0x89) sit at
// half. Spell 0x12 skips the frame it appears on. A master or grandmaster of the
// bow fires a second shot 0x20 higher. It reports whether the cast was paid for
// and launched (a bow or a breath is not paid for).
//
// mm8: 0x42685f (the shared projectile case, and 0x85..0x89)
func launch(h Host, sl *Slot, id, level, rank int, target int32) bool {
	if id < 0x85 && !h.Pay(int(sl.Roster), spellCost(h, sl, id, rank)) {
		return false
	}
	row := h.Spell(id)
	x, y, z, height := h.Place()
	z += height / 3
	if id == 0x87 || id == 0x89 {
		_, _, z0, ht := h.Place()
		z = z0 + ht/2
	}
	index, speed := h.Obj(row.Object)
	yaw, pitch := h.Aim(int(sl.Roster), target)
	var attrib uint16
	if id == 0x12 {
		attrib |= 0x40
	}
	if id >= 0x85 && id <= 0x89 {
		attrib |= 0x100
	}
	if h.TurnBased() {
		attrib |= 4
	}
	hand := 0
	shots := 1
	switch id {
	case 0x85, 0x86, 0x88:
		hand = 2
		if bowRank(h, int(sl.Roster)) > 2 {
			shots = 2
		}
	case 0x87, 0x89:
		hand = 1
	}
	item := h.Hand(int(sl.Roster), hand)
	for n := range shots {
		zz := z
		if n != 0 {
			zz += 0x20
		}
		h.Fire(Shot{
			Type: row.Object, Obj: index, X: x, Y: y, Z: zz,
			Yaw: yaw, Pitch: pitch, Speed: int32(speed),
			Attrib: attrib, Sector: h.Sector(x, y, zz),
			Caster: int32(sl.Roster)<<3 | 4, Target: target,
			Spell: int32(id), Level: int32(level), Skill: int32(rank),
			Sound: uint16(sl.Arg), Item: item, Member: int32(sl.Roster) + 1,
		})
	}
	return true
}

func spellCost(h Host, sl *Slot, id, rank int) int {
	if sl.Skill != 0 {
		return 0
	}
	row := h.Spell(id)
	if rank < 1 || rank > 4 {
		return 0
	}
	cost := int(row.Cost[rank-1])
	hour, minute := h.Hour()
	sk := school(id)
	if minute == 0 && (hour == 0 && sk == party.SkillFire+8 || hour == 12 && sk == party.SkillFire+7) {
		return 0
	}
	return cost
}

func bowRank(h Host, roster int) int {
	p, ok := h.Player(roster)
	if !ok {
		return 1
	}
	return mastery(p.Raw[party.SkillBow])
}
