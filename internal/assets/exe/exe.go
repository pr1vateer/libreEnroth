// Package exe reads constant tables out of the player's own MM8-Rel.exe by virtual
// address, so that game data compiled into the binary (class tables, layout offsets) is
// used at run time instead of being copied into this repository.
//
// Every table address is documented in re/symbols/names.tsv / re/notes.
package exe

import (
	"debug/pe"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
)

// ImageBase of MM8-Rel.exe.
const ImageBase = 0x400000

// Image is an opened PE file.
type Image struct {
	f    *pe.File
	file *os.File
}

// Open opens path (games_mm8/MM8-Rel.exe) and checks that it is the expected build.
func Open(path string) (*Image, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	f, err := pe.NewFile(file)
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	im := &Image{f: f, file: file}
	if oh, ok := f.OptionalHeader.(*pe.OptionalHeader32); !ok || oh.ImageBase != ImageBase || oh.AddressOfEntryPoint != 0xdf52b {
		im.Close()
		return nil, fmt.Errorf("%s: not the MM8-Rel.exe this code was reverse-engineered from (image base / entry point differ)", path)
	}
	return im, nil
}

// Close closes the file.
func (im *Image) Close() error { return im.file.Close() }

// ErrUnmapped is returned for addresses outside the file-backed part of a section.
var ErrUnmapped = errors.New("address not backed by file data")

// Read returns n bytes at virtual address va.
func (im *Image) Read(va uint32, n int) ([]byte, error) {
	rva := va - ImageBase
	for _, s := range im.f.Sections {
		if rva >= s.VirtualAddress && uint64(rva)+uint64(n) <= uint64(s.VirtualAddress)+uint64(s.Size) {
			b := make([]byte, n)
			if _, err := s.ReadAt(b, int64(rva-s.VirtualAddress)); err != nil && err != io.EOF {
				return nil, err
			}
			return b, nil
		}
	}
	return nil, fmt.Errorf("exe: %#x+%d: %w", va, n, ErrUnmapped)
}

// Int32s reads n little-endian int32 values at va.
func (im *Image) Int32s(va uint32, n int) ([]int32, error) {
	b, err := im.Read(va, 4*n)
	if err != nil {
		return nil, err
	}
	out := make([]int32, n)
	for i := range out {
		out[i] = int32(binary.LittleEndian.Uint32(b[4*i:]))
	}
	return out, nil
}

// CString reads a NUL-terminated string at va (at most 256 bytes).
func (im *Image) CString(va uint32) (string, error) {
	for n := 256; n > 0; n /= 2 {
		b, err := im.Read(va, n)
		if err != nil {
			continue
		}
		for i, c := range b {
			if c == 0 {
				return string(b[:i]), nil
			}
		}
		return string(b), nil
	}
	return "", fmt.Errorf("exe: string at %#x: %w", va, ErrUnmapped)
}

// CStringTable reads n pointers at va and the strings they point to.
func (im *Image) CStringTable(va uint32, n int) ([]string, error) {
	ptrs, err := im.Int32s(va, n)
	if err != nil {
		return nil, err
	}
	out := make([]string, n)
	for i, p := range ptrs {
		if out[i], err = im.CString(uint32(p)); err != nil {
			return nil, err
		}
	}
	return out, nil
}
