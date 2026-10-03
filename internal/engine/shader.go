package engine

// sharpBilinear scales pixel art by a non-integer factor without uneven pixel widths:
// inside each source texel the colour is flat, and only a band 1/Scale texels wide at
// each texel edge is interpolated (the "sharp bilinear" technique). Pixel units: srcPos
// is in source pixels.
var sharpBilinear = []byte(`//kage:unit pixels

package main

var Scale float

func tap(i vec2) vec4 {
	return imageSrc0At(imageSrc0Origin() + clamp(i, vec2(0), imageSrc0Size()-1) + 0.5)
}

func Fragment(dstPos vec4, srcPos vec2, color vec4) vec4 {
	t := srcPos - imageSrc0Origin()
	fl := floor(t)
	cd := t - fl - 0.5
	r := 0.5 - 0.5/Scale
	q := fl + (cd-clamp(cd, -r, r))*Scale + 0.5 - 0.5
	i0 := floor(q)
	w := q - i0
	top := mix(tap(i0), tap(i0+vec2(1, 0)), w.x)
	bottom := mix(tap(i0+vec2(0, 1)), tap(i0+vec2(1, 1)), w.x)
	return mix(top, bottom, w.y)
}
`)
