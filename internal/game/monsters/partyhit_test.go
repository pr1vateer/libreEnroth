package monsters

import (
	"testing"

	"libre-enroth/internal/game/items"
	"libre-enroth/internal/game/party"
	"libre-enroth/internal/game/party/partytest"
)

// mm8: 0x425f2b. Seed 1's first roll is 41. AC 0 and no attack bonus: 41 % 30 = 11,
// under 15, a miss. AC 0 and +20 attack: 41 % 70 = 41, which reaches 15.
func TestHitsFromParty(t *testing.T) {
	b := testBrain(1, 0, 0, monster(aiTables(), 1, 10, 0))
	a := &b.Actors[0]
	a.Info.AC = 0
	p := &b.Party.Members.Players[0]
	p.Stats = [7]party.StatPair{}
	p.Skills = [party.NumSkills]uint16{}
	if b.HitsFromParty(p, a, 0, 0) {
		t.Error("untrained blow hit AC 0")
	}
	b = testBrain(1, 0, 0, monster(aiTables(), 1, 10, 0))
	a = &b.Actors[0]
	a.Info.AC = 0
	p = &b.Party.Members.Players[0]
	p.Stats = [7]party.StatPair{}
	p.Skills = [party.NumSkills]uint16{}
	p.Skills[party.SkillSword] = 20
	p.Items[0] = items.Item{Number: partytest.Sword, Flags: items.FlagIdentified}
	p.Equip[party.SlotMainHand] = 1
	if !b.HitsFromParty(p, a, 0, 0) {
		t.Error("a +20 sword missed AC 0")
	}
	// Buff 21 halves AC. AC 40 becomes 20; the threshold is 35. 41 % (20+30+40) = 41,
	// which does not reach 35? 41 % 90 = 41 >= 35, still a hit. Use a small attack.
	b = testBrain(1, 0, 0, monster(aiTables(), 1, 10, 0))
	a = &b.Actors[0]
	a.Info.AC = 40
	a.Buffs[BuffACHalved].Expires = 1
	p = &b.Party.Members.Players[0]
	p.Stats = [7]party.StatPair{}
	p.Skills = [party.NumSkills]uint16{}
	// attack 0, AC 20 after the halve: 41 % 50 = 41, threshold 35, a hit. Without the
	// halve the threshold is 55 and 41 misses.
	if !b.HitsFromParty(p, a, 0, 0) {
		t.Error("halved AC should be hittable")
	}
}

// mm8: 0x4378ec, 0x4255bb. A +20 sword's 2d4+1 is 7 (seed 1). The monster has 5 HP
// and 100 experience; the blow kills it and the member gains the experience.
func TestHitFromPartyKills(t *testing.T) {
	b := testBrain(1, 0, 0, monster(aiTables(), 1, 40, 0))
	a := &b.Actors[0]
	a.Info.AC = 0
	a.Info.HP = 20
	a.Info.Exp = 100
	a.HP = 5
	a.Radius = 10
	p := &b.Party.Members.Players[0]
	p.Stats = [7]party.StatPair{}
	p.Skills = [party.NumSkills]uint16{}
	p.Skills[party.SkillSword] = 20
	p.Items[0] = items.Item{Number: partytest.Sword, Flags: items.FlagIdentified}
	p.Equip[party.SlotMainHand] = 1
	b.HitFromParty(0, 0, BlowDirection(0, 0, 0, a))
	if a.AIState != Dying || a.HP != 0 {
		t.Fatalf("state %d HP %d", a.AIState, a.HP)
	}
	if p.Exp != 100 {
		t.Errorf("experience %d, want 100", p.Exp)
	}
}
