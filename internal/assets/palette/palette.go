// Package palette loads the 256-colour palettes (pal%03i entries of bitmaps.lod) that
// textures and sprites refer to by id.
package palette

import (
	"fmt"
	"image/color"

	"libre-enroth/internal/assets/lod"
)

// Size is the size of an on-disk RGB palette.
const Size = 0x300

// Name returns the bitmaps.lod entry name of palette id.
func Name(id int) string { return fmt.Sprintf("pal%03d", id) }

// FromRGB converts a 0x300-byte RGB triplet table into an opaque colour palette.
func FromRGB(b []byte) (color.Palette, error) {
	if len(b) < Size {
		return nil, fmt.Errorf("palette: %d bytes, need %d", len(b), Size)
	}
	p := make(color.Palette, 256)
	for i := range p {
		p[i] = color.NRGBA{b[3*i], b[3*i+1], b[3*i+2], 0xff}
	}
	return p, nil
}

// Load reads palette id from bitmaps.lod. The entry is a packed file with zero-sized
// payload followed by the RGB table.
//
// The game additionally boosts saturation by x1.1 in HSV when it builds its palette
// (Palette_Load); that belongs to the renderer and is not applied here.
//
// mm8: 0x48b844 (Palette_Load)
func Load(bitmaps *lod.Archive, id int) (color.Palette, error) {
	_, _, tail, err := bitmaps.ReadPacked(Name(id))
	if err != nil {
		return nil, err
	}
	p, err := FromRGB(tail)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", Name(id), err)
	}
	return p, nil
}
