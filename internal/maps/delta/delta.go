// Package delta parses the per-map state files that go with a map: .ddm for outdoor
// and .dlv for indoor maps. games.lod holds the templates a new game starts from; saves
// hold the same files. See re/notes/blv.md.
//
// Both share one layout, with a few kind-specific blocks:
//
//	0x28    location header
//	        revealed bits: .ddm 2 x 0x3c8 (minimap), .dlv 0x36b (one per outline)
//	u32     attributes per face (.ddm: every BModel face in model order)
//	u16     flags per decoration
//	        actors (i32 count, 0x3cc each), objects (0x70), chests (0x14cc)
//	        .dlv only: 200 doors of 0x50 bytes, then L.DData (BlvHeader.DDataSize bytes)
//	200     event variables
//	0x38    location time (last visit, sky, weather, fog)
//	        optional 0x778c bytes of map notes (the .dlv templates stop 4 bytes short,
//	        and only one .ddm template, elema, has them)
package delta

import (
	"fmt"

	"libre-enroth/internal/maps/binread"
)

// Kind selects the layout.
type Kind int

const (
	DDM Kind = iota // outdoor
	DLV             // indoor
)

// Record sizes and limits (Odm_Load 0x47df28, Blv_Load 0x498050).
const (
	ActorSize  = 0x3cc
	ObjectSize = 0x70
	ChestSize  = 0x14cc
	DoorSize   = 0x50
	MaxActors  = 500
	MaxObjects = 1000
	MaxChests  = 20
	NumDoors   = 200
	NotesSize  = 0x778c
)

// Door is one 0x50-byte indoor door with its L.DData arrays.
type Door struct {
	Attr       uint32   // +0x00: DoorStartClosed, DoorMoving, DoorStopped
	ID         uint32   // +0x04
	Time       int32    // +0x08: ticks since triggered
	Dir        [3]int32 // +0x0c: 16.16 unit vector
	MoveLength int32    // +0x18
	OpenSpeed  int32    // +0x1c: used while opening (state 3)
	CloseSpeed int32    // +0x20: used while closing (state 1)
	Verts      []int16  // map vertices that move
	Faces      []int16  // map faces whose texture/plane follow
	Sectors    []int16
	DeltaU     []int16 // per face: base texture offsets (from the face extras)
	DeltaV     []int16
	XOffsets   []int16 // per vertex: the open position (distance 0)
	YOffsets   []int16
	ZOffsets   []int16
	State      uint16 // +0x4c: DoorOpen..DoorOpening
}

// Door attributes and states (Door_UpdateAll 0x46f475, Level_Load 0x45f895). A door
// moves its vertices MoveLength along Dir from their open position as it closes; most
// doors in the templates start closed.
const (
	DoorStartClosed = 0x1
	DoorMoving      = 0x2
	DoorStopped     = 0x8

	DoorOpen    = 0 // at the offsets
	DoorClosing = 1 // moving CloseSpeed*time/128 towards MoveLength
	DoorClosed  = 2 // MoveLength along Dir
	DoorOpening = 3 // moving back at OpenSpeed
)

// Delta is a parsed .ddm/.dlv. The actor/object/chest records stay raw until the
// milestones that use them.
type Delta struct {
	Header    [0x28]byte
	Revealed  []byte
	FaceAttrs []uint32
	DecFlags  []uint16
	Actors    [][]byte
	Objects   [][]byte
	Chests    [][]byte
	Doors     []Door // .dlv only
	Vars      [200]byte
	Time      [0x38]byte
	Notes     []byte // optional; may be short
}

// Parse decodes an unpacked delta blob for a map with numFaces faces and numDecs
// decorations; ddataSize is BlvHeader.DDataSize for .dlv and ignored for .ddm.
//
// mm8: 0x47df28 (.ddm part of Odm_Load), 0x498050 (.dlv part of Blv_Load)
func Parse(b []byte, kind Kind, numFaces, numDecs int, ddataSize int32) (*Delta, error) {
	r := binread.New(b)
	d := &Delta{}
	copy(d.Header[:], r.Bytes(0x28))
	if kind == DDM {
		d.Revealed = r.Bytes(2 * 0x3c8)
	} else {
		d.Revealed = r.Bytes(0x36b)
	}
	fa := binread.Rec(r.Bytes(4 * numFaces))
	d.FaceAttrs = make([]uint32, numFaces)
	for i := range d.FaceAttrs {
		d.FaceAttrs[i] = fa.U32(4 * i)
	}
	df := binread.Rec(r.Bytes(2 * numDecs))
	d.DecFlags = make([]uint16, numDecs)
	for i := range d.DecFlags {
		d.DecFlags[i] = df.U16(2 * i)
	}
	records := func(what string, size, max int) [][]byte {
		n := r.Count(what, size, max)
		out := make([][]byte, n)
		for i := range out {
			out[i] = r.Bytes(size)
		}
		return out
	}
	d.Actors = records("actors", ActorSize, MaxActors)
	d.Objects = records("objects", ObjectSize, MaxObjects)
	d.Chests = records("chests", ChestSize, MaxChests)
	if kind == DLV {
		recs := make([]binread.Rec, NumDoors)
		for i := range recs {
			recs[i] = r.Record(DoorSize)
		}
		dd := binread.New(r.Bytes(int(ddataSize)))
		d.Doors = make([]Door, NumDoors)
		for i, h := range recs {
			parseDoor(h, dd, &d.Doors[i])
		}
		if err := dd.Err(); err != nil {
			return nil, fmt.Errorf("delta: DData: %w", err)
		}
		if dd.Len() != 0 {
			return nil, fmt.Errorf("delta: DData: %d bytes unused", dd.Len())
		}
	}
	copy(d.Vars[:], r.Bytes(200))
	copy(d.Time[:], r.Bytes(0x38))
	if err := r.Err(); err != nil {
		return nil, fmt.Errorf("delta: %w", err)
	}
	// Blv_Load copies 0x778c bytes regardless (the templates are 4 bytes short); Odm_Load
	// copies them when the header says they are there.
	d.Notes = r.Bytes(min(r.Len(), NotesSize))
	if r.Len() != 0 {
		return nil, fmt.Errorf("delta: %d trailing bytes", r.Len())
	}
	return d, nil
}

// parseDoor reads a door record and its arrays from L.DData.
//
// mm8: 0x498050 (door pointer fix-up after the .dlv doors)
func parseDoor(h binread.Rec, dd *binread.Reader, d *Door) {
	d.Attr, d.ID, d.Time = h.U32(0), h.U32(4), h.I32(8)
	d.Dir = [3]int32{h.I32(0xc), h.I32(0x10), h.I32(0x14)}
	d.MoveLength, d.OpenSpeed, d.CloseSpeed = h.I32(0x18), h.I32(0x1c), h.I32(0x20)
	d.State = h.U16(0x4c)
	nv, nf, ns, no := int(h.U16(0x44)), int(h.U16(0x46)), int(h.U16(0x48)), int(h.U16(0x4a))
	arr := func(n int) []int16 {
		a := binread.Rec(dd.Bytes(2 * n))
		out := make([]int16, n)
		for k := range out {
			out[k] = a.I16(2 * k)
		}
		return out
	}
	d.Verts, d.Faces, d.Sectors = arr(nv), arr(nf), arr(ns)
	d.DeltaU, d.DeltaV = arr(nf), arr(nf)
	d.XOffsets, d.YOffsets, d.ZOffsets = arr(no), arr(no), arr(no)
}

// Settle restarts a door the way Level_Load does after loading a map: closed doors
// (and those flagged to start closed) close again and open ones open again, both with
// time 0x3c00, so the next Door_UpdateAll puts them in place.
//
// mm8: 0x45f895 (door reset)
func (d *Door) Settle() {
	switch {
	case d.Attr&DoorStartClosed != 0:
		d.Attr, d.State, d.Time = DoorMoving, DoorClosing, 0x3c00
	case d.State == DoorOpen:
		d.Attr, d.State, d.Time = DoorMoving, DoorOpening, 0x3c00
	case d.State == DoorClosed:
		d.Attr, d.State, d.Time = DoorMoving, DoorClosing, 0x3c00
	}
}

// Distance is how far the vertices of a moving door are along Dir: speed*time/128 from
// where it started, clamped to 0..MoveLength. done reports that the move is complete
// (the door is then DoorClosed or DoorOpen).
//
// mm8: 0x46f475 (Door_UpdateAll)
func (d *Door) Distance() (dist int32, done bool) {
	switch d.State {
	case DoorClosing:
		dist = d.CloseSpeed * d.Time / 128
		if dist >= d.MoveLength {
			return d.MoveLength, true
		}
		return dist, false
	case DoorOpening:
		moved := d.OpenSpeed * d.Time / 128
		if moved >= d.MoveLength {
			return 0, true
		}
		return d.MoveLength - moved, false
	case DoorClosed:
		return d.MoveLength, true
	}
	return 0, true
}
