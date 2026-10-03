// Package physics is the original's collision and floor code, ported in the same 32-bit
// integer and 16.16 fixed-point arithmetic (re/notes/physics.md). It knows maps (blv,
// odm) but nothing about rendering or the party; internal/game/party drives it.
package physics

import "math"

// Mul16 is a 16.16 product: the low 32 bits of the 64-bit product shifted right by 16
// (IMUL; SHRD EAX,EDX,16).
func Mul16(a, b int32) int32 { return int32(int64(a) * int64(b) >> 16) }

// Div16 is a 16.16 quotient: (a << 16) / b on 64 bits, truncated towards zero (IDIV),
// keeping the low 32 bits. b must not be 0.
func Div16(a, b int32) int32 { return int32((int64(a) << 16) / int64(b)) }

func abs32(v int32) int32 {
	if v < 0 {
		return -v
	}
	return v
}

// Isqrt is the integer square root, rounded to nearest; negative inputs (as int32) and
// 0, 1 return themselves.
//
// mm8: 0x451690 (Math_Isqrt)
func Isqrt(v uint32) uint32 {
	if int32(v) < 2 {
		return v
	}
	var rem, root, x uint32 = 0, 0, v
	var r2 uint32
	for range 16 {
		r2 = root * 2
		rem = rem<<2 | x>>30
		root = root*4 + 1
		x <<= 2
		if root <= rem {
			r2++
			rem -= root
		}
		root = r2
	}
	if r2-1 <= v-r2*r2 {
		r2++
	}
	return r2
}

// Angle units: 2048 per turn.
const (
	HalfTurn    = 0x400
	QuarterTurn = 0x200
	FullTurn    = 0x800
	AngleMask   = 0x7ff
)

const tableLen = 0x208

// The g_mathTables arrays: tan, cos and sec of i*pi/1024 for i < 512, in 16.16 and
// rounded (+0.5, truncated); the tails are padding.
var tanTable, cosTable, secTable [tableLen]int32

// mm8: 0x45155b (MathTables_Init)
func init() {
	step := math.Pi * 0.0009765625 // DAT_004ea590 * DAT_004ea6a8
	tanTable[0], cosTable[0], secTable[0] = 0, 0x10000, 0x10000
	for i := 1; i < QuarterTurn; i++ {
		x := float64(i) * step
		tanTable[i] = int32(math.Tan(x)*65536 + 0.5)
		cosTable[i] = int32(math.Cos(x)*65536 + 0.5)
		secTable[i] = int32(1/math.Cos(x)*65536 + 0.5)
	}
	for i := QuarterTurn; i < tableLen; i++ {
		tanTable[i], cosTable[i], secTable[i] = -0x10000001, 0, -0x10000001 // 0xefffffff
	}
}

// Cos is the 16.16 cosine of an angle in 2048ths of a turn.
//
// mm8: 0x402e04 (Math_Cos)
func Cos(a int32) int32 {
	u := a & AngleMask
	if u > HalfTurn {
		u = FullTurn - u
	}
	if u < QuarterTurn {
		return cosTable[u]
	}
	return -cosTable[HalfTurn-u]
}

// Sin is Cos(a - QuarterTurn), the way the movement code takes sines.
func Sin(a int32) int32 { return Cos(a - QuarterTurn) }

// Atan2 is the angle of (x, y) in 2048ths of a turn, 0..2047, found by a binary then a
// linear search of the tangent table.
//
// mm8: 0x4513ec (Math_Atan2)
func Atan2(x, y int32) int32 {
	ay := y
	if abs32(x) < 0x10000 && abs32(x) <= abs32(y)>>15 {
		x = 0
	}
	if x == 0 {
		if y > 0 {
			return QuarterTurn
		}
		return QuarterTurn + HalfTurn
	}
	if y == 0 {
		if x > 0 {
			return 0
		}
		return HalfTurn
	}
	var quad int
	switch {
	case x < 1 && y >= 0:
		x, quad = -x, 4
	case x < 1:
		x, quad, ay = -x, 3, -y
	case y >= 0:
		quad = 1
	default:
		quad, ay = 2, -y
	}
	t := Div16(ay, x)
	lo, hi := int32(0), int32(QuarterTurn>>1)
	if tanTable[hi] < t {
		lo, hi = hi, QuarterTurn
	}
	for range 5 {
		mid := (lo + hi) >> 1
		if tanTable[mid] < t {
			lo = mid
		} else {
			hi = mid
		}
	}
	for lo++; lo < hi-1 && t >= tanTable[lo]; lo++ {
	}
	switch quad {
	case 2:
		return FullTurn - lo
	case 3:
		return HalfTurn + lo
	case 4:
		return HalfTurn - lo
	}
	return lo
}
