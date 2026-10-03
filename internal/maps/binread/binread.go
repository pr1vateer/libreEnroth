// Package binread is a bounds-checked little-endian cursor over the unpacked map blobs
// (.odm/.blv/.ddm/.dlv). Every read past the end, or a count that cannot fit the rest of
// the blob, records an error instead of panicking; check Err once after parsing.
package binread

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
)

var le = binary.LittleEndian

// Reader reads from a byte slice.
type Reader struct {
	b   []byte
	off int
	err error
}

// New returns a Reader at offset 0.
func New(b []byte) *Reader { return &Reader{b: b} }

// Err is the first error.
func (r *Reader) Err() error { return r.err }

// Off is the current offset.
func (r *Reader) Off() int { return r.off }

// Len is the number of unread bytes.
func (r *Reader) Len() int { return len(r.b) - r.off }

// Fail records an error (the first one wins).
func (r *Reader) Fail(format string, args ...any) {
	if r.err == nil {
		r.err = fmt.Errorf("offset %#x: %s", r.off, fmt.Sprintf(format, args...))
	}
}

// Bytes returns the next n bytes (a sub-slice, not a copy), or nil after an error.
func (r *Reader) Bytes(n int) []byte {
	if r.err != nil {
		return nil
	}
	if n < 0 || n > len(r.b)-r.off {
		r.Fail("need %d bytes, %d left", n, len(r.b)-r.off)
		return nil
	}
	s := r.b[r.off : r.off+n]
	r.off += n
	return s
}

// Skip advances n bytes.
func (r *Reader) Skip(n int) { r.Bytes(n) }

// U32 reads a uint32 (0 after an error).
func (r *Reader) U32() uint32 {
	if b := r.Bytes(4); b != nil {
		return le.Uint32(b)
	}
	return 0
}

// I32 reads an int32.
func (r *Reader) I32() int32 { return int32(r.U32()) }

// Count reads an int32 count and checks 0 <= n <= max and that n records of size
// bytes fit the rest of the blob. It returns 0 after an error.
func (r *Reader) Count(what string, size, max int) int {
	n := int(r.I32())
	if r.err != nil {
		return 0
	}
	if n < 0 || n > max {
		r.Fail("%s count %d out of range 0..%d", what, n, max)
		return 0
	}
	if size > 0 && n > r.Len()/size {
		r.Fail("%s: %d x %#x bytes exceed the %d bytes left", what, n, size, r.Len())
		return 0
	}
	return n
}

// Record reads one fixed-size record.
func (r *Reader) Record(size int) Rec { return Rec(r.Bytes(size)) }

// Rec is a fixed-size record with field accessors; reads past its end return 0 so a
// truncated blob (already reported by the Reader) never panics.
type Rec []byte

func (b Rec) ok(o, n int) bool { return o >= 0 && o+n <= len(b) }

// U8 reads a byte at o.
func (b Rec) U8(o int) uint8 {
	if !b.ok(o, 1) {
		return 0
	}
	return b[o]
}

// U16 reads a uint16 at o.
func (b Rec) U16(o int) uint16 {
	if !b.ok(o, 2) {
		return 0
	}
	return le.Uint16(b[o:])
}

// I16 reads an int16 at o.
func (b Rec) I16(o int) int16 { return int16(b.U16(o)) }

// U32 reads a uint32 at o.
func (b Rec) U32(o int) uint32 {
	if !b.ok(o, 4) {
		return 0
	}
	return le.Uint32(b[o:])
}

// I32 reads an int32 at o.
func (b Rec) I32(o int) int32 { return int32(b.U32(o)) }

// F32 reads a float32 at o.
func (b Rec) F32(o int) float32 {
	return float32frombits(b.U32(o))
}

// Str reads a NUL-terminated string from the n bytes at o.
func (b Rec) Str(o, n int) string {
	if !b.ok(o, n) {
		return ""
	}
	s := b[o : o+n]
	if i := bytes.IndexByte(s, 0); i >= 0 {
		s = s[:i]
	}
	return string(s)
}

func float32frombits(u uint32) float32 { return math.Float32frombits(u) }
