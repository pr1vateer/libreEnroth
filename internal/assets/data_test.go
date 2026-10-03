package assets_test

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"image"
	"image/color"
	"path"
	"strings"
	"testing"

	"libre-enroth/internal/assets"
	"libre-enroth/internal/assets/assettest"
	"libre-enroth/internal/assets/bitmap"
	"libre-enroth/internal/assets/font"
	"libre-enroth/internal/assets/hwl"
	"libre-enroth/internal/assets/icon"
	"libre-enroth/internal/assets/lod"
	"libre-enroth/internal/assets/palette"
	"libre-enroth/internal/assets/pcx"
	"libre-enroth/internal/assets/sprite"
	"libre-enroth/internal/assets/txt"
)

func openData(t *testing.T) *assets.Data {
	t.Helper()
	d, err := assets.OpenAll(assettest.Dir(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

func TestEntryCounts(t *testing.T) {
	d := openData(t)
	for _, c := range []struct {
		a       *lod.Archive
		version string
		chapter string
		n       int
	}{
		{d.Icons, "MMVI", "icons", 3943},
		{d.Bitmaps, "MMVI", "bitmaps", 1818},
		{d.Sprites, "MMVI", "sprites08", 10688},
		{d.Games, "GameMMVI", "maps", 122},
		{d.LangT, "MMVIII", "language", 194},
		{d.LangD, "MMVIII", "language", 3450},
	} {
		if c.a.Version != c.version || len(c.a.Chapters) != 1 || c.a.Chapters[0].Name != c.chapter || len(c.a.Entries) != c.n {
			t.Errorf("%s: version %q, %d chapter(s) %q, %d entries; want %q %q %d",
				c.a.Path, c.a.Version, len(c.a.Chapters), c.a.Chapters[0].Name, len(c.a.Entries), c.version, c.chapter, c.n)
		}
	}
	if n := len(d.HwlBitmaps.Names()); n != 1526 {
		t.Errorf("d3dbitmap.hwl: %d entries, want 1526", n)
	}
	if n := len(d.HwlSprite.Names()); n != 8823 {
		t.Errorf("d3dsprite.hwl: %d entries, want 8823", n)
	}
}

func TestKnownNames(t *testing.T) {
	d := openData(t)
	for _, c := range []struct {
		a    *lod.Archive
		name string
	}{
		{d.Bitmaps, "alphanum1"}, {d.Bitmaps, "PAL005"}, {d.Sprites, "arrowa0"},
		{d.LangT, "2devents.txt"}, {d.LangT, "ARRUS.FNT"}, {d.Games, "D05.BLV"},
		{d.Icons, "ar_dn_dn"}, {d.LangD, "TITLE.PCX"},
	} {
		if _, ok := c.a.Find(c.name); !ok {
			t.Errorf("%s: %q not found", c.a.Path, c.name)
		}
	}
	if _, _, err := d.LangFile("2DEvents.txt"); err != nil {
		t.Error(err)
	}
}

func TestDecodedSizes(t *testing.T) {
	d := openData(t)

	tex, err := bitmap.Load(d.Bitmaps, "alphanum1")
	if err != nil {
		t.Fatal(err)
	}
	if tex.W != 64 || tex.H != 64 || len(tex.Levels) != 4 || len(tex.Levels[3]) != 8*8 || tex.Palette == nil {
		t.Errorf("alphanum1: %dx%d, %d levels", tex.W, tex.H, len(tex.Levels))
	}

	ic, err := icon.Load(d.Icons, "AR_DN_DN")
	if err != nil {
		t.Fatal(err)
	}
	if ic.W != 18 || ic.H != 17 || len(ic.Levels) != 1 {
		t.Errorf("AR_DN_DN: %dx%d, %d levels", ic.W, ic.H, len(ic.Levels))
	}

	sp, err := sprite.Load(d.Sprites, "ARROWA0")
	if err != nil {
		t.Fatal(err)
	}
	if sp.W != 60 || sp.H != 120 || sp.PaletteID != 338 {
		t.Errorf("ARROWA0: %dx%d pal %d", sp.W, sp.H, sp.PaletteID)
	}
	if _, err := palette.Load(d.Bitmaps, sp.PaletteID); err != nil {
		t.Error(err)
	}

	_, raw, err := d.LangFile("Arrus.fnt")
	if err != nil {
		t.Fatal(err)
	}
	f, err := font.Decode(raw)
	if err != nil {
		t.Fatal(err)
	}
	if f.First != 31 || f.Last != 255 || f.Height != 19 || f.Metrics['A'] != (font.Metrics{Left: 0, Width: 12, Right: 0}) {
		t.Errorf("Arrus.fnt: first %d last %d height %d A %+v", f.First, f.Last, f.Height, f.Metrics['A'])
	}

	img, err := pcx.Load(d.LangD, "title.pcx")
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds() != image.Rect(0, 0, 640, 480) {
		t.Errorf("title.pcx: %v", img.Bounds())
	}

	first := d.HwlBitmaps.Names()[0]
	ht, err := d.HwlBitmaps.Load(first)
	if err != nil {
		t.Fatal(err)
	}
	if ht.W != 32 || ht.H != 32 {
		t.Errorf("hwl %s: %dx%d", first, ht.W, ht.H)
	}

	raw, err = d.Games.Raw("d05.blv")
	if err != nil {
		t.Fatal(err)
	}
	m, err := lod.UnpackMap(raw)
	if err != nil {
		t.Fatal(err)
	}
	if want := binary.LittleEndian.Uint32(raw[12:]); uint32(len(m)) != want {
		t.Errorf("d05.blv: %d bytes, header says %d", len(m), want)
	}
}

func sum(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// SHA-256 of decoded pixels (indices for paletted data, NRGBA otherwise; the font is its
// atlas, the map its unpacked bytes). Captured on 2026-10-03 after viewing the lodtool
// PNGs of the same entries.
var goldenHashes = map[string]string{
	"bitmaps/alphanum1":   "a41cf06d014aceb3f983209fcd8a63c3fb427525ba9db9f87ea4e089ce520e6d",
	"icons/AR_DN_DN":      "c26295fea8096f0b3bb24f7348673648545622a6a17e97d72c6f308803ab84f9",
	"sprites/ARROWA0":     "d492120e42f1fe91c38598cf113b0d3c159e30e891eada81d3893dabb07e1547",
	"EnglishT/Arrus.fnt":  "1b5cd046ab73c883f5156c17dbce13d73a5c89045e669c473cabd6160706be31",
	"EnglishD/title.pcx":  "f22fe3cbae4cf4eba8e8db72a41f05036cdd0d556aa209ae4b1bb24ef329d872",
	"d3dbitmap/alphanum1": "9ae1a6eb847de45582673d87c29add9de2b8df25a7e0368c7ce33c66a5e2df28",
	"games/d05.blv":       "1ed2babcc1196ad3b344b7169e6c89d08caf03410901e963549f474b05a33460",
}

func TestGoldenHashes(t *testing.T) {
	d := openData(t)
	got := map[string]string{}

	tex, err := bitmap.Load(d.Bitmaps, "alphanum1")
	if err != nil {
		t.Fatal(err)
	}
	got["bitmaps/alphanum1"] = sum(tex.Levels[0])

	ic, err := icon.Load(d.Icons, "AR_DN_DN")
	if err != nil {
		t.Fatal(err)
	}
	got["icons/AR_DN_DN"] = sum(ic.Levels[0])

	sp, err := sprite.Load(d.Sprites, "ARROWA0")
	if err != nil {
		t.Fatal(err)
	}
	pal, err := palette.Load(d.Bitmaps, sp.PaletteID)
	if err != nil {
		t.Fatal(err)
	}
	got["sprites/ARROWA0"] = sum(sp.Image(pal).Pix)

	_, raw, err := d.LangFile("Arrus.fnt")
	if err != nil {
		t.Fatal(err)
	}
	f, err := font.Decode(raw)
	if err != nil {
		t.Fatal(err)
	}
	got["EnglishT/Arrus.fnt"] = sum(f.Atlas().Pix)

	img, err := pcx.Load(d.LangD, "title.pcx")
	if err != nil {
		t.Fatal(err)
	}
	got["EnglishD/title.pcx"] = sum(img.(*image.NRGBA).Pix)

	ht, err := d.HwlBitmaps.Load("alphanum1")
	if err != nil {
		t.Fatal(err)
	}
	got["d3dbitmap/alphanum1"] = sum(ht.NRGBA().Pix)

	raw, err = d.Games.Raw("d05.blv")
	if err != nil {
		t.Fatal(err)
	}
	m, err := lod.UnpackMap(raw)
	if err != nil {
		t.Fatal(err)
	}
	got["games/d05.blv"] = sum(m)

	for k, want := range goldenHashes {
		if got[k] != want {
			t.Errorf("%-20s = %q, want %q", k, got[k], want)
		}
	}
}

// unsupportedFiles must fail to decode, with the reason.
var unsupportedFiles = map[string]string{
	// In icons.lod and EnglishT.lod. Not among the 12 .fnt names in MM8-Rel.exe's strings,
	// and its metrics table holds cumulative pixel offsets, not the Font_Load layout.
	"calig.fnt": "unused font in another layout",
}

// TestDecodeEverything decodes every entry of every archive with the decoder lodtool
// would pick.
func TestDecodeEverything(t *testing.T) {
	if testing.Short() {
		t.Skip("-short")
	}
	d := openData(t)

	t.Run("packed", func(t *testing.T) {
		for _, a := range []*lod.Archive{d.Icons, d.Bitmaps, d.LangT, d.LangD} {
			counts := map[string]int{}
			for _, e := range a.Entries {
				raw, err := a.ReadEntry(e)
				if err != nil {
					t.Fatal(err)
				}
				if !lod.IsPacked(raw, int64(len(raw)), e.Name, a.IsMM8()) {
					t.Errorf("%s: %s: not a packed file", a.Path, e.Name)
					continue
				}
				h, data, tail, err := lod.SplitPacked(raw, a.IsMM8())
				if err != nil {
					t.Errorf("%s: %v", a.Path, err)
					continue
				}
				kind, err := decodePacked(e.Name, h, data, tail)
				why, unsupported := unsupportedFiles[strings.ToLower(e.Name)]
				switch {
				case unsupported && err == nil:
					t.Errorf("%s: %s now decodes; drop it from unsupportedFiles (%s)", a.Path, e.Name, why)
				case unsupported:
					kind = "unsupported"
				case err != nil:
					t.Errorf("%s: %s: %v", a.Path, e.Name, err)
				}
				counts[kind]++
			}
			t.Logf("%s: %v", a.Path, counts)
		}
	})

	t.Run("sprites", func(t *testing.T) {
		pals := map[int]color.Palette{}
		for _, e := range d.Sprites.Entries {
			raw, err := d.Sprites.ReadEntry(e)
			if err != nil {
				t.Fatal(err)
			}
			s, err := sprite.Decode(raw)
			if err != nil {
				t.Error(err)
				continue
			}
			pal, ok := pals[s.PaletteID]
			if !ok {
				if pal, err = palette.Load(d.Bitmaps, s.PaletteID); err != nil {
					t.Errorf("%s: %v", e.Name, err)
					continue
				}
				pals[s.PaletteID] = pal
			}
			s.Image(pal)
		}
		t.Logf("%d sprites, %d palettes", len(d.Sprites.Entries), len(pals))
	})

	t.Run("maps", func(t *testing.T) {
		for _, e := range d.Games.Entries {
			raw, err := d.Games.ReadEntry(e)
			if err != nil {
				t.Fatal(err)
			}
			m, err := lod.UnpackMap(raw)
			if err != nil {
				t.Errorf("%s: %v", e.Name, err)
				continue
			}
			if want := binary.LittleEndian.Uint32(raw[12:]); uint32(len(m)) != want {
				t.Errorf("%s: %d bytes, header says %d", e.Name, len(m), want)
			}
		}
	})

	t.Run("hwl", func(t *testing.T) {
		for _, h := range []*hwl.File{d.HwlBitmaps, d.HwlSprite} {
			for _, n := range h.Names() {
				if _, err := h.Load(n); err != nil {
					t.Error(err)
				}
			}
		}
	})
}

// decodePacked decodes a packed-file payload by name and header; it returns the kind.
func decodePacked(name string, h lod.FileHeader, data, tail []byte) (string, error) {
	switch ext := strings.ToLower(path.Ext(name)); {
	case ext == ".pcx":
		_, err := pcx.Decode(data)
		return "pcx", err
	case ext == ".fnt":
		_, err := font.Decode(data)
		return "fnt", err
	case ext == ".txt":
		txt.Parse(data)
		return "txt", nil
	case ext != "":
		return ext, nil // .str .evt .bin .wav: payload only, decoded in later milestones
	case h.W > 0:
		_, err := bitmap.Decode(h, data, tail)
		return "bitmap", err
	default:
		_, err := palette.FromRGB(tail)
		return "palette", err
	}
}
