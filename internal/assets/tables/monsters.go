package tables

import (
	"encoding/binary"
	"fmt"
	"strings"

	"libre-enroth/internal/assets"
	"libre-enroth/internal/assets/txt"
)

// The monster tables: monsters.txt, placemon.txt and hostile.txt as Txt_LoadMonsters,
// Txt_LoadPlaceMon and Txt_LoadHostile fill them, dmonlist.bin and dobjlist.bin as Game_Init
// copies them, and MapStats.txt's rows (re/notes/monsters.md).

// Monster table sizes.
const (
	NumMonsters      = 268  // g_monsters.count: ids 1..267 load
	NumPlaceMon      = 0x83 // placemon names 1..130
	NumHostile       = 0x43 // hostile.txt rows (class 0 = the party)
	HostileStride    = 0x59 // g_hostile row length
	HostileColumns   = 0x44 // columns 1..0x44 are read
	MonListEntrySize = 0xb8
	ObjListEntrySize = 0x38
	Immune           = 65000 // a resistance of 'i'
)

// MonsterInfo is a monsters.txt row (0x60 bytes at 0x5e9448 + id*0x60); an actor holds a
// copy. Field names follow types.h.
type MonsterInfo struct {
	Name, Picture  string // +0x00, +0x04
	Level          uint8  // +0x08
	TreasureChance uint8  // +0x09
	GoldDice       uint8  // +0x0a
	GoldSides      uint8  // +0x0b
	ItemLevel      uint8  // +0x0c
	ItemKind       uint8  // +0x0d
	Fly            uint8  // +0x0e: _strnicmp(cell, "n", 1), 0 = walks
	Move           uint8  // +0x0f: Move*
	AI             uint8  // +0x10: AI*
	Hostility      uint8  // +0x11
	Unk12          uint8  // +0x12
	AttackBonus    uint8  // +0x13: Bonus*
	AttackBonusMul uint8  // +0x14
	Attack1        Attack // +0x15..+0x19
	Attack2Chance  uint8  // +0x1a
	Attack2        Attack // +0x1b..+0x1f
	Spell1Chance   uint8  // +0x20
	Spell1         uint8  // +0x21
	Spell2Chance   uint8  // +0x22
	Spell2         uint8  // +0x23
	Resist         [10]uint16
	Special        uint8  // +0x38: Special*
	SpecialA       uint8  // +0x39
	SpecialB       uint8  // +0x3a
	SpecialC       uint8  // +0x3b
	PrefCount      uint8  // +0x3c
	Unk3d          uint8  // +0x3d
	ID             uint16 // +0x3e
	Quest          uint16 // +0x40
	Spell1Skill    uint16 // +0x42
	Spell2Skill    uint16 // +0x44
	SpecialD       uint16 // +0x46
	HP             int32  // +0x48
	AC             int32  // +0x4c
	Exp            int32  // +0x50
	Speed          int32  // +0x54
	Recovery       int32  // +0x58
	Pref           uint32 // +0x5c
	// NameID and PictureID are the string pointers of a record decoded from an actor
	// (+0x00/+0x04): kept so that the record encodes back as it was.
	NameID, PictureID uint32
}

// Attack is one of a monster's two attacks.
type Attack struct {
	Type, Dice, Sides, Add, Missile uint8
}

// MonsterInfo enumerations (Txt_LoadMonsters 0x453bc9).
const (
	MoveShort      = 0
	MoveMedium     = 1
	MoveLong       = 2
	MoveGlobal     = 3
	MoveFree       = 4
	MoveStationary = 5

	AISuicidal   = 0
	AIWimp       = 1
	AINormal     = 2
	AIAggressive = 3

	SpecialShot    = 1
	SpecialSummon  = 2
	SpecialExplode = 3
)

// Decode reads a 0x60-byte MonsterInfo (an actor's copy); the name and picture pointers
// are kept as numbers.
func (m *MonsterInfo) Decode(b []byte) {
	le := binary.LittleEndian
	*m = MonsterInfo{
		NameID: le.Uint32(b), PictureID: le.Uint32(b[4:]),
		Level: b[8], TreasureChance: b[9], GoldDice: b[0xa], GoldSides: b[0xb], ItemLevel: b[0xc], ItemKind: b[0xd],
		Fly: b[0xe], Move: b[0xf], AI: b[0x10], Hostility: b[0x11], Unk12: b[0x12], AttackBonus: b[0x13], AttackBonusMul: b[0x14],
		Attack1:       Attack{b[0x15], b[0x16], b[0x17], b[0x18], b[0x19]},
		Attack2Chance: b[0x1a], Attack2: Attack{b[0x1b], b[0x1c], b[0x1d], b[0x1e], b[0x1f]},
		Spell1Chance: b[0x20], Spell1: b[0x21], Spell2Chance: b[0x22], Spell2: b[0x23],
		Special: b[0x38], SpecialA: b[0x39], SpecialB: b[0x3a], SpecialC: b[0x3b], PrefCount: b[0x3c], Unk3d: b[0x3d],
		ID: le.Uint16(b[0x3e:]), Quest: le.Uint16(b[0x40:]), Spell1Skill: le.Uint16(b[0x42:]), Spell2Skill: le.Uint16(b[0x44:]),
		SpecialD: le.Uint16(b[0x46:]),
		HP:       int32(le.Uint32(b[0x48:])), AC: int32(le.Uint32(b[0x4c:])), Exp: int32(le.Uint32(b[0x50:])),
		Speed: int32(le.Uint32(b[0x54:])), Recovery: int32(le.Uint32(b[0x58:])), Pref: le.Uint32(b[0x5c:]),
	}
	for i := range m.Resist {
		m.Resist[i] = le.Uint16(b[0x24+2*i:])
	}
}

// Encode writes the 0x60-byte record (the pointers as NameID/PictureID).
func (m *MonsterInfo) Encode(b []byte) {
	le := binary.LittleEndian
	le.PutUint32(b, m.NameID)
	le.PutUint32(b[4:], m.PictureID)
	copy(b[8:0x15], []byte{m.Level, m.TreasureChance, m.GoldDice, m.GoldSides, m.ItemLevel, m.ItemKind,
		m.Fly, m.Move, m.AI, m.Hostility, m.Unk12, m.AttackBonus, m.AttackBonusMul})
	a1, a2 := m.Attack1, m.Attack2
	copy(b[0x15:0x24], []byte{a1.Type, a1.Dice, a1.Sides, a1.Add, a1.Missile, m.Attack2Chance,
		a2.Type, a2.Dice, a2.Sides, a2.Add, a2.Missile, m.Spell1Chance, m.Spell1, m.Spell2Chance, m.Spell2})
	for i, r := range m.Resist {
		le.PutUint16(b[0x24+2*i:], r)
	}
	copy(b[0x38:0x3e], []byte{m.Special, m.SpecialA, m.SpecialB, m.SpecialC, m.PrefCount, m.Unk3d})
	le.PutUint16(b[0x3e:], m.ID)
	le.PutUint16(b[0x40:], m.Quest)
	le.PutUint16(b[0x42:], m.Spell1Skill)
	le.PutUint16(b[0x44:], m.Spell2Skill)
	le.PutUint16(b[0x46:], m.SpecialD)
	for i, v := range []int32{m.HP, m.AC, m.Exp, m.Speed, m.Recovery} {
		le.PutUint32(b[0x48+4*i:], uint32(v))
	}
	le.PutUint32(b[0x5c:], m.Pref)
}

// MonListEntry is a dmonlist.bin record (0xb8 bytes).
type MonListEntry struct {
	Height      uint16    // +0x00
	Radius      uint16    // +0x02
	Speed       uint16    // +0x04
	ToHitRadius int16     // +0x06
	Tint        uint32    // +0x08: D3D colour 0xAARRGGBB multiplied into the sprite
	Sounds      [4]uint16 // +0x0c
	Name        string    // +0x14 char[64]
	Sprites     [10]string
}

// Monster sprite slots (MonListEntry.Sprites, Actor.Sprites): the animations.
const (
	AnimStand   = 0
	AnimWalk    = 1
	AnimAttack  = 2
	AnimShoot   = 3
	AnimGotHit  = 4
	AnimDying   = 5
	AnimDead    = 6
	AnimFidget  = 7
	NumAnimSets = 8
)

// ObjListEntry is a dobjlist.bin record (0x38 bytes).
type ObjListEntry struct {
	Name     string // +0x00 char[32]
	ID       int16  // +0x20: the object type; items.txt's sprite number
	Radius   int16  // +0x22
	Height   int16  // +0x24
	Flags    uint16 // +0x26: Obj*
	SFT      int16  // +0x28: dsft.bin frame
	Lifetime int16  // +0x2a
	Speed    int16  // +0x30
	Particle [3]uint8
}

// Object kind flags (ObjListEntry.Flags; ObjList_LoadTxt 0x457e6f).
const (
	ObjNoDraw          = 0x1
	ObjLifetime        = 0x4
	ObjFTLifetime      = 0x8
	ObjNoPickup        = 0x10
	ObjNoGravity       = 0x20
	ObjFlagOnIntercept = 0x40
	ObjBounce          = 0x80
)

// MapStats is a MapStats.txt row (0x44 bytes at 0x5e6d18 + row*0x44).
type MapStats struct {
	Name, File    string    // +0x00, +0x04
	Monster       [3]string // +0x08 Mon1..3 Pic: the dmonlist name without " A"
	Resets        int32     // +0x14 col 3
	FirstVisitDay int32     // +0x18 col 4
	RefillDays    int32     // +0x1c col 6
	AlertDays     int32     // +0x20 col 7
	StealPerm     int32     // +0x24 col 8
	Per           int32     // +0x28 col 5
	Lock          uint8     // +0x2d col 9
	Trap          uint8     // +0x2e col 10
	Treasure      uint8     // +0x2f col 11
	Encounter     uint8     // +0x30 col 12
	EncounterKind [3]uint8  // +0x31 col 13..15
	Dif           [3]uint8  // +0x34/+0x37/+0x3a
	Min, Max      [3]uint8  // +0x35/+0x36, ...
	Redbook       uint8     // +0x40 col 28 (+0x41, col 29's EAX environment, is M11's)
}

// Monsters are the monster tables.
type Monsters struct {
	// Infos is g_monsters: [id], 1..267 (index 0 and rows the file lacks are zero).
	Infos []MonsterInfo
	// PlaceMon is placemon.txt: [1..130].
	PlaceMon []string
	// Hostile is g_hostile: [class a * HostileStride + class b].
	Hostile []uint8
	MonList []MonListEntry
	ObjList []ObjListEntry
	// MapStats is MapStats.txt: [row], row 0 unused.
	MapStats []MapStats
	// Treasure is g_treasureLevels: {min, max} at [index*7 + map treasure], index 0..7.
	Treasure [8 * 7][2]uint8
}

// LoadMonsters reads the monster tables from the language LODs and g_treasureLevels from
// the executable.
//
// mm8: 0x455a6e (Txt_LoadItemsClassesSkills: monsters.txt, placemon.txt, hostile.txt,
// MapStats.txt), 0x464974 (Game_Init: dmonlist.bin, dobjlist.bin)
func LoadMonsters(d *assets.Data) (*Monsters, error) {
	files := map[string][]byte{}
	for _, n := range []string{"monsters.txt", "placemon.txt", "hostile.txt", "mapstats.txt", "dmonlist.bin", "dobjlist.bin"} {
		_, b, err := d.LangFile(n)
		if err != nil {
			return nil, err
		}
		files[n] = b
	}
	m := &Monsters{}
	var err error
	if m.MonList, err = ParseMonList(files["dmonlist.bin"]); err != nil {
		return nil, err
	}
	if m.ObjList, err = ParseObjList(files["dobjlist.bin"]); err != nil {
		return nil, err
	}
	m.Infos = ParseMonsters(files["monsters.txt"], m.MonList)
	m.PlaceMon = ParsePlaceMon(files["placemon.txt"])
	m.Hostile = ParseHostile(files["hostile.txt"])
	m.MapStats = ParseMapStats(files["mapstats.txt"])
	if d.Exe != nil {
		b, err := d.Exe.Read(vaTreasure, 2*8*7)
		if err != nil {
			return nil, err
		}
		for i := range m.Treasure {
			m.Treasure[i] = [2]uint8{b[2*i], b[2*i+1]}
		}
	}
	return m, nil
}

// Info is monster id's monsters.txt row, nil when out of range.
func (m *Monsters) Info(id int) *MonsterInfo {
	if id <= 0 || id >= len(m.Infos) {
		return nil
	}
	return &m.Infos[id]
}

// FindMonList is the dmonlist.bin index of name (case-insensitive), -1 when absent.
//
// mm8: 0x44e4a2 (MonList_Find: index & 0xffff, 0xffff when absent)
func (m *Monsters) FindMonList(name string) int {
	return findMonList(m.MonList, name)
}

func findMonList(l []MonListEntry, name string) int {
	for i := range l {
		if strings.EqualFold(l[i].Name, name) {
			return i
		}
	}
	return -1
}

// FindByPicture is the id of the first monster whose picture is name, -1 when none.
//
// mm8: 0x4550a2 (Monsters_FindByPicture: ids 1..count-1 with a name)
func (m *Monsters) FindByPicture(name string) int {
	for id := 1; id < len(m.Infos); id++ {
		if in := &m.Infos[id]; in.Name != "" && strings.EqualFold(in.Picture, name) {
			return id
		}
	}
	return -1
}

// ObjListOf is the dobjlist.bin index whose ID is id (items.txt's sprite), 0 when none.
//
// mm8: 0x44ea5f (Spawn_Items' scan of g_objList for the item's sprite)
func (m *Monsters) ObjListOf(id int16) int {
	for i := range m.ObjList {
		if m.ObjList[i].ID == id {
			return i
		}
	}
	return 0
}

// Class is a monster id's hostile.txt class: (id - 1) / 3 + 1, 0 for none (the party).
//
// mm8: 0x401051 (Actor_GetRelation)
func Class(id int) int {
	if id == 0 {
		return 0
	}
	return (id-1)/3 + 1
}

// HostileAt is g_hostile[a * 0x59 + b].
func (m *Monsters) HostileAt(a, b int) uint8 {
	i := a*HostileStride + b
	if i < 0 || i >= len(m.Hostile) {
		return 0
	}
	return m.Hostile[i]
}

// MapStatsRow is MapStats.txt row n, nil when absent.
func (m *Monsters) MapStatsRow(n int) *MapStats {
	if n <= 0 || n >= len(m.MapStats) {
		return nil
	}
	return &m.MapStats[n]
}

// ParseMonList reads dmonlist.bin: u32 count, count x 0xb8.
//
// mm8: 0x45862d (MonList_LoadBin)
func ParseMonList(b []byte) ([]MonListEntry, error) {
	n, err := binCount("dmonlist.bin", b, MonListEntrySize)
	if err != nil {
		return nil, err
	}
	le := binary.LittleEndian
	out := make([]MonListEntry, n)
	for i := range out {
		r := b[4+i*MonListEntrySize:]
		e := &out[i]
		e.Height, e.Radius, e.Speed = le.Uint16(r), le.Uint16(r[2:]), le.Uint16(r[4:])
		e.ToHitRadius = int16(le.Uint16(r[6:]))
		e.Tint = le.Uint32(r[8:])
		for k := range e.Sounds {
			e.Sounds[k] = le.Uint16(r[0xc+2*k:])
		}
		e.Name = cstr(r[0x14:0x54])
		for k := range e.Sprites {
			e.Sprites[k] = cstr(r[0x54+10*k : 0x54+10*k+10])
		}
	}
	return out, nil
}

// ParseObjList reads dobjlist.bin: u32 count, count x 0x38.
//
// mm8: 0x457e28 (ObjList_LoadBin)
func ParseObjList(b []byte) ([]ObjListEntry, error) {
	n, err := binCount("dobjlist.bin", b, ObjListEntrySize)
	if err != nil {
		return nil, err
	}
	le := binary.LittleEndian
	out := make([]ObjListEntry, n)
	for i := range out {
		r := b[4+i*ObjListEntrySize:]
		out[i] = ObjListEntry{
			Name: cstr(r[:0x20]), ID: int16(le.Uint16(r[0x20:])), Radius: int16(le.Uint16(r[0x22:])),
			Height: int16(le.Uint16(r[0x24:])), Flags: le.Uint16(r[0x26:]), SFT: int16(le.Uint16(r[0x28:])),
			Lifetime: int16(le.Uint16(r[0x2a:])), Speed: int16(le.Uint16(r[0x30:])),
			Particle: [3]uint8{r[0x32], r[0x33], r[0x34]},
		}
	}
	return out, nil
}

func binCount(name string, b []byte, size int) (int, error) {
	if len(b) < 4 {
		return 0, fmt.Errorf("%s: %d bytes", name, len(b))
	}
	n := int(int32(binary.LittleEndian.Uint32(b)))
	if n < 0 || 4+n*size > len(b) {
		return 0, fmt.Errorf("%s: %d records of %#x bytes do not fit %d bytes", name, n, size, len(b))
	}
	return n, nil
}

func cstr(b []byte) string {
	for i, c := range b {
		if c == 0 {
			return string(b[:i])
		}
	}
	return string(b)
}

// cellsOf walks a loader's line: the cells up to the first empty one (which ends the
// row, as "iVar == 0 -> local_2c = 1" does).
func cellsOf(line string) []string {
	cells := txt.Cells(line)
	for i, c := range cells {
		if c == "" {
			return cells[:i]
		}
	}
	return cells
}

func first(s string) byte {
	if s == "" {
		return 0
	}
	return lower(s[0])
}

// thousands is the HP/EXP parse: a leading quote becomes a space, and with a comma the
// value is atoi(before) * 1000 + atoi(after).
func thousands(s string) int32 {
	if s != "" && s[0] == '"' {
		s = " " + s[1:]
	}
	if i := strings.IndexByte(s, ','); i >= 0 {
		return txt.Atoi(s[:i])*1000 + txt.Atoi(s[i+1:])
	}
	return txt.Atoi(s)
}

// DamageType reads a damage type cell by its first letter.
//
// mm8: 0x453853 (Txt_DamageType)
func DamageType(s string) uint8 {
	switch first(s) {
	case 'f':
		return 0
	case 'a':
		return 1
	case 'w':
		return 2
	case 'e':
		return 3
	case 's':
		return 6
	case 'm':
		if len(s) > 1 && lower(s[1]) == 'i' {
			return 7
		}
		return 5
	case 'l':
		return 9
	case 'd':
		return 10
	}
	return 4
}

// ParseDice reads "NdM+K": count N, sides M, add K; a bare number is N d 1.
//
// mm8: 0x453921 (Txt_ParseDice)
func ParseDice(s string) (dice, sides, add uint8) {
	sides = 1
	found := false
	for i := 0; i < len(s); i++ {
		switch lower(s[i]) {
		case 'd':
			dice, sides = uint8(txt.Atoi(s[:i])), uint8(txt.Atoi(s[i+1:]))
			found = true
		case '+':
			add = uint8(txt.Atoi(s[i+1:]))
		}
	}
	if found {
		return
	}
	if s != "" && s[0] >= '0' && s[0] <= '9' {
		dice, sides = uint8(txt.Atoi(s)), 1
	}
	return
}

var missileTypes = []string{"", "ARROW", "ARROWF", "FIRE", "AIR", "WATER", "EARTH", "SPIRIT", "MIND", "BODY", "LIGHT", "DARK", "", "ENER"}

// MissileType reads a missile cell (0 for none).
//
// mm8: 0x4539df (Txt_MissileType)
func MissileType(s string) uint8 {
	for i, n := range missileTypes {
		if n != "" && strings.EqualFold(s, n) {
			return uint8(i)
		}
	}
	return 0
}

// spellWords are Txt_SpellOfWords' first words: the spell and how many more words its
// name has (the later duplicates "Spirit" and "Ice" are unreachable and left out).
var spellWords = []struct {
	word  string
	spell uint8
	extra int
}{
	{"Dispel", 0x50, 1}, {"Day", 0x55, 2}, {"Hour", 0x56, 2}, {"Shield", 0x11, 0}, {"Spirit", 0x34, 1},
	{"Power", 0x4d, 1}, {"Meteor", 9, 1}, {"Lightning", 0x12, 1}, {"Implosion", 0x14, 0}, {"Stone", 0x26, 1},
	{"Haste", 5, 0}, {"Heroism", 0x33, 0}, {"Pain", 0x5f, 1}, {"Sparks", 0xf, 0}, {"Light", 0x4e, 1},
	{"Toxic", 0x5a, 1}, {"ShrapMetal", 0x5d, 0}, {"Paralyze", 0x51, 0}, {"Fireball", 6, 0}, {"Incinerate", 0xb, 0},
	{"Fire", 2, 1}, {"Inferno", 10, 0}, {"Poison", 0x18, 1}, {"Ice", 0x20, 1}, {"Sunray", 0x57, 0},
	{"Prismatic", 0x54, 1}, {"Rock", 0x29, 1}, {"Mass", 0x2c, 1}, {"Acid", 0x1d, 1}, {"Bless", 0x2e, 0},
	{"Dragon", 0x61, 1}, {"Reanimate", 0x59, 0}, {"Summon", 0x52, 1}, {"Fate", 0x2f, 0}, {"Harm", 0x46, 0},
	{"Mind", 0x3b, 1}, {"Blades", 0x27, 0}, {"Psychic", 0x41, 1}, {"Hammerhands", 0x49, 0}, {"Heal", 0x44, 0},
}

// SpellOfWords is the spell named by words[0] and how many more words its name takes;
// an unknown word is spell 0 and one more word.
//
// mm8: 0x453404 (Txt_SpellOfWords)
func SpellOfWords(words []string) (spell uint8, extra int) {
	if len(words) == 0 {
		return 0, 0
	}
	for _, w := range spellWords {
		if strings.EqualFold(words[0], w.word) {
			return w.spell, w.extra
		}
	}
	return 0, 1
}

// SplitWords splits at spaces, commas and tabs, with double quotes grouping, into at
// most 30 words.
//
// mm8: 0x4be472 (Txt_SplitWords)
func SplitWords(s string) []string {
	var out []string
	var cur []byte
	inQuote, start := false, true
	flush := func() {
		if cur != nil {
			out = append(out, string(cur))
		}
		cur = nil
	}
	for i := 0; i < len(s) && len(out) < 30; i++ {
		c := s[i]
		switch {
		case !inQuote && (c == ' ' || c == ',' || c == '\t'):
			flush()
			start = true
		case c == '"':
			flush()
			start = true
			if !inQuote && i+1 < len(s) && s[i+1] == '"' {
				out = append(out, "")
			}
			inQuote = !inQuote
		default:
			if start {
				cur = []byte{}
				start = false
			}
			cur = append(cur, c)
		}
	}
	flush()
	if len(out) > 30 {
		out = out[:30]
	}
	return out
}

// unquote is the Spl/Special cells' copy: its first and last characters (the quotes)
// become spaces.
func unquote(cell string) string {
	b := []byte(cell)
	if len(b) > 0 {
		b[0], b[len(b)-1] = ' ', ' '
	}
	return string(b)
}

// spellCell reads a "Spl,Mas,Skil" cell into the spell and its skill word.
func spellCell(cell string) (spell uint8, skill uint16) {
	w := SplitWords(unquote(cell))
	if len(w) < 3 {
		return 0, 0
	}
	spell, extra := SpellOfWords(w)
	i := 1 + extra // the 1-based word index the original advances is extra past word 1
	if i+1 < len(w) {
		skill = uint16(txt.Atoi(w[i+1])) & 0x3f
	} else {
		skill = 0
	}
	if i < len(w) {
		switch {
		case strings.EqualFold(w[i], "E"):
			skill |= 0x40
		case strings.EqualFold(w[i], "M"):
			skill |= 0x80
		case strings.EqualFold(w[i], "G"):
			skill |= 0x100
		}
	}
	return spell, skill
}

// itemKinds are the treasure kinds after "L<n>": WEAPON 0x14 .. SCROLL 0x2b, GEM 0x2e,
// ORE 0x2f.
var itemKinds = []struct {
	name string
	kind uint8
}{
	{"WEAPON", 0x14}, {"ARMOR", 0x15}, {"MISC", 0x16}, {"SWORD", 0x17}, {"DAGGER", 0x18}, {"AXE", 0x19},
	{"SPEAR", 0x1a}, {"BOW", 0x1b}, {"MACE", 0x1c}, {"CLUB", 0x1d}, {"STAFF", 0x1e}, {"LEATHER", 0x1f},
	{"CHAIN", 0x20}, {"PLATE", 0x21}, {"SHIELD", 0x22}, {"HELM", 0x23}, {"BELT", 0x24}, {"CAPE", 0x25},
	{"GAUNTLETS", 0x26}, {"BOOTS", 0x27}, {"RING", 0x28}, {"AMULET", 0x29}, {"WAND", 0x2a}, {"SCROLL", 0x2b},
	{"GEM", 0x2e}, {"ORE", 0x2f},
}

// treasure reads the Treasure column ("NN%NdM+Ln<KIND>").
func (m *MonsterInfo) treasure(s string) {
	var pct, dice, lvl bool
	for i := 0; i < len(s); i++ {
		switch lower(s[i]) {
		case '%':
			pct = true
		case 'd':
			dice = true
		case 'l':
			lvl = true
		}
	}
	switch {
	case pct:
		m.TreasureChance = uint8(txt.Atoi(s))
	case dice || lvl:
		m.TreasureChance = 100
	default:
		return
	}
	if dice {
		afterPct := false
		for i := 0; i < len(s); i++ {
			switch lower(s[i]) {
			case '%':
				m.GoldDice = uint8(txt.Atoi(s[i+1:]))
				afterPct = true
			case 'd':
				if !afterPct {
					m.GoldDice = uint8(txt.Atoi(s))
				}
				m.GoldSides = uint8(txt.Atoi(s[i+1:]))
				i = len(s)
			}
		}
	}
	if lvl {
		for i := 0; i < len(s); i++ {
			if lower(s[i]) != 'l' {
				continue
			}
			if i+1 < len(s) {
				m.ItemLevel = s[i+1] - '0'
			} else {
				m.ItemLevel = 0xd0 // NUL - '0'
			}
			if i+2 < len(s) {
				rest := s[i+2:]
				for _, k := range itemKinds {
					if strings.EqualFold(rest, k.name) {
						m.ItemKind = k.kind
						break
					}
				}
			}
			break
		}
	}
}

var bonusWords = []struct {
	word  string
	bonus uint8
}{
	{"curse", 1}, {"weak", 2}, {"asleep", 3}, {"afraid", 0x17}, {"drunk", 4}, {"insane", 5},
	{"poison1", 6}, {"poison2", 7}, {"poison3", 8}, {"disease1", 9}, {"disease2", 10}, {"disease3", 11},
	{"paralyze", 0xc}, {"uncon", 0xd}, {"dead", 0xe}, {"stone", 0xf}, {"errad", 0x10}, {"brkitem", 0x11},
	{"brkarmor", 0x12}, {"brkweapon", 0x13}, {"steal", 0x14}, {"age", 0x15}, {"drainsp", 0x16},
}

// special reads the Special column: "shot,xN", "summon,<where>,<name...> <variant>",
// "explode,<dice>,<type>".
func (m *MonsterInfo) special(cell string, monList []MonListEntry) {
	w := SplitWords(unquote(cell))
	if len(w) == 0 || len(w) >= 10 {
		return
	}
	word := func(i int) string {
		if i < len(w) {
			return w[i]
		}
		return ""
	}
	switch {
	case strings.EqualFold(w[0], "shot"):
		m.Special = SpecialShot
		if s := word(1); s != "" {
			m.SpecialC = uint8(txt.Atoi(s[1:]))
		}
	case strings.EqualFold(w[0], "summon"):
		m.Special = SpecialSummon
		name := ""
		if len(w) > 1 {
			name = word(2)
			if len(w) < 3 {
				m.SpecialA = 0
			}
			for i := 3; i < len(w); i++ {
				name += " " + w[i]
				if i == len(w)-1 {
					switch first(w[i]) {
					case 'a':
						m.SpecialA = 1
					case 'b':
						m.SpecialA = 2
					case 'c':
						m.SpecialA = 3
					default:
						m.SpecialA = 0
					}
				}
			}
		}
		if len(monList) != 0 {
			m.SpecialD = uint16(findMonList(monList, name) + 1)
		}
		m.SpecialB = 0
		if strings.EqualFold(word(1), "ground") {
			m.SpecialB = 1
		}
		if int16(m.SpecialD) == -1 {
			m.Special = 0
		}
	case strings.EqualFold(w[0], "explode"):
		m.Special = SpecialExplode
		m.SpecialA, m.SpecialB, m.SpecialC = ParseDice(word(1))
		m.SpecialD = uint16(DamageType(cell))
	}
}

// ParseMonsters reads monsters.txt into g_monsters [id]: 4 header lines, each row stored at
// its own id until an id reaches 267; monList resolves the summon specials.
//
// mm8: 0x453bc9 (Txt_LoadMonsters)
func ParseMonsters(data []byte, monList []MonListEntry) []MonsterInfo {
	out := make([]MonsterInfo, NumMonsters)
	lines := txt.Lines(data)
	if len(lines) <= 4 {
		return out
	}
	id := 0
	for _, line := range lines[4:] {
		for col, cell := range cellsOf(line) {
			if col == 0 {
				id = int(txt.Atoi(cell))
			}
			if id < 0 || id >= len(out) {
				break
			}
			out[id].column(col, cell, id, monList)
		}
		id++
		if id > 0x10b {
			break
		}
	}
	return out
}

func (m *MonsterInfo) column(col int, cell string, id int, monList []MonListEntry) {
	resist := func(i int) {
		if first(cell) == 'i' {
			m.Resist[i] = Immune
		} else {
			m.Resist[i] = uint16(txt.Atoi(cell))
		}
	}
	switch col {
	case 0:
		m.ID = uint16(id)
	case 1:
		m.Name = txt.StripQuotes(cell)
	case 2:
		m.Picture = txt.StripQuotes(cell)
	case 3:
		m.Level = uint8(txt.Atoi(cell))
	case 4:
		m.HP = thousands(cell)
	case 5:
		m.AC = txt.Atoi(cell)
	case 6:
		m.Exp = thousands(cell)
	case 7:
		m.treasure(cell)
	case 8:
		m.Quest = 0
		if txt.Atoi(cell) != 0 {
			m.Quest |= 1
		}
	case 9:
		m.Fly = first(cell) - 'n'
	case 10:
		switch first(cell) {
		case 's':
			m.Move = MoveStationary
			if len(cell) > 1 && lower(cell[1]) == 'h' {
				m.Move = MoveShort
			}
		case 'm':
			m.Move = MoveMedium
		case 'l':
			m.Move = MoveLong
		case 'g':
			m.Move = MoveGlobal
		default:
			m.Move = MoveFree
		}
	case 11:
		switch first(cell) {
		case 's':
			m.AI = AISuicidal
		case 'w':
			m.AI = AIWimp
		case 'n':
			m.AI = AINormal
		default:
			m.AI = AIAggressive
		}
	case 12:
		m.Hostility = uint8(txt.Atoi(cell))
	case 13:
		m.Speed = txt.Atoi(cell)
	case 14:
		m.Recovery = txt.Atoi(cell)
	case 15:
		for i := 0; i < len(cell); i++ {
			switch lower(cell[i]) {
			case 'k':
				m.Pref |= 4
			case '2', '3', '4':
				m.PrefCount = cell[i] - '0'
			case 'c':
				m.Pref |= 2
			case 'd':
				m.Pref |= 0x80
			case 'e':
				m.Pref |= 0x20
			case 'm':
				m.Pref |= 0x10
			case 'n':
				m.Pref |= 1
			case 'o':
				m.Pref |= 0x200
			case 't':
				m.Pref |= 8
			case 'v':
				m.Pref |= 0x40
			case 'x':
				m.Pref |= 0x100
			}
		}
	case 16:
		m.AttackBonusMul = 1
		s := cell
		if i := strings.IndexAny(s, "xX"); i >= 0 {
			m.AttackBonusMul = uint8(txt.Atoi(s[i+1:]))
			s = s[:i] + "d" + s[i+1:]
		}
		s = strings.ToLower(s)
		for _, b := range bonusWords {
			if strings.Contains(s, b.word) {
				m.AttackBonus = b.bonus
				break
			}
		}
	case 17:
		m.Attack1.Type = DamageType(cell)
	case 18:
		m.Attack1.Dice, m.Attack1.Sides, m.Attack1.Add = ParseDice(cell)
	case 19:
		m.Attack1.Missile = MissileType(cell)
	case 20:
		m.Attack2Chance = uint8(txt.Atoi(cell))
	case 21:
		m.Attack2.Type = DamageType(cell)
	case 22:
		m.Attack2.Dice, m.Attack2.Sides, m.Attack2.Add = ParseDice(cell)
	case 23:
		m.Attack2.Missile = MissileType(cell)
	case 24:
		m.Spell1Chance = uint8(txt.Atoi(cell))
	case 25:
		m.Spell1, m.Spell1Skill = spellCell(cell)
	case 26:
		m.Spell2Chance = uint8(txt.Atoi(cell))
	case 27:
		m.Spell2, m.Spell2Skill = spellCell(cell)
	case 28, 29, 30, 31, 32, 33, 34, 35, 36, 37:
		resist(col - 28)
	case 38:
		m.special(cell, monList)
	}
}

// ParsePlaceMon reads placemon.txt: 1 header line, 130 rows, column 1 -> [1..130].
//
// mm8: 0x453af4 (Txt_LoadPlaceMon)
func ParsePlaceMon(data []byte) []string {
	out := make([]string, NumPlaceMon)
	for i, line := range rows(data, 1, NumPlaceMon-1) {
		c := cellsOf(line)
		if len(c) > 1 {
			out[i+1] = txt.StripQuotes(c[1])
		}
	}
	return out
}

// ParseHostile reads hostile.txt: 1 header line, 67 rows; row r, column c (1..0x44) ->
// [(c - 1) * 0x59 + r].
//
// mm8: 0x453306 (Txt_LoadHostile)
func ParseHostile(data []byte) []uint8 {
	out := make([]uint8, HostileColumns*HostileStride)
	for r, line := range rows(data, 1, NumHostile) {
		for c, cell := range cellsOf(line) {
			if c > HostileColumns {
				break
			}
			if c > 0 {
				out[(c-1)*HostileStride+r] = uint8(txt.Atoi(cell))
			}
		}
	}
	return out
}

// mapStatsRows is g_mapStats' size: rows 1..0x4c.
const mapStatsRows = 0x4d

// ParseMapStats reads MapStats.txt: 3 header lines, rows 1..0x4c.
//
// mm8: 0x452a82 (Txt_LoadMapStats)
func ParseMapStats(data []byte) []MapStats {
	out := make([]MapStats, mapStatsRows)
	for i, line := range rows(data, 3, mapStatsRows-1) {
		m := &out[i+1]
		cells := cellsOf(line)
		for col, cell := range cells {
			count := func(k int) {
				m.Min[k] = 1
				if j := strings.IndexByte(cell, '-'); j >= 0 {
					m.Min[k] = uint8(txt.Atoi(cell[:j]))
					m.Max[k] = uint8(txt.Atoi(cell[j+1:]))
					return
				}
				// atoi past the cell's end: the next cell's text (normally a name, 0).
				next := ""
				if col+1 < len(cells) {
					next = cells[col+1]
				}
				m.Max[k] = uint8(txt.Atoi(next))
			}
			switch col {
			case 1:
				m.Name = txt.StripQuotes(cell)
			case 2:
				m.File = txt.StripQuotes(cell)
			case 3:
				m.Resets = txt.Atoi(cell)
			case 4:
				m.FirstVisitDay = txt.Atoi(cell)
			case 5:
				m.Per = txt.Atoi(cell)
			case 6:
				m.RefillDays = txt.Atoi(cell)
			case 7:
				m.AlertDays = txt.Atoi(cell)
			case 8:
				m.StealPerm = txt.Atoi(cell)
			case 9:
				m.Lock = uint8(txt.Atoi(cell))
			case 10:
				m.Trap = uint8(txt.Atoi(cell))
			case 11:
				m.Treasure = uint8(txt.Atoi(cell))
			case 12:
				m.Encounter = uint8(txt.Atoi(cell))
			case 13, 14, 15:
				m.EncounterKind[col-13] = uint8(txt.Atoi(cell))
			case 16, 20, 24:
				m.Monster[(col-16)/4] = txt.StripQuotes(cell)
			case 18, 22, 26:
				m.Dif[(col-18)/4] = uint8(txt.Atoi(cell))
			case 19, 23, 27:
				count((col - 19) / 4)
			case 28:
				m.Redbook = uint8(txt.Atoi(cell))
			}
		}
	}
	return out
}
