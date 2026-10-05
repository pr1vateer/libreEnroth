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

// A 9 × 9 chest: fitting, putting at the first place, taking, placing on first open
// with the shuffled cells, and the record's round trip.
//
// mm8: 0x41fa59, 0x41fb8b, 0x420842, 0x41fef0, 0x41fd30
func TestChest(t *testing.T) {
	tb := partytest.Items()
	g := tables.ChestGrid{X: 42, Y: 49, W: 9, H: 9}
	var c Chest
	if !c.Fits(tb, g, 0, partytest.Chain) || c.Fits(tb, g, 8, partytest.Chain) || c.Fits(tb, g, 9*7, partytest.Chain) {
		t.Fatal("fits at the edges")
	}
	ok, full := c.Put(tb, g, Item{Number: partytest.Chain})
	if !ok || full || c.Grid[0] != 1 || c.Grid[1] != -1 || c.Grid[19] != -1 || c.Grid[2] != 0 {
		t.Fatalf("put: %v %v %v", ok, full, c.Grid[:20])
	}
	ok, _ = c.Put(tb, g, Item{Number: partytest.Sword})
	if !ok || c.Grid[2] != 2 {
		t.Fatalf("second put at %v", c.Grid[:4])
	}
	if it := c.Take(tb, g, 1); it.Number != 0 {
		t.Errorf("took from a covered cell: %v", it)
	}
	if it := c.Take(tb, g, 0); it.Number != partytest.Chain || c.Grid[0] != 0 || c.Grid[19] != 0 || c.Items[0].Number != 0 {
		t.Fatalf("take: %v", it)
	}
	// First opening: wands get charges, gold an amount, ChestIdentify identifies.
	var d Chest
	d.Flags = ChestIdentify
	d.Items[0] = Item{Number: partytest.Sword}
	d.Items[1] = Item{Number: partytest.Wand}
	d.PlaceItems(tb, g, &rng{s: 1})
	if d.Flags != ChestPlaced {
		t.Errorf("flags %#x", d.Flags)
	}
	placed := 0
	for _, n := range d.Grid {
		if n > 0 {
			placed++
		}
	}
	if placed != 2 || !d.Items[0].Identified() {
		t.Fatalf("placed %d, identified %v", placed, d.Items[0].Identified())
	}
	// The partytest wand is id 10, not a game wand: no charges.
	if d.Items[1].Charges != 0 {
		t.Errorf("charges %d", d.Items[1].Charges)
	}
	b := make([]byte, ChestSize)
	d.Encode(b)
	if e := DecodeChest(b); e != d {
		t.Error("round trip")
	}
}

// Random chest items: one rand() % 100 decides gold, nothing or an item for every
// random item; a treasure level 7 is an artifact.
//
// mm8: 0x44ec9d (Chests_GenerateItems), 0x44f074
func TestGenerateChests(t *testing.T) {
	tb := partytest.Items()
	ct := &tables.Chests{}
	for n := range ct.Treasure {
		for l := range ct.Treasure[n] {
			ct.Treasure[n][l] = [2]uint8{1, 1}
		}
	}
	ct.Treasure[6][0] = [2]uint8{7, 7}
	cs := make([]Chest, 2)
	cs[0].Items[0].Number = -1
	cs[1].Items[0].Number = -7
	var found [NumArtifacts]bool
	r := &rng{s: 1}
	GenerateChests(cs, tb, ct, 0, r, &found)
	for i := range cs {
		for _, it := range cs[i].Items {
			if it.Number < 0 {
				t.Fatalf("chest %d keeps a random item", i)
			}
		}
	}
	if n := cs[1].Items[0].Number; n < FirstArtifact || n >= FirstArtifact+NumArtifacts || !found[n-FirstArtifact] {
		t.Errorf("level 7: %v", cs[1].Items[0])
	}
	// The same seed gives the same chests.
	cs2 := make([]Chest, 2)
	cs2[0].Items[0].Number, cs2[1].Items[0].Number = -1, -7
	var found2 [NumArtifacts]bool
	GenerateChests(cs2, tb, ct, 0, &rng{s: 1}, &found2)
	if cs2[0] != cs[0] || cs2[1] != cs[1] {
		t.Error("not reproducible")
	}
}

// A temporary bonus past its time goes.
//
// mm8: 0x456f74
func TestExpireBonus(t *testing.T) {
	it := Item{Number: 1, Special: 5, Flags: FlagTempBonus | FlagIdentified, Expires: 100}
	it.ExpireBonus(100)
	if it.Special != 5 {
		t.Error("expired at its time")
	}
	it.ExpireBonus(101)
	if it.Special != 0 || it.Flags != FlagIdentified {
		t.Errorf("not expired: %v", it)
	}
}
