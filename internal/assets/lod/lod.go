// Package lod reads the original game's LOD archives (icons.lod, bitmaps.lod, sprites.lod,
// games.lod, EnglishT.lod, EnglishD.lod). Read-only.
//
// Layout (re/notes/formats.md): a 0x100-byte header, numChapters 32-byte chapter records,
// then per chapter `count` directory entries. MMVI/GameMMVI entries are 0x20 bytes
// (name[16]); MMVIII (language LODs) entries are 0x4c bytes (name[0x40]).
package lod

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

const (
	headerSize     = 0x100
	chapterSize    = 0x20
	entrySizeMM6   = 0x20
	entrySizeMM8   = 0x4c
	nameLenMM6     = 16
	nameLenMM8     = 0x40
	offNumChapters = 0xac
	offVersion     = 0x04
	offDescription = 0x54
	lenVersion     = 0x50
	lenDescription = 0x50
)

// VersionMM8 is the version string of the language LODs, whose entries are 0x4c bytes.
const VersionMM8 = "MMVIII"

// Entry is one file inside an archive.
type Entry struct {
	Name    string
	Chapter string
	Offset  int64 // absolute file offset of the entry data
	Size    int64
}

// Chapter is a directory inside an archive. Every shipped file has exactly one.
type Chapter struct {
	Name    string
	Offset  int64 // absolute
	Size    int64
	Entries []Entry
}

// Archive is an opened LOD file.
type Archive struct {
	Path        string
	Version     string // "MMVI", "GameMMVI" or "MMVIII"
	Description string
	Chapters    []Chapter
	Entries     []Entry // all chapters' entries, in file order

	r      io.ReaderAt
	closer io.Closer
	index  map[string]int // lower-case name -> Entries index (first chapter wins)
}

// Open opens a LOD file from disk.
func Open(path string) (*Archive, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	a, err := NewArchive(f, path)
	if err != nil {
		f.Close()
		return nil, err
	}
	a.closer = f
	return a, nil
}

// NewArchive parses a LOD from r. path is informational only.
//
// mm8: 0x460792 (Lod_OpenFile), 0x46084b (Lod_OpenChapter), 0x4613d9/0x461492 (MMVIII copies)
func NewArchive(r io.ReaderAt, path string) (*Archive, error) {
	hdr := make([]byte, headerSize)
	if _, err := r.ReadAt(hdr, 0); err != nil {
		return nil, fmt.Errorf("lod %s: header: %w", path, err)
	}
	if !bytes.Equal(hdr[:4], []byte("LOD\x00")) {
		return nil, fmt.Errorf("lod %s: bad magic %q", path, hdr[:4])
	}
	a := &Archive{
		Path:        path,
		Version:     cstring(hdr[offVersion : offVersion+lenVersion]),
		Description: cstring(hdr[offDescription : offDescription+lenDescription]),
		r:           r,
	}
	numChapters := int(int32(binary.LittleEndian.Uint32(hdr[offNumChapters:])))
	if numChapters < 0 || numChapters > 1024 {
		return nil, fmt.Errorf("lod %s: bad chapter count %d", path, numChapters)
	}
	entSize, nameLen := entrySizeMM6, nameLenMM6
	if a.Version == VersionMM8 {
		entSize, nameLen = entrySizeMM8, nameLenMM8
	}

	chapRecs := make([]byte, numChapters*chapterSize)
	if _, err := r.ReadAt(chapRecs, headerSize); err != nil {
		return nil, fmt.Errorf("lod %s: chapters: %w", path, err)
	}
	a.index = make(map[string]int)
	for c := 0; c < numChapters; c++ {
		rec := chapRecs[c*chapterSize:]
		ch := Chapter{
			Name:   cstring(rec[:16]),
			Offset: int64(int32(binary.LittleEndian.Uint32(rec[0x10:]))),
			Size:   int64(int32(binary.LittleEndian.Uint32(rec[0x14:]))),
		}
		count := int(int16(binary.LittleEndian.Uint16(rec[0x1c:])))
		if count < 0 {
			return nil, fmt.Errorf("lod %s: chapter %q: bad entry count %d", path, ch.Name, count)
		}
		dir := make([]byte, count*entSize)
		if _, err := r.ReadAt(dir, ch.Offset); err != nil {
			return nil, fmt.Errorf("lod %s: chapter %q directory: %w", path, ch.Name, err)
		}
		ch.Entries = make([]Entry, count)
		for i := range ch.Entries {
			e := dir[i*entSize:]
			ch.Entries[i] = Entry{
				Name:    cstring(e[:nameLen]),
				Chapter: ch.Name,
				Offset:  ch.Offset + int64(int32(binary.LittleEndian.Uint32(e[nameLen:]))),
				Size:    int64(int32(binary.LittleEndian.Uint32(e[nameLen+4:]))),
			}
		}
		a.Chapters = append(a.Chapters, ch)
		for _, e := range ch.Entries {
			if _, dup := a.index[strings.ToLower(e.Name)]; !dup {
				a.index[strings.ToLower(e.Name)] = len(a.Entries)
			}
			a.Entries = append(a.Entries, e)
		}
	}
	return a, nil
}

// Close releases the underlying file, if Open created it.
func (a *Archive) Close() error {
	if a.closer == nil {
		return nil
	}
	return a.closer.Close()
}

// IsMM8 reports whether the archive uses the MMVIII (0x4c-byte entry, 0x60-byte file
// header) layout.
func (a *Archive) IsMM8() bool { return a.Version == VersionMM8 }

// Find looks an entry up by name, ignoring case. The original binary-searches the sorted
// directory with _stricmp; the shipped directories are sorted and unique, so a map gives
// the same answers.
//
// mm8: 0x46053c (Lod_FindEntry), 0x46061b (Lod_BinarySearch), 0x4611e8, 0x461285 (MMVIII)
func (a *Archive) Find(name string) (Entry, bool) {
	i, ok := a.index[strings.ToLower(name)]
	if !ok {
		return Entry{}, false
	}
	return a.Entries[i], true
}

// ErrNotFound is returned (wrapped) when an entry does not exist.
var ErrNotFound = errors.New("entry not found")

// Raw returns the stored bytes of an entry.
func (a *Archive) Raw(name string) ([]byte, error) {
	e, ok := a.Find(name)
	if !ok {
		return nil, fmt.Errorf("lod %s: %q: %w", a.Path, name, ErrNotFound)
	}
	return a.ReadEntry(e)
}

// ReadEntry returns the stored bytes of e.
func (a *Archive) ReadEntry(e Entry) ([]byte, error) {
	if e.Size < 0 {
		return nil, fmt.Errorf("lod %s: %q: negative size", a.Path, e.Name)
	}
	buf := make([]byte, e.Size)
	if _, err := a.r.ReadAt(buf, e.Offset); err != nil {
		return nil, fmt.Errorf("lod %s: %q: %w", a.Path, e.Name, err)
	}
	return buf, nil
}

// ReadHead returns the first min(n, e.Size) bytes of e.
func (a *Archive) ReadHead(e Entry, n int) ([]byte, error) {
	if int64(n) > e.Size {
		n = int(max(e.Size, 0))
	}
	buf := make([]byte, n)
	if _, err := a.r.ReadAt(buf, e.Offset); err != nil {
		return nil, fmt.Errorf("lod %s: %q: %w", a.Path, e.Name, err)
	}
	return buf, nil
}

// cstring returns b up to the first NUL. Directory names carry junk after the terminator.
func cstring(b []byte) string {
	if i := bytes.IndexByte(b, 0); i >= 0 {
		b = b[:i]
	}
	return string(b)
}
