package tables

import (
	"strings"
	"testing"
)

// Every row of the monster tables parses, as the loaders read them (re/notes/monsters.md).
func TestMonstersData(t *testing.T) {
	a, _ := load(t)
	m := a.Monsters
	if len(m.MonList) != 198 || len(m.ObjList) != 274 {
		t.Fatalf("dmonlist %d, dobjlist %d", len(m.MonList), len(m.ObjList))
	}
	named := 0
	for id := 1; id < len(m.Infos); id++ {
		in := &m.Infos[id]
		if in.Name == "" {
			continue
		}
		named++
		if int(in.ID) != id {
			t.Errorf("monster %d: id %d", id, in.ID)
		}
		// Every monster's picture is a dmonlist record (Spawn_Monsters and
		// Actor_LoadSprites look it up).
		if m.FindMonList(in.Picture) < 0 {
			t.Errorf("monster %d %q: picture %q not in dmonlist.bin", id, in.Name, in.Picture)
		}
		if in.HP <= 0 || in.Level == 0 || in.AttackBonusMul == 0 {
			t.Errorf("monster %d %q: hp %d level %d bonus x%d", id, in.Name, in.HP, in.Level, in.AttackBonusMul)
		}
	}
	if named != 198 {
		t.Errorf("%d named monsters, want 198", named)
	}
	// monsters.txt rows 1 and 7 (traced through Txt_LoadMonsters 0x453bc9).
	p := m.Infos[1]
	if p.Name != "Lizardman Peasant" || p.Picture != "Lizardmen Male (Peasant) A" || p.Level != 4 || p.HP != 13 ||
		p.Exp != 5 || p.TreasureChance != 100 || p.GoldDice != 1 || p.GoldSides != 3 || p.Fly != 0 ||
		p.Move != MoveShort || p.AI != AIWimp || p.Hostility != 4 || p.Speed != 200 || p.Recovery != 100 ||
		p.Attack1 != (Attack{Type: 4, Dice: 3, Sides: 4, Add: 2}) || p.Quest != 1 {
		t.Errorf("monster 1 = %+v", p)
	}
	c := m.Infos[7]
	if c.TreasureChance != 5 || c.GoldDice != 2 || c.GoldSides != 5 || c.ItemLevel != 1 || c.ItemKind != 0x2e ||
		c.Move != MoveFree || c.AI != AIAggressive || c.AttackBonus != 8 || c.AttackBonusMul != 1 ||
		c.Attack2Chance != 10 || c.Attack2.Type != 0 || c.Attack2.Missile != 3 || c.Resist[2] != 5 || c.Resist[9] != 5 {
		t.Errorf("monster 7 = %+v", c)
	}
	// "explode,4D5,earth" is quoted: the type comes from the whole cell's '"' (physical).
	if e := m.Infos[142]; e.Special != SpecialExplode || e.SpecialA != 4 || e.SpecialB != 5 || e.SpecialD != 4 {
		t.Errorf("monster 142 special = %d %d %d %d %d", e.Special, e.SpecialA, e.SpecialB, e.SpecialC, e.SpecialD)
	}
	if s := m.Infos[111]; s.Special != SpecialShot || s.SpecialC != 3 {
		t.Errorf("monster 111 special = %d x%d", s.Special, s.SpecialC)
	}
	// Spells: "Lightning Bolt,M,9" -> 0x12, master 9; "...,GM,10" is not grandmaster.
	for id := range m.Infos {
		in := &m.Infos[id]
		if in.Spell1 == 0x12 && in.Spell1Skill != 0x80|9 && in.Spell1Skill != 0x80|7 && in.Spell1Skill != 0x80|5 &&
			in.Spell1Skill != 0x40|7 && in.Spell1Skill != 0x40|4 && in.Spell1Skill != 1 {
			t.Errorf("monster %d lightning skill %#x", id, in.Spell1Skill)
		}
		if in.Spell1Skill&0x100 != 0 || in.Spell2Skill&0x100 != 0 {
			t.Errorf("monster %d has a grandmaster spell %#x %#x", id, in.Spell1Skill, in.Spell2Skill)
		}
	}
	if len(m.PlaceMon) != NumPlaceMon || m.PlaceMon[1] != "Blackwell Cooper" || m.PlaceMon[4] != "Admiral Nelson" {
		t.Errorf("placemon %d: %q %q", len(m.PlaceMon), m.PlaceMon[1], m.PlaceMon[4])
	}
	// hostile.txt's "Party" row: column 4 (Couatl) 4, column 5 (Pirate Warrior Male) 3;
	// the Lizardmen Peasant row has 4 for columns 0x3e..0x40.
	if m.HostileAt(3, 0) != 4 || m.HostileAt(4, 0) != 3 || m.HostileAt(1, 1) != 0 || m.HostileAt(0x3d, 1) != 4 {
		t.Errorf("hostile [3][0] %d [4][0] %d [1][1] %d [0x3d][1] %d", m.HostileAt(3, 0), m.HostileAt(4, 0), m.HostileAt(1, 1), m.HostileAt(0x3d, 1))
	}
	ms := m.MapStatsRow(1)
	if ms == nil || ms.File != "Out01.odm" || ms.RefillDays != 672 || ms.Encounter != 10 ||
		ms.Monster != [3]string{"Lizardmen Warrior", "Wimpy Pirate Warrior Male", "Couatl (winged snake)"} ||
		ms.Dif != [3]uint8{1, 1, 1} || ms.Min != [3]uint8{2, 1, 1} || ms.Max != [3]uint8{5, 3, 3} {
		t.Errorf("mapstats 1 = %+v", ms)
	}
	// Every MapStats monster name resolves with its " A" (Spawn_Monsters' variants).
	for r := 1; r < len(m.MapStats); r++ {
		for _, n := range m.MapStats[r].Monster {
			if n == "" || n == "0" {
				continue
			}
			for _, v := range []string{" A", " B", " C"} {
				if m.FindMonList(n+v) < 0 {
					t.Errorf("mapstats %d: %q not in dmonlist.bin", r, n+v)
				}
			}
		}
	}
	if o := m.ObjList[1]; o.Name != "longsword" || o.ID != 1 || o.Radius != 64 || o.Flags != ObjBounce || o.SFT != 2 {
		t.Errorf("objlist 1 = %+v", o)
	}
	if e := m.MonList[0]; e.Height != 160 || e.Radius != 60 || e.Tint != 0x7eff9d || !strings.HasPrefix(e.Sprites[0], "m401") {
		t.Errorf("monlist 0 = %+v", e)
	}
	if m.Treasure[7] != [2]uint8{1, 1} && m.Treasure[7][0] == 0 {
		t.Errorf("treasure %v", m.Treasure[7:14])
	}
}

// spells.txt's schools ("Res"; none -> 4) and the exe's damage dice (0x4f6870).
//
// mm8: 0x45236e (Txt_LoadSpells)
func TestSpellsData(t *testing.T) {
	a, _ := load(t)
	s := a.Spells
	for _, c := range []struct {
		id   int
		want Spell
	}{
		{1, Spell{School: 4}},                                // Torch Light
		{2, Spell{School: 0, Sides: 3, Flags: 7}},            // Fire Bolt
		{11, Spell{School: 0, Add: 15, Sides: 15, Flags: 5}}, // Incinerate
		{18, Spell{School: 1, Sides: 8, Flags: 7}},           // Lightning Bolt
		{29, Spell{School: 4, Add: 9, Sides: 9, Flags: 5}},   // Acid Burst: "none"
		{90, Spell{School: 10, Add: 25, Sides: 10, Flags: 7}},
	} {
		if got := s.At(c.id); got != c.want {
			t.Errorf("spell %d: %+v, want %+v", c.id, got, c.want)
		}
	}
}
