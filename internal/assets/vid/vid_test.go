package vid

import (
	"encoding/binary"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestOpenFind(t *testing.T) {
	dir := t.TempDir()
	var b []byte
	b = binary.LittleEndian.AppendUint32(b, 2)
	ent := func(name string, off uint32) {
		var n [40]byte
		copy(n[:], name)
		b = append(b, n[:]...)
		b = binary.LittleEndian.AppendUint32(b, off)
	}
	ent("B.smk", 4+88+3)
	ent("a.SMK", 4+88)
	b = append(b, "aaabbbb"...)
	if err := os.MkdirAll(filepath.Join(dir, "Anims"), 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, "Anims", "MightDoD.vid")
	if err := os.WriteFile(p, b, 0o644); err != nil {
		t.Fatal(err)
	}
	v, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()
	for name, want := range map[string]string{"a.smk": "aaa", "b.SMK": "bbbb"} {
		e, ok := v.Find(name)
		if !ok {
			t.Fatalf("%s not found", name)
		}
		got, _ := io.ReadAll(v.Reader(e))
		if string(got) != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
	if _, ok := v.Find("c.smk"); ok {
		t.Error("found c.smk")
	}
	// OpenSet needs both containers.
	if _, err := OpenSet(dir); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("OpenSet without magicdod.vid: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "Anims", "Magicdod.vid"), b, 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := OpenSet(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := s.Find("A.smk"); err != nil {
		t.Error(err)
	}
	if _, err := s.Find("x.smk"); !errors.Is(err, ErrNotFound) {
		t.Errorf("missing entry: %v", err)
	}
}
