package tables

import "testing"

// TestItemsData checks the item tables against the game files.
func TestItemsData(t *testing.T) {
	a, d := load(t)
	it := a.Items
	l := it.Item(1)
	if l.Picture != "item001" || l.Name != "Longsword" || l.Value != 50 || l.EquipType != EquipWeapon ||
		l.Skill != 1 || l.DiceCount != 3 || l.DiceSides != 3 || l.Mod2 != 0 || l.IDRepair != 1 ||
		l.EquipX != 14 || l.EquipY != 131 || l.Chance != [6]uint8{5, 5, 2, 0, 0, 0} || l.W != 1 || l.H != 5 {
		t.Errorf("item 1 = %+v", *l)
	}
	if got := it.Item(802).Name; got != "Ball of Dunduck" {
		t.Errorf("item 802 = %q", got)
	}
	// Every item with a picture has its icon in icons.lod, and its size in pixels.
	for id := 1; id < NumItems; id++ {
		p := it.Items[id].Picture
		if p == "" || p == "null" {
			continue
		}
		if _, ok := d.Icons.Find(p); !ok {
			t.Errorf("item %d: icon %q missing", id, p)
		}
		if def := &it.Items[id]; def.PicW <= 0 || def.PicH <= 0 || Cells(def.PicW) != def.W || Cells(def.PicH) != def.H {
			t.Errorf("item %d: picture %dx%d, %dx%d cells", id, def.PicW, def.PicH, def.W, def.H)
		}
	}
	// Special materials resolve their VarA to a bonus.
	for id := range it.Items {
		d := &it.Items[id]
		if d.Material == MaterialSpecial && d.Special == 0 && d.StdBonus == 0 {
			t.Errorf("special item %d (%s) has no bonus", id, d.Name)
		}
	}
	if s := it.Std[0]; s.Stat != "Might" || s.OfName != "of Might" || s.Chance != [9]uint8{5, 0, 10, 10, 10, 10, 5, 10, 10} {
		t.Errorf("std 0 = %+v", s)
	}
	// The file's own totals row is stale: the loader sums the 24 rows itself.
	if it.StdTotals != [9]int32{145, 100, 145, 100, 180, 225, 150, 205, 210} {
		t.Errorf("std totals %v", it.StdTotals)
	}
	if it.StdRange != [6][2]int32{{0, 0}, {1, 5}, {3, 8}, {6, 12}, {10, 17}, {15, 25}} {
		t.Errorf("std ranges %v", it.StdRange)
	}
	if s := it.Spc[0]; s.Name != "of Protection" || s.Value != 1000 || s.Level != 1 {
		t.Errorf("spc 0 = %+v", s)
	}
	if it.BonusChance != [3][6]int32{{0, 40, 40, 40, 40, 75}, {0, 0, 10, 15, 20, 25}, {0, 0, 10, 20, 30, 50}} {
		t.Errorf("bonus chances %v", it.BonusChance)
	}
	for lvl, n := range it.LevelTotals {
		if n <= 0 {
			t.Errorf("level %d total %d", lvl+1, n)
		}
	}
	if it.SkillDesc[0][0] == "" || it.SkillDesc[38][4] == "" || it.StatDesc[6] == "" || it.ClassDesc[15] == "" {
		t.Errorf("descriptions: %q / %q / %q / %q", it.SkillDesc[0][0], it.SkillDesc[38][4], it.StatDesc[6], it.ClassDesc[15])
	}
	t.Logf("%d spc multipliers, %d special items", countSpcMult(it), countSpecial(it))
}

func countSpcMult(it *Items) int {
	n := 0
	for _, s := range it.Spc {
		if s.Value < 11 {
			n++
		}
	}
	return n
}

func countSpecial(it *Items) int {
	n := 0
	for _, d := range it.Items {
		if d.Material == MaterialSpecial {
			n++
		}
	}
	return n
}

// TestClassesData checks the class tables read from the executable.
func TestClassesData(t *testing.T) {
	a, _ := load(t)
	c := a.Classes
	if c.HPBase != [16]uint8{20, 20, 30, 30, 35, 35, 45, 45, 30, 30, 25, 25, 30, 30, 50, 50} ||
		c.SPPerLevel != [16]uint8{3, 5, 3, 5, 0, 0, 0, 0, 1, 2, 2, 4, 2, 4, 5, 10} {
		t.Errorf("hp base %v, sp per level %v", c.HPBase, c.SPPerLevel)
	}
	if s := c.Stats[6][0]; s != (StatRange{Base: 14, Max: 35, UpCost: 1, UpStep: 2}) {
		t.Errorf("troll might %+v", s)
	}
	if len(c.BonusLimits) != 29 || c.BonusLimits[28] != 0 || len(c.Bonus) != 29 {
		t.Errorf("bonus tables %d/%d", len(c.BonusLimits), len(c.Bonus))
	}
	for _, tc := range []struct{ v, want int }{{500, 30}, {1000, 30}, {499, 25}, {400, 25}, {25, 5}, {21, 4}, {20, 3}, {15, 1}, {13, 0}, {12, -1}, {3, -5}, {2, -6}, {0, -6}, {-5, -6}} {
		if got := c.StatToBonus(tc.v); got != tc.want {
			t.Errorf("StatToBonus(%d) = %d, want %d", tc.v, got, tc.want)
		}
	}
	if c.AgeLimits != [4]int32{50, 100, 150, 65535} || c.AgePercent(20, 0) != 100 || c.AgePercent(60, 0) != 75 || c.AgePercent(160, 6) != 100 {
		t.Errorf("age %v", c.AgeLimits)
	}
	if c.CondPct[0][4] != 50 || c.CondPct[0][18] != 100 {
		t.Errorf("cond %v", c.CondPct[0])
	}
	if c.EquipSlot != [13]uint8{1, 1, 2, 3, 0, 4, 5, 6, 7, 8, 10, 9, 1} {
		t.Errorf("equip slots %v", c.EquipSlot)
	}
	if c.ClassSkill(4, 1) != 2 {
		t.Errorf("knight sword %d", c.ClassSkill(4, 1))
	}
}

// TestPotionData checks potion.txt and potnotes.txt: every row read, the recipes and
// explosions in the cells, the autonotes.
func TestPotionData(t *testing.T) {
	a, _ := load(t)
	p, n := a.Items.Potion, a.Items.PotNotes
	if p.At(ItemCureWoundsID, ItemMagicPotionID) != 226 || p.At(ItemMagicPotionID, ItemCureWoundsID) != 226 ||
		p.At(0xe0, 0xde) != 225 || p.At(0xe4, 0xdf) != 1 || p.At(0xde, 0xde) != 0 {
		t.Errorf("potion cells %d %d %d %d %d", p.At(0xde, 0xdf), p.At(0xdf, 0xde), p.At(0xe0, 0xde), p.At(0xe4, 0xdf), p.At(0xde, 0xde))
	}
	if p.At(0x10f, 0xde) != 4 || p.At(0x10f, 0x10f) != 0 {
		t.Errorf("Rejuvenation row: %d %d", p.At(0x10f, 0xde), p.At(0x10f, 0x10f))
	}
	if n.At(0xde, 0xdf) != 58 || n.At(0xdf, 0xde) != 58 || n.At(0xe0, 0xe1) == 0 {
		t.Errorf("potnotes %d %d", n.At(0xde, 0xdf), n.At(0xdf, 0xde))
	}
	// Every result is an explosion 1..4 or a potion.
	for i := range p {
		for j, v := range p[i] {
			if v != 0 && (v < 1 || v > 4) && (v < 0xdd || v > 0x10f) {
				t.Errorf("cell [%d][%d] = %d", i, j, v)
			}
		}
	}
	if a.Classes.EnchantSpecial != [5]int32{11, 5, 13, 7, 59} {
		t.Errorf("enchant specials %v", a.Classes.EnchantSpecial)
	}
	if a.Classes.SkillMax[4][1] != 3 || a.Classes.SkillMax[5][1] != 4 {
		t.Errorf("knight sword ranks %d / %d", a.Classes.SkillMax[4][1], a.Classes.SkillMax[5][1])
	}
	if len(a.Scrolls) != NumScrolls || a.Scrolls[0] == "" {
		t.Errorf("%d scrolls, first %q", len(a.Scrolls), a.Scrolls[0])
	}
}

// Potion ids the data test names.
const (
	ItemCureWoundsID  = 0xde
	ItemMagicPotionID = 0xdf
)

// TestChestData checks the chest grids, the treasure levels and dchest.bin.
func TestChestData(t *testing.T) {
	_, d := load(t)
	c, err := LoadChests(d)
	if err != nil {
		t.Fatal(err)
	}
	for k, g := range c.Grid {
		if g.W != 9 || g.H != 9 || g.X < 0 || g.Y < 0 {
			t.Errorf("chest type %d grid %+v", k, g)
		}
	}
	if c.Grid[0] != (ChestGrid{42, 49, 9, 9}) || c.Grid[1] != (ChestGrid{18, 45, 9, 9}) {
		t.Errorf("grids %+v %+v", c.Grid[0], c.Grid[1])
	}
	if len(c.Pictures) != 8 || c.Picture(0) != 1 || c.Picture(7) != 8 {
		t.Errorf("pictures %v", c.Pictures)
	}
	for n := range c.Treasure {
		for l, r := range c.Treasure[n] {
			if r[0] < 1 || r[0] > r[1] || r[1] > 7 {
				t.Errorf("treasure -%d at level %d: %v", n+1, l, r)
			}
		}
	}
	if c.Treasure[0][0] != [2]uint8{1, 1} || c.Treasure[6][6] != [2]uint8{7, 7} {
		t.Errorf("treasure corners %v %v", c.Treasure[0][0], c.Treasure[6][6])
	}
}
