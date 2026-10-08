package tables

import (
	"strings"
	"testing"
)

// crlf joins lines the way the shipped tables end them.
func crlf(lines ...string) []byte { return []byte(strings.Join(lines, "\r\n") + "\r\n") }

// A monsters.txt row through every column parser of Txt_LoadMonsters (0x453bc9).
func TestParseMonsters(t *testing.T) {
	ml := []MonListEntry{{Name: "Imp A"}, {Name: "Imp B"}}
	row := strings.Join([]string{"2", "\"Imp\"", "Imp A", "7", "\"1,234\"", "12", "\"2,000\"", "30%3d6+L2ring",
		"0", "y", "Stay", "Wimp", "3", "150", "90", "kN2x", "Poison2x3", "Fire", "2D4+1", "arrowf", "25",
		"MIND", "7", "x", "10", "\"Hour of Power,M,5\"", "20", "\"Shield,E,4\"",
		"1", "2", "3", "4", "Imm", "6", "7", "8", "9", "10", "\"summon,ground,Imp,B\""}, "\t")
	data := crlf("head", "head", "", "\tA", row, "300\tStop")
	m := ParseMonsters(data, ml)
	if len(m) != NumMonsters {
		t.Fatalf("%d rows", len(m))
	}
	in := m[2]
	want := MonsterInfo{
		Name: "Imp", Picture: "Imp A", Level: 7, HP: 1234, AC: 12, Exp: 2000, TreasureChance: 30, GoldDice: 3, GoldSides: 6,
		ItemLevel: 2, ItemKind: 0x28, Quest: 0, Fly: 'y' - 'n', Move: MoveStationary, AI: AIWimp, Hostility: 3, Speed: 150,
		Recovery: 90, Pref: 4 | 1 | 0x100, PrefCount: 2, AttackBonus: 7, AttackBonusMul: 3,
		Attack1: Attack{Type: 0, Dice: 2, Sides: 4, Add: 1, Missile: 2}, Attack2Chance: 25,
		Attack2: Attack{Type: 7, Dice: 7, Sides: 1, Missile: 0}, Spell1Chance: 10, Spell1: 0x56, Spell1Skill: 0x80 | 5,
		Spell2Chance: 20, Spell2: 0x11, Spell2Skill: 0x40 | 4, Resist: [10]uint16{1, 2, 3, 4, Immune, 6, 7, 8, 9, 10},
		Special: SpecialSummon, SpecialA: 2, SpecialB: 1, SpecialD: 2, ID: 2,
	}
	if in != want {
		t.Errorf("row 2\n got %+v\nwant %+v", in, want)
	}
	if m[300%len(m)].Name != "" || m[1].Name != "" {
		t.Error("rows past the cap or missing loaded")
	}
}

// Txt_ParseDice, Txt_DamageType, Txt_MissileType, Txt_SplitWords.
func TestMonsterCells(t *testing.T) {
	for _, c := range []struct {
		s          string
		d, sd, add uint8
	}{{"3D4+2", 3, 4, 2}, {"2d6", 2, 6, 0}, {"5", 5, 1, 0}, {"0", 0, 1, 0}, {"x", 0, 1, 0}} {
		if d, sd, add := ParseDice(c.s); d != c.d || sd != c.sd || add != c.add {
			t.Errorf("ParseDice(%q) = %d %d %d", c.s, d, sd, add)
		}
	}
	for s, want := range map[string]uint8{"Fire": 0, "Air": 1, "water": 2, "Earth": 3, "Spirit": 6, "Mind": 7,
		"Magic": 5, "Light": 9, "Dark": 10, "Phys": 4, "Body": 4, "": 4} {
		if got := DamageType(s); got != want {
			t.Errorf("DamageType(%q) = %d, want %d", s, got, want)
		}
	}
	if MissileType("ener") != 13 || MissileType("Arrow") != 1 || MissileType("0") != 0 {
		t.Error("MissileType")
	}
	if w := SplitWords(` a,"b c" d  ""x `); strings.Join(w, "|") != "a|b c|d||x" {
		t.Errorf("SplitWords = %q", w)
	}
}

// hostile.txt stores column c of row r at [(c - 1) * 0x59 + r]; placemon.txt's column 1.
func TestParseHostilePlaceMon(t *testing.T) {
	h := ParseHostile(crlf("\tParty\tA\tB", "Party\t0\t4\t3", "A\t1\t2\t"))
	if h[0*HostileStride+0] != 0 || h[1*HostileStride+0] != 4 || h[2*HostileStride+0] != 3 ||
		h[0*HostileStride+1] != 1 || h[1*HostileStride+1] != 2 {
		t.Error("hostile layout")
	}
	p := ParsePlaceMon(crlf("\tnames", "1\tAnn", "2\t\"Bob\""))
	if p[1] != "Ann" || p[2] != "Bob" || len(p) != NumPlaceMon {
		t.Errorf("placemon %q", p[:3])
	}
}

// MapStats' "min-max" counts; without a dash min stays 1 and max is atoi of the next cell.
func TestParseMapStats(t *testing.T) {
	cells := make([]string, 30)
	for i := range cells {
		cells[i] = "0"
	}
	cells[1], cells[2], cells[6], cells[11] = "Isle", "x.odm", "56", "3"
	cells[16], cells[18], cells[19] = "Imp", "2", " 2-5"
	cells[20], cells[22], cells[23], cells[24] = "Orc", "1", "4", "7"
	m := ParseMapStats(crlf("h", "h", "h", strings.Join(cells, "\t")))
	r := &m[1]
	if r.File != "x.odm" || r.RefillDays != 56 || r.Treasure != 3 || r.Monster[0] != "Imp" || r.Dif[0] != 2 ||
		r.Min[0] != 2 || r.Max[0] != 5 || r.Monster[1] != "Orc" || r.Min[1] != 1 || r.Max[1] != 7 {
		t.Errorf("mapstats row %+v", r)
	}
}
