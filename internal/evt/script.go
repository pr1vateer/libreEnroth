// Package evt is the event script VM: the .evt files of global.evt and of each map,
// their .str strings, and the interpreter that runs them (re/notes/evt.md). The VM
// knows nothing about maps or the UI; it acts through a Host.
package evt

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

// Op is a record's opcode.
type Op uint8

// Record is one script line: the bytes of the record from its length byte on.
//
//	+0 u8  length of what follows
//	+1 u16 event id
//	+3 u8  step
//	+4 u8  opcode
//	+5     arguments
type Record []byte

// ID is the event the record belongs to.
func (r Record) ID() int { return int(binary.LittleEndian.Uint16(r[1:])) }

// Step is the record's step within its event.
func (r Record) Step() int { return int(r[3]) }

// Op is the record's opcode.
func (r Record) Op() Op { return Op(r[4]) }

// U8 reads the byte at offset o from the start of the record (0 past the end).
func (r Record) U8(o int) int {
	if o >= len(r) {
		return 0
	}
	return int(r[o])
}

// U16 reads a little-endian u16 at offset o.
func (r Record) U16(o int) int { return r.U8(o) | r.U8(o+1)<<8 }

// U32 reads a little-endian u32 at offset o.
func (r Record) U32(o int) uint32 {
	return uint32(r.U8(o)) | uint32(r.U8(o+1))<<8 | uint32(r.U8(o+2))<<16 | uint32(r.U8(o+3))<<24
}

// I32 reads a little-endian i32 at offset o.
func (r Record) I32(o int) int32 { return int32(r.U32(o)) }

// Str reads the NUL-terminated string at offset o (to the end of the record).
func (r Record) Str(o int) string {
	if o >= len(r) {
		return ""
	}
	s := r[o:]
	if i := bytes.IndexByte(s, 0); i >= 0 {
		s = s[:i]
	}
	return string(s)
}

// Script is a parsed .evt file: its records in file order, which is the order the
// interpreter scans them in (the index the game builds has one {id, step, offset}
// entry per record).
type Script struct {
	Records []Record
}

// Buffer limits of the game's event loaders: a file must be smaller than its buffer.
//
// mm8: 0x441a6f (Evt_LoadGlobal: 0xb400 bytes, index 5000 entries), 0x442119
// (Evt_LoadMapEvents: 0x2800 bytes .evt and .str), 0x4419ae (Evt_LoadFile: size >= buffer
// is an error)
const (
	GlobalMaxBytes   = 0xb400
	MapMaxBytes      = 0x2800
	MaxRecords       = 5000 // the index holds 60000 bytes of 12-byte entries
	StrMaxStrings    = 500
	StrMaxStringLen  = 800
	StrMaxBytes      = 0x2800
	recordHeaderSize = 5
)

// Parse splits an .evt file into records; maxBytes is its buffer (GlobalMaxBytes or
// MapMaxBytes).
//
// mm8: 0x441a6f (Evt_LoadGlobal: index walk), 0x441bcb (Evt_BuildMapIndex)
func Parse(b []byte, maxBytes int) (*Script, error) {
	if len(b) >= maxBytes {
		return nil, fmt.Errorf("evt: %d bytes, buffer %d", len(b), maxBytes)
	}
	s := &Script{}
	for off := 0; off < len(b); {
		n := int(b[off]) + 1
		if n < recordHeaderSize || off+n > len(b) {
			return nil, fmt.Errorf("evt: record at %#x: length %d", off, n-1)
		}
		s.Records = append(s.Records, Record(b[off:off+n:off+n]))
		off += n
	}
	if len(s.Records) > MaxRecords {
		return nil, fmt.Errorf("evt: %d records, index holds %d", len(s.Records), MaxRecords)
	}
	return s, nil
}

// ParseStrings splits a .str file into its NUL-terminated strings (what follows the
// last NUL is not one), each with surrounding quotes removed.
//
// mm8: 0x441aff (Evt_LoadStr), 0x451806 (Txt_StripQuotes)
func ParseStrings(b []byte) ([]string, error) {
	if len(b) >= StrMaxBytes {
		return nil, fmt.Errorf("evt: .str %d bytes, buffer %d", len(b), StrMaxBytes)
	}
	var out []string
	for {
		i := bytes.IndexByte(b, 0)
		if i < 0 {
			break
		}
		s := b[:i]
		if len(s) > StrMaxStringLen {
			return nil, fmt.Errorf("evt: .str string %d is %d long (MAX_EVENT_TEXT_LENGTH %d)", len(out), len(s), StrMaxStringLen)
		}
		if len(s) > 0 && s[0] == '"' {
			s = s[1:max(len(s)-1, 1)]
		}
		out = append(out, string(s))
		b = b[i+1:]
	}
	if len(out) > StrMaxStrings {
		return nil, fmt.Errorf("evt: .str has %d strings, at most %d", len(out), StrMaxStrings)
	}
	return out, nil
}

// HoverText is what the status line says over something with event id: the house
// name when the event's Hint is followed by SpeakInHouse or the event goes into a
// house (id < 600) later, else the Hint's string; "" when the event has no Hint.
//
// mm8: 0x4424e6 (Evt_HoverText)
func (s *Script) HoverText(id int, strs []string, house func(int) string) string {
	recs := s.Records
	for i, r := range recs {
		if r.ID() != id || r.Op() != OpHint {
			continue
		}
		if i+1 < len(recs) && recs[i+1].Op() == OpSpeakInHouse {
			return house(int(recs[i+1].U32(5)))
		}
		for _, n := range recs[i+1:] {
			if n.ID() != id {
				break
			}
			if n.Op() == OpSpeakInHouse && n.U32(5) < 600 {
				return house(int(n.U32(5)))
			}
		}
		if k := r.U8(5); k < len(strs) {
			return strs[k]
		}
		return ""
	}
	return ""
}
