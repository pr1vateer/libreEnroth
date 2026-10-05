package items

import (
	"testing"

	"libre-enroth/internal/assets/tables"
	"libre-enroth/internal/game/party/partytest"
)

type namer struct{}

func (namer) Global(i int) string {
	if i == txtJarS {
		return "%s' Jar"
	}
	return "%s's Jar"
}
func (namer) MemberName(i int) string { return []string{"Zoltan", "Ariss"}[i] }

// Values and names.
//
// mm8: 0x4550e5, 0x45513c, 0x455156
func TestValueName(t *testing.T) {
	tb := partytest.Items()
	tb.Spc[0xf] = tables.SpcBonus{Name: "Vampiric", Value: 2000}
	for _, c := range []struct {
		it    Item
		value int32
		name  string
	}{
		{Item{Number: partytest.Sword}, 100, "?Sword"},
		{Item{Number: partytest.Sword, Flags: FlagIdentified}, 100, "Sword"},
		{Item{Number: partytest.Sword, Flags: FlagIdentified, Bonus: partytest.OfMight, Strength: 5}, 600, "Sword of Might"},
		{Item{Number: partytest.Sword, Flags: FlagIdentified, Special: partytest.OfGods}, 300, "Sword of The Gods"},
		{Item{Number: partytest.Sword, Flags: FlagIdentified, Special: 1}, 1100, "Sword of Protection"},
		{Item{Number: partytest.Sword, Flags: FlagIdentified | FlagTempBonus, Special: 1}, 100, "Sword of Protection"},
		{Item{Number: partytest.Sword, Flags: FlagIdentified, Special: 0x10}, 2100, "Vampiric Sword"},
		{Item{Number: partytest.Potion, Flags: FlagIdentified, Bonus: 12}, 10, "Potion"},
	} {
		if v, n := c.it.Value(tb), c.it.Name(tb, namer{}); v != c.value || n != c.name {
			t.Errorf("%v: %d %q, want %d %q", c.it, v, n, c.value, c.name)
		}
	}
	tb.Items[0].Name = "Lich Jar" // item 601 reads the empty row here
	j := Item{Number: LichJar, Flags: FlagIdentified, Owner: 2}
	if n := j.Name(tb, namer{}); n != "Ariss' Jar" {
		t.Errorf("jar %q", n)
	}
	j.Owner = 5
	if n := j.Name(tb, namer{}); n != "Lich Jar" {
		t.Errorf("jar of member 5 %q", n)
	}
}

type rng struct{ s uint32 }

func (r *rng) Int() int {
	r.s = r.s*0x343fd + 0x269ec3
	return int(r.s >> 16 & 0x7fff)
}

// The generator, traced by hand against ItemGen_Generate with srand(1): rand() gives
// 41, 18467, 6334, 26500, 19169 ...
//
// mm8: 0x4552ca
func TestGenerate(t *testing.T) {
	tb := partytest.Items()
	gen := func(level, kind int, force bool) (Item, int) {
		r := &rng{s: 1}
		it := Generate(tb, level, kind, force, r, &[NumArtifacts]bool{})
		return it, r.Int()
	}
	for _, c := range []struct {
		name        string
		level, kind int
		force       bool
		want        Item
		next        int
	}{
		// 41: the artifact roll; 18467 % 60 = 47 walks past 10 and 30 to the ring;
		// level 1 has no standard bonuses.
		{"level 1", 1, KindAny, false, Item{Number: partytest.Ring, Flags: FlagIdentified}, 6334},
		// weapons at level 3: the sword (5) and the spear (0); 41 % 5; 18467 % 100 = 67
		// is not below the 10 % of a special bonus.
		{"weapon", 3, 0x14, false, Item{Number: partytest.Sword}, 6334},
		// forced: of the gods, the only level A-B weapon bonus (6334 % 5 + 1 = 5).
		{"weapon forced", 3, 0x14, true, Item{Number: partytest.Sword, Special: partytest.OfGods}, 26500},
		// armour at level 2 weighs nothing: no draw; 41 % 100 >= 40 and no specials.
		{"armour", 2, 0x15, false, Item{Number: partytest.Chain}, 18467},
		// forced: 18467 % 40 = 27 < 40, standard; 6334 % 15 = 4 -> of Might;
		// strength 26500 % 5 + 1.
		{"armour forced", 2, 0x15, true, Item{Number: partytest.Chain, Bonus: partytest.OfMight, Strength: 1}, 19169},
		// a potion's power: (41 % 4 + 1 + 18467 % 4 + 1) * 2
		{"potion", 2, 0x2c, false, Item{Number: partytest.Potion, Bonus: 12, Flags: FlagIdentified}, 6334},
		// a wand: 41 % 6 + 1 + mod2 2 charges
		{"wand", 2, 0x2a, false, Item{Number: partytest.Wand, Charges: 8, MaxCharges: 8}, 18467},
	} {
		it, next := gen(c.level, c.kind, c.force)
		if it != c.want || next != c.next {
			t.Errorf("%s: %v (next rand %d), want %v (%d)", c.name, it, next, c.want, c.next)
		}
	}
	// Level 6 hands out an artifact not yet found one time in 20.
	var found [NumArtifacts]bool
	for i := range found {
		found[i] = i != 3
	}
	for s := uint32(1); ; s++ {
		r := &rng{s: s}
		if r.Int()%100 >= 5 {
			continue
		}
		it := Generate(tb, 6, KindAny, false, &rng{s: s}, &found)
		if it.Number != FirstArtifact+3 || !found[3] {
			t.Errorf("seed %d: %v", s, it)
		}
		break
	}
}

func TestGrid(t *testing.T) {
	var g Grid
	if !g.Fits(0, 14, 9) || g.Fits(1, 14, 1) || g.Fits(14*8, 1, 2) {
		t.Error("bounds")
	}
	g.Place(15, 2, 2, 7)
	if g[15] != 7 || g[16] != -16 || g[29] != -16 || g[30] != -16 || g.Fits(16, 1, 1) {
		t.Errorf("place %v", g[14:32])
	}
	if c := g.Free(2, 2); c != 28+14 {
		t.Errorf("free 2x2 at %d", c)
	}
	g.Clear(15, 2, 2)
	if g != (Grid{}) {
		t.Error("clear")
	}
}
