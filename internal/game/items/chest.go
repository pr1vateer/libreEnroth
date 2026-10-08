package items

import (
	"encoding/binary"

	"libre-enroth/internal/assets/tables"
)

// Chest is a chest record (0x14cc bytes in the .dlv/.ddm, g_chests 0x602450): its
// type, flags, up to 140 items and the grid over them. Every MM8 chest type has a 9 × 9
// grid (tables.ChestGrid), so only the first 81 cells and items are used.
type Chest struct {
	Type  int16  // +0x0000 dchest.bin record: the picture and grid
	Flags uint16 // +0x0002 Chest*
	// Items are the chest's items; a negative number is a random item of that treasure
	// level, made on the first visit (GenerateChests).
	Items [ChestSlots]Item // +0x0004
	// Grid is a cell per grid position, row-major by the grid width: n > 0 the top-left
	// cell of Items[n-1], -1-c a cell covered by the item whose top-left is c.
	Grid [ChestSlots]int16 // +0x13b4
}

// Chest sizes and flags.
const (
	ChestSlots    = 140
	ChestSize     = 0x14cc
	ChestTrapped  = 0x1 // the next opening sets off the trap unless disarmed
	ChestPlaced   = 0x2 // the items are in the grid (Chest_PlaceItems ran)
	ChestIdentify = 0x4 // placing the items identifies them
)

// ItemSize is the size of an item record.
const ItemSize = 0x24

// DecodeItem reads a 0x24-byte item record.
func DecodeItem(b []byte) Item {
	le := binary.LittleEndian
	return Item{
		Number: int32(le.Uint32(b)), Bonus: int32(le.Uint32(b[4:])), Strength: int32(le.Uint32(b[8:])),
		Special: int32(le.Uint32(b[0xc:])), Charges: int32(le.Uint32(b[0x10:])), Flags: le.Uint32(b[0x14:]),
		Slot: b[0x18], MaxCharges: b[0x19], Owner: b[0x1a], Unk1b: b[0x1b], Expires: int64(le.Uint64(b[0x1c:])),
	}
}

// Encode writes the item as a 0x24-byte record.
func (it *Item) Encode(b []byte) {
	le := binary.LittleEndian
	le.PutUint32(b, uint32(it.Number))
	le.PutUint32(b[4:], uint32(it.Bonus))
	le.PutUint32(b[8:], uint32(it.Strength))
	le.PutUint32(b[0xc:], uint32(it.Special))
	le.PutUint32(b[0x10:], uint32(it.Charges))
	le.PutUint32(b[0x14:], it.Flags)
	b[0x18], b[0x19], b[0x1a], b[0x1b] = it.Slot, it.MaxCharges, it.Owner, it.Unk1b
	le.PutUint64(b[0x1c:], uint64(it.Expires))
}

// DecodeChest reads a 0x14cc-byte chest record.
func DecodeChest(b []byte) Chest {
	var c Chest
	c.Type = int16(binary.LittleEndian.Uint16(b))
	c.Flags = binary.LittleEndian.Uint16(b[2:])
	for i := range c.Items {
		c.Items[i] = DecodeItem(b[4+i*ItemSize:])
	}
	for i := range c.Grid {
		c.Grid[i] = int16(binary.LittleEndian.Uint16(b[0x13b4+2*i:]))
	}
	return c
}

// Encode writes the chest as a 0x14cc-byte record.
func (c *Chest) Encode(b []byte) {
	binary.LittleEndian.PutUint16(b, uint16(c.Type))
	binary.LittleEndian.PutUint16(b[2:], c.Flags)
	for i := range c.Items {
		c.Items[i].Encode(b[4+i*ItemSize:])
	}
	for i := range c.Grid {
		binary.LittleEndian.PutUint16(b[0x13b4+2*i:], uint16(c.Grid[i]))
	}
}

// Fits reports item id fitting with its top-left at cell: inside the grid and over free
// cells.
//
// mm8: 0x41fa59 (Chest_ItemFits)
func (c *Chest) Fits(t *tables.Items, g tables.ChestGrid, cell int, id int32) bool {
	d := t.Item(id)
	w, h := d.W, d.H
	if cell < 0 || g.W < cell%g.W+w || g.H < cell/g.W+h {
		return false
	}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if i := cell + y*g.W + x; i >= ChestSlots || c.Grid[i] != 0 {
				return false
			}
		}
	}
	return true
}

// mark covers item index's cells from cell and puts index + 1 in its top-left one.
func (c *Chest) mark(t *tables.Items, g tables.ChestGrid, cell, index int) {
	d := t.Item(c.Items[index].Number)
	for y := 0; y < d.H; y++ {
		for x := 0; x < d.W; x++ {
			if i := cell + y*g.W + x; i < ChestSlots {
				c.Grid[i] = int16(-1 - cell)
			}
		}
	}
	c.Grid[cell] = int16(index + 1)
}

// PlaceItem puts item index into the grid at cell. An item of special material gets its
// bonus first; a wand without charges gets 10..30, a gold pile without an amount its
// amount (identified).
//
// mm8: 0x41fd30 (Chest_PlaceItem)
func (c *Chest) PlaceItem(t *tables.Items, g tables.ChestGrid, cell, index int, rng Rand) {
	it := &c.Items[index]
	id := it.Number
	it.ApplySpecial(t)
	switch {
	case id > 0x97 && id < 0xb1:
		if it.Charges == 0 {
			it.Charges = int32(rng.Int()%0x15 + 10)
			if it.Charges < 1 {
				it.Charges = 1
			}
			it.MaxCharges = uint8(it.Charges)
		}
	case id >= GoldSmall && id <= GoldLarge && it.Special == 0:
		switch id {
		case GoldMedium:
			it.Special = int32(rng.Int()%0x65 + 100)
		case GoldLarge:
			it.Special = int32(rng.Int()%0x12d + 200)
		default:
			it.Special = int32(rng.Int()%0x33 + 0x32)
		}
		it.Flags |= FlagIdentified
	}
	c.mark(t, g, cell, index)
}

// Gold piles: their amount is in Special.
const (
	GoldSmall  = 0xbb
	GoldMedium = 0xbc
	GoldLarge  = 0xbd
)

// PlaceItems puts the items into the grid the first time the chest opens: the grid's
// cells are shuffled (rand() & 0xff until below the cell count, then the next free
// place), and each of the first w × h items goes to the first cell in that order it
// fits at. With ChestIdentify the placed items are identified; afterwards the chest is
// ChestPlaced.
//
// mm8: 0x41fef0 (Chest_PlaceItems)
func (c *Chest) PlaceItems(t *tables.Items, g tables.ChestGrid, rng Rand) {
	n := g.W * g.H
	var order [0x90]int
	for k := 0; k < n; k++ {
		r := rng.Int() & 0xff
		for r >= n {
			r = rng.Int() & 0xff
		}
		// A place holding 0 counts as free, so the first cell drawn (0) can be taken
		// again; one place is left 0 at the end either way.
		for order[r] != 0 {
			if r++; r == n {
				r = 0
			}
		}
		order[r] = k
	}
	for i := 0; i < n && i < ChestSlots; i++ {
		it := &c.Items[i]
		if it.Number == 0 {
			continue
		}
		for k := 0; k < n; k++ {
			if c.Fits(t, g, order[k], it.Number) {
				c.PlaceItem(t, g, order[k], i, rng)
				if c.Flags&ChestIdentify != 0 {
					it.Flags |= FlagIdentified
				}
				break
			}
		}
	}
	c.Flags = c.Flags&^ChestIdentify | ChestPlaced
}

// FreeSlot is the first empty item of the first w × h, -1 when none is.
//
// mm8: 0x41fb4d (Chest_FreeSlot)
func (c *Chest) FreeSlot(g tables.ChestGrid) int {
	for i := 0; i < g.W*g.H && i < ChestSlots; i++ {
		if c.Items[i].Number == 0 {
			return i
		}
	}
	return -1
}

// Put stores it in the chest at the first cell it fits at (row by row). It reports
// false when no item slot is free or the item fits nowhere (the original then has the
// selected member say so, 0xf, when a slot was free).
//
// mm8: 0x41fb8b (Chest_PutItem with cell -1)
func (c *Chest) Put(t *tables.Items, g tables.ChestGrid, it Item) (ok, full bool) {
	slot := c.FreeSlot(g)
	if slot < 0 {
		return false, false
	}
	for cell := 0; cell < g.W*g.H; cell++ {
		if c.Fits(t, g, cell, it.Number) {
			c.Items[slot] = it
			c.mark(t, g, cell, slot)
			return true, false
		}
	}
	return false, true
}

// Take removes the item whose top-left is cell and returns it (Number 0: none there).
//
// mm8: 0x4209ac (Chest_Click: the index from the grid), 0x420842 (Chest_RemoveItem)
func (c *Chest) Take(t *tables.Items, g tables.ChestGrid, cell int) Item {
	if cell < 0 || cell >= ChestSlots || c.Grid[cell] <= 0 {
		return Item{}
	}
	index := int(c.Grid[cell]) - 1
	it := c.Items[index]
	d := t.Item(it.Number)
	for y := 0; y < d.H; y++ {
		for x := 0; x < d.W; x++ {
			if i := cell + y*g.W + x; i < ChestSlots {
				c.Grid[i] = 0
			}
		}
	}
	c.Items[index] = Item{}
	return it
}

// GenerateArtifact makes it an artifact not found yet (marking it found), or reports
// false when every artifact has been.
//
// mm8: 0x44f074 (Item_GenerateArtifact)
func GenerateArtifact(it *Item, t *tables.Items, rng Rand, found *[NumArtifacts]bool) bool {
	missing := 0
	for _, f := range found {
		if !f {
			missing++
		}
	}
	if missing < 1 {
		return false
	}
	k := rng.Int() % missing
	i := 0
	for ; i < NumArtifacts; i++ {
		if !found[i] {
			if k == 0 {
				break
			}
			k--
		}
	}
	found[i] = true
	it.Flags = 0
	it.Number = int32(FirstArtifact + i)
	it.ApplySpecial(t)
	return true
}

// GoldPile makes it a pile of gold for treasure level 1..6 (another level leaves it
// empty).
func GoldPile(it *Item, level int, rng Rand) {
	*it = Item{}
	var amount int
	switch level {
	case 1:
		amount, it.Number = rng.Int()%0x33+0x32, GoldSmall
	case 2:
		amount, it.Number = rng.Int()%0x65+100, GoldSmall
	case 3:
		amount, it.Number = rng.Int()%0x12d+200, GoldMedium
	case 4:
		amount, it.Number = rng.Int()%0x1f5+500, GoldMedium
	case 5:
		amount, it.Number = rng.Int()%0x3e9+1000, GoldLarge
	case 6:
		amount, it.Number = rng.Int()%0xbb9+2000, GoldLarge
	}
	it.Special = int32(amount)
}

// GenerateChests makes the random items of the chests (negative numbers) on entering a
// map. One rand() % 100 drawn first decides for every random item: below 20 nothing, below
// 60 gold, else an item of a treasure level drawn from the item's range at the map's
// treasure level (7: an artifact, or a forced level-6 item when none is left). Each also
// draws up to rand() % 5 extra items into the chest's next empty slots, each with its own
// rand() % 100 (which the later random items then use).
//
// mm8: 0x44ec9d (Chests_GenerateItems, from 0x45ff25 on every map entry; the original
// walks all 20 chest records whatever the map has)
func GenerateChests(chests []Chest, t *tables.Items, ct *tables.Chests, treasure int, rng Rand, found *[NumArtifacts]bool) {
	pick := rng.Int() % 100
	treasure = min(max(treasure, 0), 6)
	make1 := func(it *Item, level int, identified bool) {
		if pick < 60 {
			GoldPile(it, level, rng)
			if identified {
				it.Flags |= FlagIdentified
			}
			return
		}
		if level >= 1 {
			*it = Generate(t, level, KindAny, false, rng, found)
		}
	}
	for ci := range chests {
		c := &chests[ci]
		for i := range c.Items {
			it := &c.Items[i]
			if it.Number >= 0 {
				continue
			}
			extra := rng.Int() % 5
			n := int(-it.Number)
			if n > len(ct.Treasure) {
				*it = Item{} // outside the table; the data never has one
				continue
			}
			lo, hi := int(ct.Treasure[n-1][treasure][0]), int(ct.Treasure[n-1][treasure][1])
			level := rng.Int()%(hi-lo+1) + lo
			if level >= 7 {
				if !GenerateArtifact(it, t, rng, found) {
					*it = Generate(t, 6, KindAny, true, rng, found)
				}
				continue
			}
			if pick < 20 {
				*it = Item{}
			} else {
				make1(it, level, true)
			}
			next := 0
			for k := 0; k < extra; k++ {
				for j := next; j < ChestSlots; j++ {
					e := &c.Items[j]
					if e.Number != 0 {
						continue
					}
					pick = rng.Int() % 100
					if pick >= 20 {
						make1(e, level, false)
						next = j + 1
					}
					break
				}
			}
		}
	}
}
