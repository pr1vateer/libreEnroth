package tables

import (
	"strings"

	"libre-enroth/internal/assets"
	"libre-enroth/internal/assets/exe"
	"libre-enroth/internal/assets/txt"
)

// NumSpells is the number of spells (ids 1..132; Txt_LoadSpells fills up to 0x4f72c3).
const NumSpells = 132

// Spell is what the monster code reads of a spell so far: its damage school
// (spells.txt) and the damage dice compiled into the exe. The rest is M9's.
type Spell struct {
	// School is the damage type of spells.txt's "Res" column (Txt_DamageType's
	// numbering: fire 0, air 1, water 2, earth 3, none 4, magic 5, spirit 6, mind 7,
	// body 8, light 9, dark 10).
	School uint8
	// Add and Sides are the damage: Add + skill d Sides (g_spellStats +0, +1).
	Add, Sides uint8
	// Flags are spells.txt's "Stats" letters (g_spellStats +2): m 1, e 2, c 4, x 8.
	Flags uint8
}

// Spells are the spells by id (index 0 unused).
type Spells []Spell

// Exe address of the spell stats (0x14 bytes per spell, from spell 0).
const (
	vaSpellStats  = 0x4f6870
	spellStatSize = 0x14
)

// spellSchools are the "Res" names Txt_LoadSpells knows; anything else (none) is 4.
var spellSchools = map[string]uint8{
	"fire": 0, "air": 1, "water": 2, "earth": 3, "spirit": 6, "mind": 7, "body": 8,
	"light": 9, "dark": 10, "magic": 5,
}

// LoadSpells reads spells.txt and the exe's spell stats.
func LoadSpells(d *assets.Data) (Spells, error) {
	_, b, err := d.LangFile("spells.txt")
	if err != nil {
		return nil, err
	}
	s := ParseSpells(b)
	return s, s.readStats(d.Exe)
}

// ParseSpells reads spells.txt: two header lines, then the spells from id 1, with a
// school's header line skipped after every 11th spell. Column 3 is the school, column
// 10 the stats letters.
//
// mm8: 0x45236e (Txt_LoadSpells)
func ParseSpells(data []byte) Spells {
	out := make(Spells, NumSpells+1)
	lines := txt.Lines(data)
	k := 2
	for id := 1; id <= NumSpells && k < len(lines); id++ {
		sp := &out[id]
		sp.School = 4
		for col, cell := range cellsOf(lines[k]) {
			switch col {
			case 3:
				if v, ok := spellSchools[strings.ToLower(cell)]; ok {
					sp.School = v
				}
			case 10:
				for _, c := range strings.ToLower(cell) {
					switch c {
					case 'c':
						sp.Flags |= 4
					case 'e':
						sp.Flags |= 2
					case 'm':
						sp.Flags |= 1
					case 'x':
						sp.Flags |= 8
					}
				}
			}
		}
		k++
		if id%11 == 0 {
			k++
		}
	}
	return out
}

// readStats fills the damage dice from the exe; spells.txt's letters are ORed into the
// compiled flags.
func (s Spells) readStats(im *exe.Image) error {
	raw, err := im.Read(vaSpellStats, (NumSpells+1)*spellStatSize)
	if err != nil {
		return err
	}
	for id := range s {
		r := raw[id*spellStatSize:]
		s[id].Add, s[id].Sides = r[0], r[1]
		s[id].Flags |= r[2]
	}
	return nil
}

// At is spell id, the zero Spell when out of range.
func (s Spells) At(id int) Spell {
	if id < 0 || id >= len(s) {
		return Spell{School: 4}
	}
	return s[id]
}
