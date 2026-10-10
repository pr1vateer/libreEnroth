package tables

import (
	"encoding/binary"
	"errors"
	"fmt"

	"libre-enroth/internal/assets"
	"libre-enroth/internal/assets/exe"
)

// All is every table this package parses, as the game holds them after Game_Init.
type All struct {
	Houses     []House
	HouseAnims []HouseAnim
	NPC        *NPCTables
	Topics     *Topics
	Quests     []string
	Autonotes  []Autonote
	Awards     []Award
	Merchant   *Merchant
	Trans      []string
	Scrolls    []string // scroll.txt
	Roster     []RosterEntry
	Travel     *Travel
	Shops      *Shops
	Learning   *Learning
	// Items are the item tables and the descriptions loaded with them; Classes the class
	// and stat tables of the executable.
	Items   *Items
	Classes *Classes
	// Monsters are monsters.txt, placemon.txt, hostile.txt, dmonlist.bin, dobjlist.bin and
	// MapStats.txt's rows.
	Monsters *Monsters
	// Spells are the spells' schools and damage dice (spells.txt, the exe).
	Spells Spells
}

// Load reads the tables from the language LODs and the house clip and travel tables
// from MM8-Rel.exe.
//
// mm8: 0x4779fb (the NPC/topic/quest/autonote/award/trans/merchant loaders),
// 0x4414e8 (Txt_Load2DEvents), 0x49680a (Txt_LoadRoster)
func Load(d *assets.Data) (*All, error) {
	files := map[string][]byte{}
	for _, n := range []string{"2DEvents.txt", "npcdata.txt", "npcgreet.txt", "npcgroup.txt", "npcnews.txt",
		"npctopic.txt", "npctext.txt", "quests.txt", "autonote.txt", "awards.txt", "merchant.txt",
		"trans.txt", "roster.txt", "scroll.txt"} {
		_, b, err := d.LangFile(n)
		if err != nil {
			return nil, err
		}
		files[n] = b
	}
	a := &All{
		Houses:    ParseHouses(files["2DEvents.txt"]),
		NPC:       ParseNPCs(files["npcdata.txt"], files["npcgreet.txt"], files["npcgroup.txt"], files["npcnews.txt"]),
		Topics:    ParseTopics(files["npctopic.txt"], files["npctext.txt"]),
		Quests:    ParseQuests(files["quests.txt"]),
		Autonotes: ParseAutonotes(files["autonote.txt"]),
		Awards:    ParseAwards(files["awards.txt"]),
		Merchant:  ParseMerchant(files["merchant.txt"]),
		Trans:     ParseTrans(files["trans.txt"]),
		Scrolls:   ParseScrolls(files["scroll.txt"]),
		Roster:    ParseRoster(files["roster.txt"]),
	}
	var err error
	if a.Items, err = LoadItems(d); err != nil {
		return nil, err
	}
	if a.Classes, err = ReadClasses(d.Exe); err != nil {
		return nil, err
	}
	if a.HouseAnims, err = ReadHouseAnims(d.Exe); err != nil {
		return nil, err
	}
	if a.Travel, err = ReadTravel(d.Exe); err != nil {
		return nil, err
	}
	if a.Shops, err = ReadShops(d.Exe); err != nil {
		return nil, err
	}
	if a.Learning, err = ReadLearning(d.Exe); err != nil {
		return nil, err
	}
	if a.Monsters, err = LoadMonsters(d); err != nil {
		return nil, err
	}
	if a.Spells, err = LoadSpells(d); err != nil {
		return nil, err
	}
	return a, nil
}

// HouseAnim is a g_houseAnims record (0x10 bytes at 0x4f86a0): the clip, the proprietor
// portrait and the house type the house screen goes by.
type HouseAnim struct {
	Video     string // +0x00 clip name, ".smk" appended
	Unk04     int32  // +0x04
	Portrait  int32  // +0x08 the proprietor's npc%04u, 0 = no proprietor
	Type      uint8  // +0x0c house type (re/notes/houses.md)
	RoomSound uint8  // +0x0d sounds (RoomSound + 300) * 100 + n
}

// Exe addresses of the house tables.
const (
	vaHouseAnims  = 0x4f86a0
	NumHouseAnims = 0xa2
	vaExitIcons   = 0x4f86a0 + 0xa1*0x10 // ticon01..03 follow the table: [exitPic] of this
)

// ReadHouseAnims reads g_houseAnims from the executable.
//
// mm8: 0x4be408 (House_Anim)
func ReadHouseAnims(im *exe.Image) ([]HouseAnim, error) {
	raw, err := im.Read(vaHouseAnims, NumHouseAnims*0x10)
	if err != nil {
		return nil, err
	}
	out := make([]HouseAnim, NumHouseAnims)
	for i := range out {
		r := raw[i*0x10:]
		p := binary.LittleEndian.Uint32(r)
		name, err := im.CString(p)
		if errors.Is(err, exe.ErrUnmapped) {
			name = "" // entry 161 points at an empty string in the uninitialised data
		} else if err != nil {
			return nil, fmt.Errorf("house anim %d: %w", i, err)
		}
		out[i] = HouseAnim{
			Video: name, Unk04: int32(binary.LittleEndian.Uint32(r[4:])),
			Portrait: int32(binary.LittleEndian.Uint32(r[8:])), Type: r[0xc], RoomSound: r[0xd],
		}
	}
	return out, nil
}

// Anim is the clip record of video index v, entry 0 when out of range.
//
// mm8: 0x4be408 (House_Anim)
func (a *All) Anim(v int) *HouseAnim {
	if v < 0 || v >= NumHouseAnims {
		v = 0
	}
	return &a.HouseAnims[v]
}

// House returns house id's row, nil when out of range.
func (a *All) House(id int) *House {
	if id <= 0 || id >= len(a.Houses) {
		return nil
	}
	return &a.Houses[id]
}

// ExitIcon is the portrait of an "Other Exits" picture (1..3: ticon01..03).
//
// mm8: 0x443da1 (House_LoadResidents: (&g_houseAnims[0xa1].video)[exitPic])
func ExitIcon(im *exe.Image, pic int) (string, error) {
	if pic < 1 || pic > 3 {
		return "", fmt.Errorf("exit picture %d", pic)
	}
	p, err := im.Int32s(vaExitIcons+uint32(pic)*4, 1)
	if err != nil {
		return "", err
	}
	return im.CString(uint32(p[0]))
}

// LoadItems reads the items table from the language LODs, with the item sizes from
// icons.lod.
//
// mm8: 0x455a6e (Txt_LoadItemsClassesSkills)
func LoadItems(d *assets.Data) (*Items, error) {
	files := map[string][]byte{}
	for _, n := range ItemFiles {
		_, b, err := d.LangFile(n)
		if err != nil {
			return nil, err
		}
		files[n] = b
	}
	return ParseItems(files, d.Icons), nil
}

// LoadChests reads the chest tables from MM8-Rel.exe and dchest.bin.
//
// mm8: 0x4205af, dchest.bin (Lod_LoadLanguageFile("dchest.bin", 1))
func LoadChests(d *assets.Data) (*Chests, error) {
	if d.Exe == nil {
		return nil, fmt.Errorf("chests: MM8-Rel.exe not opened")
	}
	_, b, err := d.LangFile("dchest.bin")
	if err != nil {
		return nil, err
	}
	return ReadChests(d.Exe, b)
}
