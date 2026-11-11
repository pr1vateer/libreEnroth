package party

import (
	"testing"

	"libre-enroth/internal/game/items"
	"libre-enroth/internal/game/party/partytest"
)

func combatKnight() (*Player, *Env, *Rand) {
	c := &Ctx{Rand: NewRand(1), Items: partytest.Items(), Classes: partytest.Classes()}
	m := &Members{Players: []Player{{Class: 4}}}
	m.DefaultMember(0, c)
	p := &m.Players[0]
	p.Stats = [7]StatPair{}
	p.Skills = [NumSkills]uint16{}
	return p, m.Env(c), c.Rand
}

func wear(p *Player, id int32, slot int) {
	p.Items[slot] = items.Item{Number: id, Flags: items.FlagIdentified}
	p.Equip[slot] = int32(slot + 1)
}

// mm8: 0x48e317. Seed 1's first two rolls are 41 and 18467: a 2d4+1 sword is
// (1+41%4) + (1+18467%4) + 1 = 7, and a zero Might adds nothing.
func TestRollMeleeDamage(t *testing.T) {
	p, e, rng := combatKnight()
	if got := p.RollMeleeDamage(e, 0, nil, false, false, rng); got != 1+41%3 {
		t.Errorf("unarmed %d, want %d", got, 1+41%3)
	}
	p, e, rng = combatKnight()
	wear(p, partytest.Sword, SlotMainHand)
	if got := p.RollMeleeDamage(e, 0, nil, false, false, rng); got != 7 {
		t.Errorf("sword %d, want 7", got)
	}
	p, e, rng = combatKnight()
	wear(p, partytest.Sword, SlotMainHand)
	// Might 20 is +2 in the test table, added once after the dice.
	p.Stats[0].Base = 20
	rng = NewRand(1)
	if got := p.RollMeleeDamage(e, 0, nil, false, false, rng); got != 7+2 {
		t.Errorf("sword with might %d, want 9", got)
	}
}

// mm8: 0x48f237, 0x42e27f. A sword is 90; plate at normal pays the leather rank's 90
// on top; expert plate pays half; a normal shield pays 10; haste takes 25. The wait
// is that many units (at least 30) times 2.1333.
func TestRecovery(t *testing.T) {
	p, e, _ := combatKnight()
	if got := p.RecoveryUnits(e, false); got != 100 {
		t.Errorf("bare hands %d, want 100", got)
	}
	p.Skills[SkillUnarmed] = 1
	if got := p.RecoveryUnits(e, false); got != 60 {
		t.Errorf("unarmed skill %d, want 60", got)
	}
	p.Skills[SkillUnarmed] = 0
	wear(p, partytest.Sword, SlotMainHand)
	if got := p.RecoveryUnits(e, false); got != 90 {
		t.Errorf("sword %d, want 90", got)
	}
	wear(p, partytest.Plate, SlotArmor)
	if got := p.RecoveryUnits(e, false); got != 180 {
		t.Errorf("sword and plate %d, want 180", got)
	}
	p.Skills[SkillPlate] = SkillExpert | 1
	if got := p.RecoveryUnits(e, false); got != 90+45 {
		t.Errorf("expert plate %d, want 135", got)
	}
	p.Skills[SkillPlate] = 1
	wear(p, partytest.Shield, SlotOffhand)
	p.Skills[SkillShield] = 1
	if got := p.RecoveryUnits(e, false); got != 180+10 {
		t.Errorf("shield %d, want 190", got)
	}
	p.Buffs[BuffHaste].Expires = 1
	if got := p.RecoveryUnits(e, false); got != 190-25 {
		t.Errorf("haste %d, want 165", got)
	}
	if got := AttackDelay(90, false); got != 192 {
		t.Errorf("delay %d, want 192", got)
	}
	if got := AttackDelay(10, false); got != 64 {
		t.Errorf("minimum delay %d, want 64", got)
	}
	if got := AttackDelay(90, true); got != 0 {
		t.Errorf("turn-based delay %d", got)
	}
}

// mm8: 0x4255bb. 100 experience, one of two members unconscious, the other a
// normal student of Learning (the bonus is 9 + the level): 100 + 10.
func TestAwardExp(t *testing.T) {
	c := &Ctx{Rand: NewRand(1), Items: partytest.Items(), Classes: partytest.Classes()}
	m := &Members{Players: []Player{{Class: 4}, {Class: 4}}}
	m.Players[0].Skills[SkillLearning] = 1
	m.Players[1].Conditions[CondUnconscious] = 1
	m.AwardExp(100, c)
	if m.Players[0].Exp != 110 || m.Players[1].Exp != 0 {
		t.Errorf("exp %d %d, want 110 0", m.Players[0].Exp, m.Players[1].Exp)
	}
	m.Players[0].Exp = 3999999990
	m.AwardExp(100, c)
	if m.Players[0].Exp != 4000000000 {
		t.Errorf("cap %d", m.Players[0].Exp)
	}
}
