// Package icon decodes icons.lod images: the bitmap format without mips, carrying its
// own palette.
package icon

import (
	"fmt"

	"libre-enroth/internal/assets/bitmap"
	"libre-enroth/internal/assets/lod"
)

// Load reads an icon by name. Non-image entries (.pcx, .fnt, .txt) are rejected.
//
// mm8: 0x4113e0 (Texture_Load with the icons LOD, palette mode 1)
func Load(icons *lod.Archive, name string) (*bitmap.Texture, error) {
	t, err := bitmap.Load(icons, name)
	if err != nil {
		return nil, err
	}
	if t.Palette == nil {
		return nil, fmt.Errorf("icon %q: no embedded palette", name)
	}
	return t, nil
}
