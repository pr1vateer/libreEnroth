package party

import (
	"testing"

	"libre-enroth/internal/game/clock"
	"libre-enroth/internal/game/items"
	"libre-enroth/internal/game/party/partytest"
)

func membersOf(ps ...Player) *Members {
	return &Members{Players: ps, Time: clock.NewGame}
}

// A resistance halves the damage while rand() % (luck bonus + resist + 30) >= 30, at
// most four times. With srand(1): 41, 18467, 6334, 26500 mod 61 = 41, 45, 51, 26.
//
// mm8: 0x48e967, 0x48ec51
func TestReceiveDamageResist(t *testing.T) {
	c := testCtx()
	n := Player{Class: 0, HP: 50}
	*n.BasePtr(StatLuck) = 15 // bonus 1
	n.Resists[DamageFire] = 30
	m := membersOf(n)
	if got := m.ReceiveDamage(0, 100, DamageFire, c); got != 12 || m.Players[0].HP != 38 {
		t.Errorf("fire 100: %d done, HP %d; want 12, 38", got, m.Players[0].HP)
	}
	if got := c.Rand.Int(); got != 19169 {
		t.Errorf("next rand %d: the halving took more or fewer draws", got)
	}
	// the immune lich takes nothing
	l := Player{Class: 1, HP: 10}
	m = membersOf(l)
	if got := m.ReceiveDamage(0, 50, DamageBody, testCtx()); got != 0 || m.Players[0].HP != 10 {
		t.Errorf("lich body: %d", got)
	}
}

// Chain (grandmaster) and plate (master) shrink physical damage; a broken one does
// not; pain reflection stops it.
func TestReceiveDamageArmor(t *testing.T) {
	for _, c := range []struct {
		item  int32
		skill int
		sk    uint16
		flags uint32
		want  int32
	}{
		{partytest.Chain, SkillChain, SkillGM | 1, 0, 66},
		{partytest.Chain, SkillChain, SkillMaster | 1, 0, 100},
		{partytest.Plate, SkillPlate, SkillMaster | 1, 0, 50},
		{partytest.Plate, SkillPlate, SkillMaster | 1, items.FlagBroken, 100},
	} {
		p := knight()
		p.HP = 500
		p.Skills[c.skill] = c.sk
		p.wear(1, SlotArmor, items.Item{Number: c.item, Flags: c.flags})
		m := membersOf(p)
		if got := m.ReceiveDamage(0, 100, DamagePhysical, testCtx()); got != c.want {
			t.Errorf("item %d skill %#x flags %d: %d, want %d", c.item, c.sk, c.flags, got, c.want)
		}
	}
	p := knight()
	p.HP = 10
	p.Buffs[BuffPainReflect].Expires = 1
	m := membersOf(p)
	if got := m.ReceiveDamage(0, 100, DamagePhysical, testCtx()); got != 0 {
		t.Errorf("pain reflection: %d", got)
	}
}

// Down to 0 HP: unconscious, or dead when HP + Endurance < 1 (unless Preservation);
// -10 HP breaks the armour, and so does dying while the game time's low dword is not 0.
func TestReceiveDamageDeath(t *testing.T) {
	run := func(hp, end int32, dmg int32, pres bool, flags uint32) Player {
		p := knight()
		p.HP = hp
		*p.BasePtr(StatEndurance) = int16(end)
		p.Buffs[BuffPreservation].Expires = clock.Time(boolInt(pres))
		p.wear(1, SlotArmor, items.Item{Number: partytest.Chain, Flags: flags})
		m := membersOf(p)
		m.ReceiveDamage(0, dmg, DamagePhysical, testCtx())
		return m.Players[0]
	}
	if p := run(5, 10, 20, false, 0); p.Conditions[CondDead] == 0 || p.HP != -15 || !p.Items[0].Broken() {
		t.Errorf("death: dead %d HP %d broken %v", p.Conditions[CondDead], p.HP, p.Items[0].Broken())
	}
	if p := run(5, 10, 20, true, 0); p.Conditions[CondDead] != 0 || p.Conditions[CondUnconscious] == 0 {
		t.Error("preservation: not unconscious")
	}
	if p := run(5, 10, 8, false, 0); p.Conditions[CondUnconscious] == 0 || p.Items[0].Broken() {
		t.Errorf("knocked out: unconscious %d broken %v", p.Conditions[CondUnconscious], p.Items[0].Broken())
	}
	if p := run(5, 30, 20, false, 0); p.Conditions[CondUnconscious] == 0 || !p.Items[0].Broken() {
		t.Errorf("-15 HP: broken %v", p.Items[0].Broken())
	}
	if p := run(5, 30, 20, false, items.FlagHardened); p.Items[0].Broken() {
		t.Error("hardened armour broke")
	}
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// Heal: up to the maximum, waking the unconscious; not the dead.
//
// mm8: 0x48ebf7
func TestHeal(t *testing.T) {
	e := testEnv()
	p := knight()
	p.HP = -3
	p.Conditions[CondUnconscious] = 1
	p.Heal(e, 100)
	if p.HP != 60 || p.Conditions[CondUnconscious] != 0 {
		t.Errorf("HP %d unconscious %d", p.HP, p.Conditions[CondUnconscious])
	}
	p.Conditions[CondDead], p.HP = 1, -5
	p.Heal(e, 100)
	if p.HP != -5 {
		t.Errorf("healed the dead to %d", p.HP)
	}
}

// Fall damage: ftol(MaxHP * 0.1) * drop / 256, and a recovery of (20 - End bonus) *
// 2.1333 ticks; one rand() per member for the cry.
//
// mm8: 0x472e86 (Party_MoveIndoor)
func TestFallDamage(t *testing.T) {
	c := testCtx()
	p := knight() // max HP 60, Endurance 20 (bonus 2)
	p.HP = 60
	q := knight()
	q.HP = 60
	q.wear(1, SlotBoots, items.Item{Number: partytest.Ring, Special: 0x48})
	m := membersOf(p, q)
	m.FallDamage(1024, c)
	if got := m.Players[0]; got.HP != 60-24 || got.Recovery != 38 {
		t.Errorf("HP %d recovery %d, want 36, 38", got.HP, got.Recovery)
	}
	if got := m.Players[1]; got.HP != 60 || got.Recovery != 0 {
		t.Errorf("feather falling: HP %d recovery %d", got.HP, got.Recovery)
	}
	if got := c.Rand.Int(); got != 18467 {
		t.Errorf("next rand %d: one draw per hurt member", got)
	}
}

// Regeneration: items 1 HP and 1 SP, the skill its rank in HP, up to the maximum.
//
// mm8: 0x493a34 (Party_Regen)
func TestRegen(t *testing.T) {
	c := testCtx()
	p := Player{Class: 2, LevelBase: 2, BirthYear: clock.BaseYear - 20, HP: 10, SP: 5}
	*p.BasePtr(StatPersonality) = 10
	p.wear(1, SlotRing, items.Item{Number: partytest.Ring, Special: 0x42})
	p.Skills[SkillRegeneration] = SkillMaster | 1
	d := p
	d.Conditions[CondDead] = 1
	m := membersOf(p, d)
	m.RegenMembers(c)
	if got := m.Players[0]; got.HP != 10+1+3 || got.SP != 6 {
		t.Errorf("HP %d SP %d, want 14, 6", got.HP, got.SP)
	}
	if got := m.Players[1]; got.HP != 10 || got.SP != 6 {
		t.Errorf("dead: HP %d SP %d (SP regenerates anyway)", got.HP, got.SP)
	}
	m.Players[0].HP = 34
	m.RegenMembers(c)
	if got := m.Players[0].HP; got != 34 { // max HP 30 + (0 + 2) * 2 = 34, reached
		t.Errorf("capped HP %d", got)
	}
}

// The HP checks of the clock: dead below -Endurance, else unconscious at 0.
func TestVitals(t *testing.T) {
	c := testCtx()
	h := StatHooks{C: c}
	p := knight()
	p.HP = -25
	q := knight()
	q.HP = 0
	m := membersOf(p, q)
	h.M = m
	h.Vitals(m, 0)
	h.Vitals(m, 1)
	if m.Players[0].Conditions[CondDead] == 0 || m.Players[1].Conditions[CondUnconscious] == 0 {
		t.Errorf("dead %d unconscious %d", m.Players[0].Conditions[CondDead], m.Players[1].Conditions[CondUnconscious])
	}
	if got := h.MaxHP(&m.Players[0]); got != 60 {
		t.Errorf("hook MaxHP %d", got)
	}
}
