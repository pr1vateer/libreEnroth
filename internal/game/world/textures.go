package world

import (
	"image"
	"image/color"
	"strings"

	"libre-enroth/internal/assets"
	"libre-enroth/internal/assets/bitmap"
	"libre-enroth/internal/assets/hwl"
	"libre-enroth/internal/assets/palette"
	"libre-enroth/internal/assets/sprite"
	"libre-enroth/internal/render"
)

// Tex is a texture with the size of the original art it stands for: texture
// coordinates in the maps are in texels of the bitmaps.lod original, and sprites are
// placed by their full frame although the hwl stores only the opaque crop.
type Tex struct {
	*render.Texture
	OrigW, OrigH int
	Crop         image.Rectangle // sprites: the part of the OrigW x OrigH frame Texture shows
}

// TextureCache loads textures the way the Direct3D renderer does: d3dbitmap.hwl and
// d3dsprite.hwl first, then bitmaps.lod / sprites.lod with their pal%03d palettes;
// sprite frames with a palette of their own come from sprites.lod (SpritePal). Missing
// names are remembered as nil.
type TextureCache struct {
	d        *assets.Data
	bitmaps  map[string]*Tex
	sprites  map[string]*Tex
	palSpr   map[palKey]*Tex
	palettes map[int]color.Palette
}

type palKey struct {
	name string
	pal  int
}

// NewTextureCache returns an empty cache.
func NewTextureCache(d *assets.Data) *TextureCache {
	return &TextureCache{d: d, bitmaps: map[string]*Tex{}, sprites: map[string]*Tex{}, palSpr: map[palKey]*Tex{}, palettes: map[int]color.Palette{}}
}

// Bitmap loads a wall/terrain/sky texture. Names starting with "wtrdr" (shoreline
// tiles) use their "h" variant with an alpha channel, as the D3D loader does.
//
// mm8: 0x4113e0 (Texture_Load, D3D branch: "wtrdr*" -> "h" + name; hwl lookup)
func (c *TextureCache) Bitmap(name string) *Tex {
	key := strings.ToLower(name)
	if t, ok := c.bitmaps[key]; ok {
		return t
	}
	var t *Tex
	if strings.HasPrefix(key, "wtrdr") {
		t = c.hwlBitmap("h" + name)
	}
	if t == nil {
		t = c.hwlBitmap(name)
	}
	if t == nil {
		t = c.lodBitmap(name)
	}
	c.bitmaps[key] = t
	return t
}

func (c *TextureCache) hwlBitmap(name string) *Tex {
	if c.d.HwlBitmaps == nil {
		return nil
	}
	h, err := c.d.HwlBitmaps.Load(name)
	if err != nil {
		return nil
	}
	return &Tex{Texture: render.FromARGB1555(h.W, h.H, h.Pix), OrigW: h.OrigW, OrigH: h.OrigH, Crop: image.Rect(0, 0, h.OrigW, h.OrigH)}
}

func (c *TextureCache) palette(id int) color.Palette {
	if p, ok := c.palettes[id]; ok {
		return p
	}
	p, err := palette.Load(c.d.Bitmaps, id)
	if err != nil {
		p = nil
	}
	c.palettes[id] = p
	return p
}

func (c *TextureCache) lodBitmap(name string) *Tex {
	t, err := bitmap.Load(c.d.Bitmaps, name)
	if err != nil {
		return nil
	}
	pal := c.palette(t.PaletteID)
	if pal == nil {
		pal = t.Palette
	}
	if pal == nil {
		return nil
	}
	pix := make([]uint32, t.W*t.H)
	for i, ix := range t.Levels[0] {
		r, g, b, _ := pal[ix].RGBA()
		pix[i] = r>>8 | (g>>8)<<8 | (b>>8)<<16 | 0xff000000
	}
	return &Tex{Texture: render.FromRGBA(t.W, t.H, pix), OrigW: t.W, OrigH: t.H, Crop: image.Rect(0, 0, t.W, t.H)}
}

// Sprite loads a billboard sprite.
//
// mm8: 0x4acd0f (SpriteList_Load; the D3D path reads d3dsprite.hwl)
func (c *TextureCache) Sprite(name string) *Tex {
	key := strings.ToLower(name)
	if t, ok := c.sprites[key]; ok {
		return t
	}
	t := c.hwlSprite(name)
	if t == nil {
		t = c.lodSprite(name)
	}
	c.sprites[key] = t
	return t
}

// SpritePal loads a billboard sprite for a sprite frame with palette pal (pal%03d), the
// way the software renderer does: a frame whose palette is not the sprite's own (the
// monster variants A/B/C share sprites and differ by palette) is drawn from sprites.lod
// with that palette; any other frame uses Sprite (the hwl sprite when there is one).
// pal 0 is the sprite's own palette.
//
// The Direct3D renderer instead draws the hwl sprite (stored once, in the base colours)
// multiplied by the dmonlist tint (Screen_DrawBillboardD3D 0x4a42b7), which casts the
// whole monster green, blue or magenta; libre-enroth keeps the palettes.
//
// mm8: 0x44beaa (SFT_LoadSprites: Palette_Load(frame.palette)), 0x4acd0f
func (c *TextureCache) SpritePal(name string, pal int) *Tex {
	if pal == 0 {
		return c.Sprite(name)
	}
	k := palKey{strings.ToLower(name), pal}
	if t, ok := c.palSpr[k]; ok {
		return t
	}
	var t *Tex
	if s, err := sprite.Load(c.d.Sprites, name); err != nil || s.PaletteID == pal {
		t = c.Sprite(name)
	} else {
		t = c.spriteWithPal(s, pal)
	}
	c.palSpr[k] = t
	return t
}

func (c *TextureCache) hwlSprite(name string) *Tex {
	if c.d.HwlSprite == nil {
		return nil
	}
	h, err := c.d.HwlSprite.Load(name)
	if err != nil {
		return nil // not in the hwl: fall back to sprites.lod
	}
	return spriteTex(h)
}

func spriteTex(h *hwl.Texture) *Tex {
	crop := h.Crop
	if crop.Empty() {
		crop = image.Rect(0, 0, h.OrigW, h.OrigH)
	}
	return &Tex{Texture: render.FromARGB1555(h.W, h.H, h.Pix), OrigW: h.OrigW, OrigH: h.OrigH, Crop: crop}
}

func (c *TextureCache) lodSprite(name string) *Tex {
	s, err := sprite.Load(c.d.Sprites, name)
	if err != nil {
		return nil
	}
	return c.spriteWithPal(s, s.PaletteID)
}

// spriteWithPal converts sprites.lod sprite s with palette palID.
func (c *TextureCache) spriteWithPal(s *sprite.Sprite, palID int) *Tex {
	pal := c.palette(palID)
	if pal == nil {
		return nil
	}
	img := s.Image(pal)
	pix := make([]uint32, s.W*s.H)
	for i, ix := range img.Pix {
		if ix == 0 {
			continue
		}
		r, g, b, _ := pal[ix].RGBA()
		pix[i] = r>>8 | (g>>8)<<8 | (b>>8)<<16 | 0xff000000
	}
	return &Tex{Texture: render.FromRGBA(s.W, s.H, pix), OrigW: s.W, OrigH: s.H, Crop: image.Rect(0, 0, s.W, s.H)}
}
