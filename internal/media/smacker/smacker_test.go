package smacker

import (
	"bytes"
	"encoding/binary"
	"image/color"
	"io"
	"testing"
)

// bitWriter writes a bitstream least significant bit first.
type bitWriter struct {
	buf []byte
	n   uint
}

func (w *bitWriter) put(v uint32, n uint) {
	for i := uint(0); i < n; i++ {
		if w.n%8 == 0 {
			w.buf = append(w.buf, 0)
		}
		if v>>i&1 != 0 {
			w.buf[len(w.buf)-1] |= 1 << (w.n % 8)
		}
		w.n++
	}
}

// tnode is a tree to encode: a leaf (kids == nil) or a node.
type tnode struct {
	val  uint16
	kids *[2]tnode
}

func leaf(v uint16) tnode          { return tnode{val: v} }
func node(a, b tnode) tnode        { return tnode{kids: &[2]tnode{a, b}} }
func (w *bitWriter) tree8(t tnode) { w.put(1, 1); w.tree8Node(t); w.put(0, 1) }
func (w *bitWriter) tree8Node(t tnode) {
	if t.kids == nil {
		w.put(0, 1)
		w.put(uint32(t.val), 8)
		return
	}
	w.put(1, 1)
	w.tree8Node(t.kids[0])
	w.tree8Node(t.kids[1])
}

// codes returns the code (LSB-first) and length of every leaf value of t.
func codes(t tnode) map[uint16][2]uint32 {
	m := map[uint16][2]uint32{}
	var walk func(t tnode, code, n uint32)
	walk = func(t tnode, code, n uint32) {
		if t.kids == nil {
			m[t.val] = [2]uint32{code, n}
			return
		}
		walk(t.kids[0], code, n+1)
		walk(t.kids[1], code|1<<n, n+1)
	}
	walk(t, 0, 0)
	return m
}

// bigTree writes a big tree whose 16-bit leaf values are coded through the low/high
// 8-bit trees lo and hi.
func (w *bitWriter) bigTree(t tnode, lo, hi tnode, esc [3]uint16) {
	w.put(1, 1)
	w.tree8(lo)
	w.tree8(hi)
	for _, e := range esc {
		w.put(uint32(e), 16)
	}
	lc, hc := codes(lo), codes(hi)
	var walk func(t tnode)
	walk = func(t tnode) {
		if t.kids == nil {
			w.put(0, 1)
			c := lc[t.val&0xff]
			w.put(c[0], uint(c[1]))
			c = hc[t.val>>8]
			w.put(c[0], uint(c[1]))
			return
		}
		w.put(1, 1)
		walk(t.kids[0])
		walk(t.kids[1])
	}
	walk(t)
	w.put(0, 1)
}

// code writes the code of value v of tree t.
func (w *bitWriter) code(t tnode, v uint16) {
	c, ok := codes(t)[v]
	if !ok {
		panic("no such leaf")
	}
	w.put(c[0], uint(c[1]))
}

func TestTree8(t *testing.T) {
	tr := node(leaf('A'), node(leaf('B'), node(leaf('C'), leaf('D'))))
	var w bitWriter
	w.put(1, 1) // presence, read by the caller
	w.tree8Node(tr)
	w.put(0, 1)
	for _, v := range []uint16{'C', 'A', 'D', 'B', 'A'} {
		w.code(tr, v)
	}
	b := &bits{buf: w.buf}
	if !b.bit() {
		t.Fatal("presence bit")
	}
	h, err := readTree8(b)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []uint16{'C', 'A', 'D', 'B', 'A'} {
		if got := h.decode(b); got != want {
			t.Fatalf("decode = %c, want %c", got, want)
		}
	}
	if b.pos != w.n {
		t.Errorf("consumed %d bits, wrote %d", b.pos, w.n)
	}
}

// A deep (longer than the first-level table) chain decodes through the walk.
func TestTree8Deep(t *testing.T) {
	tr := leaf(20)
	for v := uint16(19); v > 0; v-- {
		tr = node(leaf(v), tr)
	}
	var w bitWriter
	w.tree8Node(tr)
	w.put(0, 1)
	seq := []uint16{20, 1, 15, 19, 12, 2}
	for _, v := range seq {
		w.code(tr, v)
	}
	b := &bits{buf: w.buf}
	h, err := readTree8(b)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range seq {
		if got := h.decode(b); got != want {
			t.Fatalf("decode = %d, want %d", got, want)
		}
	}
}

// The three escape leaves return the last three distinct values decoded; a value equal
// to the most recent one does not shift the cache. reset zeroes them.
func TestBigTreeCache(t *testing.T) {
	lo := node(leaf(0x11), node(leaf(0x22), node(leaf(0x33), node(leaf(0xe0), node(leaf(0xe1), leaf(0xe2))))))
	hi := node(leaf(0x00), leaf(0xee))
	esc := [3]uint16{0xeee0, 0xeee1, 0xeee2}
	tr := node(node(leaf(0x0011), leaf(0x0022)), node(leaf(0x0033), node(leaf(0xeee0), node(leaf(0xeee1), leaf(0xeee2)))))
	var w bitWriter
	w.bigTree(tr, lo, hi, esc)
	// 0x11, 0x22, 0x33, then last[0] (0x33), last[1] (0x22), last[2] (0x11 after shifts)...
	seq := []uint16{0x0011, 0x0022, 0x0033, 0xeee0, 0xeee1, 0xeee2, 0x0022, 0x0022, 0xeee1}
	for _, v := range seq {
		w.code(tr, v)
	}
	b := &bits{buf: w.buf}
	bt, err := readBigTree(b)
	if err != nil {
		t.Fatal(err)
	}
	// The cache slots hold values: after 11,22,33 it is [33 22 11]. esc0 -> 33 (no shift).
	// esc1 -> 22 and shifts: slot 2 takes slot 1 (22), slot 1 takes slot 0: [22 33 22], so
	// 11 is gone and esc2 -> 22. The last esc1 -> 33.
	want := []uint16{0x11, 0x22, 0x33, 0x33, 0x22, 0x22, 0x22, 0x22, 0x33}
	for i, wv := range want {
		if got := bt.decode(b); got != wv {
			t.Fatalf("decode %d = %#x, want %#x", i, got, wv)
		}
	}
	bt.reset()
	for i := range bt.last {
		if v := bt.h.vals[bt.last[i]]; v != 0 {
			t.Errorf("after reset, cache %d = %#x", i, v)
		}
	}
}

func TestEmptyBigTree(t *testing.T) {
	b := &bits{buf: []byte{0}}
	bt, err := readBigTree(b)
	if err != nil {
		t.Fatal(err)
	}
	if v := bt.decode(b); v != 0 || b.pos != 1 {
		t.Errorf("empty tree: %#x after %d bits", v, b.pos)
	}
}

// synth builds an 8×8 SMK2 file of the given frames (each: palette chunk or nil, and a
// function writing the video bitstream with the trees below).
type synthTrees struct {
	typ, mclr, mmap, full tnode
}

var st = synthTrees{
	// type codes: solid colour 7 run 1 (7<<8|0<<2|3), mono run 1, full run 1, void run 2
	// (run index 1), solid colour 9 run 4 (index 3).
	typ:  node(node(leaf(0x0703), leaf(0x0000)), node(leaf(0x0001), node(leaf(0x0006), leaf(0x090f)))),
	mclr: node(leaf(0x0201), leaf(0x0403)),
	mmap: node(leaf(0x8421), leaf(0xffff)),
	full: node(node(leaf(0x0a0b), leaf(0x0c0d)), node(leaf(0x0e0f), leaf(0x1011))),
}

func lowHigh(t tnode) (lo, hi tnode) {
	seen := [2]map[uint16]bool{{}, {}}
	var vals [2][]uint16
	for v := range codes(t) {
		for i, b := range []uint16{v & 0xff, v >> 8} {
			if !seen[i][b] {
				seen[i][b] = true
				vals[i] = append(vals[i], b)
			}
		}
	}
	mk := func(vs []uint16) tnode {
		tr := leaf(vs[len(vs)-1])
		for i := len(vs) - 2; i >= 0; i-- {
			tr = node(leaf(vs[i]), tr)
		}
		return tr
	}
	return mk(vals[0]), mk(vals[1])
}

func synth(t *testing.T, ring bool, frames [][2][]byte) []byte {
	t.Helper()
	var tw bitWriter
	for _, tr := range []tnode{st.mmap, st.mclr, st.full, st.typ} {
		lo, hi := lowHigh(tr)
		tw.bigTree(tr, lo, hi, [3]uint16{0xfff0, 0xfff1, 0xfff2})
	}
	n := len(frames)
	nf := n
	if ring {
		nf--
	}
	var hdr [104]byte
	copy(hdr[:], "SMK2")
	le := binary.LittleEndian
	le.PutUint32(hdr[4:], 8)
	le.PutUint32(hdr[8:], 8)
	le.PutUint32(hdr[12:], uint32(nf))
	rate := int32(-6666)
	le.PutUint32(hdr[16:], uint32(rate))
	if ring {
		le.PutUint32(hdr[20:], FlagRingFrame)
	}
	le.PutUint32(hdr[52:], uint32(len(tw.buf)))
	var out bytes.Buffer
	out.Write(hdr[:])
	var bodies [][]byte
	for _, f := range frames {
		var body []byte
		if f[0] != nil {
			chunk := append([]byte{0}, f[0]...)
			for len(chunk)%4 != 0 {
				chunk = append(chunk, 0)
			}
			chunk[0] = byte(len(chunk) / 4)
			body = append(body, chunk...)
		}
		// One audio chunk on track 0, to be skipped.
		body = append(body, 6, 0, 0, 0, 0xaa, 0xbb)
		body = append(body, f[1]...)
		for len(body)%4 != 0 {
			body = append(body, 0)
		}
		bodies = append(bodies, body)
	}
	for i, b := range bodies {
		size := uint32(len(b))
		if i == 0 {
			size |= 1
		}
		binary.Write(&out, le, size)
	}
	for _, f := range frames {
		typ := byte(2) // audio track 0
		if f[0] != nil {
			typ |= 1
		}
		out.WriteByte(typ)
	}
	out.Write(tw.buf)
	for _, b := range bodies {
		out.Write(b)
	}
	return out.Bytes()
}

func video(f func(w *bitWriter)) []byte {
	var w bitWriter
	f(&w)
	return w.buf
}

func TestSynthetic(t *testing.T) {
	// Palette: entry 0 new (63,0,0), skip 2, copy 1 from old index 0 (black at frame 0).
	pal0 := []byte{63, 0, 0, 0x81, 0x40, 0, 0x80 | 0x7f, 0x80 | 0x7b}
	f0 := video(func(w *bitWriter) {
		w.code(st.typ, 0x0703) // block 0: solid 7
		w.code(st.typ, 0x0000) // block 1: mono
		w.code(st.mclr, 0x0201)
		w.code(st.mmap, 0x8421)
		w.code(st.typ, 0x0001) // block 2: full
		for _, v := range []uint16{0x0a0b, 0x0c0d, 0x0e0f, 0x1011, 0x0a0b, 0x0c0d, 0x0e0f, 0x1011} {
			w.code(st.full, v)
		}
		w.code(st.typ, 0x0703) // block 3: solid 7
	})
	// Frame 1: void 2 (blocks 0,1 kept), solid 9 run 4 (clipped to the 2 left).
	pal1 := []byte{0x80, 0x40 | 0, 0, 0x80 | 0x7f, 0x80 | 0x7d} // entry 0 kept, entry 1 := old 0
	f1 := video(func(w *bitWriter) {
		w.code(st.typ, 0x0006)
		w.code(st.typ, 0x090f)
	})
	f2 := f0 // ring frame
	data := synth(t, true, [][2][]byte{{pal0, f0}, {pal1, f1}, {pal0, f2}})
	v, err := Open(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if v.Frames != 2 || v.Width != 8 || v.FrameDuration().Microseconds() != 66660 {
		t.Fatalf("header: %d frames %dx%d %v", v.Frames, v.Width, v.Height, v.FrameDuration())
	}
	img, err := v.NextFrame()
	if err != nil {
		t.Fatal(err)
	}
	if got := img.Palette[0]; got != (color.RGBA{255, 0, 0, 255}) {
		t.Errorf("palette 0 = %v", got)
	}
	if got := img.Palette[3]; got != (color.RGBA{0, 0, 0, 255}) {
		t.Errorf("palette 3 (copied from black) = %v", got)
	}
	want0 := []string{
		"\x07\x07\x07\x07\x02\x01\x01\x01",
		"\x07\x07\x07\x07\x01\x02\x01\x01",
		"\x07\x07\x07\x07\x01\x01\x02\x01",
		"\x07\x07\x07\x07\x01\x01\x01\x02",
		"\x0d\x0c\x0b\x0a\x07\x07\x07\x07",
		"\x11\x10\x0f\x0e\x07\x07\x07\x07",
		"\x0d\x0c\x0b\x0a\x07\x07\x07\x07",
		"\x11\x10\x0f\x0e\x07\x07\x07\x07",
	}
	check := func(name string, want []string) {
		t.Helper()
		for y, row := range want {
			if got := string(img.Pix[y*img.Stride : y*img.Stride+8]); got != row {
				t.Errorf("%s row %d = %q, want %q", name, y, got, row)
			}
		}
	}
	check("frame 0", want0)
	if img, err = v.NextFrame(); err != nil {
		t.Fatal(err)
	}
	want1 := append([]string(nil), want0[:4]...)
	for range 4 {
		want1 = append(want1, "\x09\x09\x09\x09\x09\x09\x09\x09")
	}
	check("frame 1", want1)
	if img.Palette[1] != (color.RGBA{255, 0, 0, 255}) {
		t.Errorf("frame 1 palette 1 = %v (copy of old 0)", img.Palette[1])
	}
	if _, err := v.NextFrame(); err != io.EOF {
		t.Fatalf("past the end without Loop: %v", err)
	}
	v.Loop = true
	if img, err = v.NextFrame(); err != nil {
		t.Fatal(err)
	}
	if v.FrameNumber() != 0 {
		t.Errorf("ring frame number %d", v.FrameNumber())
	}
	check("ring frame", want0)
	if _, err = v.NextFrame(); err != nil || v.FrameNumber() != 1 {
		t.Errorf("after ring: frame %d, %v", v.FrameNumber(), err)
	}
}

func TestBadSignature(t *testing.T) {
	if _, err := Open(bytes.NewReader(make([]byte, 200))); err == nil {
		t.Error("accepted zeros")
	}
}
