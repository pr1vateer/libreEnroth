package world

import (
	"testing"

	"libre-enroth/internal/assets/tables"
	"libre-enroth/internal/game/party"
	"libre-enroth/internal/game/ui"
)

// statSession is a quiet session with the real item and class tables and a party of
// faces, each with its class defaults.
func (e *env) statSession(faces ...int) *Session {
	s := quietSession()
	c := s.Ctx
	c.Items, c.Classes = e.tables.Game.Items, e.tables.Game.Classes
	c.Hooks = party.StatHooks{M: s.Party, C: c}
	s.Party.Players = nil
	for _, f := range faces {
		s.Party.Players = append(s.Party.Players, party.Player{Face: f, Voice: f, Class: ui.ClassForFace(f)})
	}
	for i := range s.Party.Players {
		s.Party.DefaultMember(i, c)
	}
	return s
}

// The 50 roster characters: stats, skills, spells and equipment from roster.txt.
//
// mm8: 0x49680a (Txt_LoadRoster)
func TestRosterStats(t *testing.T) {
	e := newEnv(t)
	s := e.statSession(0)
	m := s.Party
	r := party.NewRoster(e.tables.Game.Roster, s.Ctx, &m.ArtifactsFound)
	if len(r) != tables.NumRoster {
		t.Fatalf("%d roster characters", len(r))
	}
	env := &party.Env{Items: s.Ctx.Items, Classes: s.Ctx.Classes}
	worn, packed := 0, 0
	for i := range r {
		p := &r[i]
		if p.LevelBase < 1 || p.HP != p.MaxHP(env) || p.HP < 1 || p.SP != p.MaxSP(env) {
			t.Errorf("%d %s: level %d HP %d/%d SP %d/%d", i, p.Name, p.LevelBase, p.HP, p.MaxHP(env), p.SP, p.MaxSP(env))
		}
		for _, n := range p.Equip {
			if n != 0 {
				worn++
				if it := p.Item(n); it.Number == 0 || !it.Identified() {
					t.Errorf("%s: worn slot %d holds %v", p.Name, n, it)
				}
			}
		}
		for _, c := range p.Grid {
			if c > 0 {
				packed++
			}
		}
	}
	d := &r[1]
	// roster.txt row 1: fire, air, water, earth B 1, dark B 2; two fire and air spells, three
	// dark ones; a staff, a dagger and leather, potions and scrolls.
	if d.Name != "Devlin Arcanus" || d.Class != 0 || d.Skills[party.SkillFire] != 1 || d.Skills[party.SkillFire+8] != 2 ||
		!d.Spells[1] || d.Spells[2] || !d.Spells[12] || !d.Spells[8*11+2] || d.Spells[8*11+3] || d.SP < 1 || d.Exp != 10000 {
		t.Errorf("Devlin: class %d fire %#x dark %#x spells %v SP %d", d.Class, d.Skills[party.SkillFire], d.Skills[party.SkillFire+8], d.Spells, d.SP)
	}
	if d.Equip[party.SlotMainHand] == 0 || d.Equip[party.SlotArmor] == 0 {
		t.Errorf("Devlin wears %v", d.Equip)
	}
	t.Logf("%d items worn, %d packed; Devlin: HP %d SP %d AC %d, %s",
		worn, packed, d.HP, d.SP, d.AC(env), d.MeleeDamageText(env, func(int) string { return "" }))
	if worn == 0 {
		t.Error("nobody wears anything")
	}
}

// Every class at level 1 with its base stats has HP, and spell points unless it is a
// knight or a troll.
//
// mm8: 0x48f5b9, 0x48f61d
func TestClassHPSP(t *testing.T) {
	e := newEnv(t)
	s := e.statSession()
	env := &party.Env{Items: s.Ctx.Items, Classes: s.Ctx.Classes}
	for cl := 0; cl < tables.NumClasses; cl++ {
		p := party.Player{Class: cl, LevelBase: 1, BirthYear: party.NewBirthYear}
		p.ResetCreation(s.Ctx.Classes)
		hp, sp := p.MaxHP(env), p.MaxSP(env)
		caster := !(cl >= 4 && cl <= 7)
		if hp < 1 || (sp > 0) != caster {
			t.Errorf("class %d: HP %d SP %d", cl, hp, sp)
		}
		t.Logf("class %2d: HP %3d SP %3d AC %d", cl, hp, sp, p.AC(env))
	}
}

// The end of party creation: the ring, each skill's item, the first spells, all
// identified, HP and SP full.
//
// mm8: 0x495cb6 (PartyCreation_Run)
func TestFinishCreation(t *testing.T) {
	e := newEnv(t)
	s := e.statSession(8) // a necromancer
	m, c := s.Party, s.Ctx
	h := &m.Players[0]
	h.ChooseSkill(c.Classes, 0)
	h.ChooseSkill(c.Classes, 1)
	m.FinishCreation(c)
	env := m.Env(c)
	var got []int32
	for _, it := range h.Items {
		if it.Number != 0 {
			got = append(got, it.Number)
			if !it.Identified() {
				t.Errorf("%d not identified", it.Number)
			}
		}
	}
	if len(got) < 3 || c.Items.Item(got[0]).EquipType != tables.EquipRing {
		t.Errorf("pack %v", got)
	}
	if h.HP != h.MaxHP(env) || h.SP != h.MaxSP(env) || h.SP < 1 {
		t.Errorf("HP %d/%d SP %d/%d", h.HP, h.MaxHP(env), h.SP, h.MaxSP(env))
	}
	t.Logf("necromancer skills %v: items %v, spells %v", h.Skills[:24], got, h.Spells[:24])
	var shuffled [32]bool
	for _, k := range m.Shuffle {
		shuffled[k] = true
	}
	if shuffled != [32]bool{true, true, true, true, true, true, true, true, true, true, true, true, true, true, true, true,
		true, true, true, true, true, true, true, true, true, true, true, true, true, true, true, true} {
		t.Errorf("shuffle %v is not a permutation", m.Shuffle)
	}
}
