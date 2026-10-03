package desc

import (
	"encoding/binary"
	"testing"

	"libre-enroth/internal/assets"
	"libre-enroth/internal/assets/assettest"
)

func put16(b []byte, o, v int) { binary.LittleEndian.PutUint16(b[o:], uint16(v)) }

func TestTiles(t *testing.T) {
	b := make([]byte, 4+3*0x1a)
	binary.LittleEndian.PutUint32(b, 3)
	for i, x := range []struct {
		name            string
		set, sect, attr int
	}{{"pending", 0, 254, 0}, {"grastyl", 0, 0, 0}, {"wtrtyl", 5, 0, TileWater}} {
		r := b[4+i*0x1a:]
		copy(r, x.name)
		put16(r, 0x14, x.set)
		put16(r, 0x16, x.sect)
		put16(r, 0x18, x.attr)
	}
	tl, err := ParseTiles(b)
	if err != nil {
		t.Fatal(err)
	}
	if tl[2].Name != "wtrtyl" || tl[2].Attr != TileWater || tl.First(5) != 2 || tl.First(0) != 1 || tl.First(9) != 0 {
		t.Errorf("tiles %+v", tl)
	}
	if _, err := ParseTiles(b[:20]); err == nil {
		t.Error("truncated table accepted")
	}
}

func TestSFT(t *testing.T) {
	// One 3-frame sequence (times 2, 3, 4 = length 9) and a single frame.
	b := make([]byte, 8+4*0x3c+2)
	binary.LittleEndian.PutUint32(b, 4)
	binary.LittleEndian.PutUint32(b[4:], 1)
	for i, x := range []struct {
		name         string
		flags        uint32
		time, length int
	}{{"fire", FrameHasMore, 2, 9}, {"fire", FrameHasMore, 3, 0}, {"fire", 0, 4, 0}, {"tree", FrameOneView, 0, 0}} {
		r := b[8+i*0x3c:]
		copy(r, x.name)
		copy(r[12:], x.name)
		binary.LittleEndian.PutUint32(r[0x28:], 0x10000)
		binary.LittleEndian.PutUint32(r[0x2c:], x.flags)
		put16(r, 0x36, x.time)
		put16(r, 0x38, x.length)
	}
	s, err := ParseSFT(b)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ tick, want int }{{0, 0}, {15, 0}, {16, 0}, {24, 1}, {40, 1}, {48, 2}, {71, 2}, {72, 0}, {1000, 2}} {
		if got := s.At(0, c.tick); got != c.want {
			t.Errorf("At(0, %d) = %d, want %d", c.tick, got, c.want)
		}
	}
	if s.At(3, 999) != 3 {
		t.Error("single frame animated")
	}
}

func TestViewNames(t *testing.T) {
	f := Frame{Sprite: "dec", Flags: FrameMirror0 << 5}
	names, mirror := f.ViewNames()
	if names[0] != "dec0" || names[5] != "dec3" || !mirror[5] || mirror[3] {
		t.Errorf("mirror: %v %v", names, mirror)
	}
	f.Flags = FrameThreeView
	if names, _ = f.ViewNames(); names != [8]string{"dec0", "dec0", "dec2", "dec4", "dec4", "dec4", "dec2", "dec0"} {
		t.Errorf("three views: %v", names)
	}
	f = Frame{Sprite: "monfid", Flags: FrameFidget}
	if names, _ = f.ViewNames(); names[3] != "monstA3" || names[4] != "monstA4" || names[1] != "monfid1" {
		t.Errorf("fidget: %v", names)
	}
	f.Flags = FrameOneView
	if names, _ = f.ViewNames(); names[6] != "monfid" {
		t.Errorf("one view: %v", names)
	}
}

func TestTFT(t *testing.T) {
	b := make([]byte, 4+3*0x14)
	binary.LittleEndian.PutUint32(b, 3)
	for i, x := range []struct {
		name               string
		time, length, flag int
	}{{"lava1", 8, 16, 1}, {"lava2", 8, 0, 0}, {"still", 0, 0, 0}} {
		r := b[4+i*0x14:]
		copy(r, x.name)
		put16(r, 0xe, x.time)
		put16(r, 0x10, x.length)
		put16(r, 0x12, x.flag)
	}
	tf, err := ParseTFT(b)
	if err != nil {
		t.Fatal(err)
	}
	if tf.Find("LAVA1") != 0 || tf.Find("nope") != -1 {
		t.Error("Find")
	}
	if tf.At(0, 0) != 0 || tf.At(0, 8*9) != 1 || tf.At(0, 8*16) != 0 || tf.At(2, 500) != 2 {
		t.Errorf("At: %d %d %d", tf.At(0, 0), tf.At(0, 8*9), tf.At(0, 8*16))
	}
}

// TestShipped parses the tables of EnglishT.lod.
func TestShipped(t *testing.T) {
	d, err := assets.OpenAll(assettest.Dir(t))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	load := func(name string) []byte {
		_, b, err := d.LangFile(name)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	for _, c := range []struct {
		name string
		n    int
	}{{"dtile.bin", 882}, {"dtile2.bin", 450}, {"dtile3.bin", 450}} {
		tl, err := ParseTiles(load(c.name))
		if err != nil || len(tl) != c.n {
			t.Errorf("%s: %d tiles, %v", c.name, len(tl), err)
		}
	}
	dl, err := ParseDecList(load("ddeclist.bin"))
	if err != nil || len(dl) != 286 {
		t.Fatalf("ddeclist: %d, %v", len(dl), err)
	}
	sft, err := ParseSFT(load("dsft.bin"))
	if err != nil || len(sft.Frames) != 7641 || len(sft.EIndex) != 1775 {
		t.Fatalf("dsft: %v", err)
	}
	for i, dec := range dl {
		if dec.SFT < 0 || dec.SFT >= len(sft.Frames) {
			t.Errorf("decoration %d (%s): sft %d", i, dec.Name, dec.SFT)
		}
	}
	tf, err := ParseTFT(load("dtft.bin"))
	if err != nil || len(tf) != 25 {
		t.Fatalf("dtft: %d, %v", len(tf), err)
	}
}
