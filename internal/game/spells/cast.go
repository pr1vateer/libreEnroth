package spells

import "libre-enroth/internal/game/party"

// Dispatch runs the cast preamble and the projectile casts. A slot that still needs
// a target stays queued. Any other spell is cleared, noted once, and not paid for:
// buffs, cures and the rest of the book are M9c through M9e. One cursed failure
// ends the whole pass, the way the original returns out of it.
//
// mm8: 0x42685f (Spell_Dispatch, the preamble and the projectile cases)
func (q *Queue) Dispatch(h Host) {
	if h == nil {
		return
	}
	for i := range q.s {
		sl := &q.s[i]
		if sl.Spell == 0 {
			continue
		}
		h.ClearInvisibility()
		p, ok := h.Player(int(sl.Roster))
		if !ok {
			sl.Spell = 0
			continue
		}
		if sl.Flags&Pending != 0 {
			if !p.CanAct {
				q.CancelPending()
			}
			continue
		}
		id := int(sl.Spell)
		sk := school(id)
		var word uint16
		var level uint16
		if sl.Skill != 0 {
			word = sl.Skill
			level = word & party.SkillLevel
		} else if sk != 0 || id < 0x85 {
			if p.Level != nil {
				level = p.Level(sk) & party.SkillLevel
			}
			if sk >= 0 && sk < len(p.Raw) {
				word = p.Raw[sk]
			}
		}
		rank := mastery(word)
		row := h.Spell(id)
		cost, rec := 0, 0
		if rank >= 1 && rank <= 4 {
			cost = int(row.Cost[rank-1])
			rec = int(row.Recovery[rank-1])
		}
		if sl.Skill != 0 {
			cost = 0
		}
		hour, minute := h.Hour()
		if minute == 0 && (hour == 0 && sk == party.SkillFire+8 || hour == 12 && sk == party.SkillFire+7) {
			cost = 0
		}
		if id < 100 && p.SP < cost {
			h.NoMana()
			sl.Spell = 0
			continue
		}
		if p.Cursed && id < 100 && h.Rand()%100 < 0x32 {
			h.Recover(int(sl.Roster), party.SpellDelay(rec, h.TurnBased()))
			h.Failed()
			h.Spend(int(sl.Roster), cost)
			sl.Spell = 0
			return
		}
		if !known(id) {
			h.Note("spell", "Spell_Dispatch 0x42685f: buffs, cures and the other casts (M9c..M9e)")
			sl.Spell = 0
			continue
		}
		target := sl.Target
		if target == 0 {
			target = h.Target(int(sl.Roster), id)
		}
		if id >= 0x85 && id <= 0x89 {
			ranged := id == 0x85 || id == 0x86 || id == 0x88
			rec = h.Recovery(int(sl.Roster), ranged)
		}
		fired := launch(h, sl, id, int(level), rank, target)
		if sl.Flags&0x20 == 0 {
			h.Recover(int(sl.Roster), party.SpellDelay(rec, h.TurnBased()))
		}
		if fired && id < 0x85 {
			h.Speak(int(sl.Roster), 0x31) // sound: the spell's word at 0x50012c (M11)
		}
		sl.Spell = 0
	}
}

// school is the skill a spell draws on. 0x86 is the bow's other arrow and 0x87 the
// blaster: the preamble names a skill only for 0x85, 0x88 and 0x89, and those two
// take the skill of the attack they share a case with.
func school(spell int) int {
	switch {
	case spell < 0xc:
		return party.SkillFire
	case spell < 0x17:
		return party.SkillFire + 1
	case spell < 0x22:
		return party.SkillFire + 2
	case spell < 0x2d:
		return party.SkillFire + 3
	case spell < 0x38:
		return party.SkillFire + 4
	case spell < 0x43:
		return party.SkillFire + 5
	case spell < 0x4e:
		return party.SkillFire + 6
	case spell < 0x59:
		return party.SkillFire + 7
	case spell < 100:
		return party.SkillFire + 8
	case spell < 0x6f:
		return party.SkillDarkElf
	case spell < 0x7a:
		return party.SkillVampire
	case spell < 0x85 || spell == 0x89:
		return party.SkillDragon
	case spell == 0x85 || spell == 0x86 || spell == 0x88:
		return party.SkillBow
	case spell == 0x87:
		return party.SkillBlaster
	default:
		return 0
	}
}

func mastery(word uint16) int {
	switch {
	case word&party.SkillGM != 0:
		return 4
	case word&party.SkillMaster != 0:
		return 3
	case word&party.SkillExpert != 0:
		return 2
	default:
		return 1
	}
}

func known(id int) bool {
	switch id {
	case 2, 6, 0xb, 0x12, 0x1a, 0x20, 0x22, 0x25, 0x3b, 0x41, 0x46, 0x4e,
		0x60, 0x61, 0x66, 0x67, 0x6f, 0x71, 0x7b,
		0x85, 0x86, 0x87, 0x88, 0x89:
		return true
	default:
		return false
	}
}
