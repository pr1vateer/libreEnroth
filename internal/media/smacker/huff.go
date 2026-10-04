package smacker

import (
	"encoding/binary"
	"errors"
)

// errOverrun is set when a bitstream is read past its end.
var errOverrun = errors.New("smacker: bitstream overrun")

// bits reads a byte slice as a bitstream, least significant bit of each byte first.
type bits struct {
	buf []byte
	pos uint // in bits
	err error
}

// peek returns the next 32 bits (zero past the end) without consuming them.
func (b *bits) peek() uint32 {
	i := b.pos >> 3
	var w uint64
	if i+8 <= uint(len(b.buf)) {
		w = binary.LittleEndian.Uint64(b.buf[i:])
	} else {
		for k := uint(0); i+k < uint(len(b.buf)) && k < 8; k++ {
			w |= uint64(b.buf[i+k]) << (8 * k)
		}
	}
	return uint32(w >> (b.pos & 7))
}

func (b *bits) skip(n uint) {
	b.pos += n
	if b.pos > uint(len(b.buf))*8 && b.err == nil {
		b.err = errOverrun
	}
}

func (b *bits) bit() bool {
	v := b.peek() & 1
	b.skip(1)
	return v != 0
}

func (b *bits) read(n uint) uint32 {
	v := b.peek() & (1<<n - 1)
	b.skip(n)
	return v
}

// tableBits is the width of the first-level decode table; longer codes finish with a walk.
const tableBits = 11

// huff is a decoded Huffman tree. Leaves hold indexes into vals, so that the "big" trees
// can rewrite the values of their three escape leaves while decoding.
type huff struct {
	// kids[2n], kids[2n+1]: the 0/1 children of internal node n; >= 0 is a node, < 0 is
	// ^leaf. Node 0 is the root unless root is a leaf.
	kids []int32
	root int32 // a node, or ^leaf; with no leaves at all the tree decodes 0
	vals []uint16
	// tbl, indexed by the next tableBits bits: leaf<<8 | length, or for codes longer than
	// the table, 1<<31 | node<<8 (the node reached after tableBits bits).
	tbl   []uint32
	empty bool
}

const maxDepth = 32

// readTree8 reads a packed 8-bit tree: the recursive tree (1 = node, 0-branch then
// 1-branch; 0 = leaf followed by its 8-bit value), then a 0 bit. The caller has read the
// presence bit.
func readTree8(b *bits) (*huff, error) {
	h := &huff{}
	var err error
	h.root, err = h.readNode(b, 0, func() uint16 { return uint16(b.read(8)) })
	if err != nil {
		return nil, err
	}
	b.skip(1)
	h.build()
	return h, b.err
}

func (h *huff) readNode(b *bits, depth int, leaf func() uint16) (int32, error) {
	if b.err != nil {
		return 0, b.err
	}
	if depth > maxDepth {
		return 0, errors.New("smacker: huffman tree too deep")
	}
	if !b.bit() {
		h.vals = append(h.vals, leaf())
		return ^int32(len(h.vals) - 1), nil
	}
	n := int32(len(h.kids) / 2)
	h.kids = append(h.kids, 0, 0)
	for i := int32(0); i < 2; i++ {
		k, err := h.readNode(b, depth+1, leaf)
		if err != nil {
			return 0, err
		}
		h.kids[2*n+i] = k
	}
	return n, nil
}

// build fills the first-level table.
func (h *huff) build() {
	h.tbl = make([]uint32, 1<<tableBits)
	var fill func(n int32, code uint32, depth uint)
	fill = func(n int32, code uint32, depth uint) {
		if n < 0 {
			for s := uint32(0); s < 1<<(tableBits-depth); s++ {
				h.tbl[code|s<<depth] = uint32(^n)<<8 | uint32(depth)
			}
			return
		}
		if depth == tableBits {
			h.tbl[code] = 1<<31 | uint32(n)<<8
			return
		}
		fill(h.kids[2*n], code, depth+1)
		fill(h.kids[2*n+1], code|1<<depth, depth+1)
	}
	fill(h.root, 0, 0)
}

// leaf decodes one code and returns its leaf index (-1 for an empty tree).
func (h *huff) leaf(b *bits) int {
	if h.empty || h.tbl == nil {
		return -1
	}
	e := h.tbl[b.peek()&(1<<tableBits-1)]
	if e&(1<<31) == 0 {
		b.skip(uint(e & 0xff))
		return int(e >> 8)
	}
	b.skip(tableBits)
	n := int32(e>>8) & 0x7fffff
	for n >= 0 {
		n = h.kids[2*n+int32(b.read(1))]
		if b.err != nil {
			return -1
		}
	}
	return int(^n)
}

// decode returns the value of the next code (0 for an empty tree).
func (h *huff) decode(b *bits) uint16 {
	if i := h.leaf(b); i >= 0 {
		return h.vals[i]
	}
	return 0
}

// bigTree is a 16-bit tree: its leaves are a low-byte and a high-byte code from two
// 8-bit trees, and three escape values mark leaves whose values are a cache of the last
// three values decoded (reset to 0 at every frame).
type bigTree struct {
	h    huff
	last [3]int // leaf indexes of the three cache leaves
}

// readBigTree reads a big tree: presence bit; if set, the low and high 8-bit trees (each
// with its own presence bit), three 16-bit escape values, the recursive tree, a 0 bit.
func readBigTree(b *bits) (*bigTree, error) {
	t := &bigTree{}
	if !b.bit() {
		t.h.empty = true
		return t, b.err
	}
	var sub [2]*huff
	for i := range sub {
		if !b.bit() {
			sub[i] = &huff{empty: true}
			continue
		}
		var err error
		if sub[i], err = readTree8(b); err != nil {
			return nil, err
		}
	}
	var esc [3]uint16
	for i := range esc {
		esc[i] = uint16(b.read(16))
	}
	t.last = [3]int{-1, -1, -1}
	var err error
	t.h.root, err = t.h.readNode(b, 0, func() uint16 {
		v := sub[0].decode(b) | sub[1].decode(b)<<8
		for i, e := range esc {
			if v == e {
				t.last[i] = len(t.h.vals)
				return 0
			}
		}
		return v
	})
	if err != nil {
		return nil, err
	}
	b.skip(1)
	// Escapes that no leaf carries still get a (never decoded) cache slot.
	for i := range t.last {
		if t.last[i] < 0 {
			t.last[i] = len(t.h.vals)
			t.h.vals = append(t.h.vals, 0)
		}
	}
	t.h.build()
	return t, b.err
}

// reset zeroes the cache leaves (start of every frame).
func (t *bigTree) reset() {
	if t.h.empty {
		return
	}
	for _, i := range t.last {
		t.h.vals[i] = 0
	}
}

// decode returns the next value and pushes it onto the cache when it differs from the
// most recent one.
func (t *bigTree) decode(b *bits) uint16 {
	i := t.h.leaf(b)
	if i < 0 {
		return 0
	}
	v := t.h.vals[i]
	if c := t.h.vals; c[t.last[0]] != v {
		c[t.last[2]] = c[t.last[1]]
		c[t.last[1]] = c[t.last[0]]
		c[t.last[0]] = v
	}
	return v
}
