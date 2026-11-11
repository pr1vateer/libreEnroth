package tables

import (
	"encoding/binary"
	"strings"

	"libre-enroth/internal/assets"
	"libre-enroth/internal/assets/exe"
	"libre-enroth/internal/assets/txt"
)

// NumSpells is the number of spells (ids 1..132; Txt_LoadSpells fills up to 0x4f72c3).
const NumSpells = 132

// Spell is one spell: its school (spells.txt), the damage dice, the SP cost and
// recovery of each mastery, and the object type a projectile of it launches.
type Spell struct {
	// School is the damage type of spells.txt's "Res" column (Txt_DamageType's
	// numbering: fire 0, air 1, water 2, earth 3, none 4, magic 5, spirit 6, mind 7,
	// body 8, light 9, dark 10).
	School uint8
	// Add and Sides are the damage: Add + skill d Sides (the row's +0x10, +0x11).
	Add, Sides uint8
	// Flags are spells.txt's "Stats" letters (the row's +0x12): m 1, e 2, c 4, x 8.
	Flags uint8
	// Cost and Recovery are one u16 per mastery, normal through grandmaster.
	Cost, Recovery [4]uint16
	// Object is the sprite a cast of it launches (the low half of the int at
	// 0x4f6634 + 4*spell). 0 launches nothing.
	Object int16
}

// Spells are the spells by id (index 0 unused).
type Spells []Spell

// A spell row is 0x14 bytes at 0x4f6860: u16 cost[4], u16 recovery[4], then the
// damage add, sides and flags (which 0x4f6870 is the first byte of). Object types
// are a separate int per spell at 0x4f6634; the dispatcher reads the low 16 bits.
// 0x4f6638, four bytes later, is spell 1's type. Wands index 0x4f7064 by item number.
const (
	vaSpellRow  = 0x4f6860
	spellRow    = 0x14
	vaSpellObj  = 0x4f6634
	vaWandSpell = 0x4f7064
	// spellRows covers the projectile attacks past the 132 spells of spells.txt
	// (0x85..0x89 are the bow, the blaster and a dragon's breath).
	spellRows  = 0x8a
	wandSpells = 177
)

// spellSchools are the "Res" names Txt_LoadSpells knows; anything else (none) is 4.
var spellSchools = map[string]uint8{
	"fire": 0, "air": 1, "water": 2, "earth": 3, "spirit": 6, "mind": 7, "body": 8,
	"light": 9, "dark": 10, "magic": 5,
}

// LoadSpells reads spells.txt and the exe's spell rows, object types and wand spells.
func LoadSpells(d *assets.Data) (Spells, []int32, error) {
	_, b, err := d.LangFile("spells.txt")
	if err != nil {
		return nil, nil, err
	}
	s := ParseSpells(b)
	if len(s) < spellRows {
		n := make(Spells, spellRows)
		copy(n, s)
		s = n
	}
	wands, err := s.readStats(d.Exe)
	return s, wands, err
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

// readStats fills costs, recovery and damage dice from the exe, and the object type.
// spells.txt's letters are ORed into the compiled flags. It also returns the wand
// item's spell (0x4f7064, indexed by item number).
func (s Spells) readStats(im *exe.Image) ([]int32, error) {
	raw, err := im.Read(vaSpellRow, len(s)*spellRow)
	if err != nil {
		return nil, err
	}
	objs, err := im.Read(vaSpellObj, len(s)*4)
	if err != nil {
		return nil, err
	}
	wraw, err := im.Read(vaWandSpell, wandSpells*4)
	if err != nil {
		return nil, err
	}
	for id := range s {
		r := raw[id*spellRow:]
		for k := range 4 {
			s[id].Cost[k] = binary.LittleEndian.Uint16(r[2*k:])
			s[id].Recovery[k] = binary.LittleEndian.Uint16(r[8+2*k:])
		}
		s[id].Add, s[id].Sides = r[0x10], r[0x11]
		s[id].Flags |= r[0x12]
		s[id].Object = int16(binary.LittleEndian.Uint16(objs[4*id:]))
	}
	wands := make([]int32, wandSpells)
	for i := range wands {
		wands[i] = int32(binary.LittleEndian.Uint32(wraw[4*i:]))
	}
	return wands, nil
}

// At is spell id, the zero Spell when out of range.
func (s Spells) At(id int) Spell {
	if id < 0 || id >= len(s) {
		return Spell{School: 4}
	}
	return s[id]
}
