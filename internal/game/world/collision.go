package world

import (
	"libre-enroth/internal/assets/desc"
	"libre-enroth/internal/game/physics"
)

// collisionDecorations turns the level decorations into the cylinders the party's
// sweep tests: the descriptor's radius and height at the decoration's position, solid
// unless the level hides it (flag 0x20) or the descriptor lets things through (flag 1).
// at returns a decoration's level flags and position.
//
// mm8: 0x46e2dc, 0x46e493 (the tests on g_levelDecorations and g_decList)
func collisionDecorations(decs desc.DecList, idx []int, n int, at func(i int) (uint16, [3]int32)) []physics.Decoration {
	out := make([]physics.Decoration, n)
	for i := range n {
		flags, pos := at(i)
		di := idx[i]
		if di < 0 || di >= len(decs) {
			continue
		}
		d := &decs[di]
		out[i] = physics.Decoration{
			Cylinder: physics.Cylinder{X: pos[0], Y: pos[1], Z: pos[2], Radius: int32(d.Radius), Height: int32(d.Height)},
			Solid:    flags&decHidden == 0 && d.Flags&desc.DecMoveThrough == 0,
		}
	}
	return out
}
