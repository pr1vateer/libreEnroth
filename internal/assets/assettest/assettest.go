// Package assettest locates the game data for tests that need it.
package assettest

import (
	"os"
	"path/filepath"
	"testing"
)

// Dir returns $MM8_DATA, skipping the test when it is unset. A relative path is taken
// relative to the module root (go test runs in each package's directory).
func Dir(t testing.TB) string {
	t.Helper()
	d := os.Getenv("MM8_DATA")
	if d == "" {
		t.Skip("MM8_DATA not set; skipping test that needs the game data")
	}
	if !filepath.IsAbs(d) {
		d = filepath.Join(moduleRoot(t), d)
	}
	if _, err := os.Stat(filepath.Join(d, "Data")); err != nil {
		t.Fatalf("MM8_DATA=%s: %v", d, err)
	}
	return d
}

// File returns the path of a file under Data/.
func File(t testing.TB, name string) string {
	t.Helper()
	return filepath.Join(Dir(t), "Data", name)
}

func moduleRoot(t testing.TB) string {
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}
