// Package items is the item logic of the original's itemdata.cpp: an item's value and
// name, and the random item generator (re/notes/items.md).
package items

import (
	"fmt"

	"libre-enroth/internal/assets/tables"
)

// Item is an item in a pack, on the mouse, in a chest or on the ground (0x24 bytes).
type Item struct {
	Number int32 // +0x00 items.txt id, 0 = none
	// Bonus is the standard bonus (stditems number, 1-based); on a potion, its power.
	Bonus    int32  // +0x04
	Strength int32  // +0x08 the standard bonus's strength
	Special  int32  // +0x0c special bonus (spcitems number, 1-based)
	Charges  int32  // +0x10 a wand's charges
	Flags    uint32 // +0x14 Flag*
	// Slot is the equipment slot + 1 while equipped (Player +0x1c04).
	Slot       uint8 // +0x18
	MaxCharges uint8 // +0x19
	Owner      uint8 // +0x1a the lich jar's owner (1-based member)
	Unk1b      uint8 // +0x1b
	Expires    int64 // +0x1c a temporary bonus's end (game time)
}

// Item flags.
const (
	FlagIdentified = 0x1
	FlagBroken     = 0x2
	FlagTempBonus  = 0x8 // a potion's temporary enchantment: the value ignores it
	FlagStolen     = 0x100
	FlagHardened   = 0x200 // cannot break
)

// Item ids the code tests.
const (
	LichJar       = 0x259 // 601: named after its owner
	FirstArtifact = 500
	NumArtifacts  = 0x2b // 500..542: Party +0x7d4 records which were found
	EmptyBottle   = 0xdc // 220: the generator gives potions a power, but not it
	FirstWand     = 0x98 // 152..176: wands (Add item 0x11 gives them charges)
	LastWand      = 0xb0
)

// Rand is the game's rand() (party.Rand).
type Rand interface{ Int() int }

// Def is the item's items.txt row.
func (it *Item) Def(t *tables.Items) *tables.ItemDef { return t.Item(it.Number) }

// Identified, Broken report the flags.
func (it *Item) Identified() bool { return it.Flags&FlagIdentified != 0 }
func (it *Item) Broken() bool     { return it.Flags&FlagBroken != 0 }

// Value is what the item is worth: an artifact, relic or special item, or one with a
// temporary bonus, is worth its base value; a standard bonus adds 100 per point of
// strength; a special bonus adds its value, or multiplies the base by it below 11.
//
// mm8: 0x4550e5 (Item_GetValue)
func (it *Item) Value(t *tables.Items) int32 {
	d := it.Def(t)
	base := d.Value
	if it.Flags&FlagTempBonus != 0 || d.IsRare() {
		return base
	}
	if it.Bonus != 0 {
		return uint32Add(uint32(it.Strength*100), base)
	}
	if it.Special == 0 {
		return base
	}
	v := spc(t, it.Special).Value
	if uint32(v) < 11 {
		return int32(uint32(v) * uint32(base))
	}
	return uint32Add(uint32(v), base)
}

func uint32Add(a uint32, b int32) int32 { return int32(a + uint32(b)) }

// spc returns special bonus n (1-based); out of range, an empty one.
func spc(t *tables.Items, n int32) *tables.SpcBonus {
	if n < 1 || int(n) > len(t.Spc) {
		return &tables.SpcBonus{}
	}
	return &t.Spc[n-1]
}

// std returns standard bonus n (1-based); out of range, an empty one.
func std(t *tables.Items, n int32) *tables.StdBonus {
	if n < 1 || int(n) > len(t.Std) {
		return &tables.StdBonus{}
	}
	return &t.Std[n-1]
}

// prefixSpecials are the special bonuses whose name goes before the item's
// ("Vampiric Sword") rather than after it ("Sword of Ice").
//
// mm8: 0x455156 (Item_FullName)
var prefixSpecials = map[int32]bool{
	0x10: true, 0x27: true, 0x28: true, 0x2d: true, 0x38: true, 0x39: true, 0x3a: true,
	0x3b: true, 0x3c: true, 0x3d: true, 0x3f: true, 0x40: true, 0x43: true, 0x44: true,
}

// Name is what the item is called: its full name once identified, else its
// unidentified name.
//
// mm8: 0x45513c (Item_GetName)
func (it *Item) Name(t *tables.Items, n Namer) string {
	if it.Identified() {
		return it.FullName(t, n)
	}
	return it.Def(t).UnidentifiedName
}

// Namer gives FullName what it needs besides the tables: global.txt strings and the
// party members' names.
type Namer interface {
	Global(i int) string
	// MemberName is the name of the member in 0-based slot i.
	MemberName(i int) string
}

// Global.txt formats of the lich jar ("%s's Jar", and for a name ending in 's').
const (
	txtJar  = 0x28e
	txtJarS = 0x28f
)

// FullName is the identified name: reagents, potions and gold (and the other types the
// code lists) only by their name; the lich jar after its owner (a member 1..4); an
// artifact, relic or special item by its name; otherwise the name with the standard
// bonus after it, or the special bonus before or after it.
//
// mm8: 0x455156 (Item_FullName)
func (it *Item) FullName(t *tables.Items, n Namer) string {
	d := it.Def(t)
	if t := d.EquipType; t >= tables.EquipReagent && t <= tables.EquipPotion || t == tables.EquipGold {
		return d.Name
	}
	if it.Number == LichJar && it.Owner != 0 && it.Owner <= 4 {
		name := n.MemberName(int(it.Owner) - 1)
		f := n.Global(txtJar)
		if len(name) > 0 && name[len(name)-1] == 's' {
			f = n.Global(txtJarS)
		}
		return cfmt(f, name)
	}
	switch {
	case d.IsRare():
		return d.Name
	case it.Bonus != 0:
		return d.Name + " " + std(t, it.Bonus).OfName
	case it.Special == 0:
		return d.Name
	case prefixSpecials[it.Special]:
		return spc(t, it.Special).Name + " " + d.Name
	}
	return d.Name + " " + spc(t, it.Special).Name
}

// cfmt fills a C "%s" format.
func cfmt(f, s string) string {
	out := []byte{}
	for i := 0; i < len(f); i++ {
		if f[i] == '%' && i+1 < len(f) && f[i+1] == 's' {
			out = append(out, s...)
			i++
			continue
		}
		out = append(out, f[i])
	}
	return string(out)
}

// ApplySpecial gives an item of special material its fixed bonus from items.txt.
//
// mm8: 0x455a01
func (it *Item) ApplySpecial(t *tables.Items) {
	d := it.Def(t)
	if d.Material != tables.MaterialSpecial {
		return
	}
	it.Bonus = int32(d.StdBonus)
	it.Special = int32(d.Special)
	it.Strength = int32(d.Strength)
}

// ExpireBonus removes a temporary bonus (FlagTempBonus) whose time is past now.
//
// mm8: 0x456f74 (Item_ExpireBonus)
func (it *Item) ExpireBonus(now int64) {
	if it.Flags&FlagTempBonus != 0 && it.Expires < now {
		it.Bonus, it.Special = 0, 0
		it.Flags &^= FlagTempBonus
	}
}

// New is an item of id as the code makes them from a number: identified when its
// difficulty is 0.
//
// mm8: 0x48dd30 (Party_AddItem), 0x4552ca (ItemGen_Generate: flags = difficulty == 0)
func New(t *tables.Items, id int32) Item {
	it := Item{Number: id}
	if t.Item(id).IDRepair == 0 {
		it.Flags = FlagIdentified
	}
	return it
}

func (it Item) String() string {
	return fmt.Sprintf("item %d (bonus %d/%d, special %d, flags %#x)", it.Number, it.Bonus, it.Strength, it.Special, it.Flags)
}
