package party

import (
	"testing"

	"libre-enroth/internal/game/party/partytest"
)

// The creation screen's point costs: the cheap Might of the test knight goes up 2 for
// a point, its dear Intellect 1 for two, and back; two below the base is the floor.
//
// mm8: 0x49170c, 0x491516, 0x491385
func TestPointCosts(t *testing.T) {
	cls := partytest.Classes()
	p := Player{Class: 4}
	p.ResetCreation(cls)
	if p.Base(StatMight) != 14 || p.Base(StatIntellect) != 7 || p.PointsLeft(cls) != 15 || p.BirthYear != NewBirthYear {
		t.Fatalf("reset: might %d int %d points %d", p.Base(StatMight), p.Base(StatIntellect), p.PointsLeft(cls))
	}
	steps := []struct {
		up     bool
		s      Stat
		ok     bool
		val    int
		points int
	}{
		{true, StatMight, true, 16, 14},
		{true, StatIntellect, true, 8, 12},
		{false, StatIntellect, true, 7, 14},
		{false, StatIntellect, true, 5, 15},
		{false, StatIntellect, false, 5, 15},
		{true, StatIntellect, true, 7, 14},
		{false, StatMight, true, 14, 15},
		{false, StatMight, true, 13, 17}, // below the base a cheap stat gives 2 a step
		{false, StatMight, true, 12, 19},
		{false, StatMight, false, 12, 19},
	}
	for i, s := range steps {
		var ok bool
		if s.up {
			ok = p.StatUp(cls, s.s)
		} else {
			ok = p.StatDown(cls, s.s)
		}
		if ok != s.ok || p.Base(s.s) != s.val || p.PointsLeft(cls) != s.points {
			t.Fatalf("step %d: ok %v value %d points %d, want %v %d %d", i, ok, p.Base(s.s), p.PointsLeft(cls), s.ok, s.val, s.points)
		}
	}
	for p.StatUp(cls, StatLuck) {
	}
	for p.StatUp(cls, StatSpeed) {
	}
	if p.PointsLeft(cls) != 0 || p.Base(StatLuck) != 25 || p.Base(StatSpeed) != 11+5 {
		t.Errorf("spent: points %d luck %d speed %d", p.PointsLeft(cls), p.Base(StatLuck), p.Base(StatSpeed))
	}
	if p.StatUp(cls, StatMight) {
		t.Error("raised with no points")
	}
}

// The skill slots: two from the class, two chosen, nine choosable.
//
// mm8: 0x4912b0, 0x4916e0, 0x433bbd (msgs 0x40, 0x1cd, 0x1ce)
func TestCreationSkills(t *testing.T) {
	cls := partytest.Classes()
	p := Player{Class: 4}
	p.ResetCreation(cls)
	got := []int{}
	for n := 0; n < 13; n++ {
		got = append(got, p.CreationSkill(cls, n))
	}
	want := []int{1, 9, SkillNone, SkillNone, 4, 8, 10, 11, 25, SkillNone, SkillNone, SkillNone, SkillNone}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("slots %v, want %v", got, want)
		}
	}
	p.ChooseSkill(cls, 3) // plate
	p.ChooseSkill(cls, 0) // spear
	if p.CreationSkill(cls, 2) != 4 || p.CreationSkill(cls, 3) != 11 || !p.HasTwoExtraSkills() {
		t.Errorf("chosen %d %d", p.CreationSkill(cls, 2), p.CreationSkill(cls, 3))
	}
	p.ChooseSkill(cls, 1) // shield: both taken
	if p.Skills[8] != 0 {
		t.Error("a third extra skill")
	}
	p.DropSkill(cls, 2)
	if p.Skills[4] != 0 || p.CreationSkill(cls, 2) != 11 || p.HasTwoExtraSkills() {
		t.Error("drop")
	}
	p.DropSkill(cls, 3) // empty: nothing
}
