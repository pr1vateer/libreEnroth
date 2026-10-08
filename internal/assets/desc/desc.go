// Package desc parses the binary descriptor tables of EnglishT.lod that the game loads at
// startup (Game_Init 0x464974): dtile*.bin (terrain tiles), ddeclist.bin (decorations),
// dsft.bin (sprite frames) and dtft.bin (texture animation frames). Each is a count
// followed by fixed-size records; see re/notes/odm.md#descriptor-tables.
package desc

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"strings"
)

var le = binary.LittleEndian

func cstr(b []byte) string {
	if i := bytes.IndexByte(b, 0); i >= 0 {
		b = b[:i]
	}
	return string(b)
}

func i16(b []byte, o int) int { return int(int16(le.Uint16(b[o:]))) }

// records validates "u32 count, count x size bytes" (plus extra trailing bytes).
func records(name string, b []byte, size int) (int, error) {
	if len(b) < 4 {
		return 0, fmt.Errorf("%s: %d bytes", name, len(b))
	}
	n := int(int32(le.Uint32(b)))
	if n < 0 || 4+n*size > len(b) {
		return 0, fmt.Errorf("%s: %d records of %#x bytes do not fit %d bytes", name, n, size, len(b))
	}
	return n, nil
}

// Tile is one dtile.bin record (0x1a bytes).
type Tile struct {
	Name    string // bitmaps.lod texture, +0x00 char[16]
	ID      int    // +0x10
	Tileset int    // +0x14: TTtype_* (0 grass .. 4 dirt, 5 water, ...)
	Section int    // +0x16: TTsect_* (0 = the plain base tile of the set)
	Attr    int    // +0x18: TTattr_* bits (Burn, Water, Block, ...)
}

// Tile attribute bits (tile.def TTattr_*; FUN_0048940e).
const (
	TileWater = 0x2
)

// Tiles is a terrain tile table.
type Tiles []Tile

// ParseTiles parses dtile.bin / dtile2.bin / dtile3.bin.
//
// mm8: 0x4893c7 (TileTable binary load, 0x1a-byte records)
func ParseTiles(b []byte) (Tiles, error) {
	const size = 0x1a
	n, err := records("dtile", b, size)
	if err != nil {
		return nil, err
	}
	t := make(Tiles, n)
	for i := range t {
		r := b[4+i*size:]
		t[i] = Tile{Name: cstr(r[:16]), ID: i16(r, 0x10), Tileset: i16(r, 0x14), Section: i16(r, 0x16), Attr: int(le.Uint16(r[0x18:]))}
	}
	return t, nil
}

// First returns the index of the first tile of tileset with section 0 (0 if none).
//
// mm8: 0x48934a (TileTable find), called as 0x48929c(table, tileset, 1)
func (t Tiles) First(tileset int) int {
	for i, x := range t {
		if x.Tileset == tileset && x.Section == 0 {
			return i
		}
	}
	return 0
}

// Decoration is one ddeclist.bin record (0x54 bytes).
type Decoration struct {
	Name, GameName string // +0x00 char[32], +0x20 char[32]
	Type           int    // +0x40
	Height         int    // +0x42
	Radius         int    // +0x44
	LightRadius    int    // +0x46
	SFT            int    // +0x48: dsft.bin frame index
	Flags          int    // +0x4a
	Sound          int    // +0x4c
	R, G, B        uint8  // +0x50: coloured light (D3D)
}

// Decoration flags (FUN_0047b61b).
const (
	DecMoveThrough = 0x1
	DecNoDraw      = 0x2  // with 0x20: not drawn (markers)
	DecMarker      = 0x20 // event / sound markers
	DecEmitFire    = 0x400
	DecEmitSmoke   = 0x80
)

// DecList is the decoration table. Entry 0 is a placeholder; lookups by name start at 1.
type DecList []Decoration

// ParseDecList parses ddeclist.bin.
//
// mm8: 0x457360 (DecorationList binary load, 0x54-byte records)
func ParseDecList(b []byte) (DecList, error) {
	const size = 0x54
	n, err := records("ddeclist", b, size)
	if err != nil {
		return nil, err
	}
	d := make(DecList, n)
	for i := range d {
		r := b[4+i*size:]
		d[i] = Decoration{
			Name: cstr(r[:0x20]), GameName: cstr(r[0x20:0x40]),
			Type: i16(r, 0x40), Height: i16(r, 0x42), Radius: i16(r, 0x44), LightRadius: i16(r, 0x46),
			SFT: i16(r, 0x48), Flags: int(le.Uint16(r[0x4a:])), Sound: i16(r, 0x4c),
			R: r[0x50], G: r[0x51], B: r[0x52],
		}
	}
	return d, nil
}

// Find returns the index of the decoration called name, searching from 1 like the
// original; 0 when not found.
//
// mm8: 0x446c9e (DecorationList find by name)
func (d DecList) Find(name string) int {
	for i := 1; i < len(d); i++ {
		if strings.EqualFold(d[i].Name, name) {
			return i
		}
	}
	return 0
}

// Frame is one dsft.bin sprite frame (0x3c bytes).
type Frame struct {
	Group   string // +0x00 char[12]: the sequence name
	Sprite  string // +0x0c char[12]: sprite base name
	Scale   int32  // +0x28: 16.16 world units per texel
	Flags   uint32 // +0x2c
	Glow    int    // +0x30: light radius
	Palette int    // +0x32: pal%03d
	Time    int    // +0x36: frame duration in 1/16 s (ticks >> 3)
	Length  int    // +0x38: whole sequence length, on its first frame
}

// Sprite frame flags (sft.txt keywords, FUN_0044c433).
const (
	FrameHasMore   = 0x1     // the next record continues this sequence
	FrameLuminous  = 0x2     // not darkened by light
	FrameOneView   = 0x10    // one sprite for all directions
	FrameFidget    = 0x40    // "stA" views 3-5
	FrameMirror0   = 0x100   // Mirror0..Mirror7: view i is a mirrored copy
	FrameThreeView = 0x10000 // 3 views (0, 2, 4)
	FrameGlow      = 0x20000
	FrameBlend     = 0x40000 // drawn half-transparent
)

// SFT is the sprite frame table.
type SFT struct {
	Frames []Frame
	EIndex []int // "E frames": indices of the sequence starts, sorted by group name
}

// ParseSFT parses dsft.bin: u32 frames, u32 sequences, frames x 0x3c, sequences x i16.
//
// mm8: 0x44c378 (CSpriteFrameTable binary load)
func ParseSFT(b []byte) (*SFT, error) {
	const size = 0x3c
	if len(b) < 8 {
		return nil, fmt.Errorf("dsft: %d bytes", len(b))
	}
	n, e := int(int32(le.Uint32(b))), int(int32(le.Uint32(b[4:])))
	if n < 0 || e < 0 || 8+n*size+2*e > len(b) {
		return nil, fmt.Errorf("dsft: %d frames + %d sequences do not fit %d bytes", n, e, len(b))
	}
	s := &SFT{Frames: make([]Frame, n), EIndex: make([]int, e)}
	for i := range s.Frames {
		r := b[8+i*size:]
		s.Frames[i] = Frame{
			Group: cstr(r[:12]), Sprite: cstr(r[12:24]), Scale: int32(le.Uint32(r[0x28:])), Flags: le.Uint32(r[0x2c:]),
			Glow: i16(r, 0x30), Palette: i16(r, 0x32), Time: i16(r, 0x36), Length: i16(r, 0x38),
		}
	}
	for i := range s.EIndex {
		s.EIndex[i] = i16(b, 8+n*size+2*i)
	}
	return s, nil
}

// At returns the index of the frame of the sequence starting at first that shows at
// time t (game ticks, 128 per second).
//
// mm8: 0x44c26c (CSpriteFrameTable::GetFrame)
func (s *SFT) At(first, t int) int {
	if first < 0 || first >= len(s.Frames) {
		return first
	}
	f := &s.Frames[first]
	if f.Flags&FrameHasMore == 0 || f.Length == 0 {
		return first
	}
	rem := (t >> 3) % f.Length
	i := first
	for i < len(s.Frames) && s.Frames[i].Time < rem {
		rem -= s.Frames[i].Time
		i++
	}
	return min(i, len(s.Frames)-1)
}

// AtReverse is At with the sequence played backwards: the frame at length - t's offset.
//
// mm8: 0x44c2bc (SFT_GetFrameReverse)
func (s *SFT) AtReverse(first, t int) int {
	if first < 0 || first >= len(s.Frames) {
		return first
	}
	f := &s.Frames[first]
	if f.Flags&FrameHasMore == 0 || f.Length == 0 {
		return first
	}
	rem := f.Length - (t>>3)%f.Length
	i := first
	for i < len(s.Frames) && s.Frames[i].Time < rem {
		rem -= s.Frames[i].Time
		i++
	}
	return min(i, len(s.Frames)-1)
}

// stricmp is the C runtime's _stricmp: bytes compared lower-cased, unsigned.
func stricmp(a, b string) int {
	lower := func(c byte) byte {
		if 'A' <= c && c <= 'Z' {
			return c + 'a' - 'A'
		}
		return c
	}
	for i := 0; ; i++ {
		var ca, cb byte
		if i < len(a) {
			ca = lower(a[i])
		}
		if i < len(b) {
			cb = lower(b[i])
		}
		if ca != cb || ca == 0 {
			return int(ca) - int(cb)
		}
	}
}

// FindGroup is the first frame of the sequence named name (case-insensitive), 0 when there
// is none. The search is the original's: a binary search over the sorted sequences that
// turns linear once fewer than 5 remain.
//
// mm8: 0x44c1ad (SFT_FindGroup), 0x44c1d4 (SFT_BinarySearch)
func (s *SFT) FindGroup(name string) int {
	key := func(i int) string {
		if f := s.EIndex[i]; f >= 0 && f < len(s.Frames) {
			return s.Frames[f].Group
		}
		return ""
	}
	found := -1
	linear := func(lo, hi int) {
		found = -1
		for i := lo; i < hi; i++ {
			if stricmp(name, key(i)) == 0 {
				found = i
				return
			}
		}
	}
	lo, hi := 0, len(s.EIndex)
	for {
		n := hi - lo
		mid := n/2 + lo
		c := 0
		if mid < len(s.EIndex) {
			c = stricmp(name, key(mid))
		} else {
			c = -1
		}
		if c == 0 {
			found = mid
		}
		if lo == hi {
			found = -1
			break
		}
		if c < 0 {
			if n < 5 {
				linear(lo, hi)
				break
			}
			hi = mid
			continue
		}
		if n <= 4 {
			linear(lo, hi)
			break
		}
		lo = mid
	}
	if found < 0 {
		return 0
	}
	return s.EIndex[found]
}

// ViewNames returns the sprites.lod / d3dsprite.hwl names of the 8 views of frame f
// and which views are drawn mirrored.
//
// mm8: 0x44beaa (CSpriteFrameTable::LoadSprites)
func (f *Frame) ViewNames() (names [8]string, mirror [8]bool) {
	n := f.Sprite
	switch {
	case f.Flags&FrameOneView != 0:
		for i := range names {
			names[i] = n
		}
	case f.Flags&FrameThreeView != 0:
		for i, v := range [8]int{0, 0, 2, 4, 4, 4, 2, 0} {
			names[i] = fmt.Sprintf("%s%d", n, v)
		}
	case f.Flags&FrameFidget != 0:
		st := n
		if len(st) >= 3 {
			st = st[:len(st)-3] + "stA"
		}
		names = [8]string{n + "0", n + "1", n + "2", st + "3", st + "4", st + "3", n + "2", n + "1"}
	default:
		// A mirrored view i loads the sprite of view 8-i. The original has no name
		// for mirrored views 0 and 4 and reuses whatever its buffer held (the previous
		// view's name for 4); view 0 is never mirrored in the shipped dsft.bin.
		for i := range names {
			switch {
			case f.Flags&(FrameMirror0<<i) == 0 || i == 0:
				names[i] = fmt.Sprintf("%s%d", n, i)
			case i == 4:
				names[i] = names[3]
			default:
				names[i] = fmt.Sprintf("%s%d", n, 8-i)
			}
		}
	}
	for i := range mirror {
		mirror[i] = f.Flags&(FrameMirror0<<i) != 0
	}
	return names, mirror
}

// TexFrame is one dtft.bin record (0x14 bytes).
type TexFrame struct {
	Name   string // +0x00 char[12]: bitmaps.lod texture
	Time   int    // +0x0e: duration in ticks >> 3
	Length int    // +0x10: sequence length, on its first frame
	Flags  int    // +0x12: 1 = the next record continues the sequence
}

// TFT is the texture animation table.
type TFT []TexFrame

// ParseTFT parses dtft.bin.
//
// mm8: 0x44ca63 (CTextureFrameTable binary load, 0x14-byte records)
func ParseTFT(b []byte) (TFT, error) {
	const size = 0x14
	n, err := records("dtft", b, size)
	if err != nil {
		return nil, err
	}
	t := make(TFT, n)
	for i := range t {
		r := b[4+i*size:]
		t[i] = TexFrame{Name: cstr(r[:12]), Time: i16(r, 0x0e), Length: i16(r, 0x10), Flags: i16(r, 0x12)}
	}
	return t, nil
}

// Find returns the index of the sequence named name, or -1.
//
// mm8: 0x44cb2a (CTextureFrameTable::FindTextureByName)
func (t TFT) Find(name string) int {
	for i, f := range t {
		if strings.EqualFold(f.Name, name) {
			return i
		}
	}
	return -1
}

// At returns the frame of the sequence starting at first that shows at time t (ticks).
//
// mm8: 0x44cb61 (CTextureFrameTable::GetFrameTexture)
func (t TFT) At(first, tick int) int {
	if first < 0 || first >= len(t) || t[first].Flags&1 == 0 || t[first].Length == 0 {
		return first
	}
	rem := (tick >> 3) % t[first].Length
	i := first
	for i < len(t)-1 && t[i].Time < rem {
		rem -= t[i].Time
		i++
	}
	return i
}

// PlayerFrame is one dpft.bin record (10 bytes): a frame of a portrait expression.
type PlayerFrame struct {
	Expr    int // +0: expression id on a sequence's first record, 0 on the others
	Texture int // +2: face frame t, icons.lod "<prefix>%02d"
	Time    int // +4: duration in ticks >> 3
	Total   int // +6: the sequence's summed Time, on its first record
	Flags   int // +8: PlayerFrameMore, PlayerFrameNew
}

// Player frame flags (pft.def, Pft_LoadDef 0x494d91).
const (
	PlayerFrameMore = 0x1 // the next record continues this sequence
	PlayerFrameNew  = 0x4 // "New": starts a sequence
)

// PFT is the player (portrait) frame table.
type PFT []PlayerFrame

// ParsePFT parses dpft.bin.
//
// mm8: 0x494d46 (Pft_LoadBin)
func ParsePFT(b []byte) (PFT, error) {
	const size = 10
	n, err := records("dpft", b, size)
	if err != nil {
		return nil, err
	}
	t := make(PFT, n)
	for i := range t {
		r := b[4+i*size:]
		u := func(o int) int { return int(le.Uint16(r[o:])) }
		t[i] = PlayerFrame{Expr: u(0), Texture: u(2), Time: u(4), Total: u(6), Flags: u(8)}
	}
	return t, nil
}

// Find returns the index of the first record of expression expr, or 0.
//
// mm8: 0x494c1d (Pft_Find)
func (t PFT) Find(expr int) int {
	for i, f := range t {
		if f.Expr == expr {
			return i
		}
	}
	return 0
}

// FrameAt returns the index of the record of the sequence starting at i that shows at
// time t (ticks into the expression).
//
// mm8: 0x494c40 (Pft_FrameAt)
func (t PFT) FrameAt(i, time int) int {
	if i < 0 || i >= len(t) {
		return i
	}
	if f := t[i]; f.Flags&PlayerFrameMore != 0 && f.Total != 0 {
		rem := (time >> 3) % f.Total
		for i < len(t)-1 && t[i].Time < rem {
			rem -= t[i].Time
			i++
		}
	}
	return i
}

// TalkFrame advances a talking face (expression 0x15) by ticks: when the current talk
// record (*frame, one of 0x15..0x18) has run its course, rand picks the next one. It
// returns the record to show.
//
// mm8: 0x494c95 (Pft_TalkFrame)
func (t PFT) TalkFrame(frame, time *int, ticks int, rand func() int) int {
	next := *time + ticks
	if next < t[*frame].Time*8 {
		*time = next
	} else {
		*frame = rand()%4 + 0x15
		// The original reduces the time by the new record's Time and scales by 8
		// afterwards ((t % time) << 3), not (t % (time << 3)).
		*time = next % t[*frame].Time << 3
	}
	return *frame
}
