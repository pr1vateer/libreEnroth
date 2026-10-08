package tables

import (
	"encoding/binary"

	"libre-enroth/internal/assets/exe"
)

// The tables of the houses' teaching compiled into MM8-Rel.exe: how far each training
// hall trains and the quest items an NPC gives back when they are lost
// (re/notes/houses.md#skills-m7d).

// Training halls are houses 0x59..0x65.
const (
	FirstTrainingHall = 0x59
	NumTrainingHalls  = 13
	NumLostItems      = 25
)

// Exe addresses of the teaching tables.
const (
	vaLevelCaps = 0x502c4a // ushort [house]
	vaLostItems = 0x502c08 // {int qbit, item} [25]
)

// LostItem is a quest item NPC_LostItemTopic gives back: the item while its quest bit
// is set and nobody in the party has it.
type LostItem struct {
	QBit, Item int32
}

// Learning holds the teaching tables.
type Learning struct {
	// LevelCaps is the level each house trains to (training halls 0x59..0x65; 0 does
	// not train, 0xffff has no limit).
	LevelCaps [FirstTrainingHall + NumTrainingHalls]uint16
	LostItems [NumLostItems]LostItem
}

// ReadLearning reads the teaching tables from the executable.
//
// mm8: 0x4b5573 (Training_Price), 0x4b5618 (House_DrawTraining), 0x4b2ac7
// (NPC_LostItemTopic)
func ReadLearning(im *exe.Image) (*Learning, error) {
	l := &Learning{}
	b, err := im.Read(vaLevelCaps, 2*len(l.LevelCaps))
	if err != nil {
		return nil, err
	}
	for i := range l.LevelCaps {
		l.LevelCaps[i] = binary.LittleEndian.Uint16(b[2*i:])
	}
	v, err := im.Int32s(vaLostItems, 2*NumLostItems)
	if err != nil {
		return nil, err
	}
	for i := range l.LostItems {
		l.LostItems[i] = LostItem{QBit: v[2*i], Item: v[2*i+1]}
	}
	return l, nil
}

// LevelCap is the level house trains to, 0 for a house that does not train.
func (l *Learning) LevelCap(house int) int {
	if house < 0 || house >= len(l.LevelCaps) {
		return 0
	}
	return int(l.LevelCaps[house])
}

// SkillNameGlobal is the global.txt string of each skill's name, as Txt_LoadGlobal
// copies them into the skill name table (0xbb2f58); entry 39 is "None".
//
// mm8: 0x45181f (Txt_LoadGlobal)
var SkillNameGlobal = [NumSkills + 1]int{
	0x10f, 0x110, 0x111, 0x112, 0x113, 0x114, 0x115, 0x116, 0x117, 0x118,
	0x119, 0x11a, 0x11b, 0x11c, 0x11d, 0x11e, 0x121, 0x122, 0x123, 0x11f,
	0x120, 0x2b5, 0x2b6, 0x2b7, 0x124, 0x125, 0x126, 0x127, 0x128, 0x129,
	0xea, 300, 0x32, 0x4d, 0x58, 0x59, 0x5a, 0x5f, 0x12d, 0x99,
}

// ClassNameGlobal is the global.txt string of class 0's name; class c is at
// ClassNameGlobal + c (Txt_LoadGlobal 0x45181f).
const ClassNameGlobal = 0x2a5
