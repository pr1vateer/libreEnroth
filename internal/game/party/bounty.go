package party

import "libre-enroth/internal/game/clock"

// NumHunts is the number of bounty hunt slots (one per town hall in the original's
// layout).
const NumHunts = 5

// Hunt is a bounty hunt: the month's monster and whether it has been killed.
type Hunt struct {
	Refresh clock.Time // +0x3c + 8k: when a new monster is due
	Monster uint16     // +0x760 + 2k: monsters.txt id, 0 = claimed
	Killed  uint16     // +0x76a + 2k: set by Actor_Die
	// Kills is +0x774 + 2k, which the town hall sets to rand() % 10 + 1 and the bounty
	// topic only reads (npctopic 618, "not yet reached the number of kills needed"):
	// a count of kills nothing counts.
	Kills uint16
}

// MarkKilled flags every hunt whose monster is id as killed.
//
// mm8: 0x402ec9 (Actor_Die: the loop over party +0x760..0x768)
func (m *Members) MarkKilled(id int) {
	for k := range m.Hunts {
		if int(m.Hunts[k].Monster) == id {
			m.Hunts[k].Killed = 1
		}
	}
}
