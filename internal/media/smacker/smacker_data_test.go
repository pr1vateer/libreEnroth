package smacker_test

import (
	"crypto/sha256"
	"fmt"
	"image"
	"image/draw"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"libre-enroth/internal/assets/assettest"
	"libre-enroth/internal/assets/vid"
	"libre-enroth/internal/media/smacker"
)

func openSet(t *testing.T) vid.Set {
	t.Helper()
	s, err := vid.OpenSet(assettest.Dir(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// Every .smk entry of both containers decodes all its frames, then the ring frame.
func TestDecodeAll(t *testing.T) {
	s := openSet(t)
	if len(s) != 2 || len(s[0].Entries) != 91 || len(s[1].Entries) != 51 {
		t.Fatalf("containers: %d", len(s))
	}
	clips, frames, truncated := 0, 0, 0
	sizes := map[string]int{}
	for _, v := range s {
		for _, e := range v.Entries {
			if !strings.EqualFold(filepath.Ext(e.Name), ".smk") {
				continue
			}
			sv, err := smacker.Open(v.Reader(e))
			if err != nil {
				t.Errorf("%s: %v", e.Name, err)
				continue
			}
			sizes[fmt.Sprintf("SMK%c %dx%d", sv.Version, sv.Width, sv.Height)]++
			sv.Loop = true
			for i := 0; i <= sv.Frames; i++ {
				if _, err := sv.NextFrame(); err != nil {
					t.Errorf("%s: frame %d: %v", e.Name, i, err)
					break
				}
			}
			clips++
			frames += sv.Frames
			truncated += sv.Truncated
		}
	}
	// cquixote.smk's ring frame is the one truncated frame.
	if clips != 129 || frames != 6991 || truncated != 1 {
		t.Errorf("%d clips, %d frames, %d truncated", clips, frames, truncated)
	}
	if want := "map[SMK2 460x344:127 SMK2 640x480:1 SMK4 460x344:1]"; fmt.Sprint(sizes) != want {
		t.Errorf("sizes %v, want %s", sizes, want)
	}
}

func frameHash(t *testing.T, s vid.Set, name string, frame int) (string, *smacker.Video) {
	t.Helper()
	r, err := s.Find(name)
	if err != nil {
		t.Fatal(err)
	}
	sv, err := smacker.Open(r)
	if err != nil {
		t.Fatal(err)
	}
	sv.Loop = true
	for i := 0; ; i++ {
		img, err := sv.NextFrame()
		if err != nil {
			t.Fatal(err)
		}
		if i < frame {
			continue
		}
		h := sha256.New()
		h.Write(img.Pix)
		for _, c := range img.Palette {
			r, g, b, _ := c.RGBA()
			h.Write([]byte{byte(r >> 8), byte(g >> 8), byte(b >> 8)})
		}
		return fmt.Sprintf("%x", h.Sum(nil)[:8]), sv
	}
}

func TestFrozenFrames(t *testing.T) {
	s := openSet(t)
	for _, c := range []struct {
		name   string
		frame  int
		frames int
		want   string
	}{
		{"detavern.smk", 0, 64, "904ae68beedfba67"},
		{"detavern.smk", 63, 64, "2eb285036dc27738"},
		{"LTEMPLE.SMK", 0, 94, "dc936a5c1d9a4856"},
		{"ltemple.smk", 64, 94, "fd1b6523e7e2e144"},
		{"stables.smk", 0, 40, "ce094cac69586f09"},
		{"stables.smk", 39, 40, "8a06001c72bd80c4"},
	} {
		got, sv := frameHash(t, s, c.name, c.frame)
		if sv.Frames != c.frames || got != c.want {
			t.Errorf("%s frame %d: %d frames, hash %s; want %d, %s", c.name, c.frame, sv.Frames, got, c.frames, c.want)
		}
	}
}

// The ring frame brings a looping clip back to (a lossy copy of) frame 0's image.
func TestRingFrame(t *testing.T) {
	s := openSet(t)
	r, err := s.Find("stables.smk")
	if err != nil {
		t.Fatal(err)
	}
	sv, err := smacker.Open(r)
	if err != nil {
		t.Fatal(err)
	}
	sv.Loop = true
	img, _ := sv.NextFrame()
	f0 := image.NewRGBA(img.Rect)
	draw.Draw(f0, f0.Rect, img, image.Point{}, draw.Src)
	for range sv.Frames {
		if img, err = sv.NextFrame(); err != nil {
			t.Fatal(err)
		}
	}
	if sv.FrameNumber() != 0 {
		t.Errorf("ring frame shows as frame %d", sv.FrameNumber())
	}
	diff := 0
	for y := range img.Rect.Dy() {
		for x := range img.Rect.Dx() {
			if f0.At(x, y) != img.Palette[img.ColorIndexAt(x, y)] {
				diff++
			}
		}
	}
	if diff == 0 || diff > len(img.Pix)/10 {
		t.Errorf("ring frame differs from frame 0 in %d pixels", diff)
	}
	sv.Loop = false
	sv.Rewind()
	n := 0
	for {
		if _, err := sv.NextFrame(); err == io.EOF {
			break
		} else if err != nil {
			t.Fatal(err)
		}
		n++
	}
	if n != 40 {
		t.Errorf("without Loop: %d frames", n)
	}
}
