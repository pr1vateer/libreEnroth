// Package smacker decodes the video of Smacker files (SMK2 and SMK4), the format of the
// house and transition clips in Anims/*.vid. Audio tracks are skipped (their sizes are
// kept); decoding them is M11.
//
// The game plays these through Smackw32.dll (SmackOpen/SmackDoFrame/SmackNextFrame); this
// is an implementation of the file format that DLL consumes, written from the format, so
// there is no game code to tag here. The callers in the game are Video.cpp (0x4beb80…),
// see re/notes/houses.md.
//
// Layout: a 104-byte header, per-frame sizes (u32, low 2 bits are flags) and types (u8:
// bit 0 palette, bits 1..7 audio tracks 0..6), the packed Huffman trees, then the frames.
// A frame is [palette chunk][audio chunks][video bitstream]. Video is coded in 4×4 blocks.
package smacker

import (
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/color"
	"io"
	"time"
)

// Header flags.
const (
	FlagRingFrame  = 1 // one extra frame (index Frames) that leads from the last frame back to frame 0
	FlagYInterlace = 2
	FlagYDouble    = 4
)

// Header is the file header.
type Header struct {
	Version       byte // '2' or '4'
	Width, Height int
	Frames        int
	// FrameRate > 0: milliseconds per frame; < 0: 10-microsecond units per frame (negated);
	// 0: 10 frames per second.
	FrameRate  int32
	Flags      uint32
	AudioSize  [7]uint32
	TreesSize  uint32
	MMapSize   uint32 // allocation sizes of the four trees, informational
	MClrSize   uint32
	FullSize   uint32
	TypeSize   uint32
	AudioRate  [7]uint32
	FrameSizes []uint32 // as stored (bits 0..1 are flags), Frames (+1 with a ring frame) entries
	FrameTypes []byte
}

// FrameDuration is the time one frame is shown.
func (h *Header) FrameDuration() time.Duration {
	switch {
	case h.FrameRate > 0:
		return time.Duration(h.FrameRate) * time.Millisecond
	case h.FrameRate < 0:
		return time.Duration(-h.FrameRate) * 10 * time.Microsecond
	}
	return 100 * time.Millisecond
}

// Video is an opened Smacker file.
type Video struct {
	Header
	// Loop makes NextFrame continue past the last frame (through the ring frame when the
	// file has one) instead of returning io.EOF. The game loops house clips (Video+0x64).
	Loop bool
	// Truncated counts frames whose video bitstream ran out before the last block.
	Truncated int

	r       io.ReaderAt
	offsets []int64 // file offset of each frame
	trees   [4]*bigTree
	img     *image.Paletted
	pal     [256]color.RGBA
	next    int // index of the next frame to decode
	frameNo int // display number of the frame NextFrame returned last
	buf     []byte
}

// Tree order in the file.
const (
	treeMMap = iota
	treeMClr
	treeFull
	treeType
)

// Open reads the header and the Huffman trees.
func Open(r io.ReaderAt) (*Video, error) {
	var hdr [104]byte
	if _, err := r.ReadAt(hdr[:], 0); err != nil {
		return nil, fmt.Errorf("smacker: header: %w", err)
	}
	if string(hdr[:3]) != "SMK" || (hdr[3] != '2' && hdr[3] != '4') {
		return nil, fmt.Errorf("smacker: bad signature %q", hdr[:4])
	}
	le := binary.LittleEndian
	v := &Video{r: r}
	h := &v.Header
	h.Version = hdr[3]
	h.Width = int(le.Uint32(hdr[4:]))
	h.Height = int(le.Uint32(hdr[8:]))
	h.Frames = int(le.Uint32(hdr[12:]))
	h.FrameRate = int32(le.Uint32(hdr[16:]))
	h.Flags = le.Uint32(hdr[20:])
	for i := range h.AudioSize {
		h.AudioSize[i] = le.Uint32(hdr[24+4*i:])
		h.AudioRate[i] = le.Uint32(hdr[72+4*i:])
	}
	h.TreesSize = le.Uint32(hdr[52:])
	h.MMapSize = le.Uint32(hdr[56:])
	h.MClrSize = le.Uint32(hdr[60:])
	h.FullSize = le.Uint32(hdr[64:])
	h.TypeSize = le.Uint32(hdr[68:])
	if h.Width <= 0 || h.Height <= 0 || h.Width > 4096 || h.Height > 4096 || h.Frames <= 0 || h.Frames > 1<<20 {
		return nil, fmt.Errorf("smacker: bad dimensions %dx%d, %d frames", h.Width, h.Height, h.Frames)
	}
	if h.Flags&(FlagYInterlace|FlagYDouble) != 0 {
		return nil, fmt.Errorf("smacker: unsupported flags %#x", h.Flags)
	}
	n := h.Frames
	if h.Flags&FlagRingFrame != 0 {
		n++
	}
	tab := make([]byte, 5*n+int(h.TreesSize))
	if _, err := r.ReadAt(tab, 104); err != nil {
		return nil, fmt.Errorf("smacker: frame tables: %w", err)
	}
	h.FrameSizes = make([]uint32, n)
	v.offsets = make([]int64, n)
	off := int64(104 + len(tab))
	for i := range h.FrameSizes {
		h.FrameSizes[i] = le.Uint32(tab[4*i:])
		v.offsets[i] = off
		off += int64(h.FrameSizes[i] &^ 3)
	}
	h.FrameTypes = tab[4*n : 5*n]
	b := &bits{buf: tab[5*n:]}
	for i := range v.trees {
		t, err := readBigTree(b)
		if err != nil {
			return nil, fmt.Errorf("smacker: tree %d: %w", i, err)
		}
		v.trees[i] = t
	}
	if b.err != nil {
		return nil, fmt.Errorf("smacker: trees: %w", b.err)
	}
	v.img = image.NewPaletted(image.Rect(0, 0, h.Width, h.Height), nil)
	return v, nil
}

// Frame returns the image of the last decoded frame (nil before the first NextFrame).
// It is updated in place by NextFrame.
func (v *Video) Frame() *image.Paletted {
	if v.img.Palette == nil {
		return nil
	}
	return v.img
}

// FrameNumber is the display number (0..Frames-1) of the frame NextFrame returned last.
// The ring frame counts as frame 0.
func (v *Video) FrameNumber() int { return v.frameNo }

// Rewind restarts at frame 0.
func (v *Video) Rewind() {
	v.next = 0
	v.img.Palette = nil
}

// NextFrame decodes the next frame and returns the image, which is reused (copy it to
// keep it). After the last frame it returns io.EOF unless Loop is set.
func (v *Video) NextFrame() (*image.Paletted, error) {
	idx := v.next
	if idx >= v.Frames {
		if !v.Loop {
			return nil, io.EOF
		}
		if v.Flags&FlagRingFrame == 0 {
			idx = 0
		}
		// else: the ring frame (index Frames) shows frame 0 again.
	}
	if idx == 0 {
		for i := range v.pal {
			v.pal[i] = color.RGBA{A: 0xff}
		}
		clear(v.img.Pix)
	}
	if err := v.decode(idx); err != nil {
		return nil, fmt.Errorf("smacker: frame %d: %w", idx, err)
	}
	if idx >= v.Frames {
		v.frameNo = 0
	} else {
		v.frameNo = idx
	}
	v.next = v.frameNo + 1
	return v.img, nil
}

func (v *Video) decode(idx int) error {
	size := int(v.FrameSizes[idx] &^ 3)
	if cap(v.buf) < size {
		v.buf = make([]byte, size)
	}
	buf := v.buf[:size]
	if _, err := v.r.ReadAt(buf, v.offsets[idx]); err != nil {
		return err
	}
	typ := v.FrameTypes[idx]
	if typ&1 != 0 {
		if len(buf) == 0 {
			return errors.New("palette chunk missing")
		}
		n := 4 * int(buf[0])
		if n == 0 || n > len(buf) {
			return fmt.Errorf("palette chunk size %d", n)
		}
		if err := v.decodePalette(buf[1:n]); err != nil {
			return err
		}
		buf = buf[n:]
	}
	for t := 0; t < 7; t++ {
		if typ&(2<<t) == 0 {
			continue
		}
		if len(buf) < 4 {
			return fmt.Errorf("audio chunk %d truncated", t)
		}
		n := int(binary.LittleEndian.Uint32(buf))
		if n < 4 || n > len(buf) {
			return fmt.Errorf("audio chunk %d size %d", t, n)
		}
		buf = buf[n:]
	}
	pal := make(color.Palette, 256)
	for i := range pal {
		pal[i] = v.pal[i]
	}
	v.img.Palette = pal
	return v.decodeVideo(buf)
}

// decodePalette applies a palette chunk to the current palette. Each byte starts a run:
// 0x80|n skips n+1 entries (kept), 0x40|n copies n+1 entries from the previous palette
// starting at the next byte, otherwise it is the 6-bit red of a new entry followed by
// green and blue.
func (v *Video) decodePalette(p []byte) error {
	old := v.pal
	i := 0
	for i < 256 {
		if len(p) == 0 {
			return errors.New("palette chunk truncated")
		}
		c := p[0]
		p = p[1:]
		switch {
		case c&0x80 != 0:
			i += int(c&0x7f) + 1
		case c&0x40 != 0:
			if len(p) == 0 {
				return errors.New("palette chunk truncated")
			}
			src := int(p[0])
			p = p[1:]
			n := int(c&0x3f) + 1
			if src+n > 256 {
				return fmt.Errorf("palette copy %d+%d out of range", src, n)
			}
			for ; n > 0 && i < 256; n-- {
				v.pal[i] = old[src]
				i++
				src++
			}
		default:
			if len(p) < 2 {
				return errors.New("palette chunk truncated")
			}
			v.pal[i] = color.RGBA{expand6(c), expand6(p[0]), expand6(p[1]), 0xff}
			p = p[2:]
			i++
		}
	}
	return nil
}

// expand6 widens a 6-bit component to 8 bits by bit replication.
func expand6(c byte) uint8 {
	c &= 0x3f
	return c<<2 | c>>4
}

// runLengths maps the 6-bit run field of a type code to a number of blocks.
var runLengths = func() (t [64]int) {
	for i := range 59 {
		t[i] = i + 1
	}
	copy(t[59:], []int{128, 256, 512, 1024, 2048})
	return
}()

// Block types (low 2 bits of a type code).
const (
	blockMono = iota
	blockFull
	blockVoid
	blockSolid
)

func (v *Video) decodeVideo(data []byte) error {
	for _, t := range v.trees {
		t.reset()
	}
	b := &bits{buf: data}
	bw, bh := v.Width/4, v.Height/4
	blocks := bw * bh
	stride := v.img.Stride
	pix := v.img.Pix
	at := func(blk int) int { return (blk/bw)*4*stride + (blk%bw)*4 }
	for blk := 0; blk < blocks; {
		typ := v.trees[treeType].decode(b)
		run := runLengths[typ>>2&0x3f]
		switch typ & 3 {
		case blockMono:
			for ; run > 0 && blk < blocks; run, blk = run-1, blk+1 {
				clr := v.trees[treeMClr].decode(b)
				m := v.trees[treeMMap].decode(b)
				hi, lo := uint8(clr>>8), uint8(clr)
				o := at(blk)
				for y := 0; y < 4; y++ {
					row := pix[o+y*stride : o+y*stride+4]
					for x := range row {
						if m&1 != 0 {
							row[x] = hi
						} else {
							row[x] = lo
						}
						m >>= 1
					}
				}
			}
		case blockFull:
			mode := 0
			if v.Version == '4' {
				if b.bit() {
					mode = 1
				} else if b.bit() {
					mode = 2
				}
			}
			for ; run > 0 && blk < blocks; run, blk = run-1, blk+1 {
				v.fullBlock(b, pix[at(blk):], stride, mode)
			}
		case blockVoid:
			blk += run
		case blockSolid:
			c := uint8(typ >> 8)
			for ; run > 0 && blk < blocks; run, blk = run-1, blk+1 {
				o := at(blk)
				for y := 0; y < 4; y++ {
					row := pix[o+y*stride : o+y*stride+4]
					row[0], row[1], row[2], row[3] = c, c, c, c
				}
			}
		}
	}
	if b.err != nil {
		// A truncated frame (cquixote.smk's ring frame stops 15 blocks short) reads
		// zero bits for the rest, as a decoder reading past its buffer would; count it.
		v.Truncated++
	}
	return nil
}

// fullBlock decodes a block of 16-bit codes, each two pixels (low byte left). Mode 0
// (SMK2): per row, the right pair then the left pair. SMK4 adds mode 1 (two codes, each
// a row of 2×2-doubled pixels covering two rows) and mode 2 (per two rows, the right pair
// then the left pair, each repeated on both rows).
func (v *Video) fullBlock(b *bits, out []byte, stride, mode int) {
	t := v.trees[treeFull]
	put := func(o int, c uint16) { out[o], out[o+1] = uint8(c), uint8(c>>8) }
	switch mode {
	case 0:
		for y := 0; y < 4; y++ {
			put(y*stride+2, t.decode(b))
			put(y*stride, t.decode(b))
		}
	case 1:
		for y := 0; y < 4; y += 2 {
			c := t.decode(b)
			for r := y; r < y+2; r++ {
				o := r * stride
				out[o], out[o+1] = uint8(c), uint8(c)
				out[o+2], out[o+3] = uint8(c>>8), uint8(c>>8)
			}
		}
	case 2:
		for y := 0; y < 4; y += 2 {
			right := t.decode(b)
			left := t.decode(b)
			for r := y; r < y+2; r++ {
				put(r*stride, left)
				put(r*stride+2, right)
			}
		}
	}
}
