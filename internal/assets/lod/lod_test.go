package lod

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"errors"
	"testing"
)

type synthEntry struct {
	name string
	data []byte
}

// buildLOD assembles a one-chapter LOD in memory.
func buildLOD(version string, entries []synthEntry) []byte {
	entSize, nameLen := entrySizeMM6, nameLenMM6
	if version == VersionMM8 {
		entSize, nameLen = entrySizeMM8, nameLenMM8
	}
	le := binary.LittleEndian
	hdr := make([]byte, headerSize)
	copy(hdr, "LOD\x00")
	copy(hdr[offVersion:], version)
	copy(hdr[offDescription:], "Synthetic test LOD")
	le.PutUint32(hdr[0xa4:], 100)
	le.PutUint32(hdr[offNumChapters:], 1)

	chapOff := headerSize + chapterSize
	dirSize := len(entries) * entSize
	dir := make([]byte, dirSize)
	var blob []byte
	for i, e := range entries {
		rec := dir[i*entSize:]
		copy(rec, e.name)
		le.PutUint32(rec[nameLen:], uint32(dirSize+len(blob)))
		le.PutUint32(rec[nameLen+4:], uint32(len(e.data)))
		blob = append(blob, e.data...)
	}
	chap := make([]byte, chapterSize)
	copy(chap, "chapter")
	le.PutUint32(chap[0x10:], uint32(chapOff))
	le.PutUint32(chap[0x14:], uint32(dirSize+len(blob)))
	le.PutUint16(chap[0x1c:], uint16(len(entries)))

	out := append(hdr, chap...)
	out = append(out, dir...)
	return append(out, blob...)
}

func zlibBytes(b []byte) []byte {
	var buf bytes.Buffer
	w := zlib.NewWriter(&buf)
	w.Write(b)
	w.Close()
	return buf.Bytes()
}

// packedFile builds a packed-file entry: header, payload (zlib if compress), tail.
func packedFile(name string, mm8 bool, w, h int, payload []byte, compress bool, tail []byte) []byte {
	hs, nameLen := FileHeaderSize, 16
	if mm8 {
		hs, nameLen = FileHeaderSizeMM8, 0x40
	}
	le := binary.LittleEndian
	hd := make([]byte, hs)
	copy(hd, name)
	t := hd[nameLen:]
	stored := payload
	if compress {
		stored = zlibBytes(payload)
		le.PutUint32(t[0x18:], uint32(len(payload)))
	}
	le.PutUint32(t[0x00:], uint32(w*h))
	le.PutUint32(t[0x04:], uint32(len(stored)))
	le.PutUint16(t[0x08:], uint16(w))
	le.PutUint16(t[0x0a:], uint16(h))
	le.PutUint16(t[0x14:], 7)
	out := append(hd, stored...)
	return append(out, tail...)
}

func TestArchiveBothEntrySizes(t *testing.T) {
	for _, version := range []string{"MMVI", "GameMMVI", VersionMM8} {
		t.Run(version, func(t *testing.T) {
			mm8 := version == VersionMM8
			long := "a_rather_long_entry_name.txt" // > 16 chars only fits MMVIII
			if !mm8 {
				long = "ab_entry"
			}
			ents := []synthEntry{
				{"Alpha", []byte("first")},
				{"Beta", packedFile("Beta", mm8, 0, 0, []byte("hello packed world"), true, nil)},
				{long, []byte{1, 2, 3}},
			}
			a, err := NewArchive(bytes.NewReader(buildLOD(version, ents)), "synthetic")
			if err != nil {
				t.Fatal(err)
			}
			if a.Version != version || a.Description != "Synthetic test LOD" || a.IsMM8() != mm8 {
				t.Errorf("header: %q %q mm8=%v", a.Version, a.Description, a.IsMM8())
			}
			if len(a.Chapters) != 1 || a.Chapters[0].Name != "chapter" || len(a.Entries) != 3 {
				t.Fatalf("chapters %+v entries %d", a.Chapters, len(a.Entries))
			}
			for _, q := range []string{"alpha", "ALPHA", "Alpha"} {
				b, err := a.Raw(q)
				if err != nil || string(b) != "first" {
					t.Errorf("Raw(%q) = %q, %v", q, b, err)
				}
			}
			if e, ok := a.Find(long); !ok || e.Size != 3 {
				t.Errorf("Find(%q) = %+v, %v", long, e, ok)
			}
			if _, err := a.Raw("missing"); !errors.Is(err, ErrNotFound) {
				t.Errorf("Raw(missing) err = %v", err)
			}
			h, data, tail, err := a.ReadPacked("beta")
			if err != nil {
				t.Fatal(err)
			}
			if h.Name != "Beta" || string(data) != "hello packed world" || len(tail) != 0 || h.PaletteID != 7 {
				t.Errorf("ReadPacked: %+v %q %d", h, data, len(tail))
			}
			raw, _ := a.Raw("beta")
			if !IsPacked(raw, int64(len(raw)), "BETA", mm8) || IsPacked(raw, int64(len(raw)), "other", mm8) {
				t.Error("IsPacked")
			}
		})
	}
}

func TestSplitPackedTail(t *testing.T) {
	pal := bytes.Repeat([]byte{9}, 0x300)
	raw := packedFile("tex", false, 2, 2, []byte{1, 2, 3, 4}, false, pal)
	h, data, tail, err := SplitPacked(raw, false)
	if err != nil {
		t.Fatal(err)
	}
	if h.W != 2 || h.H != 2 || h.BmpSize != 4 || !bytes.Equal(data, []byte{1, 2, 3, 4}) || len(tail) != 0x300 {
		t.Errorf("%+v %v %d", h, data, len(tail))
	}
	if _, _, _, err := SplitPacked(raw[:FileHeaderSize+2], false); err == nil {
		t.Error("truncated payload accepted")
	}
}

func mapBlob(packed, unpacked uint32, body []byte) []byte {
	b := make([]byte, MapHeaderSize)
	binary.LittleEndian.PutUint32(b, MapMagic)
	copy(b[4:], "mvii")
	binary.LittleEndian.PutUint32(b[8:], packed)
	binary.LittleEndian.PutUint32(b[12:], unpacked)
	return append(b, body...)
}

func TestUnpackMap(t *testing.T) {
	plain := bytes.Repeat([]byte("map data "), 50)
	z := zlibBytes(plain)
	got, err := UnpackMap(mapBlob(uint32(len(z)), uint32(len(plain)), z))
	if err != nil || !bytes.Equal(got, plain) {
		t.Errorf("zlib: %v", err)
	}
	got, err = UnpackMap(mapBlob(uint32(len(plain)), uint32(len(plain)), plain))
	if err != nil || !bytes.Equal(got, plain) {
		t.Errorf("stored: %v", err)
	}
	if _, err := UnpackMap(mapBlob(10, 5, plain)); err == nil {
		t.Error("packed > unpacked accepted")
	}
	if _, err := UnpackMap([]byte("not a map blob at all")); err == nil {
		t.Error("bad magic accepted")
	}
}

func TestBadMagic(t *testing.T) {
	b := buildLOD("MMVI", nil)
	b[0] = 'X'
	if _, err := NewArchive(bytes.NewReader(b), "x"); err == nil {
		t.Error("bad magic accepted")
	}
}
