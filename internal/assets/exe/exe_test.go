package exe_test

import (
	"path/filepath"
	"testing"

	"libre-enroth/internal/assets/assettest"
	"libre-enroth/internal/assets/exe"
)

func TestReadTables(t *testing.T) {
	im, err := exe.Open(filepath.Join(assettest.Dir(t), "MM8-Rel.exe"))
	if err != nil {
		t.Fatal(err)
	}
	defer im.Close()
	// re/notes/ui.md: "%s01" formats the creation portraits, pc01- .. pc30- feed it.
	if s, err := im.CString(0x500db0); err != nil || s != "%s01" {
		t.Errorf("CString(0x500db0) = %q, %v", s, err)
	}
	names, err := im.CStringTable(0x4feb88, 30)
	if err != nil || names[0] != "pc01-" || names[29] != "pc30-" {
		t.Errorf("portrait prefixes = %q, %v", names, err)
	}
	base, err := im.Int32s(0x4f7858, 2)
	if err != nil || base[0] != 467 || base[1] != 23 {
		t.Errorf("paper doll base = %v, %v", base, err)
	}
	if _, err := im.Read(0x100, 4); err == nil {
		t.Error("Read below the image succeeded")
	}
}
