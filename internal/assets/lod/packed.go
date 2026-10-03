package lod

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"fmt"
	"io"
	"strings"
)

// Header sizes of "packed file" entries: textures, icons, palettes, .pcx/.fnt/.txt/.str ...
const (
	FileHeaderSize    = 0x30 // MMVI: name[16] + 0x20-byte tail
	FileHeaderSizeMM8 = 0x60 // MMVIII: name[0x40] + the same tail
)

// FileFlagMips marks a bitmap that stores 4 mip levels (w*h, /4, /16, /64).
//
// mm8: 0x4113e0 (Texture_Load tests flags & 2)
const FileFlagMips = 0x2

// FileHeader is the normalised header of a packed file (TextureHeader / TextureHeaderMM8 in
// re/symbols/types.h).
type FileHeader struct {
	Name         string
	BmpSize      int32 // w*h of mip 0
	DataSize     int32 // payload size on disk
	W, H         int
	WLog2, HLog2 int
	WMask, HMask int
	PaletteID    int
	UnpackedSize int32 // 0 = stored raw, else zlib-compressed
	Flags        int32
}

// ParseFileHeader decodes a 0x30-byte (mm8 == false) or 0x60-byte header.
//
// mm8: 0x4113e0 (0x30 header), 0x4121c9 (Lod_LoadLanguageFile, 0x60 header)
func ParseFileHeader(b []byte, mm8 bool) (FileHeader, error) {
	size, nameLen := FileHeaderSize, 16
	if mm8 {
		size, nameLen = FileHeaderSizeMM8, 0x40
	}
	if len(b) < size {
		return FileHeader{}, fmt.Errorf("file header: %d bytes, need %d", len(b), size)
	}
	t := b[nameLen:size]
	le := binary.LittleEndian
	i16 := func(o int) int { return int(int16(le.Uint16(t[o:]))) }
	return FileHeader{
		Name:         cstring(b[:nameLen]),
		BmpSize:      int32(le.Uint32(t[0x00:])),
		DataSize:     int32(le.Uint32(t[0x04:])),
		W:            i16(0x08),
		H:            i16(0x0a),
		WLog2:        i16(0x0c),
		HLog2:        i16(0x0e),
		WMask:        i16(0x10),
		HMask:        i16(0x12),
		PaletteID:    i16(0x14),
		UnpackedSize: int32(le.Uint32(t[0x18:])),
		Flags:        int32(le.Uint32(t[0x1c:])),
	}, nil
}

// SplitPacked splits a packed-file entry into header, (inflated) payload and the bytes
// that follow the payload (the 0x300-byte RGB palette of textures and icons, else empty).
func SplitPacked(raw []byte, mm8 bool) (FileHeader, []byte, []byte, error) {
	h, err := ParseFileHeader(raw, mm8)
	if err != nil {
		return h, nil, nil, err
	}
	hs := FileHeaderSize
	if mm8 {
		hs = FileHeaderSizeMM8
	}
	if h.DataSize < 0 || int64(hs)+int64(h.DataSize) > int64(len(raw)) {
		return h, nil, nil, fmt.Errorf("%q: payload size %d exceeds entry (%d bytes)", h.Name, h.DataSize, len(raw))
	}
	payload := raw[hs : hs+int(h.DataSize)]
	tail := raw[hs+int(h.DataSize):]
	if h.UnpackedSize != 0 {
		payload, err = Inflate(payload, int(h.UnpackedSize))
		if err != nil {
			return h, nil, nil, fmt.Errorf("%q: %w", h.Name, err)
		}
	}
	return h, payload, tail, nil
}

// IsPacked reports whether an entry of size bytes, starting with head, looks like a
// packed file named name: the header name matches and the payload is followed by nothing
// or by a 0x300-byte palette. This holds for every entry of icons, bitmaps, EnglishT and
// EnglishD.
func IsPacked(head []byte, size int64, name string, mm8 bool) bool {
	h, err := ParseFileHeader(head, mm8)
	if err != nil || !strings.EqualFold(h.Name, name) || h.DataSize < 0 {
		return false
	}
	hs := FileHeaderSize
	if mm8 {
		hs = FileHeaderSizeMM8
	}
	rest := size - int64(hs) - int64(h.DataSize)
	return rest == 0 || rest == 0x300
}

// ReadPacked reads a packed-file entry: header, inflated payload, and trailing bytes.
func (a *Archive) ReadPacked(name string) (FileHeader, []byte, []byte, error) {
	raw, err := a.Raw(name)
	if err != nil {
		return FileHeader{}, nil, nil, err
	}
	h, data, tail, err := SplitPacked(raw, a.IsMM8())
	if err != nil {
		return h, nil, nil, fmt.Errorf("lod %s: %w", a.Path, err)
	}
	return h, data, tail, nil
}

// Inflate zlib-decompresses packed, which must expand to exactly unpackedSize bytes.
//
// mm8: 0x4d4d00 (zlib 1.1.3 uncompress)
func Inflate(packed []byte, unpackedSize int) ([]byte, error) {
	zr, err := zlib.NewReader(bytes.NewReader(packed))
	if err != nil {
		return nil, fmt.Errorf("zlib: %w", err)
	}
	defer zr.Close()
	out := make([]byte, unpackedSize)
	if _, err := io.ReadFull(zr, out); err != nil {
		return nil, fmt.Errorf("zlib: want %d bytes: %w", unpackedSize, err)
	}
	return out, nil
}

// Map blob header (games.lod .odm/.blv/.ddm/.dlv): u32 0x16741, "mvii", packed, unpacked.
const (
	MapMagic      = 0x16741
	MapHeaderSize = 16
)

// IsMap reports whether raw starts with the mvii map header.
func IsMap(raw []byte) bool {
	return len(raw) >= MapHeaderSize && binary.LittleEndian.Uint32(raw) == MapMagic &&
		string(raw[4:8]) == "mvii"
}

// UnpackMap returns the payload of an mvii map blob. packed < unpacked means zlib,
// packed == unpacked means stored; anything else is rejected ("Can't load file!").
//
// mm8: 0x47df28 (Odm_Load), 0x498050 (Blv_Load)
func UnpackMap(raw []byte) ([]byte, error) {
	if !IsMap(raw) {
		return nil, fmt.Errorf("map: missing mvii header")
	}
	packed := int64(binary.LittleEndian.Uint32(raw[8:]))
	unpacked := int64(binary.LittleEndian.Uint32(raw[12:]))
	body := raw[MapHeaderSize:]
	if packed > int64(len(body)) {
		return nil, fmt.Errorf("map: packed size %d exceeds blob (%d bytes)", packed, len(body))
	}
	switch {
	case packed == unpacked:
		return body[:packed], nil
	case packed < unpacked:
		return Inflate(body[:packed], int(unpacked))
	default:
		return nil, fmt.Errorf("map: packed %d > unpacked %d", packed, unpacked)
	}
}
