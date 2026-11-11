package world

import (
	"testing"

	"libre-enroth/internal/game/items"
	"libre-enroth/internal/game/monsters"
	"libre-enroth/internal/game/party"
	"libre-enroth/internal/game/ui"
)

// A blow kills a living monster of out01, the party gains its experience and the
// corpse can be searched.
//
// mm8: 0x4378ec, 0x4255bb, 0x4256b6
func TestKillMonsterOut01(t *testing.T) {
	e := newEnv(t)
	s := quietSession()
	s.Ctx.Items, s.Ctx.Classes = e.tables.Game.Items, e.tables.Game.Classes
	e.global(t, s)
	w := e.load(t, "out01.odm", s)
	i := -1
	for n := range w.Actors() {
		a := &w.Actors()[n]
		if a.CanAct() && a.HP > 5 && a.Info.Exp > 0 && a.NPC == 0 {
			i = n
			break
		}
	}
	if i < 0 {
		t.Fatal("no monster")
	}
	a := &w.Actors()[i]
	p := w.PartyState()
	p.Teleport(int32(a.Pos[0])+20, int32(a.Pos[1]), int32(a.Pos[2]), 0)
	pl := &s.Party.Players[0]
	pl.Skills = [party.NumSkills]uint16{}
	pl.Skills[party.SkillUnarmed] = 40
	pl.Equip = [party.NumSlots]int32{}
	pl.Stats = [7]party.StatPair{}
	before := pl.Exp
	a.Info.AC = -100
	a.Info.Resist = [10]uint16{}
	a.HP = 1
	w.brain().HitFromParty(0, i, monsters.BlowDirection(p.X, p.Y, p.Z, a))
	if a.AIState != monsters.Dying || pl.Exp <= before {
		t.Fatalf("state %d HP %d exp %d -> %d (monster %d exp %d)", a.AIState, a.HP, before, pl.Exp, a.ID(), a.Info.Exp)
	}
	w.searchCorpse(i)
}

// A bow from the skill items fires, and the arrow kills a monster in out01 whose
// corpse can be searched. A wand spends one charge.
//
// mm8: 0x42e27f, 0x4263ca, 0x42685f, 0x46bd3a
func TestBowAndWand(t *testing.T) {
	e := newEnv(t)
	s := quietSession()
	s.Ctx.Items, s.Ctx.Classes = e.tables.Game.Items, e.tables.Game.Classes
	e.global(t, s)
	w := e.load(t, "out01.odm", s)
	i := -1
	for n := range w.Actors() {
		a := &w.Actors()[n]
		if a.CanAct() && a.HP > 5 && a.Info.Exp > 0 && a.NPC == 0 {
			i = n
			break
		}
	}
	if i < 0 {
		t.Fatal("no monster")
	}
	a := &w.Actors()[i]
	p := w.PartyState()
	p.Teleport(int32(a.Pos[0])-80, int32(a.Pos[1]), int32(a.Pos[2]), 0)
	pl := &s.Party.Players[0]
	pl.Skills = [party.NumSkills]uint16{}
	pl.Skills[party.SkillBow] = 5
	pl.Equip = [party.NumSlots]int32{}
	s.Party.DebugEquip(0, s.Ctx)
	if pl.Equipped(party.SlotBow) == nil || pl.Equipped(party.SlotBow).Number != 0x38 {
		t.Fatalf("bow %+v", pl.Equipped(party.SlotBow))
	}
	a.Info.AC = -100
	a.Info.Resist = [10]uint16{}
	a.HP = 1
	w.AIFrozen = true
	for range 30 {
		w.Update(&ui.Input{})
	}
	w.Update(&ui.Input{Keys: []ui.Key{'A'}})
	for range 40 {
		w.Update(&ui.Input{})
	}
	if a.AIState != monsters.Dying && a.AIState != monsters.Dead && a.HP > 0 {
		t.Fatalf("bow: state %d HP %d objects %d", a.AIState, a.HP, len(w.Objects()))
	}
	w.searchCorpse(i)

	// A wand spends a charge and does not erase the item while charges remain.
	pl.Equip = [party.NumSlots]int32{}
	pl.Skills[party.SkillBow] = 0
	wand := items.Item{Number: 0x98, Charges: 3, MaxCharges: 3}
	pl.Equip[party.SlotMainHand] = pl.AddItem(s.Ctx.Items, -1, wand)
	s.Party.Selected = 1
	w.attack(0, 0)
	if got := pl.Item(pl.Equip[party.SlotMainHand]); got == nil || got.Charges != 2 {
		et := -1
		if got != nil && s.Ctx.Items != nil {
			et = int(got.Def(s.Ctx.Items).EquipType)
		}
		t.Fatalf("wand charges %d equip %d selected %d", got.Charges, et, s.Party.Selected)
	}
	w.casts.Dispatch(spellHost{w})
	n := 0
	for _, o := range w.Objects() {
		if o.ObjList != 0 && o.Spell == 2 {
			n++
		}
	}
	if n == 0 {
		t.Fatal("wand fired no fire bolt")
	}
}

// A chest trap at the party deals 5 plus the trap level, and a grandmaster of
// perception who rolls at least 0x15 is spared.
//
// mm8: 0x43726d
func TestChestTrap(t *testing.T) {
	e := newEnv(t)
	s := quietSession()
	s.Ctx.Items, s.Ctx.Classes = e.tables.Game.Items, e.tables.Game.Classes
	e.global(t, s)
	w := e.load(t, "out01.odm", s)
	pl := &s.Party.Players[0]
	pl.HP = 50
	pl.Conditions[party.CondDead] = 1
	w.trapDamage(0x32b, w.PartyState().X, w.PartyState().Y, w.PartyState().Z+w.PartyState().EyeLevel, 0)
	if pl.HP >= 50 {
		t.Fatalf("unspared HP %d", pl.HP)
	}
	pl.HP = 50
	pl.Conditions[party.CondDead] = 0
	pl.Skills[party.SkillPerception] = party.SkillGM | 1
	s.Ctx.Rand = party.NewRand(1) // first roll 41, which is at least 0x15
	w.trapDamage(0x32c, w.PartyState().X, w.PartyState().Y, w.PartyState().Z+w.PartyState().EyeLevel, 0)
	if pl.HP != 50 {
		t.Fatalf("perception save HP %d", pl.HP)
	}
}

// Nobody left who can act is revived at the temple: gold gone, an award noted, a
// week later, full health, and the actors sent home.
//
// mm8: 0x46261d (g_partyCreateResult 8)
func TestPartyDeadRevives(t *testing.T) {
	e := newEnv(t)
	s := quietSession()
	s.Ctx.Items, s.Ctx.Classes = e.tables.Game.Items, e.tables.Game.Classes
	e.global(t, s)
	w := e.load(t, "d05.blv", s)
	s.Party.Gold = 50
	s.Party.Players[0].Conditions[party.CondDead] = 1
	s.Party.Players[0].HP = 0
	w.Update(&ui.Input{})
	pl := &s.Party.Players[0]
	if s.Party.Deaths != 1 || s.Party.Gold != 0 || pl.Conditions[party.CondDead] != 0 || pl.HP < 1 {
		t.Fatalf("deaths %d gold %d dead %d HP %d", s.Party.Deaths, s.Party.Gold, pl.Conditions[party.CondDead], pl.HP)
	}
	if !pl.Awards.Get(0x2b) {
		t.Error("award 0x2b not set")
	}
	name, ok := w.Travel()
	if !ok || name != "out01.odm" {
		t.Fatalf("travel %q %v", name, ok)
	}
}
