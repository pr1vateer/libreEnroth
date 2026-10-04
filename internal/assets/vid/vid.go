// Package vid reads the .vid containers of Anims/ (mightdod.vid, Magicdod.vid): a u32
// count, count × {char name[40]; u32 offset}, then the Smacker/Bink files back to back.
// An entry runs from its offset to the next larger offset (or the end of the file).
package vid

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Entry is one file of the container.
type Entry struct {
	Name         string
	Offset, Size int64
}

// File is an opened .vid container.
type File struct {
	Path    string
	Entries []Entry
	f       *os.File
}

// ErrNotFound is returned when no container has an entry of that name.
var ErrNotFound = errors.New("vid: entry not found")

// Open reads the directory of the container at path.
//
// mm8: 0x4beb80 (Video_OpenContainers)
func Open(path string) (*File, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	v, err := parse(f)
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	v.Path = path
	return v, nil
}

func parse(f *os.File) (*File, error) {
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	var n uint32
	if err := binary.Read(f, binary.LittleEndian, &n); err != nil {
		return nil, err
	}
	if int64(n)*44+4 > st.Size() {
		return nil, fmt.Errorf("vid: %d entries do not fit in %d bytes", n, st.Size())
	}
	dir := make([]byte, int(n)*44)
	if _, err := io.ReadFull(f, dir); err != nil {
		return nil, err
	}
	v := &File{f: f, Entries: make([]Entry, n)}
	for i := range v.Entries {
		rec := dir[i*44:]
		name := rec[:40]
		if j := strings.IndexByte(string(name), 0); j >= 0 {
			name = name[:j]
		}
		off := int64(binary.LittleEndian.Uint32(rec[40:]))
		if off < int64(len(dir))+4 || off > st.Size() {
			return nil, fmt.Errorf("vid: entry %q offset %#x out of range", name, off)
		}
		v.Entries[i] = Entry{Name: string(name), Offset: off}
	}
	// Sizes: up to the next entry in file order.
	idx := make([]int, n)
	for i := range idx {
		idx[i] = i
	}
	sort.Slice(idx, func(a, b int) bool { return v.Entries[idx[a]].Offset < v.Entries[idx[b]].Offset })
	for k, i := range idx {
		end := st.Size()
		if k+1 < len(idx) {
			end = v.Entries[idx[k+1]].Offset
		}
		v.Entries[i].Size = end - v.Entries[i].Offset
	}
	return v, nil
}

// Close closes the file.
func (v *File) Close() error { return v.f.Close() }

// Find returns the entry named name, compared case-insensitively (the game uses _stricmp),
// first match in directory order.
//
// mm8: 0x4bf3b1 (Video_OpenSmk, the lookup loop)
func (v *File) Find(name string) (Entry, bool) {
	for _, e := range v.Entries {
		if strings.EqualFold(e.Name, name) {
			return e, true
		}
	}
	return Entry{}, false
}

// Reader returns a reader over entry e.
func (v *File) Reader(e Entry) *io.SectionReader { return io.NewSectionReader(v.f, e.Offset, e.Size) }

// Set is the pair of containers the game opens, searched in order.
type Set []*File

// OpenSet opens Anims/mightdod.vid and Anims/magicdod.vid under the game directory dir.
// The file names are matched case-insensitively (the install has "Magicdod.vid").
//
// mm8: 0x4beb80 (Video_OpenContainers)
func OpenSet(dir string) (Set, error) {
	anims, err := findFold(dir, "anims")
	if err != nil {
		return nil, err
	}
	var s Set
	for _, name := range []string{"mightdod.vid", "magicdod.vid"} {
		p, err := findFold(anims, name)
		if err == nil {
			var v *File
			if v, err = Open(p); err == nil {
				s = append(s, v)
				continue
			}
		}
		s.Close()
		return nil, err
	}
	return s, nil
}

// Find looks name up in mightdod.vid, then magicdod.vid.
//
// mm8: 0x4bf3b1 (Video_OpenSmk)
func (s Set) Find(name string) (*io.SectionReader, error) {
	for _, v := range s {
		if e, ok := v.Find(name); ok {
			return v.Reader(e), nil
		}
	}
	return nil, fmt.Errorf("%w: %s", ErrNotFound, name)
}

// Close closes every container.
func (s Set) Close() error {
	var errs []error
	for _, v := range s {
		errs = append(errs, v.Close())
	}
	return errors.Join(errs...)
}

func findFold(dir, name string) (string, error) {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	for _, e := range ents {
		if strings.EqualFold(e.Name(), name) {
			return filepath.Join(dir, e.Name()), nil
		}
	}
	return "", fmt.Errorf("%s: no %q: %w", dir, name, os.ErrNotExist)
}
