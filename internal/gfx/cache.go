package gfx

import (
	"errors"
	"fmt"
	"image/color"
	"strings"

	"libre-enroth/internal/assets"
	"libre-enroth/internal/assets/bitmap"
	"libre-enroth/internal/assets/font"
	"libre-enroth/internal/assets/icon"
	"libre-enroth/internal/assets/lod"
	"libre-enroth/internal/assets/pcx"
)

// Cache loads UI images and fonts by name and keeps them.
type Cache struct {
	d       *assets.Data
	sprites map[string]*Sprite
	fonts   map[string]*font.Font
	fontPal color.Palette
}

// NewCache returns an empty cache over d.
func NewCache(d *assets.Data) *Cache {
	return &Cache{d: d, sprites: map[string]*Sprite{}, fonts: map[string]*font.Font{}}
}

// Data returns the archives the cache reads from.
func (c *Cache) Data() *assets.Data { return c.d }

// Icon loads an icon. With fromLangD it is looked up in EnglishD.lod first (localised
// button art) and then in icons.lod, otherwise only in icons.lod.
//
// mm8: 0x411278 (TexLod_LoadTexture: fromLangD -> Texture_Load(g_langDLod), then this LOD)
func (c *Cache) Icon(name string, fromLangD bool) (*Sprite, error) {
	key := "icon:" + strings.ToLower(name)
	if fromLangD {
		key = "iconD:" + strings.ToLower(name)
	}
	if s, ok := c.sprites[key]; ok {
		return s, nil
	}
	var t *bitmap.Texture
	var err error
	if fromLangD {
		t, err = icon.Load(c.d.LangD, name)
	}
	if !fromLangD || errors.Is(err, lod.ErrNotFound) {
		t, err = icon.Load(c.d.Icons, name)
	}
	if err != nil {
		return nil, err
	}
	s, err := FromTexture(t)
	if err != nil {
		return nil, err
	}
	c.sprites[key] = s
	return s, nil
}

// MustIcon is Icon for names the UI layout hard-codes; a missing one is a broken install.
func (c *Cache) MustIcon(name string, fromLangD bool) *Sprite {
	s, err := c.Icon(name, fromLangD)
	if err != nil {
		panic(fmt.Sprintf("gfx: %v", err))
	}
	return s
}

// Pcx loads a 24-bit PCX image, from the language LODs (EnglishD, then EnglishT) when
// fromLang is set, else from icons.lod.
//
// mm8: 0x410bf8 (Pcx_Load)
func (c *Cache) Pcx(name string, fromLang bool) (*Sprite, error) {
	key := "pcx:" + strings.ToLower(name)
	if s, ok := c.sprites[key]; ok {
		return s, nil
	}
	var raw []byte
	var err error
	if fromLang {
		_, raw, err = c.d.LangFile(name)
	} else {
		_, raw, _, err = c.d.Icons.ReadPacked(name)
	}
	if err != nil {
		return nil, err
	}
	img, err := pcx.Decode(raw)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	s := FromImage(img)
	c.sprites[key] = s
	return s, nil
}

// Font loads a .fnt from the language LODs.
//
// mm8: 0x44ad1d (Font_Load)
func (c *Cache) Font(name string) (*font.Font, error) {
	key := strings.ToLower(name)
	if f, ok := c.fonts[key]; ok {
		return f, nil
	}
	_, raw, err := c.d.LangFile(name)
	if err != nil {
		return nil, err
	}
	f, err := font.Decode(raw)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	c.fonts[key] = f
	return f, nil
}

// FontPalette is the palette of the FONTPAL icon, which every Font_Load call binds: text
// drawn without an explicit colour maps glyph values through it.
//
// mm8: 0x44ad1d (Font_Load(name, "FONTPAL", 0))
func (c *Cache) FontPalette() (color.Palette, error) {
	if c.fontPal == nil {
		t, err := icon.Load(c.d.Icons, "FONTPAL")
		if err != nil {
			return nil, err
		}
		c.fontPal = t.Palette
	}
	return c.fontPal, nil
}

// Text reads a text file from the language LODs.
func (c *Cache) Text(name string) ([]byte, error) {
	_, raw, err := c.d.LangFile(name)
	return raw, err
}
