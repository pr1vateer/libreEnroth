package tables

import (
	"strings"

	"libre-enroth/internal/assets/txt"
)

// The mixing tables: potion.txt gives what potion a pair makes (or an explosion), and
// potnotes.txt the autonote the recipe sets. Both are indexed by the two potions, the one
// on the cursor first, from FirstMixPotion (re/notes/inventory.md#potions).
const (
	FirstMixPotion = 0xde // Cure Wounds, the first row and column
	NumMixPotions  = 50   // 0xde..0x10f
)

// MixTable is a 50 × 50 table of shorts ([cursor][target]): 0 nothing, 1..4 an
// explosion's strength (the cells "E1".."E4"), else an item id or an autonote.
type MixTable [NumMixPotions][NumMixPotions]int16

// At is the cell of potions a (on the cursor) and b, 0 outside the table.
func (t *MixTable) At(a, b int32) int16 {
	a, b = a-FirstMixPotion, b-FirstMixPotion
	if a < 0 || b < 0 || a >= NumMixPotions || b >= NumMixPotions {
		return 0
	}
	return t[a][b]
}

// parseMix reads potion.txt or potnotes.txt the way the game does: strtok on "\t\r\n"
// (so empty cells vanish) up to the second "222" token, the id of the first row; then
// per row six tokens are skipped (name, colour, effect and the three reagent counts),
// 50 cells are read with atoi (a cell reading 0 that starts with 'e' or 'E' is read
// again from its second character), and the next row's id is skipped. A table that
// ends early leaves the rest 0 (the game shows a parsing error).
//
// mm8: 0x452662 (Txt_LoadPotion), 0x452807 (Txt_LoadPotNotes)
func parseMix(data []byte) *MixTable {
	t := &MixTable{}
	toks := strings.FieldsFunc(string(data), func(r rune) bool { return r == '\t' || r == '\r' || r == '\n' })
	i, seen := 0, 0
	for ; i < len(toks) && seen < 2; i++ {
		if toks[i] == "222" {
			seen++
		}
	}
	if seen < 2 {
		return t
	}
	for row := 0; row < NumMixPotions; row++ {
		if i+6 > len(toks) {
			return t
		}
		i += 6
		for col := 0; col < NumMixPotions; col++ {
			if i >= len(toks) {
				return t
			}
			s := toks[i]
			i++
			v := int16(txt.Atoi(s))
			if v == 0 && len(s) > 0 && lower(s[0]) == 'e' {
				v = int16(txt.Atoi(s[1:]))
			}
			t[row][col] = v
		}
		i++ // the next row's id
	}
	return t
}
