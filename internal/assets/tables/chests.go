package tables

import (
	"encoding/binary"
	"fmt"

	"libre-enroth/internal/assets/exe"
)

// Chest tables: the grid of each chest type compiled into MM8-Rel.exe, the treasure
// levels of the random items, and dchest.bin's pictures (re/notes/inventory.md#chests).

// ChestGrid is a chest type's item grid: where its top-left cell is on the screen and
// its size in cells.
type ChestGrid struct{ X, Y, W, H int }

// Chests are the chest tables.
type Chests struct {
	// Grid per chest type (0x4f5ab4 x, 0x4f5ad4 y, 0x4f5af4 width, 0x4f5b14 height;
	// every MM8 chest is 9 × 9).
	Grid [NumChestTypes]ChestGrid
	// Treasure is the treasure level range {min, max} of a random item -1..-7
	// ([n-1]) in a map of treasure level 0..6 (0x4f9c7e).
	Treasure [7][7][2]uint8
	// Pictures are dchest.bin's picture numbers ("chest%02d") per chest type.
	Pictures []int16
}

// NumChestTypes is the size of the grid tables.
const NumChestTypes = 8

const (
	vaChestGridX = 0x4f5ab4
	vaChestGridY = 0x4f5ad4
	vaChestGridW = 0x4f5af4
	vaChestGridH = 0x4f5b14
	vaTreasure   = 0x4f9c7e
)

// ReadChests reads the chest grids and the treasure levels from the executable and the
// pictures from dchest.bin.
//
// mm8: 0x4205af (Chest_Draw), 0x41fa59 (Chest_ItemFits), 0x44ec9d
// (Chests_GenerateItems), dchest.bin (g_chestDescs 0x61c44c)
func ReadChests(im *exe.Image, dchest []byte) (*Chests, error) {
	c := &Chests{}
	var v [4][]int32
	for i, va := range []uint32{vaChestGridX, vaChestGridY, vaChestGridW, vaChestGridH} {
		var err error
		if v[i], err = im.Int32s(va, NumChestTypes); err != nil {
			return nil, err
		}
	}
	for t := range c.Grid {
		c.Grid[t] = ChestGrid{int(v[0][t]), int(v[1][t]), int(v[2][t]), int(v[3][t])}
	}
	// g_treasureLevels is read as pairs from index n * 7 + level, n = 1..7.
	b, err := im.Read(vaTreasure, 2*8*7)
	if err != nil {
		return nil, err
	}
	for n := range c.Treasure {
		for l := range c.Treasure[n] {
			i := 2 * ((n+1)*7 + l)
			c.Treasure[n][l] = [2]uint8{b[i], b[i+1]}
		}
	}
	if c.Pictures, err = ParseDChest(dchest); err != nil {
		return nil, err
	}
	return c, nil
}

// ParseDChest reads dchest.bin: a u32 count, then 0x24-byte records (name[32], width,
// height, i16 picture at +0x22). Only the pictures are used: the grid sizes come from
// the executable.
func ParseDChest(b []byte) ([]int16, error) {
	if len(b) < 4 {
		return nil, fmt.Errorf("dchest.bin: %d bytes", len(b))
	}
	n := int(binary.LittleEndian.Uint32(b))
	if n < 0 || 4+n*0x24 > len(b) {
		return nil, fmt.Errorf("dchest.bin: %d records in %d bytes", n, len(b))
	}
	out := make([]int16, n)
	for i := range out {
		out[i] = int16(binary.LittleEndian.Uint16(b[4+i*0x24+0x22:]))
	}
	return out, nil
}

// GridOf is chest type t's grid (type 0's for an unknown type).
func (c *Chests) GridOf(t int) ChestGrid {
	if t < 0 || t >= NumChestTypes {
		t = 0
	}
	return c.Grid[t]
}

// Picture is chest type t's picture number, 0 when unknown.
func (c *Chests) Picture(t int) int {
	if t < 0 || t >= len(c.Pictures) {
		return 0
	}
	return int(c.Pictures[t])
}
