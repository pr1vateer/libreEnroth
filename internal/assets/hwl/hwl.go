// Package hwl reads the Direct3D texture caches d3dbitmap.hwl and d3dsprite.hwl:
// "D3DT", u32 directory offset; directory = u32 count, count x name[20] (sorted),
// count x u32 entry offsets. Each entry: u32 packed size, 8 x u32 (origW, origH, cropW,
// cropH, width, height, cropX, cropY), then width*height ARGB1555 pixels (zlib when
// packed != 0).
//
// The pixels are resampled to power-of-two sizes: a bitmap is its origW x origH
// original at half size, a sprite is only its opaque crop rectangle (cropX, cropY,
// cropW, cropH inside the origW x origH frame). The crop words are meaningful only in
// d3dsprite.hwl; see re/notes/render.md#billboards.
package hwl

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"io"
	"os"
	"strings"

	"libre-enroth/internal/assets/lod"
)

const (
	nameLen         = 20
	entryHeaderSize = 4 + 8*4
)

// File is an opened .hwl file.
type File struct {
	Path    string
	names   []string
	offsets []int64
	index   map[string]int
	r       io.ReaderAt
	closer  io.Closer
}

// Header is the per-entry header.
type Header struct {
	Packed       uint32
	OrigW, OrigH int             // size of the original image
	W, H         int             // size of the stored pixels
	Crop         image.Rectangle // sprites: the opaque part of the OrigW x OrigH frame
}

// Texture is a decoded hardware texture.
type Texture struct {
	Name         string
	OrigW, OrigH int
	W, H         int
	Crop         image.Rectangle // see Header.Crop
	Pix          []uint16        // ARGB1555, row-major
}

// Open opens an .hwl file.
func Open(path string) (*File, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	h, err := NewFile(f, path)
	if err != nil {
		f.Close()
		return nil, err
	}
	h.closer = f
	return h, nil
}

// NewFile parses an .hwl directory from r.
//
// mm8: 0x450f23 (Hwl_Open)
func NewFile(r io.ReaderAt, path string) (*File, error) {
	var hdr [12]byte
	if _, err := r.ReadAt(hdr[:], 0); err != nil {
		return nil, fmt.Errorf("hwl %s: %w", path, err)
	}
	if string(hdr[:4]) != "D3DT" {
		return nil, fmt.Errorf("hwl %s: bad magic %q", path, hdr[:4])
	}
	le := binary.LittleEndian
	dir := int64(le.Uint32(hdr[4:]))
	var cnt [4]byte
	if _, err := r.ReadAt(cnt[:], dir); err != nil {
		return nil, fmt.Errorf("hwl %s: directory: %w", path, err)
	}
	n := int(int32(le.Uint32(cnt[:])))
	if n < 0 || n > 100000 { // the original's tables hold 50000 entries
		return nil, fmt.Errorf("hwl %s: bad entry count %d", path, n)
	}
	buf := make([]byte, n*(nameLen+4))
	if _, err := r.ReadAt(buf, dir+4); err != nil {
		return nil, fmt.Errorf("hwl %s: directory: %w", path, err)
	}
	h := &File{Path: path, r: r, names: make([]string, n), offsets: make([]int64, n), index: make(map[string]int, n)}
	for i := 0; i < n; i++ {
		name := buf[i*nameLen : (i+1)*nameLen]
		if j := bytes.IndexByte(name, 0); j >= 0 {
			name = name[:j]
		}
		h.names[i] = string(name)
		h.offsets[i] = int64(le.Uint32(buf[n*nameLen+4*i:]))
		if _, dup := h.index[strings.ToLower(h.names[i])]; !dup {
			h.index[strings.ToLower(h.names[i])] = i
		}
	}
	return h, nil
}

// Close releases the underlying file, if Open created it.
func (h *File) Close() error {
	if h.closer == nil {
		return nil
	}
	return h.closer.Close()
}

// Names returns the entry names in directory order.
func (h *File) Names() []string { return h.names }

// Header reads the header of an entry without its pixels.
func (h *File) Header(name string) (Header, error) {
	i, ok := h.index[strings.ToLower(name)]
	if !ok {
		return Header{}, fmt.Errorf("hwl %s: %q: %w", h.Path, name, lod.ErrNotFound)
	}
	var b [entryHeaderSize]byte
	if _, err := h.r.ReadAt(b[:], h.offsets[i]); err != nil {
		return Header{}, fmt.Errorf("hwl %s: %q: %w", h.Path, name, err)
	}
	le := binary.LittleEndian
	u := func(k int) uint32 { return le.Uint32(b[4+4*k:]) }
	return Header{
		Packed: le.Uint32(b[:]),
		OrigW:  int(int32(u(0))), OrigH: int(int32(u(1))),
		W: int(int32(u(4))), H: int(int32(u(5))),
		Crop: image.Rect(int(int32(u(6))), int(int32(u(7))), int(int32(u(6)+u(2))), int(int32(u(7)+u(3)))),
	}, nil
}

// Load reads and decodes a texture. Lookup is case-insensitive like the original's
// _stricmp binary search over the sorted names. The original can also halve the
// texture on load (Hwl_Average4); that is a render setting and not done here.
//
// mm8: 0x451090 (Hwl_LoadTexture)
func (h *File) Load(name string) (*Texture, error) {
	hd, err := h.Header(name)
	if err != nil {
		return nil, err
	}
	if hd.W < 0 || hd.H < 0 || hd.W*hd.H > 1<<24 {
		return nil, fmt.Errorf("hwl %s: %q: bad size %dx%d", h.Path, name, hd.W, hd.H)
	}
	off := h.offsets[h.index[strings.ToLower(name)]] + entryHeaderSize
	n := hd.W * hd.H * 2
	var data []byte
	if hd.Packed == 0 {
		data = make([]byte, n)
		if _, err := h.r.ReadAt(data, off); err != nil {
			return nil, fmt.Errorf("hwl %s: %q: %w", h.Path, name, err)
		}
	} else {
		packed := make([]byte, hd.Packed)
		if _, err := h.r.ReadAt(packed, off); err != nil {
			return nil, fmt.Errorf("hwl %s: %q: %w", h.Path, name, err)
		}
		if data, err = lod.Inflate(packed, n); err != nil {
			return nil, fmt.Errorf("hwl %s: %q: %w", h.Path, name, err)
		}
	}
	t := &Texture{Name: h.names[h.index[strings.ToLower(name)]], OrigW: hd.OrigW, OrigH: hd.OrigH, W: hd.W, H: hd.H, Crop: hd.Crop, Pix: make([]uint16, hd.W*hd.H)}
	for i := range t.Pix {
		t.Pix[i] = binary.LittleEndian.Uint16(data[2*i:])
	}
	return t, nil
}

// ARGB1555 converts one pixel: bit 15 alpha, then 5 bits each of red, green, blue
// (masks 0x7c00/0x3e0/0x1f; cf. Hwl_Average4 0x450fe6).
func ARGB1555(v uint16) color.NRGBA {
	x5 := func(c uint16) uint8 { return uint8(c<<3 | c>>2) }
	a := uint8(0)
	if v&0x8000 != 0 {
		a = 0xff
	}
	return color.NRGBA{x5(v >> 10 & 0x1f), x5(v >> 5 & 0x1f), x5(v & 0x1f), a}
}

// NRGBA converts the texture to an image.
func (t *Texture) NRGBA() *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, t.W, t.H))
	for i, v := range t.Pix {
		c := ARGB1555(v)
		copy(img.Pix[4*i:], []byte{c.R, c.G, c.B, c.A})
	}
	return img
}
