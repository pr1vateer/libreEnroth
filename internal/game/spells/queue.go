package spells

// The cast queue (re/notes/spells.md). Ten slots. A slot whose flags meet Pending
// waits for a target and is not dispatched; the picker itself is M9c.

// Pending is the flag mask of a cast that still needs a target (0x426787, 0x42685f).
const Pending = 0x3ca

const queueLen = 10

// Slot is one CastSlot, 0x14 bytes at 0x51d768.
type Slot struct {
	Spell     int16
	Roster    int16
	PartySlot int16
	Flags     uint16
	Skill     uint16 // explicit skill word; 0 means the school's
	Target    int32  // pid; 0 means the actor under the mouse, else the nearest
	Arg       int32
}

// Queue is g_castQueue.
type Queue struct {
	s [queueLen]Slot
}

// Enqueue drops every queued cast (a cast that itself needs a target also drops the
// one pending slot first) and writes one. member is the party index; roster is
// g_partyRoster. The index of the new slot, or -1 when it could not be written.
//
// mm8: 0x4263ca (Spell_Enqueue), 0x4267ce (Cast_WriteSlot)
func (q *Queue) Enqueue(spell, member int, skill, flags uint16, arg int32, roster []int) int {
	if flags&Pending != 0 {
		for i := range q.s {
			if q.s[i].Flags&Pending != 0 {
				q.s[i].Spell = 0
				break
			}
		}
	}
	for i := range q.s {
		if q.s[i].Spell != 0 {
			q.s[i].Spell = 0
		}
	}
	for i := range q.s {
		if q.s[i].Spell != 0 {
			continue
		}
		sl := &q.s[i]
		sl.Spell = int16(spell)
		if member >= 0 && member < len(roster) {
			sl.Roster = int16(roster[member])
		}
		sl.PartySlot = 0
		if flags&0x10 != 0 {
			sl.PartySlot = -1
			for k, id := range roster {
				if id == member {
					sl.PartySlot = int16(k)
					break
				}
			}
		}
		sl.Flags = flags
		sl.Skill = skill
		sl.Target = 0
		sl.Arg = arg
		return i
	}
	return -1
}

// CancelPending clears every slot whose flags meet Pending.
//
// mm8: 0x426787 (Cast_CancelPending)
func (q *Queue) CancelPending() {
	for i := range q.s {
		if q.s[i].Spell != 0 && q.s[i].Flags&Pending != 0 {
			q.s[i].Spell = 0
		}
	}
}

// At is slot i, or a zero slot.
func (q *Queue) At(i int) Slot {
	if i < 0 || i >= len(q.s) {
		return Slot{}
	}
	return q.s[i]
}
