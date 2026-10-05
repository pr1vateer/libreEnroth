package items

// The backpack: a 14×9 grid of cells over the player's item slots (Player +0x1810).
// A cell is 0 when free, n > 0 at the top-left cell of item n (1-based slot of
// Player.Items), and -1-c on the other cells of the item whose top-left is cell c.
const (
	GridW     = 14
	GridH     = 9
	GridCells = GridW * GridH
	// PackSlots is how many item slots the backpack uses (0x492bef looks at 126 of
	// the 138).
	PackSlots = 0x7e
	// Slots is the size of a player's item array (Player +0x4a8).
	Slots = 0x8a
)

// Grid is a player's backpack cells.
type Grid [GridCells]int32

// Fits reports a w×h item fitting at cell: inside the grid and over free cells.
//
// mm8: 0x492b25
func (g *Grid) Fits(cell, w, h int) bool {
	if cell < 0 || cell%GridW+w >= GridW+1 || cell/GridW+h >= GridH+1 {
		return false
	}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if g[cell+y*GridW+x] != 0 {
				return false
			}
		}
	}
	return true
}

// Free returns the first cell a w×h item fits at, going down each column from the
// left, or -1.
//
// mm8: 0x492ddb, 0x492e59 (the cell -1 search)
func (g *Grid) Free(w, h int) int {
	for x := 0; x < GridW; x++ {
		for c := x; c < GridCells; c += GridW {
			if g.Fits(c, w, h) {
				return c
			}
		}
	}
	return -1
}

// Place marks a w×h item n (1-based slot) at cell.
//
// mm8: 0x492c09, 0x492ecf, 0x492fad
func (g *Grid) Place(cell, w, h int, n int32) {
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			g[cell+y*GridW+x] = int32(-1 - cell)
		}
	}
	g[cell] = n
}

// Clear frees the w×h cells from cell.
//
// mm8: 0x49305c
func (g *Grid) Clear(cell, w, h int) {
	for y := 0; y < h && cell+y*GridW < GridCells; y++ {
		for x := 0; x < w && cell+y*GridW+x < GridCells; x++ {
			g[cell+y*GridW+x] = 0
		}
	}
}
