// Package tables parses the text tables of the language LODs that houses, NPCs and
// dialogues use: 2DEvents.txt, npcdata/npcgreet/npcgroup/npcnews, npctopic/npctext,
// quests, autonote, awards, merchant, trans and roster (re/notes/houses.md).
//
// Each parser follows its loader in MM8-Rel.exe: the same header lines skipped, the
// same number of rows (extra file rows are ignored, as the game ignores them), rows
// taken in file order (the id column is not read), empty cells skipped or ending the
// row exactly where the loader does, and the same quote stripping and atoi.
package tables

import (
	"strings"

	"libre-enroth/internal/assets/txt"
)

// walk feeds the cells of line, column 0 first, to fn up to maxCols columns. An empty
// cell is not passed on; with stopEmpty it also ends the line (the loaders that test
// "iVar == 0 -> bVar2 = true").
func walk(line string, maxCols int, stopEmpty bool, fn func(col int, cell string)) {
	for col, cell := range txt.Cells(line) {
		if col >= maxCols {
			return
		}
		if cell == "" {
			if stopEmpty {
				return
			}
			continue
		}
		fn(col, cell)
	}
}

// rows returns n data lines after skip header lines (fewer when the file is shorter;
// the game would crash there).
func rows(data []byte, skip, n int) []string {
	l := txt.Lines(data)
	if len(l) <= skip {
		return nil
	}
	l = l[skip:]
	if len(l) > n {
		l = l[:n]
	}
	return l
}

// House types of 2DEvents.txt's Type column: the first three letters, compared
// case-insensitively; anything else is 0. The house screen goes by g_houseAnims' type
// instead (House_Enter 0x443f4b).
//
// mm8: 0x4414e8 (Txt_Load2DEvents: _strnicmp(type, prefix, 3) against 0x4f9718..0x4f9770)
var houseTypePrefixes = []struct {
	prefix string
	typ    int16
}{
	{"wea", 1}, {"arm", 2}, {"mag", 3}, {"alc", 4}, {"sta", 0x1b}, {"boa", 0x1c}, {"tem", 0x17},
	{"tra", 0x1e}, {"tow", 0x11}, {"tav", 0x15}, {"ban", 0x16}, {"fir", 5}, {"air", 6},
	{"wat", 7}, {"ear", 8}, {"spi", 9}, {"min", 0xa}, {"bod", 0xb}, {"lig", 0xc}, {"dar", 0xd},
	{"ele", 0xe}, {"sel", 0xf}, {"spe", 0xe},
}

// House is one 2DEvents.txt row (0x34 bytes at 0x5a5670 + id*0x34).
type House struct {
	Type      int16   // +0x00 Type column (houseTypePrefixes)
	Video     int16   // +0x02 Picture: index into g_houseAnims
	Name      string  // +0x04
	Owner     string  // +0x08 Proprietor Name
	EnterText string  // +0x0c Text
	Title     string  // +0x10
	Picture   int16   // +0x14 the proprietor's portrait (the house screen uses g_houseAnims')
	State     int16   // +0x16
	Rep       int16   // +0x18
	Per       int16   // +0x1a
	C         int16   // +0x1c
	Val       float32 // +0x20 price factor
	A         float32 // +0x24
	Open      int16   // +0x28 opening hour
	Closed    int16   // +0x2a closing hour
	ExitPic   int16   // +0x2c "Other Exits" Pic: a ticon01..03 portrait leading out
	ExitMap   int16   // +0x2e "Other Exits" Map: MapStats row
	QBit      int16   // +0x30 Restrictions: the exit needs this quest bit (< 0: arrival table)
}

// NumHouses is the number of 2DEvents rows the game reads (ids 1..524).
const NumHouses = 524

// ParseHouses reads 2DEvents.txt; the result is indexed by row id, entry 0 unused.
// Columns B (14), Notes (16, 17) and Map (3) are not read.
//
// mm8: 0x4414e8 (Txt_Load2DEvents)
func ParseHouses(data []byte) []House {
	out := make([]House, NumHouses+1)
	for i, line := range rows(data, 2, NumHouses) {
		h := &out[i+1]
		walk(line, 24, false, func(col int, s string) {
			switch col {
			case 2:
				for _, p := range houseTypePrefixes {
					if len(s) >= 3 && strings.EqualFold(s[:3], p.prefix) {
						h.Type = p.typ
						break
					}
				}
			case 4:
				h.Video = int16(txt.Atoi(s))
			case 5:
				h.Name = txt.StripQuotes(s)
			case 6:
				h.Owner = txt.StripQuotes(s)
			case 7:
				h.Title = txt.StripQuotes(s)
			case 8:
				h.Picture = int16(txt.Atoi(s))
			case 9:
				h.State = int16(txt.Atoi(s))
			case 10:
				h.Rep = int16(txt.Atoi(s))
			case 11:
				h.Per = int16(txt.Atoi(s))
			case 12:
				h.Val = float32(txt.Atof(s))
			case 13:
				h.A = float32(txt.Atof(s))
			case 15:
				h.C = int16(txt.Atoi(s))
			case 18:
				h.Open = int16(txt.Atoi(s))
			case 19:
				h.Closed = int16(txt.Atoi(s))
			case 20:
				h.ExitPic = int16(txt.Atoi(s))
			case 21:
				h.ExitMap = int16(txt.Atoi(s))
			case 22:
				h.QBit = int16(txt.Atoi(s))
			case 23:
				h.EnterText = txt.StripQuotes(s)
			}
		})
	}
	return out
}

// NPC is one npcdata.txt row (0x4c bytes; the template at 0x761898 + id*0x4c, copied to
// the game's NPC table 0x76bc2c by Party_InitNewGame).
type NPC struct {
	Name       string   // +0x00
	Portrait   int32    // +0x04 npc%04u
	Flags      uint32   // +0x08 0 in the file; bits 0-1 count visits, 0x80 joined/gone
	House      int32    // +0x14 2D Location: the 2DEvents house the NPC is in
	Profession int32    // +0x18
	Greeting   int32    // +0x1c npcgreet.txt row
	Join       int32    // +0x20 "Join" column starts with 'y' (lower case only)
	Topics     [6]int32 // +0x28..+0x3c events A..F
}

// NumNPCs is the number of npcdata rows the game reads (ids 1..550; the file has 1000).
//
// mm8: 0x443ae2 ("NPC id exceeds MAX_DATA!" above 0x226)
const NumNPCs = 0x226

// NPCTables are the NPC loader's tables.
type NPCTables struct {
	NPCs      []NPC       // ids 1..550, entry 0 unused
	Greetings [][2]string // ids 1..205: first and later visits; entry 0 unused
	GroupNews []int16     // groups 0..50 (npcgroup.txt): the news each group tells
	News      []string    // ids 0..50 (npcnews.txt)
}

// Row counts of the NPC loader.
const (
	NumGreetings = 0xcd
	NumGroups    = 0x33
)

// ParseNPCs reads npcdata.txt, npcgreet.txt, npcgroup.txt and npcnews.txt.
//
// mm8: 0x47766d (Txt_LoadNpcData)
func ParseNPCs(npcdata, npcgreet, npcgroup, npcnews []byte) *NPCTables {
	t := &NPCTables{
		NPCs:      make([]NPC, NumNPCs+1),
		Greetings: make([][2]string, NumGreetings+1),
		GroupNews: make([]int16, NumGroups),
		News:      make([]string, NumGroups),
	}
	for i, line := range rows(npcdata, 2, NumNPCs) {
		n := &t.NPCs[i+1]
		walk(line, 16, false, func(col int, s string) {
			switch {
			case col == 1:
				n.Name = txt.StripQuotes(s)
			case col == 2:
				n.Portrait = txt.Atoi(s)
			case col == 6:
				n.House = txt.Atoi(s)
			case col == 7:
				n.Profession = txt.Atoi(s)
			case col == 8:
				n.Greeting = txt.Atoi(s)
			case col == 9:
				if s[0] == 'y' {
					n.Join = 1
				}
			case col >= 10:
				n.Topics[col-10] = txt.Atoi(s)
			}
		})
	}
	for i, line := range rows(npcgreet, 1, NumGreetings) {
		g := &t.Greetings[i+1]
		walk(line, 3, false, func(col int, s string) {
			if col >= 1 {
				g[col-1] = txt.StripQuotes(s)
			}
		})
	}
	for i, line := range rows(npcgroup, 1, NumGroups) {
		walk(line, 2, false, func(col int, s string) {
			if col == 1 {
				t.GroupNews[i] = int16(txt.Atoi(s))
			}
		})
	}
	for i, line := range rows(npcnews, 1, NumGroups) {
		walk(line, 2, false, func(col int, s string) {
			if col == 1 {
				t.News[i] = txt.StripQuotes(s)
			}
		})
	}
	return t
}

// Row counts of the topic and text tables (0x75e348: {topic, text} per id).
const (
	NumTopics = 750
	NumTexts  = 1000
)

// Topics are npctopic.txt and npctext.txt by id.
type Topics struct {
	Topic []string // ids 1..750
	Text  []string // ids 1..1000
}

// ParseTopics reads npctext.txt and npctopic.txt; column 1 of each row, file order. An
// empty cell ends its row.
//
// mm8: 0x4774ad (Txt_LoadNpcTopicText)
func ParseTopics(npctopic, npctext []byte) *Topics {
	t := &Topics{Topic: column1(npctopic, NumTopics), Text: column1(npctext, NumTexts)}
	return t
}

// column1 is the shape most loaders share: one header line, n rows, column 1 stripped of
// quotes, an empty cell ending the row; the result is 1-based.
func column1(data []byte, n int) []string {
	out := make([]string, n+1)
	for i, line := range rows(data, 1, n) {
		walk(line, 3, true, func(col int, s string) {
			if col == 1 {
				out[i+1] = txt.StripQuotes(s)
			}
		})
	}
	return out
}

// NumQuests is the number of quests.txt rows (ids 1..512, 0x760290).
const NumQuests = 512

// ParseQuests reads quests.txt (the quest log text of each quest bit).
//
// mm8: 0x4773dd (Txt_LoadQuests)
func ParseQuests(data []byte) []string { return column1(data, NumQuests) }

// Autonote is an autonote.txt row (0x760a98 + id*8).
type Autonote struct {
	Text     string
	Category int32 // 0 potion, 1 stat, 2 obelisk, 3 seer, 4 teacher, 5 other
}

// NumAutonotes is the number of autonote rows (ids 1..300).
const NumAutonotes = 300

// ParseAutonotes reads autonote.txt.
//
// mm8: 0x477282 (Txt_LoadAutonote)
func ParseAutonotes(data []byte) []Autonote {
	out := make([]Autonote, NumAutonotes+1)
	for i, line := range rows(data, 1, NumAutonotes) {
		a := &out[i+1]
		walk(line, 4, true, func(col int, s string) {
			switch col {
			case 1:
				a.Text = txt.StripQuotes(s)
			case 2:
				switch {
				case strings.EqualFold(s, "potion"):
					a.Category = 0
				case strings.EqualFold(s, "stat"):
					a.Category = 1
				case strings.EqualFold(s, "seer"):
					a.Category = 3
				case strings.EqualFold(s, "obelisk"):
					a.Category = 2
				case strings.EqualFold(s, "teacher"):
					a.Category = 4
				default:
					a.Category = 5
				}
			}
		})
	}
	return out
}

// Award is an awards.txt row (0x761548 + id*8).
type Award struct {
	Text string
	Sort int32
}

// NumAwards is the number of awards rows (ids 1..104).
const NumAwards = 104

// ParseAwards reads awards.txt.
//
// mm8: 0x476f0a (Txt_LoadAwards)
func ParseAwards(data []byte) []Award {
	out := make([]Award, NumAwards+1)
	for i, line := range rows(data, 1, NumAwards) {
		a := &out[i+1]
		walk(line, 4, true, func(col int, s string) {
			switch col {
			case 1:
				a.Text = txt.StripQuotes(s)
			case 2:
				a.Sort = txt.Atoi(s)
			}
		})
	}
	return out
}

// Merchant replies: rows of merchant.txt (not enough gold, no/regular/good merchant
// skill, wrong type of merchant, unnecessary, stolen item) by column.
type Merchant struct {
	Buy, Sell, Repair, Identify [7]string
}

// ParseMerchant reads merchant.txt (four arrays of 7 at 0x77a0e0, 0x77a0fc, 0x77a118,
// 0x77a134).
//
// mm8: 0x4770be (Txt_LoadMerchant)
func ParseMerchant(data []byte) *Merchant {
	m := &Merchant{}
	for i, line := range rows(data, 1, 7) {
		walk(line, 6, true, func(col int, s string) {
			switch col {
			case 1:
				m.Buy[i] = txt.StripQuotes(s)
			case 2:
				m.Sell[i] = txt.StripQuotes(s)
			case 3:
				m.Repair[i] = txt.StripQuotes(s)
			case 4:
				m.Identify[i] = txt.StripQuotes(s)
			}
		})
	}
	return m
}

// NumTrans is the number of trans.txt rows (0x4fce00..0x4fd5d0).
const NumTrans = 500

// ParseTrans reads trans.txt, the transition descriptions: index k is the row with 2D#
// k (the game reads 0x4fcdfc[k]), entry 0 unused.
//
// mm8: 0x4771b2 (Txt_LoadTrans)
func ParseTrans(data []byte) []string { return column1(data, NumTrans) }

// RosterEntry is a roster.txt row: a character that can join the party (g_players
// 0xb2177c, 50 of 0x1d28 bytes; entry 0 is the created hero). Only what the party uses
// before M7 is decoded; Cells keeps the row for the stats, skills and items.
type RosterEntry struct {
	Name       string
	Class      int // ClassByName of the Class column
	Birth      int32
	Face       int32 // Picture
	Voice      int32
	Experience int64
	Level      int32
	Cells      []string
}

// NumRoster is the number of roster rows (g_players).
const NumRoster = 50

// ParseRoster reads roster.txt: two header lines, then 50 rows.
//
// mm8: 0x49680a (Txt_LoadRoster: name col 1, class col 2, birth col 3 (at most 1156),
// picture col 4, voice col 5, experience col 6 (_atoi64), level col 7 (at least 1))
func ParseRoster(data []byte) []RosterEntry {
	out := make([]RosterEntry, NumRoster)
	for i, line := range rows(data, 2, NumRoster) {
		c := txt.Cells(line)
		cell := func(k int) string {
			if k < len(c) {
				return c[k]
			}
			return ""
		}
		e := &out[i]
		e.Cells = c
		e.Name = cell(1)
		if len(e.Name) > 0x1f {
			e.Name = e.Name[:0x1f]
		}
		e.Class = ClassByName(cell(2))
		e.Birth = min(txt.Atoi(cell(3)), 0x484)
		e.Face = txt.Atoi(cell(4))
		e.Voice = txt.Atoi(cell(5))
		e.Experience = int64(txt.Atoi(cell(6)))
		e.Level = max(txt.Atoi(cell(7)), 1)
	}
	return out
}

// classNames are the class names roster.txt uses, by class id (two spellings for some).
var classNames = [][]string{
	{"necromancer"}, {"necromancer2", "lich"}, {"cleric"}, {"cleric2", "PRIESTOFSUN"},
	{"knight"}, {"knight2", "champion"}, {"troll"}, {"troll2"}, {"minotaur"}, {"minotaur2"},
	{"darkelf"}, {"darkelf2"}, {"vampire"}, {"vampire2"}, {"dragon"}, {"dragon2"},
}

// ClassByName is the class id of a roster class name, case-insensitive; unknown names
// are 0 (Necromancer).
//
// mm8: 0x496cae
func ClassByName(s string) int {
	for id, names := range classNames {
		for _, n := range names {
			if strings.EqualFold(n, s) {
				return id
			}
		}
	}
	return 0
}
