// Command lodtool lists and extracts entries of the original game's LOD and HWL archives.
//
//	lodtool ls  <archive>
//	lodtool x   <archive> <glob> [-o dir]          decompressed payloads
//	lodtool png <archive> <glob> [-o dir] [-mips]  decoded images
//	lodtool vid ls  <file.vid>                     container entries (+ Smacker headers)
//	lodtool vid png <file.vid> <name> [frame] [-o dir]  one decoded frame (default 0)
//
// Globs use path.Match syntax and ignore case. The default output directory is out/.
package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path"
	"path/filepath"
	"strings"

	"libre-enroth/internal/assets/bitmap"
	"libre-enroth/internal/assets/font"
	"libre-enroth/internal/assets/hwl"
	"libre-enroth/internal/assets/lod"
	"libre-enroth/internal/assets/palette"
	"libre-enroth/internal/assets/pcx"
	"libre-enroth/internal/assets/sprite"
)

type options struct {
	outDir string
	mips   bool
	args   []string
}

func usage() {
	fmt.Fprintln(os.Stderr, `usage:
  lodtool ls  <archive>
  lodtool x   <archive> <glob> [-o dir]
  lodtool png <archive> <glob> [-o dir] [-mips]
  lodtool vid ls  <file.vid>
  lodtool vid png <file.vid> <name> [frame] [-o dir]`)
	os.Exit(2)
}

// parseArgs accepts flags anywhere on the command line.
func parseArgs(args []string) options {
	o := options{outDir: "out"}
	for i := 0; i < len(args); i++ {
		switch a := args[i]; a {
		case "-o":
			if i+1 >= len(args) {
				usage()
			}
			o.outDir = args[i+1]
			i++
		case "-mips":
			o.mips = true
		default:
			if strings.HasPrefix(a, "-") {
				usage()
			}
			o.args = append(o.args, a)
		}
	}
	return o
}

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	cmd, o := os.Args[1], parseArgs(os.Args[2:])
	var err error
	switch {
	case cmd == "ls" && len(o.args) == 1:
		err = ls(o.args[0])
	case cmd == "x" && len(o.args) == 2:
		err = extract(o)
	case cmd == "png" && len(o.args) == 2:
		err = toPNG(o)
	case cmd == "vid" && len(o.args) == 2 && o.args[0] == "ls":
		err = vidList(o.args[1])
	case cmd == "vid" && (len(o.args) == 3 || len(o.args) == 4) && o.args[0] == "png":
		err = vidPNG(o)
	default:
		usage()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "lodtool:", err)
		os.Exit(1)
	}
}

func isHwl(p string) bool { return strings.EqualFold(filepath.Ext(p), ".hwl") }

func match(glob, name string) bool {
	ok, err := path.Match(strings.ToLower(glob), strings.ToLower(name))
	return err == nil && ok
}

func ls(p string) error {
	if isHwl(p) {
		h, err := hwl.Open(p)
		if err != nil {
			return err
		}
		defer h.Close()
		for _, n := range h.Names() {
			hd, err := h.Header(n)
			if err != nil {
				return err
			}
			kind := "raw"
			if hd.Packed != 0 {
				kind = "zlib"
			}
			fmt.Printf("%-20s %9d %-6s %dx%d (orig %dx%d)\n", n, hd.Packed, kind, hd.W, hd.H, hd.OrigW, hd.OrigH)
		}
		return nil
	}
	a, err := lod.Open(p)
	if err != nil {
		return err
	}
	defer a.Close()
	fmt.Printf("# %s  version %q  %q  %d chapter(s)\n", a.Path, a.Version, a.Description, len(a.Chapters))
	for _, e := range a.Entries {
		head, err := a.ReadHead(e, lod.FileHeaderSizeMM8)
		if err != nil {
			return err
		}
		fmt.Printf("%-24s %9d %s\n", e.Name, e.Size, describe(a, e, head))
	}
	return nil
}

// describe summarises an entry from its first bytes.
func describe(a *lod.Archive, e lod.Entry, head []byte) string {
	switch {
	case lod.IsMap(head):
		return fmt.Sprintf("mvii map packed %d unpacked %d",
			binary.LittleEndian.Uint32(head[8:]), binary.LittleEndian.Uint32(head[12:]))
	case strings.EqualFold(e.Chapter, "sprites08"):
		h, err := sprite.ParseHeader(head)
		if err != nil {
			return "sprite (bad: " + err.Error() + ")"
		}
		pk := "raw"
		if h.UnpackedSize != 0 {
			pk = fmt.Sprintf("zlib->%d", h.UnpackedSize)
		}
		return fmt.Sprintf("%-14s %dx%d pal %d sprite", pk, h.W, h.H, h.PaletteID)
	case lod.IsPacked(head, e.Size, e.Name, a.IsMM8()):
		h, _ := lod.ParseFileHeader(head, a.IsMM8())
		pk := "raw"
		if h.UnpackedSize != 0 {
			pk = fmt.Sprintf("zlib->%d", h.UnpackedSize)
		}
		if h.W > 0 {
			return fmt.Sprintf("%-14s %dx%d pal %d flags %#x", pk, h.W, h.H, h.PaletteID, h.Flags)
		}
		return pk
	}
	return "raw"
}

// payload returns the decompressed content of an entry: map blobs and packed files are
// unwrapped, everything else (sprites included) is returned as stored.
func payload(a *lod.Archive, e lod.Entry) ([]byte, error) {
	raw, err := a.ReadEntry(e)
	if err != nil {
		return nil, err
	}
	switch {
	case lod.IsMap(raw):
		return lod.UnpackMap(raw)
	case strings.EqualFold(e.Chapter, "sprites08"):
		return raw, nil
	case lod.IsPacked(raw, int64(len(raw)), e.Name, a.IsMM8()):
		_, data, _, err := lod.SplitPacked(raw, a.IsMM8())
		return data, err
	}
	return raw, nil
}

func extract(o options) error {
	p, glob := o.args[0], o.args[1]
	if err := os.MkdirAll(o.outDir, 0o755); err != nil {
		return err
	}
	n := 0
	if isHwl(p) {
		h, err := hwl.Open(p)
		if err != nil {
			return err
		}
		defer h.Close()
		for _, name := range h.Names() {
			if !match(glob, name) {
				continue
			}
			t, err := h.Load(name)
			if err != nil {
				return err
			}
			b := make([]byte, 2*len(t.Pix))
			for i, v := range t.Pix {
				b[2*i], b[2*i+1] = byte(v), byte(v>>8)
			}
			if err := write(o.outDir, name+".argb1555", b); err != nil {
				return err
			}
			n++
		}
	} else {
		a, err := lod.Open(p)
		if err != nil {
			return err
		}
		defer a.Close()
		for _, e := range a.Entries {
			if !match(glob, e.Name) {
				continue
			}
			b, err := payload(a, e)
			if err != nil {
				return fmt.Errorf("%s: %w", e.Name, err)
			}
			if err := write(o.outDir, e.Name, b); err != nil {
				return err
			}
			n++
		}
	}
	fmt.Printf("%d file(s) -> %s\n", n, o.outDir)
	return nil
}

func write(dir, name string, b []byte) error {
	return os.WriteFile(filepath.Join(dir, filepath.Base(name)), b, 0o644)
}

func writePNG(dir, name string, img image.Image) error {
	f, err := os.Create(filepath.Join(dir, filepath.Base(name)+".png"))
	if err != nil {
		return err
	}
	if err := png.Encode(f, img); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

var errSkip = errors.New("not an image")

func toPNG(o options) error {
	p, glob := o.args[0], o.args[1]
	if err := os.MkdirAll(o.outDir, 0o755); err != nil {
		return err
	}
	n, skipped, failed := 0, 0, 0
	if isHwl(p) {
		h, err := hwl.Open(p)
		if err != nil {
			return err
		}
		defer h.Close()
		for _, name := range h.Names() {
			if !match(glob, name) {
				continue
			}
			t, err := h.Load(name)
			if err != nil {
				return err
			}
			if err := writePNG(o.outDir, name, t.NRGBA()); err != nil {
				return err
			}
			n++
		}
		fmt.Printf("%d png(s) -> %s\n", n, o.outDir)
		return nil
	}

	a, err := lod.Open(p)
	if err != nil {
		return err
	}
	defer a.Close()
	var bitmaps *lod.Archive // for sprite palettes, opened on demand
	defer func() {
		if bitmaps != nil {
			bitmaps.Close()
		}
	}()
	pals := map[int]color.Palette{}
	spritePal := func(id int) (color.Palette, error) {
		if pal, ok := pals[id]; ok {
			return pal, nil
		}
		if bitmaps == nil {
			var err error
			if bitmaps, err = lod.Open(filepath.Join(filepath.Dir(p), "bitmaps.lod")); err != nil {
				return nil, fmt.Errorf("sprite palettes: %w", err)
			}
		}
		pal, err := palette.Load(bitmaps, id)
		pals[id] = pal
		return pal, err
	}

	for _, e := range a.Entries {
		if !match(glob, e.Name) {
			continue
		}
		imgs, err := decodeImages(a, e, o.mips, spritePal)
		if errors.Is(err, errSkip) {
			skipped++
			continue
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "lodtool: %s: %v\n", e.Name, err)
			failed++
			continue
		}
		for suffix, img := range imgs {
			if err := writePNG(o.outDir, e.Name+suffix, img); err != nil {
				return err
			}
			n++
		}
	}
	fmt.Printf("%d png(s) -> %s (%d non-image entries skipped)\n", n, o.outDir, skipped)
	if failed > 0 {
		return fmt.Errorf("%d entries failed to decode", failed)
	}
	return nil
}

// decodeImages picks a decoder from the chapter name and extension. The map key is a
// file-name suffix ("" for the main image, "_mip1".. with -mips).
func decodeImages(a *lod.Archive, e lod.Entry, mips bool, spritePal func(int) (color.Palette, error)) (map[string]image.Image, error) {
	ext := strings.ToLower(path.Ext(e.Name))
	raw, err := a.ReadEntry(e)
	if err != nil {
		return nil, err
	}
	one := func(img image.Image) map[string]image.Image { return map[string]image.Image{"": img} }

	if strings.EqualFold(e.Chapter, "sprites08") {
		s, err := sprite.Decode(raw)
		if err != nil {
			return nil, err
		}
		pal, err := spritePal(s.PaletteID)
		if err != nil {
			return nil, err
		}
		return one(s.Image(pal)), nil
	}
	if !lod.IsPacked(raw, int64(len(raw)), e.Name, a.IsMM8()) {
		return nil, errSkip
	}
	h, data, tail, err := lod.SplitPacked(raw, a.IsMM8())
	if err != nil {
		return nil, err
	}
	switch {
	case ext == ".pcx":
		img, err := pcx.Decode(data)
		if err != nil {
			return nil, err
		}
		return one(img), nil
	case ext == ".fnt":
		f, err := font.Decode(data)
		if err != nil {
			return nil, err
		}
		return one(f.Atlas()), nil
	case ext == "" && h.W > 0:
		t, err := bitmap.Decode(h, data, tail)
		if err != nil {
			return nil, err
		}
		out := map[string]image.Image{"": t.Image(0)}
		if mips {
			for i := 1; i < len(t.Levels); i++ {
				out[fmt.Sprintf("_mip%d", i)] = t.Image(i)
			}
		}
		return out, nil
	case ext == "" && len(tail) >= palette.Size:
		// pal%03i: show the palette as a 16x16 swatch, 8 px per colour.
		pal, err := palette.FromRGB(tail)
		if err != nil {
			return nil, err
		}
		img := image.NewPaletted(image.Rect(0, 0, 128, 128), pal)
		for y := 0; y < 128; y++ {
			for x := 0; x < 128; x++ {
				img.Pix[y*img.Stride+x] = uint8(y/8*16 + x/8)
			}
		}
		return one(img), nil
	}
	return nil, errSkip
}
