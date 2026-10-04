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

func pftBytes(recs [][5]int) []byte {
	b := make([]byte, 4+10*len(recs))
	binary.LittleEndian.PutUint32(b, uint32(len(recs)))
	for i, r := range recs {
		for j, v := range r {
			put16(b, 4+10*i+2*j, v)
		}
	}
	return b
}

func TestPFT(t *testing.T) {
	// expr 0 and 1 single frames; expr 9 a 3-record sequence (times 2, 3, 4 = 9).
	p, err := ParsePFT(pftBytes([][5]int{
		{0, 1, 8, 8, PlayerFrameNew}, {1, 1, 8, 8, PlayerFrameNew},
		{9, 5, 2, 9, PlayerFrameNew | PlayerFrameMore}, {0, 6, 3, 0, PlayerFrameMore}, {0, 7, 4, 0, 0},
	}))
	if err != nil {
		t.Fatal(err)
	}
	if p.Find(1) != 1 || p.Find(9) != 2 || p.Find(0x62) != 0 {
		t.Errorf("Find: %d %d %d", p.Find(1), p.Find(9), p.Find(0x62))
	}
	// rem = (t >> 3) % 9; a record shows while its Time >= the remainder left.
	for _, c := range []struct{ time, want int }{{0, 2}, {23, 2}, {24, 3}, {40, 3}, {48, 4}, {71, 4}, {72, 2}, {16 * 8, 4}} {
		if got := p.FrameAt(2, c.time); got != c.want {
			t.Errorf("FrameAt(2, %d) = %d, want %d", c.time, got, c.want)
		}
	}
	if p.FrameAt(1, 1000) != 1 {
		t.Error("single frame animated")
	}
	if _, err := ParsePFT(pftBytes(nil)[:3]); err == nil {
		t.Error("truncated table accepted")
	}
}

func TestPFTTalk(t *testing.T) {
	recs := make([][5]int, 0x19)
	for i := range recs {
		recs[i] = [5]int{i, i, 8, 8, PlayerFrameNew}
	}
	recs[0x15][2], recs[0x16][2] = 2, 3
	p, _ := ParsePFT(pftBytes(recs))
	frame, time := 0x15, 0
	if p.TalkFrame(&frame, &time, 15, func() int { panic("rand") }) != 0x15 || time != 15 {
		t.Fatalf("within the frame: %#x %d", frame, time)
	}
	// 15 + 3 = 18 >= 2*8: rand 5 -> 0x16, time (18 % 3) << 3 = 0.
	if p.TalkFrame(&frame, &time, 3, func() int { return 5 }) != 0x16 || time != 0 {
		t.Fatalf("next: %#x %d", frame, time)
	}
	time = 20
	if p.TalkFrame(&frame, &time, 5, func() int { return 7 }) != 0x18 || time != (25%8)<<3 {
		t.Fatalf("next: %#x %d", frame, time)
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
	// dpft.bin: 764 bytes = 4 + 76 x 10. Expression 1 is the plain face (texture 1),
	// 0x15..0x18 the talking frames; the condition faces 0x62/0x63 are not in it.
	pft, err := ParsePFT(load("dpft.bin"))
	if err != nil || len(pft) != 76 {
		t.Fatalf("dpft: %d, %v", len(pft), err)
	}
	if pft[pft.Find(1)].Texture != 1 || pft.Find(0x15) != 0x15 || pft.Find(0x62) != 0 {
		t.Errorf("dpft: expr 1 %+v, talk %d", pft[pft.Find(1)], pft.Find(0x15))
	}
	for i, f := range pft {
		if f.Texture < 1 || f.Texture > 56 {
			t.Errorf("dpft %d: texture %d", i, f.Texture)
		}
	}
}
