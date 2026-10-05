package party

import (
	"testing"

	"libre-enroth/internal/game/clock"
	"libre-enroth/internal/game/items"
	"libre-enroth/internal/game/party/partytest"
)

// testEnv is the made-up tables at game time 0.
func testEnv() *Env {
	return &Env{Items: partytest.Items(), Classes: partytest.Classes(), Buffs: &[NumPartyBuffs]Buff{}}
}

// testCtx is a Ctx with the made-up tables and rand() seeded with 1.
func testCtx() *Ctx {
	return &Ctx{Rand: NewRand(1), Items: partytest.Items(), Classes: partytest.Classes()}
}

// knight is a level-3 knight of 20 years with every stat at 10 and Endurance 20.
func knight() Player {
	p := Player{Class: 4, LevelBase: 3, BirthYear: clock.BaseYear - 20}
	for i := range p.Stats {
		p.Stats[i].Base = 10
	}
	*p.BasePtr(StatEndurance) = 20
	return p
}

// wear puts item id in item slot n (1-based) worn in equipment slot s.
func (p *Player) wear(n int32, s int, it items.Item) {
	p.Items[n-1] = it
	p.Equip[s] = n
}

// ActualStat: base * age % * condition % + bonus + items + buffs.
//
// mm8: 0x48dfc4, 0x48fd79, 0x48fd62
func TestActualStat(t *testing.T) {
	e := testEnv()
	p := Player{Class: 4, BirthYear: clock.BaseYear - 60}
	p.Stats[0] = StatPair{Base: 20, Bonus: 3}
	if got := p.Age(e); got != 60 {
		t.Fatalf("age %d", got)
	}
	check := func(want int, what string) {
		t.Helper()
		if got := p.ActualStat(e, StatMight); got != want {
			t.Errorf("%s: might %d, want %d", what, got, want)
		}
	}
	check(3+15, "60 years (75 %)")
	p.Conditions[CondWeak] = 1
	check(3+50*15/100, "weak (50 %)")
	p.wear(1, SlotRing, items.Item{Number: partytest.Ring, Bonus: partytest.OfMight, Strength: 5})
	check(10+5, "ring of might +5")
	if got := p.BaseStat(e, StatMight); got != 25 {
		t.Errorf("base stat %d, want 25", got)
	}
	p.Items[0].Flags |= items.FlagBroken
	check(10, "broken ring")
	e.Buffs[2].Power = 4 // Day of the Gods
	check(14, "party buff")
	p.AgeBonus = 50 // 110 years: 40 %
	check(4+3+50*(40*20/100)/100, "aged")
}

// The two stat orders: codes put Accuracy before Speed, the struct Speed first.
func TestStatOrder(t *testing.T) {
	var p Player
	*p.BasePtr(StatAccuracy) = 7
	*p.BasePtr(StatSpeed) = 9
	if p.Stats[4].Base != 9 || p.Stats[5].Base != 7 {
		t.Errorf("struct %+v", p.Stats)
	}
}

// MaxHP and MaxSP per class.
//
// mm8: 0x48f5b9, 0x48f61d, 0x490cc3 (params 7, 8)
func TestMaxHPSP(t *testing.T) {
	e := testEnv()
	p := knight()
	if got := p.MaxHP(e); got != 35+(2+3)*5 {
		t.Errorf("knight max HP %d, want 60", got)
	}
	p.Skills[SkillBodybuilding] = SkillExpert | 2
	if got := p.MaxHP(e); got != 60+2*2*5 {
		t.Errorf("bodybuilding: %d, want 80", got)
	}
	p.wear(1, SlotRing, items.Item{Number: partytest.Ring, Bonus: partytest.OfHealth, Strength: 7})
	if got := p.MaxHP(e); got != 87 {
		t.Errorf("ring of health: %d, want 87", got)
	}
	if got := p.MaxSP(e); got != 0 {
		t.Errorf("knight SP %d", got)
	}
	c := Player{Class: 2, LevelBase: 2, BirthYear: clock.BaseYear - 20}
	*c.BasePtr(StatPersonality) = 10
	if got := c.MaxSP(e); got != (1+2)*3+15 {
		t.Errorf("cleric SP %d, want 24", got)
	}
	c.Skills[SkillMeditation] = SkillMaster | 3
	if got := c.MaxSP(e); got != 24+3*3*3 {
		t.Errorf("meditation: %d, want 51", got)
	}
	n := Player{Class: 0, LevelBase: 1, BirthYear: clock.BaseYear - 20}
	*n.BasePtr(StatIntellect) = 25
	if got := n.MaxSP(e); got != (2+1)*4+25 {
		t.Errorf("necromancer SP %d, want 37", got)
	}
	if got := n.MaxHP(e); got != 20+(0+1)*3 {
		t.Errorf("necromancer HP %d, want 23", got)
	}
}

// AC: StatToBonus(Speed) + armour + armour skills (+ dodging) + AC bonus.
//
// mm8: 0x48f6f7, 0x490cc3 (param 9)
func TestAC(t *testing.T) {
	e := testEnv()
	p := knight()
	if got := p.AC(e); got != 1 {
		t.Errorf("bare AC %d", got)
	}
	p.Skills[SkillDodge] = SkillExpert | 5
	if got := p.AC(e); got != 1+5+5 {
		t.Errorf("dodging expert: %d, want 11", got)
	}
	p.wear(1, SlotArmor, items.Item{Number: partytest.Chain})
	p.Skills[SkillChain] = SkillExpert | 4
	if got := p.AC(e); got != 1+12+4 {
		t.Errorf("chain: %d, want 17 (no dodging in chain)", got)
	}
	p.ACBonus = 3
	if got := p.AC(e); got != 20 {
		t.Errorf("AC bonus: %d", got)
	}
	p.ACBonus = 0
	p.wear(1, SlotArmor, items.Item{Number: partytest.Leather})
	p.Skills[SkillLeather] = SkillMaster | 3
	p.Skills[SkillDodge] = 5
	if got := p.AC(e); got != 1+4+3+3 {
		t.Errorf("leather master: %d, want 11", got)
	}
	p.Skills[SkillDodge] = SkillGM | 5
	if got := p.AC(e); got != 11+5*3 {
		t.Errorf("leather + dodging GM: %d, want 26", got)
	}
	p.Items[0].Flags |= items.FlagBroken
	if got := p.AC(e); got != 1+5*3 {
		t.Errorf("broken leather: %d, want 16", got)
	}
}

// Melee attack and damage, the weapon skills, spear in both hands, unarmed, armsmaster.
//
// mm8: 0x48e24e, 0x48e293, 0x48e2d5, 0x48fe4d, 0x490cc3 (0x19, 0x1a)
func TestMelee(t *testing.T) {
	e := testEnv()
	p := knight()
	*p.BasePtr(StatMight) = 20
	p.wear(1, SlotMainHand, items.Item{Number: partytest.Sword})
	p.Skills[SkillSword] = 3
	if lo, hi := p.MeleeDamageMin(e), p.MeleeDamageMax(e); lo != 2+3 || hi != 2+9 {
		t.Errorf("sword damage %d-%d, want 5-11", lo, hi)
	}
	if got := p.MeleeAttack(e, false); got != 1+1+3 {
		t.Errorf("sword attack %d, want 5", got)
	}
	if got := p.MeleeDamageText(e, nil); got != "5 - 11" {
		t.Errorf("text %q", got)
	}
	p.Skills[SkillArmsmaster] = SkillMaster | 4
	if got, lo := p.MeleeAttack(e, false), p.MeleeDamageMin(e); got != 9 || lo != 9 {
		t.Errorf("armsmaster master: attack %d (want 9), damage %d (want 9)", got, lo)
	}
	p.Skills[SkillArmsmaster] = 0
	p.wear(1, SlotMainHand, items.Item{Number: partytest.Spear})
	p.Skills[SkillSpear] = SkillExpert | 4
	if lo, hi := p.MeleeDamageMin(e), p.MeleeDamageMax(e); lo != 2+2+4 || hi != 2+12+4 {
		t.Errorf("two-handed spear %d-%d, want 8-18", lo, hi)
	}
	p.wear(2, SlotOffhand, items.Item{Number: partytest.Shield})
	if lo, hi := p.MeleeDamageMin(e), p.MeleeDamageMax(e); lo != 2+1+4 || hi != 2+6+4 {
		t.Errorf("spear and shield %d-%d, want 7-12", lo, hi)
	}
	var u Player = knight()
	*u.BasePtr(StatMight) = 20
	if lo, hi := u.MeleeDamageMin(e), u.MeleeDamageMax(e); lo != 3 || hi != 5 {
		t.Errorf("unarmed %d-%d, want 3-5", lo, hi)
	}
	u.Skills[SkillUnarmed] = SkillExpert | 6
	if lo, a := u.MeleeDamageMin(e), u.MeleeAttack(e, false); lo != 9 || a != 1+6 {
		t.Errorf("unarmed expert: damage %d (want 9), attack %d (want 7)", lo, a)
	}
}

// Ranged values: the bow's dice and a grandmaster's level.
//
// mm8: 0x48e5ba, 0x48e5f3, 0x48e65b
func TestRanged(t *testing.T) {
	e := testEnv()
	p := knight()
	if got := p.RangedDamageText(e, nil); got != "N/A" {
		t.Errorf("no bow: %q", got)
	}
	p.wear(1, SlotBow, items.Item{Number: partytest.Bow})
	p.Skills[SkillBow] = SkillGM | 5
	if lo, hi, a := p.RangedDamageMin(e), p.RangedDamageMax(e), p.RangedAttack(e); lo != 3+1+5 || hi != 1+6+5 || a != 1+1+5 {
		t.Errorf("bow GM %d-%d attack %d, want 9-12 attack 7", lo, hi, a)
	}
}

// Resistances: base, race, items, the leather grandmaster, the immunities.
//
// mm8: 0x48f96f, 0x48f7a7
func TestResist(t *testing.T) {
	e := testEnv()
	n := Player{Class: 0}
	n.Resists[DamageFire] = 10
	if got := n.Resist(e, StatFireRes); got != 10 {
		t.Errorf("necromancer fire %d", got)
	}
	if got := n.Resist(e, StatSpiritRes); got != 5 {
		t.Errorf("necromancer spirit %d (race +5)", got)
	}
	n.wear(1, SlotRing, items.Item{Number: partytest.Ring, Special: 1})
	if got := n.Resist(e, StatFireRes); got != 20 {
		t.Errorf("ring of protection: %d", got)
	}
	for _, c := range []struct {
		class int
		s     Stat
		want  int
	}{{12, StatMindRes, ImmuneResist}, {1, StatBodyRes, ImmuneResist}, {1, StatMindRes, ImmuneResist},
		{10, StatFireRes, 5}, {6, StatEarthRes, 5}, {6, StatBodyRes, 5}, {8, StatMindRes, 5}} {
		p := Player{Class: c.class}
		if got := p.Resist(e, c.s); got != c.want {
			t.Errorf("class %d stat %#x: %d, want %d", c.class, c.s, got, c.want)
		}
	}
	k := knight()
	k.Skills[SkillLeather] = SkillGM | 7
	k.wear(1, SlotArmor, items.Item{Number: partytest.Leather})
	if got := k.Resist(e, StatWaterRes); got != 7 {
		t.Errorf("leather GM water %d", got)
	}
	if got := k.BaseResist(e, StatWaterRes); got != 0 {
		t.Errorf("leather GM base water %d (the bonus is not part of it)", got)
	}
}

// The item bonus switch: "of the Gods", "of Doom", the skill bonuses' maximum, the
// half skill of "of Air Magic", an artifact.
//
// mm8: 0x48fe4d
func TestItemBonus(t *testing.T) {
	e := testEnv()
	p := knight()
	p.wear(1, SlotRing, items.Item{Number: partytest.Ring, Special: partytest.OfGods})
	p.wear(2, SlotRing+1, items.Item{Number: partytest.Ring, Special: partytest.OfDoom})
	if got := p.ItemBonus(e, StatLuck, false); got != 11 {
		t.Errorf("gods + doom luck %d", got)
	}
	if got := p.ItemBonus(e, StatFireRes, false); got != 1 {
		t.Errorf("doom fire %d", got)
	}
	// a skill's standard bonus counts only with the skill, the highest one
	p.wear(3, SlotRing+2, items.Item{Number: partytest.Ring, Bonus: int32(StatArmsmaster) + 1, Strength: 4})
	p.wear(4, SlotRing+3, items.Item{Number: partytest.Ring, Bonus: int32(StatArmsmaster) + 1, Strength: 6})
	if got := p.ItemBonus(e, StatArmsmaster, false); got != 0 {
		t.Errorf("armsmaster without the skill %d", got)
	}
	p.Skills[SkillArmsmaster] = 1
	if got := p.ItemBonus(e, StatArmsmaster, false); got != 6 {
		t.Errorf("armsmaster %d, want the best (6)", got)
	}
	if got := p.Skill(e, SkillArmsmaster); got != 7 {
		t.Errorf("Skill(armsmaster) %d", got)
	}
	p.Skills[SkillFire+1] = SkillExpert | 9
	p.wear(5, SlotAmulet, items.Item{Number: partytest.Ring, Special: partytest.OfAirMagi})
	if got := p.Skill(e, SkillFire+1); got != SkillExpert|(9+4) {
		t.Errorf("air magic %#x, want expert 13", got)
	}
	p.Skills[SkillFire+1] = 0x3a // the level stops at 60
	if got := p.Skill(e, SkillFire+1); got != 0x3c {
		t.Errorf("air magic capped %#x", got)
	}
	a := knight()
	a.wear(1, SlotMainHand, items.Item{Number: 0x219})
	if got := a.ItemBonus(e, StatMight, false); got != 0 {
		t.Errorf("artifact 0x219 of an ordinary item row (not rare) %d", got)
	}
	e.Items.Items[0].Material = 1 // item 0x219 reads row 0 here: make it an artifact
	if got, ac := a.ItemBonus(e, StatMight, false), a.ItemBonus(e, StatAC, false); got != 100 || ac != -15 {
		t.Errorf("artifact 0x219: might %d (100), AC %d (-15)", got, ac)
	}
}

// Merchant: (rank - 1) * level + 7 + level - reputation; GM 10000; the buff's power.
//
// mm8: 0x491f5e
func TestMerchantDiscount(t *testing.T) {
	e := testEnv()
	p := knight()
	if got := p.MerchantDiscount(e, 5); got != -5 {
		t.Errorf("no skill %d", got)
	}
	p.Skills[SkillMerchant] = SkillExpert | 4
	if got := p.MerchantDiscount(e, 5); got != 4-5+7+4 {
		t.Errorf("expert 4: %d, want 10", got)
	}
	p.Buffs[BuffMerchant] = Buff{Expires: 1, Power: 3}
	if got := p.MerchantDiscount(e, 5); got != 4-5+7+7 {
		t.Errorf("with the buff: %d, want 13", got)
	}
	p.Skills[SkillMerchant] = SkillGM | 1
	if got := p.MerchantDiscount(e, 5); got != 10000 {
		t.Errorf("GM %d", got)
	}
}

// Level, training, the other skill values.
//
// mm8: 0x48df95, 0x48e90c, 0x491fb0, 0x491fed, 0x49203f
func TestLevelAndSkills(t *testing.T) {
	e := testEnv()
	p := knight()
	p.LevelBonus = 2
	if got := p.Level(e); got != 5 {
		t.Errorf("level %d", got)
	}
	p.Exp = 5999
	if p.CanTrain() {
		t.Error("trains at 5999 for level 4")
	}
	p.Exp = 6000
	if !p.CanTrain() {
		t.Error("cannot train at 6000")
	}
	p.Skills[SkillLearning] = SkillMaster | 2
	if got := p.LearningBonus(e); got != (3-1)*2+9+2 {
		t.Errorf("learning %d", got)
	}
	p.Skills[SkillPerception] = SkillExpert | 3
	if got := p.Perception(e); got != 3+3 {
		t.Errorf("perception %d", got)
	}
}

func TestMastery(t *testing.T) {
	for _, c := range []struct {
		sk         uint16
		rank, mult int
	}{{0, 1, 1}, {5, 1, 1}, {0x45, 2, 2}, {0x85, 3, 3}, {0x105, 4, 5}, {0xc5, 3, 3}} {
		if Mastery(c.sk) != c.rank || MasteryMult(c.sk) != c.mult {
			t.Errorf("%#x: %d/%d, want %d/%d", c.sk, Mastery(c.sk), MasteryMult(c.sk), c.rank, c.mult)
		}
	}
}
