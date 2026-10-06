package tables

import (
	"encoding/binary"

	"libre-enroth/internal/assets/exe"
)

// The shop tables compiled into MM8-Rel.exe: what each shop's shelves are stocked with,
// how many items a shop type shows, its shelf picture, the books of the guild shelves
// and the armour shop's shelf places (re/notes/houses.md#shops).

// Shop houses (2DEvents ids): weapons 1..14, armour 15..28, magic 29..41, alchemists
// 42..53; the guilds' shelves are kept for houses 0x8b.. (139..172).
const (
	FirstArmourShop  = 15
	FirstMagicShop   = 29
	FirstAlchemist   = 42
	NumShopHouses    = 54 // houses 0..53
	FirstGuild       = 0x8b
	NumGuilds        = 34 // 139..172
	ShopSlots        = 12 // items per shelf (g_shopStandard[house * 12 + slot])
	NumSchools       = 12 // guild shelves per guild house: one per magic school
	SpellsPerSchool  = 11
	FirstSpellBook   = 400
	NumShopTypes     = 5    // g_shopItemCount by house type 0..4
	NumShopPictures  = 0x13 // g_shopBackgrounds: house types below 0x13
	NumArmourShelfXs = 8
)

// Exe addresses of the shop tables.
const (
	vaShopRaw        = 0x502700 // the stock records 0x50279a.. 0x502bb0 in one read
	shopRawSize      = 0x4b0
	vaShopCounts     = 0x5029d0 // uchar [type]
	vaShopPictures   = 0x502758 // char *[type]
	vaGuildSpells    = 0x5030c2 // short [house]
	vaArmourShelfX   = 0x4ea9bc // int [8]
	vaStdWeapons     = 0x50279a // {level, kinds[4]} + 10 * house
	vaStdArmour      = 0x502704 // + 10 * (2 * house + row)
	vaStdMagicLevel  = 0x50290e // + 2 * house
	vaStdAlchemyLvl  = 0x5028fa // + 2 * house
	vaSpcWeapons     = 0x5029ce // + 10 * house
	vaSpcArmour      = 0x502938 // + 10 * (2 * house + row)
	vaSpcMagicLevel  = 0x502b42 // + 2 * house
	vaSpcAlchemyLvl  = 0x502b44 // + 2 * house
	numGuildSpellHse = FirstGuild + NumGuilds
)

// Shelf is a stock record: the treasure level and the four item kinds one of which
// rand() % 4 picks for each item (ItemGen_Generate's kinds).
type Shelf struct {
	Level int
	Kinds [4]int
}

// Shops holds the shop tables.
type Shops struct {
	raw []byte // vaShopRaw..
	// Counts is the number of items a shop type shows (by house type 1..4: 6, 8, 12, 12).
	Counts [NumShopTypes]int
	// Pictures are the shelf pictures of the house types below 0x13 (WEPNTABL, ARMORY,
	// MAGSHELF...); "" for none.
	Pictures []string
	// GuildSpells is the number of books a guild house's shelves draw from, by house.
	GuildSpells []int16
	// ArmourShelfX are the centres of the armour shop's eight places.
	ArmourShelfX []int32
}

// ReadShops reads the shop tables from the executable.
//
// mm8: 0x4b9820, 0x4b99e4 (the stock), 0x4bcea6 (the guilds), 0x4bd028 (the pictures),
// 0x4bb328 (the armour shelf)
func ReadShops(im *exe.Image) (*Shops, error) {
	s := &Shops{}
	var err error
	if s.raw, err = im.Read(vaShopRaw, shopRawSize); err != nil {
		return nil, err
	}
	c, err := im.Read(vaShopCounts, NumShopTypes)
	if err != nil {
		return nil, err
	}
	for i, n := range c {
		s.Counts[i] = int(n)
	}
	// The types without a picture point at an empty string in the BSS (0x517a94).
	ptrs, err := im.Int32s(vaShopPictures, NumShopPictures)
	if err != nil {
		return nil, err
	}
	s.Pictures = make([]string, NumShopPictures)
	for i, p := range ptrs {
		s.Pictures[i], _ = im.CString(uint32(p))
	}
	g, err := im.Read(vaGuildSpells, 2*numGuildSpellHse)
	if err != nil {
		return nil, err
	}
	s.GuildSpells = make([]int16, numGuildSpellHse)
	for i := range s.GuildSpells {
		s.GuildSpells[i] = int16(binary.LittleEndian.Uint16(g[2*i:]))
	}
	if s.ArmourShelfX, err = im.Int32s(vaArmourShelfX, NumArmourShelfXs); err != nil {
		return nil, err
	}
	return s, nil
}

// NewShops makes shop tables from a raw image of 0x502700..0x502bb0 (tests).
func NewShops(raw []byte) *Shops {
	s := &Shops{raw: make([]byte, shopRawSize)}
	copy(s.raw, raw)
	return s
}

// short is the int16 at va within the stock records, 0 outside them.
func (s *Shops) short(va uint32) int {
	o := int(va) - vaShopRaw
	if o < 0 || o+2 > len(s.raw) {
		return 0
	}
	return int(int16(binary.LittleEndian.Uint16(s.raw[o:])))
}

// SetShort sets the int16 at va (tests).
func (s *Shops) SetShort(va uint32, v int) {
	if o := int(va) - vaShopRaw; o >= 0 && o+2 <= len(s.raw) {
		binary.LittleEndian.PutUint16(s.raw[o:], uint16(v))
	}
}

func (s *Shops) shelf(va uint32) Shelf {
	sh := Shelf{Level: s.short(va)}
	for i := range sh.Kinds {
		sh.Kinds[i] = s.short(va + 2 + uint32(2*i))
	}
	return sh
}

// StdWeapons, SpcWeapons are a weapon shop's standard and special stock.
//
// mm8: 0x4b9820 (0x50279a + 10 * house), 0x4b99e4 (0x5029ce + 10 * house)
func (s *Shops) StdWeapons(house int) Shelf { return s.shelf(vaStdWeapons + uint32(10*house)) }
func (s *Shops) SpcWeapons(house int) Shelf { return s.shelf(vaSpcWeapons + uint32(10*house)) }

// StdArmour, SpcArmour are an armour shop's stock: row 0 for slots 0..3, row 1 for
// slots 4.. (the two shelves).
//
// mm8: 0x4b9820 (0x502704 + 10 * (2 * house + row)), 0x4b99e4 (0x502938 + ...)
func (s *Shops) StdArmour(house, row int) Shelf {
	return s.shelf(vaStdArmour + uint32(10*(2*house+row)))
}
func (s *Shops) SpcArmour(house, row int) Shelf {
	return s.shelf(vaSpcArmour + uint32(10*(2*house+row)))
}

// MagicLevel, AlchemyLevel are the treasure levels of the magic shops' and the
// alchemists' stock (standard or special). The alchemists' standard levels are read
// from a table whose entries lie three places past the magic shops' (as the original
// reads them).
//
// mm8: 0x4b9820 (0x50290e / 0x5028fa + 2 * house), 0x4b99e4 (0x502b42 / 0x502b44 + 2 * house)
func (s *Shops) MagicLevel(house int, special bool) int {
	if special {
		return s.short(vaSpcMagicLevel + uint32(2*house))
	}
	return s.short(vaStdMagicLevel + uint32(2*house))
}
func (s *Shops) AlchemyLevel(house int, special bool) int {
	if special {
		return s.short(vaSpcAlchemyLvl + uint32(2*house))
	}
	return s.short(vaStdAlchemyLvl + uint32(2*house))
}

// Count is the number of items a shop of house type typ shows (0 for other types).
func (s *Shops) Count(typ int) int {
	if typ < 0 || typ >= NumShopTypes {
		return 0
	}
	return s.Counts[typ]
}

// Picture is the shelf picture of house type typ, "" for none.
func (s *Shops) Picture(typ int) string {
	if typ < 0 || typ >= len(s.Pictures) {
		return ""
	}
	return s.Pictures[typ]
}

// GuildSpellCount is the number of books a guild house's shelves draw from.
func (s *Shops) GuildSpellCount(house int) int {
	if house < 0 || house >= len(s.GuildSpells) {
		return 0
	}
	return int(s.GuildSpells[house])
}
