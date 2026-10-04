// Package render is a CPU rasteriser for the 3D view: perspective-correct, bilinear
// texture mapping with per-vertex light and fog, alpha test, and a float32 z-buffer
// holding 1/depth. It knows nothing about the game; internal/game/world feeds it.
//
// Determinism: frames are bit-identical for any number of worker goroutines and on any
// architecture. Per-polygon setup is float64 with every product wrapped in an explicit
// float64(...) conversion (the Go spec lets a compiler fuse x*y+z into an FMA, e.g. on
// arm64, unless a conversion forces the rounding), and the per-pixel loop steps 1/w by
// float64 additions and texture coordinates, light and fog in fixed point.
package render

import (
	"image"
	"unsafe"
)

// Frame is a render target: RGBA pixels and a depth buffer of 1/depth (0 = infinitely
// far, so larger is nearer), and optionally an object-ID (pick) buffer.
type Frame struct {
	W, H int
	Pix  []uint32  // R | G<<8 | B<<16 | A<<24, row-major
	Z    []float32 // 1/depth
	// ID holds the Renderer.SetID value of the primitive that wrote each pixel's depth
	// (0 = none), when EnablePick asked for it.
	ID []uint32
}

// EnablePick allocates the ID buffer.
func (f *Frame) EnablePick() {
	if len(f.ID) != f.W*f.H {
		f.ID = make([]uint32, f.W*f.H)
	}
}

// PickAt returns the ID and the depth (0 when nothing was drawn) at pixel (x, y).
func (f *Frame) PickAt(x, y int) (id uint32, depth float64) {
	if x < 0 || y < 0 || x >= f.W || y >= f.H || f.ID == nil {
		return 0, 0
	}
	i := y*f.W + x
	if z := f.Z[i]; z > 0 {
		depth = 1 / float64(z)
	}
	return f.ID[i], depth
}

// NewFrame allocates a w x h frame.
func NewFrame(w, h int) *Frame {
	f := &Frame{}
	f.Resize(w, h)
	return f
}

// Resize reallocates the buffers when the size changes.
func (f *Frame) Resize(w, h int) {
	w, h = max(w, 1), max(h, 1)
	if f.W == w && f.H == h {
		return
	}
	f.W, f.H = w, h
	f.Pix = make([]uint32, w*h)
	f.Z = make([]float32, w*h)
	if f.ID != nil {
		f.ID = make([]uint32, w*h)
	}
}

// Clear fills the frame with c and resets the depth buffer.
func (f *Frame) Clear(c uint32) {
	for i := range f.Pix {
		f.Pix[i] = c
	}
	clear(f.Z)
	clear(f.ID)
}

// Bytes returns the pixels as RGBA bytes (a view, not a copy), for
// ebiten.Image.WritePixels and hashing.
func (f *Frame) Bytes() []byte {
	if len(f.Pix) == 0 {
		return nil
	}
	return unsafe.Slice((*byte)(unsafe.Pointer(&f.Pix[0])), 4*len(f.Pix))
}

// RGBA copies the frame into an image.
func (f *Frame) RGBA() *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, f.W, f.H))
	copy(img.Pix, f.Bytes())
	return img
}

// RGB packs an opaque colour.
func RGB(r, g, b uint8) uint32 { return uint32(r) | uint32(g)<<8 | uint32(b)<<16 | 0xff000000 }
