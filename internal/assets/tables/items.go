package tables

import (
	"encoding/binary"
	"strings"

	"libre-enroth/internal/assets/lod"
	"libre-enroth/internal/assets/txt"
)

// The item tables: items.txt, stditems.txt, spcitems.txt and rnditems.txt as
// Txt_LoadItemsClassesSkills fills the items table (0x5efae0), and the skill, stat and
// class descriptions it loads after them (re/notes/items.md).

// Equip types: items.txt's "Equip Stat" column (ItemDef.EquipType).
//
// mm8: 0x455a6e (Txt_LoadItemsClassesSkills, column 4)
const (
	EquipWeapon        = 0 // "weapon", "weapon1or2"
	EquipWeapon2       = 1 // two-handed
	EquipMissile       = 2 // "missile", "bow"
	EquipArmor         = 3
	EquipShield        = 4
	EquipHelm          = 5
	EquipBelt          = 6
	EquipCloak         = 7
	EquipGauntlets     = 8
	EquipBoots         = 9
	EquipRing          = 10
	EquipAmulet        = 11
	EquipWand          = 12 // "weaponw"
	EquipReagent       = 13 // "herb", "reagent"
	EquipPotion        = 14 // "bottle"
	EquipSpellScroll   = 15 // "sscroll"
	EquipBook          = 16
	EquipMessageScroll = 17 // "mscroll"
	EquipGold          = 18
	EquipGem           = 19 // "gem", "ore"
	EquipNone          = 21 // anything else
)

var equipTypes = []struct {
	name string
	typ  uint8
}{
	{"weapon", EquipWeapon}, {"weapon2", EquipWeapon2}, {"weapon1or2", EquipWeapon},
	{"missile", EquipMissile}, {"bow", EquipMissile}, {"armor", EquipArmor},
	{"shield", EquipShield}, {"helm", EquipHelm}, {"belt", EquipBelt}, {"cloak", EquipCloak},
	{"gauntlets", EquipGauntlets}, {"boots", EquipBoots}, {"ring", EquipRing},
	{"amulet", EquipAmulet}, {"weaponw", EquipWand}, {"herb", EquipReagent},
	{"reagent", EquipReagent}, {"bottle", EquipPotion}, {"sscroll", EquipSpellScroll},
	{"book", EquipBook}, {"mscroll", EquipMessageScroll}, {"gold", EquipGold},
	{"gem", EquipGem}, {"ore", EquipGem},
}

// SkillMisc is the skill of items whose "Skill Group" names no weapon or armour skill.
const SkillMisc = 0x28

// Skill groups of items.txt's "Skill Group" column (column 5): the weapon and armour
// skills 0..11, else SkillMisc.
var skillGroups = []string{
	"staff", "sword", "dagger", "axe", "spear", "bow", "mace", "blaster", "shield",
	"leather", "chain", "plate",
}

// Materials (ItemDef.Material, items.txt column 8).
const (
	MaterialNormal   = 0
	MaterialArtifact = 1
	MaterialRelic    = 2
	MaterialSpecial  = 3 // its bonus is fixed by VarA/VarB
)

// ItemDef is one items.txt row (0x30 bytes at 0x5efae4 + id*0x30).
type ItemDef struct {
	Picture          string   // +0x00 icons.lod name
	Name             string   // +0x04
	UnidentifiedName string   // +0x08
	Notes            string   // +0x0c
	Value            int32    // +0x10
	Sprite           int16    // +0x14
	EquipX, EquipY   int16    // +0x18, +0x1a paper-doll offsets
	EquipType        uint8    // +0x1c Equip*
	Skill            uint8    // +0x1d skill 0..11, or SkillMisc
	DiceCount        uint8    // +0x1e "Mod1" NdS
	DiceSides        uint8    // +0x1f
	Mod2             uint8    // +0x20
	Material         uint8    // +0x21 Material*
	Special          uint8    // +0x22 MaterialSpecial: spcitems number (1-based)
	StdBonus         uint8    // +0x23 MaterialSpecial: stditems number (1-based)
	Strength         int8     // +0x24 MaterialSpecial with StdBonus: its strength
	Chance           [6]uint8 // +0x28 rnditems: weight per treasure level 1..6
	IDRepair         uint8    // +0x2e identify and repair difficulty (0: identified when found)
	// W and H are the item's size in backpack cells, from its icon's size (0x41a904).
	W, H int
}

// StdBonus is one stditems.txt row (0x14 bytes at 0x5f9174): a standard bonus.
type StdBonus struct {
	OfName string   // +0x00 "of Might"
	Stat   string   // +0x04 "Might"
	Chance [9]uint8 // +0x08 weight per equip type EquipArmor..EquipAmulet
}

// SpcBonus is one spcitems.txt row (0x1c bytes at 0x5f9354): a special bonus.
type SpcBonus struct {
	Name        string    // +0x00 "of Protection"
	Description string    // +0x04
	Chance      [12]uint8 // +0x08 weight per equip type EquipWeapon..EquipAmulet
	// Value adds to the item's value; below 11 it multiplies it instead (the "X n"
	// cells).
	Value int32 // +0x14
	Level uint8 // +0x18 A..D: 0..3, the treasure levels it can appear at
}

// Items is the items table.
type Items struct {
	Items []ItemDef // ids 0..802 (0 is the empty row)
	Std   []StdBonus
	Spc   []SpcBonus
	// StdTotals are the sums of the standard bonus weights per equip type
	// EquipArmor..EquipAmulet (0x5f9f28).
	StdTotals [9]int32
	// StdRange is the strength range {min, max} of a standard bonus per treasure level
	// (0x5f9f4c).
	StdRange [6][2]int32
	// SpcCount is the number of special bonuses the generator draws from: the loader
	// sums and walks only the first 0x47 of the 72 (0x5f9f5c).
	SpcCount int
	// SpcTotals are the sums of their weights per equip type EquipWeapon..EquipAmulet
	// (0x5f9efc).
	SpcTotals [12]int32
	// LevelTotals are the sums of the rnditems weights per treasure level (0x5f9e48).
	LevelTotals [6]int32
	// BonusChance is rnditems' bonus table: the chance in % of a standard bonus, of a
	// special one, and of a special one on a weapon, per treasure level (0x5f9e60).
	BonusChance [3][6]int32
	// SkillDesc are skilldes.txt's texts per skill: the description, then normal,
	// expert, master and grandmaster (0x5e4bcc, 0x5e4b30, 0x5e4a94, 0x5e49f8, 0x5e495c).
	SkillDesc [NumSkills][5]string
	// StatDesc are stats.txt's 26 descriptions (the 7 stats from 0x5e48c0, then HP, SP,
	// AC and the rest from 0x5e4c68).
	StatDesc [26]string
	// ClassDesc are class.txt's descriptions (0x5e4824).
	ClassDesc [NumClasses]string
	// Potion is potion.txt's mixing table (g_items + 0xf048), PotNotes potnotes.txt's
	// autonotes (+0x103d0).
	Potion, PotNotes *MixTable
}

// Table sizes.
const (
	NumItems    = 0x323
	NumStd      = 0x18
	NumSpc      = 0x48
	spcDrawn    = 0x47
	NumSkills   = 39
	NumClasses  = 16
	numStatDesc = 26
)

// walkFirst feeds the cells of line, column 0 first, to fn up to maxCols columns; an
// empty cell is skipped, and an empty first cell ends the line (the loaders' "iVar == 0
// and column 0 -> done").
func walkFirst(line string, maxCols int, fn func(col int, cell string)) {
	for col, cell := range txt.Cells(line) {
		if col >= maxCols {
			return
		}
		if cell == "" {
			if col == 0 {
				return
			}
			continue
		}
		fn(col, cell)
	}
}

// lines returns the table's lines after skip header lines (nil when shorter).
func lines(data []byte, skip int) []string {
	l := txt.Lines(data)
	if len(l) <= skip {
		return nil
	}
	return l[skip:]
}

// ItemFiles are the language-LOD files the items table is read from.
var ItemFiles = []string{"items.txt", "stditems.txt", "spcitems.txt", "rnditems.txt", "skilldes.txt", "stats.txt", "class.txt", "potion.txt", "potnotes.txt"}

// ParseItems reads the items table from files (keyed by ItemFiles' names); icons, when
// not nil, gives every item its cell size from its icon.
//
// mm8: 0x455a6e (Txt_LoadItemsClassesSkills), 0x452662, 0x452807
func ParseItems(files map[string][]byte, icons *lod.Archive) *Items {
	t := &Items{Items: make([]ItemDef, NumItems), Std: make([]StdBonus, NumStd), Spc: make([]SpcBonus, NumSpc), SpcCount: spcDrawn}
	t.parseStd(files["stditems.txt"])
	t.parseSpc(files["spcitems.txt"])
	t.parseItems(files["items.txt"])
	t.parseRnd(files["rnditems.txt"])
	t.parseDescs(files["skilldes.txt"], files["stats.txt"], files["class.txt"])
	t.Potion, t.PotNotes = parseMix(files["potion.txt"]), parseMix(files["potnotes.txt"])
	for i := range t.Items {
		t.Items[i].W, t.Items[i].H = 1, 1
		if icons != nil {
			t.Items[i].W, t.Items[i].H = iconCells(icons, t.Items[i].Picture)
		}
	}
	return t
}

// parseStd reads stditems.txt: four header lines, 24 bonuses, five more lines, then the
// strength ranges of the six treasure levels.
func (t *Items) parseStd(data []byte) {
	l := lines(data, 4)
	for i := 0; i < NumStd && i < len(l); i++ {
		b := &t.Std[i]
		walkFirst(l[i], 0xb, func(col int, s string) {
			switch {
			case col == 0:
				b.Stat = txt.StripQuotes(s)
			case col == 1:
				b.OfName = txt.StripQuotes(s)
			default:
				b.Chance[col-2] = uint8(txt.Atoi(s))
			}
		})
	}
	for k := range t.StdTotals {
		for i := range t.Std {
			t.StdTotals[k] += int32(t.Std[i].Chance[k])
		}
	}
	for lvl := 0; lvl < 6 && NumStd+5+lvl < len(l); lvl++ {
		walk(l[NumStd+5+lvl], 4, false, func(col int, s string) {
			if col >= 2 {
				t.StdRange[lvl][col-2] = txt.Atoi(s)
			}
		})
	}
}

// parseSpc reads spcitems.txt: four header lines, then 72 bonuses. A value cell that
// reads 0 ("X 2") is read again from its second character, a multiplier.
func (t *Items) parseSpc(data []byte) {
	l := lines(data, 4)
	for i := 0; i < NumSpc && i < len(l); i++ {
		b := &t.Spc[i]
		walkFirst(l[i], 0x10, func(col int, s string) {
			switch {
			case col == 0:
				b.Description = txt.StripQuotes(s)
			case col == 1:
				b.Name = txt.StripQuotes(s)
			case col <= 0xd:
				b.Chance[col-2] = uint8(txt.Atoi(s))
			case col == 0xe:
				if b.Value = txt.Atoi(s); b.Value == 0 && len(s) > 1 {
					b.Value = txt.Atoi(s[1:])
				}
			case col == 0xf:
				b.Level = lower(s[0]) - 'a'
			}
		})
	}
	for k := range t.SpcTotals {
		for i := 0; i < t.SpcCount; i++ {
			t.SpcTotals[k] += int32(t.Spc[i].Chance[k])
		}
	}
}

func lower(c byte) byte {
	if 'A' <= c && c <= 'Z' {
		return c + 'a' - 'A'
	}
	return c
}

// parseItems reads items.txt: two header lines, then rows until the id column passes
// 802 (rows without an id take the next one).
func (t *Items) parseItems(data []byte) {
	id := 0
	for _, line := range lines(data, 2) {
		if id >= NumItems {
			break
		}
		walk(line, 0x11, false, func(col int, s string) {
			if col == 0 {
				id = int(txt.Atoi(s))
				return
			}
			if id < 0 || id >= NumItems {
				return
			}
			it := &t.Items[id]
			switch col {
			case 1:
				it.Picture = txt.StripQuotes(s)
			case 2:
				it.Name = txt.StripQuotes(s)
			case 3:
				it.Value = txt.Atoi(s)
			case 4:
				it.EquipType = EquipNone
				for _, e := range equipTypes {
					if strings.EqualFold(s, e.name) {
						it.EquipType = e.typ
						break
					}
				}
			case 5:
				it.Skill = SkillMisc
				for k, n := range skillGroups {
					if strings.EqualFold(s, n) {
						it.Skill = uint8(k)
						break
					}
				}
			case 6:
				it.DiceCount, it.DiceSides = parseDice(s)
			case 7:
				it.Mod2 = uint8(txt.Atoi(s))
			case 8:
				switch {
				case strings.EqualFold(s, "artifact"):
					it.Material = MaterialArtifact
				case strings.EqualFold(s, "relic"):
					it.Material = MaterialRelic
				case strings.EqualFold(s, "special"):
					it.Material = MaterialSpecial
				default:
					it.Material = MaterialNormal
				}
			case 9:
				it.IDRepair = uint8(txt.Atoi(s))
			case 10:
				it.UnidentifiedName = txt.StripQuotes(s)
			case 11:
				it.Sprite = int16(txt.Atoi(s))
			case 12:
				// A special item's VarA names its standard bonus, else its special one.
				it.Special, it.StdBonus = 0, 0
				if it.Material != MaterialSpecial {
					break
				}
				for k := range t.Std {
					if strings.EqualFold(t.Std[k].OfName, s) {
						it.StdBonus = uint8(k + 1)
						break
					}
				}
				if it.StdBonus == 0 {
					for k := range t.Spc {
						if strings.EqualFold(t.Spc[k].Name, s) {
							it.Special = uint8(k + 1)
							break
						}
					}
				}
			case 13:
				it.Strength = 0
				if it.Material == MaterialSpecial && it.StdBonus != 0 {
					if it.Strength = int8(txt.Atoi(s)); it.Strength == 0 {
						it.Strength = 1
					}
				}
			case 14:
				it.EquipX = int16(txt.Atoi(s))
			case 15:
				it.EquipY = int16(txt.Atoi(s))
			case 16:
				it.Notes = txt.StripQuotes(s)
			}
		})
		id++
	}
}

// parseDice reads a Mod1 cell: "NdS", else a plain number N (sides 1) unless it starts
// with 's'.
func parseDice(s string) (count, sides uint8) {
	for i := 0; i < len(s); i++ {
		if lower(s[i]) == 'd' {
			return uint8(txt.Atoi(s[:i])), uint8(txt.Atoi(s[i+1:]))
		}
	}
	if lower(s[0]) != 's' {
		return uint8(txt.Atoi(s)), 1
	}
	return 0, 0
}

// parseRnd reads rnditems.txt: four header lines, the weights of ids 1..802 (the loop
// ends once the id column reaches 802), five more lines, then the three rows of the
// bonus chances.
func (t *Items) parseRnd(data []byte) {
	l := lines(data, 4)
	id, n := 0, 0
	for id < NumItems && n < len(l) {
		walkFirst(l[n], 8, func(col int, s string) {
			switch {
			case col == 0:
				id = int(txt.Atoi(s))
			case col >= 2 && col <= 7 && id >= 0 && id < NumItems:
				t.Items[id].Chance[col-2] = uint8(txt.Atoi(s))
			}
		})
		id++
		n++
	}
	for lvl := range t.LevelTotals {
		for i := range t.Items {
			t.LevelTotals[lvl] += int32(t.Items[i].Chance[lvl])
		}
	}
	n += 5
	for row := 0; row < 3 && n+row < len(l); row++ {
		walk(l[n+row], 8, false, func(col int, s string) {
			if col >= 2 {
				t.BonusChance[row][col-2] = txt.Atoi(s)
			}
		})
	}
}

// parseDescs reads skilldes.txt (39 rows), stats.txt (26) and class.txt (16), each after
// one header line.
func (t *Items) parseDescs(skilldes, stats, class []byte) {
	l := lines(skilldes, 1)
	for i := 0; i < NumSkills && i < len(l); i++ {
		walkFirst(l[i], 6, func(col int, s string) {
			if col >= 1 {
				t.SkillDesc[i][col-1] = txt.StripQuotes(s)
			}
		})
	}
	l = lines(stats, 1)
	for i := 0; i < numStatDesc && i < len(l); i++ {
		walkFirst(l[i], 2, func(col int, s string) {
			if col == 1 {
				t.StatDesc[i] = txt.StripQuotes(s)
			}
		})
	}
	l = lines(class, 1)
	for i := 0; i < NumClasses && i < len(l); i++ {
		walkFirst(l[i], 2, func(col int, s string) {
			if col == 1 {
				t.ClassDesc[i] = txt.StripQuotes(s)
			}
		})
	}
}

// iconCells is an item's size in backpack cells: (max(size, 14) - 14) / 32 + 1 of its
// icon's width and height; an icon that is not there counts as 1×1.
//
// mm8: 0x492b25 (TexLod_LoadTexture, header +0x18/+0x1a), 0x41a904
func iconCells(icons *lod.Archive, name string) (w, h int) {
	e, ok := icons.Find(name)
	if name == "" || !ok {
		return 1, 1
	}
	head, err := icons.ReadHead(e, 0x1c)
	if err != nil || len(head) < 0x1c {
		return 1, 1
	}
	return Cells(int(int16(binary.LittleEndian.Uint16(head[0x18:])))), Cells(int(int16(binary.LittleEndian.Uint16(head[0x1a:]))))
}

// Cells is the number of 32-pixel backpack cells a picture size takes.
//
// mm8: 0x41a904
func Cells(px int) int {
	return (max(px, 14)-14)>>5 + 1
}

// Item returns item id's row, the empty row 0 when out of range.
func (t *Items) Item(id int32) *ItemDef {
	if id < 0 || int(id) >= len(t.Items) {
		return &t.Items[0]
	}
	return &t.Items[id]
}

// IsRare reports an artifact, relic or special item (0x455a48): their value and name
// ignore bonuses.
//
// mm8: 0x455a48
func (d *ItemDef) IsRare() bool {
	return d.Material == MaterialArtifact || d.Material == MaterialRelic || d.Material == MaterialSpecial
}
