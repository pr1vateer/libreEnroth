// Package assets opens the original game's data files. The subpackages decode the
// individual formats; see re/notes/formats.md for the layouts and their sources.
package assets

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"libre-enroth/internal/assets/exe"
	"libre-enroth/internal/assets/hwl"
	"libre-enroth/internal/assets/lod"
)

// DataDir returns the game install directory: dir if set (the -data flag), else
// $MM8_DATA. There is no fallback; with neither set it returns an error.
func DataDir(dir string) (string, error) {
	if dir != "" {
		return dir, nil
	}
	if d := os.Getenv("MM8_DATA"); d != "" {
		return d, nil
	}
	return "", errors.New("no game directory: pass -data or set MM8_DATA")
}

// Data holds the archives the game opens at startup.
type Data struct {
	Dir                   string
	Icons, Bitmaps        *lod.Archive
	Sprites, Games        *lod.Archive
	LangT, LangD          *lod.Archive // EnglishT.lod, EnglishD.lod
	HwlBitmaps, HwlSprite *hwl.File    // d3dbitmap.hwl, d3dsprite.hwl
	Exe                   *exe.Image   // MM8-Rel.exe, for tables compiled into the game
}

// OpenAll opens every archive under dir/Data and dir/MM8-Rel.exe. On error, files
// already opened are closed.
func OpenAll(dir string) (*Data, error) {
	d := &Data{Dir: dir}
	var err error
	openLod := func(dst **lod.Archive, name string) {
		if err == nil {
			*dst, err = lod.Open(filepath.Join(dir, "Data", name))
		}
	}
	openHwl := func(dst **hwl.File, name string) {
		if err == nil {
			*dst, err = hwl.Open(filepath.Join(dir, "Data", name))
		}
	}
	openLod(&d.Icons, "icons.lod")
	openLod(&d.Bitmaps, "bitmaps.lod")
	openLod(&d.Sprites, "sprites.lod")
	openLod(&d.Games, "games.lod")
	openLod(&d.LangT, "EnglishT.lod")
	openLod(&d.LangD, "EnglishD.lod")
	openHwl(&d.HwlBitmaps, "d3dbitmap.hwl")
	openHwl(&d.HwlSprite, "d3dsprite.hwl")
	if err == nil {
		d.Exe, err = exe.Open(filepath.Join(dir, "MM8-Rel.exe"))
	}
	if err != nil {
		d.Close()
		return nil, err
	}
	return d, nil
}

// Close closes every opened archive.
func (d *Data) Close() error {
	var errs []error
	for _, a := range []*lod.Archive{d.Icons, d.Bitmaps, d.Sprites, d.Games, d.LangT, d.LangD} {
		if a != nil {
			errs = append(errs, a.Close())
		}
	}
	for _, h := range []*hwl.File{d.HwlBitmaps, d.HwlSprite} {
		if h != nil {
			errs = append(errs, h.Close())
		}
	}
	if d.Exe != nil {
		errs = append(errs, d.Exe.Close())
	}
	return errors.Join(errs...)
}

// LangFile reads a packed file from the language LODs, EnglishD first, then EnglishT.
//
// mm8: 0x4121c9 (Lod_LoadLanguageFile)
func (d *Data) LangFile(name string) (lod.FileHeader, []byte, error) {
	for _, a := range []*lod.Archive{d.LangD, d.LangT} {
		if _, ok := a.Find(name); ok {
			h, data, _, err := a.ReadPacked(name)
			return h, data, err
		}
	}
	return lod.FileHeader{}, nil, fmt.Errorf("language LODs: %q: %w", name, lod.ErrNotFound)
}
