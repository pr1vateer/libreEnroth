package render

import "image"

// Texture is an RGBA texture with premultiplied alpha (a transparent texel is 0), so
// bilinear filtering never bleeds the colour of transparent texels.
type Texture struct {
	W, H   int
	Pix    []uint32 // premultiplied R | G<<8 | B<<16 | A<<24
	Alpha  bool     // some texel is not opaque
	mw, mh int      // W-1, H-1 when both are powers of two, else -1
}

func newTexture(w, h int) *Texture {
	t := &Texture{W: w, H: h, Pix: make([]uint32, w*h), mw: -1, mh: -1}
	if w > 0 && h > 0 && w&(w-1) == 0 && h&(h-1) == 0 {
		t.mw, t.mh = w-1, h-1
	}
	return t
}

// FromARGB1555 converts Direct3D texels (bit 15 alpha, 5:5:5 colour), as stored in
// d3dbitmap.hwl / d3dsprite.hwl.
func FromARGB1555(w, h int, pix []uint16) *Texture {
	t := newTexture(w, h)
	for i, v := range pix[:w*h] {
		if v&0x8000 == 0 {
			t.Alpha = true
			continue
		}
		x5 := func(c uint16) uint32 { c &= 0x1f; return uint32(c<<3 | c>>2) }
		t.Pix[i] = x5(v>>10) | x5(v>>5)<<8 | x5(v)<<16 | 0xff000000
	}
	return t
}

// FromImage converts any image (transparent where alpha < 128).
func FromImage(img image.Image) *Texture {
	b := img.Bounds()
	t := newTexture(b.Dx(), b.Dy())
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			r, g, bl, a := img.At(b.Min.X+x, b.Min.Y+y).RGBA()
			if a < 0x8000 {
				t.Alpha = true
				continue
			}
			// Colour components are premultiplied already; alpha is snapped to opaque.
			r, g, bl = r*0xffff/a, g*0xffff/a, bl*0xffff/a
			t.Pix[y*t.W+x] = r>>8 | (g>>8)<<8 | (bl>>8)<<16 | 0xff000000
		}
	}
	return t
}

// FromRGBA wraps already converted texels (premultiplied, see Texture).
func FromRGBA(w, h int, pix []uint32) *Texture {
	t := newTexture(w, h)
	copy(t.Pix, pix)
	for _, p := range t.Pix {
		if p>>24 != 0xff {
			t.Alpha = true
			break
		}
	}
	return t
}

// wrap maps a texel coordinate into 0..n-1 (repeat).
func wrap(v, n, mask int) int {
	if mask >= 0 {
		return v & mask
	}
	v %= n
	if v < 0 {
		v += n
	}
	return v
}

func clampi(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// sample returns the bilinear texel at u, v (16.16 fixed point texel coordinates,
// texel centres at +0.5).
func (t *Texture) sample(u, v int64, clampUV bool) uint32 {
	u -= 0x8000
	v -= 0x8000
	fx := uint32(u>>8) & 0xff
	fy := uint32(v>>8) & 0xff
	x0, y0 := int(u>>16), int(v>>16)
	var x1, y1 int
	if clampUV {
		x0, x1 = clampi(x0, 0, t.W-1), clampi(x0+1, 0, t.W-1)
		y0, y1 = clampi(y0, 0, t.H-1), clampi(y0+1, 0, t.H-1)
	} else {
		x1 = wrap(x0+1, t.W, t.mw)
		y1 = wrap(y0+1, t.H, t.mh)
		x0 = wrap(x0, t.W, t.mw)
		y0 = wrap(y0, t.H, t.mh)
	}
	r0, r1 := y0*t.W, y1*t.W
	return bilerp(t.Pix[r0+x0], t.Pix[r0+x1], t.Pix[r1+x0], t.Pix[r1+x1], fx, fy)
}

// lerp2 blends two packed colours by f/256, two channels at a time.
func lerp2(a, b, f uint32) uint32 {
	g := 256 - f
	rb := ((a&0x00ff00ff)*g + (b&0x00ff00ff)*f) >> 8 & 0x00ff00ff
	ga := ((a>>8&0x00ff00ff)*g + (b>>8&0x00ff00ff)*f) & 0xff00ff00
	return rb | ga
}

func bilerp(c00, c10, c01, c11, fx, fy uint32) uint32 {
	return lerp2(lerp2(c00, c10, fx), lerp2(c01, c11, fx), fy)
}
